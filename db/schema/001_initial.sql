-- Pluris Database Schema
-- SQLite with WAL mode for concurrency
-- Based on UX_INVARIANTS.md VII.A field lists

-- Enable foreign keys and WAL mode
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

-- Tenants (multi-tenant isolation root)
CREATE TABLE IF NOT EXISTS tenants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Sites (geographic/network boundary within tenant)
CREATE TABLE IF NOT EXISTS sites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, slug)
);

-- Groups (assignment-target collections)
CREATE TABLE IF NOT EXISTS groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    site_id INTEGER REFERENCES sites(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, slug)
);

-- Assets (computers, servers, printers, desks with dynamic params)
CREATE TABLE IF NOT EXISTS assets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE, -- mTLS subject CN
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    site_id INTEGER REFERENCES sites(id) ON DELETE SET NULL,
    
    -- Asset subtype discriminator
    subtype TEXT NOT NULL CHECK(subtype IN ('computer', 'server', 'printer', 'desk')),
    
    -- Subtype-specific dynamic parameters (JSON)
    -- Computer: hostname, fqdn, os_family, os_distribution, os_version, architecture, cpu_summary, ram_mb, storage_mb
    -- Server: hostname, fqdn, os_*, services[], uptime_started_at, role
    -- Printer: model, vendor, ip, queues[], supported_formats, current_consumables[]
    -- Desk: location_label, dock_model, monitor_count, guest_profile_id
    subtype_payload TEXT NOT NULL DEFAULT '{}', -- JSON
    
    -- Common fields across all subtypes
    enrollment_state TEXT NOT NULL DEFAULT 'pending' CHECK(enrollment_state IN ('pending', 'approved', 'enrolled', 'disabled', 'revoked')),
    enrolled_at TIMESTAMP,
    last_seen_at TIMESTAMP,
    agent_version TEXT,
    labels TEXT DEFAULT '{}', -- JSON map<string,string>
    
    -- Asset management (CMDB) fields
    lifecycle_state TEXT DEFAULT 'active' CHECK(lifecycle_state IN ('active', 'in_repair', 'decommissioned', 'disposed')),
    location TEXT, -- free text + site_id reference
    owner_identity_id INTEGER REFERENCES identities(id) ON DELETE SET NULL,
    vendor TEXT,
    purchase_date DATE,
    purchase_price REAL,
    warranty_expires_at DATE,
    
    -- Human-readable asset ID (comp.demo.hq.0001)
    human_id TEXT UNIQUE,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common asset queries
CREATE INDEX IF NOT EXISTS idx_assets_tenant ON assets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_assets_site ON assets(site_id);
CREATE INDEX IF NOT EXISTS idx_assets_subtype ON assets(subtype);
CREATE INDEX IF NOT EXISTS idx_assets_enrollment ON assets(enrollment_state);
CREATE INDEX IF NOT EXISTS idx_assets_last_seen ON assets(last_seen_at);

-- Full-text search on hostname/name in JSON payload (requires JSON functions)
-- CREATE INDEX IF NOT EXISTS idx_assets_hostname ON assets(json_extract(subtype_payload, '$.hostname'));

-- Asset-to-Asset relationships (peripheral_of, docked_to, etc)
CREATE TABLE IF NOT EXISTS asset_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    from_asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    to_asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    relation TEXT NOT NULL CHECK(relation IN ('peripheral_of', 'docked_to', 'printed_via', 'hosts_vm', 'connected_to', 'other')),
    metadata TEXT DEFAULT '{}', -- JSON (relation-specific data)
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_asset_id, to_asset_id, relation)
);

CREATE INDEX IF NOT EXISTS idx_asset_links_from ON asset_links(from_asset_id);
CREATE INDEX IF NOT EXISTS idx_asset_links_to ON asset_links(to_asset_id);
CREATE INDEX IF NOT EXISTS idx_asset_links_relation ON asset_links(relation);

-- Identities (end users) are defined in db/schema/002_identity_ad_compat.sql,
-- not here. (An earlier version of this file created a bare-bones
-- `identities` table at this point; 002 used to DROP and replace it with
-- the richer AD-compatible schema on every migrate() run. Now that 002
-- no longer drops the table -- see 002's header comment -- this file must
-- not create its own `identities` table, or 002's later
-- `CREATE TABLE IF NOT EXISTS identities` would be a no-op against this
-- file's bare-bones columns on a fresh database, breaking every query in
-- db/queries/identities.sql that expects the full schema. Foreign keys
-- below that reference identities(id) are forward references, which
-- SQLite allows since FK targets aren't resolved until enforcement time.)

-- Group memberships (Assets and Identities belong to Groups)
CREATE TABLE IF NOT EXISTS group_memberships (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    asset_id INTEGER REFERENCES assets(id) ON DELETE CASCADE,
    identity_id INTEGER REFERENCES identities(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK((asset_id IS NOT NULL AND identity_id IS NULL) OR (asset_id IS NULL AND identity_id IS NOT NULL)),
    UNIQUE(group_id, asset_id),
    UNIQUE(group_id, identity_id)
);

CREATE INDEX IF NOT EXISTS idx_group_memberships_group ON group_memberships(group_id);
CREATE INDEX IF NOT EXISTS idx_group_memberships_asset ON group_memberships(asset_id);
CREATE INDEX IF NOT EXISTS idx_group_memberships_identity ON group_memberships(identity_id);

-- Policy Catalog (in-memory for v1, but schema ready for custom policies)
-- Note: Bundled policies live in catalog/policies/ Go code
-- This table is for tenant-custom policies only
CREATE TABLE IF NOT EXISTS custom_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    policy_urn TEXT NOT NULL, -- tenant.<tenant_id>.<slug>
    name TEXT NOT NULL,
    description TEXT,
    scope TEXT NOT NULL CHECK(scope IN ('machine', 'user', 'both')),
    category_path TEXT NOT NULL, -- Pluris category tree path
    parameters_schema TEXT, -- JSON Schema
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, policy_urn)
);

-- Policy Modules (versioned, signed enforcement scripts)
CREATE TABLE IF NOT EXISTS policy_modules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_urn TEXT NOT NULL UNIQUE, -- pluris.sshd.password-auth-disable
    tenant_id INTEGER REFERENCES tenants(id) ON DELETE CASCADE, -- NULL = bundled
    title TEXT NOT NULL,
    description TEXT,
    is_bundled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_policy_modules_tenant ON policy_modules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_policy_modules_bundled ON policy_modules(is_bundled);

-- Policy Module Versions (immutable once published)
CREATE TABLE IF NOT EXISTS policy_module_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL REFERENCES policy_modules(id) ON DELETE CASCADE,
    version TEXT NOT NULL, -- semver: 1.2.0
    state TEXT NOT NULL DEFAULT 'draft' CHECK(state IN ('draft', 'published', 'superseded', 'revoked')),
    
    -- Manifest fields
    manifest_yaml TEXT NOT NULL, -- source of truth
    target_os TEXT NOT NULL, -- JSON array: ["linux"] or ["linux", "windows"]
    scope TEXT NOT NULL CHECK(scope IN ('machine', 'user', 'both')),
    runtime TEXT NOT NULL CHECK(runtime IN ('bash', 'python', 'go', 'ansible-task')),
    satisfies TEXT NOT NULL, -- JSON array of policy URNs
    parameters_schema TEXT, -- JSON Schema
    depends_on TEXT DEFAULT '[]', -- JSON array: [{module_id, version_constraint}]
    conflicts TEXT DEFAULT '[]', -- JSON array of module URNs
    
    -- Lifecycle scripts
    enforce_script TEXT NOT NULL,
    validate_script TEXT,
    rollback_script TEXT,
    
    -- Signing (tenant Ed25519 key)
    signature_algo TEXT, -- 'ed25519'
    signature_data TEXT, -- base64
    signed_by TEXT, -- tenant:<tenant_id>:key:<key_id>
    
    published_at TIMESTAMP,
    published_by INTEGER REFERENCES identities(id),
    superseded_by_version TEXT,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(module_id, version)
);

CREATE INDEX IF NOT EXISTS idx_module_versions_module ON policy_module_versions(module_id);
CREATE INDEX IF NOT EXISTS idx_module_versions_state ON policy_module_versions(state);

-- Configuration Groups (bind policies to targets with values)
CREATE TABLE IF NOT EXISTS configuration_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    scope TEXT NOT NULL CHECK(scope IN ('machine', 'user', 'both')),
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_configuration_groups_tenant ON configuration_groups(tenant_id);

-- Configuration Group Bindings (policy URN + parameter values + chosen module)
CREATE TABLE IF NOT EXISTS configuration_group_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    configuration_group_id INTEGER NOT NULL REFERENCES configuration_groups(id) ON DELETE CASCADE,
    policy_urn TEXT NOT NULL, -- references policy catalog entry
    
    -- Configured value
    state TEXT NOT NULL DEFAULT 'enabled' CHECK(state IN ('enabled', 'disabled', 'not_configured')),
    parameter_values TEXT DEFAULT '{}', -- JSON, validated against policy's parameters_schema
    
    -- Module selection
    module_id INTEGER REFERENCES policy_modules(id),
    module_version_pinned TEXT, -- explicit version pin
    
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(configuration_group_id, policy_urn)
);

CREATE INDEX IF NOT EXISTS idx_cg_bindings_group ON configuration_group_bindings(configuration_group_id);
CREATE INDEX IF NOT EXISTS idx_cg_bindings_policy ON configuration_group_bindings(policy_urn);

-- Configuration Group Assignments (which assets/identities get this CG)
CREATE TABLE IF NOT EXISTS configuration_group_assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    configuration_group_id INTEGER NOT NULL REFERENCES configuration_groups(id) ON DELETE CASCADE,
    
    -- Target (one of: asset, identity, group, site, tenant)
    target_type TEXT NOT NULL CHECK(target_type IN ('asset', 'identity', 'group', 'site', 'tenant')),
    target_id INTEGER NOT NULL, -- references assets.id / identities.id / groups.id / sites.id / tenants.id
    
    priority INTEGER NOT NULL DEFAULT 0,
    enforced BOOLEAN NOT NULL DEFAULT FALSE, -- GP "enforced" flag
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(configuration_group_id, target_type, target_id)
);

CREATE INDEX IF NOT EXISTS idx_cg_assignments_group ON configuration_group_assignments(configuration_group_id);
CREATE INDEX IF NOT EXISTS idx_cg_assignments_target ON configuration_group_assignments(target_type, target_id);

-- Module Installations (which modules are installed on which assets, with refcount)
CREATE TABLE IF NOT EXISTS module_installations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    module_id INTEGER NOT NULL REFERENCES policy_modules(id) ON DELETE CASCADE,
    module_version_pinned TEXT NOT NULL,
    
    state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending', 'applied', 'disabled', 'failing', 'orphaned')),
    applied_at TIMESTAMP,
    last_validated_at TIMESTAMP,
    last_report TEXT, -- JSON
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(asset_id, module_id, module_version_pinned)
);

CREATE INDEX IF NOT EXISTS idx_module_installations_asset ON module_installations(asset_id);
CREATE INDEX IF NOT EXISTS idx_module_installations_module ON module_installations(module_id);
CREATE INDEX IF NOT EXISTS idx_module_installations_state ON module_installations(state);

-- Module Installation Dependencies (edges for refcount calculation)
CREATE TABLE IF NOT EXISTS module_installation_dependencies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    installation_id INTEGER NOT NULL REFERENCES module_installations(id) ON DELETE CASCADE,
    depends_on_installation_id INTEGER NOT NULL REFERENCES module_installations(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(installation_id, depends_on_installation_id)
);

CREATE INDEX IF NOT EXISTS idx_mod_deps_installation ON module_installation_dependencies(installation_id);
CREATE INDEX IF NOT EXISTS idx_mod_deps_depends_on ON module_installation_dependencies(depends_on_installation_id);
