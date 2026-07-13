# Policy Module Grants & Ownership — Design Record

**Date:** 2026-07-12
**Author:** Claude (Opus 4.8) + Peter (owner)
**Status:** Shipped
**Builds on:** `docs/history/specs/2026-07-12-module-persistence-and-param-injection.md` (the module persistence layer this grants model sits on top of).

---

## Goal

Policy modules needed per-module access control beyond the coarse `endpoint_policy.manage_modules` grant — a tenant author should be able to own a module and (eventually) share edit/view access with specific identities, groups, or roles without handing out blanket module-management rights. This record documents migration 007's ownership/grants model, the `ModuleCanView/Edit/Admin` decision matrix, the decision to reuse `module_grants` for Scripts rather than build a parallel table, and the known RoleIDs follow-up that must close before a grant-sharing UI ships.

## Migration 007 — ownership + grants schema

`db/schema/007_module_ownership_grants.sql`:

- `policy_modules.owner_identity_id` — nullable FK to `identities`; NULL means bundled/unowned.
- `module_grants` table — `(module_id, subject_type, subject_id, level, created_at)`, `UNIQUE(module_id, subject_type, subject_id)`, `ON DELETE CASCADE` from `policy_modules`. `subject_type` is `identity | group | role`. `level` is `view | edit | admin` (hierarchical — `admin` implies `edit` implies `view`).
- Header comment on the migration explicitly records the "no parallel `script_grants` table" decision (below).

## `ModuleCanView/Edit/Admin` — the decision matrix

`pkg/authz/modules.go`. `ModuleAccessInput{ IdentityID, GroupIDs, RoleIDs, Grants *auth.Grants, OwnerID *int64, IsBundled bool, ExplicitGrants []ModuleGrant }`.

| condition | view | edit | admin |
|---|---|---|---|
| super_admin bypass | yes | yes | yes (bundled included — documented break-glass, not the normal path) |
| `endpoint_policy.manage_modules`, tenant module | yes | yes | yes |
| `endpoint_policy.manage_modules`, bundled module | yes | no | no |
| owner (`OwnerID != nil && == IdentityID`) | yes | yes | yes (never true for a bundled module — `OwnerID` is always nil there) |
| explicit `module_grants` row, level ≥ view | yes | no | no |
| explicit `module_grants` row, level ≥ edit | yes | yes | no |
| explicit `module_grants` row, level = admin | yes | yes | yes |
| explicit grant on a **bundled** module | yes (capped at view) | no | no |
| `endpoint_policy.view`, bundled module | yes | no | no |
| `endpoint_policy.view`, tenant module | no | no | no (default-deny) |
| stranger | no | no | no |

**Bundled modules are categorically un-editable/un-admin-able** through `manage_modules`, ownership, or explicit grants — `ModuleCanEdit`/`ModuleCanAdmin` short-circuit to `false` for any `IsBundled` module immediately after the super_admin bypass check. The intended path for customizing a bundled module is clone-to-tenant (the module editor's "Clone into my tenant" action), not editing the bundled original.

Explicit-grant matching is hierarchical (`admin ⊇ edit ⊇ view` via a small rank map) and covers all three subject types: `identity` (direct `IdentityID` match), `group` (membership in caller-supplied `GroupIDs`), `role` (membership in caller-supplied `RoleIDs` — see the known gap below).

## Where this is enforced

Every module handler (`console/handlers/policy_module_editor.go`) builds a `ModuleAccessInput` via `moduleAccessInput` and checks the matching function: `PolicyModuleDetail` (GET) → `ModuleCanView` else 404; field/script/version-lifecycle mutations → `ModuleCanEdit` else 403; delete → `ModuleCanAdmin` else 403 (bundled modules can never satisfy this, by construction); clone → `ModuleCanView` of the SOURCE only (cloning, not editing, is the intended path for a bundled module). Cross-tenant modules 404 before any authz decision runs, mirroring the identity-resolution fail-closed pattern used elsewhere. The module editor's parameter tree is additionally filtered by the creating user's grants (via the parameter-registry's `FilterByGrants`, not a second ad hoc check).

Route-level gating layers a coarser check first: `/policy/modules/*` and `/api/modules/*` both require `endpoint_policy.manage_modules` at the route table (`pkg/auth/rbac.go`). This is a **known limitation, not fixed here**: a session with only an explicit `module_grants` row, or plain ownership without `manage_modules`, cannot reach the module editor UI at all today even though the finer matrix above would allow it — the coarse route gate blocks first. The matrix itself is correct and unit-tested (`pkg/authz/modules_test.go`); it's simply unreachable end-to-end for that narrower case without a route-table redesign (opening the route the way `/api/params` is open, and pushing all authorization into the handlers). Flagged as a follow-up, deliberately not attempted here to avoid destabilizing the existing Library/Defaults/Sources route gating.

## Scripts-reuse decision: no parallel `script_grants` table

A "script" in this codebase is an unpackaged module — the same `policy_modules`/`policy_module_versions`/`policy_module_scripts` tables and the same `PolicyModuleService` back both the Modules and (eventual) Scripts surfaces; there is no separate Script entity. Given that, `module_grants` is reused unchanged for script-level access control rather than standing up a parallel `script_grants` table with identical shape — one grants table, one `ModuleCanView/Edit/Admin` matrix, for both concepts. This decision is recorded in the migration 007 header comment specifically so a future task doesn't accidentally duplicate it.

## Known follow-up: `RoleIDs` is direct-roles-only

`ModuleAccessInput.RoleIDs` — as populated by `moduleAccessInput` in `console/handlers/module_authz.go` — covers only a session's **directly-assigned** roles, not roles inherited via group membership (RBAC v2's `group_roles` + `ListGroupRolesForIdentity`). A role-subject `module_grants` row therefore only matches a caller holding that role directly. **This must close before a per-module grant-sharing UI ships** — the same reasoning `pkg/authz.EffectiveGrants` already applies (direct ∪ group-inherited roles, both inheritance-resolved) needs to reach `moduleAccessInput` too, or a tenant admin sharing module access "with the Technician role" will silently miss anyone who holds Technician only via a group. Tracked here rather than fixed, since group-role plumbing for this specific call site wasn't in this task's scope.

## Tests

`pkg/authz/modules_test.go::TestModuleAccessMatrix` — 18 subtests covering every row of the matrix above (bypass, `manage_modules` on tenant/bundled, owner, stranger, `endpoint_policy.view` on bundled/tenant, explicit identity/group/role grants at each level, non-membership negatives, the bundled+explicit-grant view cap). `pkg/database/module_grants_test.go::TestModuleOwnershipAndGrantsRoundTrip` — owner set/get round-trip, grant create/list/upgrade-in-place/delete, and an `ON DELETE CASCADE` check (deleting a module removes its `module_grants` rows).
