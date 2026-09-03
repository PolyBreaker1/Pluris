# Modular Module System: Tests, Standardized Dependencies, Full Editor, .pmdl Export/Import — Design Spec

**Date:** 2026-07-17
**Author:** Claude (Sisyphus) + Peter (owner)
**Status:** Approved 2026-07-17 (owner) — all three open items confirmed: yaml.v3 dependency OK, bundled-module export allowed, plain renamed tar.gz (no magic bytes) in v1.
**Builds on:** `docs/history/specs/2026-07-12-module-persistence-and-param-injection.md`, `docs/history/specs/2026-07-12-condition-builder-and-script-conditions.md`, `docs/history/specs/2026-07-12-module-grants-and-ownership.md`, ADR-006/ADR-007/ADR-008.

---

## Goal

Make Policy Modules a complete, self-contained, portable unit:

1. **Module tests ("queries")** — a module version carries an unlimited, ordered list of applicability tests, authored through the ONE existing `ConditionBuilderDialog` (INV-CB — no new dialog, no forked code). Every test follows one standardized shape: **subject → operator → expected value**.
2. **Standardized dependencies** — `depends_on`/`conflicts` stop being comma-joined text; they get a real module picker with version constraints. All parameter references use canonical pluris paths (INV-CPP).
3. **Complete editor** — every part of a module is editable from the GUI: metadata, versions (including draft delete), scripts (including per-script delete), parameters, sandbox, report schema, tests, dependencies.
4. **`.pmdl` export/import** — one compressed file per module (`<urn>.pmdl`, a renamed tar.gz), generated from the structured columns (which stay the source of truth), importable on the Sources tab.

## Non-goals

- The endpoint agent. All test kinds that require execution (bash/script) keep the existing "console verdict is `unknown` until an agent reports" contract — same as script conditions today.
- Signing/verification of `.pmdl` files (Ed25519 columns exist; signing is deferred until the agent work defines the trust chain; the format reserves the slot).
- The Scripts library page itself (`/scripts` is still a stub). This spec defines the *contract* the script picker consumes, so the Scripts surface can land later without reworking tests.
- Registry/community sources (Sources tab gets file upload only).

---

## Part 1 — Unified test model (condition builder extension)

### 1.1 One shape for every test

Every test, regardless of kind, is **subject → operator → expected value**, using the existing operator set (`catalog/dependencygroups.AllOperators()` — `in`, `not_in`, `exists`, `equals`, `not_equals`, `contains`, `not_contains`, `starts_with`, `ends_with`, `gt`, `gte`, `lt`, `lte`, `matches`). No per-kind operator forks.

| Kind | Subject | Example |
|---|---|---|
| `param` (existing) | canonical pluris path | `computer/hardware/os_family` · `in` · `[linux]` |
| `command` (new) | a one-line shell command | `uname -r` · `contains` · `"3"` |
| `script` (evolved) | a script reference from the Scripts library, or inline source | `custom.sh` · `contains` · `"example"` |

- `param` is unchanged: evaluated console-side against device facts via `EvalGroup`.
- `command` and `script` are agent-executed. The comparison target is the **stdout** of the run (trimmed of one trailing newline). A non-zero exit code makes the test evaluate to `fail`; a never-reported result evaluates to `unknown` (neither pass nor fail — same semantics `evalCondition` applies to script conditions today).
- The legacy `script_expect` JSON (`{exit_code, output_equals}`) is **superseded** by operator + values. Existing rows are migrated: `output_equals: "x"` → operator `equals`, values `["x"]`; a row with only `exit_code: 0` → operator `exists` with empty values (pass = ran, exit 0). `script_expect` column is retained but no longer written (documented dead column, like `custom_policies.parameters_schema`).

### 1.2 Storage — same columns, all three condition tables

`dependency_group_conditions`, `group_membership_rules`, and the new `module_version_conditions` (Part 2) share one column vocabulary (INV-CB schema parity, the migration-009 precedent):

- `kind` — `param` | `command` | `script` (widened CHECK/validation).
- `param_path` — kind=`param` only.
- `script_source` — kind=`command`: the command line; kind=`script` with inline source: the script body.
- `script_ref` — **new column** (TEXT, default `''`): kind=`script` referencing a library script by stable id. Exactly one of `script_source`/`script_ref` is non-empty for kind=`script`.
- `operator`, `value_json` — the standardized expectation for ALL kinds.
- `seq` — ordering.

Migration 011 adds `script_ref` to the two existing condition tables and rewrites legacy `script_expect` rows into operator/values as above.

### 1.3 Condition builder dialog — extended, not forked

`web/templates/condition_builder.templ` + `web/static/condition-builder.js` gain a third tab and small changes:

- Tabs: **Pluris condition** (param) · **Bash command** (command) · **Script** (script). Same dialog instance everywhere it is mounted today (dependency groups, dynamic group rules) plus the module editor.
- The **command tab**: single-line command input → operator `<select>` → expected-value input. Save-enable rule mirrors param: non-empty command + operator (+ value when `needsValue`).
- The **script tab** is restructured to the standardized shape: script picker (see 1.4) OR inline source (`PlurisCodeEditor`, exactly as mounted today) → operator → expected value. The bespoke exit-code/output-equals inputs are removed.
- A `data-cb-allowed-kinds` attribute (comma-joined kinds, mirroring `TargetPickerDialog`'s `data-allowed-kinds`) lets a mounting page restrict tabs. Default: all three. All current mount points allow all three.
- `condition:save` event detail gains `scriptRef`; prefill JSON gains the same. Everything else (repeated `values=` encoding, `data-cb-prefill`, remove-then-add edit flow) is unchanged.
- Server-side validation extends the existing unexported helpers in `pkg/services` (`validConditionOperator` + a widened `validateConditionPayload` replacing `validateScriptExpect`): kind ∈ {param, command, script}; command requires non-empty `script_source`; script requires exactly one of `script_source`/`script_ref`; operator must be valid for every kind.

### 1.4 Script picker contract (Scripts library is not built yet)

"Add script test" opens a picker listing available scripts. Contract:

- `GET /api/scripts` returns `[{id, title, filename}]` — the Scripts library feed. **v1 implementation returns an empty list** (the `/scripts` page is a stub); the picker shows an empty state ("No library scripts yet — paste inline source instead") with the inline-source editor as fallback.
- `script_ref` stores the script's stable id. When the Scripts library lands, it implements `GET /api/scripts` and the picker fills in — zero changes to the condition model.

### 1.5 Eval + agent contract

`EvalGroup(g, facts)` stays the single eval engine. `command` and `script` tests read the fact key `script_result/<condition-id>` (the existing namespace — an agent reports the run's stdout + exit code there); the console compares stdout via operator/values. Absent fact → `unknown`. No second eval path.

---

## Part 2 — Module tests + standardized dependencies (the Dependencies tab, rebuilt)

### 2.1 `module_version_conditions` — unlimited per-version tests

New table (migration 011), schema-parity with the other two condition tables, keyed by `version_id` (FK → `policy_module_versions` ON DELETE CASCADE) instead of `group_id`. Version-scoped because tests travel with the version on export and are frozen by publish.

- CRUD via `PolicyModuleService` methods (`AddVersionCondition`/`UpdateVersionCondition`.../`RemoveVersionCondition`), **draft-guarded** exactly like `SetScript` (state-guarded UPDATE/INSERT via `:execrows`, `ErrVersionNotDraft` on a published version).
- Match mode: a new `conditions_match_mode` TEXT column on `policy_module_versions` (`all` | `any`, default `all`) — rendered as the same header `<select>` the dependency-group tab uses.
- Evaluated by the same `EvalGroup` (a version's conditions marshal into a `dependencygroups.Group` value at eval time — no new engine).

### 2.2 Dependencies tab layout (module editor)

The tab becomes four clearly-labeled sections, all draft-guarded:

1. **Module dependencies** (`depends_on`) — a picker-driven list replacing the comma CSV. Each row: module (picked from a filtered module list — the tenant's visible modules + bundled, via the existing library feed), version constraint (free-text semver constraint input, default `*` — no longer hardcoded), remove button. Stored shape unchanged: `[{module_id, version_constraint}]`.
2. **Conflicts** — same picker, URN list, remove buttons. Stored shape unchanged: `[]string`.
3. **Tests (conditions & queries)** — the unlimited `module_version_conditions` list: human-readable rows (`OS family · is any of · linux`, `Bash · uname -r · contains · "3"`, `Script · custom.sh · contains · "example"`), Edit (prefill) / Remove per row, "Add test" opening the shared `ConditionBuilderDialog`, match-mode select in the section header. Row summaries reuse the dependency-group summary rendering, not a fork.
4. **Dependency groups** (existing reusable platform/requirement links) — `moduleDepsDetails` unchanged. Labeled "Reusable dependency groups" so per-module tests vs shared groups stay visually distinct (dev_notes requirement).

### 2.3 Pluris paths everywhere

- `param` tests use canonical paths (INV-CPP) fed by the permission-filtered `/api/params` — already true of the shared dialog.
- **`satisfies` normalization**: bundled seeds currently mix styles (`sec.remote-access.ssh.password-auth` vs `Computer/WindowsComponents/...`). v1: validate entries against the policy catalog on save (warn, don't block, on unknown URNs); normalize the bundled seed data to catalog URNs. A full URN-scheme migration is out of scope.

---

## Part 3 — Editor completion (every part editable)

- **Report schema tab** — new "Report" tab (or a section under Parameters) editing `report_schema` through the SAME JSON-Schema row editor component the Parameters tab uses (one universal editor block — dev_notes standardization rule). Draft-guarded, saved via the existing version-fields endpoint.
- **Draft version delete** — a danger-styled Delete button on draft rows in the Versions tab. `DeletePolicyModuleVersion` query already exists; new handler `POST /policy/modules/:id/versions/:vid/delete`, draft-state-guarded, `ModuleCanEdit`.
- **Per-script delete** — a Remove button per phase script (drafts only). `DeleteModuleScript` query already exists; wire `POST /api/modules/:id/versions/:vid/scripts/delete`.
- **URN** — stays immutable after create (it is the cross-tenant reference key); the editor shows it read-only with an explanatory title instead of hiding it.
- **Origin** — migration 011 adds `policy_modules.origin` TEXT CHECK (`bundled` | `tenant` | `imported`) backfilled from `is_bundled`; `imported` becomes DB-representable (closes the known gap in `extension_adapter.go`). `is_bundled` retained (existing queries key on it).

Everything continues through the existing three save endpoints + inline-edit mechanism; no new save pattern.

### Read-only module view (published / no-edit sessions)

The same editor page IS the module view (one canonical surface, INV-U — never a second detail layout). Every new surface this spec adds must render correctly in its read-only state:

- **Tests section**: published versions and view-only sessions (`ModuleCanView` without `ModuleCanEdit`) render the human-readable test rows WITHOUT Add/Edit/Remove controls and with the match-mode select `disabled` — same pattern as the published-immutable script tabs today.
- **Dependencies/Conflicts**: picker and remove buttons hidden; rows render as plain chips linking to the referenced module's detail page (view is navigation, not editing).
- **Report tab**: row editor renders read-only, same gating as Parameters.
- **Versions tab**: draft Delete button only on drafts AND only for `ModuleCanEdit`; the published-immutable banner behavior is unchanged.
- **Hero**: Export button is a VIEW-level action — visible to any `ModuleCanView` session (including bundled modules, where edit is categorically locked); Import lives on Sources behind `manage_modules`.
- Template tests assert both states (draft+edit vs published/view-only) for the Tests, Dependencies, and Report surfaces, mirroring the existing published-immutable-banner test.

---

## Part 4 — `.pmdl` export/import

### 4.1 Format

`<module-urn>.pmdl` — a gzip'd tar (renamed `.tar.gz`). Layout (one version per directory, matching the ADR-006 example shape):

```
pluris.sshd.password-auth-disable.pmdl
└── module.yaml                     # module-level: format_version, urn, title, description, origin
└── 1.2.0/
    ├── version.yaml                # everything version-scoped (see below)
    └── scripts/
        ├── apply/10_apply.sh       # <seq>_<filename> per phase directory
        ├── validate/10_validate.wasm
        └── ...
```

`version.yaml` carries: `version`, `state` (informational), `target_os`, `scope`, `satisfies`, `parameters` (JSON Schema), `sandbox`, `report_schema`, `conditions_match_mode`, `tests` (the standardized list: `{kind, subject, operator, values}` where subject is `param_path` / command / `script_ref`+embedded source), `depends_on` (`{module, constraint}`), `conflicts`, `scripts` (phase/seq/filename index), and a reserved `signature` block (empty in v1).

- `format_version: 1` at the top of `module.yaml`; import rejects unknown major versions with a clear error.
- The manifest is **generated at export time from the structured columns** — the DB stays the source of truth (the 2026-07-12 `manifest_yaml`-as-derived decision). The generated `version.yaml` is also persisted into the existing `manifest_yaml` column as a cache/audit artifact; nothing reads it back.
- A `script` test with a `script_ref` embeds the referenced script's source in the export (portable — the target console may not have the library script).

### 4.2 Export

- `GET /policy/modules/:id/export?version=<v>` — `ModuleCanView`-gated. Default: latest published version; `?version=` selects another; `?all=1` includes every non-revoked version. Streams the tarball with `Content-Disposition: attachment; filename="<urn>.pmdl"`.
- Export button on the module detail hero + an Export mass-action on the Library list (single-selection in v1).
- Implementation: a `pkg/services` export builder (pure: rows in → `io.Writer` out) using stdlib `archive/tar` + `compress/gzip`. YAML: **needs `gopkg.in/yaml.v3` — the one new dependency this spec requires; owner approval needed** (stdlib has no YAML; hand-rolling YAML serialization is worse).

### 4.3 Import

- The Sources tab's placeholder becomes a real upload form: `POST /policy/modules/import` (multipart, `.pmdl`), `manage_modules`-gated.
- Import parses the manifest into the structured columns: creates the module (`origin='imported'`, owner = importer, tenant-scoped), each version as a **draft** regardless of exported state — publish is an explicit local decision (keeps INV-M3's publish gate and the signing story intact).
- URN collision with an existing module in the tenant (or a bundled URN) → reject with a clear inline error offering "import as copy" (re-submits with a `?as_copy=1` flag that suffixes the URN `-imported-<n>`), never silent overwrite.
- Hard limits: max archive size (16 MiB), max per-file size, entry-count cap, and path sanitization (reject absolute paths / `..` — standard tar-slip guard). Round-trip property test: export → import → export produces an identical manifest.

---

## Migrations (011)

1. `dependency_group_conditions` + `group_membership_rules`: ADD `script_ref TEXT NOT NULL DEFAULT ''`; rewrite legacy `script_expect` rows into `operator`/`value_json` (data migration in Go if SQL-only is awkward — see `pkg/database/database.go` conventions).
2. CREATE `module_version_conditions` (parity columns + `version_id` FK CASCADE + `seq`, UNIQUE index on `(version_id, seq)`).
3. `policy_module_versions`: ADD `conditions_match_mode TEXT NOT NULL DEFAULT 'all'` CHECK.
4. `policy_modules`: ADD `origin TEXT NOT NULL DEFAULT ''`; backfill `bundled`/`tenant` from `is_bundled`.

SQL comments plain ASCII (AGENTS.md rule 5). Never edits an applied migration.

## Invariants (new/extended)

- **INV-CB (extended)**: one condition data model, one operator set, one eval engine, one dialog — now across THREE consumers (dependency groups, dynamic group membership, module version tests) and THREE kinds (param, command, script). Any fourth consumer reuses the same parity columns.
- **INV-TEST**: every test is subject → operator → expected value. No kind gets a bespoke expectation format again (`script_expect` is the cautionary dead column).
- **INV-PMDL**: `.pmdl` is generated from structured columns; import writes structured columns. The manifest is never a live source of truth inside the console.

## Test plan (high-level)

- Services: version-condition CRUD draft-guards; migration rewrite of legacy `script_expect`; export builder golden-file test; import round-trip + tar-slip/oversize rejection; URN-collision paths.
- Handlers: export content-type/attachment + authz (view vs edit vs cross-tenant 404); import happy path + as-copy; draft delete; script delete; dependency picker save (constraint preserved, not `*`-clobbered).
- Templates/JS: dialog third tab renders; `data-cb-allowed-kinds` filtering; Dependencies tab four-section structure; test-row human-readable summaries.
- Full suite green per AGENTS.md rule 9.

## Open items (owner decisions)

1. **`gopkg.in/yaml.v3` dependency** — required for manifest generation/parsing (AGENTS.md rule 10 requires explicit OK).
2. Export of **bundled** modules: allowed (they're public catalog content) — confirm.
3. `.pmdl` MIME type registered as `application/gzip`; filename is the contract — confirm no magic-bytes/branding header wanted in v1.
