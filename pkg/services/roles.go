package services

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// RoleService manages the roles table and identity-role assignments
// (Task 10). The four builtins exist per tenant; custom roles are
// created from a builtin template (permission editor comes later).
// identities.role stays a denormalized cache of the highest-privilege
// assigned role so the RBAC middleware keeps working unchanged.
type RoleService struct {
	db *database.Database
}

// NewRoleService creates a new role service.
func NewRoleService(db *database.Database) *RoleService {
	return &RoleService{db: db}
}

// builtinRoleInfo is the display name/description for a builtin slug (the
// slug list itself lives in permissions.BuiltinSlugs() -- single source of
// truth shared with pkg/authz).
type builtinRoleInfo struct {
	Name, Description string
}

// builtinRoleDetails maps each permissions.BuiltinSlugs() entry to its
// seed name/description for EnsureBuiltins.
var builtinRoleDetails = map[string]builtinRoleInfo{
	"super_admin": {"Super Admin", "Full access to every tenant and server administration."},
	"admin":       {"Admin", "Full access within the tenant, including server administration."},
	"technician":  {"Technician", "Day-to-day management access without server administration."},
	"user":        {"User", "Self-service access to own identity and assigned assets."},
}

// rolePrivilege ranks role slugs; higher wins the identities.role cache.
// Custom roles rank as their template_slug.
var rolePrivilege = map[string]int{
	"super_admin": 4,
	"admin":       3,
	"technician":  2,
	"user":        1,
}

// EnsureBuiltins idempotently creates the four builtin roles for a
// tenant. Safe to call on every setup and login path.
func (s *RoleService) EnsureBuiltins(ctx context.Context, tenantID int64) error {
	for _, slug := range permissions.BuiltinSlugs() {
		info := builtinRoleDetails[slug]
		_, err := s.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: slug})
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.db.Queries.CreateRole(ctx, db.CreateRoleParams{
			TenantID:    tenantID,
			Slug:        slug,
			Name:        info.Name,
			Description: sql.NullString{String: info.Description, Valid: true},
			IsBuiltin:   true,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListByTenant returns every role in the tenant.
func (s *RoleService) ListByTenant(ctx context.Context, tenantID int64) ([]db.Role, error) {
	return s.db.Queries.ListRolesByTenant(ctx, tenantID)
}

// ListForIdentityDetail returns assigned roles with assignment times
// (rows for the Roles tab).
func (s *RoleService) ListForIdentityDetail(ctx context.Context, identityID int64) ([]db.ListRolesForIdentityDetailRow, error) {
	return s.db.Queries.ListRolesForIdentityDetail(ctx, identityID)
}

// ListForIdentity returns the roles assigned to an identity.
func (s *RoleService) ListForIdentity(ctx context.Context, identityID int64) ([]db.Role, error) {
	return s.db.Queries.ListRolesForIdentity(ctx, identityID)
}

// ListForGroupDetail returns a group's directly-assigned roles with
// assignment times (rows for the group detail Roles tab, Task 7).
func (s *RoleService) ListForGroupDetail(ctx context.Context, groupID int64) ([]db.ListRolesForGroupDetailRow, error) {
	return s.db.Queries.ListRolesForGroupDetail(ctx, groupID)
}

// ListGroupRolesForIdentityDetail returns the roles an identity inherits
// via group membership, one row per (role, group) pair, for the user
// detail Roles tab's read-only "via <group>" rows (Task 7).
func (s *RoleService) ListGroupRolesForIdentityDetail(ctx context.Context, identityID int64) ([]db.ListGroupRolesForIdentityDetailRow, error) {
	return s.db.Queries.ListGroupRolesForIdentityDetail(ctx, sql.NullInt64{Int64: identityID, Valid: true})
}

// Assign grants a role to an identity and recomputes the role cache.
func (s *RoleService) Assign(ctx context.Context, identityID, roleID, actorID int64) error {
	assignedBy := sql.NullInt64{}
	if actorID != 0 {
		assignedBy = sql.NullInt64{Int64: actorID, Valid: true}
	}
	if err := s.db.Queries.AssignRoleToIdentity(ctx, db.AssignRoleToIdentityParams{
		IdentityID: identityID,
		RoleID:     roleID,
		AssignedBy: assignedBy,
	}); err != nil {
		return err
	}
	return s.recomputeRoleCache(ctx, identityID)
}

// Remove revokes a role from an identity and recomputes the role cache.
func (s *RoleService) Remove(ctx context.Context, identityID, roleID int64) error {
	if err := s.db.Queries.RemoveRoleFromIdentity(ctx, db.RemoveRoleFromIdentityParams{
		IdentityID: identityID,
		RoleID:     roleID,
	}); err != nil {
		return err
	}
	return s.recomputeRoleCache(ctx, identityID)
}

// recomputeRoleCache writes the highest-privilege role slug -- across
// both direct role assignments and roles inherited via group membership
// (RBAC v2) -- to identities.role. Custom roles rank as their template;
// an identity with no assignments falls back to "user".
func (s *RoleService) recomputeRoleCache(ctx context.Context, identityID int64) error {
	direct, err := s.db.Queries.ListRolesForIdentity(ctx, identityID)
	if err != nil {
		return err
	}
	viaGroups, err := s.db.Queries.ListGroupRolesForIdentity(ctx, sql.NullInt64{Int64: identityID, Valid: true})
	if err != nil {
		return err
	}

	best, bestRank := "user", 0
	rank := func(r db.Role) {
		slug := r.Slug
		if !r.IsBuiltin && r.TemplateSlug.Valid {
			slug = r.TemplateSlug.String
		}
		if v := rolePrivilege[slug]; v > bestRank {
			best, bestRank = slug, v
		}
	}
	for _, r := range direct {
		rank(r)
	}
	for _, r := range viaGroups {
		rank(r)
	}
	return s.db.Queries.UpdateIdentityRole(ctx, db.UpdateIdentityRoleParams{Role: best, ID: identityID})
}

// RecomputeForIdentity re-derives identityID's identities.role cache from
// its current direct + group role assignments. Exported so handlers can
// trigger a recompute after a group-membership change without going
// through Assign/Remove (which mutate identity_roles, not group
// membership).
func (s *RoleService) RecomputeForIdentity(ctx context.Context, identityID int64) error {
	return s.recomputeRoleCache(ctx, identityID)
}

// RecomputeForGroupMembers re-derives the identities.role cache for every
// identity member of groupID. Used after a group's role assignments
// change (a role add/remove on the group affects every member's
// effective privilege) and is also usable by a group-role service added
// later (Task 4). Groups with no identity members are a no-op.
func (s *RoleService) RecomputeForGroupMembers(ctx context.Context, groupID int64) error {
	members, err := s.db.Queries.ListIdentitiesInGroup(ctx, groupID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := s.recomputeRoleCache(ctx, m.ID); err != nil {
			return err
		}
	}
	return nil
}
