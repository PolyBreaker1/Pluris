// Package handlers implements one HTTP handler per top-level sidebar
// route. Each handler renders a templ page from web/templates; the
// data-testid anchor on each page is asserted by the mount-point test
// in console/server/server_test.go.
package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

type Handler struct {
	db             *database.Database
	assetSvc       *services.AssetService
	identitySvc    *services.IdentityService
	groupSvc       *services.GroupService
	roleSvc        *services.RoleService
	assignmentSvc  *services.AssignmentService
	depGroupSvc    *services.DependencyGroupService
	moduleSvc      *services.PolicyModuleService
	targetSvc      *services.TargetService
	configGroupSvc *services.ConfigGroupService
	retentionSvc   *services.RetentionService
	authzSvc       *authz.Service
	sessions       *auth.SessionManager
}

func New(db *database.Database) *Handler {
	moduleSvc := services.NewPolicyModuleService(db)
	return &Handler{
		db:             db,
		assetSvc:       services.NewAssetService(db),
		identitySvc:    services.NewIdentityService(db),
		groupSvc:       services.NewGroupService(db),
		roleSvc:        services.NewRoleService(db),
		assignmentSvc:  services.NewAssignmentService(db),
		depGroupSvc:    services.NewDependencyGroupService(db),
		moduleSvc:      moduleSvc,
		targetSvc:      services.NewTargetService(db),
		configGroupSvc: services.NewConfigGroupService(db, moduleSvc),
		retentionSvc:   services.NewRetentionService(db),
		authzSvc:       authz.NewService(db),
		sessions:       auth.NewSessionManager(db),
	}
}

// render writes a templ.Component as HTML.
func render(c echo.Context, comp templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	return comp.Render(c.Request().Context(), c.Response().Writer)
}

// Dashboard renders the top-level overview page (item 1 in the sidebar).
func (h *Handler) Dashboard(c echo.Context) error {
	return render(c, templates.DashboardPage())
}

// Users renders the identity directory page (item 2 in the sidebar).
// Mirrors assetsPage: fetch real rows scoped to the caller's effective
// tenant, then hand them to the templ page.
func (h *Handler) Users(c echo.Context) error {
	ctx := c.Request().Context()

	sess := auth.FromContext(ctx)
	tenantID := sess.TenantID

	deleted := c.QueryParam("state") == "deleted"
	var rows []identities.Identity
	var err error
	if deleted {
		rows, err = h.identitySvc.ListDeleted(ctx, tenantID, 200, 0)
	} else {
		rows, err = h.identitySvc.List(ctx, tenantID, 200, 0)
	}
	if err != nil {
		return err
	}
	setting, err := h.retentionSvc.GetSetting(ctx, services.EntityKindIdentity)
	if err != nil {
		return err
	}
	return render(c, templates.UsersPage(rows, deleted, services.RetentionDeleteCopy(setting, "users"), csrfTokenFrom(c)))
}

// UserDetail renders the full-page detail view of a single user, including
// the assets currently assigned to them.
func (h *Handler) UserDetail(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	user, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	assigned, err := h.identitySvc.ListAssignedAssets(ctx, sess.TenantID, id)
	if err != nil {
		return err
	}
	groups, err := h.groupSvc.ListForIdentity(ctx, id)
	if err != nil {
		return err
	}
	allGroups, err := h.groupSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	// Builtin roles are seeded at setup; EnsureBuiltins here upgrades
	// tenants created before the roles feature existed (idempotent).
	if err := h.roleSvc.EnsureBuiltins(ctx, sess.TenantID); err != nil {
		return err
	}
	roles, err := h.roleSvc.ListForIdentityDetail(ctx, id)
	if err != nil {
		return err
	}
	allRoles, err := h.roleSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	// Roles reaching this identity via group membership (RBAC v2 Task
	// 7): rendered as read-only "via <group>" rows on the Roles tab,
	// distinct from the direct assignments in roles above.
	viaGroupRoles, err := h.roleSvc.ListGroupRolesForIdentityDetail(ctx, id)
	if err != nil {
		return err
	}
	groupIDs := make([]int64, 0, len(groups))
	for _, g := range groups {
		groupIDs = append(groupIDs, g.ID)
	}
	applied, err := h.assignmentSvc.ResolveForTarget(ctx, sess.TenantID, "identity", id, groupIDs, 0)
	if err != nil {
		return err
	}
	setting, err := h.retentionSvc.GetSetting(ctx, services.EntityKindIdentity)
	if err != nil {
		return err
	}
	return render(c, templates.UserDetailPage(user, assigned, csrfTokenFrom(c), groups, allGroups, roles, allRoles, viaGroupRoles, applied, c.QueryParam("warn"), services.RetentionDeleteCopy(setting, "user")))
}

// UserNewShow renders the full-page "add user" form (Task 8, spec §6):
// the same standardized section-card layout as the detail page's General
// tab, but every editable field is an open input (create mode has
// nothing to toggle) and there is one Create button instead of per-
// section Save.
func (h *Handler) UserNewShow(c echo.Context) error {
	if err := requirePermission(c, "identity.create"); err != nil {
		return err
	}
	return render(c, templates.UserCreatePage(identities.Identity{}, "", csrfTokenFrom(c)))
}

// userCreateCoreKeys are the schema keys populated directly onto the
// identities.Identity passed to IdentityService.Create -- everything
// CreateIdentityParams (pkg/services/identities.go) actually writes on
// insert. Every other submitted, editable schema key is applied in a
// second pass through UpdateFields (shared coercion/editability rules),
// so this map doubles as the "already applied, don't reapply" exclusion
// set for that pass.
var userCreateCoreKeys = map[string]bool{
	"username": true, "email": true, "display_name": true,
	"given_name": true, "surname": true, "user_principal_name": true,
	"title": true, "department": true, "company": true,
	"employee_id": true, "employee_type": true,
	"phone_office": true, "phone_mobile": true,
}

// identityFromCreateForm reads every text-valued schema field the
// UserCreatePage form can submit into an identities.Identity, so both the
// validation-failure re-render (which needs every entered value echoed
// back) and the Create call (which needs the core subset) share one
// source of truth. Mirrors pkg/services/identities.go's
// applyIdentityField field-by-field, restricted to the plain-string
// fields the create form renders (security section fields never appear
// in the form, so no bool/time parsing is needed here).
func identityFromCreateForm(c echo.Context) identities.Identity {
	return identities.Identity{
		Username:          c.FormValue("username"),
		UserPrincipalName: c.FormValue("user_principal_name"),
		Email:             c.FormValue("email"),
		DisplayName:       c.FormValue("display_name"),
		GivenName:         c.FormValue("given_name"),
		Surname:           c.FormValue("surname"),
		Initials:          c.FormValue("initials"),
		Title:             c.FormValue("title"),
		Department:        c.FormValue("department"),
		Company:           c.FormValue("company"),
		EmployeeID:        c.FormValue("employee_id"),
		EmployeeType:      c.FormValue("employee_type"),
		PhoneOffice:       c.FormValue("phone_office"),
		PhoneMobile:       c.FormValue("phone_mobile"),
		PhoneHome:         c.FormValue("phone_home"),
		Fax:               c.FormValue("fax"),
		Office:            c.FormValue("office"),
		StreetAddress:     c.FormValue("street_address"),
		City:              c.FormValue("city"),
		State:             c.FormValue("state"),
		PostalCode:        c.FormValue("postal_code"),
		Country:           c.FormValue("country"),
		CountryCode:       c.FormValue("country_code"),
		HomeDirectory:     c.FormValue("home_directory"),
		HomeDrive:         c.FormValue("home_drive"),
		ProfilePath:       c.FormValue("profile_path"),
		LogonScript:       c.FormValue("logon_script"),
		Locale:            c.FormValue("locale"),
		Timezone:          c.FormValue("timezone"),
		Description:       c.FormValue("description"),
		Notes:             c.FormValue("notes"),
	}
}

// UserCreateSubmit creates a new identity from the full-page "add user"
// form (Task 8). Username and email are required; display_name
// auto-fills from First+Last when blank (AD behavior). Create persists
// the core identity fields (see userCreateCoreKeys); every other
// submitted, editable schema field is then applied through
// IdentityService.UpdateFields -- the same validation/coercion path the
// detail page's inline editor uses, so this handler never duplicates
// that logic. Per-section UpdateFields failures after a successful
// Create do not roll back the new account (it already exists and is
// otherwise valid); instead the handler redirects to the new user's
// detail page with a `warn` query param surfaced as a dismissible banner
// there, naming which fields were dropped.
func (h *Handler) UserCreateSubmit(c echo.Context) error {
	if err := requirePermission(c, "identity.create"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	in := identityFromCreateForm(c)
	// AD auto-generates displayName from First + Last name when it isn't
	// set explicitly; mirror that here so admins don't have to type the
	// same name twice.
	if in.DisplayName == "" && (in.GivenName != "" || in.Surname != "") {
		in.DisplayName = strings.TrimSpace(strings.TrimSpace(in.GivenName) + " " + strings.TrimSpace(in.Surname))
	}
	if in.Username == "" || in.Email == "" {
		return render(c, templates.UserCreatePage(in, "Username and email are required.", csrfTokenFrom(c)))
	}
	in.Role = identities.RoleUser

	created, err := h.identitySvc.Create(ctx, sess.TenantID, in)
	if err != nil {
		log.Printf("user create failed: %v", err)
		return render(c, templates.UserCreatePage(in, "Could not create user. Check the username/email aren't already taken.", csrfTokenFrom(c)))
	}
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID: sess.TenantID, EntityType: "identity", EntityID: created.ID,
		Event: "user_created", Detail: sql.NullString{String: created.Username, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})

	// Second pass: every other submitted, editable schema field, applied
	// per-section through UpdateFields.
	var warnings []string
	for _, sec := range params.SchemaIdentity.Sections {
		if sec.Key == "security" {
			continue
		}
		fields := make(map[string]string)
		for _, key := range sec.Params {
			if key == "avatar_url" || identities.NonEditableFieldKeys[key] || userCreateCoreKeys[key] {
				continue
			}
			if v := c.FormValue(key); v != "" {
				fields[key] = v
			}
		}
		if len(fields) == 0 {
			continue
		}
		if _, err := h.identitySvc.UpdateFields(ctx, sess.TenantID, created.ID, sec.Key, fields); err != nil {
			warnings = append(warnings, err.Error())
		}
	}

	target := "/users/" + strconv.FormatInt(created.ID, 10)
	if len(warnings) > 0 {
		target += "?warn=" + url.QueryEscape("User created, but some fields could not be saved: "+strings.Join(warnings, "; "))
	}
	return c.Redirect(http.StatusFound, target)
}

// UserEditShow renders the edit form pre-filled with the existing user.
func (h *Handler) UserEditShow(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	user, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	return render(c, templates.UserFormPage(user, csrfTokenFrom(c), "", false))
}

// UserUpdateSubmit writes back the editable fields of an existing user.
func (h *Handler) UserUpdateSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	existing, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	// Username is intentionally NOT taken from the form: UpdateIdentity's
	// SQL never writes it back (username-as-login-identifier changing
	// silently under a user is its own can of worms, left for later), and
	// UserFormPage renders it read-only in edit mode. existing.Username is
	// left as-is here so an accidental client-side edit is a no-op rather
	// than a silently-swallowed write.
	existing.Email = c.FormValue("email")
	existing.DisplayName = c.FormValue("display_name")
	existing.GivenName = c.FormValue("given_name")
	existing.Surname = c.FormValue("surname")
	existing.Title = c.FormValue("title")
	existing.Department = c.FormValue("department")

	if existing.Email == "" || existing.DisplayName == "" {
		return render(c, templates.UserFormPage(existing, csrfTokenFrom(c), "Email and display name are required.", false))
	}

	if _, err := h.identitySvc.Update(ctx, existing); err != nil {
		log.Printf("user update failed: %v", err)
		return render(c, templates.UserFormPage(existing, csrfTokenFrom(c), "Could not save changes.", false))
	}
	if sess := auth.FromContext(ctx); sess != nil {
		_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
			TenantID: sess.TenantID, EntityType: "identity", EntityID: id,
			Event: "user_updated", Detail: sql.NullString{String: existing.Username, Valid: true},
			ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
		})
	}
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(id, 10))
}

// UserDeleteSubmit permanently removes a user.
//
// Safety guard: a caller may not delete their OWN identity. Nothing else in
// this handler chain enforces row-level self-scoping yet (RBAC is
// route-prefix-only per pkg/auth/rbac.go's permission map, and
// "user"-role callers already reach every /users/* route), so
// without this check a logged-in user — self-service or admin — could
// delete their own active account out from under themselves via a stray
// POST. A "last remaining super_admin" guard was deliberately NOT added:
// it would require a new cross-tenant query IdentityService doesn't have
// today (super_admin status isn't scoped to the caller's tenant), which is
// more surface than a single simple check — left as a follow-up.
func (h *Handler) UserDeleteSubmit(c echo.Context) error {
	if err := requirePermission(c, "identity.delete"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	if sess := auth.FromContext(ctx); sess != nil && sess.IdentityID == id {
		return echo.NewHTTPError(http.StatusBadRequest, "you cannot delete your own account")
	}
	sess := auth.FromContext(ctx)
	if err := h.identitySvc.Delete(ctx, sess.TenantID, id, sess.IdentityID); err != nil {
		return err
	}
	if sess != nil {
		_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
			TenantID: sess.TenantID, EntityType: "identity", EntityID: id,
			Event:           "user_deleted",
			ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
		})
	}
	return c.Redirect(http.StatusFound, "/users")
}

// Assets — all four subtype tabs (computers / servers / printers / desks)
// render the SAME canonical editor with a `subtype=` filter. The redirect
// handler below points the bare /assets URL at the computers tab.

// AssetsRedirect 302-redirects /assets to /assets/computers (the default tab).
func (h *Handler) AssetsRedirect(c echo.Context) error {
	return c.Redirect(302, "/assets/computers")
}

func (h *Handler) AssetsComputers(c echo.Context) error {
	return h.assetsPage(c, "computers", "computer")
}

func (h *Handler) AssetsServers(c echo.Context) error {
	return h.assetsPage(c, "servers", "server")
}

func (h *Handler) AssetsPrinters(c echo.Context) error {
	return h.assetsPage(c, "printers", "printer")
}

func (h *Handler) AssetsDesks(c echo.Context) error {
	return h.assetsPage(c, "desks", "desk")
}

// assetsPage fetches assets from database and renders the page
func (h *Handler) assetsPage(c echo.Context, subtypeSlug, subtypeValue string) error {
	ctx := c.Request().Context()

	sess := auth.FromContext(ctx)
	tenantID := sess.TenantID

	deleted := c.QueryParam("state") == "deleted"
	var assetRows []assets.Asset
	var err error
	if deleted {
		assetRows, err = h.assetSvc.ListDeletedBySubtype(ctx, tenantID, subtypeValue)
	} else {
		assetRows, err = h.assetSvc.ListBySubtype(ctx, tenantID, subtypeValue)
	}
	if err != nil {
		return err
	}
	setting, err := h.retentionSvc.GetSetting(ctx, services.EntityKindAsset)
	if err != nil {
		return err
	}
	return render(c, templates.AssetsPage(subtypeSlug, assetRows, deleted, services.RetentionDeleteCopy(setting, "assets"), csrfTokenFrom(c)))
}

// AssetDetail renders the full-page detail view of a single asset.
// The :subtype param provides breadcrumb context; :id is the asset's
// stable ID. If the asset doesn't exist, redirects to the subtype list.
func (h *Handler) AssetDetail(c echo.Context) error {
	ctx := c.Request().Context()
	subtype := c.Param("subtype")
	id := c.Param("id")

	// Fetch asset from database
	asset, err := h.assetSvc.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// If not found, show "not found" page
	if asset == nil {
		return render(c, templates.AssetDetailPage(subtype, id))
	}

	sess := auth.FromContext(ctx)
	owners, err := h.identitySvc.List(ctx, sess.TenantID, 200, 0)
	if err != nil {
		return err
	}

	// Software + Logs tab rows. The catalog Asset carries the human ID;
	// the inventory tables key on the numeric DB id, so resolve it. A
	// resolution failure is impossible here (GetByID just succeeded),
	// but fail soft to empty tabs rather than a 500 either way.
	var software []db.InstalledSoftware
	var logs []db.ActivityLog
	var groups []services.GroupRow
	var applied []services.AppliedPolicy
	if dbID, rerr := h.assetSvc.ResolveDBID(ctx, id); rerr == nil {
		if rows, qerr := h.db.Queries.ListSoftwareForAsset(ctx, dbID); qerr == nil {
			software = rows
		}
		if rows, qerr := h.db.Queries.ListActivityForEntity(ctx, db.ListActivityForEntityParams{
			TenantID:   sess.TenantID,
			EntityType: "asset",
			EntityID:   dbID,
			Limit:      100,
		}); qerr == nil {
			logs = rows
		}
		if rows, qerr := h.groupSvc.ListForAsset(ctx, dbID); qerr == nil {
			groups = rows
		}
		groupIDs := make([]int64, 0, len(groups))
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
		}
		var siteID int64
		if row, qerr := h.db.Queries.GetAsset(ctx, dbID); qerr == nil && row.SiteID.Valid {
			siteID = row.SiteID.Int64
		}
		if rows, qerr := h.assignmentSvc.ResolveForTarget(ctx, sess.TenantID, "asset", dbID, groupIDs, siteID); qerr == nil {
			applied = rows
		}
	}
	allGroups, err := h.groupSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}

	return render(c, templates.AssetDetailPageWithData(subtype, *asset, owners, csrfTokenFrom(c), software, logs, groups, allGroups, applied))
}

// AssetSetOwner assigns or clears the owning identity of an asset. An
// empty or "0" owner_id form value clears ownership; any other value
// must resolve to a real identity ID.
func (h *Handler) AssetSetOwner(c echo.Context) error {
	if err := requirePermission(c, "asset.set_owner"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	subtype := c.Param("subtype")
	humanOrUUID := c.Param("id")

	dbID, err := h.assetSvc.ResolveDBID(ctx, humanOrUUID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}

	ownerIDStr := c.FormValue("owner_id")
	if ownerIDStr == "" || ownerIDStr == "0" {
		if err := h.assetSvc.ClearOwner(ctx, dbID); err != nil {
			return err
		}
	} else {
		ownerID, err := strconv.ParseInt(ownerIDStr, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid owner_id")
		}
		// The owner picker's <select> is built from the caller's own
		// tenant (see AssetDetail above), but nothing stops a forged POST
		// from naming an identity ID belonging to a different tenant —
		// verify it resolves to an identity in the caller's own tenant
		// before writing it as the asset's owner.
		sess := auth.FromContext(ctx)
		owner, err := h.identitySvc.Get(ctx, ownerID)
		if err != nil || sess == nil || owner.TenantID != sess.TenantID {
			return echo.NewHTTPError(http.StatusForbidden, "owner must belong to your tenant")
		}
		if err := h.assetSvc.SetOwner(ctx, dbID, ownerID); err != nil {
			return err
		}
	}

	if sess := auth.FromContext(ctx); sess != nil {
		_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
			TenantID:        sess.TenantID,
			EntityType:      "asset",
			EntityID:        dbID,
			Event:           "owner_changed",
			Detail:          sql.NullString{String: ownerIDStr, Valid: ownerIDStr != ""},
			ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
		})
	}
	return c.Redirect(http.StatusFound, "/assets/"+subtype+"/"+humanOrUUID)
}

// PolicyCatalogDetail renders the full-page view of one catalog policy
// (Task 15): definition, candidate modules, current assignments.
func (h *Handler) PolicyCatalogDetail(c echo.Context) error {
	ctx := c.Request().Context()
	pol := policyByID(c.Param("id"))
	if pol == nil {
		return echo.NewHTTPError(http.StatusNotFound, "unknown policy")
	}
	modules := policymodules.CandidatesForPolicy(pol.ID, policymodules.OSAny)
	sess := auth.FromContext(ctx)
	assignments, err := h.db.Queries.ListAssignmentsByPolicy(ctx, db.ListAssignmentsByPolicyParams{
		PolicyUrn: pol.ID,
		TenantID:  sess.TenantID,
	})
	if err != nil {
		return err
	}
	return render(c, templates.PolicyDetailPage(*pol, modules, assignments))
}

// Policy — Catalog / Configuration Groups / Modules all live under
// /policy/*, navigated via the sidebar. Modules moved here from
// /scripts/policy-modules on 2026-05-16: a Policy Module IS a policy
// concept, not an automation concept.

// PolicyRedirect 302-redirects /policy to /policy/catalog (the default tab).
func (h *Handler) PolicyRedirect(c echo.Context) error {
	return c.Redirect(302, "/policy/catalog")
}
func (h *Handler) PolicyCatalog(c echo.Context) error {
	return render(c, templates.PolicyCatalogPage())
}

// PolicyGroups (list), the create/detail pages, and the Assignments/
// Bindings tab endpoints live in console/handlers/config_groups.go
// (Task 5.2 -- retired the popup dialog + catalog/configgroups mock in
// favor of full standardized pages backed by services.ConfigGroupService).

// PolicyModules — Library view. Three sub-tabs (library / defaults /
// sources) share the same page chrome; the canonical editor remains
// editors/PolicyModuleEditor (R1: single source of truth).
func (h *Handler) PolicyModules(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	deleted := c.QueryParam("state") == "deleted"
	// Best-effort: a fresh tenant may not have builtins seeded yet. Errors
	// here shouldn't block rendering the page.
	_ = h.depGroupSvc.EnsureBuiltins(ctx, sess.TenantID)
	_ = h.moduleSvc.SeedBundled(ctx)
	groups, err := h.depGroupSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	byID := make(map[int64]dependencygroups.Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}
	var mods []policymodules.Module
	if deleted {
		mods, err = h.moduleSvc.ListDeletedModules(ctx, sess.TenantID)
	} else {
		mods, err = h.moduleSvc.ListModules(ctx, sess.TenantID)
	}
	if err != nil {
		return err
	}
	deps := make(map[string]templates.ModuleDepsView, len(mods))
	for _, m := range mods {
		links, err := h.depGroupSvc.ListLinksForModule(ctx, sess.TenantID, m.ID)
		if err != nil {
			continue
		}
		var dv templates.ModuleDepsView
		for _, l := range links {
			g, ok := byID[l.GroupID]
			if !ok {
				continue
			}
			chip := templates.ModuleDepChip{GroupID: g.ID, Name: g.Name}
			if l.Role == "platform" {
				dv.Platforms = append(dv.Platforms, chip)
			} else {
				dv.Requirements = append(dv.Requirements, chip)
			}
		}
		deps[m.ID] = dv
	}
	isAdmin := sess.Role == identities.RoleAdmin || sess.Role == identities.RoleSuperAdmin
	setting, err := h.retentionSvc.GetSetting(ctx, services.EntityKindPolicyModule)
	if err != nil {
		return err
	}
	return render(c, templates.PolicyModulesPage("library", mods, nil, deps, groups, csrfTokenFrom(c), isAdmin, deleted, services.RetentionDeleteCopy(setting, "policy modules")))
}
func (h *Handler) PolicyModulesDefaults(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	_ = h.moduleSvc.SeedBundled(ctx)
	mods, err := h.moduleSvc.ListModules(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	defaults := policymodules.TenantDefaults("")
	return render(c, templates.PolicyModulesPage("defaults", mods, defaults, nil, nil, "", false, false, ""))
}
func (h *Handler) PolicyModulesSources(c echo.Context) error {
	return render(c, templates.PolicyModulesPage("sources", nil, nil, nil, nil, csrfTokenFrom(c), false, false, c.QueryParam("warn")))
}

// Profiles renders the profile-list + editor page (item 5 in the sidebar).
func (h *Handler) Profiles(c echo.Context) error {
	return render(c, templates.ProfilesPage())
}

// Scripts renders the admin-authored automation-scripts page (item 6).
// Was previously a two-tab surface; the Policy Modules tab moved to
// /policy/modules on 2026-05-16.
func (h *Handler) Scripts(c echo.Context) error {
	return render(c, templates.ScriptsPage())
}

// Wine renders the Windows-app compatibility page (item 7 in the sidebar).
func (h *Handler) Wine(c echo.Context) error {
	return render(c, templates.WinePage())
}

// Package Management — three tabs (managers / packages / cycles).

// PackagesRedirect 302-redirects /packages to /packages/managers (default tab).
func (h *Handler) PackagesRedirect(c echo.Context) error {
	return c.Redirect(302, "/packages/managers")
}
func (h *Handler) PackagesManagers(c echo.Context) error {
	return render(c, templates.PackagesPage("managers"))
}
func (h *Handler) PackagesPackages(c echo.Context) error {
	return render(c, templates.PackagesPage("packages"))
}
func (h *Handler) PackagesCycles(c echo.Context) error {
	return render(c, templates.PackagesPage("cycles"))
}

// ServerAdmin renders the tenant/AD/GP-import admin page (item 9).
func (h *Handler) ServerAdmin(c echo.Context) error {
	return render(c, templates.ServerAdminPage())
}

// Preferences renders the per-user + admin preferences page (item 10).
func (h *Handler) Preferences(c echo.Context) error {
	return render(c, templates.PreferencesPage())
}
