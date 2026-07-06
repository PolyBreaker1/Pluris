-- Migration 002: Identity system with AD-familiar attributes.
--
-- Like 001_initial.sql, this migration is idempotent via `CREATE TABLE
-- IF NOT EXISTS` — pkg/database.Database.migrate() re-runs every
-- migration file on EVERY Open() call, not just once ever, so this file
-- must never contain a DROP TABLE. (An earlier version of this file did,
-- reasoning "no production data yet" — true for a one-time migration,
-- but wrong here since it wiped all identities/sessions/audit-log rows
-- on every single app restart. Found and fixed during Task 13 review.)
--
-- Scope note: this migration intentionally does NOT include an
-- organizational-unit tree, AD-sync config table, or sync-tracking
-- columns (source/source_guid/last_synced_at/...). Those were drafted in
-- an earlier, never-applied version of this file but have no sync engine
-- behind them (ADR-009: external directory sync is Phase 2). Add them
-- back when that engine actually exists.

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
