-- Migration 012: module scripts become first-class named rows, and
-- enforcement wiring becomes a separate policy_module_actions table.
--
-- Spec: docs/history/specs/2026-07-18-module-scripts-enforcement-redesign-design.md
-- Plan: docs/history/plans/2026-07-18-module-scripts-enforcement-redesign.md
--
-- Migration 008 keyed scripts by lifecycle PHASE
-- (apply/disable/uninstall/validate/report), one row per phase, with a
-- seq column reserved for a future multi-file phase that never
-- materialized. This redesign replaces that model:
--
--   1. Scripts are now named (name + language + origin), not
--      phase-keyed. A version can carry any number of scripts under
--      any name -- the fixed five-phase enum no longer bounds the
--      script set. language replaces the old phase-derived Runtime()
--      split (sh/powershell/python).
--   2. Enforcement wiring (which action fires what) moves to a new
--      policy_module_actions table. An action references a script by
--      name (kind='script', value=<script name>) or carries an inline
--      command (kind='command', value=<command text>). This decouples
--      "what scripts exist" from "what runs when".
--   3. origin (default|custom) distinguishes seeded defaults (never
--      mutated in place) from tenant-authored rows. The Go service
--      layer (pkg/services/policymodules.go) implements fork-on-edit
--      of a default: editing a default writes a custom row while the
--      pristine default survives under a reserved name suffix, so
--      both can coexist under the UNIQUE(version_id, name) constraint.
--
-- SQLite cannot cleanly drop or retype columns, so this follows the
-- established rename + create + copy-forward + drop pattern from
-- 008_module_scripts.sql.
--
-- Copy-forward mapping for existing policy_module_scripts rows: the
-- old phase becomes the new name verbatim (apply/disable/uninstall/
-- validate/report), language is 'sh' for the bash-run phases
-- (apply/disable/uninstall) and 'python' for the phases that used to
-- run in the WASM sandbox (validate/report) -- kept scripted rather
-- than dropped, since a real WASM/python runtime split is out of
-- scope for this migration. Every carried-forward script also gets a
-- same-named custom-origin action so no enforcement wiring is lost:
-- a module that previously ran its apply script now still runs it,
-- via an action pointing at the script by name.
--
-- Contains no PRAGMA, so it runs inside the shared migration tracker
-- transaction; a crash mid-migration rolls back cleanly and re-runs
-- from scratch next boot.

ALTER TABLE policy_module_scripts RENAME TO policy_module_scripts_pre012;

CREATE TABLE policy_module_scripts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version_id INTEGER NOT NULL REFERENCES policy_module_versions(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    language TEXT NOT NULL DEFAULT 'sh' CHECK(language IN ('sh','powershell','python')),
    source TEXT NOT NULL DEFAULT '',
    origin TEXT NOT NULL DEFAULT 'custom' CHECK(origin IN ('default','custom')),
    seq INTEGER NOT NULL DEFAULT 0,
    UNIQUE(version_id, name)
);

CREATE INDEX IF NOT EXISTS idx_module_scripts_version ON policy_module_scripts(version_id);

-- Copy-forward: phase becomes name; validate/report keep running as
-- scripted (python) rather than bash. Preserve id where the source
-- table ids do not collide across the rename boundary (SQLite keeps
-- rowids stable across INSERT ... SELECT when we pass id explicitly).
INSERT INTO policy_module_scripts (id, version_id, name, language, source, origin, seq)
SELECT
    id,
    version_id,
    phase,
    CASE WHEN phase IN ('validate', 'report') THEN 'python' ELSE 'sh' END,
    source,
    'custom',
    seq
FROM policy_module_scripts_pre012;

CREATE TABLE policy_module_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version_id INTEGER NOT NULL REFERENCES policy_module_versions(id) ON DELETE CASCADE,
    action_key TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'script' CHECK(kind IN ('command','script')),
    value TEXT NOT NULL DEFAULT '',
    origin TEXT NOT NULL DEFAULT 'custom' CHECK(origin IN ('default','custom')),
    seq INTEGER NOT NULL DEFAULT 0,
    UNIQUE(version_id, action_key)
);

CREATE INDEX IF NOT EXISTS idx_module_actions_version ON policy_module_actions(version_id);

-- Seed one action per carried-forward script so no enforcement wiring
-- is lost: the action key matches the script name and points at it.
INSERT INTO policy_module_actions (version_id, action_key, label, kind, value, origin, seq)
SELECT version_id, name, '', 'script', name, 'custom', seq
FROM policy_module_scripts;

DROP TABLE policy_module_scripts_pre012;
