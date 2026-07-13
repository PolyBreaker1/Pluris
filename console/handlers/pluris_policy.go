package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/web/templates"
)

// Pluris Policy handlers (Task 6): the role list/detail pages plus
// clone/save/delete mutations. See
// docs/history/specs/2026-07-08-pluris-policy-authz-design.md
// section "4. Pluris Policy UI". Route-level RBAC already gates
// "/policy/pluris" on console_access.view_roles (pkg/auth/rbac.go); the
// handler-level requirePermission calls below are defense in depth and
// the only gate for the manage_permissions-scoped mutations.

// PlurisPolicy renders the role list. EnsureBuiltins/EnsureBuiltinGrants
// are called best-effort so a tenant created before this feature (or
// before builtin grants existed) still gets a populated page.
func (h *Handler) PlurisPolicy(c echo.Context) error {
	if err := requirePermission(c, "console_access.view_roles"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)

	_ = h.roleSvc.EnsureBuiltins(ctx, sess.TenantID)
	_ = h.authzSvc.EnsureBuiltinGrants(ctx, sess.TenantID)

	roles, err := h.db.Queries.ListRolesByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}

	byID := make(map[int64]db.Role, len(roles))
	for _, r := range roles {
		byID[r.ID] = r
	}

	rows := make([]templates.PlurisRoleRow, 0, len(roles))
	for _, r := range roles {
		members, err := h.db.Queries.ListIdentitiesForRole(ctx, r.ID)
		if err != nil {
			return err
		}
		parentName := "—"
		if r.ParentRoleID.Valid {
			if parent, ok := byID[r.ParentRoleID.Int64]; ok {
				parentName = parent.Name
			}
		}
		rows = append(rows, templates.PlurisRoleRow{
			ID:             r.ID,
			Name:           r.Name,
			Slug:           r.Slug,
			Builtin:        r.IsBuiltin,
			Members:        int64(len(members)),
			GrantedCount:   countGranted(authz.Parse(r.Permissions)),
			Parent:         parentName,
			TemplateFamily: roleTemplateFamily(r),
		})
	}

	return render(c, templates.PlurisPolicyPage(rows, roles))
}

// roleTemplateFamily mirrors the grouping rule in
// web/templates/role_picker_helpers.go's rolePickerOptions (unexported
// there, so re-derived here for the list page's search blob): builtin
// roles are their own family root (their own slug); custom roles belong
// to their TemplateSlug family, falling back to "custom" when unset.
func roleTemplateFamily(r db.Role) string {
	if r.IsBuiltin {
		return r.Slug
	}
	if r.TemplateSlug.Valid && r.TemplateSlug.String != "" {
		return r.TemplateSlug.String
	}
	return "custom"
}

// PlurisPolicyDetail renders a single role: its parsed grants, assigned
// members/groups, and everything Task 6's origin-badge / parent-chain UI
// needs (own overrides, ancestor chain, per-key origin labels, and the
// pool of roles eligible to become this role's parent).
func (h *Handler) PlurisPolicyDetail(c echo.Context) error {
	if err := requirePermission(c, "console_access.view_roles"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	role, err := h.resolveTenantRole(c, c.Param("id"))
	if err != nil {
		return err
	}
	members, err := h.db.Queries.ListIdentitiesForRole(ctx, role.ID)
	if err != nil {
		return err
	}
	groups, err := h.db.Queries.ListGroupsForRole(ctx, role.ID)
	if err != nil {
		return err
	}
	effective, err := h.authzSvc.ResolveRoleMatrix(ctx, role)
	if err != nil {
		return err
	}
	own := authz.Parse(role.Permissions)
	chain, err := h.roleParentChain(ctx, role)
	if err != nil {
		return err
	}
	originByKey := roleOriginByKey(own, chain)
	inheritedByKey, err := h.roleInheritedGrants(ctx, role)
	if err != nil {
		return err
	}
	eligibleRoles, err := h.roleEligibleParents(ctx, role)
	if err != nil {
		return err
	}
	return render(c, templates.PlurisPolicyDetailPage(role, effective, own, chain, members, groups, originByKey, inheritedByKey, eligibleRoles, csrfTokenFrom(c)))
}

// roleOriginByKey builds the per-permission-key origin label the
// Permissions tab badges render: "own" when role's own raw overrides
// carry the key; otherwise "inherited from <name>" for the first
// ancestor (walking immediate parent -> root, i.e. chain's tail -> head,
// since roleParentChain returns root-first) whose OWN overrides carry the
// key; "default" when no level in the chain has an opinion.
func roleOriginByKey(own authz.Grants, chain []db.Role) map[string]string {
	origin := make(map[string]string, len(permissions.AllKeys()))
	for _, key := range permissions.AllKeys() {
		if _, ok := own[key]; ok {
			origin[key] = "own"
			continue
		}
		origin[key] = "default"
		for i := len(chain) - 1; i >= 0; i-- {
			ancestorOwn := authz.Parse(chain[i].Permissions)
			if _, ok := ancestorOwn[key]; ok {
				origin[key] = "inherited from " + chain[i].Name
				break
			}
		}
	}
	return origin
}

// roleInheritedGrants resolves what role's effective matrix would be if
// its OWN overrides were empty -- i.e. its parent's effective matrix
// (completed with none/no defaults), or all-default for a parentless
// role. This is the "reset to inherited" target value for each row.
func (h *Handler) roleInheritedGrants(ctx context.Context, role db.Role) (authz.Grants, error) {
	out := make(authz.Grants, len(permissions.AllKeys()))
	var parentEffective authz.Grants
	if role.ParentRoleID.Valid {
		parent, err := h.db.Queries.GetRole(ctx, role.ParentRoleID.Int64)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			resolved, err := h.authzSvc.ResolveRoleMatrix(ctx, parent)
			if err != nil {
				return nil, err
			}
			parentEffective = resolved
		}
	}
	for _, key := range permissions.AllKeys() {
		if v, ok := parentEffective[key]; ok {
			out[key] = v
			continue
		}
		out[key] = defaultGrantForKey(key)
	}
	return out, nil
}

// defaultGrantForKey returns the "no grant" default form value for key:
// "none" for scoped actions, "no" for unscoped ones.
func defaultGrantForKey(key string) string {
	if a := permissions.ActionByKey(key); a != nil && a.Scoped {
		return "none"
	}
	return "no"
}

// roleEligibleParents returns the tenant's roles minus role itself and
// minus role's own descendants (a descendant becoming role's parent
// would create a cycle -- SetRoleParent rejects it anyway, but the
// picker shouldn't offer it). Descendant walk uses ListRoleChildren,
// depth-capped at authz.MaxRoleDepth as a defensive bound.
func (h *Handler) roleEligibleParents(ctx context.Context, role db.Role) ([]db.Role, error) {
	all, err := h.db.Queries.ListRolesByTenant(ctx, role.TenantID)
	if err != nil {
		return nil, err
	}
	descendants := make(map[int64]bool)
	frontier := []int64{role.ID}
	for depth := 0; depth < authz.MaxRoleDepth && len(frontier) > 0; depth++ {
		next := make([]int64, 0)
		for _, id := range frontier {
			children, err := h.db.Queries.ListRoleChildren(ctx, sql.NullInt64{Int64: id, Valid: true})
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				if !descendants[child.ID] {
					descendants[child.ID] = true
					next = append(next, child.ID)
				}
			}
		}
		frontier = next
	}
	eligible := make([]db.Role, 0, len(all))
	for _, r := range all {
		if r.ID == role.ID || descendants[r.ID] {
			continue
		}
		eligible = append(eligible, r)
	}
	return eligible, nil
}

// roleParentChain walks role's ancestor chain (via ParentRoleID) and
// returns it root-first (furthest ancestor first, role's own row NOT
// included). A missing parent row breaks the walk early, same defensive
// behavior as authz.Service.ResolveRoleMatrix. Capped at
// authz.MaxRoleDepth levels.
func (h *Handler) roleParentChain(ctx context.Context, role db.Role) ([]db.Role, error) {
	chain := make([]db.Role, 0, authz.MaxRoleDepth-1)
	current := role
	for depth := 1; depth < authz.MaxRoleDepth; depth++ {
		if !current.ParentRoleID.Valid {
			break
		}
		parent, err := h.db.Queries.GetRole(ctx, current.ParentRoleID.Int64)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, err
		}
		chain = append(chain, parent)
		current = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// PlurisPolicyCreate creates a new standalone custom role (name required,
// optional inheritance parent) and redirects to its detail page. Unlike
// PlurisPolicyClone this never copies another role's overrides -- the new
// role starts deny-all (its own permissions JSON is "{}"), inheriting
// whatever parent_id specifies (if any).
func (h *Handler) PlurisPolicyCreate(c echo.Context) error {
	if err := requirePermission(c, "console_access.manage_permissions"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	name := c.FormValue("name")
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	var parentID int64
	if raw := c.FormValue("parent_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid parent_id")
		}
		parentID = id
	}
	role, err := h.authzSvc.CreateCustomRole(ctx, sess.TenantID, name, parentID)
	if err != nil {
		return mapRoleParentError(err)
	}
	h.logRoleActivity(c, role.ID, "role_created", role.Name)
	return c.Redirect(http.StatusFound, "/policy/pluris/"+strconv.FormatInt(role.ID, 10))
}

// PlurisPolicySetParent changes (or clears, parent_id="" / "0") roleID's
// inheritance parent. Cycle and builtin-parent rejections surface as 400
// with the service's error message; a missing/cross-tenant parent id
// surfaces as 404 (mirrors resolveTenantRole's not-found shape).
func (h *Handler) PlurisPolicySetParent(c echo.Context) error {
	if err := requirePermission(c, "console_access.manage_permissions"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	role, err := h.resolveTenantRole(c, c.Param("id"))
	if err != nil {
		return err
	}
	var parentID int64
	if raw := c.FormValue("parent_id"); raw != "" && raw != "0" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid parent_id")
		}
		parentID = id
	}
	if err := h.authzSvc.SetRoleParent(ctx, role.ID, parentID); err != nil {
		return mapRoleParentError(err)
	}
	h.logRoleActivity(c, role.ID, "role_parent_changed", role.Name)
	return c.Redirect(http.StatusFound, "/policy/pluris/"+strconv.FormatInt(role.ID, 10))
}

// PlurisPolicyRename updates a custom role's name/description from the
// Settings tab. Builtin roles cannot be renamed (their names are part of
// the shipped template); a blank name is rejected the same way
// PlurisPolicyCreate rejects one.
func (h *Handler) PlurisPolicyRename(c echo.Context) error {
	if err := requirePermission(c, "console_access.manage_permissions"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	role, err := h.resolveTenantRole(c, c.Param("id"))
	if err != nil {
		return err
	}
	if role.IsBuiltin {
		return echo.NewHTTPError(http.StatusBadRequest, "builtin roles cannot be renamed")
	}
	name := c.FormValue("name")
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	description := c.FormValue("description")
	if err := h.db.Queries.UpdateRoleSettings(ctx, db.UpdateRoleSettingsParams{
		ID:          role.ID,
		Name:        name,
		Description: sql.NullString{String: description, Valid: description != ""},
	}); err != nil {
		return err
	}
	h.logRoleActivity(c, role.ID, "role_renamed", name)
	return c.Redirect(http.StatusFound, "/policy/pluris/"+strconv.FormatInt(role.ID, 10))
}

// mapRoleParentError translates authz.Service parent-change errors into
// the HTTP status the brief specifies: cycle/builtin-parent are caller
// mistakes (400, with the service's own message); a missing/cross-tenant
// parent role reads as not-found (404), matching resolveTenantRole's
// shape elsewhere. Any other error passes through unchanged (500 via
// Echo's default error handler).
func mapRoleParentError(err error) error {
	switch {
	case errors.Is(err, authz.ErrRoleCycle), errors.Is(err, authz.ErrBuiltinParent):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, sql.ErrNoRows):
		return echo.NewHTTPError(http.StatusNotFound, "parent role not found")
	default:
		return err
	}
}

// PlurisPolicyClone creates a new custom role from an existing one
// (builtin or custom) and redirects to the clone's detail page.
// Creation is clone-only -- there is no bare "new role" form.
func (h *Handler) PlurisPolicyClone(c echo.Context) error {
	if err := requirePermission(c, "console_access.manage_permissions"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	source, err := h.resolveTenantRole(c, c.Param("id"))
	if err != nil {
		return err
	}
	name := c.FormValue("name")
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	clone, err := h.authzSvc.CloneRole(ctx, sess.TenantID, source.ID, name)
	if err != nil {
		return err
	}
	h.logRoleActivity(c, clone.ID, "role_cloned", clone.Name)
	return c.Redirect(http.StatusFound, "/policy/pluris/"+strconv.FormatInt(clone.ID, 10))
}

// PlurisPolicySave persists a permission matrix submission for a custom
// role. Builtin roles cannot be edited here (their templates are
// upgraded via EnsureBuiltinGrants, not hand-edited). The self-lockout
// guard refuses a save that would leave the ACTING identity without
// console_access.manage_permissions, unless the actor is super_admin.
func (h *Handler) PlurisPolicySave(c echo.Context) error {
	if err := requirePermission(c, "console_access.manage_permissions"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	role, err := h.resolveTenantRole(c, c.Param("id"))
	if err != nil {
		return err
	}
	if role.IsBuiltin {
		return echo.NewHTTPError(http.StatusBadRequest, "builtin roles cannot be edited")
	}
	newGrants, err := parseGrantsForm(c)
	if err != nil {
		return err
	}
	locksOut, err := h.wouldLockOutActor(ctx, sess, role.ID, newGrants)
	if err != nil {
		return err
	}
	if locksOut {
		return echo.NewHTTPError(http.StatusBadRequest, "this change would lock you out of managing permissions")
	}
	if err := h.authzSvc.SaveRoleOverrides(ctx, role.ID, newGrants); err != nil {
		return err
	}
	h.logRoleActivity(c, role.ID, "role_permissions_updated", role.Name)
	return c.Redirect(http.StatusFound, "/policy/pluris/"+strconv.FormatInt(role.ID, 10))
}

// PlurisPolicyDelete removes a custom role. Builtins are protected, and
// a role with any members must be unassigned first.
func (h *Handler) PlurisPolicyDelete(c echo.Context) error {
	if err := requirePermission(c, "console_access.manage_permissions"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	role, err := h.resolveTenantRole(c, c.Param("id"))
	if err != nil {
		return err
	}
	if role.IsBuiltin {
		return echo.NewHTTPError(http.StatusBadRequest, "builtin roles cannot be deleted")
	}
	members, err := h.db.Queries.ListIdentitiesForRole(ctx, role.ID)
	if err != nil {
		return err
	}
	if len(members) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "role still has members assigned")
	}
	// A role with children still grants its permissions to every
	// descendant through inheritance; deleting it out from under them
	// would silently drop those children's inherited grants. Reparent or
	// delete the children first.
	children, err := h.db.Queries.ListRoleChildren(ctx, sql.NullInt64{Int64: role.ID, Valid: true})
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "role has child roles - reparent or delete them first")
	}
	if err := h.db.Queries.DeleteRole(ctx, role.ID); err != nil {
		return err
	}
	h.logRoleActivity(c, role.ID, "role_deleted", role.Name)
	return c.Redirect(http.StatusFound, "/policy/pluris")
}

// logRoleActivity appends a best-effort activity_log row for a Pluris
// Policy role mutation. Distinct from roles.go's logRoleEvent, which
// hardcodes entity_type "identity" for the user-detail Roles tab; role
// mutations here use entity_type "role" with the role's own id.
func (h *Handler) logRoleActivity(c echo.Context, roleID int64, event, roleName string) {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	if sess == nil {
		return
	}
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID:        sess.TenantID,
		EntityType:      "role",
		EntityID:        roleID,
		Event:           event,
		Detail:          sql.NullString{String: roleName, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
}

// countGranted counts registry keys whose stored grant is a non-deny
// value ("yes", "own", or "all") -- used for the list page's summary
// column.
func countGranted(g authz.Grants) int {
	n := 0
	for _, key := range permissions.AllKeys() {
		switch g[key] {
		case "yes", "own", "all":
			n++
		}
	}
	return n
}

// parseGrantsForm builds a Grants map from the submitted permission
// matrix. It iterates permissions.AllKeys() (never the submitted form
// fields), so unknown keys are never written and unknown form fields are
// silently ignored. Scoped keys read "perm_<key>" expecting
// none|own|all (missing = none, anything else invalid -> 400); unscoped
// keys read "perm_<key>" as a checkbox ("yes" present = yes, else no).
func parseGrantsForm(c echo.Context) (authz.Grants, error) {
	out := make(authz.Grants)
	for _, key := range permissions.AllKeys() {
		action := permissions.ActionByKey(key)
		if action == nil {
			continue
		}
		v := c.FormValue("perm_" + key)
		if action.Scoped {
			if v == "" {
				v = "none"
			}
			switch v {
			case "none", "own", "all":
				out[key] = v
			default:
				return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid value for "+key)
			}
			continue
		}
		if v == "yes" {
			out[key] = "yes"
		} else {
			out[key] = "no"
		}
	}
	return out, nil
}

// wouldLockOutActor reports whether saving newGrants for roleID would
// leave sess's identity without console_access.manage_permissions.
// Super_admin sessions bypass the check entirely (they always carry the
// authz.BypassKey grant, so no role edit can lock them out). Actors who
// don't currently hold roleID (directly or via a group) are unaffected by
// its edit regardless of content.
//
// newGrants is the FULL submitted matrix (already effective-shaped, per
// parseGrantsForm), so it is used as roleID's candidate directly -- no
// further resolution needed. Every OTHER held role is resolved through
// its inheritance chain (ResolveRoleMatrix) rather than read as raw
// stored overrides, so a parented role's own-overrides-only storage
// doesn't undercount the actor's real effective grants (RBAC v2).
func (h *Handler) wouldLockOutActor(ctx context.Context, sess *auth.UserSession, roleID int64, newGrants authz.Grants) (bool, error) {
	if sess == nil {
		return false, nil
	}
	if sess.Role == identities.RoleSuperAdmin {
		return false, nil
	}
	direct, err := h.db.Queries.ListRolesForIdentity(ctx, sess.IdentityID)
	if err != nil {
		return false, err
	}
	viaGroups, err := h.db.Queries.ListGroupRolesForIdentity(ctx, sql.NullInt64{Int64: sess.IdentityID, Valid: true})
	if err != nil {
		return false, err
	}
	seen := make(map[int64]bool, len(direct)+len(viaGroups))
	roles := make([]db.Role, 0, len(direct)+len(viaGroups))
	for _, r := range direct {
		if !seen[r.ID] {
			seen[r.ID] = true
			roles = append(roles, r)
		}
	}
	for _, r := range viaGroups {
		if !seen[r.ID] {
			seen[r.ID] = true
			roles = append(roles, r)
		}
	}
	if !seen[roleID] {
		return false, nil
	}
	parsed := make([]authz.Grants, 0, len(roles))
	for _, r := range roles {
		if r.ID == roleID {
			parsed = append(parsed, newGrants)
			continue
		}
		resolved, err := h.authzSvc.ResolveRoleMatrix(ctx, r)
		if err != nil {
			return false, err
		}
		parsed = append(parsed, resolved)
	}
	effective := authz.Union(parsed...)
	return !effective.Can("console_access.manage_permissions"), nil
}
