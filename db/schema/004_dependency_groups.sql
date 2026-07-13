-- Migration 004: dependency groups (module applicability filters).
-- A dependency group is an AND set of conditions over device fact keys.
-- Modules link to groups in two roles: platform (match any) and
-- requirement (match all). Contains no PRAGMA, so it runs inside the
-- tracker transaction.

-- Named, reusable applicability filter, scoped per tenant.
CREATE TABLE IF NOT EXISTS dependency_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    is_builtin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, slug)
);

-- One predicate inside a group. All conditions in a group are ANDed.
-- param_path is a canonical parameter path; value_json is a JSON array
-- of strings (empty array for the exists operator).
CREATE TABLE IF NOT EXISTS dependency_group_conditions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL REFERENCES dependency_groups(id) ON DELETE CASCADE,
    param_path TEXT NOT NULL,
    operator TEXT NOT NULL,
    value_json TEXT NOT NULL DEFAULT '[]',
    seq INTEGER NOT NULL DEFAULT 0
);

-- Link from a policy module (catalog mock slug, no FK) to a dependency
-- group, tagged with the role the group plays for that module.
CREATE TABLE IF NOT EXISTS module_dependency_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    module_id TEXT NOT NULL,
    group_id INTEGER NOT NULL REFERENCES dependency_groups(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    UNIQUE(tenant_id, module_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_depgroups_tenant ON dependency_groups(tenant_id);
CREATE INDEX IF NOT EXISTS idx_depconditions_group ON dependency_group_conditions(group_id);
CREATE INDEX IF NOT EXISTS idx_modulelinks_module ON module_dependency_links(tenant_id, module_id);
CREATE INDEX IF NOT EXISTS idx_modulelinks_group ON module_dependency_links(group_id);
