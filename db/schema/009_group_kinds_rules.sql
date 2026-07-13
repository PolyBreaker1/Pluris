-- Migration 009: group member-kind typing + dynamic rule-based
-- membership. Extends the AD-style groups model (001/003/005) with:
--
--   1. description on groups (free text; groups had none before).
--   2. member_kind on groups: 'asset' | 'identity' | 'mixed'. Enforced in
--      Go (pkg/services/groups.go), not a CHECK, matching the
--      established convention for post-001 enum columns (SQLite's
--      ALTER TABLE cannot add CHECK constraints to an existing table --
--      see 006/007's header comments for the same reasoning).
--   3. membership on groups: 'static' | 'dynamic'. Static groups behave
--      exactly as before (direct add/remove only); dynamic groups are
--      reconciled by EvaluateDynamicMembership from group_membership_rules.
--   4. rules_match_mode on groups: 'all' | 'any', mirroring
--      dependency_groups.match_mode (006) -- the SAME condition
--      combination semantics, reused rather than reinvented.
--   5. source on group_memberships: 'direct' | 'rule'. Distinguishes
--      admin-added members from rule-computed ones so
--      EvaluateDynamicMembership can reconcile source='rule' rows
--      without ever touching source='direct' rows.
--   6. group_membership_rules: ONE condition per row, mirroring
--      dependency_group_conditions (004/006) column-for-column so both
--      tables are driven by the exact same eval engine
--      (catalog/dependencygroups/eval.go) and, later, the same
--      condition-builder popup. This is deliberate: one rule system
--      across the product, not a parallel implementation.
--
-- Column parity against dependency_group_conditions (004 base +
-- 006 widening: kind, script_source, script_expect):
--   dependency_group_conditions: id, group_id, param_path, operator,
--     value_json, seq, kind, script_source, script_expect
--   group_membership_rules:      id, group_id, kind, param_path,
--     operator, value_json, script_source, script_expect, seq
-- Same nine domain columns; the field ORDER differs (kind is declared
-- up front here, defaulted alongside the row's other columns) but that
-- is purely cosmetic -- SQL column order never matters to SELECT * or to
-- sqlc's named-field mapping. No other divergence: param_path/operator/
-- value_json/script_source/script_expect keep the identical types,
-- NOT NULL-ness and defaults as their dependency_group_conditions
-- counterparts.
--
-- Contains no PRAGMA, so it runs inside the shared migration-tracker
-- transaction like 004 onward.

ALTER TABLE groups ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE groups ADD COLUMN member_kind TEXT NOT NULL DEFAULT 'mixed';    -- 'asset'|'identity'|'mixed' (Go-enforced)
ALTER TABLE groups ADD COLUMN membership TEXT NOT NULL DEFAULT 'static';    -- 'static'|'dynamic' (Go-enforced)
ALTER TABLE groups ADD COLUMN rules_match_mode TEXT NOT NULL DEFAULT 'all'; -- 'all'|'any'

ALTER TABLE group_memberships ADD COLUMN source TEXT NOT NULL DEFAULT 'direct'; -- 'direct'|'rule'

CREATE TABLE IF NOT EXISTS group_membership_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    kind TEXT NOT NULL DEFAULT 'param',
    param_path TEXT NOT NULL DEFAULT '',
    operator TEXT NOT NULL DEFAULT '',
    value_json TEXT NOT NULL DEFAULT '[]',
    script_source TEXT NOT NULL DEFAULT '',
    script_expect TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_group_membership_rules_group ON group_membership_rules(group_id);
CREATE INDEX IF NOT EXISTS idx_group_memberships_source ON group_memberships(source);
