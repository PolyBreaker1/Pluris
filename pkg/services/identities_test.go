package services

import (
	"context"
	"os"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func setupIdentityTestDB(t *testing.T) (*database.Database, int64) {
	t.Helper()
	dbPath := "test_identity_service.db"
	t.Cleanup(func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-shm")
		os.Remove(dbPath + "-wal")
	})

	database, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	tenant, err := database.Queries.CreateTenant(context.Background(), db.CreateTenantParams{
		Name: "Test Org", Slug: "test-org-svc",
	})
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	return database, tenant.ID
}

func TestIdentityServiceCreateGetListSearch(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	svc := NewIdentityService(database)
	ctx := context.Background()

	created, err := svc.Create(ctx, tenantID, identities.Identity{
		Username:    "jdoe",
		Email:       "jdoe@example.com",
		DisplayName: "Jane Doe",
		Department:  "IT",
		Role:        identities.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Email != "jdoe@example.com" {
		t.Fatalf("expected jdoe@example.com, got %s", got.Email)
	}

	list, err := svc.List(ctx, tenantID, 100, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(list))
	}

	results, err := svc.Search(ctx, tenantID, "Jane", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}

	updated := got
	updated.Title = "IT Manager"
	after, err := svc.Update(ctx, updated)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if after.Title != "IT Manager" {
		t.Fatalf("expected title IT Manager, got %s", after.Title)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := svc.Get(ctx, created.ID); err == nil {
		t.Fatal("expected error getting deleted identity")
	}
}

func TestIdentityServiceGetByEmailGlobal(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	svc := NewIdentityService(database)
	ctx := context.Background()

	_, err := svc.Create(ctx, tenantID, identities.Identity{
		Username: "asuper", Email: "asuper@example.com",
		DisplayName: "Alice Super", Role: identities.RoleSuperAdmin,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := svc.GetByEmailGlobal(ctx, "asuper@example.com")
	if err != nil {
		t.Fatalf("GetByEmailGlobal failed: %v", err)
	}
	if found.TenantID != tenantID {
		t.Fatalf("expected tenant %d, got %d", tenantID, found.TenantID)
	}
}

func TestIdentityServiceGetByEmailGlobalNotFound(t *testing.T) {
	database, _ := setupIdentityTestDB(t)
	svc := NewIdentityService(database)
	ctx := context.Background()

	if _, err := svc.GetByEmailGlobal(ctx, "nobody@example.com"); err == nil {
		t.Fatal("expected an error for an email that doesn't exist")
	}
}

func TestIdentityServiceGetByEmailGlobalAmbiguousAcrossTenants(t *testing.T) {
	database, tenant1ID := setupIdentityTestDB(t)
	svc := NewIdentityService(database)
	ctx := context.Background()

	tenant2, err := database.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Second Org", Slug: "second-org-svc",
	})
	if err != nil {
		t.Fatalf("failed to create second tenant: %v", err)
	}

	shared := "shared@example.com"
	if _, err := svc.Create(ctx, tenant1ID, identities.Identity{
		Username: "user1", Email: shared, DisplayName: "User One", Role: identities.RoleAdmin,
	}); err != nil {
		t.Fatalf("Create in tenant1 failed: %v", err)
	}
	if _, err := svc.Create(ctx, tenant2.ID, identities.Identity{
		Username: "user2", Email: shared, DisplayName: "User Two", Role: identities.RoleAdmin,
	}); err != nil {
		t.Fatalf("Create in tenant2 failed: %v", err)
	}

	if _, err := svc.GetByEmailGlobal(ctx, shared); err == nil {
		t.Fatal("expected an ambiguous-account error when two tenants share an email, got nil error")
	}
}
