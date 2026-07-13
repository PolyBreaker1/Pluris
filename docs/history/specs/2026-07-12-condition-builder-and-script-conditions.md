# Condition Builder & Script Conditions — Design Record

**Date:** 2026-07-12
**Author:** Claude (Opus 4.8) + Peter (owner)
**Status:** Shipped
**Builds on:** `catalog/dependencygroups` (pre-existing evaluator), `docs/history/specs/2026-07-06-dependency-groups-design.md`.

---

## Goal

Dependency Group conditions were originally authored through a flat add-form (Task 1.3-era) with a hardcoded operator/param picker. This record documents the reusable `ConditionBuilderDialog` that replaced it, the widened operator set, the script-condition kind + its agent execution contract, and — critically — that the SAME dialog and eval engine now also drive dynamic Group membership rules (migration 009, see the dynamic-groups spec), making this the one rule-authoring UI in the console (INV-CB).

## Operator set

Widened from the original `in`/`not_in`/`exists` to the full type-appropriate set: `in`, `not_in`, `exists`, `equals`, `not_equals`, `contains`, `not_contains`, `starts_with`, `ends_with`, `gt`, `gte`, `lt`, `lte`, `matches` (regex). Validated server-side by the pre-existing `validConditionOperator` in `pkg/services/dependencygroups.go` (unexported, called directly by both `DependencyGroupService.AddCondition` and the new `GroupService.AddRule` since both live in package `services` — no export/duplication needed).

## The reusable dialog contract

`web/templates/condition_builder.templ` (`@ConditionBuilderDialog()`) + `web/static/condition-builder.js`. Mounted once per page.

- **Open**: `data-condition-builder-open` on a trigger button.
- **Prefill (edit flow)**: `data-cb-prefill` carries HTML-escaped JSON on the row's Edit control; `data-cb-cond-id` carries the condition's row id so the page's own click listener can stash it for the save handler. There is no update route — edit is implemented as remove-then-add, orchestrated by page JS, documented as non-atomic: if the add half fails after a successful remove, the page reloads with a stashed `sessionStorage` error rather than attempting a rollback it can't safely perform (the removed row's id/seq isn't re-materializable).
- **Save**: dispatches `condition:save` on `document` with the built condition payload; the mounting page's own listener does the actual `POST`.
- **Values encoding**: repeated `values=` form keys, one per array element (`URLSearchParams.append('values', v)` per element on the JS side; the handler reads `c.FormParams()["values"]`, an `url.Values`). Chosen over comma-joining so a value containing a literal comma (realistic for `contains`/`matches`) round-trips exactly. A condition with no values (e.g. `exists`) persists `[]`, not `[""]`.
- **Match mode**: `all` (AND) / `any` (OR), a per-group setting rendered as a `<select>` in the owning tab's header (not buried in a General section — the reader deciding AND vs OR is looking at the condition list, not a settings page). Builtin/protected groups render the control `disabled` with an explanatory `title`.
- **Script tab**: mounts a `PlurisCodeEditor` (see [[layout-system]]) for the script source, wired for `{{` completion where applicable (module editor context only — the condition builder's own script tab is plain bash with no param-completion source).

## Script-condition kind + agent execution contract

A condition's `kind` is `param` (the original path/operator/values shape) or `script`. A script condition carries:

- **`script_source`** — a sandboxed shell script (bash), executed by an agent, never by the console.
- **`script_expect`** — a small JSON object: `{ "exit_code": <int>, "output_equals"?: <string> }`. Pass = the script's actual exit code matches AND (if `output_equals` is set) stdout matches exactly.
- **Console-side verdict is always `"unknown"` until an agent reports back** via `script_result/<condition-id>` — there is no agent in this repo yet (see [[handoff]]), so every script condition evaluates to `unknown` in the console today, by design, not as a bug. `catalog/dependencygroups/eval.go`'s `evalCondition` treats `unknown` as neither pass nor fail (never a false positive from an unreported script).
- Validated server-side by `validateScriptExpect` (unexported, shared the same way `validConditionOperator` is): empty `script_source` rejected; malformed `script_expect` JSON rejected.

## Match mode + group-level semantics

`dependencygroups.MatchMode` (`all` | `any`) combines a group's conditions. `EvalGroup(g Group, facts map[string]string) string` (newly exported from the previously-private `evalGroup`) is the one entry point both dependency-group eligibility (`Eligible`) and dynamic group membership (`EvaluateDynamicMembership`) call — same function, same semantics, no fork. A group with zero conditions evaluates to a vacuous pass for `all` (documented pre-existing behavior, inherited — see the dynamic-groups spec for why this matters for group membership specifically and the guard that was added there).

## One rule system: dependency groups AND dynamic group membership

`group_membership_rules` (migration 009) is schema-parity with `dependency_group_conditions` (same 9 domain columns: `id, group_id, kind, param_path, operator, value_json, script_source, script_expect, seq` — column order differs cosmetically because 009 is a fresh `CREATE TABLE` vs. 006's `ALTER TABLE`-appended `kind`, no semantic difference). Both condition tables are authored through the identical `ConditionBuilderDialog`/`condition-builder.js`, validated through the identical `validConditionOperator`/`validateScriptExpect`, and evaluated through the identical `EvalGroup`. This is what INV-CB in [[invariants]] locks down: one condition data model, one operator set, one eval path, used by two features.

## Tests

`console/handlers/dependency_groups_conditionbuilder_test.go` — repeated-values encoding (incl. a literal-comma value), script-kind add via form + empty-source 400. `console/handlers/dependency_groups_test.go`'s `TestDependencyGroupDetailMountsConditionBuilder` — dialog mounted, old flat-form markup gone, human-readable condition-row summaries, escaped `data-cb-prefill`. `pkg/services/group_rules_test.go` — rule CRUD validation matrix reusing the same sentinel errors as dependency-group conditions.
