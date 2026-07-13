package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/db"
)

// TestModuleOwnershipAndGrantsRoundTrip covers migration 007 on a fresh
// database: the policy_modules.owner_identity_id column and the
// module_grants table, via the generated sqlc queries
// (SetModuleOwner/GetModuleOwner/UpsertModuleGrant/ListGrantsForModule/
// DeleteModuleGrant). This table is shared by policy modules today and
// will be reused unchanged by the future Scripts feature -- see
// db/schema/007_module_ownership_grants.sql.
func TestModuleOwnershipAndGrantsRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "module_grants.db")
	ctx := context.Background()

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	tenant, err := database.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Module Org", Slug: "module-org",
	})
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	owner, err := database.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "owner1", Email: "owner1@example.com",
		DisplayName: "Owner One", Role: "user",
	})
	if err != nil {
		t.Fatalf("CreateIdentity(owner) failed: %v", err)
	}

	grantee, err := database.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "grantee1", Email: "grantee1@example.com",
		DisplayName: "Grantee One", Role: "user",
	})
	if err != nil {
		t.Fatalf("CreateIdentity(grantee) failed: %v", err)
	}

	module, err := database.Queries.CreatePolicyModule(ctx, db.CreatePolicyModuleParams{
		ModuleUrn:   "tenant.module-org.test-module",
		TenantID:    sql.NullInt64{Int64: tenant.ID, Valid: true},
		Title:       "Test Module",
		Description: sql.NullString{String: "A module for the round-trip test", Valid: true},
		IsBundled:   false,
	})
	if err != nil {
		t.Fatalf("CreatePolicyModule failed: %v", err)
	}
	if module.OwnerIdentityID.Valid {
		t.Fatalf("new module should have no owner yet, got %+v", module.OwnerIdentityID)
	}

	// Owner round-trip: set then read back via GetModuleOwner.
	if err := database.Queries.SetModuleOwner(ctx, db.SetModuleOwnerParams{
		ID:              module.ID,
		OwnerIdentityID: sql.NullInt64{Int64: owner.ID, Valid: true},
	}); err != nil {
		t.Fatalf("SetModuleOwner failed: %v", err)
	}
	gotOwner, err := database.Queries.GetModuleOwner(ctx, module.ID)
	if err != nil {
		t.Fatalf("GetModuleOwner failed: %v", err)
	}
	if !gotOwner.Valid || gotOwner.Int64 != owner.ID {
		t.Fatalf("GetModuleOwner = %+v, want valid %d", gotOwner, owner.ID)
	}
	// GetPolicyModule's SELECT * should also carry the new column.
	reloaded, err := database.Queries.GetPolicyModule(ctx, module.ID)
	if err != nil {
		t.Fatalf("GetPolicyModule failed: %v", err)
	}
	if !reloaded.OwnerIdentityID.Valid || reloaded.OwnerIdentityID.Int64 != owner.ID {
		t.Fatalf("GetPolicyModule.OwnerIdentityID = %+v, want valid %d", reloaded.OwnerIdentityID, owner.ID)
	}

	// Grant round-trip: create at "view", list, upsert to "edit" (must
	// update in place rather than duplicate, per the UNIQUE constraint),
	// list again, then delete.
	created, err := database.Queries.UpsertModuleGrant(ctx, db.UpsertModuleGrantParams{
		ModuleID:    module.ID,
		SubjectType: "identity",
		SubjectID:   grantee.ID,
		Level:       "view",
	})
	if err != nil {
		t.Fatalf("UpsertModuleGrant (create) failed: %v", err)
	}
	if created.Level != "view" {
		t.Fatalf("created grant level = %q, want view", created.Level)
	}

	grants, err := database.Queries.ListGrantsForModule(ctx, module.ID)
	if err != nil {
		t.Fatalf("ListGrantsForModule failed: %v", err)
	}
	if len(grants) != 1 || grants[0].Level != "view" {
		t.Fatalf("expected exactly one view grant, got %+v", grants)
	}

	upgraded, err := database.Queries.UpsertModuleGrant(ctx, db.UpsertModuleGrantParams{
		ModuleID:    module.ID,
		SubjectType: "identity",
		SubjectID:   grantee.ID,
		Level:       "edit",
	})
	if err != nil {
		t.Fatalf("UpsertModuleGrant (upgrade) failed: %v", err)
	}
	if upgraded.Level != "edit" {
		t.Fatalf("upgraded grant level = %q, want edit", upgraded.Level)
	}
	if upgraded.ID != created.ID {
		t.Fatalf("upsert on same (module,subject) must update in place, got new id %d != %d", upgraded.ID, created.ID)
	}

	grants, err = database.Queries.ListGrantsForModule(ctx, module.ID)
	if err != nil {
		t.Fatalf("ListGrantsForModule (after upgrade) failed: %v", err)
	}
	if len(grants) != 1 || grants[0].Level != "edit" {
		t.Fatalf("expected exactly one edit grant after upgrade, got %+v", grants)
	}

	if err := database.Queries.DeleteModuleGrant(ctx, db.DeleteModuleGrantParams{
		ModuleID:    module.ID,
		SubjectType: "identity",
		SubjectID:   grantee.ID,
	}); err != nil {
		t.Fatalf("DeleteModuleGrant failed: %v", err)
	}

	grants, err = database.Queries.ListGrantsForModule(ctx, module.ID)
	if err != nil {
		t.Fatalf("ListGrantsForModule (after delete) failed: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("expected no grants after delete, got %+v", grants)
	}

	// module_id ON DELETE CASCADE: deleting the module removes its
	// module_grants rows too.
	if _, err := database.Queries.UpsertModuleGrant(ctx, db.UpsertModuleGrantParams{
		ModuleID:    module.ID,
		SubjectType: "group",
		SubjectID:   42,
		Level:       "admin",
	}); err != nil {
		t.Fatalf("UpsertModuleGrant (for cascade check) failed: %v", err)
	}
	if err := database.Queries.DeletePolicyModule(ctx, module.ID); err != nil {
		t.Fatalf("DeletePolicyModule failed: %v", err)
	}
	var remaining int
	if err := database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM module_grants WHERE module_id = ?`, module.ID).Scan(&remaining); err != nil {
		t.Fatalf("counting module_grants after module delete failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected ON DELETE CASCADE to remove module_grants rows, found %d remaining", remaining)
	}
}
