# Pluris UX Invariants

> **Canonical source**: `docs/Pluris UX structure plan.md` (user-authored).
> This document is a formal extraction of the rules implied by that plan, in checkable form. If anything here contradicts the user-authored plan, the plan wins and this document is wrong — fix this file.

This document and the user-authored plan together form the **UX/IA contract** for the Pluris management platform. Every screen, handler, schema entity, and editor in `pluris/` is judged against these rules. New work must reference the relevant invariants in its design before code is written.

Status: **append-only**. Add new invariants and concept-registry entries; never silently rewrite existing ones (revise via a dated note).

---

## I. Hierarchy

**INV-H1.** There is exactly one hierarchy in Pluris: `Tenant → Site → Group → (Asset | Identity)`.
- Tenant: top-level isolation boundary (multi-tenancy root).
- Site: geographic / network boundary; subnets attach here.
- Group: assignment-target collection; can hold Assets and/or Identities. Sourced locally or from Kanidm.
- **Asset**: any managed hardware. Discriminated by `subtype ∈ {computer, server, printer, desk, …}`. Subtype is extensible.
- Identity: end-user (human).

**INV-H1a.** Asset-to-asset relationships (e.g. monitor docked-to laptop, peripheral attached-to host) are modeled by a single `AssetLink` entity with a typed `relation` enum. One model, many relation types — no per-relation table.

**INV-H2.** Every navigation tree, dashboard tile picker, search filter, policy assignment selector, and audit-log facet **reads from this same hierarchy model** (one Go package, one Templ partial). Alternative groupings are mapped onto this hierarchy, not added beside it.

**INV-H3.** AD/GP compatibility is built in at the hierarchy layer:
- Site ↔ AD Site
- Group ↔ AD Security Group / OU (configurable per-Group)
- Asset(subtype=computer) ↔ AD Computer; other Asset subtypes have no AD equivalent and are Pluris-native.
- Identity ↔ AD User
- Configuration Group ↔ AD GPO  *(renamed from "Policy Group" 2026-05-05; the schema path may still read `policy_group.go` until the backend slice lands)*

**INV-H4.** Hierarchical search works from any control that selects an entity. Inspiration: Qualys-style typeahead with breadcrumbs and lazy expansion.

---

## II. Scope (User vs Computer)

**INV-S1.** Every Configuration Group, Profile, Script, and PolicySetting carries `scope: machine | user | both`.

**INV-S2.** Editors visually partition User Configuration vs Computer Configuration like Windows Group Policy. The two sections are independently collapsible and clearly labeled.

**INV-S3.** Backend resolution treats machine-scope and user-scope as independent inheritance chains. They merge into the effective policy at evaluation time on the agent (machine policy on boot, user policy on `pam_open_session`).

**INV-S4.** A setting whose scope is `both` is conceptually two settings sharing a definition; per-tenant configured values can differ between the machine and user sides.

**INV-S5 (Loopback).** A **user-targeted** Configuration Group MAY carry computer-scope settings. Those settings apply to the host the user is currently logged into, for the duration of the session — analogous to Windows GP "loopback / merge" mode. The editor labels them "Session-host settings" and pairs them with a one-line reminder so the admin cannot confuse them with settings that follow the user across hosts. Computer-scope settings in computer-targeted groups apply unconditionally; the loopback case applies only to user-targeted groups.

---

## III. Single Source of Truth UI (the most important invariant)

**INV-U1.** Each concept has exactly one canonical editor. The canonical editors are listed in §VII Concept Registry.

**INV-U2.** When the same concept is reachable from multiple navigation paths, every path mounts the **same component instance** with at most a context filter / scope prop applied.

> Example: opening a Configuration Group from the Policy → Configuration Groups tab vs. from a Computer's "Configuration Groups" sub-tab vs. from inside a Profile editor must all mount `editors/ConfigurationGroupEditor` (the v1 in-page modal lives at `templates.ConfigurationGroupDialog`). The Computer-tab entry passes `targetDevice=$id`; the Policy-menu entry passes nothing; the Profile-embedded entry passes `embedded=true`. The component is identical.

**INV-U3.** If a context needs **less** than the canonical editor exposes, pass a filter or scope prop (`hideTabs`, `readOnly`, `scope=machine`). Never create a "simplified" or "lite" sibling component.

**INV-U4.** If a context needs **more** than the canonical editor exposes, extend the canonical editor. Never create a sibling that adds the missing feature locally. The bar for extension is "would all callers benefit from optional access to this feature?" — if yes, extend.

**INV-U5.** A bug where "this dialog is missing a feature it has elsewhere" is, in practice, almost always a routing bug: the entry point isn't mounting the canonical editor. Diagnose routing before adding new code.

**INV-U6.** Each canonical editor has a **mount-point test** that asserts every documented entry point in §VII routes to the same component. Adding a new entry point requires extending the test.

---

## IV. Hierarchical search & dynamic value pickers

**INV-D1.** Wherever the user picks an entity (e.g. dashboard tile data source, policy target, group member, computer-for-script-test), the picker is the **same hierarchical search component** consuming the model from INV-H2.

**INV-D2.** Pickers support typeahead, lazy expansion, and breadcrumb display.

**INV-D3.** Picker scope can be filtered (`only=device`, `only=identity`, `under=site:berlin`) but the component is unchanged.

---

## V. Roles & self-service

**INV-R1.** Roles include at minimum: `super_admin`, `admin`, `user_self_service`. Roles gate which menu items render and which editors are read-only vs. read-write.

**INV-R2.** Self-service for end users uses the **same editors** as admin views, with `readOnly=true` and a scope-of-self filter. No "user portal" component branch.

**INV-R3.** "Admin testing workstations" is an admin-preference setting referencing existing Computer entities (per the UX plan: "machines near admin, not necessarily admin owned"). Not a separate concept; a typed reference list on the admin's preference record.

---

## VI. Top-level navigation (left sidebar)

The left sidebar contains exactly these top-level items, in this order:

1. **Dashboard** — adjustable, scalable tiles. Tile data source is selected from the hierarchical picker (INV-D). Tile display types: graph, text, update view, table.
2. **Users** — Identity list; supports AD import and manual add; role assignment; semi-automatic asset pairing.
3. **Assets** — unified hardware list with subtype tabs:
   - **Computers** (workstations, laptops)
   - **Servers**
   - **Printers**
   - **Desks** (docks + monitors; profiles ease guest connection)
   Default columns vary per subtype; common columns: Name, Subtype, Site, Last Seen, Group. Detail view has tabbed sub-modules (Configuration Groups, Installed Software, Wine Groups, Assigned Scripts, Logs) plus subtype-specific tabs.
4. **Policy** — three sub-tabs:
   - **Policy Catalog** — read-only browser of every catalog entry Pluris knows how to express, partitioned into Computer / User scope (INV-S2). Searchable + filterable; entries are bound to targets only via Configuration Groups.
   - **Configuration Groups** — list + editor for the entity that binds catalog policies (with values) to a target (computer / user / computer group / user group / another Configuration Group / regex *(planned)*). Loopback semantics per INV-S5. Every editor section is partitioned into Computer Configuration and User Configuration (INV-S2). When a Windows policy is bound, a side-by-side view shows compatible Policy Modules (filtered by `satisfies` and `target_os`).
   - **Modules** — Policy Module Library / Tenant defaults / Sources (INV-M13). Canonical editor `editors/PolicyModuleEditor`; canonical chooser `editors/PolicyModulePicker` (INV-M12). Moved from Scripts on 2026-05-16: a Policy Module IS-A policy concept.
5. **Profiles** — Profile list and editor. Profiles bundle assigned software, Wine groups, scripts, and packages.
6. **Scripts** — single page for admin-authored automation scripts. Editor pane, manual-trigger test against admin testing workstation, live agent log stream, trigger picker (`shutdown`, `app_open`, `policy_change`, `custom`, …). Pluris ships at least 5 predefined scripts. (Policy Modules used to live as a second tab here; moved to Policy on 2026-05-16.)
7. **Wine** — tabs for Applications and Configuration Groups.
8. **Package Management** — three tabs:
   - **Package Managers** (apt / dnf / pacman / flatpak / snap / winget / brew / … with per-asset overrides)
   - **Packages** (cross-manager catalog, install status, version distribution, vulnerability state)
   - **Update Cycles** (windows, schedules, staged rollouts; connected to assets, policies, scripts)
9. **Server Administration** — AD connection settings, GP import wizard, **Policy Enforcement Scripts** (admin view of `editors/PolicyModuleEditor`), server (application) logs.
10. **User/Admin Preferences** — per-user (light/dark, profile picture) and admin-specific (admin testing workstations).

**INV-N1.** Adding a new top-level menu item requires user approval and a Concept Registry entry (§VII).

**INV-N2.** Cross-navigation between menu items is allowed and encouraged via tabs/links inside detail views; these always honor INV-U2.

**INV-N3.** A concept reachable from multiple top-level menus (Policy Module from Scripts and from Server Administration; Package from Package Management and from Asset detail) mounts the **same canonical editor** at every entry point.

---

## VII. Concept Registry

For each concept: the entity, its scope semantics, its canonical editor, every entry point that mounts that editor, and the filter passed by each entry point.

| Concept | Backend entity | Scope | Canonical editor | Entry points (entry → filter) |
|---|---|---|---|---|
| Tenant | `db/schema/tenant.go` | n/a | `editors/TenantEditor` | Server Admin → Tenants list |
| Site | `db/schema/site.go` | n/a | `editors/SiteEditor` | Server Admin → Sites; Asset detail → Site field (link) |
| Group | `db/schema/group.go` | n/a | `editors/GroupEditor` | Users menu → Groups tab; Assets menu → Groups tab; AD import results |
| Identity (User) | `db/schema/identity.go` | n/a | `editors/UserEditor` | Users menu list; Asset detail → Assigned Users; Policy assignment target picker |
| **Asset** (root) | `db/schema/asset.go` *(rename from `device.go`; subtype enum + JSON payload)* | n/a | `editors/AssetEditor` | Assets menu (all tabs); search; dashboard tile drilldown; Policy assignment target picker; Discovery results |
| └ Computer | reuses Asset | n/a | `editors/AssetEditor` | Assets → Computers tab | `subtype=computer` |
| └ Server | reuses Asset | n/a | `editors/AssetEditor` | Assets → Servers tab | `subtype=server` |
| └ Printer | reuses Asset | n/a | `editors/AssetEditor` | Assets → Printers tab | `subtype=printer` |
| └ Desk | reuses Asset | n/a | `editors/AssetEditor` | Assets → Desks tab; Profile editor (guest-connection profile reference) | `subtype=desk` |
| AssetLink | `db/schema/asset_link.go` *(new)* | n/a | inline UI inside `editors/AssetEditor` (Relations tab) | Asset detail Relations tab |
| Configuration Group *(formerly "Policy Group")* | `db/schema/configuration_group.go` *(planned; v1 in-memory mock at `pluris/catalog/configgroups`)* | machine \| user \| both, with INV-S5 loopback for user-targeted | `editors/ConfigurationGroupEditor` *(v1 = `templates.ConfigurationGroupDialog` — shared in-page modal)* | Policy → Configuration Groups tab; Asset detail → Configuration Groups tab; User detail → Configuration Groups tab; Profile editor → Embedded Configuration section; Server Admin → GP Import results |
| Profile | *(TBD `db/schema/profile.go`)* | machine \| user \| both | `editors/ProfileEditor` | Profiles menu list; Asset detail → Profiles section; User detail → Profiles section; Desk asset → Guest Profile reference |
| Script | *(TBD `db/schema/script.go`)* | machine \| user \| both | `editors/ScriptEditor` | Scripts menu → Scripts tab; Asset detail → Assigned Scripts tab; Profile editor → Scripts section; Update Cycle editor (pre/post hooks) |
| **Policy Module** | `db/schema/policy_module.go` *(planned; v1 in-memory mock at `pluris/catalog/policymodules`)* | machine \| user \| both | `editors/PolicyModuleEditor` (editor); `editors/PolicyModulePicker` (chooser, INV-M12) | Policy → Modules → Library (canonical list); Server Admin → Policy Enforcement Scripts; Configuration Group editor binding row → Module picker; Custom Policy Wizard step 5 (lifecycle scripts) | compatibility filter (`satisfies` ∩ selected policy URN; `target_os` ∩ selected device OS) |
| **Tenant Module Default** *(new, 2026-05-16, ADR-006 §UI)* | `db/schema/tenant_module_default.go` *(planned; v1 in-memory at `pluris/catalog/policymodules.tenantDefaults`)* | n/a (lives at tenant level) | inline UI inside `/policy/modules/defaults` | Policy → Modules → Tenant defaults | one row per (tenant, policy_urn); resolution per INV-M11 |
| **Module Installation** *(new, ADR-007)* | `db/schema/module_installation.go` *(planned)* | derived from module's scope | inline UI inside `editors/AssetEditor` (Installed Modules tab) | Asset detail → Installed Modules tab; debug from Configuration Group editor ("will install N" preview) | refcount computed from `installed_via` edges; not directly editable — created/destroyed by binding lifecycle |
| **Custom Policy** *(new, ADR-007)* | `db/schema/custom_policy.go` *(planned; extends `pluris/catalog/policies.Policy` with `tenant_id` + `custom: true`)* | machine \| user \| both | `editors/CustomPolicyWizard` *(multi-step)* | Policy Catalog → “+ New custom policy” button; Policy Catalog row context menu → “Edit” (custom rows only) | tenant scope filter; bundled rows are read-only and never mount the wizard |
| Wine Application | *(TBD)* | n/a | `editors/WineApplicationEditor` | Wine → Applications tab; Asset detail → Installed Software (Wine type) |
| Wine Configuration Group | *(TBD)* | n/a | `editors/WineConfigGroupEditor` | Wine → Configuration tab; Asset detail → Wine Groups tab; Profile editor → Wine section |
| **Package Manager** | *(TBD `db/schema/package_manager.go`)* | n/a | `editors/PackageManagerEditor` | Package Mgmt → Package Managers tab; Asset detail → Package Manager overrides |
| **Package** | *(TBD `db/schema/package.go`)* | n/a | `editors/PackageEditor` | Package Mgmt → Packages tab; Asset detail → Installed Software; Profile editor → Packages; Configuration Group editor → Required Packages; Policy Module manifest → declared package deps |
| **Update Cycle** | *(TBD `db/schema/update_cycle.go`)* | n/a | `editors/UpdateCycleEditor` | Package Mgmt → Update Cycles tab; Asset detail → Update Cycle field; Configuration Group → Required Cycle |
| Dashboard Tile | *(TBD)* | n/a | `editors/DashboardTileEditor` | Dashboard → "+ Tile" button; Tile context menu → Edit |
| Role | *(TBD)* | n/a | `editors/RoleEditor` | Server Admin → Roles; User detail → Role field |
| Admin Testing Workstation | reuses Asset (subtype=computer) | n/a | reuses `editors/AssetEditor` (read-only via filter) | User/Admin Preferences → Testing Workstations list (picks existing Assets) |

Entries marked *(TBD)* require an Ent schema and editor before the corresponding menu item or feature ships. Bold rows are concepts added in the 2026-05-05 update.

**INV-CR1.** When a new concept is introduced, an entry is added to this table **before** any handler or template for it is written. Entries without a backend entity, scope, canonical editor, and at least one entry point are incomplete and block implementation.

---

## VII.A Concept field lists (v1, locked 2026-05-05)

> Field lists are the **IA contract** for each Concept Registry entry. They are versioned and explicitly evolvable — adding fields is additive (no breaking changes); removing or repurposing fields requires a §VII.A revision note. Fields marked **derived** are computed from edges and not stored. Fields marked *(?)* are best-guess placeholders to be confirmed during first implementation.
>
> **Extensibility patterns to use:**
> - **Subtype payloads** (Asset.subtype_payload): typed JSON column with per-subtype Go structs and JSON Schema validation. Adding a new subtype = new struct + new enum value, no migration of others.
> - **Additive enums** (Trigger.kind, Asset.subtype, Module.runtime): always add `custom` / `unknown` reserves; never reuse old values.
> - **Versioned children** (PolicyModuleVersion, Tile snapshots): catalog row + version rows so the same logical concept survives repeated edits.
> - **Edge-based composition** (Profile contains Scripts/PolicyGroups/Packages/WineConfigGroups): use Ent edges with link tables, never embed lists in JSON.

### A1. Asset hierarchy

**Asset** (`db/schema/asset.go` — renames `device.go`)
- `id`, `uuid` *(stable; mTLS subject CN)*, `tenant`, `site` *(edge)*, `groups` *(edges)*
- `subtype`: enum `computer | server | printer | desk | … (extensible)`
- `subtype_payload`: typed JSON, schema per subtype:
  - **Computer**: hostname, fqdn, os_family, os_distribution, os_version, architecture, cpu_summary, ram_mb, storage_mb, screen_render *(asset photo / OS image — per UX plan)*
  - **Server**: hostname, fqdn, os_*, services *(list)*, uptime_started_at, role *(db / web / dns / file / app / other)*
  - **Printer**: model, vendor, ip, queues *(list)*, supported_formats, current_consumables *(list of {kind, level_pct})*
  - **Desk**: location_label, dock_model, monitor_count, peripherals *(derived from AssetLink)*, `guest_profile_id` *(Profile ref; INV: when an Identity is `docked_to` this Desk, the resolver merges this Profile)*
- **Common across subtypes**: `enrollment_state` (pending|approved|enrolled|disabled|revoked), `enrolled_at`, `last_seen_at`, `agent_version`, `labels` (map<string,string>)
- **Asset-management fields** (CMDB): `lifecycle_state` (active|in_repair|decommissioned|disposed), `location` (free text + Site ref), `owner_identity` (Identity ref nullable), `vendor`, `purchase_date`, `purchase_price`, `warranty_expires_at`, `contract_ref` *(?)*, `depreciation_method` *(?)*

**AssetLink** (`db/schema/asset_link.go` — new)
- `id`, `tenant`, `from_asset` *(edge)*, `to_asset` *(edge)*
- `relation`: enum `peripheral_of | docked_to | printed_via | hosts_vm | connected_to | other`
- `metadata`: JSON (relation-specific, e.g. dock port, USB hub slot)
- Indexed on `(from_asset, relation)` and `(to_asset, relation)`.

### A2. Profile / Script / Wine

**Profile** (`db/schema/profile.go` — TBD → locked)
- `id`, `slug`, `tenant`, `name`, `description`
- `scope`: `machine | user | both`
- `disabled`: bool
- Edges (multi):
  - `policy_groups` → PolicyGroup
  - `scripts` → Script (via ProfileScriptBinding with optional trigger override)
  - `wine_config_groups` → WineConfigGroup
  - `packages` → Package (declares "must be installed")
- **Decision (locked)**: profiles are **flat**. No profile-includes-profile composition. Inheritance is via the existing Tenant→Site→Group→Asset|Identity chain plus the Profile's assignment scope; profile composition would create a second inheritance dimension and we don't want that.

**Script** (`db/schema/script.go` — TBD → locked)
- `id`, `slug`, `tenant`, `name`, `description`
- `scope`: `machine | user | both`
- `target_os`: `linux | windows | macos | any`
- `runtime`: `bash | powershell | python | nu | ansible-task` *(extensible)*
- `source`: text *(edited inline in the web UI; versioned via ScriptVersion **TBD if we need history; default yes — mirrors PolicyModuleVersion pattern)*
- `parameters_schema`: JSON Schema
- `triggers`: list of `{ kind, config }`. `kind` ∈ `manual | shutdown | startup | login | logout | app_open | app_close | policy_change | schedule | custom`
- `run_as`: `root | user | specified_user`
- `timeout_seconds`: int
- `testing_workstation_id`: Asset ref (subtype=computer)
- `last_test_at`, `last_test_result`: derived from execution history

**Predefined scripts shipped with Pluris** (5+, per UX plan):
- `update-on-shutdown` — apply pending package updates on shutdown if window allows
- `cleanup-temp-on-logout` — purge `/tmp/<user>` and browser caches
- `compliance-snapshot-daily` — invoke osquery and ship results
- `force-screen-lock-on-policy-change` — re-lock active sessions when a relevant policy changes
- `wake-and-update-windowed` — WoL + run UpdateCycle within window, suspend after

**WineConfigGroup** (`db/schema/wine_config_group.go` — TBD → locked)
- `id`, `slug`, `tenant`, `name`, `description`
- `wine_version`: `system | staging | specific(x.y.z)`
- `arch`: `win32 | win64`
- `registry_keys`: list of `{ hive, path, name, type, value }`
- `file_overrides`: list of `{ path, content_or_template, mode, owner }`
- `path_mappings`: list of `{ wine_path_pattern_regex, host_path_template }` *(was "regex path setup" in the UX plan — reads as "translate Windows-side paths to Linux host paths inside the prefix"; revisit if intent was different)*
- `environment`: map<string,string> (e.g. `WINEDLLOVERRIDES`)
- `dll_overrides`: list of `{ dll_name, mode: native|builtin|both|disabled }`
- `tricks`: list of winetricks verb strings *(reserved; runtime support optional)*

**WineApplication** (`db/schema/wine_application.go` — TBD → locked)
- `id`, `slug`, `tenant`, `name`, `vendor`, `version`
- `target_os`: `linux` (always — Wine is the Linux compat layer)
- `wine_config_group` → WineConfigGroup (the prefix this app installs into)
- `installer_source`: `{ kind: package_ref | url | uploaded_blob, value }`
- `silent_install_args`: text
- `post_install_script` → Script (optional)
- `installed_on_assets`: derived edge

### A3. Policy Module + Package Management

**PolicyModule** (catalog) (`db/schema/policy_module.go`)
- `id` (URN), `tenant` *(or null for bundled / global)*, `current_version` (semver)
- `description`, `maintainers` (text list)
- `is_bundled`: bool
- `registry_source`: `{ kind: bundled | uploaded | git_registry | oci, url? }`

**PolicyModuleVersion** (`db/schema/policy_module_version.go`) — one row per `(module, version)`
- `module` *(edge)*, `version` (semver)
- `manifest_yaml`: raw text (source of truth)
- `enforce_script`: text (required)
- `validate_script`, `rollback_script`: text (nullable)
- `target_os`: list of `linux | windows | macos`
- `scope`: `machine | user | both`
- `runtime`: `bash | python | go | ansible-task`
- `satisfies`: list of policy URNs (denormalized from manifest for index lookup)
- `parameters_schema`: JSON Schema
- `depends_on`: list of `{ module_id, version_constraint }`
- `conflicts`: list of module_id
- `signature`: `{ scheme, signer, blob }` nullable. **In dev**, allowed null with `dev_mode_unsigned: true`. In prod, agent rejects unsigned.
- `approved_for_production`: bool
- `created_by` *(Identity edge; nullable for bundled)*, `created_at`
- `superseded_by_version`: semver (nullable; marks a version EOL)

**PolicyModuleBinding** — links a `PolicyGroup × policy URN` to a chosen module + parameters.
- `policy_group` *(edge)*, `policy_urn`: string
- `module` *(edge)*, `module_version_pinned`: semver (explicit pin)
- `parameter_values`: JSON (validated against the version's `parameters_schema`)
- `disabled`: bool

**Package** (`db/schema/package.go`)
- `id`, `slug` (canonical, e.g. `firefox`), `tenant` *(or null for bundled catalog)*
- `display_name`, `description`, `homepage_url`, `vendor`, `license`
- `categories`: list (Browser / Office / Dev / Security / Multimedia / System / Other)
- `target_os`: list
- `manager_ids`: map `{ manager_slug → install_id }` (e.g. `apt: "firefox"`, `winget: "Mozilla.Firefox"`)
- `vulnerability_state`: nullable JSON; populated in Phase 2+ from a CVE feed (NVD / OSV / distro tracker). Reserved field; OK to leave empty in v1.

**PackageManager** (`db/schema/package_manager.go`)
- `id`, `slug` (`apt | dnf | pacman | flatpak | snap | winget | brew | chocolatey | other`), `tenant`
- `target_os`
- `requires_root`: bool
- `repository_config`: JSON (custom repos, channels, GPG keys)
- `enabled`: bool *(per-tenant default; per-Asset overrides via AssetPackageManagerOverride **TBD; defer until needed)*

**UpdateCycle** (`db/schema/update_cycle.go`)
- `id`, `slug`, `tenant`, `name`, `description`
- `schedule`: `{ kind: cron | window, expr | start_dow_time, duration, frequency }`
- `staged_rollout`: list of `{ percent: int, dwell_hours: int }`
- `pre_hook_script`, `post_hook_script` *(Script edges; nullable)*
- Edges: `assigned_packages` (Package), `assigned_assets` (Asset; or via Group/Site/Tenant inheritance)
- `enabled`: bool

### A3b. Policy Catalog (locked 2026-05-05)

The **Policy Catalog** is the static, versioned list of settings Pluris knows how to express as Linux-native state. It is the *vocabulary* on which `PolicyGroup` entries, `PolicyModuleBinding`s, and GP-import mapping all operate. The catalog lives in code at `pluris/catalog/policies/` and is compiled into the binary — it is not tenant-editable.

**Why catalog-in-code, not catalog-in-DB (v1):**
- Catalog entries are semantic metadata (name, Windows-GP equivalent, category, scope, description). They change on Pluris releases, not on tenant operations.
- Having the catalog compile-in keeps reviewers honest — every new policy is a code change that shows in git.
- `PolicyGroupBinding` / `PolicyModuleBinding` reference catalog entries by stable `policy_urn` string. Renaming or moving categories in code does not break bindings.

**Policy** (catalog entry — Go struct at `pluris/catalog/policies.Policy`)
- `id` — stable dotted slug, lowercase, unique. Example: `sec.account.password.min-length`. This is the `policy_urn` used by bindings. **Never renamed.**
- `name` — Pluris canonical name, Linux-native phrasing.
- `win_gp_name` — exact Windows Group Policy setting name (used by GP import fuzzy match).
- `win_gp_path` — full pipe-separated Windows GP tree path.
- `category` — Pluris category path (parallels `win_gp_path`, trimmed of redundant rungs). The Policy page left-tree is built from these paths.
- `scope` — `computer | user | both` (INV-S: enforced identically to every other scope field in the system).
- `description` — admin-facing explanation, 1–3 sentences; must call out any Linux nuance.
- `linux_impl` — short pointer at the mechanism(s) the module engine (ADR-006) will use. This is metadata, **not** the implementation — the implementation lives in a `PolicyModuleVersion` that declares `satisfies: [<policy.id>]`.

**Canonical UI surface:** `/policy` — mounts `editors/PolicyCatalogBrowser` (v1 is rendered server-side by `web/templates/pages.templ PolicyPage`; full canonical editor lands in Increment 4+). The page must always show:
1. **Policy Group selector** (dropdown, left) — picks which group the catalog bindings are being edited against. Empty selector = catalog browse-only.
2. **Policy quick-search** (datalist over `name`) — locates a policy in the catalog by Linux or Windows-GP name.
3. **Scope filter** (All / Computer Configuration / User Configuration).
4. **Category tree** (left) — collapsible navigator mirroring Windows GPEdit layout. Clicking a leaf scrolls the table to that category.
5. **Catalog table** (right) — grouped by leaf category; each row: Pluris name + stable `id`, Windows GP name + short path, scope chip, description + Linux impl hint.

**Locked invariants:**
- **INV-P-CAT1.** One canonical catalog browser. Policy Group editor, GP Import wizard (Server Administration), and standalone `/policy` page all mount the same `PolicyCatalogBrowser` with at most a `bindingContext` prop applied. No forked "import view" or "browse view" component.
- **INV-P-CAT2.** `policy.id` is immutable across Pluris releases. Deprecation is additive: add a new id, mark the old one with a `superseded_by` pointer (added as a future field; reserved), continue to render the old one in the catalog for at least one minor release.
- **INV-P-CAT3.** Every policy carries `scope`. The UI partitions Computer Configuration / User Configuration exactly as Windows GPEdit does; violations are bugs.
- **INV-P-CAT4.** Catalog ordering is editorial — the source slice order is the display order. Categories appear the first time a policy references them (see `web/templates.groupByCategory`). This gives the maintainer full control over how the page reads without needing a separate ordering field.

**Reserved / deferred fields** (may be added without a §VII.A revision note, since they are additive):
- `parameters_schema` — JSON Schema of the values admins set for this policy. Deferred until the module engine lands — schemas live with `PolicyModuleVersion` today and are denormalised per-policy later.
- `default_value` — recommended secure-baseline default; used by "Apply recommended baseline" button.
- `compliance_tags` — list of compliance frameworks a policy satisfies (`CIS-L1`, `STIG`, `PCI-DSS-8.3`, …).
- `docs_url` — link to Pluris docs for this policy.
- `superseded_by` — forward pointer to a replacement policy id.

**Catalog size:** v1 ships ~170 curated entries across the major Windows GP categories (Account Policies, Local Policies, Windows Defender Firewall, Public Key Policies, Application Control, Scripts, Administrative Templates → System/Network/Windows Components, User Configuration → Control Panel / Desktop / Start Menu / System / Windows Components, Group Policy Preferences → Registry / Files / Folders / Shortcuts / Drive Maps / Environment / Scheduled Tasks / Printers / Power Options). Entries are expected to grow — adding a policy is a single struct literal in `catalog_computer.go` or `catalog_user.go` and costs nothing at runtime.

### A4. Dashboard + Role

**Dashboard** (`db/schema/dashboard.go`)
- `id`, `tenant`, `owner` *(Identity edge)*, `name`, `is_default`: bool
- One per user (per UX plan). Admin-shared dashboards deferred (Phase 2+).

**DashboardTile** (`db/schema/dashboard_tile.go`)
- `id`, `dashboard` *(edge)*, `position`: `{ row, col, width, height }` (responsive grid; sizes: small/medium/large/xlarge)
- `title`: text
- `data_source`: `{ kind: tenant|site|group|asset|identity|aggregate, id? }` (uses the shared hierarchical picker)
- `query`: JSON DSL (locked v1 form):
  ```
  { "from": "asset|identity|policy_group|policy_module|package|update_cycle|...",
    "filter": {<field>: <value or operator>},
    "aggregate": "count|sum|avg|distinct|none",
    "group_by": [<field>, ...]    // optional
  }
  ```
  A curated **preset query library** is shipped (e.g. "Assets unseen >24h", "Pending updates by site", "Compliance state distribution") that admins pick from. Custom queries are unrestricted JSON DSL.
- `display_type`: `graph | text | update_view | table | count | status_grid`
- `refresh_interval_seconds`: nullable; null = on-load only; min 30s
- `editable`: bool (false for tiles imposed by admin defaults)
- `created_by`, `created_at`

**Role** (`db/schema/role.go`) — built-in v1; custom roles deferred to Phase 2+.
- `id`, `slug`: `super_admin | admin | user_self_service`
- `description`, `is_builtin`: bool
- `permissions`: JSON (per-menu-item RW/R/none matrix; locked initial values below)

**Role permission matrix (v1, locked)**

| Menu | super_admin | admin | user_self_service |
|---|---|---|---|
| Dashboard | RW (cross-tenant) | RW (own tenant) | R (own dashboard, fixed presets) |
| Users | RW (all tenants) | RW (own tenant) | R (own Identity) + RW preferences |
| Assets | RW | RW | R (assets paired with self) |
| Policy | RW | RW | R (effective policy on self / own assets) |
| Profiles | RW | RW | R (profiles applied to self) |
| Scripts | RW | RW | — |
| Policy → Modules | RW | RW | — |
| Wine | RW | RW | R (own assets only) |
| Package Management | RW | RW | R (installed-on-self view) |
| Server Administration | RW | RW (own tenant) | — |
| User/Admin Preferences | RW | RW (own + admin section) | RW (own only) |

`super_admin` is **cross-tenant**, intended for hosted / multi-tenant deployments. Single-tenant deployments may collapse super_admin and admin to the same effective role.

---

## VII.B Policy Module engine invariants (locked 2026-05-06; codified in ADR-007)

These govern the runtime contract between the Configuration Group editor, the Policy Module catalog, and the agent. Violations are bugs.

**INV-M1.** A module's `uninstall` lifecycle script runs **at most once** per `ModuleInstallation` row, and **only when refcount drops to zero**. Refcount = count of incoming `installed_via` edges where the source is not `orphaned`.

**INV-M2.** The module dependency graph is acyclic. The server rejects any manifest publish that would introduce a cycle. The dependency resolver is the same package on server and (offline-verified) on agent.

**INV-M3.** A Configuration Group binding cannot be saved if the chosen module has unmet dependencies in the tenant's module catalog. The save action surfaces the missing-dep set; the user resolves by either picking a module that brings them, importing the missing module, or removing the binding.

**INV-M4.** A `ModuleInstallation` row's `installed_via` edge set is never empty. An empty set is the trigger that physically uninstalls (runs `uninstall`) and deletes the row. Bindings can only add or remove edges; they cannot leave a dangling row.

**INV-M5.** Lifecycle scripts (`apply`, `disable`, `uninstall`) are bash; predicates (`validate`, `report`) are WASM (wasmtime). No other runtimes are accepted in v1. Adding a runtime requires an ADR.

**INV-M6.** Module bytes are never persisted to non-tmpfs storage on the agent. The runtime decrypts AEAD-sealed bundles into memfd / mlock'd memory, executes from per-exec tmpfs at `/run/pluris/exec/$exec_id`, and unmounts + zeros buffers at exit. Violations are detectable via the agent's audit log (which records mount points it created and unmounted).

**INV-M7.** Every module is signed by its tenant's Ed25519 key (or by the bundled-Pluris key for shipped modules). The agent rejects bundles signed by any key not in its enrollment-time pinned trust set. Sigstore/Rekor is an opt-in alternate root for tenants that need public attestation; the verification interface is the same.

**INV-M8.** A module's declared `capabilities` (filesystem read/write paths, network egress, target user) define the sandbox profile applied at exec. The agent enforces the profile via bwrap + Landlock + seccomp regardless of script content. Default-deny on FS write outside `PLURIS_TMPDIR` and on network egress.

**INV-M9.** A Custom Policy entry **must** have at least one tenant-authored module that satisfies it before it can be referenced from a Configuration Group binding. The wizard enforces this by combining policy creation and module authoring into a single transaction.

**INV-M10.** The Policy Catalog list **does not split bundled and custom policies into separate tabs**. Both render in the same tree, sorted by category. Custom entries carry a `Custom` chip and originate from the tenant's space; bundled entries are read-only and shipped in code (see §A3b).

**INV-M11.** *(Resolution order — locked 2026-05-16)* Effective module for a `(binding, policy_urn, device_os)` triple is decided in **exactly** this order, with stale levels skipped and the search continuing:

```
1. binding.ModuleOverride       (per-binding pin set in CG editor)
2. TenantDefault(tenant, urn)   (tenant-wide default; /policy/modules/defaults)
3. Pluris default                (first bundled module in catalog order
                                  that Satisfies(urn) AND SupportsOS(device_os))
```

A level is "stale" when its referenced module is missing, no longer satisfies the URN, doesn't support the device OS, or its pinned version is `draft`/`revoked`. Stale levels silently fall through; the UI surfaces staleness on the binding row with a warning chip. If all three levels are stale, the binding is **unsatisfiable** — the CG editor must block save with the explicit reason (UI must show "no compatible module for *urn* on *os*"). Implemented in `pluris/catalog/policymodules.ResolveBindingModule`.

**INV-M12.** *(Canonical picker — locked 2026-05-16)* Module selection for a binding goes through **one** dialog: `editors/PolicyModulePicker` (templ component `policyModulePickerDialog` mounted once per page). Every mount point — CG editor binding row, Profile editor binding row, Custom Policy Wizard step 5, Library row "Pick" button — opens this same `<dialog>` with `data-pmp-*` attributes hydrated from the call site. Branching to a sibling picker is forbidden (R1, R2). The picker is not an editor; "Edit module ✎" inside it delegates to `editors/PolicyModuleEditor`.

**INV-M13.** *(Policy Modules page IA — locked 2026-05-16)* The `/policy/modules` page has exactly three sub-views, each a stable route:

| Route | View | What it does |
|---|---|---|
| `/policy/modules` | **Library** | Lists every module visible to the tenant (bundled ∪ tenant-authored ∪ imported-and-fetched). One row per module; origin chip; row actions Pick / Edit / Clone / Disable / Delete. |
| `/policy/modules/defaults` | **Tenant defaults** | One row per `(tenant, policy_urn)` with a non-null tenant default. Editing here changes the resolution chain for every binding of that URN unless the binding overrides. |
| `/policy/modules/sources` | **Sources** | Lists module sources: Pluris-bundled (always on, read-only), Tenant-local (always on, the tenant's own), Imported registries (Phase 4; explicit fetch + diff approval). |

Adding a fourth sub-view requires an ADR update. The Library route is the page's default; the Policy → Modules sidebar link routes there. Legacy `/scripts/policy-modules` 301-redirects to `/policy/modules` and must remain so until at least 2027-05.

----

## VII.C List & column registry invariants (locked 2026-05-06)

These govern every tabular list in the admin console — Policy Catalog, Configuration Groups, Computers, Users, Profiles, future. Violations are bugs.

**INV-L1. One field registry per list, one cell renderer per field.** Every list registers its `FieldDef`s in `pluris/web/lists/` exactly once. Adding a new column is two co-located edits (entry in `Fields` + case in `RenderXCell`); never a templ change. Two registrations of the same list ID is a panic at startup (see `lists.Register`).

**INV-L2. The detail dialog and the list table read the same registry.** A field that exists in the list's registry is shown in the detail dialog (everything visible) and is **eligible** to appear as a column. The picker only controls **which** registry fields appear in the list table; the detail dialog always shows the full set. This is what makes "every value visible after opening" exposable as a column with zero per-list code.

**INV-L3. Visible-column state is per-user, not per-tenant.** v1 persists in `localStorage` keyed by `pluris.cols.<listId>`; Phase 1 migrates to a per-user DB row without changing the picker UI or the registry. Server-side renderers always emit every cell — the picker only toggles `style.display` — so a user with no saved preference sees the registry's `DefaultVisible` set, identical to the previous hard-coded UI.

**INV-L4. Field keys are stable identifiers.** Renaming a key is a migration (old key → new key in the localStorage upgrade path); silently changing semantics under an existing key is forbidden. Group keys (`identity`, `windows-gp`, `linux`, `modular`, …) follow the same rule.

**INV-L5. The picker is a generic component.** `templates.ColumnPickerButton(listID)` mounts a popover for any registered list. New lists do not author their own picker; they `@ColumnPickerButton(lists.ListIDXxx)` and the framework reads `lists.GroupsFor`/`FieldsByGroup` to populate it.

**INV-L6. Column widths are declared, not styled.** `FieldDef.Width` carries the CSS width hint; the column-picker init reads `data-width` from each `<th>` and applies `style.width` once at startup. (templ rejects dynamic `style=` attrs, so this is the structural workaround.) Widths are CSS percentages so the table reflows when columns toggle.

**INV-L7. Detail-only fields exist.** Setting `FieldDef.DetailOnly=true` means the field is shown in the detail dialog but **never** in the list (verbose source previews, multi-line markdown, etc.). The picker hides ineligible entries via `lists.ListEligibleFields`.

**INV-L9. One unified filter / sort / divider engine.** Every list table in the admin console wires its filter inputs, sort buttons, divider visibility, count chip, empty state, and search highlighting through the **single** static asset pair `web/static/lists.js` + `web/static/lists.css`. New lists do not author their own filter or sort script — duplicating the engine is a bug. The HTML contract:

- Section: `<… data-pluris-list="<listId>">` wraps either the toolbar, the table, or both. (The engine matches by `data-list-id` on the `<table>` to find rows; the section attribute is the discovery anchor.)
- Filter control: `<input|select data-pluris-filter="<listId>" data-filter-attr="<rowAttr>" data-filter-mode="contains|equals|prefix|numGte">`. The engine reads `row.dataset[camelCase(rowAttr)]`; rows must emit a matching `data-<row-attr>` attribute. Add `data-pluris-highlight="1"` to wrap matched substrings in `<mark class="pf-mark">`.
- Table: `<table class="pluris-list" data-list-id="<listId>">`. Sortable headers add `class="th-sortable"`, `data-field="<key>"`, `data-sort-type="alpha|num"`, and contain `<button data-sort-key="<key>">`; cells add `data-field="<key>"` and `data-sort="<sortValue>"`.
- Empty state: `<… data-pluris-empty="<listId>" hidden>` toggled visible when zero rows match.
- Count chip: `<… data-pluris-count="<listId>" data-template="{visible} of {total}">`.
- Tree-leaf integration: `<a data-pluris-tree="<listId>" href="#<dividerId>">` on category-tree leaves; the engine clears the active sort when a leaf is clicked (so the anchor target is in DOM order) and hides leaves whose target divider is filtered to zero.
- Persistence: filter state in `localStorage["pluris.filter.<listId>"]`, sort state in `localStorage["pluris.sort.<listId>"]`. Keys must follow this pattern.
- Events: the engine fires `pluris:list-filtered` and `pluris:list-reordered` (`detail = { listId, … }`) on `document`. List-specific code that reacts to row reordering subscribes to these — never to scroll, observer, or polling alternatives.

A list MAY skip the engine only when the table is a private dialog component with bespoke selection logic (e.g. the target picker's allowed-kinds restriction); such exemptions are documented in the file that defines the component and limited to a single `<table>` instance.

----

## VII.D Onboarding & in-product orientation invariants (locked 2026-05-09, revised 2026-05-16)

The target audience is L1/L2 Windows admins migrating from AD + GP + Intune. AD/GP equivalence appears in **contextual surfaces** (cell tooltips, detail dialogs, docs) rather than as always-on chrome. Sidebar items and page titles are self-explanatory; permanent orientation banners and sidebar subtitles were removed after review (2026-05-16) as redundant.

**INV-O1. No bare Linux jargon in user-facing copy.** Every distinct Linux noun shown to the admin (`pam_pwquality`, `apparmor`, `dconf`, `systemd-resolved`, …) resolves through `web/glossary/glossary.go`. The templ helper `@Term("<key>")` and the lists-side `glossifyTokens()` wrap the token in a `<span class="term">` carrying the OneLine + ADEquivalent in the `title` attribute. Unknown tokens render as plain text — they degrade gracefully but the lint test `TestEveryTermHasShape` requires every registered term to have non-empty OneLine and ADEquivalent.

**INV-O2. Empty-state contract.** Every list whose Concept defines `EmptyTitle` / `EmptyHelp` mounts `@ConceptEmptyState("<conceptKey>", listID)` as a child of its `data-pluris-list` section. The lists.js engine toggles visibility via the `[data-pluris-empty="<listID>"]` attribute (INV-L9); `@ConceptEmptyState` supplies the *content*. Empty states must contain (a) a noun-led heading ("No …"), (b) a one-paragraph explanation framed against the Windows admin's mental model, and (c) at most one primary CTA. No secondary actions in empty states.

**INV-O3. Docs mirror code; gendocs is canon.** `docs/for-windows-admins/concepts.md` and `docs/for-windows-admins/glossary.md` are auto-generated from `web/orientation/` and `web/glossary/` by `go run ./cmd/gendocs`. Editing those files by hand is forbidden — edit the data file, regenerate. The hand-authored partners (`README.md`, `cheatsheet.md`) live in the same directory but the generator never touches them. CI gate: `go run ./cmd/gendocs && git diff --exit-code docs/for-windows-admins/concepts.md docs/for-windows-admins/glossary.md` must be clean.

`web/orientation/orientation.go` remains the source of truth for the **Concept** metadata that drives ConceptEmptyState and the docs generator. The `SidebarHint` field on `Concept` is no longer read by any UI surface (kept for future docs use); a sidebar item's label is its only chrome.

**INV-L8. One table per list. Visual grouping is divider rows, not separate tables.** Every list renders **exactly one** `<table>` with **one** `<thead>`. Category / section grouping (e.g. the policy catalog's tree path) is conveyed by `<tr class="cat-divider" data-divider="1" data-row-index="N" id="<anchor>">` rows interleaved between data rows; the divider's `<td colspan="99">` makes it span the full width. **No separate `<table>` per group.** Splitting one logical list into multiple tables breaks header sorting, breaks multi-column scanning, breaks filter counts, and forces users to re-orient at every group break — it is forbidden. The sort engine hides dividers automatically when a sort is active (grouping no longer applies); the filter engine hides dividers whose run has zero visible rows. The natural-order restore key is `data-row-index` (server-rendered editorial order). Every data row carries `data-<entity>-id` (`data-policy-id`, `data-computer-id`, …) so scripts can distinguish data rows from divider rows reliably.

----

## VII.E Extension Framework invariants (locked 2026-05-17; codified in ADR-008)

The five user-supplied content families — Policy Modules, Profiles, Scripts, Wine Configurations, Packages — share one shape (id, title, source, lifecycle, versions, signatures). `pkg/extension/` defines that shape; each concrete kind registers itself and provides an adapter from its domain type to `extension.Extension`. Cross-kind chrome (Sources page, global search, audit log, "browse extensions" surface) reads through the framework only.

**INV-X1. One framework, one entry per kind.** Every user-supplied content family lives in `catalog/<kind>/` and registers via `extension.RegisterKind` from `init()`. Adding a parallel package, a `*_simple.go` sibling, or a per-kind clone of the Sources/picker code is forbidden — extend the canonical kind. The lint check `TestKindsRegistered` walks `catalog/` and asserts every package containing a domain entity also calls `RegisterKind`.

**INV-X2. Sources pages read through the framework.** Any page that shows "how many of each Source do you have?" calls `extension.CountBySource(kind)` (or `CountBySource("")` for the all-kinds total). Templates do not enumerate `SourceBundled` / `SourceTenant` / `SourceImported` / `SourceCommunity` from per-kind packages, and they do not maintain their own counters. The Policy Modules Sources sub-page (`/policy/modules/sources`) is the reference implementation; new kinds reuse the same template with a `Kind` parameter.

**INV-X3. Lifecycle and signature semantics are framework-defined.** A version's `LifecycleState` and `Signature` are interpreted by the framework's predicates (`IsDeployable`, `IsTerminal`, `IsZero`); concrete kinds must not redefine those semantics. Specifically: only `published` and `superseded` versions are deployable; `revoked` is terminal and one-way; every non-draft version has `Signature.IsZero() == false` (the trust chip renders from `Signer` + `KeyID` even when the v1 mock leaves `Bytes` empty). New kinds that need additional states extend the framework, not their own enum.

**INV-X4. Cross-kind UI iterates registered kinds.** Surfaces that span kinds (global search, audit log filters, the future "Browse Extensions" page, dashboard tiles whose source is "extension activity") iterate `extension.RegisteredKinds()` and read each kind's `KindSpec.Title` / `Description` for display copy. Hard-coding the list of kinds anywhere outside `pkg/extension/types.go` (the `Kind` constants) is forbidden — adding a kind must not require touching cross-kind chrome.

----

## VIII. Process

**INV-P1. Slow down before code.** For any new top-level menu item, entity, or editor, the IA contract is written and confirmed with the user before scaffolding code. The IA contract has these fields and lives as a row in §VII:
- entity name and fields (or "reuses X")
- scope (machine | user | both | n/a)
- canonical editor file path
- every entry point and the filter it passes
- AD/GP equivalent (or "n/a")
- permission gates per role

**INV-P2. Mount-point tests.** Each canonical editor has a test asserting that every entry point listed in §VII mounts it. Adding an entry point ⇒ extending the test ⇒ then implementing the handler.

**INV-P3. Documentation precedence.** This file and `Pluris UX structure plan.md` outrank every other doc. When other docs (`MANAGEMENT_PLATFORM_RESEARCH.md`, `MANAGEMENT_INTERFACE.md`, `GROUP_POLICY_COMPATIBILITY.md`, `FORK_STRATEGY.md`) disagree, those docs are stale and must be corrected.

---

## IX. Out of scope of this document

- Visual styling (covered by `docs/BRANDING_GUIDE.md`).
- Linux-distro UX (separate product; covered by `docs/DEVELOPMENT_PLAN.md`, `docs/QUICKSTART.md`, etc.).
- Backend architecture beyond IA implications (covered by `docs/ARCHITECTURE_DECISIONS.md`).
- Endpoint enforcement details (covered by `docs/ARCHITECTURE_DECISIONS.md` ADR-003 and `docs/GROUP_POLICY_COMPATIBILITY.md`).

---

## INV-CPP — Canonical Parameter Paths (locked 2026-07-05)

Everything in Pluris that names a parameter does so through one canonical,
hierarchical path: `<entity>/<section>/<param>` (lowercase snake, forward
slashes). Examples: `user/identity/email`, `user/security/account_locked`,
`computer/hardware/ram_mb`, `computer/enrollment/last_seen_at`.

1. **Derived, not duplicated.** Paths are derived from the `catalog/params`
   registry: entity = the schema's `PathEntity` slug, section =
   `SchemaSection.Key`, param = `ParamDef.Key`. The registry stays the single
   source of truth; CPP is its addressing scheme. No parallel path table
   exists anywhere.
2. **Entity slugs are registered in one place** (`catalog/params`):
   `computer`, `server`, `printer`, `desk`, `user`. `user` maps to the
   existing `SchemaIdentity` (internal subtype key remains `identity`; the
   slug is the `PathEntity` field on `SubtypeSchema`). Future entities
   (`policy`, `config_group`, `module`, `wine_group`, `script`, `package`)
   join the same registry as their schemas are created.
3. **Uniqueness is enforced** by an init-time check + test: no two
   `(entity, section, param)` triples may collide, and every mounted param
   resolves to exactly one path per entity.
4. **Every surface uses paths**: row/detail `data-path` attributes, filter
   configs, the Configuration tab's policy-set value references,
   import/export, and the future API all address fields by path. Shared
   params (e.g. `ram_mb` on both computer and server) keep one `ParamDef`
   but get one path per mounting entity — `computer/hardware/ram_mb` and
   `server/hardware/ram_mb` both resolve to the same definition.
5. **New parameters anywhere in the project MUST be registered in
   `catalog/params` first** and therefore automatically get a path. A param
   not in the registry cannot appear in any UI, filter, config, or export
   (the existing R1/INV-L rule, extended to paths).

Go API (`catalog/params/paths.go`): `PathFor(entity, key string) string`,
`ResolvePath(path string) (*SubtypeSchema, *SchemaSection, *ParamDef, error)`
(fails closed), `SchemaByPathEntity(entity string) *SubtypeSchema`, and
`AllPaths() []string` (sorted; used by tests, export tooling, future API).
