package database

import (
	"os"
	"testing"
)

// TestMigrationsRecordedAndRunOnce: after Open, schema_migrations has one
// row per migration file; reopening does not error and does not add rows.
func TestMigrationsRecordedAndRunOnce(t *testing.T) {
	dbPath := "test_migration_tracker.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	d1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	var n1 int
	if err := d1.Conn().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n1); err != nil {
		t.Fatalf("schema_migrations missing: %v", err)
	}
	if n1 < 2 {
		t.Fatalf("expected >=2 recorded migrations, got %d", n1)
	}
	d1.Close()

	d2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer d2.Close()
	var n2 int
	if err := d2.Conn().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n2); err != nil {
		t.Fatalf("schema_migrations query after reopen: %v", err)
	}
	if n2 != n1 {
		t.Fatalf("reopen changed recorded migrations: %d -> %d", n1, n2)
	}
}
