-- Migration 003: roles, identity_roles, installed_software, activity_log,
-- asset CMDB extras, AD-style group fields, and the role vocabulary
-- change (user_self_service becomes user; technician added).
-- Runs exactly once via schema_migrations, so ALTER TABLE is safe here.
-- Contains PRAGMA statements, so it runs outside the tracker transaction
-- and must be valid executed top to bottom in a single pass.

-- Custom role definitions per tenant (RBAC beyond the builtin role slugs).
CREATE TABLE IF NOT EXISTS roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    is_builtin BOOLEAN NOT NULL DEFAULT FALSE,
    template_slug TEXT,
    permissions TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, slug)
);

-- Assignment of roles to identities (many to many).
CREATE TABLE IF NOT EXISTS identity_roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    assigned_by INTEGER REFERENCES identities(id) ON DELETE SET NULL,
    UNIQUE(identity_id, role_id)
);

-- Software inventory reported per asset.
CREATE TABLE IF NOT EXISTS installed_software (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    version TEXT,
    publisher TEXT,
    pkg_type TEXT NOT NULL DEFAULT 'native'
        CHECK(pkg_type IN ('deb','rpm','appimage','flatpak','snap','wine','native','other')),
    installed_at TIMESTAMP,
    size_mb INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Generic per entity activity feed (who did what, when).
CREATE TABLE IF NOT EXISTS activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id INTEGER NOT NULL,
    event TEXT NOT NULL,
    detail TEXT,
    actor_identity_id INTEGER REFERENCES identities(id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_activity_entity ON activity_log(tenant_id, entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_software_asset ON installed_software(asset_id);
CREATE INDEX IF NOT EXISTS idx_identity_roles_identity ON identity_roles(identity_id);

-- Asset CMDB extras.
ALTER TABLE assets ADD COLUMN description TEXT;
ALTER TABLE assets ADD COLUMN managed_by_identity_id INTEGER REFERENCES identities(id) ON DELETE SET NULL;

-- AD style group classification fields.
ALTER TABLE groups ADD COLUMN group_category TEXT NOT NULL DEFAULT 'security';
ALTER TABLE groups ADD COLUMN group_scope TEXT NOT NULL DEFAULT 'global';

-- ---------------------------------------------------------------------------
-- Identities role vocabulary rebuild.
-- SQLite cannot ALTER a CHECK constraint, so this is the standard rebuild:
-- create the new table, copy every row, drop the old table, rename.
-- The column list below is copied verbatim from
-- db/schema/002_identity_ad_compat.sql, changing ONLY the role line:
-- user_self_service becomes user, and technician is added.
-- ---------------------------------------------------------------------------

PRAGMA foreign_keys=OFF;

CREATE TABLE identities_new (
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

    -- Profile and scripts (Windows familiar)
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
    role TEXT NOT NULL DEFAULT 'user'
        CHECK(role IN ('super_admin', 'admin', 'technician', 'user')),
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

-- Copy every row. Every column is listed explicitly, id included, so all
-- foreign key references to identities(id) from other tables stay valid.
-- The CASE expression sits in the role position and remaps the retired
-- user_self_service slug to user.
INSERT INTO identities_new (
    id, tenant_id, site_id,
    username, user_principal_name, email, display_name, given_name, surname, initials,
    title, department, company, employee_id, employee_type, manager_id,
    phone_office, phone_mobile, phone_home, fax,
    office, street_address, city, state, postal_code, country, country_code,
    home_directory, home_drive, profile_path, logon_script,
    account_enabled, account_locked, account_expires_at, password_hash,
    password_last_set_at, password_never_expires, must_change_password,
    last_logon_at, logon_count, bad_password_count, last_bad_password_at,
    role, avatar_url, locale, timezone,
    description, notes,
    created_at, updated_at
)
SELECT
    id, tenant_id, site_id,
    username, user_principal_name, email, display_name, given_name, surname, initials,
    title, department, company, employee_id, employee_type, manager_id,
    phone_office, phone_mobile, phone_home, fax,
    office, street_address, city, state, postal_code, country, country_code,
    home_directory, home_drive, profile_path, logon_script,
    account_enabled, account_locked, account_expires_at, password_hash,
    password_last_set_at, password_never_expires, must_change_password,
    last_logon_at, logon_count, bad_password_count, last_bad_password_at,
    CASE WHEN role = 'user_self_service' THEN 'user' ELSE role END,
    avatar_url, locale, timezone,
    description, notes,
    created_at, updated_at
FROM identities;

-- This DROP TABLE is the ONE sanctioned exception to the no DROP rule:
-- it is part of an atomic rename rebuild that preserves all rows (copied
-- by the INSERT above), and this migration runs exactly once per database
-- via the schema_migrations tracker, never on ordinary restarts.
DROP TABLE identities;
ALTER TABLE identities_new RENAME TO identities;

-- Recreate the indexes dropped with the old table.
CREATE INDEX IF NOT EXISTS idx_identities_tenant ON identities(tenant_id);
CREATE INDEX IF NOT EXISTS idx_identities_site ON identities(site_id);
CREATE INDEX IF NOT EXISTS idx_identities_email ON identities(email);
CREATE INDEX IF NOT EXISTS idx_identities_username ON identities(username);
CREATE INDEX IF NOT EXISTS idx_identities_manager ON identities(manager_id);
CREATE INDEX IF NOT EXISTS idx_identities_department ON identities(department);
CREATE INDEX IF NOT EXISTS idx_identities_role ON identities(role);

PRAGMA foreign_keys=ON;
