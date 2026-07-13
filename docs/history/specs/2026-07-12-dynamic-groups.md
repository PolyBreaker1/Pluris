# Dynamic Groups — Design Record

**Date:** 2026-07-12
**Author:** Claude (Opus 4.8) + Peter (owner)
**Status:** Shipped
**Builds on:** `docs/history/specs/2026-07-12-condition-builder-and-script-conditions.md` (the shared eval engine this reuses), `catalog/dependencygroups` (pre-existing evaluator).

---

## Goal

Groups previously supported only static, hand-picked membership. This record documents migration 009's extension of the group model to `member_kind`/`membership`/dynamic rules, the decision to reuse the Dependency Group condition/eval machinery wholesale rather than build a second rule system, `EvaluateDynamicMembership`'s reconciliation semantics (and the mass-add hazard that was found and fixed), and the deliberate no-scheduler-yet posture.

## Member-kind / membership model — migration 009

`db/schema/009_group_kinds_rules.sql`:

- `groups.description` (free text), `groups.member_kind` (`asset | identity | mixed`), `groups.membership` (`static | dynamic`), `groups.rules_match_mode` (`all | any`, reusing `dependencygroups.MatchMode`/`ErrInvalidMatchMode` directly).
- `group_memberships.source` (`direct | rule`) — distinguishes hand-added members from rule-reconciled ones on the same membership row shape.
- New table `group_membership_rules` — schema-parity with `dependency_group_conditions` (9 domain columns: `id, group_id, kind, param_path, operator, value_json, script_source, script_expect, seq`; see the condition-builder spec for the column-order note, which is cosmetic only).
- Two supporting indexes: `idx_group_membership_rules_group`, `idx_group_memberships_source`.

## One rule system, reused — not rebuilt

Dynamic group membership is authored through the SAME `ConditionBuilderDialog` and evaluated through the SAME `catalog/dependencygroups.EvalGroup` (newly exported from the previously-private `evalGroup`) that Dependency Group conditions use — see the condition-builder spec for the full contract. `GroupService.AddRule`/`RemoveRule` (`pkg/services/group_rules.go`) call the identical unexported `validConditionOperator`/`validateScriptExpect` validators `DependencyGroupService.AddCondition` uses (both in package `services`), returning the same sentinel errors, so the condition-builder popup's error-to-message mapping needed zero changes to serve a second feature.

## Fact-building — `FactsForAsset` / `FactsForIdentity`

New `pkg/services/facts.go`. Both key their returned `map[string]string` by the **trailing** canonical-path segment (matching `catalog/dependencygroups/eval.go`'s `paramKey`) — `FactsForAsset` covers every `subtype_payload` JSON field plus the common asset columns (`enrollment_state`, `lifecycle_state`, `vendor`, `location`, `human_id`, `agent_version`, `description`, `uuid`, `subtype`); `FactsForIdentity` covers every identity column mounted under `catalog/params`' `SchemaIdentity` (`PathEntity: "user"`, so paths are `user/<section>/<key>`).

This is the one fact-building path shared by dependency-group module-eligibility evaluation (`DependencyGroupService.EvaluateForAsset`, a thin new wrapper: build facts via `FactsForAsset`, then call the pre-existing, unchanged `Evaluate`) and dynamic group membership (`EvaluateDynamicMembership`). Note for future readers: `DependencyGroupService.Evaluate` had **zero callers anywhere** in the tree before this work — there was no pre-existing duplicated fact-building code to "extract"; `FactsForAsset`/`FactsForIdentity` were built fresh to serve both features from day one, closing the loop before a second, divergent copy could ever appear.

## `EvaluateDynamicMembership` — reconciliation semantics

`GroupService.EvaluateDynamicMembership(ctx, groupID) (DynamicMembershipResult, error)`:

1. Rejects static groups (`ErrGroupNotDynamic`).
2. Builds a `dependencygroups.Group{ Conditions, MatchMode }` from the group's `group_membership_rules` rows.
3. Walks tenant assets and/or identities per `member_kind` (asset/mixed → assets; identity/mixed → identities).
4. Evaluates each candidate via `EvalGroup` fed by `FactsForAsset`/`FactsForIdentity`.
5. Reconciles `source='rule'` membership rows against the pass set: adds missing passing candidates, removes stale (no-longer-passing) ones. **Never reads, writes, or deletes `source='direct'` rows** — direct membership is orthogonal to rule evaluation and always survives a recalculation untouched.
6. Script-kind rules evaluate to `"unknown"` absent an agent report (per the condition-builder spec's agent contract) — `unknown` is never treated as a pass.

**Trigger points**: `AddRule` and `RemoveRule` both call `EvaluateDynamicMembership` after mutating the rule set. There is also an explicit "Recalculate" action on the Rules tab that calls it directly at any time.

## Zero-rules hazard — found and fixed

`dependencygroups.evalGroupAll`'s vacuous-AND semantics ("no conditions = pass") is correct and unchanged for Dependency Groups — every module-eligibility group evaluation keeps this behavior. Applied naively to group *membership*, though, a dynamic group with zero rules would resolve every tenant asset/identity as a rule-sourced member, which is a dangerous default for something that controls policy targeting.

**What shipped**: a choke-point guard in `EvaluateDynamicMembership` itself — if the rule set is empty, the function does NOT build a `dependencygroups.Group` and run the vacuous-AND eval at all. Instead it delegates to `clearRuleSourcedMembers`, which lists and deletes every `source='rule'` row (leaving `source='direct'` rows untouched) so the rule-sourced set is correctly empty rather than mass-added. This is a group-membership-service-layer policy decision only — `catalog/dependencygroups`'s vacuous-AND semantics are unchanged, since Dependency Groups have no analogous "membership" concept to protect.

This closes a hazard that was live for one review round: `RemoveRule` of a dynamic group's LAST rule used to trigger a zero-rules evaluation that (before the choke-point guard) would mass-add every tenant candidate as a rule-sourced member. Regression test: `TestRemoveLastRuleClearsRuleSourcedMembers` (default `match_mode='all'`, the dangerous case — `'any'` is fail-closed and would have missed the bug) — asserts rule-sourced members clear to 0 and only the direct-member survivor remains, not a 3-asset mass-add.

The UI-level guard on top: the Rules tab shows a warning banner on any dynamic group with 0 rules, and the "Recalculate" button is `disabled` (with a tooltip) until at least 1 rule exists — plus a server-side 400 in the recalculate handler as defense-in-depth. Rationale: a zero-rule recalculation has no legitimate outcome (a confirm dialog would just be a footgun with a speed bump), so disabling outright is strictly safer.

## No-scheduler-yet

Dynamic membership recalculates ONLY on rule save/delete (automatic) or an explicit "Recalculate" click (manual) — there is no background scheduler that re-evaluates on a timer or in response to an asset/identity field change drifting a fact out of (or into) matching. This is a deliberate, documented deferral, not an oversight: a scheduled-refresh follow-up is listed in [[handoff]]'s next-planned-work, not scheduled.

## Group deletion + membership guards

`ErrGroupReferenced` (409) blocks deleting a group still targeted by a `configuration_group_assignments` row (that target reference is polymorphic with no FK, so a silent cascade would strand assignment rows — a typed guard was chosen over a manual cascade). `RemoveAssetMember`/`RemoveIdentityMember` refuse rule-sourced rows (`ErrMemberNotDirect`) — only direct members can be removed by hand; rule-sourced membership is only ever changed by rule evaluation.

## Sidebar + permission domain

New `group` permission domain (`catalog/permissions`): `view` (scoped), `create`, `update`, `delete`, `manage_members` (all unscoped). Template grants: super_admin/admin get everything; technician gets `view` + `manage_members` (parallels their existing `identity.assign_groups`/`asset.manage_groups`) but not create/update/delete (group *definition* changes parallel role/permission administration, admin-and-up); user gets none. One canonical `/groups` list surfaced twice in the sidebar (Users▸User Groups at `?kind=identity`, Assets▸Groups at `?kind=asset`) — see [[invariants]] §VI.

## Tests

`pkg/services/group_rules_test.go` — migration-applies-fresh, rule CRUD validation matrix (7 subtests), static-group rejection, fact-helper parity with hand-built maps, dynamic-membership evaluation (asset-rule pass, direct members survive, stale removed, mixed group, script-rule-zero-members-until-agent-reports), member-kind-conflict guard, and the zero-rules regression test above. `console/handlers/groups_pages_test.go` — list kind-views, create, member add/remove (kind mapping, tenant isolation, direct-only removal), rule lifecycle (add/match-mode/recalculate-counts/remove/static-400/zero-rules-400), field-update conflict, permission sweep (10 mutating routes × denied-grant session → 403), cross-tenant 404, delete-referenced guard, per-kind/membership markup (picker allowed-kinds, builder-mount-vs-static-explainer, source chips, zero-rules warning + disabled recalculate).
