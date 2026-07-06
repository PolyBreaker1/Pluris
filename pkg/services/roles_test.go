package services

import (
	"context"
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
