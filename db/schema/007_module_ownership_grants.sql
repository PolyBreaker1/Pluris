-- Migration 007: module ownership and per-module grants.
--
-- Groundwork for the module/script editor (next phase): a policy module
-- (or, later, a script -- an unpackaged module) inherits the Pluris
-- permissions of the identity that creates it, that identity can grant
-- other identities/groups/roles per-module access, and the editor's
-- parameter tree is filtered by the creating identity's grants.
--
-- module_grants is the single, shared grants table for BOTH policy
-- modules and the future Scripts feature. A script is modeled as an
-- unpackaged module row in policy_modules, so it reuses this exact
-- table -- no parallel "script_grants" table may ever be introduced.
--
-- subject_type/level are free-text here (SQLite ALTER TABLE cannot add
-- CHECK constraints to an existing table, and this migration also alters
-- policy_modules) and are enforced in Go instead: subject_type is one of
-- 'identity' | 'group' | 'role', level is one of 'view' | 'edit' | 'admin'
-- (see pkg/authz/modules.go for the decision logic that consumes these
-- rows). Contains no PRAGMA, so it runs inside the tracker transaction.

-- owner_identity_id is the identity whose Pluris permissions the module
-- inherits (creator). NULL means bundled/unowned -- bundled modules are
-- never owned by a tenant identity and are handled specially by the
-- authz decision helpers (view-only for endpoint_policy.view holders,
-- never editable via ownership).
ALTER TABLE policy_modules ADD COLUMN owner_identity_id INTEGER REFERENCES identities(id);

-- Per-module access grants, keyed by (module, subject). A subject can
-- hold at most one grant row per module; the level is upgraded/downgraded
-- in place via the UNIQUE constraint below (see UpsertModuleGrant).
CREATE TABLE IF NOT EXISTS module_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL REFERENCES policy_modules(id) ON DELETE CASCADE,
    subject_type TEXT NOT NULL,      -- 'identity' | 'group' | 'role' (enforced in Go)
    subject_id INTEGER NOT NULL,
    level TEXT NOT NULL,             -- 'view' | 'edit' | 'admin' (enforced in Go)
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(module_id, subject_type, subject_id)
);

CREATE INDEX IF NOT EXISTS idx_module_grants_module ON module_grants(module_id);
