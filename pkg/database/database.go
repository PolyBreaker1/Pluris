package database

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pluris/pluris/db"
	schema "github.com/pluris/pluris/db/schema"
)

// Database wraps the SQL connection and sqlc queries
type Database struct {
	conn    *sql.DB
	Queries *db.Queries
}

// Open creates or opens the SQLite database with WAL mode
func Open(dbPath string) (*Database, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open with SQLite pragmas for concurrency
	connStr := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", dbPath)
	conn, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool for SQLite
	conn.SetMaxOpenConns(1) // SQLite only allows one writer
	conn.SetMaxIdleConns(1)

	database := &Database{
		conn:    conn,
		Queries: db.New(conn),
	}

	// Run migrations
	if err := database.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return database, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.conn.Close()
}

// Conn returns the underlying sql.DB connection
func (d *Database) Conn() *sql.DB {
	return d.conn
}

// migrate runs all SQL schema files exactly once per database file, recorded in
// schema_migrations table. Migrations are idempotent via CREATE TABLE IF NOT EXISTS
// for mutations 001-002; migration 003 onward may contain non-idempotent statements
// (ALTER TABLE) ONLY because of this tracker — such migrations MUST NOT contain
// DROP TABLE statements (see header comment in db/schema/002_identity_ad_compat.sql
// for the earlier incident that corrupted user data on every app restart).
// The one sanctioned exception is 003's identities rename-rebuild, whose
// DROP is part of an atomic copy-drop-rename that preserves every row and
// runs exactly once thanks to this tracker.
//
// Each PRAGMA-free migration executes and is recorded inside a single
// transaction, so a crash mid-migration cannot leave the schema changed but
// unrecorded (SQLite DDL is transactional). Migrations containing PRAGMA
// statements run outside a transaction — SQLite silently ignores
// foreign_keys pragma changes mid-transaction and cannot change journal_mode
// inside one — so they must manage their own atomicity. Cross-PROCESS races
// are out of scope: Pluris deploys as a single server process (SQLite
// single-writer plus a connection pool capped at 1 handles in-process
// concurrency).
func (d *Database) migrate() error {
	// Create migration tracker table first
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := d.conn.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	migrations := []string{
		"db/schema/001_initial.sql",
		"db/schema/002_identity_ad_compat.sql",
		"db/schema/003_roles_software_logs.sql",
		"db/schema/004_dependency_groups.sql",
		"db/schema/005_role_hierarchy_group_roles.sql",
		"db/schema/006_condition_builder.sql",
		"db/schema/007_module_ownership_grants.sql",
		"db/schema/008_module_scripts.sql",
		"db/schema/009_group_kinds_rules.sql",
		"db/schema/010_soft_delete_retention.sql",
		"db/schema/011_module_tests_origin.sql",
		"db/schema/012_module_scripts_actions.sql",
	}

	for _, migrationFile := range migrations {
		// Check if this migration has already been applied
		var alreadyApplied int
		err := d.conn.QueryRow(`SELECT 1 FROM schema_migrations WHERE filename = ?`, migrationFile).Scan(&alreadyApplied)
		if err == nil {
			// Migration already applied, skip it
			continue
		}

		// Migrations are embedded in the binary (db/schema/embed.go) so
		// a built console migrates correctly from any working directory.
		// A missing embedded file is a build defect, never skippable:
		// silently skipping used to leave fresh installs with no schema.
		schemaSQL, err := schema.Files.ReadFile(path.Base(migrationFile))
		if err != nil {
			return fmt.Errorf("embedded migration %s missing: %w", migrationFile, err)
		}

		if strings.Contains(strings.ToUpper(string(schemaSQL)), "PRAGMA") {
			// PRAGMA-bearing migrations (e.g. 001's journal_mode=WAL, or a
			// future 003's foreign_keys=OFF table rebuild) cannot run inside
			// an outer transaction: SQLite silently ignores foreign_keys
			// pragma changes mid-transaction and journal_mode cannot change
			// inside one. Execute then record as two separate statements;
			// the migration file manages its own atomicity.
			if _, err := d.conn.Exec(string(schemaSQL)); err != nil {
				return fmt.Errorf("failed to execute %s: %w", migrationFile, err)
			}
			// Record this migration as applied (only after successful execution)
			if _, err := d.conn.Exec(`INSERT INTO schema_migrations(filename) VALUES(?)`, migrationFile); err != nil {
				return fmt.Errorf("failed to record migration %s: %w", migrationFile, err)
			}
			continue
		}

		// Execute and record atomically: if the process dies mid-migration,
		// the whole transaction rolls back and the migration re-runs cleanly
		// on next boot instead of re-running an already-applied migration.
		tx, err := d.conn.Begin()
		if err != nil {
			return fmt.Errorf("begin migration tx for %s: %w", migrationFile, err)
		}
		if _, err := tx.Exec(string(schemaSQL)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute %s: %w", migrationFile, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(filename) VALUES(?)`, migrationFile); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record %s: %w", migrationFile, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit %s: %w", migrationFile, err)
		}
	}

	return nil
}

// BeginTx starts a transaction
func (d *Database) BeginTx() (*sql.Tx, error) {
	return d.conn.Begin()
}
