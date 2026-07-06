package services

import (
	"context"
	"database/sql"
	"errors"

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

// builtinRole seeds for EnsureBuiltins, in privilege order.
var builtinRoles = []struct {
	Slug, Name, Description string
}{
	{"super_admin", "Super Admin", "Full access to every tenant and server administration."},
	{"admin", "Admin", "Full access within the tenant, including server administration."},
	{"technician", "Technician", "Day-to-day management access without server administration."},
	{"user", "User", "Self-service access to own identity and assigned assets."},
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
	for _, b := range builtinRoles {
		_, err := s.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: b.Slug})
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.db.Queries.CreateRole(ctx, db.CreateRoleParams{
			TenantID:    tenantID,
			Slug:        b.Slug,
			Name:        b.Name,
			Description: sql.NullString{String: b.Description, Valid: true},
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

// recomputeRoleCache writes the highest-privilege assigned role slug to
// identities.role. Custom roles rank as their template; an identity with
// no assignments falls back to "user".
func (s *RoleService) recomputeRoleCache(ctx context.Context, identityID int64) error {
	roles, err := s.db.Queries.ListRolesForIdentity(ctx, identityID)
	if err != nil {
		return err
	}
	best, bestRank := "user", 0
	for _, r := range roles {
		slug := r.Slug
		if !r.IsBuiltin && r.TemplateSlug.Valid {
			slug = r.TemplateSlug.String
		}
		if rank := rolePrivilege[slug]; rank > bestRank {
			best, bestRank = slug, rank
		}
	}
	return s.db.Queries.UpdateIdentityRole(ctx, db.UpdateIdentityRoleParams{Role: best, ID: identityID})
}
