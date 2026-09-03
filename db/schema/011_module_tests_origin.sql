-- Migration 011: module version tests, standardized test expectations,
-- script references, and module origin.
--
-- Spec: docs/history/specs/2026-07-17-modular-module-system-design.md
--
-- 1. script_ref on both existing condition tables. A script-kind
--    condition may now reference a library script by stable id instead
--    of carrying inline source. Exactly one of script_source and
--    script_ref is non-empty for kind=script; enforced in Go
--    (pkg/services validateConditionPayload), not a CHECK, per the
--    established convention for post-001 columns (SQLite ALTER TABLE
--    cannot add CHECK constraints).
--
-- 2. Legacy script_expect rewrite. The bespoke script_expect JSON
--    ({"exit_code":..,"output_equals":..}) is superseded by the
--    standardized operator/value_json expectation every condition kind
--    now shares (INV-TEST: subject, operator, expected value). Existing
--    script-kind rows are rewritten below: output_equals present maps
--    to operator=equals with value_json=[output]; otherwise the row
--    maps to operator=exists with value_json=[] (pass = script ran and
--    exited zero). The script_expect column is retained but is dead
--    from this migration on: nothing writes it and nothing reads it.
--
-- 3. module_version_conditions: per-version module tests. Column parity
--    with dependency_group_conditions (004 base + 006 widening + this
--    migration): kind, param_path, operator, value_json, script_source,
--    script_ref, script_expect, seq -- keyed by version_id instead of
--    group_id. Version-scoped so tests travel with the version on
--    export and freeze on publish (draft-guarded writes in
--    pkg/services/policymodules.go). Same nine domain columns, same
--    eval engine (catalog/dependencygroups/eval.go), same condition
--    builder popup -- one rule system across the product (INV-CB).
--    script_expect exists here for column parity only; it is never
--    written (see point 2).
--
-- 4. conditions_match_mode on policy_module_versions: all | any,
--    mirroring dependency_groups.match_mode. Enforced in Go.
--
-- 5. origin on policy_modules: bundled | tenant | imported. Backfilled
--    from is_bundled. is_bundled is retained (existing queries key on
--    it); origin makes the imported source representable (closes the
--    known gap recorded in catalog/policymodules/extension_adapter.go).
--
-- Contains no PRAGMA, so it runs inside the shared migration tracker
-- transaction; a crash mid-migration rolls back cleanly.

ALTER TABLE dependency_group_conditions ADD COLUMN script_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE group_membership_rules ADD COLUMN script_ref TEXT NOT NULL DEFAULT '';

-- Rewrite legacy script expectations into the standardized
-- operator/value shape. Guarded by json_valid so a malformed or empty
-- script_expect (never produced by the validated write path, but cheap
-- to guard) falls into the exists mapping instead of erroring.
UPDATE dependency_group_conditions
SET operator = 'equals',
    value_json = json_array(json_extract(script_expect, '$.output_equals'))
WHERE kind = 'script'
  AND json_valid(script_expect)
  AND json_extract(script_expect, '$.output_equals') IS NOT NULL;

UPDATE dependency_group_conditions
SET operator = 'exists',
    value_json = '[]'
WHERE kind = 'script'
  AND (NOT json_valid(script_expect)
       OR json_extract(script_expect, '$.output_equals') IS NULL);

UPDATE group_membership_rules
SET operator = 'equals',
    value_json = json_array(json_extract(script_expect, '$.output_equals'))
WHERE kind = 'script'
  AND json_valid(script_expect)
  AND json_extract(script_expect, '$.output_equals') IS NOT NULL;

UPDATE group_membership_rules
SET operator = 'exists',
    value_json = '[]'
WHERE kind = 'script'
  AND (NOT json_valid(script_expect)
       OR json_extract(script_expect, '$.output_equals') IS NULL);

-- Per-version module tests (applicability queries). See header point 3.
CREATE TABLE IF NOT EXISTS module_version_conditions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version_id INTEGER NOT NULL REFERENCES policy_module_versions(id) ON DELETE CASCADE,
    kind TEXT NOT NULL DEFAULT 'param',
    param_path TEXT NOT NULL DEFAULT '',
    operator TEXT NOT NULL DEFAULT '',
    value_json TEXT NOT NULL DEFAULT '[]',
    script_source TEXT NOT NULL DEFAULT '',
    script_ref TEXT NOT NULL DEFAULT '',
    script_expect TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL DEFAULT 0,
    UNIQUE(version_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_module_version_conditions_version ON module_version_conditions(version_id);

ALTER TABLE policy_module_versions ADD COLUMN conditions_match_mode TEXT NOT NULL DEFAULT 'all';

ALTER TABLE policy_modules ADD COLUMN origin TEXT NOT NULL DEFAULT '';

UPDATE policy_modules
SET origin = CASE WHEN is_bundled THEN 'bundled' ELSE 'tenant' END
WHERE origin = '';
