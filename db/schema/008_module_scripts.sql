-- Migration 008: policy module persistence reconciliation.
--
-- Task 4.2 gives Policy Modules real DB persistence. 001_initial.sql
-- created policy_module_versions speculatively, ahead of the actual
-- domain model that later landed in catalog/policymodules (ADR-007).
-- That domain model diverges from the 001 schema in two ways this
-- migration reconciles:
--
--   1. Runtime is PER-PHASE, not per-version. catalog/policymodules'
--      LifecyclePhase.Runtime() derives bash vs wasm from the phase
--      itself (apply/disable/uninstall -> bash, validate/report ->
--      wasm) -- see catalog/policymodules/types.go. The 001 schema's
--      version-level `runtime` column (CHECK'd against a stale
--      bash|python|go|ansible-task enum that was never implemented)
--      is removed; runtime is now computed in Go from the phase, never
--      stored.
--   2. Scripts are a LIST, not three fixed columns. A module version
--      may carry apply/disable/uninstall/validate/report scripts, each
--      potentially multi-file (`seq` orders multiple scripts within one
--      phase). The 001 schema's enforce_script/validate_script/
--      rollback_script columns hard-coded exactly three phases and
--      couldn't represent validate/report at all. They're replaced by
--      the new policy_module_scripts table below.
--
-- Everything else moves forward unchanged (or gains a default): state,
-- target_os, scope, satisfies, parameters_schema (the single source of
-- truth for a module version's input parameters -- JSON Schema),
-- depends_on, conflicts, signature fields, published_at/by,
-- superseded_by_version, created_at.
--
-- manifest_yaml is KEPT but repurposed: it is no longer the source of
-- truth (001's original comment said "source of truth"). The structured
-- columns above are now the source of truth; manifest_yaml becomes a
-- DERIVED EXPORT ARTIFACT (rendered on demand for module-bundle export/
-- download), so it becomes nullable-in-practice (NOT NULL DEFAULT '').
--
-- Assumption / guard: this table was confirmed unwired by the Task 4.2
-- pre-flight grep gate (only db/queries/policy_modules.sql and
-- generated db/*.go touched policy_module_versions; no handler/service/
-- template read or wrote a row). It is expected to be empty in every
-- dev/prod database that has run 001-007 so far. SQLite cannot drop
-- columns with a single ALTER TABLE against older SQLite versions, and
-- more importantly we're also *changing semantics* (runtime removed,
-- scripts moved to a child table), so this migration recreates the
-- table via rename + create + best-effort copy-forward of compatible
-- columns rather than an in-place ALTER. The copy-forward means: IF a
-- database somehow already has rows (contrary to the assumption above),
-- their compatible fields survive; enforce_script becomes an 'apply'
-- row in the new policy_module_scripts table (best-effort preservation
-- instead of silent data loss), and validate_script/rollback_script are
-- similarly preserved as 'validate'/'uninstall' rows.
--
-- Contains no PRAGMA, so (like 003 onward) it runs inside the shared
-- migration-tracker transaction; a crash mid-migration rolls back
-- cleanly and re-runs from scratch next boot.

ALTER TABLE policy_module_versions RENAME TO policy_module_versions_pre008;

CREATE TABLE policy_module_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id INTEGER NOT NULL REFERENCES policy_modules(id) ON DELETE CASCADE,
    version TEXT NOT NULL, -- semver: 1.2.0
    state TEXT NOT NULL DEFAULT 'draft' CHECK(state IN ('draft', 'published', 'superseded', 'revoked')),

    -- Manifest fields (structured columns are the source of truth --
    -- see header comment; manifest_yaml is a derived export artifact).
    target_os TEXT NOT NULL DEFAULT '[]', -- JSON array: ["linux"] or ["linux", "windows"]
    scope TEXT NOT NULL CHECK(scope IN ('machine', 'user', 'both')),
    satisfies TEXT NOT NULL DEFAULT '[]', -- JSON array of policy URNs
    parameters_schema TEXT DEFAULT '', -- JSON Schema -- single source of truth for module input params
    depends_on TEXT DEFAULT '[]', -- JSON array: [{module_id, version_constraint}]
    conflicts TEXT DEFAULT '[]', -- JSON array of module URNs
    sandbox_profile TEXT NOT NULL DEFAULT '{}', -- JSON of catalog/policymodules.SandboxProfile
    report_schema TEXT NOT NULL DEFAULT '', -- JSON Schema for the `report` phase's output; '' = no report

    manifest_yaml TEXT NOT NULL DEFAULT '', -- DERIVED export artifact; structured columns above are the truth

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

INSERT INTO policy_module_versions (
    id, module_id, version, state, target_os, scope, satisfies,
    parameters_schema, depends_on, conflicts,
    manifest_yaml, signature_algo, signature_data, signed_by,
    published_at, published_by, superseded_by_version, created_at
)
SELECT
    id, module_id, version, state, target_os, scope, satisfies,
    parameters_schema, depends_on, conflicts,
    manifest_yaml, signature_algo, signature_data, signed_by,
    published_at, published_by, superseded_by_version, created_at
FROM policy_module_versions_pre008;

CREATE INDEX IF NOT EXISTS idx_module_versions_module ON policy_module_versions(module_id);
CREATE INDEX IF NOT EXISTS idx_module_versions_state ON policy_module_versions(state);

-- Per-phase lifecycle scripts (replaces the fixed enforce_script/
-- validate_script/rollback_script columns). `seq` orders multiple
-- scripts within the same phase (v1 always writes seq=0 via
-- SetScript's upsert; the column exists so a future multi-file phase
-- doesn't need another migration).
CREATE TABLE IF NOT EXISTS policy_module_scripts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version_id INTEGER NOT NULL REFERENCES policy_module_versions(id) ON DELETE CASCADE,
    phase TEXT NOT NULL CHECK(phase IN ('apply', 'disable', 'uninstall', 'validate', 'report')),
    filename TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL DEFAULT 0,
    UNIQUE(version_id, phase, seq)
);

CREATE INDEX IF NOT EXISTS idx_module_scripts_version ON policy_module_scripts(version_id);

-- Best-effort preservation of the old fixed-column scripts as rows in
-- the new table, in case a database somehow has pre-008 rows (see
-- header). enforce_script was NOT NULL in the old schema so every old
-- row has one; validate_script/rollback_script were nullable.
INSERT INTO policy_module_scripts (version_id, phase, filename, source, seq)
SELECT id, 'apply', 'enforce.sh', enforce_script, 0
FROM policy_module_versions_pre008
WHERE enforce_script IS NOT NULL AND enforce_script != '';

INSERT INTO policy_module_scripts (version_id, phase, filename, source, seq)
SELECT id, 'validate', 'validate.wasm', validate_script, 0
FROM policy_module_versions_pre008
WHERE validate_script IS NOT NULL AND validate_script != '';

INSERT INTO policy_module_scripts (version_id, phase, filename, source, seq)
SELECT id, 'uninstall', 'rollback.sh', rollback_script, 0
FROM policy_module_versions_pre008
WHERE rollback_script IS NOT NULL AND rollback_script != '';

DROP TABLE policy_module_versions_pre008;
