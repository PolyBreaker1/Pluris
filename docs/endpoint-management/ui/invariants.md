# UI Invariants

**What:** the checkable rules every Pluris console screen, handler, and editor is judged against — canonical parameter paths, the list registry, DetailShell as the one detail layout, sidebar/permission gating, and the role template baseline.
**Related:** [[layout-system]] [[data-model]] [[authorization]]

This is a living extraction of the full, append-only historical UX-invariants contract (condensed from a now-deleted document — full text in git history — still the tie-breaker for anything not covered here) trimmed to what is **currently true in code**. Where an invariant has been superseded, that is called out explicitly rather than silently dropped. Invariant IDs (`INV-*`) are preserved so old references still resolve.

---

## INV-CPP — Canonical Parameter Paths

Every parameter in Pluris is addressable by exactly one path: `<entity>/<section>/<param>` (lowercase snake, forward slashes) — e.g. `user/identity/email`, `computer/hardware/ram_mb`, `computer/hardware/os_package_family`.

- Paths are **derived**, never stored in a parallel table: entity = `SubtypeSchema.PathEntity`, section = `SchemaSection.Key`, param = `ParamDef.Key`. Source of truth is `catalog/params/` (registry) + `catalog/params/paths.go` (`PathFor`, `ResolvePath`, `SchemaByPathEntity`, `AllPaths`).
- Entity slugs registered today: `computer`, `server`, `printer`, `desk`, `user` (internal subtype key `identity`, path slug `user`).
- Uniqueness of `(entity, section, param)` is enforced by an init-time check + test — no two params collide, and every mounted param resolves to exactly one path per entity.
- Every surface addresses fields by path: row/detail `data-path` attributes, filter configs, the Configuration tab's bound-value references, field-update API keys (`section`/`key` in `POST /api/*/fields` bodies), and future import/export.
- A new parameter anywhere in the project **must** be registered in `catalog/params` first; only then does it get a path and become usable in any UI, filter, or config.

## INV-L — List registry (single source of truth for tables)

- **INV-L1.** One field registry per list, one cell renderer per field. Every list registers its `FieldDef`s in `web/lists/` exactly once via `lists.Register`; a duplicate list ID panics at startup. Adding a column is two co-located edits (entry in `Fields` + case in the render function) — never an ad-hoc `<thead>` in a `.templ` file.
- **INV-L2.** The detail dialog/page and the list table read the **same** registry (`lists.FieldsFor(listID)` — see `DetailTableFrame` in `web/templates/detail_shell.templ`). The column picker only controls which registry fields show as columns; detail views show the full field set.
- **INV-L3.** Visible-column state is per-user, persisted client-side in `localStorage["pluris.cols.<listId>"]`.
- **INV-L4.** Field keys are stable identifiers; renaming one is a migration of the localStorage upgrade path, not a silent redefinition.
- **INV-L9. One unified filter/sort/divider engine — `web/static/lists.js` + `web/static/lists.css`.** Every list wires filters, sort, dividers, count chip, empty state, and search-highlighting through this single pair. New lists never author their own filter/sort script. HTML contract:
  - `<… data-pluris-list="<listId>">` — discovery anchor for the toolbar/table.
  - `<input|select data-pluris-filter="<listId>" data-filter-attr="<rowAttr>" data-filter-mode="contains|equals|prefix|numGte">` — reads `row.dataset[camelCase(rowAttr)]`. `data-pluris-highlight="1"` wraps matches in `<mark class="pf-mark">`.
  - `<table class="pluris-list" data-list-id="<listId>">`; sortable `<th>` gets `class="th-sortable" data-field="<key>" data-sort-type="alpha|num"` and a `<button data-sort-key="<key>">`; cells carry `data-field="<key>" data-sort="<sortValue>"`.
  - `<… data-pluris-empty="<listId>" hidden>` — empty state, toggled by the engine.
  - `<… data-pluris-count="<listId>" data-template="{visible} of {total}">` — count chip.
  - Persistence: `localStorage["pluris.filter.<listId>"]` / `["pluris.sort.<listId>"]`.
  - Events: `pluris:list-filtered` / `pluris:list-reordered` fired on `document`, `detail = { listId, … }`.
  - Verified live in `web/templates/dependency_groups.templ` and `web/templates/pluris_policy.templ` (`data-pluris-filter` toolbars), and `web/static/lists.js`.
- **INV-L8.** One table, one `<thead>` per list. Category grouping is `<tr class="cat-divider" data-divider="1" ...><td colspan="99">` rows interleaved between data rows — never a second `<table>` per group.
- **INV-L10. One row interaction contract.** Navigable rows declare `data-row-href="<canonical-detail-url>"`; `web/static/lists.js` owns click, Ctrl/Cmd-click, and Enter-key navigation and ignores nested interactive controls. A list never adds its own row-click script or a redundant Open/Edit/View action. Separate controls are reserved for actual secondary actions such as clone, delete, or pick.
- **INV-L11. Theme-safe row readability.** `lists.js` reapplies visible-row alternation after filter/sort operations. `lists.css` consumes only `--list-row-bg`, `--list-row-alt`, and `--list-row-hover`, defined centrally by `Layout`; pages never hardcode their own table-row palette. Hover must be clearly visible without animation or decorative effects.

## Detail pages — DetailShell is THE layout

There is exactly one detail-page shell: `templates.DetailShell` (`web/templates/detail_shell.templ`). No screen invents a second detail layout — this supersedes any earlier bespoke per-entity detail markup.

- `HeroSpec` — breadcrumb (`Crumbs`), name/ID, status `Chips`, a `Defs` quick-facts list, an optional `Visual` (icon/avatar), an optional single hero `Action`, and an optional `DeleteForm` rendered inside the `⋮` dropdown.
- `TabSpec` — stable `Slug` (drives `data-tab`/`data-panel` and the URL hash), `Label`, server-rendered `Body`.
- Tab switching, hash deep-linking, the hero `⋮` dropdown, and the per-section inline-edit toggles are all handled by the **one** shared `web/static/detail.js` (INV-L9 discipline extended to detail pages — no per-page tab scripts).
- **INV-BK — Subordinate-page Back control.** Every `DetailShell` page replaces the duplicated title in the global app header with one restrained Back link. Its deterministic destination is the nearest linked `HeroSpec.Crumbs` segment. The full breadcrumb remains visible inside the page and is the canonical location/path display. Create/edit/picker workflows pass their canonical parent list explicitly. Top-level/list pages keep their title in the app header and do not show Back.
- `DetailTableFrame(listID, action)` renders the standardized embedded-table frame inside a tab: registry-driven `<thead>` from `lists.FieldsFor(listID)`, an optional single primary action, rows passed as children.
- Live examples: computer/server/printer/desk detail (`web/templates/pages.templ`), user detail (`web/templates/users.templ`), group detail (`web/templates/group_detail.templ`), policy detail (`web/templates/policy_detail.templ`), dependency-group detail (`web/templates/dependency_groups.templ`), Pluris Policy role detail (`web/templates/pluris_policy.templ`).

## §VI — Sidebar structure + route-permission gating

- The sidebar is a single source, `templates.Menu` in `web/templates/menu.go` — 10 top-level items, several with `Children`. Any sidebar change touches `Menu`, route registration in `console/server/server.go`, and the mount-point test in `console/server/server_test.go` (per the doc comment at the top of `menu.go`).

Current tree (verified against `web/templates/menu.go`):

```
Dashboard                          /                          dashboard
Users                              /users                     users
  └─ User Groups                   /groups?kind=identity      users-groups
Assets                             /assets/computers          assets
  ├─ Computers                     /assets/computers          assets-computers
  ├─ Servers                       /assets/servers            assets-servers
  ├─ Printers                      /assets/printers           assets-printers
  ├─ Desks                         /assets/desks               assets-desks
  └─ Groups                        /groups?kind=asset          assets-groups
Policy                             /policy/catalog             policy
  ├─ Policy Catalog                /policy/catalog              policy-catalog
  ├─ Configuration Groups          /policy/groups                policy-groups
  ├─ Modules                       /policy/modules                policy-modules
  ├─ Dependency Groups             /policy/dependency-groups        policy-dependency-groups
  └─ Pluris Policy                 /policy/pluris                    policy-pluris
Profiles                           /profiles                   profiles
Scripts                            /scripts                    scripts
Wine                                /wine                       wine
Package Management                 /packages/managers           packages
  ├─ Package Managers              /packages/managers            packages-managers
  ├─ Packages                      /packages/packages             packages-packages
  └─ Update Cycles                 /packages/cycles                packages-cycles
Server Administration               /server-admin                server-admin
User / Admin Preferences            /preferences                 preferences
```

- **One canonical group list.** Users▸User Groups and Assets▸Groups both point at the SAME `/groups` list (`web/templates/groups.templ`); the `kind` query param (`identity` | `asset`) presets the list's member-kind filter server-side — there is no second groups implementation. `auth.RoutePermissionKey`'s prefix match ignores the query string, so both entries gate on the same `group.view` permission (see [[authorization]]).
- Gating is **not** a hardcoded role table. `templates.MenuItemVisible(sess, item)` calls `auth.CanAccessGrants(sess.Grants, item.Href)`, which resolves via `auth.RoutePermissionKey(path)` (longest-prefix match, `pkg/auth/rbac.go`) against the permission registry in `catalog/permissions/`. A route with no mapped key is open to any authenticated session.
- The same `auth.CanAccessGrants` gate protects the actual route handlers (`requirePermission`/`requirePermissionScoped` in `console/handlers`) — the sidebar link and the handler read the same map, so a hidden menu item is never reachable by URL alone and a visible one is never a 403 trap.
- **INV-AK — Menu active-key contract.** `keyMatches(itemKey, active)` (`web/templates/menu.go`) is boundary-prefix matching: `active == itemKey` OR `strings.HasPrefix(active, itemKey+"-")`. A sub-view page passes `active = "<parentkey>-<view>"` (e.g. a module detail page passes `policy-modules-library` or similar) so it both highlights the correct top-level item and expands its children (`expandChildren` uses the same `keyMatches`), without the parent needing a hardcoded list of every sub-view key. Any new sub-view under an existing sidebar item must follow this `<parentkey>-<suffix>` naming, not invent an unrelated key string.
- This **supersedes** the old UX-invariants document's (condensed from a now-deleted document, full text in git history) §V/§VII hardcoded `super_admin / admin / user_self_service` per-menu RW/R/none matrix. Roles are now a template + custom-clone system with real grants; read `catalog/permissions/registry.go` and `catalog/permissions/templates.go` for current keys and builtin values, not the old table. See [[authorization]] for the full grants model.

## The 4-role template baseline

Four builtin role templates: `super_admin`, `admin`, `technician`, `user`. `catalog/permissions.TemplateGrants(slug)` returns each template's full grant matrix; `super_admin` bypasses all grant checks in code and is cross-tenant (hosted/multi-tenant use). Roles support parent-chain inheritance and diff-only override storage (`pkg/authz.ResolveRoleMatrix`, `SaveRoleOverrides`) and can be assigned directly to a user or via group membership (`group_roles` table) — resolved through `pkg/authz.EffectiveGrants` (direct ∪ group roles, each inheritance-resolved). Custom roles are created by cloning a template or an existing role and setting a `parent_role_id`; the canonical editor is `/policy/pluris` (`web/templates/pluris_policy.templ`, handlers in `console/handlers/pluris_policy.go`).

## INV-PK — Popup-only-for-pickers

Native `<dialog>` popups are reserved for **pickers and small, single-purpose selections** — `TargetPickerDialog` (`web/templates/pages.templ`), the module picker (`policyModulePickerDialog`), the single-condition `ConditionBuilderDialog` (`web/templates/condition_builder.templ`), and the avatar expand/upload overlay. Everything else that used to be a popup form is now a full `DetailShell` page:

- The **Configuration Group dialog was retired** (Task 5.2) in favor of `/policy/groups`, `/policy/groups/new`, `/policy/groups/:id` (General/Assignments/Policy Bindings tabs).
- The **Custom Policy Wizard was deleted** (Task 4.4) — it was a pure stub (its `cpw:save` event had zero listeners anywhere in the codebase; nothing it produced was ever persisted). The module editor (`/policy/modules/new`, `/policy/modules/:id`) is the canonical authoring surface for policy modules; there is no tenant custom-policy authoring UI today (see `docs/history/specs/2026-07-12-module-persistence-and-param-injection.md`).
- New features follow the same rule: if it needs more than a picker/small-edit, it gets a `DetailShell` page, not a new `<dialog>`.

## INV-PS — Single-source parameters

- **Entity/tenant parameters** are defined exactly once, in `catalog/params/` (INV-CPP above). No consumer hardcodes a parallel param list, label set, or operator list — every filter, condition-builder dropdown, and field-update path reads the registry (`catalog/params.Definitions`, `SchemaByPathEntity`, `OperatorsForParam`) or the JSON-shaped `GET /api/params` endpoint (`console/handlers/params_api.go`).
- **Module input parameters** are defined exactly once per module version, in `policy_module_versions.parameters_schema` (a JSON Schema). This is the ONE editable definition of a module's inputs — the module editor's Parameters tab writes it, `catalog/params.ModuleInputDefs` parses it into the same `ParamDef` shape as entity params, and it is exposed dynamically under the `module/input/<key>` path namespace via `GET /api/params?module_id=<urn>`. There is no second, hand-maintained list of a module's inputs anywhere.
- `ParamDef.Permission` (+ schema `DefaultPermission`) and `catalog/params.FilterByGrants`/`VisibleDefs`/`EffectiveDefs` are the one permission-filtering path for parameter visibility — a caller checks access ONCE against these, never re-derives visibility with an ad-hoc field blocklist. See `docs/history/specs/2026-07-12-parameter-registry-and-params-api.md`.

## INV-CB — Condition builder is the sole rule-authoring UI

`ConditionBuilderDialog` (`web/templates/condition_builder.templ` + `web/static/condition-builder.js`) is the one reusable rule-authoring component in the console. Dependency Group conditions, dynamic Group membership rules (`group_membership_rules`, migration 009), AND Policy Module version tests (`module_version_conditions`, migration 011) are all authored through it and evaluated by the SAME engine (`catalog/dependencygroups.EvalGroup`) — exactly one condition data model, one operator set, one eval path, never a fork per feature. Any fourth consumer reuses the same parity columns. See `docs/history/specs/2026-07-12-condition-builder-and-script-conditions.md`, `docs/history/specs/2026-07-12-dynamic-groups.md`, and `docs/history/specs/2026-07-17-modular-module-system-design.md`.

## INV-TEST — Standardized test shape

Every condition, regardless of kind, is **subject · operator · expected value**: `param` (canonical pluris path), `command` (one-line bash whose stdout is compared), `script` (library reference via `script_ref` or inline source, stdout compared). No kind ever gets a bespoke expectation format again — the pre-011 `script_expect` JSON is the cautionary dead column. Validation is one path (`pkg/services.validateConditionPayload`); operator vocabulary is `catalog/dependencygroups.AllOperators()`.

## INV-PMDL — .pmdl is derived, structured columns are truth

A `.pmdl` module package (renamed tar.gz) is generated at export time from the structured DB columns and parsed back INTO structured columns on import. The manifest (`module.yaml`/`version.yaml`, and the `manifest_yaml` cache column) is never a live source of truth inside the console. Imports always land as drafts with `origin='imported'`; publish is an explicit local decision.

## Testid conventions

Every top-level page and detail page carries `data-testid="page-<slug>"` on its outermost wrapper (e.g. `page-users`, `page-user-detail`, `page-asset-detail`, `page-dependency-groups`, `page-pluris-policy-detail`). Detail pages additionally carry an entity-id attribute (`data-user-id`, `data-asset-id`, `data-policy-id`, `data-role-id`, `data-group-id`). Forms carry a `data-testid` too (`setup-form`, `login-form`, `user-form`). Handler/render tests assert against these attributes rather than CSS classes or text content — see `web/templates/detail_shell_test.go` for the pattern.

## No-new-JS-frameworks rule

The frontend stack is fixed: server-rendered Templ + vanilla JS (`web/static/*.js`) + htmx (loaded in `Layout`, `web/templates/layout.templ`) for the handful of places that use it. No React/Vue/Svelte/build-step JS framework, no bundler. New interactive behavior extends `lists.js` / `detail.js` / `filters-modern.js` or adds a new small vanilla-JS file wired the same way — never a framework dependency. This is the UI-layer instance of AGENTS.md's "no new dependencies without the owner's explicit OK."

## Still-true invariants carried forward unchanged

These are unaffected by any code change since the original UX-invariants document was written and remain load-bearing as written there (condensed from a now-deleted document — full text in git history):

- **INV-H1–H4** (hierarchy: Tenant → Site → Group → Asset|Identity; `AssetLink` for asset-to-asset relations).
- **INV-S1–S5** (machine/user/both scope on every Configuration Group / Profile / Script / PolicySetting, loopback semantics).
- **INV-U1–U6** (single canonical editor per concept; extend, don't fork).
- **INV-D1–D3** (one hierarchical-search/picker component).
- **INV-M1–M13** (Policy Module engine: refcount uninstall, acyclic deps, sandboxing, signing, resolution order, canonical picker/library routes).
- **INV-X1–X4** (Extension Framework: one registration per content kind in `pkg/extension/`).
- **INV-O1–O3** (no bare Linux jargon; glossary-backed terms; onboarding empty states).

## Superseded / dropped since the original UX-invariants document was written

- **§VII's original Role field list** (`permissions: JSON GLPI-style domain.action grant map... resolved by pkg/authz`) — description was already forward-looking and now matches code; no change needed beyond noting role hierarchy (parent chain, group-role assignment) landed after that entry was written (`docs/history/specs/2026-07-09-rbac-v2-design.md`).
- **The old fixed `super_admin/admin/user_self_service` per-menu RW/R/none table** referenced by §V/§VI/§VII (INV-R1 role list, INV-N gating) — replaced by the `catalog/permissions/` registry + Pluris Policy matrix UI as described above. Read the registry, not the old table.
- **The full branding guide** (condensed from a now-deleted document, full text in git history) is about the *Pluris OS Linux distro* (boot theme, SDDM, GRUB, KDE Plasma) — out of scope for the console; its essence for console work is folded into [[layout-system]] instead.
- **Configuration Groups and Policy Modules mock data are gone.** Both are now real, DB-backed services (`pkg/services/configgroups.go`, `pkg/services/policymodules.go`) — any earlier invariant text or comment describing `configgroups.MockGroups`/`policymodules.AllModules()` as the live data source is stale; see [[handoff]] for current mock-vs-real status and the dated specs under `docs/history/specs/2026-07-12-*` for what shipped.
- **Redundant in-page tab bars are gone.** `PolicyTabs`/`AssetSubtypeTabs`/`PackagesTabs` (per-section tab strips duplicating the sidebar's own children) no longer exist in code — the sidebar's `Children` are the one navigation for those sub-views. `policyModulesSubTabs` is a genuine sub-view strip *within* the Modules page (Library/Defaults/Sources) and was kept; it is not a duplicate of sidebar navigation. `PageHeader` also no longer takes a `section`/breadcrumb argument — see [[layout-system]].
