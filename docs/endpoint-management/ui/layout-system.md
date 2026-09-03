# Layout System

**What:** the building blocks every console page is assembled from — Layout/PageHeader, the DetailShell anatomy, the PlurisCodeEditor component, list-page anatomy, the inline-edit and avatar systems, and the remaining bespoke dialogs.
**Related:** [[invariants]] [[data-model]]

## Page shell — `Layout`

`templates.Layout(active, title, backHref)` (`web/templates/layout.templ`) is the outermost wrapper for every page: dark sidebar (`.sidebar`, brand chrome) + light content area (`.app-content`). It loads the fixed asset set once — `/static/lists.css`, `/static/filters-modern.css`, `/static/lists.js`, `/static/filters-modern.js`, `/static/detail.js` (all `defer`) — plus Tailwind CDN, Inter/JetBrains Mono fonts, and htmx. All CSS custom properties (`--chrome-*`, `--paper*`, `--accent*`, semantic `--ok/--warn/--err/--info`, and list-surface `--list-row-*`) are declared once in `:root` inside this file; no page defines its own palette.

`backHref == ""` keeps the page title in the app header and is the standard for top-level/list pages. `DetailShell` supplies the nearest linked breadcrumb as `backHref`, replacing the redundant app-header title with a compact Back link while leaving the complete breadcrumb inside the page untouched. Create/edit/picker workflows pass their canonical parent list explicitly.

Scroll discipline: `.app-content` is the **only** scroll container in the layout (`overflow: auto`). `<main>` must never get `overflow: hidden` — that would hijack `position: sticky` from descendants (see the comment at `layout.templ:85`). Page content sits in `.page-content` (24px/28px padding) for padded sections, or unwrapped for full-width sections like tables (`.page-content-full` / no wrapper) — tables intentionally flow to the natural bottom of the page with no nested scroll box (condensed from now-deleted documents on natural table flow and earlier layout improvements — full text in git history — both superseded by this section).

`PageHeader(title, description, primaryActionLabel, primaryActionHref)` (`web/templates/pages.templ`) renders the standard title/description/primary-action header used at the top of every list and top-level page. `primaryActionHref` set → renders an `<a class="btn btn-primary">`; label-only (no href) → a `<button>` for JS-driven actions (e.g. opening a dialog). **`PageHeader` no longer takes a `section` argument and renders no breadcrumb/crumb** — the redundant in-page tab strips it used to sit above were removed (below), and the sidebar's own `Children` are the one navigation for a menu item's sub-pages. `DetailShell`'s hero `Crumbs` (see the anatomy below) are unrelated and unaffected — those are per-page breadcrumbs on detail pages, not this removed section crumb.

**Redundant in-page tab bars are gone.** `PolicyTabs`/`AssetSubtypeTabs`/`PackagesTabs` (per-section `.tabs` strips duplicating a sidebar item's own `Children`) no longer exist in code — they were pure duplication once the sidebar reliably highlighted/expanded the same sub-views (INV-AK, [[invariants]]). `policyModulesSubTabs` (`web/templates/pages.templ`) is a genuine exception and was **kept**: it's a sub-view strip *within* the single Modules page (Library / Defaults / Sources) — those aren't separate sidebar children, so there's no duplication to remove.

## DetailShell anatomy

See [[invariants]] for the invariant; this section is the shape.

```
DetailShell(activeNav, title, attrs, hero HeroSpec, tabs []TabSpec)
  └─ Layout(activeNav, title, nearest linked breadcrumb)
       └─ .asset-detail { attrs... }         ← attrs carries data-testid + entity-id
            ├─ .asset-detail-crumb            ← hero.Crumbs, "›" separated
            ├─ .asset-detail-hero             ← glass card
            │    ├─ .hero-main-row
            │    │    ├─ .asset-detail-hero-visual   (hero.Visual, optional)
            │    │    ├─ .asset-detail-hero-info     (Name, mono ID, Chips)
            │    │    └─ .hero-actions                (⋮ dropdown → hero.DeleteForm)
            │    ├─ .hero-detail-table          ← hero.Defs, label/value pairs
            │    └─ .hero-action-row            ← hero.Action, optional
            ├─ .asset-detail-tabs               ← one .asset-detail-tab per TabSpec
            └─ .detail-tab-panel × N            ← one per TabSpec, all server-rendered;
                                                   detail.js toggles is-active + hash
```

`DetailTableFrame(listID, action)` is the standard embedded table inside a tab body: a `.card.detail-table-card` wrapping a `<table class="pm-table">` whose `<thead>` comes from `lists.FieldsFor(listID)` (title-attribute tooltips from each `FieldDef.Description`). Callers render `<tr>` rows as children; `DetailEmptyRow(colspan, message)` is the standard empty-state row.

**DetailShell pages today** (the standard for every detail/create surface, not just assets): computer/server/printer/desk detail, user detail, group detail (`web/templates/group_detail.templ` — General/Members/Rules/Roles tabs), dependency-group detail, Pluris Policy role detail, **Configuration Group detail** (`web/templates/config_groups.templ` — General/Assignments/Policy Bindings tabs, replacing the retired dialog), and **Policy Module detail/create** (`web/templates/policy_module_editor.templ` — metadata + per-phase Scripts tabs + Parameters tab + Dependencies tab). Create-mode pages (`/policy/groups/new`, `/policy/modules/new`, `/groups/new`, `/users/new`) reuse the same shell with editable-open fields, per the module-editor/full-page-user-create pattern established in `docs/history/specs/2026-07-09-rbac-v2-design.md`.

## PlurisCodeEditor — self-hosted CodeMirror 6

Lifecycle scripts (policy module Scripts tab) and condition-builder script-condition sources are edited with `window.PlurisCodeEditor`, a thin wrapper (`web/static/code-editor.js`) around a self-hosted CodeMirror 6 bundle (`web/static/vendor/codemirror/codemirror-pluris.js`, ~413 KB minified IIFE exposing `window.CM6`) — **no CDN dependency at runtime**, consistent with the no-new-JS-frameworks / no-bundler rule below.

- `PlurisCodeEditor.mount(el, { language, value, readOnly, onChange, completionSource })` → `{ getValue(), setValue(v), destroy(), view }`. `language` ∈ `'bash' | 'yaml' | 'json'` (default `'bash'`). Theme is `oneDark` + a small Pluris-chrome override.
- `PlurisCodeEditor.upgradeTextareas(root, opts)` is the usual mount path: finds every `textarea[data-code-editor]` under `root`, mounts an editor into a sibling `.pluris-code-editor` div, hides (not removes) the original textarea, and keeps it bidirectionally synced (editor→textarea via an `updateListener` dispatching a real `input` event; textarea→editor via a `defineProperty` override on `.value`, so any existing prefill code that does `textarea.value = "..."` — e.g. the condition builder's edit-flow prefill — also updates the live CM6 doc with zero changes to that code). Idempotent via a `WeakSet`, safe to call on every dialog open. `opts.completionSource` (optional, passed through to `mount`) wires autocomplete — the module editor's Scripts tab uses this for `{{ param "<path>" }}` completion (see the module-persistence spec).
- Defensive by construction: missing/failed-to-load bundle → one `console.warn` + a plain-JS-variable fallback (`getValue`/`setValue`/`destroy` all still work, no crash, no framework dependency introduced).
- Mounting is **first-open**, not page-load: the owning JS (e.g. `condition-builder.js`'s `openDialog()`, the module editor's phase-tab-open handler) calls `upgradeTextareas` right after `showModal()`/making the tab visible — mounting CM6 into a `display:none` ancestor produces a zero-height editor, so the call must happen after the DOM is actually visible.
- No CDN, no bundler config committed to the app build — the vendor bundle is a checked-in build artifact with its exact reproduction steps recorded in `web/static/vendor/codemirror/README.md`.

## List-page anatomy

Every list page (Computers, Users, Configuration Groups, Dependency Groups, Pluris Policy roles, Policy Catalog, …) follows the same skeleton:

1. `PageHeader(...)` — section/title/description/primary action.
2. A `.card` toolbar wrapping the search input and quick filters, wired to INV-L9:
   - `data-pluris-list="<listId>"` on the wrapping `<section>`.
   - `<input data-pluris-filter="<listId>" data-filter-attr="..." data-filter-mode="contains" data-pluris-highlight="1">` for free-text search.
   - `<select data-pluris-filter="<listId>" data-filter-attr="..." data-filter-mode="equals">` for quick filters (e.g. type/scope dropdowns).
   - A count chip: `<span data-pluris-count="<listId>" data-template="{visible} of {total}">`.
3. The `<table class="pluris-list" data-list-id="<listId>">` itself, columns from the `web/lists/` registry, sortable headers per INV-L9, category dividers per INV-L8 where the list is grouped (e.g. Policy Catalog by GP category).
4. Row interaction: a navigable `<tr>` declares `data-row-href="<canonical-detail-url>"`. The shared `lists.js` provides click/Ctrl-or-Cmd-click/Enter navigation and ignores nested controls. There are no per-page row-navigation scripts and no separate Open/Edit/View button; action columns contain only real secondary operations.
5. Readability: visible rows alternate using the shared `--list-row-bg` / `--list-row-alt` theme tokens, and hover uses `--list-row-hover`. The engine recalculates parity after filtering and sorting, so visible rows remain correctly striped.

Reference implementations: `web/templates/dependency_groups.templ` (list at `/policy/dependency-groups`) and `web/templates/pluris_policy.templ` (list at `/policy/pluris`) — both carry the full `data-pluris-filter` toolbar contract and are the pattern to copy for new lists (per `docs/history/specs/2026-07-09-rbac-v2-design.md` task 5, "standardized lists").

## Inline-edit system

Per-section inline editing lives entirely in `web/static/detail.js` (`toggleSectionEdit` / `cancelSectionEdit` / `saveSectionEdit`) and is shared by every detail page — no per-page edit script.

- HTML contract: a `<section class="card" data-section="<sectionKey>">` with a `.section-header` containing `.section-edit-btn` (pencil) / `.section-cancel-btn` / `.section-save-btn`, and field spans `<span class="field-value" data-editable="true" data-field-key="<key>" data-copy="<rawValue>">`.
- Edit mode swaps each editable span for an `<input class="inline-edit-input" data-field-key data-original>`, pre-filled from `data-copy` (falls back to trimmed `textContent`).
- **Save is dirty-only**: `saveSectionEdit` only includes an input in the `changed` payload if its value differs from `data-original`. If nothing changed, it just calls `cancelSectionEdit`.
- The save POST target is derived from the URL, not hardcoded: `fieldUpdateURL()` maps `/users/:id` → `/api/users/:id/fields` and `/assets/:subtype/:id` → `/api/assets/:subtype/:id/fields`. Body: `{ "section": "<sectionKey>", "fields": { "<key>": "<value>", ... } }`. The CSRF token is read from the page's `[name=_csrf]` hidden input and sent as the `X-CSRF-Token` header (the CSRF middleware's `TokenLookup` accepts both `form:_csrf` and `header:X-CSRF-Token` specifically for this).
- **This POSTs to a real backend today** — `console/handlers/field_api.go` (`UserFieldUpdate` / `AssetFieldUpdate`, routes `POST /api/users/:id/fields` and `POST /api/assets/:subtype/:id/fields`). Field keys are validated against `catalog/params` section/key schemas; asset updates are storage-routed (some subtype params are real table columns, e.g. printer vendor, others live in the JSON payload blob). Gated by `identity.update` / `asset.update` scoped grants, with a self-service allowlist (`identities.SelfServiceEditableKeys`) for the `own` scope. **The earlier "save-stub, just `console.log`s" history is gone** — do not follow the old inline-edit-save planning note's (condensed from a now-deleted document, full text in git history) "Required Backend Implementation" section as a to-do list; it describes the pre-implementation plan and the shape it proposed is close to, but not identical to, what shipped (real handler is `field_api.go`, not a hypothetical `UpdateFields` service method).

## Avatar expand / upload

`window.openAvatarModal(el)` in `detail.js` animates the small hero avatar (img or initials div) into a centered overlay, offering "Change photo" (native file picker) or drag-and-drop onto either the expanded view or the original small avatar (`setupAvatarDrop`, glow via `.avatar-drop-glow` using the theme accent, never a hardcoded color). On file select/drop, `applyAvatarFile` does an optimistic local preview (`FileReader` → data URL) and POSTs the real file as multipart form-data to `POST /api/users/:id/avatar` (`console/handlers/avatar.go`) — content-sniffed png/jpeg/webp, ≤2MB, stored at `data/avatars/<id>.<ext>` (gitignored), served back at `/avatars/...` behind auth (not currently tenant-scoped — see [[handoff]] known caveats).

## Role picker

`RolePickerSelect(fieldName, roles, selectedID, includeNone)` (`web/templates/role_picker.templ`) is the one canonical `<select>` for assigning a role to a user or group: `<optgroup>` per template family, options indented by inheritance depth (`groupRolePickerOptions(rolePickerOptions(roles))`). Shared across the user detail Roles tab, the group detail Roles tab, and the create-role parent picker on `/policy/pluris` — never a bespoke `<select>` per call site.

## Modals / dialogs that remain

Per INV-PK ([[invariants]]), native `<dialog>` popups are now reserved for pickers and small selections only. The **Configuration Group dialog was retired** (Task 5.2 — replaced by the Configuration Group DetailShell pages above) and the **Custom Policy Wizard was deleted** (Task 4.4 — it was a pure stub; see [[invariants]] INV-PK for the evidence). What remains, each a single shared component reused at every mount point (INV-U2):

- **Target Picker** — `templates.TargetPickerDialog(allowedKinds []TargetKind, targets []Target)` (`web/templates/pages.templ`). Real, tenant-scoped rows from `pkg/services/targets.go`'s `TargetService.Catalog` (computers/servers, users, computer/user groups, configuration groups) — no more mock target list. Mounted once per page (Configuration Group detail's Assignments tab, Group detail's Members tab) and opened via `data-target-picker-open` (optionally `data-allowed-kinds="computer,user"` to narrow per call site); picking dispatches `target:pick` on `document` with `{ kind, ref, label, meta }` — `ref` is always the row's numeric DB primary key as a decimal string, ready to `strconv.ParseInt` straight into an assignment/membership row.
- **Condition Builder** — `templates.ConditionBuilderDialog()` (`web/templates/condition_builder.templ` + `web/static/condition-builder.js`), the sole rule-authoring UI (INV-CB, [[invariants]]) shared by Dependency Group conditions and dynamic Group membership rules. Opened via `data-condition-builder-open`; edit-flow prefill via `data-cb-prefill` (HTML-escaped JSON) + `data-cb-cond-id`; saving dispatches `condition:save` on `document`. Mounts a `PlurisCodeEditor` for the script-condition source (above). See `docs/history/specs/2026-07-12-condition-builder-and-script-conditions.md` for the full operator set and the script-condition agent contract.
- **Policy Module Picker** — `policyModulePickerDialog()` (`web/templates/pages.templ`, "canonical reusable dialog" per INV-M12), mounted once per page and driven by `data-pmp-*` attributes from each call site (Configuration Group binding row, Modules Library "Pick" button). **Still v1/stub**: row-action handlers (`data-pm-action="edit"|"clone"|"delete"`) were removed from the Library list (which now uses real links/forms per the module editor), but the picker dialog's own Save path remains a stub that toasts and closes rather than wiring a pick into a binding row — unchanged by this overhaul, flagged here for a future task.
- **Avatar expand/upload** — unchanged, see below.

These dialogs are intentionally self-contained `.templ` components (not DetailShell) because they're transient pickers layered over a page, not their own navigable page.

## Branding essence (name usage, tone)

The full branding guide (condensed from a now-deleted document, full text in git history) describes the **Pluris OS** Linux distro's boot/login/desktop theming (Plymouth, GRUB, SDDM, KDE Plasma) and is out of scope for the console. What carries over to console UI work:

- **Name**: always "Pluris" (not "PLURIS" or "pluris" in prose); page `<title>` is `"{ title } — Pluris"` (see `Layout`).
- **Tone**: clean, minimalistic, enterprise-ready — professional, not playful. Deep-blue chrome (`--chrome-bg: #0a1628`) reserved for the sidebar; the content area is light (`--paper: #ffffff` / `--paper-alt: #f8fafc`). Cyan (`--accent: #0099c2` on light content, `--chrome-accent: #00d4ff` on dark chrome) is the one accent color — used for active states, focus rings, links, and the single primary-action button style; it is not sprinkled decoratively.
- **Typography**: Inter for UI text, JetBrains Mono (`.font-mono`) for IDs, hostnames, and other identifier-shaped values.

## The templ-fmt canonical formatting note

**Do not run `templ fmt`.** Per `AGENTS.md`, it is explicitly excluded from the standard workflow. A past task (`docs/history/specs/2026-07-09-rbac-v2-design.md` task 6) had `templ fmt` reformat many `.templ` files cosmetically as a side effect of an editor action, and that whitespace churn is called out as a known, accepted diff-noise source — not something to reproduce intentionally. After editing a `.templ` file, run `make gen` only; let `templ generate` (not `templ fmt`) be the only formatter-adjacent tool that touches these files.
