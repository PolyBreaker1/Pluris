package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func setupIdentityTestDB(t *testing.T) (*database.Database, int64) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_identity_service.db")

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

func TestIdentitySoftDeleteRestoreImmediateAndPurge(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	svc := NewIdentityService(database)
	retention := NewRetentionService(database)
	create := func(username string) identities.Identity {
		t.Helper()
		identity, err := svc.Create(ctx, tenantID, identities.Identity{
			Username: username, Email: username + "@example.com", DisplayName: username, Role: identities.RoleUser,
		})
		if err != nil {
			t.Fatal(err)
		}
		return identity
	}

	soft := create("soft-user")
	if err := svc.Delete(ctx, tenantID, soft.ID, 99); err != nil {
		t.Fatal(err)
	}
	active, err := svc.List(ctx, tenantID, 100, 0)
	if err != nil || len(active) != 0 {
		t.Fatalf("active identities = %+v, %v", active, err)
	}
	deleted, err := svc.ListDeleted(ctx, tenantID, 100, 0)
	if err != nil || len(deleted) != 1 || deleted[0].ID != soft.ID {
		t.Fatalf("deleted identities = %+v, %v", deleted, err)
	}
	if _, err := svc.GetByEmailGlobal(ctx, soft.Email); err == nil {
		t.Fatal("soft-deleted identity can still resolve for login")
	}
	targets, err := NewTargetService(database).Catalog(ctx, tenantID, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Ref == fmt.Sprintf("%d", soft.ID) && target.Kind == configgroups.KindUser {
			t.Fatalf("soft-deleted identity leaked into picker: %+v", target)
		}
	}
	if err := svc.Restore(ctx, tenantID, soft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, soft.ID); err != nil {
		t.Fatalf("get after restore: %v", err)
	}

	if _, err := retention.UpdateSetting(ctx, EntityKindIdentity, RetentionModeImmediate, nil, 99); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, soft.ID, 99); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Queries.GetIdentityIncludingDeleted(ctx, soft.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("immediate identity lookup = %v", err)
	}

	if _, err := retention.UpdateSetting(ctx, EntityKindIdentity, RetentionModeSoft, nil, 99); err != nil {
		t.Fatal(err)
	}
	expired := create("expired-user")
	if err := svc.Delete(ctx, tenantID, expired.ID, 99); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Conn().ExecContext(ctx, "UPDATE identities SET deleted_at = datetime('now', '-2 days') WHERE id = ?", expired.ID); err != nil {
		t.Fatal(err)
	}
	results, err := retention.PurgeExpired(ctx)
	if err != nil || len(results) != 0 {
		t.Fatalf("NULL identity window purge = %+v, %v", results, err)
	}
	days := int64(1)
	if _, err := retention.UpdateSetting(ctx, EntityKindIdentity, RetentionModeSoft, &days, 99); err != nil {
		t.Fatal(err)
	}
	results, err = retention.PurgeExpired(ctx)
	if err != nil || len(results) != 1 || results[0].EntityKind != EntityKindIdentity || !results[0].Purged {
		t.Fatalf("identity purge = %+v, %v", results, err)
	}
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

	if err := svc.Delete(ctx, tenantID, created.ID, created.ID); err != nil {
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
