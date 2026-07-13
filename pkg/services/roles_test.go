package services

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
)

// TestRoleServiceLifecycle covers Task 10: EnsureBuiltins is idempotent,
// and Assign/Remove keep the identities.role cache at the highest
// assigned privilege, falling back to "user" when nothing is assigned.
func TestRoleServiceLifecycle(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	svc := NewRoleService(database)

	if err := svc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	if err := svc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins second run failed: %v", err)
	}
	roles, err := svc.ListByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListByTenant failed: %v", err)
	}
	if len(roles) != 4 {
		t.Fatalf("builtin roles = %d, want 4 (idempotent seed)", len(roles))
	}
	byslug := map[string]db.Role{}
	for _, r := range roles {
		byslug[r.Slug] = r
	}

	idSvc := NewIdentityService(database)
	user, err := idSvc.Create(ctx, tenantID, identities.Identity{
		Username:    "roleuser",
		Email:       "roleuser@example.com",
		DisplayName: "Role User",
		Role:        identities.RoleUser,
	})
	if err != nil {
		t.Fatalf("Create identity failed: %v", err)
	}

	cache := func() string {
		got, err := idSvc.Get(ctx, user.ID)
		if err != nil {
			t.Fatalf("Get identity failed: %v", err)
		}
		return string(got.Role)
	}

	// Assign admin + technician: cache must be the higher one (admin).
	if err := svc.Assign(ctx, user.ID, byslug["admin"].ID, user.ID); err != nil {
		t.Fatalf("Assign admin failed: %v", err)
	}
	if err := svc.Assign(ctx, user.ID, byslug["technician"].ID, user.ID); err != nil {
		t.Fatalf("Assign technician failed: %v", err)
	}
	if got := cache(); got != "admin" {
		t.Fatalf("role cache = %q, want admin", got)
	}

	// Remove admin: technician remains the highest.
	if err := svc.Remove(ctx, user.ID, byslug["admin"].ID); err != nil {
		t.Fatalf("Remove admin failed: %v", err)
	}
	if got := cache(); got != "technician" {
		t.Fatalf("role cache = %q, want technician", got)
	}

	// Remove all: fallback to user.
	if err := svc.Remove(ctx, user.ID, byslug["technician"].ID); err != nil {
		t.Fatalf("Remove technician failed: %v", err)
	}
	if got := cache(); got != "user" {
		t.Fatalf("role cache = %q, want user", got)
	}
}

// TestRecomputeForGroupMembers covers RBAC v2: an identity with only a
// direct "user" role gains the higher privilege of a role assigned to a
// group it joins (once RecomputeForGroupMembers runs), and loses it again
// after the membership is removed and recompute runs again.
func TestRecomputeForGroupMembers(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	svc := NewRoleService(database)

	if err := svc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	roles, err := svc.ListByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListByTenant failed: %v", err)
	}
	byslug := map[string]db.Role{}
	for _, r := range roles {
		byslug[r.Slug] = r
	}

	idSvc := NewIdentityService(database)
	user, err := idSvc.Create(ctx, tenantID, identities.Identity{
		Username:    "groupcacheuser",
		Email:       "groupcacheuser@example.com",
		DisplayName: "Group Cache User",
		Role:        identities.RoleUser,
	})
	if err != nil {
		t.Fatalf("Create identity failed: %v", err)
	}
	// Identity has only the direct "user" role -- Create seeds that
	// itself, but assign it explicitly so identity_roles matches the
	// cache for clarity.
	if err := svc.Assign(ctx, user.ID, byslug["user"].ID, user.ID); err != nil {
		t.Fatalf("Assign user failed: %v", err)
	}

	cache := func() string {
		got, err := idSvc.Get(ctx, user.ID)
		if err != nil {
			t.Fatalf("Get identity failed: %v", err)
		}
		return string(got.Role)
	}
	if got := cache(); got != "user" {
		t.Fatalf("role cache before group role = %q, want user", got)
	}

	group, err := database.Queries.CreateGroup(ctx, db.CreateGroupParams{
		TenantID: tenantID, Name: "Techs", Slug: "techs",
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if err := database.Queries.AssignRoleToGroup(ctx, db.AssignRoleToGroupParams{
		GroupID: group.ID, RoleID: byslug["technician"].ID,
	}); err != nil {
		t.Fatalf("AssignRoleToGroup failed: %v", err)
	}
	if err := database.Queries.AddIdentityToGroup(ctx, db.AddIdentityToGroupParams{
		GroupID: group.ID, IdentityID: sql.NullInt64{Int64: user.ID, Valid: true},
	}); err != nil {
		t.Fatalf("AddIdentityToGroup failed: %v", err)
	}

	if err := svc.RecomputeForGroupMembers(ctx, group.ID); err != nil {
		t.Fatalf("RecomputeForGroupMembers failed: %v", err)
	}
	if got := cache(); got != "technician" {
		t.Fatalf("role cache after group role assign + recompute = %q, want technician", got)
	}

	if err := database.Queries.RemoveIdentityFromGroup(ctx, db.RemoveIdentityFromGroupParams{
		GroupID: group.ID, IdentityID: sql.NullInt64{Int64: user.ID, Valid: true},
	}); err != nil {
		t.Fatalf("RemoveIdentityFromGroup failed: %v", err)
	}
	if err := svc.RecomputeForIdentity(ctx, user.ID); err != nil {
		t.Fatalf("RecomputeForIdentity failed: %v", err)
	}
	if got := cache(); got != "user" {
		t.Fatalf("role cache after membership removal + recompute = %q, want user", got)
	}
}
