# Standardized Detail Pages, Canonical Parameter Paths & Roles — Design Spec

**Date**: 2026-07-05
**Status**: awaiting owner confirmation
**Supersedes**: the popup-based Policy Catalog detail dialog; the flat (tab-less) Asset and User detail pages.
**Mockups reviewed**: `.superpowers/brainstorm/212728-1783243505/content/computer-detail.html` and `user-detail.html` (approved shape, with corrections incorporated below).

---

## 0. The most important rule: Canonical Parameter Paths (CPP)

Everything in Pluris that names a parameter does so through one canonical,
hierarchical path:

```
<entity>/<section>/<param>
```

Examples:

| Path | Meaning | AD equivalent |
|---|---|---|
| `user/identity/email` | User's primary email | `mail` |
| `user/security/account_locked` | Lockout state | `lockoutTime` |
| `computer/hardware/ram_mb` | Installed RAM | — |
| `computer/enrollment/last_seen_at` | Agent heartbeat | `lastLogonTimestamp` |
| `printer/consumables/toner_level` | Toner level | — |

Rules (these become a new locked invariant, **INV-CPP**, appended to
`docs/UX_INVARIANTS.md`):

1. **Derived, not duplicated.** Paths are derived from the existing
   `catalog/params` registry: entity = the schema's path-entity slug,
   section = `SchemaSection.Key`, param = `ParamDef.Key`. The registry
   stays the single source of truth; CPP is its addressing scheme. No
   parallel path table exists anywhere.
2. **Entity slugs** are registered in one place (`catalog/params`):
   `computer`, `server`, `printer`, `desk`, `user`. (`user` maps to the
   existing `SchemaIdentity`, whose internal subtype key remains
   `identity` — the slug is a new `PathEntity` field on `SubtypeSchema`,
   so no ripple through existing filter/list code.) Future entities
   (`policy`, `config_group`, `module`, `wine_group`, `script`,
   `package`) join the same registry as their schemas are created.
3. **Uniqueness is enforced** by an init-time check + test: no two
   `(entity, section, param)` triples may collide, and every mounted
   param resolves to exactly one path per entity.
4. **Every surface uses paths**: row/detail `data-*` attributes gain a
   `data-path` per field, filter configs key filters by path, the
   Configuration tab references policy-set values by path, import/export
   and the future API address fields by path. Shared params (e.g.
   `ram_mb` mounted by both computer and server) keep one `ParamDef` but
   have one path per entity that mounts them — `computer/hardware/ram_mb`
   and `server/hardware/ram_mb` both resolve to the same definition.
5. **New parameters anywhere in the project MUST be registered in
   `catalog/params` first** and therefore automatically get a path.
   A param not in the registry cannot appear in any UI, filter, config
   or export. (This is the existing R1/INV-L rule, extended to paths.)

Go API added to `catalog/params`:

```go
// PathFor returns "user/identity/email" for (entity="user", key="email"),
// or "" if the entity doesn't mount that key.
func PathFor(entity, key string) string

// ResolvePath("user/identity/email") -> (schema, section, def, error).
// Fails closed on unknown entity, section, or param.
func ResolvePath(path string) (*SubtypeSchema, *SchemaSection, *ParamDef, error)

// AllPaths returns every registered path, sorted — used by tests,
// export tooling, and the (future) API surface.
func AllPaths() []string
```

## 1. Standardized detail-page shell (one component, every entity)

One templ component set replaces today's three divergent patterns
(bespoke policy dialog, flat asset page, flat user page):

```
┌──────────────────────────────────────────────────────────────┐
│ Breadcrumb (Assets › Computers › dev-laptop-001)             │
│ ┌──────────────────────────────────────┐  ┌───────────────┐  │
│ │ NAME  (24px, bold)                   │  │   VISUAL      │  │
│ │ stable-id (mono, muted)              │  │ (OS logo /    │  │
│ │ [chip] [chip]                        │  │  avatar /     │  │
│ │ Site · Owner · OS · Last seen (defs) │  │  policy icon) │  │
│ └──────────────────────────────────────┘  └───────────────┘  │
├──────────────────────────────────────────────────────────────┤
│ [General] [Tab2] [Tab3] …            ← real switching tabs   │
├──────────────────────────────────────────────────────────────┤
│ tab panel content (schema sections OR a standardized table)  │
└──────────────────────────────────────────────────────────────┘
```

- **Templates**: `web/templates/detail_shell.templ` —
  `DetailShell(hero HeroSpec, tabs []TabSpec)` where each `TabSpec` has
  a stable slug, label, and a templ.Component body. The existing
  `.asset-detail-*` CSS classes in `layout.templ` are reused as-is (they
  are already the right visual); they are documented as the standard
  shell classes. Renaming them to `.detail-*` is cosmetic debt, not done
  now.
- **Tab switching**: one small shared static file `web/static/detail.js`
  (same INV-L9 discipline as `lists.js`): server renders all panels,
  JS toggles visibility, active tab syncs to `location.hash`
  (`#groups`) so tabs are deep-linkable and survive refresh. No
  per-page tab scripts.
- **Tab content types** are exactly two, both standardized:
  1. **Schema sections** (the General tab): driven by
     `catalog/params` sections, rendered by the existing
     `buildDetailSections` mechanism generalized to any entity. Each
     field's `<dd>` carries `data-path`.
  2. **Standardized table**: see §2.

## 2. Standardized embedded table (real data only)

Every tab that lists related records uses one table mechanism — the
existing `web/lists` FieldDef registry + `pm-table` styling — via a new
templ helper:

- Each tab table registers a list ID in `web/lists` (e.g.
  `computer-groups`, `applied-policies`, `user-roles`,
  `installed-software`), with columns declared as FieldDefs. **No
  hand-rolled `<table>` markup per tab.**
- One helper `DetailTable(listID string, header actions, rows)` renders:
  registry-driven `<thead>`, rows from a real service call, a single
  primary action button in the table header (e.g. **"Add from
  catalog"**, **"Add role"**), and per-row actions (Remove/…). Empty
  data renders the standard `ConceptEmptyState` inside the same table
  frame — never a bespoke placeholder.
- **One action button per table.** No duplicated apply/save buttons.
  Add/remove operations are immediate (with a confirm prompt for
  destructive ones); there is no page-level "apply all" state.

## 3. Computer (Asset) detail page

Route stays `/assets/:subtype/:id`. Hero: name, human ID, enrollment +
lifecycle chips, Site/Owner/OS/Last-seen defs, device visual with OS
logo (existing `assetDetailDeviceIcon`). Tabs (slugs fixed):

| Tab (slug) | Content | Data source — honest status |
|---|---|---|
| General (`general`) | Schema sections: Identity, Hardware, Enrollment, Lifecycle + new fields below | **Live** (existing params + new ones) |
| Groups (`groups`) | DetailTable `computer-groups`: Group, Category, Scope, Source (Direct/Inherited) + Add/Remove | **Live** — `group_memberships`/`groups` tables exist; this phase adds the queries + UI |
| Applied Policies (`policies`) | DetailTable `applied-policies`: Policy, Source (config group), Scope, Value summary, Status + **Add from catalog** | **Live** — reads `configuration_group_assignments` (see §6) |
| Configuration (`configuration`) | DetailTable `asset-configuration`: Setting (by CPP path), Policy value, Reported value, Status, Override | **Live for policy values**; Reported value shows "awaiting agent" until the agent exists; Override button rendered but disabled with an explanatory tooltip until the permission editor ships (§5) |
| Installed Software (`software`) | DetailTable `installed-software`: Name, Version, Publisher, Type (deb/appimage/wine/native), Installed, Size | **Structured placeholder** — table + columns registered, standard empty state; data arrives with the agent inventory. Schema table `installed_software` created now so seeding/testing works |
| Wine Groups (`wine-groups`) | DetailTable `asset-wine-groups`: Group, Wine version, Arch, Applications, Assigned | **Structured placeholder** — Wine feature not built; columns locked per `UX_INVARIANTS` WineConfigGroup spec |
| Assigned Scripts (`scripts`) | DetailTable `asset-scripts`: Script, Trigger, Last run, Status, Next run | **Structured placeholder** — Scripts feature not built |
| Logs (`logs`) | DetailTable `asset-logs`: Time, Event, Detail, Actor | **Live minimal** — new generic `activity_log` table (tenant, entity_type, entity_id, event, detail, actor) written by owner-changes/group-changes/policy-assignments from day one; agent events append later |

New Computer parameters (registered in `catalog/params`, mounted in the
computer/server schemas, AD equivalents shown in the detail UI):

- `description` (AD `description`) — new `assets.description` column
- `managed_by` (AD `managedBy`) — new `assets.managed_by_identity_id`
  column, link-type param to user
- (identity registry) `initials` (AD `initials`) — column already
  exists, param was missing

## 4. User detail page

Route stays `/users/:id`. Hero: display name, UPN, role + enabled
chips, Dept/Site/Title/Manager defs, avatar circle (initials until
`thumbnailPhoto` import exists). Tabs:

| Tab (slug) | Content | Status |
|---|---|---|
| General (`general`) | All 8 existing schema sections (Identity, Organization, Contact, Location, Profile & Scripts, Security, Preferences, Metadata), AD-attribute hints shown next to labels | **Live** |
| Groups (`groups`) | DetailTable `user-groups`, same shape as computer-groups | **Live** |
| Applied Policies (`policies`) | DetailTable `applied-policies` (same list ID as computers — same columns, same component) + **Add from catalog** | **Live** |
| Roles (`roles`) | DetailTable `user-roles`: Role, Type (Built-in/Custom), Description, Assigned at + **Add role** button + per-row Remove. **No permission checkboxes here** — assignment only | **Live** (§5) |

## 5. Roles model (assignment now, permission editor later)

- New tables (migration, see §8):
  - `roles`: id, tenant_id, slug, name, description, `is_builtin`,
    `template_slug` (nullable — which builtin a custom role was cloned
    from), `permissions` JSON (reserved; the separate permission-editor
    design will define its schema — nothing reads it yet).
  - `identity_roles`: identity_id, role_id, assigned_at, assigned_by.
- **Four built-in roles seeded per tenant**: `super_admin`, `admin`,
  `technician` (new), `user` (renames `user_self_service`; migration
  updates existing rows and the CHECK constraint; Go constants and the
  RBAC matrix updated accordingly).
- **Coarse enforcement mapping** (until the permission editor exists):
  `technician` = `admin` minus Server Administration and minus
  role-assignment rights. The route-prefix matrix in `pkg/auth/rbac.go`
  gains a `technician` column. Custom roles enforce as their
  `template_slug` builtin.
- `identity_roles` is authoritative; `identities.role` remains as a
  derived cache holding the highest-privilege assigned role (used on
  the login hot path), updated on every assignment change and
  backfilled by the migration.
- **Deferred to a separate design** (explicitly out of scope here): the
  permission editor UI (granular capability checkboxes, creating custom
  roles from templates, the `permissions` JSON schema, and enforcement
  of fine-grained capabilities such as *Assets → Override policy-set
  configuration*). The Roles tab links to it with a disabled
  "Manage role permissions" affordance labelled "coming with the
  permission editor".

## 6. Applied Policies & Add-from-catalog

**Read path** (both computers and users): resolve all
`configuration_group_assignments` whose target matches the entity
directly (`asset`/`identity`) or via inheritance (its groups, its site,
its tenant) → their configuration groups → bindings → policy catalog
entries. Each row: Policy display name, Source config group, Scope,
value summary (params by CPP path), Status. Status vocabulary now:
`Assigned` (no agent yet) / `Disabled`; `Applied`/`Drift`/`Error`
activate when the agent lands (same enum, forward-compatible).

**Write path ("Add from catalog")**: one button on the Applied Policies
table. Flow: standardized picker (the policy catalog list filtered,
reusing the existing lists engine) → parameter form for the chosen
policy's params → creates a direct configuration group named
`Direct — <entity name>` (reused if it already exists for that target)
with the binding, plus an assignment targeting this asset/user.
Everything created is a real DB row; the Policy → Configuration Groups
page will show it (that page's own DB unification is tracked separately
in PROGRESS.md and not blocked by this).

## 7. Policy Catalog detail pages (popup removed)

- Every catalog entry gets `/policy/catalog/:id` rendered with the same
  DetailShell. Hero: policy display name, URN (mono), category + scope
  chips, Windows GP path as a def row. Tabs:
  - **General** (`general`): definition fields — description, GP
    equivalent + path, Linux mechanism, parameters (each with CPP-style
    param addressing).
  - **Modules** (`modules`): DetailTable `policy-modules-for-policy` —
    candidate modules that satisfy this policy (origin, version, status,
    target OS), row-click navigates to the module (module detail pages
    stay as they are for now).
  - **Assignments** (`assignments`): DetailTable — configuration groups
    binding this policy and their targets. **Live** from the same data
    as §6.
- The catalog list's row-click navigates to the page (same pattern as
  assets/users lists); the `pdd-dialog` popup and its bespoke script are
  deleted. Custom-policy editing (the wizard) is reachable from the
  detail page's single header action for `origin=custom` policies.

## 8. Migrations & data plumbing

- **`schema_migrations` tracking table** (filename, applied_at) added to
  `pkg/database.migrate()`: each migration file runs exactly once.
  Required because this design introduces `ALTER TABLE` statements,
  which are not idempotent under the current run-every-start scheme —
  and it permanently closes the bug class behind the July restart
  data-wipe incident. Existing installs: 001/002 are marked applied if
  their tables already exist.
- **Migration 003** (new): `roles`, `identity_roles`,
  `installed_software`, `activity_log` tables; `assets.description`,
  `assets.managed_by_identity_id` columns; role value migration
  `user_self_service`→`user` + CHECK update + `technician`.
- **Seeder**: `cmd/seed` gains `-tenant <slug>` (default: the first
  existing tenant) so demo data lands in the tenant created by `/setup`
  — closing the "fresh install has zero assets" gap. Seeds additionally:
  2 groups with memberships, 1 configuration group with a binding +
  assignment, and a handful of `installed_software` rows, so every live
  tab has visible data for GUI testing.
- sqlc queries for: groups/memberships CRUD-lite (list for entity,
  add, remove), roles + identity_roles, assignments resolution,
  activity_log append/list, installed_software list.

## 9. Explicitly out of scope (tracked, not forgotten)

- Permission editor (granular zero-trust capability UI) — **next design
  after this ships**; the `permissions` JSON column and disabled
  Override/Manage-permissions affordances are its landing pads.
- Agent-sourced data: installed-software inventory, reported
  configuration values, drift detection, log streaming.
- Wine feature implementation and Scripts execution (tabs are
  structured placeholders with locked columns).
- Module detail page redesign; Configuration Groups list-page DB
  unification (separate, already-tracked task).
- AD import itself (fields are AD-compatible by design; the importer is
  a later phase).

## 10. Implementation phases (one plan, ordered)

1. **Foundations**: `schema_migrations` tracker; migration 003; CPP
   (`PathFor`/`ResolvePath`/`AllPaths` + `PathEntity` + tests); new
   params (`description`, `managed_by`, `initials`); seeder `-tenant`
   flag + expanded seed data.
2. **Shell**: `DetailShell`/`DetailTable` templ components +
   `web/static/detail.js`; rebuild Computer and User detail pages on it
   (General tabs live; placeholder tabs structured).
3. **Groups + Roles**: membership queries + Groups tabs (add/remove);
   roles tables, seeding, RBAC `technician`, Roles tab (add/remove).
4. **Policies**: assignment resolution service; Applied Policies tabs;
   Add-from-catalog flow; Configuration tab (policy values by path,
   disabled Override); activity_log wiring for these mutations.
5. **Policy pages**: `/policy/catalog/:id` DetailShell pages; delete the
   popup dialog + script; catalog row-click navigation.

Each phase ends green (full test suite + live smoke test) and is
independently shippable.
