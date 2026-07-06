package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/db"
)

// readMigrationFile reads a migration file the same way migrate() does:
// relative to the project root first, then relative to this package.
func readMigrationFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		data, err = os.ReadFile("../../" + path)
		if err != nil {
			t.Fatalf("failed to read migration file %s: %v", path, err)
		}
	}
	return data
}

// TestRolesSchemaAndQueries covers migration 003 on a fresh database:
// the new roles/identity_roles/installed_software/activity_log tables,
// the new asset and group columns, and the rebuilt identities role CHECK
// (user_self_service removed, technician and user added).
func TestRolesSchemaAndQueries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "roles_schema.db")
	ctx := context.Background()

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	tenant, err := database.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Roles Org", Slug: "roles-org",
	})
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	// Roles table + role assignment round-trip via generated queries.
	role, err := database.Queries.CreateRole(ctx, db.CreateRoleParams{
		TenantID:     tenant.ID,
		Slug:         "helpdesk",
		Name:         "Helpdesk",
		Description:  sql.NullString{String: "First-line support", Valid: true},
		IsBuiltin:    false,
		TemplateSlug: sql.NullString{String: "technician", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}

	identity, err := database.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "tech1", Email: "tech1@example.com",
		DisplayName: "Tech One", Role: "technician",
	})
	if err != nil {
		t.Fatalf("CreateIdentity with role=technician must succeed under the new CHECK: %v", err)
	}

	if err := database.Queries.AssignRoleToIdentity(ctx, db.AssignRoleToIdentityParams{
		IdentityID: identity.ID,
		RoleID:     role.ID,
	}); err != nil {
		t.Fatalf("AssignRoleToIdentity failed: %v", err)
	}
	// INSERT OR IGNORE: assigning the same pair twice must not error.
	if err := database.Queries.AssignRoleToIdentity(ctx, db.AssignRoleToIdentityParams{
		IdentityID: identity.ID,
		RoleID:     role.ID,
	}); err != nil {
		t.Fatalf("re-assigning the same role should be a no-op, got: %v", err)
	}

	got, err := database.Queries.ListRolesForIdentity(ctx, identity.ID)
	if err != nil {
		t.Fatalf("ListRolesForIdentity failed: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "helpdesk" {
		t.Fatalf("expected exactly the helpdesk role assigned, got %+v", got)
	}

	// New role vocabulary: 'user' valid, 'user_self_service' rejected.
	if _, err := database.Conn().Exec(`
		INSERT INTO identities (tenant_id, username, email, display_name, role)
		VALUES (?, 'plainuser', 'plainuser@example.com', 'Plain User', 'user')
	`, tenant.ID); err != nil {
		t.Fatalf("INSERT with role='user' must succeed: %v", err)
	}
	if _, err := database.Conn().Exec(`
		INSERT INTO identities (tenant_id, username, email, display_name, role)
		VALUES (?, 'olduser', 'olduser@example.com', 'Old User', 'user_self_service')
	`, tenant.ID); err == nil {
		t.Fatal("INSERT with role='user_self_service' must FAIL under the new CHECK, but it succeeded")
	}

	// New columns exist on assets and groups. Close the rows immediately:
	// the pool is capped at one connection, so leaked rows would deadlock
	// every query after this point.
	rows, err := database.Conn().Query(`SELECT description, managed_by_identity_id FROM assets LIMIT 0`)
	if err != nil {
		t.Fatalf("assets.description / assets.managed_by_identity_id missing: %v", err)
	}
	rows.Close()
	rows, err = database.Conn().Query(`SELECT group_category, group_scope FROM groups LIMIT 0`)
	if err != nil {
		t.Fatalf("groups.group_category / groups.group_scope missing: %v", err)
	}
	rows.Close()

	// Smoke test activity_log via generated queries.
	if err := database.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID:        tenant.ID,
		EntityType:      "identity",
		EntityID:        identity.ID,
		Event:           "created",
		Detail:          sql.NullString{String: "test entry", Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: identity.ID, Valid: true},
	}); err != nil {
		t.Fatalf("InsertActivity failed: %v", err)
	}
	activity, err := database.Queries.ListActivityForEntity(ctx, db.ListActivityForEntityParams{
		TenantID:   tenant.ID,
		EntityType: "identity",
		EntityID:   identity.ID,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListActivityForEntity failed: %v", err)
	}
	if len(activity) != 1 || activity[0].Event != "created" {
		t.Fatalf("expected one 'created' activity entry, got %+v", activity)
	}

	// Smoke test installed_software via generated queries.
	asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid: "roles-test-asset-uuid", TenantID: tenant.ID, Subtype: "computer",
		SubtypePayload: "{}", EnrollmentState: "pending", Labels: sql.NullString{String: "{}", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateAsset failed: %v", err)
	}
	sw, err := database.Queries.CreateInstalledSoftware(ctx, db.CreateInstalledSoftwareParams{
		AssetID:   asset.ID,
		Name:      "firefox",
		Version:   sql.NullString{String: "128.0", Valid: true},
		Publisher: sql.NullString{String: "Mozilla", Valid: true},
		PkgType:   "deb",
	})
	if err != nil {
		t.Fatalf("CreateInstalledSoftware failed: %v", err)
	}
	swList, err := database.Queries.ListSoftwareForAsset(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListSoftwareForAsset failed: %v", err)
	}
	if len(swList) != 1 || swList[0].ID != sw.ID || swList[0].Name != "firefox" {
		t.Fatalf("expected the firefox row back, got %+v", swList)
	}
}

// TestUpgradeFromPre003Database simulates an existing install that was
// created before migration 003 existed (only 001 and 002 applied, with an
// identity using the old 'user_self_service' role slug), then opens it
// with Open(). This proves the owner's live database will upgrade in
// place on next start: 003 must apply cleanly, remap the role to 'user',
// and create the new tables, all without losing rows.
func TestUpgradeFromPre003Database(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "upgrade_pre003.db")
	ctx := context.Background()

	// --- Phase 1: hand-build a pre-003 database (001 + 002 only). ---
	connStr := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", dbPath)
	raw, err := sql.Open("sqlite3", connStr)
	if err != nil {
		t.Fatalf("failed to open raw pre-003 database: %v", err)
	}

	for _, f := range []string{"db/schema/001_initial.sql", "db/schema/002_identity_ad_compat.sql"} {
		if _, err := raw.Exec(string(readMigrationFile(t, f))); err != nil {
			raw.Close()
			t.Fatalf("failed to execute %s: %v", f, err)
		}
	}
	if _, err := raw.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO schema_migrations(filename) VALUES
			('db/schema/001_initial.sql'),
			('db/schema/002_identity_ad_compat.sql');
	`); err != nil {
		raw.Close()
		t.Fatalf("failed to record pre-003 migrations: %v", err)
	}

	res, err := raw.Exec(`INSERT INTO tenants (name, slug) VALUES ('Legacy Org', 'legacy-org')`)
	if err != nil {
		raw.Close()
		t.Fatalf("failed to insert legacy tenant: %v", err)
	}
	tenantID, _ := res.LastInsertId()

	// The old CHECK allows user_self_service; this row must be remapped.
	res, err = raw.Exec(`
		INSERT INTO identities (tenant_id, username, email, display_name, role)
		VALUES (?, 'legacy', 'legacy@example.com', 'Legacy User', 'user_self_service')
	`, tenantID)
	if err != nil {
		raw.Close()
		t.Fatalf("failed to insert legacy user_self_service identity: %v", err)
	}
	legacyID, _ := res.LastInsertId()
	if err := raw.Close(); err != nil {
		t.Fatalf("failed to close pre-003 database: %v", err)
	}

	// --- Phase 2: open with Open(), which applies 003 in place. ---
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open on a pre-003 database must upgrade cleanly, got: %v", err)
	}
	defer database.Close()

	upgraded, err := database.Queries.GetIdentity(ctx, legacyID)
	if err != nil {
		t.Fatalf("legacy identity lost during 003 rebuild: %v", err)
	}
	if upgraded.Role != "user" {
		t.Fatalf("expected legacy role user_self_service remapped to 'user', got %q", upgraded.Role)
	}
	if upgraded.Email != "legacy@example.com" || upgraded.DisplayName != "Legacy User" {
		t.Fatalf("legacy identity data corrupted during rebuild: %+v", upgraded)
	}

	// New tables must exist after the upgrade.
	var n int
	if err := database.Conn().QueryRow(`SELECT COUNT(*) FROM roles`).Scan(&n); err != nil {
		t.Fatalf("roles table missing after upgrade: %v", err)
	}
	if err := database.Conn().QueryRow(`SELECT COUNT(*) FROM identity_roles`).Scan(&n); err != nil {
		t.Fatalf("identity_roles table missing after upgrade: %v", err)
	}

	// 003 must be recorded so it never runs twice.
	if err := database.Conn().QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE filename = 'db/schema/003_roles_software_logs.sql'`,
	).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected 003 recorded exactly once, count=%d err=%v", n, err)
	}

	// Reopen once more: 003 must not re-run (rebuild is not idempotent).
	database.Close()
	database2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen after upgrade failed: %v", err)
	}
	defer database2.Close()
	again, err := database2.Queries.GetIdentity(ctx, legacyID)
	if err != nil || again.Role != "user" {
		t.Fatalf("identity did not survive a post-upgrade restart: role=%q err=%v", again.Role, err)
	}
}
