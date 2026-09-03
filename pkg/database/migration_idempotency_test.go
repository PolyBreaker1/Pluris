package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/db"
)

// TestReopeningDatabasePreservesIdentities is a regression test for a
// critical bug: migrate() re-runs every migration file on every Open()
// call, and an earlier version of 002_identity_ad_compat.sql had
// DROP TABLE statements that wiped all identities/sessions/audit-log
// rows on every single app restart. This test simulates a restart
// (closing and re-opening the same database file) and confirms an
// identity created before the "restart" still exists after it.
func TestReopeningDatabasePreservesIdentities(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_reopen_preserves_data.db")

	ctx := context.Background()

	// First "boot": create a tenant + identity.
	database1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	tenant, err := database1.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Test Org", Slug: "test-org-reopen",
	})
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	identity, err := database1.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "jdoe", Email: "jdoe@example.com",
		DisplayName: "Jane Doe", Role: "admin",
	})
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}
	database1.Close()

	// Simulate a restart: open a brand-new *Database against the SAME
	// file. This re-runs migrate() on every migration file, which is
	// exactly the scenario that used to wipe everything.
	database2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open (simulated restart) failed: %v", err)
	}
	defer database2.Close()

	got, err := database2.Queries.GetIdentity(ctx, identity.ID)
	if err != nil {
		t.Fatalf("identity created before 'restart' is gone after reopening the database: %v", err)
	}
	if got.Email != "jdoe@example.com" {
		t.Fatalf("expected email jdoe@example.com to survive a restart, got %s", got.Email)
	}

	gotTenant, err := database2.Queries.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("tenant created before 'restart' is gone after reopening the database: %v", err)
	}
	if gotTenant.Name != "Test Org" {
		t.Fatalf("expected tenant name 'Test Org' to survive a restart, got %s", gotTenant.Name)
	}
}
