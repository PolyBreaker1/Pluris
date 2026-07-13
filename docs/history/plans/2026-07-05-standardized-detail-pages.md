# Standardized Detail Pages, CPP & Roles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One standardized hero+tabs detail shell for Computers, Users, and Policy Catalog entries; canonical parameter paths (`entity/section/param`) as the project-wide addressing scheme; a roles model (4 builtins + custom-from-template, assignment-only UI); real Groups/Applied-Policies/Roles/Logs tabs backed by the database; migration tracking; seeder pointed at the real tenant.

**Architecture:** Extends `catalog/params` (single source of truth) with path derivation; one `DetailShell`/`DetailTable` templ component pair + one shared `detail.js` replaces the bespoke policy dialog and the flat asset/user pages; new tables (`roles`, `identity_roles`, `installed_software`, `activity_log`, `schema_migrations`) ride a new run-once migration runner; Applied Policies reads the existing `configuration_group_assignments` chain.

**Tech Stack:** Go 1.25, Echo v4, Templ, sqlc (SQLite), vanilla JS static assets.

**Spec:** `docs/superpowers/specs/2026-07-05-standardized-detail-pages-design.md` (approved)

## Global Constraints

- **No git repo in this working copy** — skip every commit step; report changed files instead.
- Run everything from `/home/peter/AI Builds/Pluris/Pluris-main`. Use `-buildvcs=false` on all go build/test commands.
- After editing `.templ` run `~/go/bin/templ generate`; after editing `db/schema|queries/*.sql` run `~/go/bin/sqlc generate`. Generated code is authoritative over plan samples — fix call sites, not sqlc.
- sqlc parser quirks (learned this project): no em-dashes and no apostrophes/contractions in `.sql` comments.
- CPP path format is exactly `entity/section/param`, lowercase snake, forward slashes. Entity slugs this plan registers: `computer`, `server`, `printer`, `desk`, `user`.
- Roles slugs are exactly: `super_admin`, `admin`, `technician`, `user`. The string `user_self_service` must be gone from Go code and data when Phase 3 completes.
- One action button per DetailTable. No page-level apply/save state on detail tabs.
- Every list/table column set is registered in `web/lists` (INV-L); every new param is registered in `catalog/params` (INV-CPP §0.5). No hand-rolled `<table>` in tabs.
- Full suite (`go test -buildvcs=false -count=1 ./...`) must be green at the end of every task.

---

## Phase 1 — Foundations

### Task 1: `schema_migrations` run-once tracker

**Files:**
- Modify: `pkg/database/database.go` (migrate())
- Test: `pkg/database/migration_tracker_test.go` (new)

**Interfaces:**
- Produces: migrations listed in `migrate()` now execute exactly once per database file, recorded in `schema_migrations(filename TEXT PRIMARY KEY, applied_at TIMESTAMP)`. Task 2 relies on this to safely use `ALTER TABLE`.

- [ ] **Step 1: Failing test** — `pkg/database/migration_tracker_test.go`:

```go
package database

import (
	"os"
	"testing"
)

// TestMigrationsRecordedAndRunOnce: after Open, schema_migrations has one
// row per migration file; reopening does not error and does not add rows.
func TestMigrationsRecordedAndRunOnce(t *testing.T) {
	dbPath := "test_migration_tracker.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	d1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	var n1 int
	if err := d1.Conn().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n1); err != nil {
		t.Fatalf("schema_migrations missing: %v", err)
	}
	if n1 < 2 {
		t.Fatalf("expected >=2 recorded migrations, got %d", n1)
	}
	d1.Close()

	d2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer d2.Close()
	var n2 int
	d2.Conn().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n2)
	if n2 != n1 {
		t.Fatalf("reopen changed recorded migrations: %d -> %d", n1, n2)
	}
}
```

- [ ] **Step 2: Run to confirm FAIL** (`no such table: schema_migrations`).
- [ ] **Step 3: Implement** in `migrate()`: before the loop, `CREATE TABLE IF NOT EXISTS schema_migrations (filename TEXT PRIMARY KEY, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);`. For each file: skip if `SELECT 1 FROM schema_migrations WHERE filename=?` hits; else execute the file then `INSERT INTO schema_migrations(filename) VALUES(?)`. Keep the existing read-from-two-locations logic. Existing installs are safe because 001/002 are `IF NOT EXISTS`-idempotent: they re-run harmlessly exactly once more, then are recorded. Add a comment stating migrations from 003 on may contain non-idempotent statements ONLY because of this tracker.
- [ ] **Step 4: Test passes; full suite green** (`TestReopeningDatabasePreservesIdentities` must still pass).

### Task 2: Migration 003 + sqlc for roles / software / logs / asset extras / role rename

**Files:**
- Create: `db/schema/003_roles_software_logs.sql`
- Create: `db/queries/roles.sql`, `db/queries/activity_log.sql`, `db/queries/installed_software.sql`
- Modify: `db/queries/assets.sql` (two narrow setters), `db/queries/tenants.sql` (group membership helpers if missing)
- Modify: `pkg/database/database.go` (add 003 to list)
- Test: `pkg/database/roles_schema_test.go` (new)

**Interfaces:**
- Produces (generated by sqlc; names authoritative from generator): `CreateRole`, `GetRoleBySlug(tenant_id, slug)`, `ListRolesByTenant`, `AssignRoleToIdentity`, `RemoveRoleFromIdentity`, `ListRolesForIdentity(identity_id)`; `InsertActivity(tenant_id, entity_type, entity_id, event, detail, actor_identity_id)`, `ListActivityForEntity(tenant_id, entity_type, entity_id, limit)`; `ListSoftwareForAsset(asset_id)`, `CreateInstalledSoftware`; `SetAssetDescription(id, description)`, `SetAssetManagedBy(id, managed_by)`; `ListGroupsForAsset(asset_id)`, `AddAssetToGroup`, `RemoveAssetFromGroup`, `RemoveIdentityFromGroup` (add whichever of these four are missing — check `db/querier.go` first).
- Produces columns: `assets.description`, `assets.managed_by_identity_id`, `groups.group_category` (default `'security'`), `groups.group_scope` (default `'global'`). Identities `role` CHECK becomes `('super_admin','admin','technician','user')` and existing `user_self_service` rows become `user`.

- [ ] **Step 1: Failing test** — `pkg/database/roles_schema_test.go`: open fresh DB; create tenant; `INSERT INTO roles(tenant_id, slug, name, is_builtin) VALUES (?,?,?,1)`; insert identity with `role='technician'` (must succeed) and one with `role='user_self_service'` (must FAIL against the new CHECK); `SELECT description FROM assets LIMIT 0` and `SELECT group_category FROM groups LIMIT 0` must not error; `INSERT INTO activity_log(...)`, `INSERT INTO installed_software(...)` smoke rows.
- [ ] **Step 2: FAIL** (`no such table: roles`).
- [ ] **Step 3: Write 003.** Contents in order:

```sql
-- Migration 003: roles, identity_roles, installed_software, activity_log,
-- asset CMDB extras, AD-style group fields, and the role vocabulary
-- change (user_self_service becomes user; technician added).
-- Runs exactly once via schema_migrations, so ALTER TABLE is safe here.

CREATE TABLE IF NOT EXISTS roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    is_builtin BOOLEAN NOT NULL DEFAULT FALSE,
    template_slug TEXT,
    permissions TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, slug)
);
CREATE TABLE IF NOT EXISTS identity_roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id INTEGER NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    assigned_by INTEGER REFERENCES identities(id) ON DELETE SET NULL,
    UNIQUE(identity_id, role_id)
);
CREATE TABLE IF NOT EXISTS installed_software (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    version TEXT,
    publisher TEXT,
    pkg_type TEXT NOT NULL DEFAULT 'native'
        CHECK(pkg_type IN ('deb','rpm','appimage','flatpak','snap','wine','native','other')),
    installed_at TIMESTAMP,
    size_mb INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id INTEGER NOT NULL,
    event TEXT NOT NULL,
    detail TEXT,
    actor_identity_id INTEGER REFERENCES identities(id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_activity_entity ON activity_log(tenant_id, entity_type, entity_id);

ALTER TABLE assets ADD COLUMN description TEXT;
ALTER TABLE assets ADD COLUMN managed_by_identity_id INTEGER REFERENCES identities(id) ON DELETE SET NULL;
ALTER TABLE groups ADD COLUMN group_category TEXT NOT NULL DEFAULT 'security';
ALTER TABLE groups ADD COLUMN group_scope TEXT NOT NULL DEFAULT 'global';
```

Then the identities role-vocabulary rebuild (SQLite cannot alter a CHECK): `PRAGMA foreign_keys=OFF;` → `CREATE TABLE identities_new (...)` — copy the full column list **verbatim from `db/schema/002_identity_ad_compat.sql`**, changing ONLY the role line to `role TEXT NOT NULL DEFAULT 'user' CHECK(role IN ('super_admin', 'admin', 'technician', 'user'))` → `INSERT INTO identities_new SELECT` all columns with `CASE WHEN role = 'user_self_service' THEN 'user' ELSE role END` in the role position → `DROP TABLE identities;` → `ALTER TABLE identities_new RENAME TO identities;` → recreate the 7 identities indexes from 002 → `PRAGMA foreign_keys=ON;`. Column order in the INSERT SELECT must match exactly; list columns explicitly, never `SELECT *`.

- [ ] **Step 4: Query files.** `roles.sql`, `activity_log.sql`, `installed_software.sql` with the queries named in Interfaces (`:one`/`:many`/`:exec` as appropriate; `ListRolesForIdentity` joins identity_roles→roles). Asset setters in `assets.sql`:

```sql
-- name: SetAssetDescription :exec
UPDATE assets SET description = @description, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetAssetManagedBy :exec
UPDATE assets SET managed_by_identity_id = @managed_by, updated_at = CURRENT_TIMESTAMP WHERE id = @id;
```

Check `db/querier.go` for existing group-membership queries; add the missing ones of `ListGroupsForAsset`, `AddAssetToGroup`, `RemoveAssetFromGroup`, `RemoveIdentityFromGroup` to `tenants.sql` following the existing membership queries' style.

- [ ] **Step 5:** `sqlc generate`; wire 003 into `migrate()`; test passes; full suite green. Note: 002's rich SELECT columns are unchanged by the rebuild, so existing identity queries keep compiling.

### Task 3: Canonical Parameter Paths (CPP)

**Files:**
- Modify: `catalog/params/types.go` (add `PathEntity string` to `SubtypeSchema`)
- Modify: `catalog/params/schemas.go` (set PathEntity on all 5 schemas: computer/server/printer/desk = same as Subtype; SchemaIdentity → `"user"`)
- Create: `catalog/params/paths.go`
- Test: `catalog/params/paths_test.go`

**Interfaces:**
- Produces: `params.PathFor(entity, key string) string`, `params.ResolvePath(path string) (*SubtypeSchema, *SchemaSection, *ParamDef, error)`, `params.AllPaths() []string`, `params.SchemaByPathEntity(entity string) *SubtypeSchema`. Tasks 7/8/12/14 consume these for `data-path` attributes and Configuration rows.

- [ ] **Step 1: Failing tests** — `paths_test.go`:

```go
func TestPathForAndResolveRoundTrip(t *testing.T) {
	p := PathFor("user", "email")
	if p != "user/identity/email" {
		t.Fatalf("got %q", p)
	}
	schema, sec, def, err := ResolvePath(p)
	if err != nil || schema.PathEntity != "user" || sec.Key != "identity" || def.Key != "email" {
		t.Fatalf("resolve failed: %v %v %v %v", schema, sec, def, err)
	}
	if PathFor("computer", "ram_mb") != "computer/hardware/ram_mb" {
		t.Fatal("computer path wrong")
	}
	if PathFor("computer", "email") != "" {
		t.Fatal("unmounted key must return empty")
	}
}

func TestResolvePathFailsClosed(t *testing.T) {
	for _, bad := range []string{"", "user", "user/identity", "ghost/identity/email", "user/ghost/email", "user/identity/ghost", "user/identity/email/extra"} {
		if _, _, _, err := ResolvePath(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestAllPathsUniqueAndComplete(t *testing.T) {
	paths := AllPaths()
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] {
			t.Fatalf("duplicate path %q", p)
		}
		seen[p] = true
	}
	// every mounted (entity, param) has a path
	for _, s := range Schemas {
		for _, sec := range s.Sections {
			for _, k := range sec.Params {
				if PathFor(s.PathEntity, k) == "" {
					t.Fatalf("no path for %s key %s", s.PathEntity, k)
				}
			}
		}
	}
}
```

- [ ] **Step 2: FAIL** (undefined symbols).
- [ ] **Step 3: Implement `paths.go`**: build an index in `init()` (after schema registration; use a lazily-built `sync.Once` map keyed by `entity -> key -> path` and `path -> (schema, section index, def)` to avoid init-order pitfalls with the Schemas map). `ResolvePath` splits on `/`, requires exactly 3 non-empty segments, fails closed with wrapped errors. `AllPaths` returns sorted. Doc comment cites INV-CPP and the spec.
- [ ] **Step 4: PASS; full suite green.** Also append INV-CPP (the 5 rules from spec §0) to `docs/UX_INVARIANTS.md` as a new section.

### Task 4: New params + model plumbing (`description`, `managed_by`, `initials`)

**Files:**
- Modify: `catalog/params/definitions.go` (add `managed_by` link ParamDef + `initials` ParamDef; `description` already exists — reuse)
- Modify: `catalog/params/schemas.go` (mount: `description` + `managed_by` into computer & server Identity sections; `description` into printer & desk Identity sections; `initials` into SchemaIdentity Identity section)
- Modify: `catalog/assets/types.go` (add `Description string`, `ManagedBy string` fields), `pkg/services/assets.go` (map new columns in conversions; `SetDescription`, `SetManagedBy` wrappers), `pkg/services/identities.go` (map `Initials` — verify it is already mapped from Task 4 of the prior cycle; add if missing)
- Modify: `web/lists/assets.go` (`getAssetParamValue` cases `description`, `managed_by`), `web/lists/identities.go` (case `initials` if missing)
- Test: extend `catalog/params/identity_schema_test.go` + `pkg/services` round-trip test

**Interfaces:**
- Produces: `assets.Asset.Description/ManagedBy` populated from DB (ManagedBy = display name via LEFT JOIN identities on managed_by_identity_id — extend the list/get JOIN queries in `db/queries/assets.sql` with `m.display_name as managed_by_name` following the exact `owner_name` pattern); paths `computer/identity/description`, `computer/identity/managed_by`, `user/identity/initials` resolve.

- [ ] Steps: failing test asserting the three paths resolve via `ResolvePath` and a service-level test seeding an asset, calling `SetDescription`/`SetManagedBy`, reading back through `GetByID`. Then implement, `sqlc generate` after the JOIN edits, suite green. New ParamDefs:

```go
{Key: "managed_by", Label: "Managed by", Description: "Responsible administrator or owner of record.", Category: "identity", Type: TypeLink, LinkTarget: "user", Filter: FilterEquals, Sort: SortAlpha},
{Key: "initials", Label: "Initials", Description: "Name initials.", Category: "identity", Type: TypeString, Filter: FilterNone, Sort: SortNone},
```

Also add `{ParamKey: "managed_by", TargetEntity: "user", Cardinality: "one", Bidirectional: true, InverseLabel: "Manages assets"}` to `allLinks`.

### Task 5: Seeder `-tenant` flag + rich seed data

**Files:**
- Modify: `cmd/seed/main.go`

**Interfaces:**
- Produces: `go run ./cmd/seed` seeds into the FIRST existing tenant by default; `-tenant <slug>` selects explicitly; creates demo tenant only under `-tenant demo` when absent. Seeds additionally: 2 groups (`Engineering-Laptops` security/global, `HQ-All-Staff` security/domain_local) with 3 asset + 2 identity memberships; 1 configuration group `HQ-Baseline` with 1 binding + 1 assignment targeting the first computer; 4 `installed_software` rows on the first computer (deb/appimage/wine/native); `description` + `managed_by` set on 2 computers.

- [ ] Steps: add `flag.String("tenant", "", ...)`; resolve: flag set → `GetTenantBySlug` (error if missing, except `demo` which is created); flag empty → `ListTenants` first row (error with clear message if zero tenants: "run /setup first or pass -tenant demo"). Insert the extra seed rows using the Task-2 queries (check generated names). Binding/assignment inserts: read `db/schema/001_initial.sql`'s `configuration_group_bindings`/`configuration_group_assignments` column lists first and use raw `database.Conn().Exec` if no sqlc queries exist yet for creation (Task 12 adds read queries; creation via seed can stay raw SQL with a comment). Manual verify: fresh setup tenant → `go run ./cmd/seed` → `/assets/computers` shows rows; suite green (seed has no tests — verify by running it against a scratch DB twice: second run must not crash on UNIQUE, use `INSERT OR IGNORE`/existence checks).

---

## Phase 2 — Shell

### Task 6: `DetailShell` + `DetailTable` + `detail.js`

**Files:**
- Create: `web/templates/detail_shell.templ`, `web/static/detail.js`
- Modify: `web/templates/layout.templ` (add `<script src="/static/detail.js" defer></script>` next to lists.js)
- Test: `console/server/detail_shell_test.go` (rendered-markup assertions come in Tasks 7/8; here: a template unit test via `templ.Component.Render` into a buffer)

**Interfaces:**
- Produces (exact signatures Tasks 7/8/15 call):

```go
type Crumb struct { Label, Href string }        // Href "" = current page (plain span)
type Chip struct { Label, Class string }        // Class appended to "asset-chip"
type HeroDef struct { Label, Value string }
type HeroSpec struct {
    Crumbs []Crumb
    Name, ID string
    Chips  []Chip
    Defs   []HeroDef
    Visual templ.Component   // device icon / avatar / policy icon
    Action templ.Component   // optional single header action (owner picker, Edit button); nil ok
}
type TabSpec struct { Slug, Label string; Body templ.Component }

templ DetailShell(activeNav string, title string, hero HeroSpec, tabs []TabSpec)
templ DetailTableFrame(listID string, action templ.Component)   // opens card+table+thead from lists.FieldsFor(listID); caller renders tbody via children
templ DetailEmptyRow(listID string, colspan int, message string) // standard empty tbody row
```

- `DetailShell` renders: `@Layout(activeNav, title)` → breadcrumb → `.asset-detail-hero` (existing classes) → `.asset-detail-tabs` with `div.asset-detail-tab[data-tab=slug]` per tab (first `is-active`) → per tab `div.detail-tab-panel[data-panel=slug]` (first `is-active`). Panels all server-rendered.
- `detail.js` (INV-L9 single shared file): click delegation on `.asset-detail-tabs .asset-detail-tab[data-tab]` toggles `is-active` on tabs+panels; sets `location.hash = slug` via `history.replaceState`; on `DOMContentLoaded` reads `location.hash` and activates the matching tab if present. Add `.detail-tab-panel { display:none; } .detail-tab-panel.is-active { display:block; }` to `layout.templ`'s style block near the existing `.asset-detail-tabs` rules.

- [ ] Steps: write the render-to-buffer unit test asserting: N tab divs, matching panels, first active, `data-tab`/`data-panel` slugs equal, hero name/ID present. FAIL → implement → `templ generate` → PASS → suite green.

### Task 7: Computer detail on the shell (8 tabs)

**Files:**
- Modify: `web/templates/pages.templ` (`AssetDetailPageWithData` rebuilt on `DetailShell`; keep `data-testid="page-asset-detail"` and `data-canonical-editor` anchors), `web/templates/assets_helpers.go` (generalize `buildDetailSections` output into the General tab body; add `data-path` attribute per field using `params.PathFor(schema.PathEntity, key)`)
- Modify: `web/lists/` — new file `web/lists/detail_tabs.go` registering list IDs + FieldDefs: `computer-groups` (Group, Category, Scope, Source), `applied-policies` (Policy, Source, Scope, Value, Status), `asset-configuration` (Setting, Policy value, Reported value, Status, Action), `installed-software` (Name, Version, Publisher, Type, Installed, Size), `asset-wine-groups` (Wine Config Group, Wine version, Arch, Applications, Assigned), `asset-scripts` (Script, Trigger, Last run, Status, Next run), `asset-logs` (Time, Event, Detail, Actor)
- Modify: `console/handlers/handlers.go` (`AssetDetail` passes software rows — `ListSoftwareForAsset` — and logs rows — `ListActivityForEntity`; groups/policies data arrives Tasks 9/12, until then those tabs render `DetailEmptyRow`)
- Test: `console/server/server_test.go` — extend the asset-detail path: assert response contains all 8 `data-tab` slugs (`general`, `groups`, `policies`, `configuration`, `software`, `wine-groups`, `scripts`, `logs`) and `data-path="computer/hardware/ram_mb"`.

**Tab slugs and bodies:** General = schema sections grid (live). Software + Logs = DetailTableFrame with real rows (live via seed data). Groups/Policies/Configuration = frame + `DetailEmptyRow` placeholder until Tasks 9/12/14. Wine-Groups/Scripts = frame + `ConceptEmptyState`-style message ("arrives with the Wine feature" / "arrives with Scripts"). Hero keeps the owner-picker form as `HeroSpec.Action`.

- [ ] Steps: failing server test → rebuild template → regenerate → PASS → full suite green → manual: `/assets/computers/<seeded id>` shows tabs, switching works, hash deep-link works.

### Task 8: User detail on the shell (4 tabs)

**Files:**
- Modify: `web/templates/users.templ` (`UserDetailPage` rebuilt on `DetailShell`; keep `data-testid="page-user-detail"`), list IDs in `web/lists/detail_tabs.go`: `user-groups` (same columns as computer-groups), `user-roles` (Role, Type, Description, Assigned)
- Modify: `console/handlers/handlers.go` (`UserDetail` — assigned-assets moves into the General tab as its existing section; Groups/Policies/Roles tabs empty-frame until Tasks 9/11/12)
- Test: extend `console/server/server_test.go` user-detail test: assert 4 `data-tab` slugs (`general`, `groups`, `policies`, `roles`) and `data-path="user/identity/email"`.

Hero: name, UPN mono, chips (role label + Enabled/Disabled/Locked), defs (Dept, Site, Title, Manager), avatar-initials visual (reuse `firstRuneUpper` logic), `HeroSpec.Action` = Edit link; Delete stays as a small form under the General tab (single confirm, unchanged guard).

- [ ] Steps: failing test → rebuild → PASS → suite green → manual check.

---

## Phase 3 — Groups + Roles

### Task 9: Groups tabs live (both entities)

**Files:**
- Modify: `pkg/services/` — new `groups.go`: `GroupService` with `ListForAsset(ctx, assetID) ([]GroupRow, error)`, `ListForIdentity`, `ListByTenant`, `AddAssetMember`, `AddIdentityMember`, `RemoveAssetMember`, `RemoveIdentityMember` (`GroupRow{ID int64; Name, Category, Scope, Source string; AddedAt time.Time}`; Source is `"Direct"` for now — inheritance labels arrive with sites/tenant assignment later, note in comment)
- Modify: `console/handlers/handlers.go` + `console/server/server.go`: `POST /assets/:subtype/:id/groups` (form `group_id`, add) / `POST /assets/:subtype/:id/groups/:groupID/remove`; same pair under `/users/:id/groups...`; handlers append `activity_log` rows (`event: "group_added"/"group_removed"`)
- Modify: Task 7/8 tab bodies: real rows + "Add to group" action (a `<select>` of `ListByTenant` groups not yet joined + submit, one button) + per-row Remove form
- Test: `pkg/services/groups_test.go` (add/list/remove round-trip for both member kinds) + one handler test in `console/server` using `doCSRFPost`

- [ ] Steps: TDD service first, then handlers/UI; suite green; manual: seeded groups visible on the seeded computer, add/remove works, activity_log rows appear on the Logs tab.

### Task 10: Role vocabulary + RoleService + enforcement

**Files:**
- Modify: `catalog/identities/types.go`: `RoleTechnician Role = "technician"`, `RoleUser Role = "user"`; DELETE `RoleUserSelfService`; update `IsValid`/`Label` (Technician label "Technician", user label "User")
- Modify: `pkg/auth/rbac.go`: matrix gains `technician` (= admin row except `/server-admin`: false) and renames `user_self_service` keys to `user`; update `rbac_test.go`
- Modify: ALL references to `user_self_service` / `RoleUserSelfService` across Go + templ + params EnumValues (`role` ParamDef EnumValues becomes the 4 new slugs) — grep is mandatory: `grep -rn "user_self_service\|RoleUserSelfService" --include=*.go --include=*.templ --include=*.sql .` must return zero hits in non-generated files when done (regenerate templ/sqlc after)
- Create: `pkg/services/roles.go`: `RoleService` with `EnsureBuiltins(ctx, tenantID)` (idempotent seed of the 4 builtin rows: names Super Admin/Admin/Technician/User, descriptions one-liners), `ListByTenant`, `ListForIdentity`, `Assign(ctx, identityID, roleID, actorID)`, `Remove(...)`; Assign/Remove recompute the `identities.role` cache = highest-privilege assigned builtin (order: super_admin > admin > technician > user; custom roles rank as their `template_slug`) via `UpdateIdentityRole`
- Modify: `console/handlers/auth.go` `SetupSubmit`: after creating the first identity call `EnsureBuiltins` + `Assign(super_admin)`
- Test: `pkg/services/roles_test.go` (EnsureBuiltins idempotent; assign admin+technician → cache "admin"; remove admin → cache "technician"; remove all → cache "user")

- [ ] Steps: TDD; full suite green (expect fallout in `middleware_test.go`/`rbac_test.go`/`identities_test.go` string literals — fix them to the new slugs); manual: fresh setup → login works, roles table has 4 rows, first admin has super_admin assigned.

### Task 11: Roles tab UI (assignment only)

**Files:**
- Modify: `web/templates/users.templ` Roles tab body: `DetailTableFrame("user-roles", addRoleAction)` — rows from `RoleService.ListForIdentity` (Role name, Type = Built-in/Custom, Description, Assigned date; per-row Remove form with confirm); `addRoleAction` = single form: `<select name="role_id">` of unassigned tenant roles + "Add role" submit; below the table a disabled `btn-secondary` "Manage role permissions" with `title="Coming with the permission editor"`
- Modify: handlers + routes: `POST /users/:id/roles` (assign), `POST /users/:id/roles/:roleID/remove`; both require actor role admin or super_admin (explicit check via `auth.FromContext`, 403 otherwise); guard: an actor cannot modify their OWN roles (mirrors self-delete guard, 400); both append activity_log (`role_assigned`/`role_removed`) and re-render via redirect back to `#roles`
- Test: handler test — technician actor gets 403 on assign; admin actor assigns/removes successfully; self-modification 400

- [ ] Steps: TDD handler test with `doCSRFPost` → implement → suite green → manual browser pass.

---

## Phase 4 — Policies

### Task 12: Assignment resolution + Applied Policies tabs

**Files:**
- Create: `db/queries/assignments.sql`: `ListAssignmentsByTarget(target_type, target_id) :many` (JOIN configuration_groups + configuration_group_bindings; read `db/schema/001_initial.sql` lines ~204-250 FIRST for real column names — bindings reference policies by the column that exists there), plus `CreateConfigurationGroup`, `CreateBinding`, `CreateAssignment`, `GetConfigurationGroupByName` if 001-backed creation queries are missing
- Create: `pkg/services/assignments.go`: `AssignmentService.ResolveForTarget(ctx, tenantID int64, targetType string, targetID int64, groupIDs []int64) ([]AppliedPolicy, error)` — calls `ListAssignmentsByTarget` for the direct target, each group, the site (if asset has one), and `("tenant", tenantID)`; dedupes by binding; `AppliedPolicy{PolicyID, PolicyName, SourceGroup, Scope, ValueSummary, Status string}`; policy display name resolved from `catalog/policies` by the stored identifier (fallback: raw ID, never error); Status = `"Assigned"` or `"Disabled"` (binding/group disabled flags); ValueSummary = `k=v` pairs joined, keys rendered as CPP paths when `params.ResolvePath` recognizes them
- Modify: Computer + User `policies` tab bodies: `DetailTableFrame("applied-policies", addFromCatalogAction)` with real rows; the action button is a LINK (`btn-primary`) to Task 13's picker route
- Test: `pkg/services/assignments_test.go` — seed tenant/group/config group/binding/assignments (direct + via group), resolve for the asset, assert both rows, dedupe, Disabled status

- [ ] Steps: read 001 schema → TDD service → wire tabs → suite green → manual: seeded `HQ-Baseline` policy shows on the seeded computer's Policies tab.

### Task 13: Add-from-catalog flow

**Files:**
- Modify: handlers + routes: `GET /assets/:subtype/:id/policies/add` and `GET /users/:id/policies/add` → picker page: standardized list of `catalog/policies` entries (reuse the policy-catalog list registry columns; each row has a "Select" link carrying `?policy=<id>`) → `GET ...?policy=<id>` renders the parameter form (policy params from the catalog entry if it declares any — check `catalog/policies/types.go`; if none, show "This policy has no parameters") → `POST` creates: find-or-create configuration group named `Direct - <entity display name>` scoped to the tenant, insert binding (policy id + values JSON), insert assignment (`target_type` `asset`/`identity`, target id), activity_log `policy_assigned`, redirect to detail `#policies`
- New templ: `web/templates/policy_picker.templ` (uses `DetailShell`-less simple `Layout` + the standard list frame; one page, two states)
- Test: handler test with `doCSRFPost`: POST creates all three rows (assert via direct SQL) and the policy then appears in `ResolveForTarget`

- [ ] Steps: TDD → implement → suite green → manual browser: add a policy to a user from the catalog, see it in the tab.

### Task 14: Configuration tab + remaining activity-log wiring

**Files:**
- Modify: Computer `configuration` tab body: rows derived from `ResolveForTarget` bindings — one row per bound value: Setting = CPP path (mono) with the param Label when resolvable, Policy value, Reported value = `awaiting agent` muted text, Status chip `Assigned`, Action = disabled `Override` button with `title="Requires the Assets permission: Override policy-set configuration (permission editor coming)"`; registered columns already exist (`asset-configuration`)
- Modify: `AssetSetOwner` and `UserDeleteSubmit`/`UserCreateSubmit`/`UserUpdateSubmit` handlers: append activity_log rows (`owner_changed`, `user_created`, `user_updated`, `user_deleted`) — target entity `asset`/`identity`
- Test: extend the Task 12 service test asserting configuration rows derive one-per-value; handler test asserting owner change writes an activity row that then renders on the Logs tab

- [ ] Steps: TDD → implement → suite green.

---

## Phase 5 — Policy pages

### Task 15: `/policy/catalog/:id` detail pages, popup deleted

**Files:**
- Modify: `console/server/server.go` (`GET /policy/catalog/:id`), `console/handlers/handlers.go` (`PolicyCatalogDetail`: look up `catalog/policies` by ID, 404 page if unknown)
- Create: `web/templates/policy_detail.templ`: `DetailShell` — hero (display name; URN mono as ID; chips: category + scope; defs: Windows GP equivalent, GP path, Linux mechanism; visual: policy icon from existing icon set), tabs: `general` (definition fields incl. parameters with per-param CPP-style addressing `policy_param/<policy-id>/<param>` rendered as plain labels — policies are not in the params registry yet, display only), `modules` (`DetailTableFrame("policy-modules-for-policy")`: Module, Origin, Version, Status, Target OS — rows from `catalog/policymodules` candidates for this policy; register the list ID), `assignments` (`DetailTableFrame("policy-assignments")`: Configuration Group, Target, Scope, Status — new sqlc query `ListAssignmentsByPolicy` joining bindings→groups→assignments)
- Modify: `web/templates/pages.templ` + `web/templates/menu.go`: DELETE `PolicyDetailDialog`, `policyDetailDataIsland`, and the `policyDetailScript` const; catalog rows get row-click navigation to the new page (same pattern as `usersListNavigationScript`); the custom-policy Edit button moves to the detail hero `Action` (only when `origin == custom`, dispatches the existing wizard-open path — check how `cpw:open` is triggered and preserve equivalent behavior; if the wizard lives only on the list page, the hero action links back to `/policy/catalog#edit=<id>` and the list page keeps the wizard)
- Test: extend `server_test.go`: new mount-point row for a known bundled policy id (200 + `data-testid="page-policy-detail"`), catalog list page no longer contains `pdd-dialog`, unknown id → 404 page

- [ ] Steps: failing tests → implement → delete dead dialog code (grep `pdd-` for leftovers) → regenerate → suite green → manual: click a catalog row, land on its page, tabs work.

### Task 16: End-to-end verification

No new files. Fresh DB → rebuild all (`templ generate`, `sqlc generate`, build, full suite) → setup wizard → seed (`go run ./cmd/seed`) → walkthrough: computer detail all 8 tabs (General fields with `data-path`, Software + Logs + Groups + Policies live), add/remove group, add policy from catalog, configuration rows with disabled Override, user detail 4 tabs, assign technician role to a second user (verify cache + 403 behavior for technician on `/server-admin`), policy catalog row-click → detail page, popup gone, restart survival, no `user_self_service` anywhere (`grep -rn "user_self_service" --include=*.go --include=*.templ . | grep -v _templ.go` clean, and DB rows migrated). Report results.

---

## Self-review notes

- Spec coverage: §0→Task 3(+7/8 data-path), §1→6, §2→6(+7/8 registrations), §3→7(+9/12/14 data), §4→8(+9/11/12), §5→10/11, §6→12/13, §7→15, §8→1/2/5, §9 honored (no permission editor, no agent data), §10 phases = task order.
- Known soft spots flagged inline: 001 binding/assignment column names (Task 12 reads schema first), existing group-membership query coverage (Task 2 checks querier.go), custom-policy wizard reachability (Task 15 preserves either path), generated sqlc names authoritative everywhere.
