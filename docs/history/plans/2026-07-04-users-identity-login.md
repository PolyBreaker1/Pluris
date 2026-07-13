# Users / Identity System + Real Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stub `/users` page and the orphaned identity schema with a real, AD-familiar identity directory plus the app's first working authentication (login, sessions, RBAC, first-run setup wizard, super_admin tenant switcher), and pair identities to assets as owners.

**Architecture:** Follows the existing Asset pattern end-to-end: `db/schema` → `db/queries` (sqlc) → `catalog/identities` (pure types) → `pkg/services` (DB adapter) → `console/handlers` → `web/templates`/`web/lists`. Auth is a new `pkg/auth` package (password hashing, sessions, RBAC, Echo middleware) that injects the current session into `context.Context`, which templ components read directly (templ passes `ctx` into every component's generated `Render` function) — this means the shared `Layout`/`Header` chrome can show the real logged-in user and tenant switcher without changing the signature of the ~20 existing page templates that call `@Layout(...)`.

**Tech Stack:** Go 1.25, Echo v4, sqlc (SQLite), Templ, `golang.org/x/crypto/argon2`, standard library `crypto/rand`/`crypto/sha256`.

**Spec:** `docs/superpowers/specs/2026-07-04-users-identity-login-design.md`

---

## Working agreement for every task

- Run all Go commands from the repo root: `/home/peter/AI Builds/Pluris/Pluris-main`.
- After editing any `.templ` file, run `~/go/bin/templ generate` before `go build`/`go test`.
- After editing `db/schema/*.sql` or `db/queries/*.sql`, run `~/go/bin/sqlc generate` before `go build`/`go test`.
- Use `go build -buildvcs=false ./...` and `go test -buildvcs=false ./...` (this working copy has no `.git`, so `-buildvcs=false` avoids the VCS-stamping error seen earlier in this project).
- If a generated sqlc method/struct name in a code sample doesn't exactly match what `sqlc generate` produces, trust the compiler error over the sample — fix the call site to match the generated signature, don't fight sqlc's naming.

---

### Task 1: Identity schema migration

**Files:**
- Modify: `db/schema/002_identity_ad_compat.sql` (full rewrite — this migration has never been applied to any real database; see spec §4)
- Modify: `pkg/database/database.go:70-72` (add the migration to the list)
- Test: `pkg/database/identity_schema_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
// pkg/database/identity_schema_test.go
package database

import (
	"database/sql"
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

	tenant, err := database.Queries.CreateTenant(nil, struct{}{}) //nolint:staticcheck // placeholder call removed in Step 3
	_ = tenant
	_ = err

	// Raw insert exercising the new identities columns. Uses database/sql
	// directly (not sqlc) because this test only needs to prove the
	// migration ran — it must not depend on Task 2's query rewrite.
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

var _ = sql.ErrNoRows
```

- [ ] **Step 2: Delete the bogus placeholder line and fix imports**

The `CreateTenant(nil, struct{}{})` line above was written wrong on purpose to
force you to look at it — delete it now along with the `_ = tenant` / `_ = err`
lines and the trailing `var _ = sql.ErrNoRows`. The real test body is the two
`database.Conn().Exec(...)` calls plus the two `QueryRow` checks. Remove the
unused `"database/sql"` import if nothing else in the file uses it.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/database/... -run TestIdentitySchemaHasRichColumns -v`
Expected: FAIL — `no such column: title` (or similar), because migration 002
is not yet in the `migrate()` list and its content is still the orphaned
OU/sync version.

- [ ] **Step 4: Rewrite the schema file**

Replace the entire content of `db/schema/002_identity_ad_compat.sql` with:

```sql
-- Migration 002: Identity system with AD-familiar attributes.
--
-- Supersedes the identities table created in 001_initial.sql. That table
-- is dropped and recreated here because this project has no production
-- data yet (pre-launch) — see docs/ARCHITECTURE_DECISIONS.md and
-- docs/superpowers/specs/2026-07-04-users-identity-login-design.md.
--
-- Scope note: this migration intentionally does NOT include an
-- organizational-unit tree, AD-sync config table, or sync-tracking
-- columns (source/source_guid/last_synced_at/...). Those were drafted in
-- an earlier, never-applied version of this file but have no sync engine
-- behind them (ADR-009: external directory sync is Phase 2). Add them
-- back when that engine actually exists.

DROP TABLE IF EXISTS identity_audit_log;
DROP TABLE IF EXISTS identity_sessions;
DROP TABLE IF EXISTS identities;

CREATE TABLE IF NOT EXISTS identities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    site_id INTEGER REFERENCES sites(id) ON DELETE SET NULL,

    -- Account
    username TEXT NOT NULL,
    user_principal_name TEXT,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    given_name TEXT,
    surname TEXT,
    initials TEXT,

    -- Organization
    title TEXT,
    department TEXT,
    company TEXT,
    employee_id TEXT,
    employee_type TEXT,
    manager_id INTEGER REFERENCES identities(id) ON DELETE SET NULL,

    -- Contact
    phone_office TEXT,
    phone_mobile TEXT,
    phone_home TEXT,
    fax TEXT,

    -- Location
    office TEXT,
    street_address TEXT,
    city TEXT,
    state TEXT,
    postal_code TEXT,
    country TEXT,
    country_code TEXT,

    -- Profile & scripts (Windows-familiar)
    home_directory TEXT,
    home_drive TEXT,
    profile_path TEXT,
    logon_script TEXT,

    -- Security / login
    account_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    account_locked BOOLEAN NOT NULL DEFAULT FALSE,
    account_expires_at TIMESTAMP,
    password_hash TEXT,
    password_last_set_at TIMESTAMP,
    password_never_expires BOOLEAN NOT NULL DEFAULT FALSE,
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    last_logon_at TIMESTAMP,
    logon_count INTEGER NOT NULL DEFAULT 0,
    bad_password_count INTEGER NOT NULL DEFAULT 0,
    last_bad_password_at TIMESTAMP,

    -- Pluris-specific
    role TEXT NOT NULL DEFAULT 'user_self_service'
        CHECK(role IN ('super_admin', 'admin', 'user_self_service')),
    avatar_url TEXT,
    locale TEXT NOT NULL DEFAULT 'en-US',
    timezone TEXT NOT NULL DEFAULT 'UTC',

    -- Metadata
    description TEXT,
    notes TEXT,

    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(tenant_id, username),
    UNIQUE(tenant_id, email)
);

CREATE INDEX IF NOT EXISTS idx_identities_tenant ON identities(tenant_id);
CREATE INDEX IF NOT EXISTS idx_identities_site ON identities(site_id);
CREATE INDEX IF NOT EXISTS idx_identities_email ON identities(email);
CREATE INDEX IF NOT EXISTS idx_identities_username ON identities(username);
CREATE INDEX IF NOT EXISTS idx_identities_manager ON identities(manager_id);
CREATE INDEX IF NOT EXISTS idx_identities_department ON identities(department);
CREATE INDEX IF NOT EXISTS idx_identities_role ON identities(role);

CREATE TABLE IF NOT EXISTS identity_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    active_tenant_id INTEGER REFERENCES tenants(id) ON DELETE SET NULL,
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP,
    UNIQUE(token_hash)
);

CREATE INDEX IF NOT EXISTS idx_sessions_identity ON identity_sessions(identity_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON identity_sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON identity_sessions(expires_at);

CREATE TABLE IF NOT EXISTS identity_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER REFERENCES tenants(id) ON DELETE CASCADE,
    identity_id INTEGER REFERENCES identities(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL CHECK(event_type IN (
        'login_success', 'login_failure', 'logout',
        'password_change', 'account_locked', 'created', 'updated'
    )),
    ip_address TEXT,
    detail TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_tenant ON identity_audit_log(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_identity ON identity_audit_log(identity_id);
CREATE INDEX IF NOT EXISTS idx_audit_event_type ON identity_audit_log(event_type);
```

- [ ] **Step 5: Wire the migration into `migrate()`**

In `pkg/database/database.go`, change:

```go
	migrations := []string{
		"db/schema/001_initial.sql",
	}
```

to:

```go
	migrations := []string{
		"db/schema/001_initial.sql",
		"db/schema/002_identity_ad_compat.sql",
	}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/database/... -run TestIdentitySchemaHasRichColumns -v`
Expected: PASS

- [ ] **Step 7: Run the full existing test suite to check for regressions**

Run: `go test -buildvcs=false ./...`
Expected: all packages still pass (this migration replaces a table other
tests don't touch directly, but `assets.owner_identity_id` references
`identities(id)` — confirm `pkg/database` and `pkg/services` tests still
pass).

- [ ] **Step 8: Delete the stale dev database so the new schema applies cleanly**

The running `pluris.db` was created under the old (never-migrated-002)
schema. `IF NOT EXISTS` guards mean `identities` from 001 already exists,
so 002's `DROP TABLE identities` will still fire correctly on next
startup — but confirm by deleting the dev artifacts and letting `make dev`
recreate them:

Run: `rm -f pluris.db pluris.db-shm pluris.db-wal`

- [ ] **Step 9: Commit**

```bash
git add db/schema/002_identity_ad_compat.sql pkg/database/database.go pkg/database/identity_schema_test.go
git commit -m "feat: apply trimmed AD-familiar identity schema (sessions + audit log)"
```

---

### Task 2: Identity query layer (sqlc)

**Files:**
- Modify: `db/queries/identities.sql` (full rewrite)
- Create: `db/queries/sessions.sql`
- Create: `db/queries/identity_audit.sql`
- Test: `pkg/database/identity_queries_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
// pkg/database/identity_queries_test.go
package database

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/pluris/pluris/db"
)

func TestIdentityQueriesCRUD(t *testing.T) {
	dbPath := "test_identity_queries.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	tenant, err := database.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Test Org", Slug: "test-org-idq",
	})
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	identity, err := database.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID:    tenant.ID,
		Username:    "jdoe",
		Email:       "jdoe@example.com",
		DisplayName: "Jane Doe",
		Role:        "admin",
	})
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}
	if identity.Username != "jdoe" {
		t.Fatalf("expected username jdoe, got %s", identity.Username)
	}

	got, err := database.Queries.GetIdentity(ctx, identity.ID)
	if err != nil {
		t.Fatalf("failed to get identity: %v", err)
	}
	if got.Email != "jdoe@example.com" {
		t.Fatalf("expected email jdoe@example.com, got %s", got.Email)
	}

	byEmail, err := database.Queries.GetIdentityByEmail(ctx, db.GetIdentityByEmailParams{
		TenantID: tenant.ID, Email: "jdoe@example.com",
	})
	if err != nil {
		t.Fatalf("failed to get identity by email: %v", err)
	}
	if byEmail.ID != identity.ID {
		t.Fatalf("expected id %d, got %d", identity.ID, byEmail.ID)
	}

	list, err := database.Queries.ListIdentitiesByTenant(ctx, db.ListIdentitiesByTenantParams{
		TenantID: tenant.ID, Limit: 100, Offset: 0,
	})
	if err != nil {
		t.Fatalf("failed to list identities: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(list))
	}

	err = database.Queries.SetIdentityPasswordHash(ctx, db.SetIdentityPasswordHashParams{
		ID:           identity.ID,
		PasswordHash: sql.NullString{String: "hashed", Valid: true},
	})
	if err != nil {
		t.Fatalf("failed to set password hash: %v", err)
	}

	session, err := database.Queries.CreateIdentitySession(ctx, db.CreateIdentitySessionParams{
		IdentityID: identity.ID,
		TokenHash:  "deadbeef",
		ExpiresAt:  sql.NullTime{}, // Step 3 replaces this with a real time
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	_ = session

	err = database.Queries.InsertIdentityAuditLog(ctx, db.InsertIdentityAuditLogParams{
		TenantID:   sql.NullInt64{Int64: tenant.ID, Valid: true},
		IdentityID: sql.NullInt64{Int64: identity.ID, Valid: true},
		EventType:  "created",
	})
	if err != nil {
		t.Fatalf("failed to insert audit log: %v", err)
	}
}
```

- [ ] **Step 2: Fix the `ExpiresAt` placeholder before running**

`sql.NullTime{}` above is a zero-value placeholder — SQLite's `TIMESTAMP`
column will accept it, but replace it with a real future time so later
tasks that filter on `expires_at > now()` have something valid to test
against:

```go
	session, err := database.Queries.CreateIdentitySession(ctx, db.CreateIdentitySessionParams{
		IdentityID: identity.ID,
		TokenHash:  "deadbeef",
		ExpiresAt:  time.Now().Add(30 * 24 * time.Hour),
	})
```

Add `"time"` to the imports.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/database/... -run TestIdentityQueriesCRUD -v`
Expected: FAIL to compile — `db.CreateIdentityParams` has no field
`DisplayName`/`Role` etc. matching this shape yet (current generated code
still reflects the old 3-column `identities` table).

- [ ] **Step 4: Rewrite `db/queries/identities.sql`**

```sql
-- Identity management queries — matches the schema in
-- db/schema/002_identity_ad_compat.sql. Identities are both the
-- end-user directory (owner of assets) and the accounts that log into
-- the console (role-gated).

-- name: CreateIdentity :one
INSERT INTO identities (
    tenant_id, site_id, username, user_principal_name, email, display_name,
    given_name, surname, title, department, company, employee_id,
    employee_type, manager_id, phone_office, phone_mobile, role
) VALUES (
    @tenant_id, @site_id, @username, @user_principal_name, @email, @display_name,
    @given_name, @surname, @title, @department, @company, @employee_id,
    @employee_type, @manager_id, @phone_office, @phone_mobile, @role
) RETURNING *;

-- name: GetIdentity :one
SELECT * FROM identities WHERE id = @id LIMIT 1;

-- name: GetIdentityByEmail :one
SELECT * FROM identities
WHERE tenant_id = @tenant_id AND email = @email
LIMIT 1;

-- name: GetIdentityByEmailGlobal :one
-- Used at login: the user only enters an email, not a tenant, so this
-- resolves both across all tenants. Fails closed if two tenants somehow
-- share an email (UNIQUE is per-tenant, so this can return >1 row in
-- theory; callers must handle sql.ErrNoRows and treat >1 as an error).
SELECT * FROM identities WHERE email = @email LIMIT 1;

-- name: ListIdentitiesByTenant :many
SELECT * FROM identities
WHERE tenant_id = @tenant_id
ORDER BY display_name
LIMIT @limit OFFSET @offset;

-- name: CountIdentitiesByTenant :one
SELECT COUNT(*) FROM identities WHERE tenant_id = @tenant_id;

-- name: CountIdentitiesGlobal :one
-- Used by the setup-gate middleware: "does any identity exist anywhere?"
SELECT COUNT(*) FROM identities;

-- name: SearchIdentities :many
SELECT * FROM identities
WHERE tenant_id = @tenant_id
  AND (display_name LIKE '%' || @search || '%'
       OR email LIKE '%' || @search || '%'
       OR username LIKE '%' || @search || '%'
       OR department LIKE '%' || @search || '%')
ORDER BY display_name
LIMIT @limit;

-- name: UpdateIdentity :one
UPDATE identities SET
    display_name = @display_name,
    given_name = @given_name,
    surname = @surname,
    email = @email,
    title = @title,
    department = @department,
    company = @company,
    employee_id = @employee_id,
    employee_type = @employee_type,
    manager_id = @manager_id,
    phone_office = @phone_office,
    phone_mobile = @phone_mobile,
    site_id = @site_id,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING *;

-- name: UpdateIdentityRole :exec
UPDATE identities SET role = @role, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetIdentityEnabled :exec
UPDATE identities SET account_enabled = @account_enabled, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetIdentityLocked :exec
UPDATE identities SET account_locked = @account_locked, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetIdentityPasswordHash :exec
UPDATE identities SET
    password_hash = @password_hash,
    password_last_set_at = CURRENT_TIMESTAMP,
    must_change_password = FALSE,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: RecordLoginSuccess :exec
UPDATE identities SET
    last_logon_at = CURRENT_TIMESTAMP,
    logon_count = logon_count + 1,
    bad_password_count = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: RecordLoginFailure :exec
UPDATE identities SET
    bad_password_count = bad_password_count + 1,
    last_bad_password_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: LockIdentityIfThresholdExceeded :exec
UPDATE identities SET account_locked = TRUE
WHERE id = @id AND bad_password_count >= @threshold;

-- name: DeleteIdentity :exec
DELETE FROM identities WHERE id = @id;
```

- [ ] **Step 5: Create `db/queries/sessions.sql`**

```sql
-- Server-side session queries. The raw session token is never stored —
-- only its SHA-256 hash (token_hash). See pkg/auth/session.go.

-- name: CreateIdentitySession :one
INSERT INTO identity_sessions (
    identity_id, token_hash, ip_address, user_agent, expires_at
) VALUES (
    @identity_id, @token_hash, @ip_address, @user_agent, @expires_at
) RETURNING *;

-- name: GetActiveSessionByTokenHash :one
SELECT * FROM identity_sessions
WHERE token_hash = @token_hash
  AND revoked_at IS NULL
  AND expires_at > CURRENT_TIMESTAMP
LIMIT 1;

-- name: SetSessionActiveTenant :exec
UPDATE identity_sessions SET active_tenant_id = @active_tenant_id WHERE id = @id;

-- name: RevokeSession :exec
UPDATE identity_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash = @token_hash;

-- name: DeleteExpiredSessions :exec
DELETE FROM identity_sessions WHERE expires_at <= CURRENT_TIMESTAMP;
```

- [ ] **Step 6: Create `db/queries/identity_audit.sql`**

```sql
-- Audit trail for authentication and identity-management events.

-- name: InsertIdentityAuditLog :exec
INSERT INTO identity_audit_log (
    tenant_id, identity_id, event_type, ip_address, detail
) VALUES (
    @tenant_id, @identity_id, @event_type, @ip_address, @detail
);

-- name: ListIdentityAuditLogByTenant :many
SELECT * FROM identity_audit_log
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @limit;
```

- [ ] **Step 7: Regenerate sqlc code**

Run: `~/go/bin/sqlc generate`
Expected: exits 0, regenerates `db/*.sql.go` and `db/models.go`.

- [ ] **Step 8: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/database/... -run TestIdentityQueriesCRUD -v`
Expected: PASS. If field names differ from the sample (e.g. sqlc emits
`Fax` vs `FAX`), fix the test call sites to match — the generated code is
authoritative.

- [ ] **Step 9: Run the full suite**

Run: `go build -buildvcs=false ./... && go test -buildvcs=false ./...`
Expected: builds clean, all tests pass (the old `identities.sql` queries
this replaced were only used by the never-wired stub, so nothing else
should reference the old shape — if `go build` finds another call site,
that's real signal to investigate, not to work around).

- [ ] **Step 10: Commit**

```bash
git add db/queries/identities.sql db/queries/sessions.sql db/queries/identity_audit.sql db/*.sql.go db/models.go db/querier.go pkg/database/identity_queries_test.go
git commit -m "feat: rewrite identity queries for the AD-familiar schema, add session/audit queries"
```

---

### Task 3: `catalog/identities` types

**Files:**
- Create: `catalog/identities/types.go`
- Test: `catalog/identities/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
// catalog/identities/types_test.go
package identities

import "testing"

func TestIdentityDisplayNameFallsBackToGivenSurname(t *testing.T) {
	id := Identity{GivenName: "Jane", Surname: "Doe"}
	if got := id.ResolvedDisplayName(); got != "Jane Doe" {
		t.Fatalf("expected 'Jane Doe', got %q", got)
	}

	id2 := Identity{DisplayName: "J. Doe", GivenName: "Jane", Surname: "Doe"}
	if got := id2.ResolvedDisplayName(); got != "J. Doe" {
		t.Fatalf("expected explicit DisplayName to win, got %q", got)
	}

	id3 := Identity{Username: "jdoe"}
	if got := id3.ResolvedDisplayName(); got != "jdoe" {
		t.Fatalf("expected fallback to username, got %q", got)
	}
}

func TestRoleIsValid(t *testing.T) {
	valid := []Role{RoleSuperAdmin, RoleAdmin, RoleUserSelfService}
	for _, r := range valid {
		if !r.IsValid() {
			t.Fatalf("expected %q to be valid", r)
		}
	}
	if Role("bogus").IsValid() {
		t.Fatal("expected 'bogus' role to be invalid")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./catalog/identities/... -v`
Expected: FAIL to compile — package `identities` doesn't exist yet.

- [ ] **Step 3: Write `catalog/identities/types.go`**

```go
// Package identities is the canonical in-memory shape for the Identity
// entity — the end-user/directory record that both owns assets and (when
// role-gated) logs into the console. pkg/services.IdentityService adapts
// database rows into this shape, mirroring how catalog/assets is the
// shape pkg/services.AssetService adapts into.
package identities

import "time"

// Role gates access per docs/UX_INVARIANTS.md's locked permission matrix.
type Role string

const (
	RoleSuperAdmin       Role = "super_admin"
	RoleAdmin            Role = "admin"
	RoleUserSelfService  Role = "user_self_service"
)

// IsValid reports whether r is one of the three locked v1 roles.
func (r Role) IsValid() bool {
	switch r {
	case RoleSuperAdmin, RoleAdmin, RoleUserSelfService:
		return true
	}
	return false
}

// Label returns the human display name for the role.
func (r Role) Label() string {
	switch r {
	case RoleSuperAdmin:
		return "Super Admin"
	case RoleAdmin:
		return "Admin"
	case RoleUserSelfService:
		return "User (Self-Service)"
	}
	return string(r)
}

// Identity is the canonical shape used by every Users surface (list,
// detail, editor) and by the Asset owner picker.
type Identity struct {
	ID       int64
	TenantID int64
	SiteID   int64 // 0 when unset

	Username          string
	UserPrincipalName string
	Email             string
	DisplayName       string
	GivenName         string
	Surname           string
	Initials          string

	Title        string
	Department   string
	Company      string
	EmployeeID   string
	EmployeeType string
	ManagerID    int64 // 0 when unset

	PhoneOffice string
	PhoneMobile string
	PhoneHome   string
	Fax         string

	Office        string
	StreetAddress string
	City          string
	State         string
	PostalCode    string
	Country       string
	CountryCode   string

	HomeDirectory string
	HomeDrive     string
	ProfilePath   string
	LogonScript   string

	AccountEnabled       bool
	AccountLocked        bool
	AccountExpiresAt     *time.Time
	PasswordLastSetAt    *time.Time
	PasswordNeverExpires bool
	MustChangePassword   bool
	LastLogonAt          *time.Time
	LogonCount           int64
	BadPasswordCount     int64

	Role      Role
	AvatarURL string
	Locale    string
	Timezone  string

	Description string
	Notes       string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ResolvedDisplayName returns DisplayName if set, else "GivenName Surname"
// if either is set, else Username. Used wherever a human-readable label
// is needed and DisplayName might not have been backfilled.
func (i Identity) ResolvedDisplayName() string {
	if i.DisplayName != "" {
		return i.DisplayName
	}
	full := i.GivenName
	if i.Surname != "" {
		if full != "" {
			full += " "
		}
		full += i.Surname
	}
	if full != "" {
		return full
	}
	return i.Username
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./catalog/identities/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add catalog/identities/types.go catalog/identities/types_test.go
git commit -m "feat: add catalog/identities canonical types"
```

---

### Task 4: `IdentityService` (CRUD)

**Files:**
- Create: `pkg/services/identities.go`
- Test: `pkg/services/identities_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/services/identities_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/services/... -run TestIdentityService -v`
Expected: FAIL to compile — `NewIdentityService` doesn't exist.

- [ ] **Step 3: Write `pkg/services/identities.go`**

```go
package services

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// IdentityService handles identity (user directory) operations, mirroring
// AssetService's role for the Asset entity.
type IdentityService struct {
	db *database.Database
}

func NewIdentityService(db *database.Database) *IdentityService {
	return &IdentityService{db: db}
}

func nullInt64(v int64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

// Create inserts a new identity for tenantID.
func (s *IdentityService) Create(ctx context.Context, tenantID int64, in identities.Identity) (identities.Identity, error) {
	row, err := s.db.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID:          tenantID,
		SiteID:            nullInt64(in.SiteID),
		Username:          in.Username,
		UserPrincipalName: nullString(in.UserPrincipalName),
		Email:             in.Email,
		DisplayName:       in.DisplayName,
		GivenName:         nullString(in.GivenName),
		Surname:           nullString(in.Surname),
		Title:             nullString(in.Title),
		Department:        nullString(in.Department),
		Company:           nullString(in.Company),
		EmployeeID:        nullString(in.EmployeeID),
		EmployeeType:      nullString(in.EmployeeType),
		ManagerID:         nullInt64(in.ManagerID),
		PhoneOffice:       nullString(in.PhoneOffice),
		PhoneMobile:       nullString(in.PhoneMobile),
		Role:              string(in.Role),
	})
	if err != nil {
		return identities.Identity{}, fmt.Errorf("create identity: %w", err)
	}
	return s.convert(row), nil
}

// Get fetches a single identity by ID.
func (s *IdentityService) Get(ctx context.Context, id int64) (identities.Identity, error) {
	row, err := s.db.Queries.GetIdentity(ctx, id)
	if err != nil {
		return identities.Identity{}, fmt.Errorf("get identity %d: %w", id, err)
	}
	return s.convert(row), nil
}

// GetByEmailGlobal fetches an identity by email across all tenants — used
// at login, where the user hasn't identified a tenant yet.
func (s *IdentityService) GetByEmailGlobal(ctx context.Context, email string) (identities.Identity, error) {
	row, err := s.db.Queries.GetIdentityByEmailGlobal(ctx, email)
	if err != nil {
		return identities.Identity{}, fmt.Errorf("get identity by email %q: %w", email, err)
	}
	return s.convert(row), nil
}

// List returns identities for tenantID, paginated.
func (s *IdentityService) List(ctx context.Context, tenantID int64, limit, offset int64) ([]identities.Identity, error) {
	rows, err := s.db.Queries.ListIdentitiesByTenant(ctx, db.ListIdentitiesByTenantParams{
		TenantID: tenantID, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	out := make([]identities.Identity, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.convert(row))
	}
	return out, nil
}

// Search finds identities matching a free-text query within tenantID.
func (s *IdentityService) Search(ctx context.Context, tenantID int64, query string, limit int64) ([]identities.Identity, error) {
	rows, err := s.db.Queries.SearchIdentities(ctx, db.SearchIdentitiesParams{
		TenantID: tenantID, Search: query, Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("search identities: %w", err)
	}
	out := make([]identities.Identity, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.convert(row))
	}
	return out, nil
}

// Update writes back the editable fields of an existing identity.
func (s *IdentityService) Update(ctx context.Context, in identities.Identity) (identities.Identity, error) {
	row, err := s.db.Queries.UpdateIdentity(ctx, db.UpdateIdentityParams{
		ID:           in.ID,
		DisplayName:  in.DisplayName,
		GivenName:    nullString(in.GivenName),
		Surname:      nullString(in.Surname),
		Email:        in.Email,
		Title:        nullString(in.Title),
		Department:   nullString(in.Department),
		Company:      nullString(in.Company),
		EmployeeID:   nullString(in.EmployeeID),
		EmployeeType: nullString(in.EmployeeType),
		ManagerID:    nullInt64(in.ManagerID),
		PhoneOffice:  nullString(in.PhoneOffice),
		PhoneMobile:  nullString(in.PhoneMobile),
		SiteID:       nullInt64(in.SiteID),
	})
	if err != nil {
		return identities.Identity{}, fmt.Errorf("update identity %d: %w", in.ID, err)
	}
	return s.convert(row), nil
}

// Delete permanently removes an identity.
func (s *IdentityService) Delete(ctx context.Context, id int64) error {
	if err := s.db.Queries.DeleteIdentity(ctx, id); err != nil {
		return fmt.Errorf("delete identity %d: %w", id, err)
	}
	return nil
}

// convert adapts a db.Identity row into the canonical identities.Identity shape.
func (s *IdentityService) convert(row db.Identity) identities.Identity {
	out := identities.Identity{
		ID:                row.ID,
		TenantID:          row.TenantID,
		Username:          row.Username,
		UserPrincipalName: row.UserPrincipalName.String,
		Email:             row.Email,
		DisplayName:       row.DisplayName,
		GivenName:         row.GivenName.String,
		Surname:           row.Surname.String,
		Initials:          row.Initials.String,
		Title:             row.Title.String,
		Department:        row.Department.String,
		Company:           row.Company.String,
		EmployeeID:        row.EmployeeID.String,
		EmployeeType:      row.EmployeeType.String,
		PhoneOffice:       row.PhoneOffice.String,
		PhoneMobile:       row.PhoneMobile.String,
		PhoneHome:         row.PhoneHome.String,
		Fax:               row.Fax.String,
		Office:            row.Office.String,
		StreetAddress:     row.StreetAddress.String,
		City:              row.City.String,
		State:             row.State.String,
		PostalCode:        row.PostalCode.String,
		Country:           row.Country.String,
		CountryCode:       row.CountryCode.String,
		HomeDirectory:     row.HomeDirectory.String,
		HomeDrive:         row.HomeDrive.String,
		ProfilePath:       row.ProfilePath.String,
		LogonScript:       row.LogonScript.String,
		AccountEnabled:    row.AccountEnabled,
		AccountLocked:     row.AccountLocked,
		PasswordNeverExpires: row.PasswordNeverExpires,
		MustChangePassword:   row.MustChangePassword,
		LogonCount:        row.LogonCount,
		BadPasswordCount:  row.BadPasswordCount,
		Role:              identities.Role(row.Role),
		AvatarURL:         row.AvatarUrl.String,
		Locale:            row.Locale,
		Timezone:          row.Timezone,
		Description:       row.Description.String,
		Notes:             row.Notes.String,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
	if row.SiteID.Valid {
		out.SiteID = row.SiteID.Int64
	}
	if row.ManagerID.Valid {
		out.ManagerID = row.ManagerID.Int64
	}
	if row.AccountExpiresAt.Valid {
		t := row.AccountExpiresAt.Time
		out.AccountExpiresAt = &t
	}
	if row.PasswordLastSetAt.Valid {
		t := row.PasswordLastSetAt.Time
		out.PasswordLastSetAt = &t
	}
	if row.LastLogonAt.Valid {
		t := row.LastLogonAt.Time
		out.LastLogonAt = &t
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/services/... -run TestIdentityService -v`
Expected: PASS. If a generated field name (e.g. `row.AvatarUrl` vs
`row.AvatarURL`) doesn't match, fix the reference to match what
`sqlc generate` actually produced in `db/models.go` (check with
`grep -n "AvatarUrl\|AvatarURL" db/models.go`).

- [ ] **Step 5: Run the full suite**

Run: `go build -buildvcs=false ./... && go test -buildvcs=false ./...`
Expected: builds clean, all tests pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/services/identities.go pkg/services/identities_test.go
git commit -m "feat: add IdentityService (create/get/list/search/update/delete)"
```

---

### Task 5: Asset ↔ owner pairing

**Files:**
- Modify: `db/queries/assets.sql` (add one narrow query)
- Modify: `pkg/services/assets.go` (add `SetOwner`)
- Modify: `pkg/services/identities.go` (add `ListAssignedAssets`)
- Test: `pkg/services/owner_pairing_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
// pkg/services/owner_pairing_test.go
package services

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
)

func TestAssetOwnerPairing(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	identitySvc := NewIdentityService(database)
	assetSvc := NewAssetService(database)
	ctx := context.Background()

	owner, err := identitySvc.Create(ctx, tenantID, identities.Identity{
		Username: "owner1", Email: "owner1@example.com",
		DisplayName: "Owner One", Role: identities.RoleUserSelfService,
	})
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}

	asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "aaaaaaaa-e29b-41d4-a716-446655440000",
		TenantID:        tenantID,
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"pair-test"}`,
		EnrollmentState: "pending",
	})
	if err != nil {
		t.Fatalf("failed to create asset: %v", err)
	}

	if err := assetSvc.SetOwner(ctx, asset.ID, owner.ID); err != nil {
		t.Fatalf("SetOwner failed: %v", err)
	}

	var ownerID sql.NullInt64
	err = database.Conn().QueryRow(`SELECT owner_identity_id FROM assets WHERE id = ?`, asset.ID).Scan(&ownerID)
	if err != nil {
		t.Fatalf("failed to read owner_identity_id: %v", err)
	}
	if !ownerID.Valid || ownerID.Int64 != owner.ID {
		t.Fatalf("expected owner_identity_id %d, got %+v", owner.ID, ownerID)
	}

	assigned, err := identitySvc.ListAssignedAssets(ctx, tenantID, owner.ID)
	if err != nil {
		t.Fatalf("ListAssignedAssets failed: %v", err)
	}
	if len(assigned) != 1 || assigned[0].ID != asset.ID {
		t.Fatalf("expected 1 assigned asset with id %d, got %+v", asset.ID, assigned)
	}

	if err := assetSvc.ClearOwner(ctx, asset.ID); err != nil {
		t.Fatalf("ClearOwner failed: %v", err)
	}
	assignedAfter, err := identitySvc.ListAssignedAssets(ctx, tenantID, owner.ID)
	if err != nil {
		t.Fatalf("ListAssignedAssets after clear failed: %v", err)
	}
	if len(assignedAfter) != 0 {
		t.Fatalf("expected 0 assigned assets after clear, got %d", len(assignedAfter))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/services/... -run TestAssetOwnerPairing -v`
Expected: FAIL to compile — `SetOwner`/`ClearOwner`/`ListAssignedAssets`
don't exist yet.

- [ ] **Step 3: Add the narrow owner-update query**

Append to `db/queries/assets.sql`:

```sql
-- name: SetAssetOwner :exec
-- Narrow, single-column update — unlike UpdateAsset (which requires
-- re-supplying site_id/subtype_payload/labels), this only ever touches
-- ownership so callers can't accidentally clobber unrelated fields.
UPDATE assets SET owner_identity_id = @owner_id, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: ClearAssetOwner :exec
UPDATE assets SET owner_identity_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = @id;
```

- [ ] **Step 4: Regenerate sqlc code**

Run: `~/go/bin/sqlc generate`

- [ ] **Step 5: Add `SetOwner`/`ClearOwner` to `AssetService`**

In `pkg/services/assets.go`, add (needs `"database/sql"` already imported):

```go
// SetOwner assigns ownerID as the owning identity of assetID.
func (s *AssetService) SetOwner(ctx context.Context, assetID, ownerID int64) error {
	if err := s.db.Queries.SetAssetOwner(ctx, db.SetAssetOwnerParams{
		ID:      assetID,
		OwnerID: sql.NullInt64{Int64: ownerID, Valid: true},
	}); err != nil {
		return fmt.Errorf("set owner on asset %d: %w", assetID, err)
	}
	return nil
}

// ClearOwner removes any owning identity from assetID.
func (s *AssetService) ClearOwner(ctx context.Context, assetID int64) error {
	if err := s.db.Queries.ClearAssetOwner(ctx, assetID); err != nil {
		return fmt.Errorf("clear owner on asset %d: %w", assetID, err)
	}
	return nil
}
```

- [ ] **Step 6: Add `ListAssignedAssets` to `IdentityService`**

In `pkg/services/identities.go`, add (needs `"github.com/pluris/pluris/catalog/assets"` imported):

```go
// ListAssignedAssets returns every asset owned by identityID within tenantID.
func (s *IdentityService) ListAssignedAssets(ctx context.Context, tenantID, identityID int64) ([]assets.Asset, error) {
	rows, err := s.db.Queries.ListAssetsByOwner(ctx, db.ListAssetsByOwnerParams{
		TenantID: tenantID,
		OwnerID:  sql.NullInt64{Int64: identityID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list assets owned by identity %d: %w", identityID, err)
	}
	out := make([]assets.Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, assetFromDBRow(row))
	}
	return out, nil
}
```

`assetFromDBRow` doesn't exist yet — check what `AssetService` currently
calls its `db.Asset` → `assets.Asset` converter (it's
`convertToAsset(dbAsset db.Asset) assets.Asset`, a method on
`*AssetService`, in `pkg/services/assets.go:213`). Since
`IdentityService` doesn't have an `AssetService` instance, add a small
**package-level** function next to it instead of calling the method:

- [ ] **Step 7: Extract a package-level asset converter**

In `pkg/services/assets.go`, find `func (s *AssetService) convertToAsset(dbAsset db.Asset) assets.Asset {` and change it to a package-level function:

```go
func convertDBAssetToAsset(dbAsset db.Asset) assets.Asset {
```

Then update its single call site (search `s.convertToAsset(` in the same
file) to call `convertDBAssetToAsset(...)` instead. Finally, in
`pkg/services/identities.go`, replace `assetFromDBRow(row)` from Step 6
with `convertDBAssetToAsset(row)` — but check first whether
`ListAssetsByOwner`'s generated return type is `db.Asset` directly or a
`db.ListAssetsByOwnerRow` (it may differ if the query joins other
tables); if it's a distinct row type, write a 2-line adapter next to
`convertDBAssetToAsset` that copies the row's fields into a `db.Asset`
before delegating, following the same pattern already used for
`convertRowToAsset`/`convertHumanIDRowToAsset` in that file.

- [ ] **Step 8: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/services/... -run TestAssetOwnerPairing -v`
Expected: PASS

- [ ] **Step 9: Run the full suite**

Run: `go build -buildvcs=false ./... && go test -buildvcs=false ./...`
Expected: all green.

- [ ] **Step 10: Commit**

```bash
git add db/queries/assets.sql db/*.sql.go pkg/services/assets.go pkg/services/identities.go pkg/services/owner_pairing_test.go
git commit -m "feat: wire asset-owner pairing (SetOwner/ClearOwner/ListAssignedAssets)"
```

---

### Task 6: Password hashing (`pkg/auth`)

**Files:**
- Create: `pkg/auth/password.go`
- Test: `pkg/auth/password_test.go`
- Modify: `go.mod` (promote `golang.org/x/crypto` to direct)

- [ ] **Step 1: Write the failing test**

```go
// pkg/auth/password_test.go
package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword (wrong password) failed: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to not verify")
	}
}

func TestHashPasswordProducesDifferentSaltsEachTime(t *testing.T) {
	h1, _ := HashPassword("same password")
	h2, _ := HashPassword("same password")
	if h1 == h2 {
		t.Fatal("expected different hashes for the same password (random salt)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/auth/... -v`
Expected: FAIL to compile — package `auth` doesn't exist yet.

- [ ] **Step 3: Write `pkg/auth/password.go`**

```go
// Package auth implements Pluris's local authentication: password
// hashing, server-side sessions, and RBAC enforcement. This is the
// app's first authentication system (see
// docs/superpowers/specs/2026-07-04-users-identity-login-design.md) —
// no external identity provider (Kanidm/AD/FreeIPA) is wired yet; that's
// explicitly Phase 2 per ADR-009.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. These match the OWASP-recommended minimums for
// interactive login (not a high-throughput API): 19 MiB memory, 2
// iterations, 1 degree of parallelism, 32-byte output. Encoded into the
// hash string itself so parameters can change later without breaking
// verification of existing hashes.
const (
	argonMemoryKiB  = 19 * 1024
	argonIterations = 2
	argonThreads    = 1
	argonKeyLen     = 32
	argonSaltLen    = 16
)

// HashPassword returns a self-describing Argon2id hash string in the form
// "$argon2id$v=19$m=19456,t=2,p=1$<salt-b64>$<hash-b64>".
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonThreads, argonKeyLen)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonIterations, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword checks password against an encoded hash produced by
// HashPassword. Uses constant-time comparison to avoid timing side
// channels.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("invalid version segment: %w", err)
	}

	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, fmt.Errorf("invalid params segment: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("invalid salt encoding: %w", err)
	}
	wantHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("invalid hash encoding: %w", err)
	}

	gotHash := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(wantHash)))

	return subtle.ConstantTimeCompare(gotHash, wantHash) == 1, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/auth/... -v`
Expected: PASS

- [ ] **Step 5: Promote `golang.org/x/crypto` to a direct dependency**

Run: `go mod tidy`
Expected: `go.mod` moves `golang.org/x/crypto` out of the `// indirect`
block (now imported directly by `pkg/auth`).

- [ ] **Step 6: Commit**

```bash
git add pkg/auth/password.go pkg/auth/password_test.go go.mod go.sum
git commit -m "feat: add Argon2id password hashing"
```

---

### Task 7: Session management (`pkg/auth`)

**Files:**
- Create: `pkg/auth/session.go`
- Test: `pkg/auth/session_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/auth/session_test.go
package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func setupAuthTestDB(t *testing.T) (*database.Database, db.Identity) {
	t.Helper()
	dbPath := "test_auth_session.db"
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
		Name: "Test Org", Slug: "test-org-auth",
	})
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	identity, err := database.Queries.CreateIdentity(context.Background(), db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "jdoe", Email: "jdoe@example.com",
		DisplayName: "Jane Doe", Role: "admin",
	})
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}
	return database, identity
}

func TestCreateAndLookupSession(t *testing.T) {
	dbase, identity := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	ctx := context.Background()

	rawToken, _, err := mgr.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if rawToken == "" {
		t.Fatal("expected non-empty raw token")
	}

	sess, err := mgr.Lookup(ctx, rawToken)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if sess.IdentityID != identity.ID {
		t.Fatalf("expected identity id %d, got %d", identity.ID, sess.IdentityID)
	}
}

func TestLookupRejectsRevokedSession(t *testing.T) {
	dbase, identity := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	ctx := context.Background()

	rawToken, _, err := mgr.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := mgr.Revoke(ctx, rawToken); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	if _, err := mgr.Lookup(ctx, rawToken); err == nil {
		t.Fatal("expected Lookup to fail for a revoked session")
	}
}

func TestLookupRejectsUnknownToken(t *testing.T) {
	dbase, _ := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	if _, err := mgr.Lookup(context.Background(), "not-a-real-token"); err == nil {
		t.Fatal("expected Lookup to fail for an unknown token")
	}
}

func TestSessionExpiryIsThirtyDaysOut(t *testing.T) {
	dbase, identity := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	ctx := context.Background()

	_, expiresAt, err := mgr.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	wantMin := time.Now().Add(29 * 24 * time.Hour)
	wantMax := time.Now().Add(31 * 24 * time.Hour)
	if expiresAt.Before(wantMin) || expiresAt.After(wantMax) {
		t.Fatalf("expected expiry ~30 days out, got %v", expiresAt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/auth/... -run TestCreateAndLookupSession -v`
Expected: FAIL to compile — `NewSessionManager` doesn't exist.

- [ ] **Step 3: Write `pkg/auth/session.go`**

```go
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// SessionTTL is fixed (not sliding) per the approved design — every
// login issues a fresh 30-day session rather than extending an old one.
const SessionTTL = 30 * 24 * time.Hour

// Session is the resolved, in-memory shape of an identity_sessions row.
type Session struct {
	ID             int64
	IdentityID     int64
	ActiveTenantID int64 // 0 when unset
	ExpiresAt      time.Time
}

// SessionManager creates, looks up, and revokes server-side sessions.
type SessionManager struct {
	db *database.Database
}

func NewSessionManager(db *database.Database) *SessionManager {
	return &SessionManager{db: db}
}

// Create issues a new session for identityID and returns the raw token
// (to be set as the cookie value — never persisted) and its expiry.
func (m *SessionManager) Create(ctx context.Context, identityID int64, ip, userAgent string) (rawToken string, expiresAt time.Time, err error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	rawToken = hex.EncodeToString(tokenBytes)
	expiresAt = time.Now().Add(SessionTTL)

	_, err = m.db.Queries.CreateIdentitySession(ctx, db.CreateIdentitySessionParams{
		IdentityID: identityID,
		TokenHash:  hashToken(rawToken),
		IpAddress:  sql.NullString{String: ip, Valid: ip != ""},
		UserAgent:  sql.NullString{String: userAgent, Valid: userAgent != ""},
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return rawToken, expiresAt, nil
}

// Lookup resolves rawToken to an active (non-expired, non-revoked) session.
func (m *SessionManager) Lookup(ctx context.Context, rawToken string) (Session, error) {
	row, err := m.db.Queries.GetActiveSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return Session{}, fmt.Errorf("lookup session: %w", err)
	}
	sess := Session{
		ID:         row.ID,
		IdentityID: row.IdentityID,
		ExpiresAt:  row.ExpiresAt,
	}
	if row.ActiveTenantID.Valid {
		sess.ActiveTenantID = row.ActiveTenantID.Int64
	}
	return sess, nil
}

// SetActiveTenant records the super_admin's currently-switched tenant on
// an existing session (see pkg/auth's RBAC and the tenant-switch handler).
func (m *SessionManager) SetActiveTenant(ctx context.Context, sessionID, tenantID int64) error {
	if err := m.db.Queries.SetSessionActiveTenant(ctx, db.SetSessionActiveTenantParams{
		ID:             sessionID,
		ActiveTenantID: sql.NullInt64{Int64: tenantID, Valid: true},
	}); err != nil {
		return fmt.Errorf("set active tenant: %w", err)
	}
	return nil
}

// Revoke invalidates rawToken immediately (used by logout).
func (m *SessionManager) Revoke(ctx context.Context, rawToken string) error {
	if err := m.db.Queries.RevokeSession(ctx, hashToken(rawToken)); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// hashToken returns the hex-encoded SHA-256 of a raw session token. Only
// this hash is ever persisted — the raw token exists solely in the
// cookie, so a stolen database dump cannot be replayed as a live session.
func hashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/auth/... -v`
Expected: PASS. If `db.CreateIdentitySessionParams` field names differ
(e.g. `IpAddress` vs `IPAddress`), check `db/sessions.sql.go` and adjust.

- [ ] **Step 5: Run the full suite**

Run: `go build -buildvcs=false ./... && go test -buildvcs=false ./...`

- [ ] **Step 6: Commit**

```bash
git add pkg/auth/session.go pkg/auth/session_test.go
git commit -m "feat: add server-side session management (create/lookup/revoke)"
```

---

### Task 8: RBAC permission matrix

**Files:**
- Create: `pkg/auth/rbac.go`
- Test: `pkg/auth/rbac_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/auth/rbac_test.go
package auth

import "testing"

func TestCanAccessMatchesLockedMatrix(t *testing.T) {
	cases := []struct {
		role   string
		prefix string
		want   bool
	}{
		{"super_admin", "/server-admin", true},
		{"admin", "/server-admin", true},
		{"user_self_service", "/server-admin", false},
		{"user_self_service", "/users", true}, // read-only own identity, still "can access"
		{"user_self_service", "/scripts", false},
		{"admin", "/scripts", true},
		{"user_self_service", "/policy/modules", false},
	}
	for _, c := range cases {
		got := CanAccess(c.role, c.prefix)
		if got != c.want {
			t.Errorf("CanAccess(%q, %q) = %v, want %v", c.role, c.prefix, got, c.want)
		}
	}
}

func TestCanAccessUnknownRoleDeniedByDefault(t *testing.T) {
	if CanAccess("bogus_role", "/") {
		t.Fatal("expected unknown role to be denied by default")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/auth/... -run TestCanAccess -v`
Expected: FAIL to compile — `CanAccess` doesn't exist.

- [ ] **Step 3: Write `pkg/auth/rbac.go`**

```go
package auth

import "strings"

// permission mirrors the locked role permission matrix in
// docs/UX_INVARIANTS.md (§ "Role permission matrix (v1, locked)"). It is
// intentionally coarse (route-prefix level): row-level self-scoping
// (e.g. user_self_service seeing only their own identity/assets) is
// applied inside the service layer using the session identity, not here.
var permission = map[string]map[string]bool{
	"/":                 {"super_admin": true, "admin": true, "user_self_service": true},
	"/users":            {"super_admin": true, "admin": true, "user_self_service": true},
	"/assets":           {"super_admin": true, "admin": true, "user_self_service": true},
	"/policy":           {"super_admin": true, "admin": true, "user_self_service": true},
	"/profiles":         {"super_admin": true, "admin": true, "user_self_service": true},
	"/scripts":          {"super_admin": true, "admin": true, "user_self_service": false},
	"/policy/modules":   {"super_admin": true, "admin": true, "user_self_service": false},
	"/wine":             {"super_admin": true, "admin": true, "user_self_service": true},
	"/packages":         {"super_admin": true, "admin": true, "user_self_service": true},
	"/server-admin":     {"super_admin": true, "admin": true, "user_self_service": false},
	"/preferences":      {"super_admin": true, "admin": true, "user_self_service": true},
	"/tenant-switch":    {"super_admin": true, "admin": false, "user_self_service": false},
}

// CanAccess reports whether role may access the route beginning with
// path. It matches the longest registered prefix (so "/policy/modules"
// overrides the broader "/policy" entry for admin/user_self_service).
func CanAccess(role, path string) bool {
	bestPrefix := ""
	for prefix := range permission {
		if strings.HasPrefix(path, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
		}
	}
	if bestPrefix == "" {
		return false
	}
	allowed, ok := permission[bestPrefix][role]
	return ok && allowed
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/auth/... -run TestCanAccess -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/auth/rbac.go pkg/auth/rbac_test.go
git commit -m "feat: add RBAC permission matrix matching the locked UX_INVARIANTS role table"
```

---

### Task 9: Session context propagation

**Files:**
- Create: `pkg/auth/context.go`
- Test: `pkg/auth/context_test.go`

This is what lets `Header`/`Layout` (called from every page) show the real
logged-in user and tenant switcher without changing the signature of
every existing `@Layout(active, title)` call site: templ passes the
request's `context.Context` into every component's generated `Render`
method, and a plain Go expression inside a `.templ` file can read `ctx`
directly.

- [ ] **Step 1: Write the failing test**

```go
// pkg/auth/context_test.go
package auth

import (
	"context"
	"testing"
)

func TestSessionRoundTripsThroughContext(t *testing.T) {
	sess := &UserSession{
		IdentityID:  7,
		Email:       "jdoe@example.com",
		DisplayName: "Jane Doe",
		Role:        "admin",
		TenantID:    3,
	}
	ctx := WithSession(context.Background(), sess)
	got := FromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil session from context")
	}
	if got.Email != "jdoe@example.com" {
		t.Fatalf("expected email jdoe@example.com, got %s", got.Email)
	}
}

func TestFromContextReturnsNilWhenAbsent(t *testing.T) {
	if got := FromContext(context.Background()); got != nil {
		t.Fatalf("expected nil session, got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/auth/... -run TestSessionRoundTrips -v`
Expected: FAIL to compile — `UserSession`/`WithSession`/`FromContext`
don't exist.

- [ ] **Step 3: Write `pkg/auth/context.go`**

```go
package auth

import "context"

// TenantOption is one entry in a super_admin's tenant switcher.
type TenantOption struct {
	ID   int64
	Name string
}

// UserSession is the resolved, request-scoped view of who is logged in.
// Populated once per request by the auth middleware and read by
// handlers (for RBAC / tenant scoping) and by templ components (for the
// nav user-menu and tenant switcher) via FromContext.
type UserSession struct {
	IdentityID  int64
	Email       string
	DisplayName string
	Role        string // "super_admin" | "admin" | "user_self_service"

	// TenantID is the EFFECTIVE tenant for this request: the super_admin's
	// switched ActiveTenantID if set, else the identity's own home tenant.
	TenantID int64

	// AvailableTenants is populated only when Role == "super_admin"; every
	// other role never sees a switcher.
	AvailableTenants []TenantOption
}

type contextKey int

const sessionContextKey contextKey = iota

// WithSession returns a new context carrying sess.
func WithSession(ctx context.Context, sess *UserSession) context.Context {
	return context.WithValue(ctx, sessionContextKey, sess)
}

// FromContext returns the session stashed by WithSession, or nil if none
// is present (e.g. before the auth middleware has run, or on /login and
// /setup which never carry one).
func FromContext(ctx context.Context) *UserSession {
	sess, _ := ctx.Value(sessionContextKey).(*UserSession)
	return sess
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/auth/... -v`
Expected: PASS (all `pkg/auth` tests so far).

- [ ] **Step 5: Commit**

```bash
git add pkg/auth/context.go pkg/auth/context_test.go
git commit -m "feat: add request-scoped session context for templ components"
```

---

### Task 10: Echo middleware (setup-gate, auth, RBAC)

**Files:**
- Create: `pkg/auth/middleware.go`
- Test: `pkg/auth/middleware_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/auth/middleware_test.go
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func setupMiddlewareTestDB(t *testing.T) *database.Database {
	t.Helper()
	dbPath := "test_auth_middleware.db"
	t.Cleanup(func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-shm")
		os.Remove(dbPath + "-wal")
	})
	dbase, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { dbase.Close() })
	return dbase
}

func TestSetupGateRedirectsWhenNoIdentitiesExist(t *testing.T) {
	dbase := setupMiddlewareTestDB(t)
	e := echo.New()
	e.Use(SetupGate(dbase))
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "dashboard") })
	e.GET("/setup", func(c echo.Context) error { return c.String(http.StatusOK, "setup") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect to /setup, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("expected redirect to /setup, got %q", loc)
	}
}

func TestSetupGateAllowsThroughOnceAnIdentityExists(t *testing.T) {
	dbase := setupMiddlewareTestDB(t)
	ctx := context.Background()
	tenant, _ := dbase.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Org", Slug: "org-mw"})
	dbase.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "a", Email: "a@example.com",
		DisplayName: "A", Role: "super_admin",
	})

	e := echo.New()
	e.Use(SetupGate(dbase))
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "dashboard") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireAuthRedirectsToLoginWithoutSession(t *testing.T) {
	dbase := setupMiddlewareTestDB(t)
	sessions := NewSessionManager(dbase)

	e := echo.New()
	e.Use(RequireAuth(dbase, sessions))
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "dashboard") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect to /login, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestRBACDeniesUserSelfServiceFromScripts(t *testing.T) {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.SetRequest(c.Request().WithContext(WithSession(c.Request().Context(), &UserSession{
				IdentityID: 1, Role: "user_self_service",
			})))
			return next(c)
		}
	})
	e.Use(RequireRole())
	e.GET("/scripts", func(c echo.Context) error { return c.String(http.StatusOK, "scripts") })

	req := httptest.NewRequest(http.MethodGet, "/scripts", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/auth/... -run TestSetupGate -v`
Expected: FAIL to compile — `SetupGate`/`RequireAuth`/`RequireRole` don't exist.

- [ ] **Step 3: Write `pkg/auth/middleware.go`**

```go
package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pluris/pluris/pkg/database"
)

const sessionCookieName = "pluris_session"

// setupExemptPaths are reachable even with zero identities in the DB.
var setupExemptPaths = map[string]bool{
	"/setup":     true,
	"/healthz":   true,
}

// authExemptPaths are reachable without a valid session.
var authExemptPaths = map[string]bool{
	"/login":   true,
	"/setup":   true,
	"/healthz": true,
}

func isStaticPath(path string) bool {
	return len(path) >= 8 && path[:8] == "/static/"
}

// SetupGate redirects every request to /setup until at least one
// identity exists anywhere in the database. No default/shared account is
// ever seeded — this is how the very first super_admin gets created.
func SetupGate(dbase *database.Database) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if setupExemptPaths[path] || isStaticPath(path) {
				return next(c)
			}
			count, err := dbase.Queries.CountIdentitiesGlobal(c.Request().Context())
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to check setup state")
			}
			if count == 0 {
				return c.Redirect(http.StatusFound, "/setup")
			}
			return next(c)
		}
	}
}

// RequireAuth resolves the session cookie into a UserSession and stashes
// it in the request context (see WithSession/FromContext). Requests
// without a valid session are redirected to /login, except for the
// exempt paths above.
func RequireAuth(dbase *database.Database, sessions *SessionManager) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if authExemptPaths[path] || isStaticPath(path) {
				return next(c)
			}

			cookie, err := c.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				return c.Redirect(http.StatusFound, "/login")
			}

			sess, err := sessions.Lookup(c.Request().Context(), cookie.Value)
			if err != nil {
				return c.Redirect(http.StatusFound, "/login")
			}

			identity, err := dbase.Queries.GetIdentity(c.Request().Context(), sess.IdentityID)
			if err != nil {
				return c.Redirect(http.StatusFound, "/login")
			}

			effectiveTenant := identity.TenantID
			if sess.ActiveTenantID != 0 {
				effectiveTenant = sess.ActiveTenantID
			}

			userSess := &UserSession{
				IdentityID:  identity.ID,
				Email:       identity.Email,
				DisplayName: identity.DisplayName,
				Role:        identity.Role,
				TenantID:    effectiveTenant,
			}

			if identity.Role == "super_admin" {
				tenants, err := dbase.Queries.ListTenants(c.Request().Context())
				if err == nil {
					opts := make([]TenantOption, 0, len(tenants))
					for _, t := range tenants {
						opts = append(opts, TenantOption{ID: t.ID, Name: t.Name})
					}
					userSess.AvailableTenants = opts
				}
			}

			c.SetRequest(c.Request().WithContext(WithSession(c.Request().Context(), userSess)))
			return next(c)
		}
	}
}

// RequireRole enforces the RBAC matrix (pkg/auth.CanAccess) against the
// session stashed by RequireAuth. Must run after RequireAuth in the
// middleware chain. Renders a plain 403 — page-level styling of this
// response is a follow-up, not required for the auth system to be
// correct.
func RequireRole() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if authExemptPaths[path] || isStaticPath(path) {
				return next(c)
			}
			sess := FromContext(c.Request().Context())
			if sess == nil {
				return echo.NewHTTPError(http.StatusForbidden, "no active session")
			}
			if !CanAccess(sess.Role, path) {
				return echo.NewHTTPError(http.StatusForbidden, "not permitted for your role")
			}
			return next(c)
		}
	}
}
```

`ListTenants` may not exist yet as a no-arg query — check
`grep -n "ListTenants" db/querier.go`. If it's missing, add to
`db/queries/tenants.sql`:

```sql
-- name: ListTenants :many
SELECT * FROM tenants ORDER BY name;
```

and run `~/go/bin/sqlc generate` before continuing.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/auth/... -v`
Expected: PASS for all `pkg/auth` tests.

- [ ] **Step 5: Run the full suite**

Run: `go build -buildvcs=false ./... && go test -buildvcs=false ./...`

- [ ] **Step 6: Commit**

```bash
git add pkg/auth/middleware.go pkg/auth/middleware_test.go db/queries/tenants.sql db/*.sql.go
git commit -m "feat: add setup-gate, auth, and RBAC Echo middleware"
```

---

### Task 11: Minimal auth layout + Setup wizard

**Files:**
- Create: `web/templates/auth.templ`
- Create: `console/handlers/auth.go`
- Modify: `console/server/server.go` (routes only — full middleware wiring is Task 13)

- [ ] **Step 1: Write `web/templates/auth.templ`**

```templ
package templates

// AuthLayout is a minimal centered-card shell for /login and /setup —
// intentionally NOT the full Layout (no sidebar/nav), since both routes
// are reachable before any session exists.
templ AuthLayout(title string) {
	<!DOCTYPE html>
	<html lang="en">
		<head>
			<meta charset="UTF-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
			<title>{ title } — Pluris</title>
			<style>
				body {
					font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
					background: #0f1115; color: #e6e8eb;
					display: flex; align-items: center; justify-content: center;
					min-height: 100vh; margin: 0;
				}
				.auth-card {
					background: #171a21; border: 1px solid #2a2e38; border-radius: 10px;
					padding: 32px; width: 360px;
				}
				.auth-card h1 { font-size: 18px; margin: 0 0 20px; }
				.auth-card label { display: block; font-size: 13px; margin: 14px 0 4px; color: #b7bcc7; }
				.auth-card input {
					width: 100%; box-sizing: border-box; padding: 8px 10px;
					background: #0f1115; border: 1px solid #2a2e38; border-radius: 6px;
					color: #e6e8eb; font-size: 14px;
				}
				.auth-card button {
					margin-top: 20px; width: 100%; padding: 10px; border: none;
					border-radius: 6px; background: #4f7cff; color: white;
					font-size: 14px; font-weight: 600; cursor: pointer;
				}
				.auth-error {
					margin-top: 12px; padding: 8px 10px; border-radius: 6px;
					background: #3a1d21; color: #ff9aa6; font-size: 13px;
				}
			</style>
		</head>
		<body>
			<div class="auth-card">
				{ children... }
			</div>
		</body>
	</html>
}

templ SetupPage(csrfToken string, errorMsg string) {
	@AuthLayout("Set up Pluris") {
		<h1>Create your admin account</h1>
		<form method="POST" action="/setup" data-testid="setup-form">
			<input type="hidden" name="_csrf" value={ csrfToken }/>
			<label for="org_name">Organization name</label>
			<input type="text" id="org_name" name="org_name" required/>
			<label for="display_name">Your name</label>
			<input type="text" id="display_name" name="display_name" required/>
			<label for="email">Email</label>
			<input type="email" id="email" name="email" required/>
			<label for="password">Password</label>
			<input type="password" id="password" name="password" minlength="8" required/>
			if errorMsg != "" {
				<div class="auth-error">{ errorMsg }</div>
			}
			<button type="submit">Create account</button>
		</form>
	}
}

templ LoginPage(csrfToken string, errorMsg string) {
	@AuthLayout("Log in") {
		<h1>Pluris</h1>
		<form method="POST" action="/login" data-testid="login-form">
			<input type="hidden" name="_csrf" value={ csrfToken }/>
			<label for="email">Email</label>
			<input type="email" id="email" name="email" required autofocus/>
			<label for="password">Password</label>
			<input type="password" id="password" name="password" required/>
			if errorMsg != "" {
				<div class="auth-error">{ errorMsg }</div>
			}
			<button type="submit">Log in</button>
		</form>
	}
}
```

- [ ] **Step 2: Generate templ code**

Run: `~/go/bin/templ generate`
Expected: `web/templates/auth_templ.go` is created.

- [ ] **Step 3: Write `console/handlers/auth.go`**

```go
// Package handlers — auth.go implements the first-run setup wizard and
// login/logout. See docs/superpowers/specs/2026-07-04-users-identity-login-design.md.
package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/web/templates"
)

// SetupShow renders the first-run wizard.
func (h *Handler) SetupShow(c echo.Context) error {
	return render(c, templates.SetupPage(csrfTokenFrom(c), ""))
}

// SetupSubmit creates the first tenant + first super_admin identity, then
// redirects to /login. Reachable only while SetupGate allows it through
// (zero identities in the DB).
func (h *Handler) SetupSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	orgName := c.FormValue("org_name")
	displayName := c.FormValue("display_name")
	email := c.FormValue("email")
	password := c.FormValue("password")

	if orgName == "" || displayName == "" || email == "" || len(password) < 8 {
		return render(c, templates.SetupPage(csrfTokenFrom(c), "All fields are required; password must be at least 8 characters."))
	}

	tenant, err := h.db.Queries.CreateTenant(ctx, dbCreateTenantParams(orgName))
	if err != nil {
		return render(c, templates.SetupPage(csrfTokenFrom(c), "Could not create organization: "+err.Error()))
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return render(c, templates.SetupPage(csrfTokenFrom(c), "Could not process password."))
	}

	username := email
	if at := indexOfAt(email); at > 0 {
		username = email[:at]
	}

	created, err := h.identitySvc.Create(ctx, tenant.ID, identities.Identity{
		Username:    username,
		Email:       email,
		DisplayName: displayName,
		Role:        identities.RoleSuperAdmin,
	})
	if err != nil {
		return render(c, templates.SetupPage(csrfTokenFrom(c), "Could not create admin account: "+err.Error()))
	}

	if err := h.db.Queries.SetIdentityPasswordHash(ctx, dbSetPasswordHashParams(created.ID, hash)); err != nil {
		return render(c, templates.SetupPage(csrfTokenFrom(c), "Could not save password."))
	}

	return c.Redirect(http.StatusFound, "/login")
}

// LoginShow renders the login form.
func (h *Handler) LoginShow(c echo.Context) error {
	return render(c, templates.LoginPage(csrfTokenFrom(c), ""))
}

// LoginSubmit verifies credentials, locks the account after repeated
// failures, and issues a session cookie on success.
func (h *Handler) LoginSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	email := c.FormValue("email")
	password := c.FormValue("password")

	identity, err := h.identitySvc.GetByEmailGlobal(ctx, email)
	if err != nil {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Invalid email or password."))
	}
	if identity.AccountLocked {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "This account is locked. Contact an admin."))
	}
	if !identity.AccountEnabled {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Invalid email or password."))
	}

	dbIdentity, err := h.db.Queries.GetIdentity(ctx, identity.ID)
	if err != nil || !dbIdentity.PasswordHash.Valid {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Invalid email or password."))
	}

	ok, err := auth.VerifyPassword(password, dbIdentity.PasswordHash.String)
	if err != nil || !ok {
		h.db.Queries.RecordLoginFailure(ctx, identity.ID)
		h.db.Queries.LockIdentityIfThresholdExceeded(ctx, dbLockThresholdParams(identity.ID))
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Invalid email or password."))
	}

	h.db.Queries.RecordLoginSuccess(ctx, identity.ID)

	rawToken, expiresAt, err := h.sessions.Create(ctx, identity.ID, c.RealIP(), c.Request().UserAgent())
	if err != nil {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Could not start a session. Try again."))
	}

	c.SetCookie(&http.Cookie{
		Name:     "pluris_session",
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isRequestTLS(c),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})

	return c.Redirect(http.StatusFound, "/")
}

// LogoutSubmit revokes the current session and clears the cookie.
func (h *Handler) LogoutSubmit(c echo.Context) error {
	if cookie, err := c.Cookie("pluris_session"); err == nil {
		h.sessions.Revoke(c.Request().Context(), cookie.Value)
	}
	c.SetCookie(&http.Cookie{
		Name: "pluris_session", Value: "", Path: "/", MaxAge: -1,
	})
	return c.Redirect(http.StatusFound, "/login")
}

func isRequestTLS(c echo.Context) bool {
	if c.Request().TLS != nil {
		return true
	}
	return c.Request().Header.Get("X-Forwarded-Proto") == "https"
}

func csrfTokenFrom(c echo.Context) string {
	token, _ := c.Get("csrf").(string)
	return token
}

func indexOfAt(s string) int {
	for i, r := range s {
		if r == '@' {
			return i
		}
	}
	return -1
}
```

`dbCreateTenantParams`, `dbSetPasswordHashParams`, and
`dbLockThresholdParams` are placeholder helper names standing in for the
real generated params structs — replace them inline with the actual
`db.CreateTenantParams{Name: orgName, Slug: slugify(orgName)}` (check
`db/tenants.sql.go` for the exact `CreateTenant` signature — it needs
both `Name` and `Slug`; write a tiny `slugify` helper in this file that
lowercases and replaces non-alphanumerics with `-`),
`db.SetIdentityPasswordHashParams{ID: created.ID, PasswordHash: sql.NullString{String: hash, Valid: true}}`,
and `db.LockIdentityIfThresholdExceededParams{ID: identity.ID, Threshold: 10}`
respectively, matching whatever `sqlc generate` actually produced in
Task 2/Task 10.

- [ ] **Step 4: Add the `identitySvc` and `sessions` fields to `Handler`**

In `console/handlers/handlers.go`, change:

```go
type Handler struct{
	db          *database.Database
	assetSvc    *services.AssetService
}

func New(db *database.Database) *Handler { 
	return &Handler{
		db:       db,
		assetSvc: services.NewAssetService(db),
	} 
}
```

to:

```go
type Handler struct {
	db          *database.Database
	assetSvc    *services.AssetService
	identitySvc *services.IdentityService
	sessions    *auth.SessionManager
}

func New(db *database.Database) *Handler {
	return &Handler{
		db:          db,
		assetSvc:    services.NewAssetService(db),
		identitySvc: services.NewIdentityService(db),
		sessions:    auth.NewSessionManager(db),
	}
}
```

Add `"github.com/pluris/pluris/pkg/auth"` to the imports.

- [ ] **Step 5: Add the routes to `console/server/server.go`**

Add, near the top (before the "10 top-level sidebar items" block):

```go
	// Auth (not in sidebar).
	e.GET("/setup", h.SetupShow)
	e.POST("/setup", h.SetupSubmit)
	e.GET("/login", h.LoginShow)
	e.POST("/login", h.LoginSubmit)
	e.POST("/logout", h.LogoutSubmit)
```

- [ ] **Step 6: Build**

Run: `go build -buildvcs=false ./...`
Expected: builds clean once the placeholder param-struct names from Step
3 are replaced with the real generated types.

- [ ] **Step 7: Manual verification**

Run: `go run ./cmd/console` (in one terminal), then in another:

```bash
curl -sI http://localhost:8080/setup | head -1
```

Expected: `HTTP/1.1 200 OK` (the setup gate isn't wired into the
middleware chain until Task 13, so this just confirms the handler
renders). Stop the server (Ctrl-C) before continuing.

- [ ] **Step 8: Commit**

```bash
git add web/templates/auth.templ web/templates/auth_templ.go console/handlers/auth.go console/handlers/handlers.go console/server/server.go
git commit -m "feat: add setup wizard and login/logout handlers + minimal auth layout"
```

---

### Task 12: CSRF middleware + full middleware chain wiring

**Files:**
- Modify: `console/server/server.go`
- Modify: `console/server/server_test.go`

This is the app's first mutating (POST) code path, so this is where CSRF
protection is added once, covering every write route that follows.

- [ ] **Step 1: Write the failing test**

```go
// Add to console/server/server_test.go
func TestSetupGateRedirectsFreshDatabase(t *testing.T) {
	e := New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect to /setup on a fresh database, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("expected redirect to /setup, got %q", loc)
	}
}
```

Check the existing `server_test.go` for how it opens a throwaway database
(likely via `os.Chdir` to a temp dir or a `PLURIS_DB_PATH`-style override)
before adding this — match whatever pattern it already uses so tests
don't collide on `pluris.db`. If `server.New()` hardcodes `"pluris.db"`
with no override, add a `NewWithDB(dbPath string) *echo.Echo` variant (or
an environment variable read) in this task so tests can point at a
throwaway file; keep `New()` calling it with `"pluris.db"` for backward
compatibility with `cmd/console/main.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./console/server/... -run TestSetupGateRedirectsFreshDatabase -v`
Expected: FAIL — currently every route returns 200 with no gating.

- [ ] **Step 3: Wire the middleware chain in `console/server/server.go`**

Change the imports to add:

```go
	"github.com/pluris/pluris/pkg/auth"
```

After `h := handlers.New(db)`, add:

```go
	sessions := auth.NewSessionManager(db)

	e.Use(auth.SetupGate(db))
	e.Use(auth.RequireAuth(db, sessions))
	e.Use(auth.RequireRole())
	e.Use(emw.CSRFWithConfig(emw.CSRFConfig{
		TokenLookup: "form:_csrf",
		Skipper: func(c echo.Context) bool {
			// CSRF only matters for state-changing requests; GETs are safe.
			return c.Request().Method == http.MethodGet
		},
	}))
```

Add `"net/http"` to imports if not already present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./console/server/... -v`
Expected: PASS, including the pre-existing mount-point tests (a fresh
test DB has zero identities, so every mount-point test that expects a
200 on e.g. `/assets/computers` will now also redirect to `/setup` —
**this is expected new behavior**; if those pre-existing tests fail, fix
them by seeding one identity in their test setup, not by weakening the
gate).

- [ ] **Step 5: Update the existing mount-point tests to seed an identity**

Whatever setup helper `server_test.go` uses to build its test `*echo.Echo`
should also create one tenant + one `super_admin` identity + a valid
session cookie attached to test requests, so the pre-existing
asset/policy/etc. mount-point tests keep testing what they were built to
test (that a canonical editor mounts) rather than accidentally testing
the login redirect. Add a small test helper:

```go
func newAuthenticatedRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.AddCookie(&http.Cookie{Name: "pluris_session", Value: testSessionToken(t)})
	return req
}
```

wiring `testSessionToken` to whatever DB the test's `*echo.Echo` opened
(via the `sessions.Create` call against a seeded identity) — the exact
shape depends on Step 1's DB-override mechanism, so make this consistent
with it.

- [ ] **Step 6: Run the full suite**

Run: `go build -buildvcs=false ./... && go test -buildvcs=false ./...`
Expected: all green.

- [ ] **Step 7: Delete the stale dev DB again and manually verify the gate**

```bash
rm -f pluris.db pluris.db-shm pluris.db-wal
go run ./cmd/console &
sleep 1
curl -sI http://localhost:8080/ | head -1   # expect 302
curl -sI http://localhost:8080/setup | head -1  # expect 200
kill %1
```

- [ ] **Step 8: Commit**

```bash
git add console/server/server.go console/server/server_test.go
git commit -m "feat: wire setup-gate/auth/RBAC middleware chain + CSRF protection"
```

---

### Task 13: Tenant switcher

**Files:**
- Modify: `console/handlers/auth.go` (add `TenantSwitchSubmit`)
- Modify: `console/server/server.go` (add route)
- Modify: `console/handlers/handlers.go` (replace hardcoded `tenantID := int64(1)`)

- [ ] **Step 1: Add `TenantSwitchSubmit` to `console/handlers/auth.go`**

```go
// TenantSwitchSubmit lets a super_admin change the active tenant for
// their current session. RBAC (pkg/auth.CanAccess) already restricts
// this route to super_admin; this handler double-checks defensively.
func (h *Handler) TenantSwitchSubmit(c echo.Context) error {
	sess := auth.FromContext(c.Request().Context())
	if sess == nil || sess.Role != "super_admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only super_admin can switch tenants")
	}

	tenantID, err := strconv.ParseInt(c.FormValue("tenant_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid tenant_id")
	}

	cookie, err := c.Cookie("pluris_session")
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "no active session")
	}
	sessionRow, err := h.sessions.Lookup(c.Request().Context(), cookie.Value)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "session not found")
	}

	if err := h.sessions.SetActiveTenant(c.Request().Context(), sessionRow.ID, tenantID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not switch tenant")
	}

	return c.Redirect(http.StatusFound, c.Request().Referer())
}
```

Add `"net/http"` and `"strconv"` to the file's imports if not already
present.

- [ ] **Step 2: Add the route in `console/server/server.go`**

```go
	e.POST("/tenant-switch", h.TenantSwitchSubmit)
```

- [ ] **Step 3: Replace the hardcoded tenant ID**

In `console/handlers/handlers.go`, find:

```go
	// TODO: Get tenant ID from session/auth
	// For now, use tenant ID 1 (will be created on first run)
	tenantID := int64(1)
```

Replace with:

```go
	sess := auth.FromContext(c.Request().Context())
	tenantID := sess.TenantID
```

Add `"github.com/pluris/pluris/pkg/auth"` to imports if not already
present (it was added in Task 11).

- [ ] **Step 4: Add the switcher UI to `Header` in `web/templates/layout.templ`**

Replace the hardcoded user chip:

```templ
			<div class="user-chip">
				<span>admin@pluris.local</span>
				<div class="user-avatar">A</div>
			</div>
```

with:

```templ
			{{ sess := auth.FromContext(ctx) }}
			if sess != nil {
				if len(sess.AvailableTenants) > 0 {
					<form method="POST" action="/tenant-switch" style="margin-right:8px;">
						<select name="tenant_id" onchange="this.form.submit()">
							for _, t := range sess.AvailableTenants {
								if t.ID == sess.TenantID {
									<option value={ strconv.FormatInt(t.ID, 10) } selected>{ t.Name }</option>
								} else {
									<option value={ strconv.FormatInt(t.ID, 10) }>{ t.Name }</option>
								}
							}
						</select>
					</form>
				}
				<div class="user-chip">
					<span>{ sess.Email }</span>
					<div class="user-avatar">{ strings.ToUpper(sess.DisplayName[:1]) }</div>
					<form method="POST" action="/logout" style="display:inline;">
						<button type="submit" style="background:none;border:none;color:inherit;cursor:pointer;">Log out</button>
					</form>
				</div>
			}
```

Add imports at the top of `web/templates/layout.templ`:

```go
import "github.com/pluris/pluris/pkg/auth"
import "strconv"
```

(Templ files declare Go imports the same way `.go` files do, at the top
of the file, outside any `templ` block — check the existing top of
`layout.templ` for where `"strings"` or similar are already imported and
match that placement; if `"strings"` isn't imported yet, add it too for
`strings.ToUpper`.)

- [ ] **Step 5: Regenerate and build**

Run: `~/go/bin/templ generate && go build -buildvcs=false ./...`
Expected: builds clean.

- [ ] **Step 6: Run the full suite**

Run: `go test -buildvcs=false ./...`

- [ ] **Step 7: Manual verification**

```bash
rm -f pluris.db pluris.db-shm pluris.db-wal
go run ./cmd/console &
sleep 1
curl -s http://localhost:8080/setup -o /dev/null -w '%{http_code}\n'
kill %1
```

Then actually walk through it in a browser once (setup → login → confirm
the header shows your real email and, since you're the only tenant, no
switcher dropdown yet — that's expected with one tenant).

- [ ] **Step 8: Commit**

```bash
git add console/handlers/auth.go console/handlers/handlers.go console/server/server.go web/templates/layout.templ web/templates/layout_templ.go
git commit -m "feat: add super_admin tenant switcher, replace hardcoded tenant id"
```

---

### Task 14: Identity parameter registry

**Files:**
- Modify: `catalog/params/definitions.go` (append identity ParamDefs)
- Modify: `catalog/params/schemas.go` (append `SchemaIdentity`)
- Test: `catalog/params/identity_schema_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
// catalog/params/identity_schema_test.go
package params

import "testing"

func TestIdentitySchemaRegistered(t *testing.T) {
	schema := SchemaBySubtype("identity")
	if schema == nil {
		t.Fatal("expected identity schema to be registered")
	}
	if !schema.HasParam("display_name") {
		t.Fatal("expected identity schema to mount display_name")
	}
	if !schema.HasParam("tenant") || !schema.HasParam("site") {
		t.Fatal("expected identity schema to reuse the shared tenant/site params")
	}
	for _, key := range schema.DefaultColumns {
		if DefByKey(key) == nil {
			t.Fatalf("default column %q has no registered ParamDef", key)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./catalog/params/... -run TestIdentitySchemaRegistered -v`
Expected: FAIL — `SchemaBySubtype("identity")` returns nil.

- [ ] **Step 3: Append identity ParamDefs to `catalog/params/definitions.go`**

Add to the `allDefs` slice (append before the closing `}`):

```go
	// --- Identity (account) ---
	{Key: "username", Label: "Username", Description: "Login/sAMAccountName-style short name.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "user_principal_name", Label: "UPN", Description: "user@domain-style principal name.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "email", Label: "Email", Description: "Primary email address.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "display_name", Label: "Name", Description: "Primary display name shown across the console.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "role", Label: "Role", Description: "Console access role (super_admin/admin/user_self_service).", Category: "identity", Type: TypeEnum, EnumValues: []string{"super_admin", "admin", "user_self_service"}, Filter: FilterEquals, Sort: SortAlpha},

	// --- Organization ---
	{Key: "title", Label: "Job title", Description: "Job title.", Category: "organization", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "department", Label: "Department", Description: "Department name.", Category: "organization", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "company", Label: "Company", Description: "Company name.", Category: "organization", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "employee_id", Label: "Employee ID", Description: "Internal employee identifier.", Category: "organization", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "employee_type", Label: "Employee type", Description: "full-time / contractor / etc.", Category: "organization", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "manager", Label: "Manager", Description: "Reporting manager.", Category: "organization", Type: TypeLink, LinkTarget: "user", Filter: FilterEquals, Sort: SortAlpha},

	// --- Contact ---
	{Key: "phone_office", Label: "Office phone", Description: "Office telephone number.", Category: "contact", Type: TypeString, Filter: FilterContains, Sort: SortNone},
	{Key: "phone_mobile", Label: "Mobile phone", Description: "Mobile telephone number.", Category: "contact", Type: TypeString, Filter: FilterContains, Sort: SortNone},
	{Key: "phone_home", Label: "Home phone", Description: "Home telephone number.", Category: "contact", Type: TypeString, Filter: FilterNone, Sort: SortNone},
	{Key: "fax", Label: "Fax", Description: "Fax number.", Category: "contact", Type: TypeString, Filter: FilterNone, Sort: SortNone},

	// --- Location (free-text physical address; distinct from the Site hierarchy link) ---
	{Key: "office", Label: "Office", Description: "Office name/number.", Category: "location", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "street_address", Label: "Street address", Description: "Street address.", Category: "location", Type: TypeString, Filter: FilterNone, Sort: SortNone},
	{Key: "city", Label: "City", Description: "City.", Category: "location", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "state", Label: "State/Province", Description: "State or province.", Category: "location", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "postal_code", Label: "Postal code", Description: "Postal/ZIP code.", Category: "location", Type: TypeString, Filter: FilterNone, Sort: SortNone},
	{Key: "country", Label: "Country", Description: "Country name.", Category: "location", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "country_code", Label: "Country code", Description: "ISO country code.", Category: "location", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha, Mono: true},

	// --- Profile & scripts (Windows-familiar) ---
	{Key: "home_directory", Label: "Home directory", Description: "Network home directory path.", Category: "profile", Type: TypeString, Filter: FilterNone, Sort: SortNone, Mono: true},
	{Key: "home_drive", Label: "Home drive", Description: "Mapped drive letter (e.g. H:).", Category: "profile", Type: TypeString, Filter: FilterNone, Sort: SortNone, Mono: true},
	{Key: "profile_path", Label: "Profile path", Description: "Roaming profile path.", Category: "profile", Type: TypeString, Filter: FilterNone, Sort: SortNone, Mono: true},
	{Key: "logon_script", Label: "Logon script", Description: "Logon script path.", Category: "profile", Type: TypeString, Filter: FilterNone, Sort: SortNone, Mono: true},

	// --- Security / login (display-only; not user-editable via the generic form) ---
	{Key: "account_enabled", Label: "Enabled", Description: "Whether this account can log in.", Category: "security", Type: TypeBool, Filter: FilterEquals, Sort: SortNone},
	{Key: "account_locked", Label: "Locked", Description: "Whether this account is locked after repeated failed logins.", Category: "security", Type: TypeBool, Filter: FilterEquals, Sort: SortNone},
	{Key: "account_expires_at", Label: "Account expires", Description: "Optional account expiry date.", Category: "security", Type: TypeTime, Filter: FilterDateLte, Sort: SortDate},
	{Key: "password_last_set_at", Label: "Password last set", Description: "When the password was last changed.", Category: "security", Type: TypeTime, Filter: FilterDateGte, Sort: SortDate},
	{Key: "password_never_expires", Label: "Password never expires", Description: "Password expiry exemption flag.", Category: "security", Type: TypeBool, Filter: FilterEquals, Sort: SortNone},
	{Key: "must_change_password", Label: "Must change password", Description: "Forces a password change on next login.", Category: "security", Type: TypeBool, Filter: FilterEquals, Sort: SortNone},
	{Key: "last_logon_at", Label: "Last logon", Description: "Most recent successful login.", Category: "security", Type: TypeTime, Filter: FilterDateGte, Sort: SortDate},
	{Key: "logon_count", Label: "Logon count", Description: "Total successful logins.", Category: "security", Type: TypeInt, Filter: FilterNumGte, Sort: SortNum},
	{Key: "bad_password_count", Label: "Failed logins", Description: "Consecutive failed login attempts since the last success.", Category: "security", Type: TypeInt, Filter: FilterNumGte, Sort: SortNum},

	// --- Preferences ---
	{Key: "locale", Label: "Locale", Description: "Preferred locale (e.g. en-US).", Category: "preferences", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "timezone", Label: "Timezone", Description: "Preferred timezone.", Category: "preferences", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "avatar_url", Label: "Avatar", Description: "Profile picture URL.", Category: "preferences", Type: TypeString, Filter: FilterNone, Sort: SortNone},

	// --- Metadata ---
	{Key: "description", Label: "Description", Description: "Free-text description.", Category: "metadata", Type: TypeString, Filter: FilterContains, Sort: SortNone},
	{Key: "notes", Label: "Notes", Description: "Internal admin notes.", Category: "metadata", Type: TypeString, Filter: FilterNone, Sort: SortNone},
```

`TypeBool` doesn't exist yet in `catalog/params/types.go`'s `ParamType`
enum (only String/Int/Float/Enum/Bool.../ — check
`grep -n "TypeBool" catalog/params/types.go`; the type list shown earlier
already includes `TypeBool ParamType = "bool"`, so this should already be
available — if not, add it to the `const` block).

- [ ] **Step 4: Append `SchemaIdentity` to `catalog/params/schemas.go`**

Add after the existing `SchemaDesk` definition:

```go
// ---------------------------------------------------------------------------
// Identity schema — the Users directory. Reuses the shared "tenant" and
// "site" ParamDefs already defined for Assets (docs/UX_INVARIANTS.md's
// INV-H1 shared hierarchy: Tenant → Site → Group → (Asset | Identity)).
// ---------------------------------------------------------------------------

var SchemaIdentity = &SubtypeSchema{
	Subtype:     "identity",
	Label:       "User",
	PluralLabel: "Users",
	Sections: []SchemaSection{
		{Key: "identity", Label: "Identity", Params: []string{
			"display_name", "username", "user_principal_name", "email", "tenant", "site", "role",
		}},
		{Key: "organization", Label: "Organization", Params: []string{
			"title", "department", "company", "employee_id", "employee_type", "manager",
		}},
		{Key: "contact", Label: "Contact", Params: []string{
			"phone_office", "phone_mobile", "phone_home", "fax",
		}},
		{Key: "location", Label: "Location", Params: []string{
			"office", "street_address", "city", "state", "postal_code", "country", "country_code",
		}},
		{Key: "profile", Label: "Profile & Scripts", Params: []string{
			"home_directory", "home_drive", "profile_path", "logon_script",
		}},
		{Key: "security", Label: "Security", Params: []string{
			"account_enabled", "account_locked", "account_expires_at",
			"password_last_set_at", "password_never_expires", "must_change_password",
			"last_logon_at", "logon_count", "bad_password_count",
		}},
		{Key: "preferences", Label: "Preferences", Params: []string{
			"locale", "timezone", "avatar_url",
		}},
		{Key: "metadata", Label: "Metadata", Params: []string{
			"description", "notes",
		}},
	},
	DefaultColumns: []string{
		"display_name", "username", "email", "department", "role", "site", "account_enabled",
	},
}
```

Then register it in the `Schemas` map:

```go
var Schemas = map[string]*SubtypeSchema{
	"computer": SchemaComputer,
	"server":   SchemaServer,
	"printer":  SchemaPrinter,
	"desk":     SchemaDesk,
	"identity": SchemaIdentity,
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -buildvcs=false ./catalog/params/... -v`
Expected: PASS

- [ ] **Step 6: Run the full suite**

Run: `go build -buildvcs=false ./... && go test -buildvcs=false ./...`

- [ ] **Step 7: Commit**

```bash
git add catalog/params/definitions.go catalog/params/schemas.go catalog/params/identity_schema_test.go
git commit -m "feat: register the Identity parameter schema (reuses shared tenant/site links)"
```

---

### Task 15: Users list (registry + page)

**Files:**
- Create: `web/lists/identities.go`
- Modify: `web/templates/pages.templ` (remove the stub `UsersPage()`)
- Create: `web/templates/users.templ`
- Modify: `console/handlers/handlers.go` (rewrite `Users` handler)
- Test: `web/lists/identities_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
// web/lists/identities_test.go
package lists

import "testing"

func TestUsersListRegistered(t *testing.T) {
	fields := FieldsFor(ListIDUsers)
	if len(fields) == 0 {
		t.Fatal("expected Users list to have registered fields")
	}
	found := false
	for _, f := range fields {
		if f.Key == "display_name" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected display_name field to be registered for Users list")
	}
}
```

Check the exact signature of `FieldsFor` (used elsewhere as
`fields := lists.FieldsFor(listID)` in `pages.templ`) — match it exactly.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./web/lists/... -run TestUsersListRegistered -v`
Expected: FAIL — `ListIDUsers` doesn't exist.

- [ ] **Step 3: Write `web/lists/identities.go`**

```go
// Package lists — Users column registry. Mirrors assets.go: all column
// definitions derive from the param registry (catalog/params); the only
// identity-specific logic here is the cell renderer.
package lists

import (
	"strconv"
	"time"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/params"
)

const ListIDUsers = "users"

func init() {
	groups, fields := buildUsersFieldsFromParams()
	Register(ListIDUsers, "Users", groups, fields)
}

func buildUsersFieldsFromParams() ([]FieldGroup, []FieldDef) {
	schema := params.SchemaIdentity
	groups := make([]FieldGroup, 0, len(schema.Sections))
	fields := make([]FieldDef, 0, len(schema.AllParamKeys()))

	for _, sec := range schema.Sections {
		groups = append(groups, FieldGroup{Key: sec.Key, Label: sec.Label})
		for _, key := range sec.Params {
			def := params.DefByKey(key)
			if def == nil {
				continue
			}
			fields = append(fields, FieldDef{
				Key:            def.Key,
				Label:          def.Label,
				Description:    def.Description,
				Group:          sec.Key,
				DefaultVisible: contains(schema.DefaultColumns, def.Key),
				Sort:           string(def.Sort),
				Width:          "",
			})
		}
	}
	return groups, fields
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// RenderUserCellValue returns the display string for one field of one
// identity — used by the Users list template's row renderer.
func RenderUserCellValue(u identities.Identity, key string) string {
	switch key {
	case "display_name":
		return u.ResolvedDisplayName()
	case "username":
		return u.Username
	case "user_principal_name":
		return u.UserPrincipalName
	case "email":
		return u.Email
	case "role":
		return u.Role.Label()
	case "title":
		return u.Title
	case "department":
		return u.Department
	case "company":
		return u.Company
	case "employee_id":
		return u.EmployeeID
	case "employee_type":
		return u.EmployeeType
	case "phone_office":
		return u.PhoneOffice
	case "phone_mobile":
		return u.PhoneMobile
	case "phone_home":
		return u.PhoneHome
	case "fax":
		return u.Fax
	case "office":
		return u.Office
	case "street_address":
		return u.StreetAddress
	case "city":
		return u.City
	case "state":
		return u.State
	case "postal_code":
		return u.PostalCode
	case "country":
		return u.Country
	case "country_code":
		return u.CountryCode
	case "home_directory":
		return u.HomeDirectory
	case "home_drive":
		return u.HomeDrive
	case "profile_path":
		return u.ProfilePath
	case "logon_script":
		return u.LogonScript
	case "account_enabled":
		return strconv.FormatBool(u.AccountEnabled)
	case "account_locked":
		return strconv.FormatBool(u.AccountLocked)
	case "password_never_expires":
		return strconv.FormatBool(u.PasswordNeverExpires)
	case "must_change_password":
		return strconv.FormatBool(u.MustChangePassword)
	case "logon_count":
		return strconv.FormatInt(u.LogonCount, 10)
	case "bad_password_count":
		return strconv.FormatInt(u.BadPasswordCount, 10)
	case "last_logon_at":
		if u.LastLogonAt == nil {
			return ""
		}
		return u.LastLogonAt.Format(time.RFC3339)
	case "locale":
		return u.Locale
	case "timezone":
		return u.Timezone
	case "avatar_url":
		return u.AvatarURL
	case "description":
		return u.Description
	case "notes":
		return u.Notes
	case "tenant":
		return strconv.FormatInt(u.TenantID, 10)
	case "site":
		if u.SiteID == 0 {
			return ""
		}
		return strconv.FormatInt(u.SiteID, 10)
	}
	return ""
}
```

Check `Register`'s exact signature in `web/lists/lists.go` (used
elsewhere as `Register(ListIDAssets, "Assets", groups, fields)`) and
match it precisely — the sample above assumes
`Register(id, title string, groups []FieldGroup, fields []FieldDef)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./web/lists/... -v`
Expected: PASS

- [ ] **Step 5: Remove the stub `UsersPage()` from `web/templates/pages.templ`**

Delete this block (it's replaced by `web/templates/users.templ` in the
next step):

```templ
// 2. Users
templ UsersPage() {
	@Layout("users", "Users") {
		@PageHeader("Identity", "Users",
			"Identity list — synced from Kanidm or manually added. Role assignment. Semi-automatic asset pairing. Self-service users see this page in read-only with scope-of-self filter.",
			"Add user")
		<div data-testid="page-users">
			@EmptyState("users", "No users yet",
				"Connect Kanidm or import from Active Directory in Server Administration to populate this list.",
				"Will mount editors/UserEditor — Increment 4+.")
			@invariantFooter()
		</div>
	}
}
```

- [ ] **Step 6: Write `web/templates/users.templ`**

```templ
package templates

import (
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/web/lists"
)

templ UsersPage(rows []identities.Identity) {
	@Layout("users", "Users") {
		<div class="page-content">
			@PageHeader("Identity", "Users",
				"Directory of people who can be assigned assets and, when role-gated, log into the console.",
				"Add user")
		</div>
		{{ listID := lists.ListIDUsers }}
		{{ fields := lists.FieldsFor(listID) }}
		<div data-testid="page-users" data-pluris-list={ listID }>
			<table>
				<thead>
					<tr>
						for _, f := range fields {
							if f.DefaultVisible {
								<th data-field={ f.Key }>{ f.Label }</th>
							}
						}
					</tr>
				</thead>
				<tbody>
					for _, row := range rows {
						<tr data-row-id={ strconv.FormatInt(row.ID, 10) } onclick={ templ.JSFuncCall("location.assign", "/users/"+strconv.FormatInt(row.ID, 10)) }>
							for _, f := range fields {
								if f.DefaultVisible {
									<td data-field={ f.Key }>{ lists.RenderUserCellValue(row, f.Key) }</td>
								}
							}
						</tr>
					}
				</tbody>
			</table>
			if len(rows) == 0 {
				@EmptyState("users", "No users yet",
					"Add your first user to start assigning asset ownership and console roles.",
					"")
			}
		</div>
	}
}
```

Add `"strconv"` to the imports. Check how `templ.JSFuncCall` or the
project's existing row-click-to-detail pattern works in
`web/templates/pages.templ`'s `assetsList` template (it likely uses a
`data-row-index`/`data-detail-id` attribute plus `lists.js` JS behavior
rather than an inline `onclick` — if so, follow that exact pattern
instead of `templ.JSFuncCall`, for consistency with every other list in
the app (INV-L9: one shared list engine, no bespoke per-list JS).

- [ ] **Step 7: Rewrite the `Users` handler in `console/handlers/handlers.go`**

Replace:

```go
func (h *Handler) Users(c echo.Context) error {
	return render(c, templates.UsersPage())
}
```

with:

```go
func (h *Handler) Users(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	rows, err := h.identitySvc.List(ctx, sess.TenantID, 200, 0)
	if err != nil {
		return err
	}
	return render(c, templates.UsersPage(rows))
}
```

- [ ] **Step 8: Regenerate, build, test**

Run: `~/go/bin/templ generate && go build -buildvcs=false ./... && go test -buildvcs=false ./...`

- [ ] **Step 9: Commit**

```bash
git add web/lists/identities.go web/lists/identities_test.go web/templates/pages.templ web/templates/pages_templ.go web/templates/users.templ web/templates/users_templ.go console/handlers/handlers.go
git commit -m "feat: replace Users stub with a real DB-backed directory list"
```

---

### Task 16: User detail page + create/edit/delete + asset pairing UI

**Files:**
- Modify: `web/templates/users.templ` (add `UserDetailPage`, `UserFormPage`)
- Modify: `console/handlers/handlers.go` (add `UserDetail`, `UserNew`, `UserCreate`, `UserEdit`, `UserUpdate`, `UserDelete`, asset owner assign/clear)
- Modify: `console/server/server.go` (routes)
- Test: extend `console/server/server_test.go`

- [ ] **Step 1: Write the failing test**

```go
// Add to console/server/server_test.go
func TestUserDetailPageMountsForExistingUser(t *testing.T) {
	// Follows the same authenticated-request pattern established in
	// Task 12/Step 5 (newAuthenticatedRequest) — seed one extra identity
	// beyond the authenticated super_admin, then request its detail page.
	e, extraIdentityID := newTestServerWithExtraIdentity(t)
	req := newAuthenticatedRequest(t, http.MethodGet, "/users/"+strconv.FormatInt(extraIdentityID, 10))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`data-testid="page-user-detail"`)) {
		t.Fatal("expected page-user-detail anchor in response body")
	}
}
```

`newTestServerWithExtraIdentity` is a small helper you add alongside
`newAuthenticatedRequest` from Task 12 — same DB, one more
`CreateIdentity` call, returning its ID.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./console/server/... -run TestUserDetailPageMountsForExistingUser -v`
Expected: FAIL — route doesn't exist (404).

- [ ] **Step 3: Add `UserDetailPage` and `UserFormPage` to `web/templates/users.templ`**

```templ
templ UserDetailPage(user identities.Identity, assigned []assets.Asset, csrfToken string) {
	@Layout("users", user.ResolvedDisplayName()) {
		<div class="page-content" data-testid="page-user-detail">
			@PageHeader("Identity", user.ResolvedDisplayName(), user.Email, "Edit")
			{{ schema := params.SchemaIdentity }}
			for _, sec := range schema.Sections {
				<section>
					<h3>{ sec.Label }</h3>
					<dl>
						for _, key := range sec.Params {
							{{ def := params.DefByKey(key) }}
							if def != nil {
								<dt>{ def.Label }</dt>
								<dd>{ lists.RenderUserCellValue(user, key) }</dd>
							}
						}
					</dl>
				</section>
			}
			<section>
				<h3>Assigned assets</h3>
				if len(assigned) == 0 {
					<p>No assets assigned to this user.</p>
				} else {
					<ul>
						for _, a := range assigned {
							<li><a href={ templ.SafeURL("/assets/" + a.Subtype.Slug() + "/" + a.ID) }>{ a.Payload.Kind() }: { a.ID }</a></li>
						}
					</ul>
				}
			</section>
			<form method="POST" action={ templ.SafeURL("/users/" + strconv.FormatInt(user.ID, 10) + "/delete") } onsubmit="return confirm('Delete this user?')">
				<input type="hidden" name="_csrf" value={ csrfToken }/>
				<button type="submit">Delete user</button>
			</form>
		</div>
	}
}

templ UserFormPage(user identities.Identity, csrfToken string, errorMsg string, isNew bool) {
	{{ action := "/users/" + strconv.FormatInt(user.ID, 10) + "/edit" }}
	if isNew {
		{{ action = "/users/new" }}
	}
	@Layout("users", "User form") {
		<div class="page-content">
			<form method="POST" action={ templ.SafeURL(action) }>
				<input type="hidden" name="_csrf" value={ csrfToken }/>
				<label for="username">Username</label>
				<input type="text" id="username" name="username" value={ user.Username } required/>
				<label for="email">Email</label>
				<input type="email" id="email" name="email" value={ user.Email } required/>
				<label for="display_name">Display name</label>
				<input type="text" id="display_name" name="display_name" value={ user.DisplayName } required/>
				<label for="title">Title</label>
				<input type="text" id="title" name="title" value={ user.Title }/>
				<label for="department">Department</label>
				<input type="text" id="department" name="department" value={ user.Department }/>
				if errorMsg != "" {
					<div class="auth-error">{ errorMsg }</div>
				}
				<button type="submit">Save</button>
			</form>
		</div>
	}
}
```

Add `"github.com/pluris/pluris/catalog/assets"` and
`"github.com/pluris/pluris/catalog/params"` to `users.templ`'s imports.
Check `assets.Asset`'s exact field names (`ID`, `Subtype`, `Payload`) and
`Subtype.Slug()`/`Payload.Kind()` against `catalog/assets/types.go` — use
whatever the real accessor names are if they differ from this sample.

- [ ] **Step 4: Add handlers to `console/handlers/handlers.go`**

```go
func (h *Handler) UserDetail(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	user, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	assigned, err := h.identitySvc.ListAssignedAssets(ctx, sess.TenantID, id)
	if err != nil {
		return err
	}
	return render(c, templates.UserDetailPage(user, assigned, csrfTokenFrom(c)))
}

func (h *Handler) UserNewShow(c echo.Context) error {
	return render(c, templates.UserFormPage(identities.Identity{}, csrfTokenFrom(c), "", true))
}

func (h *Handler) UserCreateSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	in := identities.Identity{
		Username:    c.FormValue("username"),
		Email:       c.FormValue("email"),
		DisplayName: c.FormValue("display_name"),
		Title:       c.FormValue("title"),
		Department:  c.FormValue("department"),
		Role:        identities.RoleUserSelfService,
	}
	created, err := h.identitySvc.Create(ctx, sess.TenantID, in)
	if err != nil {
		return render(c, templates.UserFormPage(in, csrfTokenFrom(c), "Could not create user: "+err.Error(), true))
	}
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(created.ID, 10))
}

func (h *Handler) UserEditShow(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	user, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	return render(c, templates.UserFormPage(user, csrfTokenFrom(c), "", false))
}

func (h *Handler) UserUpdateSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	existing, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	existing.Username = c.FormValue("username")
	existing.Email = c.FormValue("email")
	existing.DisplayName = c.FormValue("display_name")
	existing.Title = c.FormValue("title")
	existing.Department = c.FormValue("department")

	if _, err := h.identitySvc.Update(ctx, existing); err != nil {
		return render(c, templates.UserFormPage(existing, csrfTokenFrom(c), "Could not save: "+err.Error(), false))
	}
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(id, 10))
}

func (h *Handler) UserDeleteSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	if err := h.identitySvc.Delete(ctx, id); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/users")
}
```

Add `"net/http"`, `"strconv"`, and
`"github.com/pluris/pluris/catalog/identities"` to
`console/handlers/handlers.go`'s imports if not already present.

- [ ] **Step 5: Add the routes in `console/server/server.go`**

```go
	e.GET("/users/new", h.UserNewShow)
	e.POST("/users/new", h.UserCreateSubmit)
	e.GET("/users/:id", h.UserDetail)
	e.GET("/users/:id/edit", h.UserEditShow)
	e.POST("/users/:id/edit", h.UserUpdateSubmit)
	e.POST("/users/:id/delete", h.UserDeleteSubmit)
```

Add these **before** the existing `e.GET("/users", h.Users)` line's
neighbors are unaffected, but make sure `/users/new` is registered before
any catch-all — Echo matches static segments before params, so ordering
here doesn't actually matter, but keep them grouped together for
readability.

- [ ] **Step 6: Add the asset owner picker**

In `console/handlers/handlers.go`, add:

```go
func (h *Handler) AssetSetOwner(c echo.Context) error {
	ctx := c.Request().Context()
	assetID, err := strconv.ParseInt(c.Param("assetDBID"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid asset id")
	}
	ownerID, err := strconv.ParseInt(c.FormValue("owner_id"), 10, 64)
	if err != nil || ownerID == 0 {
		if err := h.assetSvc.ClearOwner(ctx, assetID); err != nil {
			return err
		}
	} else if err := h.assetSvc.SetOwner(ctx, assetID, ownerID); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, c.Request().Referer())
}
```

Note: `AssetDetail`'s route param is the asset's **human-readable** `id`
(string, e.g. `comp.demo.hq.0001`), not its numeric DB primary key —
check `pkg/services/assets.go`'s `GetByID` to see which one it resolves
by. If `AssetService` doesn't currently expose the numeric DB ID on
`assets.Asset`, add a small
`func (s *AssetService) ResolveDBID(ctx context.Context, humanOrUUID string) (int64, error)`
helper so this handler can look it up, rather than threading a second ID
scheme through the URL.

Add the route:

```go
	e.POST("/assets/:subtype/:id/owner", h.AssetSetOwner)
```

Then add an owner-assignment `<form>` (a `<select>` populated from
`h.identitySvc.List(...)`, POSTing to this route) to the Asset detail
template — find `AssetDetailPageWithData` in `web/templates/pages.templ`
and add it near the existing CMDB/lifecycle fields, following the same
`<section>`/`<dl>` layout already used there.

- [ ] **Step 7: Regenerate, build, test**

Run: `~/go/bin/templ generate && go build -buildvcs=false ./... && go test -buildvcs=false ./...`
Expected: PASS, including the new `TestUserDetailPageMountsForExistingUser`.

- [ ] **Step 8: Commit**

```bash
git add web/templates/users.templ web/templates/users_templ.go web/templates/pages.templ web/templates/pages_templ.go console/handlers/handlers.go console/server/server.go console/server/server_test.go
git commit -m "feat: add user detail/create/edit/delete pages and asset owner picker"
```

---

### Task 17: End-to-end manual verification

**Files:** none (verification only)

- [ ] **Step 1: Reset to a clean database**

```bash
rm -f pluris.db pluris.db-shm pluris.db-wal
```

- [ ] **Step 2: Full rebuild**

Run: `~/go/bin/templ generate && ~/go/bin/sqlc generate && go build -buildvcs=false -o bin/pluris-console ./cmd/console && go test -buildvcs=false ./...`
Expected: all green.

- [ ] **Step 3: Start the server on a free port**

```bash
PLURIS_HTTP_ADDR=:8081 ./bin/pluris-console &
sleep 1
```

- [ ] **Step 4: Confirm the setup gate fires**

```bash
curl -sI http://localhost:8081/ | head -1
```

Expected: `HTTP/1.1 302 Found` with `Location: /setup`.

- [ ] **Step 5: Walk through the flow in a real browser**

Open `http://localhost:8081/` (redirects to `/setup`):
1. Create org name, your name, email, an 8+ character password.
2. Confirm redirect to `/login`; log in with those same credentials.
3. Confirm the dashboard loads and the header shows your real email (not
   "admin@pluris.local").
4. Go to `/users` — confirm it lists exactly the one super_admin you just
   created (not the old "No users yet" empty state).
5. Click "Add user", create a second user (a plain `user_self_service`).
6. Open that new user's detail page — confirm "Assigned assets" shows
   "No assets assigned."
7. Go to an existing seeded asset's detail page (`/assets/computers`,
   click into `dev-laptop-001` from the earlier session), use the new
   owner picker to assign it to the new user.
8. Go back to the user's detail page — confirm the asset now appears
   under "Assigned assets."
9. Log out — confirm you're redirected to `/login` and `/` now redirects
   there too (not straight to the dashboard).
10. Try 10 wrong-password login attempts against one account — confirm
    the 11th attempt (even with the correct password) shows the "account
    locked" message.

- [ ] **Step 6: Stop the server**

```bash
kill %1
```

- [ ] **Step 7: Report results to the user**

Summarize what was verified and any deviations from the spec found along
the way (e.g. generated sqlc field names that differed from the plan's
samples) — these are expected and fine; call out anything that required
a design-level judgment call beyond a mechanical rename.

---

## Self-review notes (for whoever executes this plan)

- **Spec coverage**: Task 1–2 cover §4–5 (schema + queries), Task 3–5
  cover the identity type/service/pairing (§6–7 partially, §4 pairing),
  Task 6–10 cover §8 (auth), Task 11–13 cover §9–10 (setup + login +
  switcher), Task 14 covers §6 (param registry), Task 15–16 cover §11
  (UI), Task 17 covers §12–13 (testing + rollout verification).
- **Known soft spots flagged inline, not hidden**: several steps
  (Task 5/Step 7, Task 10's `ListTenants` check, Task 11's placeholder
  param-struct names, Task 12's DB-override mechanism, Task 16's
  human-ID-vs-DB-ID resolution) depend on exact generated code or
  existing test-harness shape that could not be fully confirmed without
  running `sqlc generate`/`templ generate` against the real tree ahead of
  time. Each of those steps says explicitly what to check and how to
  resolve it — treat compiler/test output as authoritative over the
  plan's sample code in those spots.
