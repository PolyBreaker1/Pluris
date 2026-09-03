# Policy Module — Scripts + Enforcement redesign

**Date:** 2026-07-18
**Status:** approved (owner), ready for implementation
**Related:** `catalog/policymodules/`, `web/templates/policy_module_editor.templ`,
`web/static/code-editor.js` (PlurisCodeEditor), `console/handlers/params_api.go`,
INV-CPP, INV-L, `docs/development/handoff.md`

## Problem

The policy-module editor's Scripts tab is a fixed **lifecycle-phase** editor: every
script is welded to one of five phases (`apply`, `disable`, `uninstall`, `validate`,
`report`), the phase derives the runtime (bash vs WASM), and language is never chosen or
stored. A separate **Parameters** tab edits the module's input-parameter JSON Schema.

The owner wants scripts to be **first-class, named, reusable** building blocks with a
user-chosen language, edited in a proper pop-out code editor, and wired to actions
(including custom actions) on a new **Enforcement** tab. Parameters that a script needs
come from the canonical Pluris parameter tree (INV-CPP), inserted directly into the
script — the module no longer maintains a separate input-parameter schema editor.

## Decisions (owner-approved)

1. **Scripts are a free, named list** — not phase slots. Each script has `name`,
   `language` (`sh` | `powershell` | `python`), `source`, and `origin`
   (`default` | `custom`). A script may be referenced by many actions.
2. **Language is per-script only.** The old phase-to-runtime derivation (apply=bash,
   validate/report=WASM) is dropped. Any script may be any of the three languages
   regardless of which action invokes it. Runtime enforcement/sandboxing of language
   choice is out of scope for this UI task.
3. **Scripts tab becomes an INV-L list.** Columns: Name, Language, Origin
   (`(default)` / `(custom)` tag), Used-by (which actions/dependencies reference it).
   Row-level add / rename / delete. No inline code editor on the tab.
4. **Editing opens a standalone code editor in a separate browser window** (see §3).
5. **Parameters tab is deleted, replaced by an Enforcement tab** (see §4). The module's
   own input-parameter **schema editor is retired** — scripts consume canonical Pluris
   parameters directly. The `parameters_schema` DB column stays (export/resolver keep
   reading it; empty string is already valid) but is no longer editable in the console.
6. **Enforcement maps actions to a command or a script.** The five lifecycle actions
   plus user-added **custom actions**; each action row is either an inline shell command
   or a reference to a script in the module, chosen via a shared autocomplete control.
7. **Defaults + reset.** A module ships with default scripts and default action wiring
   (seeded at creation, `origin='default'`, never mutated). Editing a default forks a
   new `custom` copy; the pristine default always survives. A Reset dialog offers two
   choices: (a) restore default scripts & assignments, or (b) full module reset (delete
   user-added scripts/actions, restore everything).
8. **Parameter insertion is two-part and is the security allow-list.** Picking a param
   from the left tree (a) **imports/declares it in a block at the top of the script** and
   (b) **inserts a reference token at the cursor**. Only referenced params are injected
   when the script is delivered to an endpoint — no blanket namespace dump.
9. **Endpoint secrecy.** Scripts are never visible, editable, or interactable on the
   endpoint. Only the exact parameters a script references leave the console for it.

## Design

### 1. Data model (new migration; the phase-keyed table is retired)

New numbered migration under `db/schema/` (never edit an applied one; read
`pkg/database/database.go` first; ASCII-only SQL comments).

- **Reshape `policy_module_scripts`**: replace `phase` with first-class script identity.
  Columns: `id`, `version_id`, `name`, `language` (`sh`|`powershell`|`python`),
  `source`, `origin` (`default`|`custom`), ordering. A stable per-version `key` (slug of
  name, or an id) is what actions reference. Carry-forward: existing phase rows migrate
  to named scripts (`apply` -> a script named "apply", language `sh`, `origin` custom,
  etc.) so no data is lost.
- **New `policy_module_actions`**: `id`, `version_id`, `action_key`
  (`apply`|`disable`|`uninstall`|`validate`|`report`|`custom:<slug>`), `label`
  (for custom actions), `kind` (`command`|`script`), `value` (inline command text, or
  the referenced script key), `origin` (`default`|`custom`), `seq`.
- **Defaults snapshot for reset**: default scripts and default actions are seeded rows
  with `origin='default'`. They are immutable — an edit never overwrites a default row;
  it forks a `custom` row and re-points any action from the default to the custom.
  "Restore defaults" re-points actions back to the default rows (customs stay).
  "Full module reset" deletes `origin='custom'` scripts/actions and restores action
  wiring to the seeded defaults.
- Regenerate with `sqlc generate`; add queries to `db/queries/policy_modules.sql`
  (list scripts, upsert script, delete script, list/upsert/delete actions, seed
  defaults, reset variants).

### 2. Service layer (`pkg/services/policymodules*.go`)

- Script CRUD: `ListScripts`, `UpsertScript` (fork-on-edit-of-default semantics),
  `RenameScript`, `DeleteScript`.
- Action CRUD: `ListActions`, `UpsertAction`, `DeleteAction`, `AddCustomAction`.
- `SeedModuleDefaults(versionID)` (called on module/version creation),
  `RestoreDefaults(versionID)`, `FullReset(versionID)`.
- `ReferencedParams(script)` — parse `{{ param "<path>" }}` tokens to compute the exact
  allow-list; used by export/delivery so only referenced params are injected (§8/§9).
- Only draft versions are editable (published/superseded stay immutable, as today).

### 3. Standalone code editor (separate window) — reusable

- New route: `GET /policy/modules/:urn/scripts/:key/edit` (draft + edit permission),
  rendering a **full-window** page (NOT DetailShell). Layout:
  - Header row: editable **script name** + **language dropdown** (sh / powershell /
    python). Changing language re-tokenises the editor.
  - Left **Parameters panel**: the canonical Pluris parameter tree (reuse the
    `/api/params` feed and the existing tree renderer). Clicking a param performs the
    **two-part insert** (§8): ensure a declaration line exists in the top **import block**
    (dedup) AND insert `{{ param "<path>" }}` at the cursor via CM6 `view.dispatch`.
  - Center/right: `PlurisCodeEditor` filling the viewport height.
- Opened via `window.open(...)` from the Scripts list; on the opener, refresh the list
  when the editor window posts a save / on focus.
- Save via a scripts endpoint (`POST /policy/modules/:urn/scripts/:key`), reusing the
  service `UpsertScript`.
- **Editor extension**: add `powershell` and `python` languages to `PlurisCodeEditor`
  (`web/static/code-editor.js`, today bash/yaml/json). The import-block management is
  editor-agnostic helper logic layered on top; keep `PlurisCodeEditor` generic and put
  the param-tree + import-block behaviour in a small standalone-editor script.

### 4. Enforcement tab + shared autocomplete-suggest control

- Replaces `moduleParametersTab`. Rows for `apply`, `disable`, `uninstall`, `validate`,
  `report`, plus an **Add action** control that creates a custom-named action row.
- Each row: one input where the user types a **shell command OR a script name**, backed
  by a **shared suggest control** that, as the user types, suggests the module's scripts.
  Suggestions are **informational**: name + language + short info, not bare names.
- Build the suggest control once as a reusable, standardized widget
  (`data-pluris-suggest` input + shared JS in `web/static/`), so the same dynamic search
  is available platform-wide (Enforcement, Dependencies, future lists). It reuses the
  autocomplete pattern already proven by `PlurisCodeEditor.completionSource`.
- Persist rows via action CRUD (§2). CSRF via the existing module-form token.

### 5. Defaults & reset UX

- `(default)` tag on unedited seeded scripts/actions; flips to `(custom)` once edited.
- **Reset** button opens a platform-styled `<dialog>` (never native `confirm()`), two
  choices:
  - **Restore default scripts & assignments** — re-point actions to the seeded defaults;
    keep user-added scripts. No data loss.
  - **Full module reset** (danger-styled confirm) — delete `origin='custom'` scripts and
    actions, restore everything to seeded defaults.
- After restore, edited `(custom)` and pristine `(default)` scripts both remain in the
  list (§7).

### 6. Security (endpoint secrecy + minimal injection)

- Scripts are console-side only. Nothing in this task exposes script source or an editing
  surface to an endpoint/agent path.
- Delivery injects **only** the params returned by `ReferencedParams` (the top import
  block == the allow-list). No other parameters or module data are bundled. Add a test
  asserting that a script referencing exactly N params yields exactly those N in the
  injected set (no leakage of unreferenced params).

### 7. Out of scope

- Runtime/language sandbox enforcement (which language is "allowed" for an action).
- A cross-module shared script library (defaults are baked into the module).
- Re-working the Report / Sandbox / Dependencies tabs beyond having Dependencies reuse
  the shared suggest control if trivial; Dependencies redesign is a separate task.
- Route-table grant-granularity caveat (unchanged, per handoff).

## Testing (AGENTS.md definition of done)

- **Migration**: applies on a fresh `t.TempDir()` DB; existing phase scripts carry
  forward as named scripts; seeded default scripts/actions present on a new version.
- **Service**: script CRUD; edit-of-default forks a custom and preserves the default;
  action CRUD incl. custom actions; `RestoreDefaults` re-points without deleting customs;
  `FullReset` deletes customs and restores; `ReferencedParams` returns exactly the
  referenced param paths (security allow-list) and nothing more; published version is
  immutable.
- **Handlers**: standalone editor page permission-gated (draft + edit) and 403 without;
  script save round-trips; enforcement action save (command vs script ref); add/delete
  custom action; reset endpoints (restore vs full). Deleted/immutable guards.
- **Templates**: Scripts tab renders the INV-L list with default/custom tags and no
  inline editor; Parameters tab gone, Enforcement tab present with action rows + suggest
  inputs; reset dialog copy for both choices.
- **JS contract**: rely on template-emitted `data-pluris-*` attributes as existing list
  features do; manual drive verifies the pop-out editor, two-part param insert, and
  suggest dropdown end to end.
- Build clean, full tests green, `gofmt -l .` clean; `make gen` after every `.templ`
  edit; `sqlc generate` after query/schema changes (ASCII-only SQL comments).

## Execution checkpoints (verify gate between each)

1. **Data model + service core**: migration, sqlc queries, script/action CRUD, seed +
   restore + full-reset, `ReferencedParams`, carry-forward of existing phase scripts.
   Unit tests green. No UI yet.
2. **Scripts tab as INV-L list**: list + add/rename/delete, default/custom tags,
   used-by column. Parameters tab still present until CP4 to avoid a half-broken editor.
3. **Standalone editor window**: route + full-window page, powershell/python languages,
   relocated param tree, two-part insert (import block + cursor token), save round-trip.
4. **Enforcement tab + shared suggest control**: replace Parameters tab, action rows
   (fixed + custom), reusable `data-pluris-suggest` widget, persistence.
5. **Defaults/reset dialog + tag flipping**, and the security/minimal-injection test.
