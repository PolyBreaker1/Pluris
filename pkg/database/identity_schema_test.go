package database

import (
	"os"
	"testing"
)

func TestIdentitySchemaHasRichColumns(t *testing.T) {
	dbPath := "test_identity_schema.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	res, err := database.Conn().Exec(`
		INSERT INTO tenants (name, slug) VALUES ('Test Org', 'test-org-ident')
	`)
	if err != nil {
		t.Fatalf("failed to insert tenant: %v", err)
	}
	tenantID, _ := res.LastInsertId()

	_, err = database.Conn().Exec(`
		INSERT INTO identities (
			tenant_id, username, email, display_name, title, department,
			role, account_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, tenantID, "jdoe", "jdoe@example.com", "Jane Doe", "IT Manager", "IT",
		"admin", true)
	if err != nil {
		t.Fatalf("expected rich identities schema, insert failed: %v", err)
	}

	var count int
	err = database.Conn().QueryRow(`
		SELECT COUNT(*) FROM identity_sessions
	`).Scan(&count)
	if err != nil {
		t.Fatalf("expected identity_sessions table to exist: %v", err)
	}

	err = database.Conn().QueryRow(`
		SELECT COUNT(*) FROM identity_audit_log
	`).Scan(&count)
	if err != nil {
		t.Fatalf("expected identity_audit_log table to exist: %v", err)
	}
}
