# Modular Module System — Execution Plan

**Date:** 2026-07-17
**Spec:** `docs/history/specs/2026-07-17-modular-module-system-design.md` (approved)
**Rules:** work one task at a time, in order. Every task ends with `go build -buildvcs=false ./...` clean + `go test -buildvcs=false -count=1 ./...` green + `gofmt -l .` clean. `.templ` edits → `make gen`; `db/queries/*.sql` or new schema → `sqlc generate`. Never touch root `pluris.db*`. New dependency `gopkg.in/yaml.v3` is pre-approved (owner, 2026-07-17) — Task 6 only.

---

## Phase 1 — Data model + condition kinds

### Task 1.1 — Migration 011 + sqlc layer
- New `db/schema/011_module_tests_origin.sql` (plain-ASCII comments):
  1. `ALTER TABLE dependency_group_conditions ADD COLUMN script_ref TEXT NOT NULL DEFAULT ''`; same for `group_membership_rules`.
  2. `CREATE TABLE module_version_conditions` — columns: `id, version_id INTEGER NOT NULL REFERENCES policy_module_versions(id) ON DELETE CASCADE, kind TEXT NOT NULL DEFAULT 'param', param_path TEXT NOT NULL DEFAULT '', operator TEXT NOT NULL DEFAULT '', value_json TEXT NOT NULL DEFAULT '[]', script_source TEXT NOT NULL DEFAULT '', script_ref TEXT NOT NULL DEFAULT '', script_expect TEXT NOT NULL DEFAULT '', seq INTEGER NOT NULL` + `UNIQUE(version_id, seq)` index. (Parity vocabulary with the two existing condition tables; `script_expect` present for parity but never written.)
  3. `ALTER TABLE policy_module_versions ADD COLUMN conditions_match_mode TEXT NOT NULL DEFAULT 'all'`.
  4. `ALTER TABLE policy_modules ADD COLUMN origin TEXT NOT NULL DEFAULT ''` + `UPDATE policy_modules SET origin = CASE WHEN is_bundled THEN 'bundled' ELSE 'tenant' END`.
- Legacy `script_expect` rewrite (both existing condition tables): Go data migration alongside the schema migration per `pkg/database/database.go` conventions — `output_equals` present → `operator='equals'`, `value_json=[output]`; else → `operator='exists'`, `value_json=[]`. Stop writing `script_expect` anywhere.
- New queries in `db/queries/policy_modules.sql`: `ListVersionConditions`, `AddVersionCondition`, `UpdateVersionCondition`, `DeleteVersionCondition`, `MaxVersionConditionSeq`; `UpdateVersionConditionsMatchMode` (draft-guarded `:execrows`); `SetModuleOrigin`/read origin in module rows. Run `sqlc generate`.
- Tests (`pkg/database`): migration applies on fresh + existing DB fixture; `script_expect` rewrite covers both legacy shapes; CASCADE delete of version removes its conditions.

### Task 1.2 — Condition kind widening (services + eval)
- `catalog/dependencygroups/types.go`: add `KindCommand ConditionKind = "command"`; `Condition` gains `ScriptRef string`.
- `pkg/services`: replace `validateScriptExpect` with `validateConditionPayload` — kind ∈ {param, command, script}; command → non-empty `script_source`; script → exactly one of `script_source`/`script_ref`; operator valid for all kinds. Wire through `DependencyGroupService.AddCondition`, `GroupService.AddRule`, and (Task 2.1) module version conditions.
- `catalog/dependencygroups/eval.go`: `command` and `script` kinds read fact `script_result/<condition-id>` (stdout), compare via the standard operator switch; absent → `unknown`; agent-reported non-zero exit → `fail`. Reuse the existing operator evaluation — no second switch.
- Tests: eval matrix for command/script kinds (present/absent/exit-fail × operators incl. `contains`), validation matrix (both-empty and both-set script source/ref rejected).

### Task 1.3 — PolicyModuleService: version conditions
- `pkg/services/policymodules.go`: `ListVersionConditions`, `AddVersionCondition`, `UpdateVersionCondition` (remove-then-add NOT used server-side — real UPDATE), `RemoveVersionCondition`, `SetConditionsMatchMode` — ALL draft-guarded via state-guarded `:execrows` (`ErrVersionNotDraft`), same pattern as `SetScript`.
- Helper: marshal a version's conditions + match mode into a `dependencygroups.Group` for eval (used later by agent-facing code; unit-test the marshaling now).
- Tests: CRUD round-trip, seq assignment, draft guard on every mutator (published version rejected), match-mode guard, validation delegation.

## Phase 2 — Condition builder dialog (one dialog, three kinds)

### Task 2.1 — Dialog third tab + standardized script tab
- `web/templates/condition_builder.templ`: add **Bash command** tab (command input → operator select → value input); restructure **Script** tab to: script picker `<select>` (fed by `GET /api/scripts`) + "inline source" fallback (`PlurisCodeEditor` textarea, unchanged mount timing) → operator → value. Remove exit-code/output-equals inputs. Add `data-cb-allowed-kinds` support (comma-joined; default all).
- `web/static/condition-builder.js`: third tab logic, save-enable rules per kind, `scriptRef` in save detail + prefill, kind filtering from `data-cb-allowed-kinds`. Values encoding unchanged.
- New handler `GET /api/scripts` → `[]` (empty JSON array) with a doc comment naming it the Scripts-library feed contract; register in server.go next to `/api/params`.
- Existing consumers (dependency groups page, groups rules page): handler save paths accept `kind=command` + `script_ref` (they already POST generic condition payloads — extend form parsing only).
- Run `make gen`. Tests: template renders 3 tabs; allowed-kinds filtering; handler add of a command condition + a script_ref condition end-to-end on dependency groups; prefill round-trip including scriptRef.

### Task 2.2 — Legacy summaries + edit flow
- Human-readable row summaries for command/script kinds (`Bash · uname -r · contains · "3"`, `Script · custom.sh · contains · "example"`) in the shared summary helper used by dependency groups + group rules — extend, don't fork.
- Verify remove-then-add edit flow carries the new fields; sessionStorage error path unchanged.
- Tests: summary rendering for all three kinds.

## Phase 3 — Module editor UI

### Task 3.1 — Dependencies tab rebuild (4 sections)
- `web/templates/policy_module_editor.templ` Dependencies tab → sections: (1) **Module dependencies**: picker rows {module select from visible library feed, constraint text input default `*`, remove}; (2) **Conflicts**: picker + chips; (3) **Tests**: condition rows + "Add test" (mounts shared dialog with all kinds) + match-mode select in section header; (4) **Reusable dependency groups**: existing `moduleDepsDetails` unchanged, retitled.
- Handlers: version-fields save path stops CSV-splitting depends_on/conflicts — accepts structured rows (module_id + constraint preserved, never clobbered to `*`); new routes `POST /policy/modules/:id/versions/:vid/conditions[/(:cid/remove|:cid)]` + match-mode save, all `ModuleCanEdit` + draft-guarded.
- Read-only state (published or view-only session): no pickers/add/remove, chips link to referenced module detail, match-mode select disabled.
- Run `make gen`. Tests: handler CRUD incl. draft-guard + authz (edit vs view vs cross-tenant 404); template asserts 4 sections, both edit and read-only states; constraint round-trip.

### Task 3.2 — Editor completion: report tab, draft delete, script delete
- **Report tab**: same JSON-Schema row-editor component as Parameters (shared, parameterized — no copy-paste), editing `report_schema` via existing version-fields endpoint. Read-only when not draft/no-edit.
- **Draft version delete**: `POST /policy/modules/:id/versions/:vid/delete` — draft-state-guarded service method wrapping existing `DeletePolicyModuleVersion`; danger-styled button on draft rows only, `ModuleCanEdit`.
- **Per-script delete**: `POST /api/modules/:id/versions/:vid/scripts/delete` wrapping `DeleteModuleScript`, draft-guarded; Remove button per phase script.
- **URN**: render read-only in General with explanatory title.
- Run `make gen`. Tests: draft delete happy/guarded/authz; script delete; report schema round-trip; URN not editable; read-only assertions.

### Task 3.3 — Satisfies normalization + origin surfacing
- Save-time validation of `satisfies` entries against the policy catalog: unknown URN → inline warning, save proceeds. Normalize bundled seed `satisfies` values to catalog URNs (`SeedBundled` fixtures) — keep `SatisfiesURN` matching compatible.
- Surface `origin` (bundled/tenant/imported) in module hero + Library list column; `extension_adapter.go` maps from the real column (closes the known gap).
- Tests: warning path, seed parity test updated, origin mapping.

## Phase 4 — .pmdl export/import

### Task 4.1 — Export builder + route (dependency: `gopkg.in/yaml.v3`)
- `go get gopkg.in/yaml.v3` (pre-approved). Pure builder in `pkg/services` (new file `policymodules_export.go`): rows in → tar.gz out via `archive/tar` + `compress/gzip`; layout per spec (`module.yaml`, `<version>/version.yaml`, `<version>/scripts/<phase>/<seq>_<filename>`); `format_version: 1`; reserved empty `signature` block; `script_ref` tests embed resolved source. Persist generated version.yaml into `manifest_yaml` (cache only).
- `GET /policy/modules/:id/export` (`?version=`, `?all=1`) — `ModuleCanView` (bundled allowed), streams `application/gzip`, `Content-Disposition: attachment; filename="<urn>.pmdl"`. Export button on module hero (view-level); Export mass-action on Library (single selection v1).
- Tests: golden-manifest test on a fixture module (deterministic field order), scripts land at correct paths, authz (view ok, cross-tenant 404), bundled exportable, `manifest_yaml` populated.

### Task 4.2 — Import
- `POST /policy/modules/import` (multipart `.pmdl`), `manage_modules`-gated, on Sources tab (placeholder text replaced by a real upload form).
- Parser: size cap 16 MiB, per-file cap, entry-count cap, tar-slip guard (reject absolute/`..` paths), unknown `format_version` major → clear error. Writes structured columns only: module `origin='imported'`, owner = importer, tenant-scoped; every version imported as **draft**; URN collision → inline error with "import as copy" (`?as_copy=1` → `-imported-<n>` suffix), never overwrite.
- Round-trip property test: export → import (as copy) → export ⇒ manifests identical modulo URN/origin. Malicious-archive tests (slip, oversize, bomb-ish entry count).

## Phase 5 — Close-out

### Task 5.1 — Final verification + docs
- Full suite + `go vet` + `gofmt -l .`; `make gen` + `sqlc generate` no-drift check.
- Manual browser pass on :8081 (owner DB untouched): three test kinds authored on a module draft, dependency picker, report tab, draft/script delete, export download, import round-trip on Sources, read-only view of a published version.
- Update `docs/development/handoff.md` (shipped bullets + caveats: `script_expect` dead column, `/api/scripts` empty-feed contract); extend INV-CB and add INV-TEST/INV-PMDL in `docs/endpoint-management/ui/invariants.md`-adjacent architecture docs as the spec defines; update `docs/endpoint-management/architecture/data-model.md` for migration 011; update the ADR-006 example README to note `.pmdl` supersedes its layout sketch.

---

## Task dependency order
1.1 → 1.2 → 1.3 → 2.1 → 2.2 → 3.1 → 3.2 → 3.3 → 4.1 → 4.2 → 5.1
(3.2/3.3 could swap; everything else is strictly ordered.)

## Strong-model territory
Migration 011 + the `script_expect` data rewrite (Task 1.1), eval semantics (1.2), and the import parser's security guards (4.2) are strong-model-only per `docs/development/workflow.md`. Template/summary/read-only-state tasks (2.2, parts of 3.x) are small-model-eligible once their contracts exist.
