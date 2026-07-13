# Endpoint Policy

**What:** Configuration pushed to managed **devices** — the policy catalog, configuration groups, policy modules, dependency groups, and assignment resolution that will eventually drive an agent.

**Related:** [[authorization]] (see disambiguation box below), [[parameters]], [[endpoint-management]]

> **Naming callout — two "policy" systems.** This document is about policy **applied to devices** (password length, firewall state, SSH hardening — the Windows Group Policy analogue). [[authorization]] ("Pluris Policy") is a completely different system: who can view/edit things **in the console itself**. They share the word "policy" and nothing else — a `console_access.*` grant does not affect what a computer receives, and an `endpoint_policy.*` grant does not affect who can log into the console.

## Concept map

```
Catalog Policy (catalog/policies)
      │  "what setting exists" — GP-equivalent name/path, description, Linux mechanism hint
      ▼
Configuration Group Binding (catalog/configgroups)
      │  "this policy, with these values, in this group"
      ▼
Assignment (configuration_group_assignments)
      │  "this group applies to this target" — asset | group | site | tenant
      ▼
Target (asset or identity)
```

A **Policy Module** (`catalog/policymodules`) is the thing that actually *implements* a catalog policy on a device — apply/disable/uninstall/validate/report scripts, versioned and (eventually) signed. A **Dependency Group** (`catalog/dependencygroups`) gates whether a module is even applicable to a given device — the closest analogue to a Windows GP **WMI filter**: an AND-set of conditions over device facts that must pass before the module's platform/requirement links are satisfied.

## Windows GP compatibility approach

Each `policies.Policy` catalog entry carries three GP-parity fields alongside its Linux description, so an AD-migrating admin can find the same lever in the same place:

- `WinGPName` — the exact Windows GP setting name (used for fuzzy-match during a future GP import).
- `WinGPPath` — the GP tree location, pipe-separated, rooted at "Computer Configuration"/"User Configuration" (kept with the redundant "Policies"/"Security Settings" rungs Windows uses, for 1:1 import targeting and admin muscle memory).
- `LinuxImpl` — a one-line pointer at the Linux mechanism(s) that will enforce it (e.g. `pam_pwquality minlen= / /etc/security/pwquality.conf`), not the implementation itself.

Example entry (`catalog/policies/catalog_computer.go`):

```go
{
    ID:          "sec.account.password.min-length",
    Name:        "Minimum password length",
    WinGPName:   "Minimum password length",
    WinGPPath:   "Computer Configuration|Policies|Windows Settings|Security Settings|Account Policies|Password Policy",
    Category:    []string{"Computer Configuration", "Security Settings", "Account Policies", "Password Policy"},
    Scope:       ScopeComputer,
    Description: "...pam_pwquality / pam_passwdqc...",
    LinuxImpl:   "pam_pwquality minlen= / /etc/security/pwquality.conf",
}
```

`Category` mirrors `WinGPPath` (trimmed) and drives `BuildTree()`'s sidebar navigator, so the Pluris category tree visually matches the GP Editor tree. Bundled entries (`computer`/`user` catalogs) are read-only in code; `Custom: true` tenant-authored entries render in the same list, distinguished by a chip — never a separate tab (INV-M10). **There is no tenant custom-policy authoring UI today** — the former Custom Policy Wizard was a pure stub (its save event had zero listeners anywhere in the codebase; nothing it produced was ever persisted) and was deleted. `custom_policies.parameters_schema` is consequently dead schema (documented in `docs/history/specs/2026-07-12-module-persistence-and-param-injection.md`). The module editor (`/policy/modules/new`) is the canonical authoring surface for policy MODULES; authoring a new Catalog Policy entry itself would need a future editor of its own.

The GP/ADMX translation-layer research (condensed from a now-deleted document, full text in git history) is the original research/architecture spec for the full ADMX/ADML translation-layer vision (parsing Windows policy definition files to auto-generate forms, a curated GP→Linux translation map for PAM/firewalld/udev/etc.) — it is **not authoritative on UI/IA** (superseded by [[invariants]] and [[decisions]] ADR-004 for anything editor-shaped); treat it as background on the mission, not a spec for current code.

## What is DB-backed vs in-memory mock today

| Layer | Package | Status |
|---|---|---|
| Catalog Policy (the vocabulary) | `catalog/policies` | In-code Go literals (`computerPolicies`, `userPolicies`) + `customPolicies` — no DB table backs the bundled catalog. `custom_policies` (the table `customPolicies` entries would persist to) is dead schema: no service reads or writes it. |
| Configuration Groups + Bindings + Assignments | `pkg/services/configgroups.go` (service) + `catalog/configgroups` (pure domain types) | **Real, DB-backed** (shipped in the 2026-07 overhaul). `ConfigGroupService` does full CRUD against `configuration_groups`/`configuration_group_bindings`/`configuration_group_assignments` (`db/schema/001_initial.sql`). The `/policy/groups` list/create/detail pages (`web/templates/config_groups.templ`) replaced the retired popup dialog; `catalog/configgroups/mock.go` was deleted along with it. `pkg/services/assignments.go`'s `AssignmentService.ResolveForTarget` now reads real, populated data. See `docs/history/specs/2026-07-06-dependency-groups-design.md` (the pattern this followed) and [[handoff]]. |
| Policy Modules | `pkg/services/policymodules.go` (service) + `catalog/policymodules` (domain types + `Catalog()` provider) | **Real, DB-backed** (shipped in the 2026-07 overhaul). `PolicyModuleService` implements the draft/publish/supersede/revoke state machine against `policy_modules`/`policy_module_versions`/`policy_module_scripts` (migration 008); bundled modules are seeded at boot (`SeedBundled`). `catalog/policymodules.Catalog()` is fed by the service via `SetCatalogProvider`, not a hardcoded literal — the former `mock.go`/`AllModules()` are gone. Module detail/create editor at `/policy/modules/:id` / `/policy/modules/new`. See `docs/history/specs/2026-07-12-module-persistence-and-param-injection.md`. |
| Dependency Groups | `catalog/dependencygroups` (pure model) + `pkg/services/dependencygroups.go` | **Real, DB-backed.** `DependencyGroupService` does full CRUD against `dependency_groups`/`dependency_group_conditions`/`module_dependency_links` tables (migration `004_dependency_groups.sql`), including builtin seeding (`EnsureBuiltins`) and evaluation (`Evaluate`). Conditions are now authored through the shared `ConditionBuilderDialog` (migration 006) — see `docs/history/specs/2026-07-12-condition-builder-and-script-conditions.md`. |
| Assignment resolution | `pkg/services/assignments.go` | Real code, reads real tables — and `configuration_group*` tables are now actually populated by `ConfigGroupService`, so `ResolveForTarget` has live data to resolve against. |
| Groups (dynamic membership) | `pkg/services/group_rules.go` + `catalog/dependencygroups` (reused eval engine) | **Real, DB-backed** (migration 009, shipped in the 2026-07 overhaul). Not part of this document's original scope (it's an identity/asset directory feature, not a device-policy one) but listed here because it reuses this document's Dependency Group condition/eval machinery wholesale — see `docs/history/specs/2026-07-12-dynamic-groups.md` and [[identity-assets]]. |

## Dependency groups — the evaluator

A `dependencygroups.Group` is an AND-set of `Condition`s over canonical parameter paths (`computer/hardware/os_package_family`, see [[parameters]]); matching uses only the path's trailing key against a device's reported facts. Three operators: `in`, `not_in`, `exists`. A fact absent from the device's report is always `unknown` — never a false pass or fail (`evalCondition`, `catalog/dependencygroups/eval.go`).

A module links to groups in one of two roles (`module_dependency_links.role`):

- **`platform`** — match-ANY (none linked = platform-agnostic pass). The WMI-filter analogue: "this module only makes sense on these OS families."
- **`requirement`** — match-ALL. Extra preconditions beyond platform (e.g. "disk encryption active").

`Eligible(links, groups, facts)` aggregates both link roles into one `Result{Status, Platforms, Requirements}` where `Status` is `eligible` / `ineligible` / `unknown`: any definitive `fail` on either aggregate makes the whole thing `ineligible`; both aggregates `pass` makes it `eligible`; anything else (a group needing agent inventory that hasn't reported) is `unknown` — never guessed.

Builtin dependency groups (`pkg/services/dependencygroups.go`'s `builtinGroups`) ship OS-family and disk-encryption conditions (`rpm-based`, `debian-based`, `arch-based`, `any-linux`, `windows`, `disk-encryption-active`, `bitlocker`, `luks`) — editable per-tenant, not deletable (`ErrBuiltinProtected`). `builtinModuleLinks` seeds one default link: the bundled `pluris.sshd.password-auth-disable` module against the `any-linux` platform group.

## Policy Module vocabulary (ADR-006/ADR-007, `catalog/policymodules/types.go`)

- **Module** — a versioned, signed package; one module ID, many immutable `ModuleVersion`s. Editing in the UI produces a new version — the old one stays for rollback and pinned assets.
- **Lifecycle phases** — `apply` (mandatory, idempotent, bash), `disable` (idempotent + reversible, bash), `uninstall` (runs only at refcount zero, bash), `validate` (pure/read-only drift check, WASM sandbox), `report` (pure/read-only structured data, WASM sandbox). The bash-vs-WASM split per phase is frozen by INV-M5.
- **Sandbox profile** — `FsRead`/`FsWrite` path allow-lists, `NetEgress` allow-list (empty = no network), and the `User` a script runs as (`root` / `$target_user` / `nobody`) — enforced by the agent regardless of script content (INV-M8, planned).
- **Signature** — Ed25519 over manifest + script hashes (INV-M7); mock carries only metadata today, real bytes land with the agent.
- **Refcount / `ModuleInstallation`** — one row per (asset, module version) actually present, kept alive by a non-empty `Reasons` set (a direct binding request, or a transitive dependency from another installation); `uninstall` only runs when the set empties (INV-M1..M4).

## Assignment resolution (`pkg/services/assignments.go`)

`AssignmentService.ResolveForTarget(tenantID, targetType, targetID, groupIDs, siteID)` walks every scope a target participates in — the target itself, each of its groups, its site (assets only), and the tenant — in that order, querying `ListAssignmentsByTarget` per scope and de-duplicating by binding ID so a policy bound once is never shown twice even when reachable through multiple scopes. Each resulting `AppliedPolicy` carries the policy's display name (looked up from `policies.Catalog()`, falling back to the raw URN for stale bindings), source group, scope, and a value summary with keys upgraded to their `catalog/params` labels when recognized.

## What enforcement will look like when the agent lands (planned)

None of the above is enforced on a real device today — there is no shipping Pluris agent, and no `{{ param "<path>" }}` module-input token or script-condition result is ever resolved/reported in the console. The designed path, per ADR-006/ADR-007 and this document's DB-backed table above (the console-side persistence for steps 1 is now done — the gap is entirely the agent itself):

1. ~~A `ConfigurationGroupService` persists bindings/assignments for real~~ — **done** (`pkg/services/configgroups.go`, replacing the old `configgroups.MockGroups`).
2. Agent check-in resolves applicable modules via `AssignmentService`-style scope walking, filters candidates through `DependencyGroupService.Evaluate` (already real) for eligibility, and picks a module per policy URN per the override order Binding-level pin → tenant default → Pluris bundled default (`configgroups.Binding.ModuleOverride` / `policymodules.TenantDefaults`).
3. The agent runs the chosen module's `apply` phase inside the declared `SandboxProfile`, resolving each `{{ param "<path>" }}` token against the target's resolved facts/module-input values, tracks the resulting `ModuleInstallation` row and refcount, and periodically re-runs `validate`/`report` phases.
4. GP import (parsing real ADMX/ADML files per the original GP-compatibility research, condensed from a now-deleted document — full text in git history) is explicitly a later increment — the catalog's `WinGPName`/`WinGPPath` fields exist today specifically to make that import a fuzzy-match problem rather than a from-scratch mapping exercise.

## Code pointers

- Catalog: `catalog/policies/types.go`, `catalog/policies/catalog_computer.go`, `catalog/policies/catalog_user.go`, `catalog/policies/catalog_custom.go`
- Configuration groups (domain types): `catalog/configgroups/types.go`, `catalog/configgroups/targets.go`
- Configuration groups (service, DB-backed): `pkg/services/configgroups.go`, `db/queries/configuration_groups.sql`, `db/schema/001_initial.sql`
- Policy modules (domain types + catalog provider): `catalog/policymodules/types.go`, `catalog/policymodules/catalog.go`, `catalog/policymodules/resolver.go`
- Policy modules (service, DB-backed): `pkg/services/policymodules.go`, `db/schema/008_module_scripts.sql`
- Policy modules (permissions): `pkg/authz/modules.go`, `db/schema/007_module_ownership_grants.sql`
- Dependency groups (model): `catalog/dependencygroups/types.go`, `catalog/dependencygroups/eval.go`
- Dependency groups (service, DB-backed): `pkg/services/dependencygroups.go`, `db/schema/004_dependency_groups.sql`
- Condition builder (shared by dependency groups + dynamic group membership): `web/templates/condition_builder.templ`, `web/static/condition-builder.js`
- Assignment resolution: `pkg/services/assignments.go`
