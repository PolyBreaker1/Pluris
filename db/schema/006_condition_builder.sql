-- Migration 006: condition builder widening (operators, script
-- conditions, group match mode). Widens dependency_group_conditions with
-- a kind discriminator (param | script) and script fields, and
-- dependency_groups with a match_mode (all | any). SQLite's ALTER TABLE
-- cannot add CHECK constraints, so the enum values are validated in the
-- Go service layer (pkg/services/dependencygroups.go), not here. Contains
-- no PRAGMA, so it runs inside the tracker transaction.

-- kind discriminates a param-path condition (existing behavior) from a
-- script condition, whose pass/fail comes from an agent-executed script
-- rather than a device fact lookup (see catalog/dependencygroups.Condition's
-- Kind field doc comment for the evaluation contract).
ALTER TABLE dependency_group_conditions ADD COLUMN kind TEXT NOT NULL DEFAULT 'param';

-- script_source and script_expect are only meaningful when kind='script'.
-- script_expect is a JSON object like {"exit_code":0,"output_equals":"..."}
-- describing what the agent-reported result must match to pass.
ALTER TABLE dependency_group_conditions ADD COLUMN script_source TEXT NOT NULL DEFAULT '';
ALTER TABLE dependency_group_conditions ADD COLUMN script_expect TEXT NOT NULL DEFAULT '';

-- match_mode controls how a group's conditions combine: 'all' (default,
-- existing AND semantics) or 'any' (OR semantics).
ALTER TABLE dependency_groups ADD COLUMN match_mode TEXT NOT NULL DEFAULT 'all';
