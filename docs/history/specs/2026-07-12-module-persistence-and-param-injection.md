# Policy Module Persistence & Parameter Injection — Design Record

**Date:** 2026-07-12
**Author:** Claude (Opus 4.8) + Peter (owner)
**Status:** Shipped
**Builds on:** `catalog/policymodules` (pre-existing domain types), ADR-006/ADR-007 ([[decisions]]).

---

## Goal

Before this work, `catalog/policymodules` served an in-memory mock (`AllModules()`, a hardcoded literal slice) — the `policy_modules`/`policy_module_versions` tables existed in schema but no service populated them. This record documents migration 008's schema reconciliation, the `pkg/services/policymodules.go` state machine that made modules real, `SeedBundled`, the module detail/create editor page, the `{{ param "<path>" }}` injection syntax, and the decision to delete the Custom Policy Wizard rather than build it against the new persistence.

## Schema reconciliation — migration 008

`db/schema/008_module_scripts.sql` recreated `policy_module_versions` (rename→create→copy-forward→drop; needed regardless of SQLite version because the semantics changed, not just a column removal):

- **Dropped** `runtime` — now derived per-phase in Go (`LifecyclePhase.Runtime()`: apply/disable/uninstall → bash, validate/report → wasm; per ADR-007 INV-M5's frozen bash/WASM split). Never stored, since it's a pure function of the phase.
- **Dropped** `enforce_script`/`validate_script`/`rollback_script` (one column each, i.e. one script per phase) — replaced by a child table **`policy_module_scripts`** (`id, version_id FK CASCADE, phase CHECK(...), filename, source, seq, UNIQUE(version_id, phase, seq)`), allowing multiple scripts per phase.
- **Added** `sandbox_profile TEXT DEFAULT '{}'` (JSON `SandboxProfile`: `FsRead`/`FsWrite` allow-lists, `NetEgress`, `User`) and `report_schema TEXT DEFAULT ''` (JSON Schema for the `report` phase's structured output).
- **`manifest_yaml` kept but repurposed** — it is now a **derived export artifact**, `DEFAULT ''`, not the source of truth. The structured columns (`parameters_schema`, `sandbox_profile`, `satisfies`, `depends_on`/`conflicts`, and the `policy_module_scripts` rows) are authoritative; a manifest YAML can be regenerated from them on demand but nothing reads it back as input.
- Pre-flight verified (grep across the tree, excluding the query/schema/generated layers themselves) that no live consumer touched the dropped columns before making the change.

## Service — `pkg/services/policymodules.go`

`PolicyModuleService`: module CRUD (tenant-scoped, `ErrModuleReferenced` guard on delete via reference counts against dependency-group links and configuration-group bindings), version lifecycle (`CreateDraft`, `ForkLatestPublished`, `UpdateDraft` — draft-only, `SetScript` — draft-only), and the publish/supersede/revoke state machine.

**State machine, made atomic after a review round:**
- `Publish(versionID, publishedBy)` runs both state transitions — publish the target version, supersede the module's prior published version — inside ONE transaction. The publish UPDATE is `WHERE id=@id AND state='draft'` as `:execrows`; 0 rows affected (lost a race, or wasn't a draft) → `ErrVersionNotDraft`, rollback, nothing touched. The supersede half is a single guarded statement (`SupersedeCurrentPublishedVersion`, not a read-then-write), so a crash mid-transaction can never leave zero published versions where there was one, nor two published at once. `ErrPublishRequiresApplyScript` (INV-M3) blocks publish without an apply-phase script.
- `UpdateDraft`/`SetScript` both use `WHERE ... AND state='draft'` guarded UPDATEs (not read-then-write), mapping 0-rows-affected-but-row-exists to `ErrVersionNotDraft` — closes the race where a concurrent publish could otherwise land between a read and a write.
- `Revoke` is state-guarded (`published`/`superseded` only via `:execrows`); `ErrRevokeInvalidState` on a draft or an already-revoked version.
- Concurrency proven under `-race`: two goroutines publishing two drafts of the same module concurrently each resolve to either success or `ErrVersionNotDraft`, and the end state always has exactly one published version (`TestPublish_ConcurrentPublishesYieldOnePublished`).

## Seeding — `SeedBundled`

Idempotent and race-tolerant (mirrors the existing `EnsureBuiltins`/`isUniqueErr` pattern used by Dependency Groups). Runs once at boot (`console/server/server.go`'s `NewWithDB`, log-and-continue on error) and again best-effort per tenant read (Modules Library/Defaults pages) in case boot-time seeding raced or hadn't happened yet for a newly-provisioned tenant. The bundled catalog is exposed to package-internal pure helpers (`CandidatesForPolicy`, the extension framework's `Loader`) via `catalog/policymodules/catalog.go`'s `SetCatalogProvider`/`Catalog()` provider hook — this is deliberately tenant-agnostic (bundled modules only), since those call sites run outside any request's tenant context.

## Module detail/create editor

`DetailShell` page (`web/templates/policy_module_editor.templ`, `/policy/modules/new` + `/policy/modules/:id`) — General (metadata), per-phase Scripts tabs (apply/disable/uninstall/validate/report, `PlurisCodeEditor`-mounted), Parameters (structured JSON-Schema row editor: key/label/type/default/required/enum), and Dependencies (reuses the existing `moduleDepsDetails` component, not a fork).

- **Save mechanism**: module metadata → `POST /api/modules/:id/fields`; every version-scoped field (target_os/scope/satisfies/sandbox/parameters_schema/depends_on/conflicts) → ONE shared endpoint `POST /api/modules/:id/versions/:vid/fields`, both mirroring the existing `field_api.go` `{section, fields}` contract exactly; scripts get their own dedicated `POST /api/modules/:id/versions/:vid/scripts` (script bodies aren't simple key/value fields). All three enforce the target version is a draft before applying anything.
- **Published versions render fully read-only** (a visible immutable banner + `readonly` attributes on every script/field control) — matches the service-level guard, not just a UI courtesy.
- Repeatable-value fields (target_os, satisfies, sandbox fs_read/fs_write/net_egress, depends_on, conflicts) are v1 comma-joined text inputs, not real multi-pickers — an accepted v1 shortcut, not a gap in the persistence model.

## `{{ param "<path>" }}` injection syntax

A lifecycle script authors a parameter reference as `{{ param "<canonical-path>" }}` (entity paths like `computer/identity/name`, or module-input paths like `module/input/timeout`). The module editor's script tab offers a parameter tree (fed by `/api/params?module_id=<urn>`, see the parameter-registry spec) with **click-to-insert** at the CodeMirror cursor (verified end-to-end: inserted token round-trips through save/reload) and **drag-and-drop** (implemented, not exercised in a live browser this pass — flagged as a follow-up manual check, not a known bug). CodeMirror's `completionSource` also offers the same token as you type `{{`.

**Who resolves it: the agent, at execution time — never the console.** The console only stores and displays the unresolved `{{ param "..." }}` template; there is no agent in this repo yet (see [[handoff]]), so no script's `{{ param }}` tokens are ever substituted today. This mirrors the script-condition contract in the condition-builder spec: the console authors and stores instructions for an agent that doesn't exist yet, and is explicit about that boundary rather than half-implementing a fake resolution path.

## `manifest_yaml`-as-derived decision

Rather than keep `manifest_yaml` as the editable source (which would require round-tripping every structured field through YAML parse/serialize on every save, and would reintroduce a second source of truth alongside the structured columns), it was demoted to a derived export artifact. Rationale: the module editor's UI is entirely structured-field-driven (script textareas, a JSON-Schema row editor, comma-list inputs) — there was never a YAML-editing surface to begin with, so keeping `manifest_yaml` as truth would have meant round-tripping through a format nothing in the UI actually edits.

## `custom_policies.parameters_schema` — dead schema

`custom_policies.parameters_schema` (`db/schema/001_initial.sql`) has zero Go call sites for `CreateCustomPolicy`/`UpdateCustomPolicy` outside the sqlc-generated query layer itself — no service, handler, or seed script ever wrote or read through it. It was the deleted Custom Policy Wizard's intended persistence target (see below) and died with the wizard's stub save path. **Left in place** — dropping it is a migration, not worth it for an unreachable column — but documented here so it isn't mistaken for live schema.

## Custom Policy Wizard: deleted, not persisted-to

Evidence for deletion over "wire it to the new persistence": `web/templates/menu.go`'s `customPolicyWizardScript` built its "Sign & publish" payload and dispatched `cpw:save` as a `CustomEvent` — a repo-wide grep for `cpw:save` found exactly that one dispatch and **zero listeners**. Nothing the wizard produced was ever persisted, at any point in its history; its steps wrote to in-memory form state only. Per the decision procedure ("a stub UI producing nothing gets deleted, not migrated"), it was removed rather than pointed at the new module-persistence layer. Both of its entry points ("New custom policy" on the Policy Catalog toolbar, "New module from policy wizard" on the Modules Library toolbar) now link directly to `/policy/modules/new` — the module editor is the canonical authoring surface (INV-U "one canonical editor," INV-PK).

## Tests

`pkg/services/policymodules_test.go` — version/script round-trip, invalid-phase rejection, draft-only mutation guards (`UpdateDraft`, `SetScript`), publish-requires-apply-script, publish-supersedes-prior, concurrent-publish race safety, revoke state guards, `SeedBundled` idempotency + parity with the former mock's fixture module, delete-blocked-when-referenced. `console/handlers/policy_module_editor_test.go` — create→detail round trip, metadata/parameters-schema field updates, draft-guarded script save (including the published-rejected case end to end), publish-without-apply-script surfaces as a redirect+warning not a 500, clone bundled→tenant, delete blocked/allowed, cross-tenant 404. `web/templates/policy_module_editor_test.go` — script mount order (vendor→wrapper→page), param-tree container present, one tab per `AllLifecyclePhases` entry, published-immutable banner.
