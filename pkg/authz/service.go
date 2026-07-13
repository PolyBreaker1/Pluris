package authz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// Service resolves and persists Pluris Policy grants against the
// roles.permissions JSON column. See
// docs/history/specs/2026-07-08-pluris-policy-authz-design.md
// section "2. Storage & resolution", and
// docs/history/specs/2026-07-09-rbac-v2-design.md section "2.
// Inheritance semantics" for the role-hierarchy additions below.
type Service struct {
	db *database.Database
}

// ErrRoleCycle indicates a SetRoleParent call would create a cycle in the
// role inheritance chain, or would push the resulting chain past
// MaxRoleDepth.
var ErrRoleCycle = errors.New("authz: role parent change would create a cycle or exceed max depth")

// ErrBuiltinParent indicates an attempt to set a parent on a builtin role.
// Builtins are always parentless roots -- customs may parent to a builtin
// or another custom role, never the reverse.
var ErrBuiltinParent = errors.New("authz: builtin roles cannot have a parent")

// MaxRoleDepth caps the inheritance chain length, counting the role
// itself. A parentless role has depth 1; a role whose parent is itself
// parentless has depth 2; and so on. ResolveRoleMatrix stops walking at
// this depth defensively; SetRoleParent rejects any change whose
// resulting chain would exceed it.
const MaxRoleDepth = 5

// NewService creates a new authz Service.
func NewService(db *database.Database) *Service {
	return &Service{db: db}
}

// EffectiveGrants returns the union of grants across every role assigned
// to identityID, directly or via group membership (RBAC v2, see
// docs/history/specs/2026-07-09-rbac-v2-design.md section 2). Each
// role is resolved through its inheritance chain (ResolveRoleMatrix)
// before the union is taken; a role held both directly and via a group
// is resolved only once. An identity with no roles gets an empty
// (deny-all) map. Super-admin bypass and error semantics are unchanged.
func (s *Service) EffectiveGrants(ctx context.Context, identityID int64) (Grants, error) {
	direct, err := s.db.Queries.ListRolesForIdentity(ctx, identityID)
	if err != nil {
		return nil, err
	}
	viaGroups, err := s.db.Queries.ListGroupRolesForIdentity(ctx, sql.NullInt64{Int64: identityID, Valid: true})
	if err != nil {
		return nil, err
	}

	// De-dup roles present in both the direct and group-via sets (same
	// role ID) so each is resolved through inheritance only once.
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

	parsed := make([]Grants, 0, len(roles))
	for _, r := range roles {
		resolved, err := s.ResolveRoleMatrix(ctx, r)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, resolved)
	}
	return Union(parsed...), nil
}

// ResolveRoleMatrix computes role's effective permission matrix by walking
// its parent chain root->leaf and merging each level's own stored keys
// over the previous level's (child keys win). Parentless roles (all
// builtins, standalone customs) resolve to exactly their stored matrix.
// A missing parent row (e.g. deleted out from under a stale ParentRoleID)
// is treated as parentless from that point -- resolution uses whatever
// chain it managed to walk. The walk is capped at MaxRoleDepth levels
// defensively, in case data ever ends up deeper than SetRoleParent would
// allow.
func (s *Service) ResolveRoleMatrix(ctx context.Context, role db.Role) (Grants, error) {
	chain := []Grants{Parse(role.Permissions)}
	current := role
	for depth := 1; depth < MaxRoleDepth; depth++ {
		if !current.ParentRoleID.Valid {
			break
		}
		parent, err := s.db.Queries.GetRole(ctx, current.ParentRoleID.Int64)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, err
		}
		chain = append(chain, Parse(parent.Permissions))
		current = parent
	}

	// chain[0] is the role's own overrides (leaf); later entries are
	// successively higher ancestors. Merge root->leaf so child keys win.
	result := make(Grants)
	for i := len(chain) - 1; i >= 0; i-- {
		for k, v := range chain[i] {
			result[k] = v
		}
	}
	return result, nil
}

// SaveRoleOverrides persists full (a complete 23-key matrix as submitted
// by the matrix form) for roleID. Parentless roles store the full matrix
// verbatim (today's SaveRolePermissions behavior). Roles with a parent
// store ONLY the keys that differ from the parent's effective matrix --
// equal values are dropped, meaning "inherit". The parent's effective
// matrix is completed with none/no defaults for any registry key it
// doesn't already have an opinion on, so the diff is stable regardless of
// which keys happen to be present up the chain.
func (s *Service) SaveRoleOverrides(ctx context.Context, roleID int64, full Grants) error {
	role, err := s.db.Queries.GetRole(ctx, roleID)
	if err != nil {
		return err
	}
	if !role.ParentRoleID.Valid {
		return s.SaveRolePermissions(ctx, roleID, full)
	}

	parent, err := s.db.Queries.GetRole(ctx, role.ParentRoleID.Int64)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Parent row is gone -- treat as parentless.
			return s.SaveRolePermissions(ctx, roleID, full)
		}
		return err
	}
	parentEffective, err := s.ResolveRoleMatrix(ctx, parent)
	if err != nil {
		return err
	}
	completedParent := completeWithDefaults(parentEffective)

	diff := make(Grants)
	for key, fullVal := range full {
		if fullVal != completedParent[key] {
			diff[key] = fullVal
		}
	}
	return s.SaveRolePermissions(ctx, roleID, diff)
}

// completeWithDefaults returns a copy of effective with every registered
// permission key present, filling in "none" (scoped actions) or "no"
// (unscoped actions) for any key effective doesn't already have a value
// for.
func completeWithDefaults(effective Grants) Grants {
	out := make(Grants, len(effective))
	for k, v := range effective {
		out[k] = v
	}
	for _, key := range permissions.AllKeys() {
		if _, ok := out[key]; !ok {
			out[key] = defaultForKey(key)
		}
	}
	return out
}

// defaultForKey returns the "no grant" default value for key: "none" for
// scoped actions, "no" for unscoped ones.
func defaultForKey(key string) string {
	if a := permissions.ActionByKey(key); a != nil && a.Scoped {
		return "none"
	}
	return "no"
}

// SetRoleParent sets (parentID != 0) or clears (parentID == 0) roleID's
// parent for inheritance. Guards, in order: roleID must exist; the role
// must not be builtin (ErrBuiltinParent -- builtins are always parentless
// roots); when clearing (parentID == 0) that's the whole check. Otherwise:
// parentID must exist in the SAME tenant as roleID (else a not-found
// style error); parentID must not equal roleID and walking parentID's own
// chain must never reach roleID (ErrRoleCycle); the resulting chain depth
// (parent's depth + 1) must not exceed MaxRoleDepth (ErrRoleCycle).
func (s *Service) SetRoleParent(ctx context.Context, roleID, parentID int64) error {
	role, err := s.db.Queries.GetRole(ctx, roleID)
	if err != nil {
		return err
	}
	if role.IsBuiltin {
		return ErrBuiltinParent
	}

	if parentID == 0 {
		return s.db.Queries.UpdateRoleParent(ctx, db.UpdateRoleParentParams{
			ID:           roleID,
			ParentRoleID: sql.NullInt64{},
		})
	}
	if parentID == roleID {
		return ErrRoleCycle
	}

	parent, err := s.db.Queries.GetRole(ctx, parentID)
	if err != nil {
		return err
	}
	if parent.TenantID != role.TenantID {
		return sql.ErrNoRows
	}

	// Walk parent's own ancestor chain: reaching roleID means a cycle;
	// exceeding MaxRoleDepth (role + parent + parent's ancestors) means
	// too deep.
	depth := 2 // roleID + parentID
	current := parent
	for {
		if current.ID == roleID {
			return ErrRoleCycle
		}
		if !current.ParentRoleID.Valid {
			break
		}
		depth++
		if depth > MaxRoleDepth {
			return ErrRoleCycle
		}
		next, err := s.db.Queries.GetRole(ctx, current.ParentRoleID.Int64)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return err
		}
		current = next
	}
	if depth > MaxRoleDepth {
		return ErrRoleCycle
	}

	return s.db.Queries.UpdateRoleParent(ctx, db.UpdateRoleParentParams{
		ID:           roleID,
		ParentRoleID: sql.NullInt64{Int64: parentID, Valid: true},
	})
}

// EnsureBuiltinGrants idempotently fills in the builtin role permission
// templates for a tenant. It never overwrites a value that is already
// present in a role's stored JSON -- only keys absent from the stored
// permissions are filled from permissions.TemplateGrants. Safe to call
// repeatedly (at setup, and lazily from the Pluris Policy page).
func (s *Service) EnsureBuiltinGrants(ctx context.Context, tenantID int64) error {
	for _, slug := range permissions.BuiltinSlugs() {
		template := permissions.TemplateGrants(slug)
		if template == nil {
			continue
		}
		role, err := s.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: slug})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}
		existing := Parse(role.Permissions)
		changed := false
		for key, value := range template {
			if _, ok := existing[key]; !ok {
				existing[key] = value
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := s.SaveRolePermissions(ctx, role.ID, existing); err != nil {
			return err
		}
	}
	return nil
}

// SaveRolePermissions writes g as the role's permissions JSON.
func (s *Service) SaveRolePermissions(ctx context.Context, roleID int64, g Grants) error {
	encoded, err := json.Marshal(g)
	if err != nil {
		return err
	}
	return s.db.Queries.UpdateRolePermissions(ctx, db.UpdateRolePermissionsParams{
		ID:          roleID,
		Permissions: string(encoded),
	})
}

// CloneRole creates a new custom (non-builtin) role in tenantID, copying
// sourceRoleID's permissions JSON verbatim. The clone's template_slug is
// the source's own slug when the source is builtin, or the source's
// template_slug when the source is itself a custom role (so clones of
// clones still trace back to a builtin template).
func (s *Service) CloneRole(ctx context.Context, tenantID, sourceRoleID int64, newName string) (db.Role, error) {
	source, err := s.db.Queries.GetRole(ctx, sourceRoleID)
	if err != nil {
		return db.Role{}, err
	}

	templateSlug := source.TemplateSlug
	if source.IsBuiltin {
		templateSlug = sql.NullString{String: source.Slug, Valid: true}
	}

	created, err := s.db.Queries.CreateRole(ctx, db.CreateRoleParams{
		TenantID:     tenantID,
		Slug:         slugify(newName),
		Name:         newName,
		IsBuiltin:    false,
		TemplateSlug: templateSlug,
	})
	if err != nil {
		return db.Role{}, err
	}

	if err := s.db.Queries.UpdateRolePermissions(ctx, db.UpdateRolePermissionsParams{
		ID:          created.ID,
		Permissions: source.Permissions,
	}); err != nil {
		return db.Role{}, err
	}
	created.Permissions = source.Permissions

	return created, nil
}

// CreateCustomRole creates a new parentless-or-parented custom role in
// tenantID named newName, with empty (deny-all) own overrides. If
// parentID != 0, the role's parent is set via SetRoleParent (which
// enforces the builtin/cycle/depth rules -- callers should propagate
// ErrRoleCycle/ErrBuiltinParent/sql.ErrNoRows as 400/404 the same way
// SetRoleParent's other callers do). The new role's TemplateSlug traces
// back to a builtin the same way CloneRole's does: the parent's own slug
// when the parent is builtin, the parent's TemplateSlug when the parent
// is itself custom, or unset when parentID is 0.
func (s *Service) CreateCustomRole(ctx context.Context, tenantID int64, name string, parentID int64) (db.Role, error) {
	var templateSlug sql.NullString
	if parentID != 0 {
		parent, err := s.db.Queries.GetRole(ctx, parentID)
		if err != nil {
			return db.Role{}, err
		}
		if parent.TenantID != tenantID {
			return db.Role{}, sql.ErrNoRows
		}
		if parent.IsBuiltin {
			templateSlug = sql.NullString{String: parent.Slug, Valid: true}
		} else {
			templateSlug = parent.TemplateSlug
		}
	}

	created, err := s.db.Queries.CreateRole(ctx, db.CreateRoleParams{
		TenantID:     tenantID,
		Slug:         slugify(name),
		Name:         name,
		IsBuiltin:    false,
		TemplateSlug: templateSlug,
	})
	if err != nil {
		return db.Role{}, err
	}

	if err := s.db.Queries.UpdateRolePermissions(ctx, db.UpdateRolePermissionsParams{
		ID:          created.ID,
		Permissions: "{}",
	}); err != nil {
		return db.Role{}, err
	}
	created.Permissions = "{}"

	if parentID != 0 {
		if err := s.SetRoleParent(ctx, created.ID, parentID); err != nil {
			return db.Role{}, err
		}
		created.ParentRoleID = sql.NullInt64{Int64: parentID, Valid: true}
	}

	return created, nil
}

// slugify converts a display name into a lowercase, dash-separated slug.
// Local copy of the pattern in pkg/services/dependencygroups.go (pkg/authz
// cannot import the unexported helper from a different package).
func slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
