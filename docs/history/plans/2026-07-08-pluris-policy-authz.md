# Pluris Policy (Console Authz) + User Management Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Zero-trust permission system for the Pluris console (GLPI-style `domain.action` grants with None/Own/All scopes, locked builtin templates + clone, matrix UI at /policy/pluris) with the inline-edit user/asset field API as its first enforced consumer.

**Architecture:** Pure-Go permission registry (`catalog/permissions`) → grants JSON in the existing `roles.permissions` column → per-request effective-grants resolution in the auth middleware → three enforcement layers (route map, handler gates, service scoping). **Spec:** `docs/superpowers/specs/2026-07-08-pluris-policy-authz-design.md` is the requirements contract — the full key list, builtin template matrices, and Own semantics live THERE; implementers must read it and not invent.

**Tech Stack:** Go 1.25, Echo v4, Templ, sqlc (SQLite), vanilla JS.

## Global Constraints

- Never touch repo-root `pluris.db*`. Tests use `t.TempDir()` scratch DBs only.
- `-buildvcs=false` on every go command. `export PATH="$HOME/go/bin:$PATH"` for templ/sqlc.
- `make gen` after `.templ` edits; `sqlc generate` after `.sql` edits; never hand-edit generated files. ASCII-only SQL comments.
- No new external Go dependencies. No DB migration (the `roles.permissions` JSON column exists since migration 003).
- Owner owns git: NO `git commit/add/push` by any agent. Where a step says "Commit", STOP and report instead.
- Suite green = done: `go test -buildvcs=false -count=1 ./...` exit 0, `gofmt -l` clean.
- Invariants: DetailShell for detail pages, INV-L (list columns from `web/lists/`), cross-tenant ids read as 404 via the `resolveTenant*` pattern (see `console/handlers/roles.go:resolveTenantRole`).
- `pkg/auth/` is the highest blast-radius area (login/session). Tasks 3-5 must each end suite-green with login still working (server_test.go's setup+login harness is the guard).

## Shared reference (names used across tasks)

```go
// catalog/permissions (Task 1)
type Action struct{ Key, Label, Description string; Scoped bool }
type Domain struct{ Key, Label string; Actions []Action }
func All() []Domain
func ActionByKey(full string) *Action   // "identity.update" -> *Action or nil
func AllKeys() []string
func TemplateGrants(slug string) map[string]string // builtin matrices from spec; nil for unknown slug

// pkg/authz (Task 2)
type Grants map[string]string            // key -> "none|own|all|no|yes"
const BypassKey = "__super_admin__"      // marker entry set by middleware for super_admin
func (g Grants) Can(key string) bool                          // yes OR any scope >= own OR bypass
func (g Grants) CanScoped(key string, ownerID, selfID int64) bool
func (g Grants) ScopeOf(key string) string
func Union(gs ...Grants) Grants          // all>own>none, yes>no
func Parse(permissionsJSON string) Grants        // unknown keys kept, invalid JSON -> empty
type Service struct{ db *database.Database }
func NewService(db *database.Database) *Service
func (s *Service) EffectiveGrants(ctx context.Context, identityID int64) (Grants, error)
func (s *Service) EnsureBuiltinGrants(ctx context.Context, tenantID int64) error
func (s *Service) SaveRolePermissions(ctx context.Context, roleID int64, g Grants) error
func (s *Service) CloneRole(ctx context.Context, tenantID, sourceRoleID int64, newName string) (db.Role, error)

// pkg/auth (Task 3)
// UserSession gains: Grants authz.Grants
// pkg/auth/rbac.go (Task 4) exports: func RoutePermissionKey(path string) string

// console/handlers (Task 5)
func requirePermission(c echo.Context, key string) error                  // 403 on deny
func requirePermissionScoped(c echo.Context, key string, ownerID int64) error
```

DB additions (Task 2, `db/queries/roles.sql`): `UpdateRolePermissions :exec` (`UPDATE roles SET permissions=@permissions, updated_at=CURRENT_TIMESTAMP WHERE id=@id`), `ListIdentitiesForRole :many` (join identity_roles→identities by role id, return id/username/display_name/email), and Task 8 adds `db/queries/assets.sql` → `UpdateAssetPayload :exec` (`UPDATE assets SET subtype_payload=@subtype_payload, updated_at=CURRENT_TIMESTAMP WHERE id=@id`) — check the assets table's real column names in `db/schema/001_initial.sql` first.

---

### Task 1: Permission registry (`catalog/permissions`)

**Files:** Create `catalog/permissions/registry.go`, `catalog/permissions/templates.go`, `catalog/permissions/registry_test.go`.

**Interfaces:** Produces the registry API in the shared reference. Zero non-stdlib imports.

- [ ] Step 1: failing test — `registry_test.go`: (a) `AllKeys()` has no duplicates and every key parses as `domain.action`; (b) `ActionByKey("identity.update")` non-nil and `.Scoped==true`; `ActionByKey("identity.create").Scoped==false`; `ActionByKey("nope.nope")==nil`; (c) for each builtin slug (`super_admin,admin,technician,user`) `TemplateGrants(slug)` is non-nil, every key in it exists in the registry, and every registry key exists in it (full coverage both ways); (d) scoped actions carry scope values (`none|own|all`) and unscoped carry `no|yes` in every template; (e) spot-assert spec values: technician `identity.delete`=="no", user `identity.update`=="own", admin `server_admin.tenant_switch`=="no", super_admin `identity.delete`=="yes".
- [ ] Step 2: run `go test -buildvcs=false ./catalog/permissions/` — fails to compile.
- [ ] Step 3: implement `registry.go` (domains/actions EXACTLY per the spec's "v1 domains and actions" table — 23 keys) with `init()` building a key index and panicking on duplicates (mirror `catalog/params/paths.go` style), and `templates.go` with the four matrices EXACTLY per the spec's "Builtin template matrices" section.
- [ ] Step 4: test passes; `gofmt -l catalog/permissions/` clean.
- [ ] Step 5: STOP and report (owner commits).

### Task 2: Authz grants engine + queries (`pkg/authz`)

**Files:** Create `pkg/authz/grants.go`, `pkg/authz/service.go`, `pkg/authz/grants_test.go`, `pkg/authz/service_test.go`. Modify `db/queries/roles.sql` (+`sqlc generate`).

**Interfaces:** Consumes Task 1 registry + existing `db.Queries` role queries. Produces the `pkg/authz` API in the shared reference.

- [ ] Step 1: failing tests. `grants_test.go`: Union ranking (`all` beats `own` beats `none`; `yes` beats `no`; disjoint keys merge); `Can` true for `yes`/`own`/`all`, false for `no`/`none`/missing; `CanScoped` all→true, own→only ownerID==selfID, none/missing→false; bypass marker makes everything true; `Parse("{bad")` → empty map; `Parse` keeps unknown keys. `service_test.go` (scratch DB via `database.Open(filepath.Join(t.TempDir(),"t.db"))`, create tenant + identity, use `services.NewRoleService` EnsureBuiltins then assign roles): `EnsureBuiltinGrants` idempotent (run twice; builtin roles' permissions JSON non-empty; a manually-pre-set non-default value survives the merge — only absent keys are filled); `EffectiveGrants` unions two assigned roles (user+technician → technician's `all` wins over user's `own` for `identity.view`); identity with no roles → empty grants (deny-all); `SaveRolePermissions` round-trips; `CloneRole` creates a custom role (`is_builtin=false`, `template_slug`=source slug, permissions JSON equal to source).
- [ ] Step 2: run — fails to compile.
- [ ] Step 3: add the two queries to `db/queries/roles.sql` (ASCII comments), `sqlc generate`. Implement `grants.go` (pure functions; `Union` iterates maps with rank helpers `scopeRank{none:0,own:1,all:2}` / `boolRank{no:0,yes:1}` choosing per key kind via `permissions.ActionByKey`; unknown keys use string compare fallback yes>no, all>own>none) and `service.go` (`EffectiveGrants`: `ListRolesForIdentity` → `Parse` each `.Permissions` → `Union`; `EnsureBuiltinGrants`: for each builtin slug `GetRoleBySlug` → merge `TemplateGrants(slug)` into parsed existing (existing keys win) → `UpdateRolePermissions` only when changed; `CloneRole`: `GetRole` source → `CreateRole` with `Slug: slugify(newName)` — copy the `slugify` helper pattern from `pkg/services/dependencygroups.go` or export/reuse it — `IsBuiltin:false`, `TemplateSlug` = source slug (builtin) or source's TemplateSlug (custom), `Permissions` copied via `UpdateRolePermissions` after create since CreateRole may not take permissions — check the generated `CreateRoleParams`; if it lacks Permissions, create then update).
- [ ] Step 4: focused tests pass; full suite green; gofmt clean.
- [ ] Step 5: STOP and report.

### Task 3: Session integration (grants on every request)

**Files:** Modify `pkg/auth/context.go` (UserSession + Grants field), `pkg/auth/middleware.go` (resolve grants where the identity is loaded, ~line 75-104), `console/handlers/auth.go` (SetupSubmit: seed `authz.NewService(db).EnsureBuiltinGrants` best-effort after the roles EnsureBuiltins). Test: `pkg/auth/middleware_test.go` additions.

**Interfaces:** Consumes Task 2. Produces `sess.Grants` populated on every authenticated request; `super_admin` gets `Grants{authz.BypassKey:"yes"}`.

- [ ] Step 1: failing test — extend the existing middleware test harness (read `pkg/auth/middleware_test.go` first and reuse its setup): after login as the setup super-admin, the request context session has `Grants.Can("identity.create")==true` (bypass); create a second identity with only the `user` builtin role (seed grants via `EnsureBuiltinGrants` + RoleService.Assign) and assert its session grants have `CanScoped("identity.update", ownID, ownID)==true` and `Can("identity.delete")==false`.
- [ ] Step 2: run — fails.
- [ ] Step 3: implement. In `middleware.go`, after `userSess` is built: `super_admin` → bypass marker; else `authz.NewService(dbase).EffectiveGrants(ctx, identity.ID)` (on error: log + empty grants = deny-all, do NOT fail the request). CAUTION: construct the authz service once where `dbase` is in scope (the middleware closure), not per request if avoidable. Keep `UserSession.Role` untouched (login bootstrap + bypass marker source).
- [ ] Step 4: full suite green (this touches every authenticated test path — fix any test constructing `UserSession` literals only if compilation requires; the new field is zero-value-safe).
- [ ] Step 5: STOP and report.

### Task 4: Route enforcement replacement (`pkg/auth/rbac.go`)

**Files:** Rewrite `pkg/auth/rbac.go` + `pkg/auth/rbac_test.go`; modify `pkg/auth/middleware.go` (`RequireRole` → grants check); modify `web/templates/menu.go` (nav gating).

**Interfaces:** Produces `RoutePermissionKey(path string) string` (longest-prefix; "" = no session-gated page). `CanAccess` becomes `CanAccessGrants(g authz.Grants, path string) bool` — keep a thin `CanAccess(role, path)` ONLY if non-test callers still need it (grep first; migrate them instead if trivial).

- [ ] Step 1: failing test — rewrite `rbac_test.go` table-driven: for each builtin template (build Grants via `permissions.TemplateGrants(slug)` + `authz.Parse`-equivalent map literal), assert route access matches the OLD matrix semantics: user can reach `/`, `/users`, `/assets`, `/policy`, `/wine`, `/packages`, `/preferences`, `/profiles`; user cannot reach `/scripts`, `/policy/modules`, `/server-admin`, `/tenant-switch`, `/policy/pluris`; technician everything except `/server-admin`, `/tenant-switch`; admin everything except `/tenant-switch`; super_admin (bypass) everything.
- [ ] Step 2: run — fails.
- [ ] Step 3: implement the route→key map (spec §3 layer 1). Cover every prefix from the OLD map plus `/policy/pluris` → `console_access.view_roles`. Mapping decisions locked here: `/users` → `identity.view`; `/assets` → `asset.view`; `/policy` → `endpoint_policy.view`; `/policy/modules` AND `/scripts` → `endpoint_policy.manage_modules` (both were technician-and-up in the old matrix; scripts are module-adjacent automation — document with a comment); `/profiles`, `/wine`, `/packages`, `/preferences`, `/` → `""` (open to any authenticated session, matching the old all-roles rows); `/server-admin` → `server_admin.access`; `/tenant-switch` → `server_admin.tenant_switch`. Scoped view keys pass when scope != none. Update `RequireRole` middleware to use session grants (rename to `RequireAccess` if churn is small; otherwise keep the name, change internals). Update `menu.go`: each sidebar item/child renders only when `RoutePermissionKey(item.Href)` is "" or `sess.Grants` passes — read how menu.go currently receives session/role and thread grants the same way.
- [ ] Step 4: full suite green (server_test.go route table is the true guard — the smoke tests log in as super_admin so nothing should 403).
- [ ] Step 5: STOP and report.

### Task 5: Handler gate migration

**Files:** Create `console/handlers/authz_helpers.go` (+ test). Modify `console/handlers/roles.go`, `console/handlers/dependency_groups.go`, `console/handlers/policy_picker.go`, `console/handlers/handlers.go` (user create/delete, asset owner), `console/handlers/groups.go` (identity/asset group membership).

**Interfaces:** Produces `requirePermission` / `requirePermissionScoped` (shared reference). Deletes `requireRoleAdmin` once all callers migrate.

- [ ] Step 1: failing test — `authz_helpers_test.go`: session with technician template grants → `requirePermission(c,"identity.delete")` returns 403 HTTPError; `requirePermission(c,"identity.create")` nil; scoped: user-template session + own id → nil, other id → 403; nil session → 403.
- [ ] Step 2: run — fails to compile.
- [ ] Step 3: implement helpers (thin wrappers over `sess.Grants.Can/CanScoped`, 403 `echo.NewHTTPError` on deny). Migrate call sites: roles.go handlers → `console_access.manage_role_assignments`; dependency-group CRUD + module links → `endpoint_policy.manage_dependency_groups` / `endpoint_policy.manage_modules`; policy_picker add/submit → `endpoint_policy.assign_policies`; UserCreateSubmit/UserNewShow → `identity.create`, UserDeleteSubmit → `identity.delete`; AssetSetOwner → `asset.set_owner`; group add/remove handlers → `identity.assign_groups` / `asset.manage_groups`. Delete `requireRoleAdmin` when zero callers remain. Existing tests asserting 403-for-technician on role management still pass (technician template has manage_role_assignments "no"); dependency-group RBAC tests: technician template has manage_dependency_groups "yes" — UPDATE those tests to use a user-template session as the denied actor and technician as an allowed one (behavior change is intended: spec grants technicians dependency-group management).
- [ ] Step 4: full suite green; gofmt clean.
- [ ] Step 5: STOP and report.

### Task 6: Pluris Policy backend (clone / save matrix / delete / members)

**Files:** Create `console/handlers/pluris_policy.go`, `console/handlers/pluris_policy_test.go`. Modify `console/server/server.go` (routes).

**Interfaces:** Consumes Tasks 2+5. Produces handlers `PlurisPolicy` (list), `PlurisPolicyDetail`, `PlurisPolicyClone`, `PlurisPolicySave`, `PlurisPolicyDelete`; routes `GET /policy/pluris`, `GET /policy/pluris/:id`, `POST /policy/pluris/:id/clone`, `POST /policy/pluris/:id`, `POST /policy/pluris/:id/delete` (register before/after :id consistently; no `/new` — creation is clone-only).

- [ ] Step 1: failing tests — model on `dependency_groups_test.go` construction: (a) clone technician → new custom role exists with copied grants; (b) save matrix on custom role persists (form fields per matrix encoding below) and unknown keys are rejected 400; (c) save on builtin → 400; (d) delete builtin → 400; delete custom with members → 400; delete empty custom → 302; (e) self-lockout: actor (non-super-admin) holding the custom role saves a matrix dropping `console_access.manage_permissions` → 400 and grants unchanged; (f) cross-tenant role id → 404 (via `resolveTenantRole`); (g) all mutations 403 for a user-template session.
- [ ] Step 2: run — fails.
- [ ] Step 3: implement. Matrix form encoding: one form field per key, name `perm_<domain>.<action>`, value `none|own|all` for scoped and `no|yes` (checkbox: present=yes, absent=no) for unscoped — handler iterates `permissions.AllKeys()`, reads each field, validates value kind per `ActionByKey`, builds Grants, then guards: role in-tenant (`resolveTenantRole`), `!role.IsBuiltin`, self-lockout check (recompute actor's effective grants substituting this role's new grants; if actor holds the role and result lacks manage_permissions and actor is not super_admin → 400), then `SaveRolePermissions` + activity log `role_permissions_updated`. Clone: `console_access.manage_permissions` gate, name required, `CloneRole`, activity `role_cloned`, redirect to detail. Delete: gate + builtin-400 + members-count-400 (`ListIdentitiesForRole` len>0) + delete via existing role delete query (check `db/queries/roles.sql` — if no DeleteRole query exists, ADD one with sqlc) + activity `role_deleted`. List/Detail handlers: gate `console_access.view_roles`; detail loads role + parsed grants + members + registry for rendering; call `authzSvc.EnsureBuiltinGrants` best-effort in the list handler (lazy upgrade for pre-feature tenants). Wire `authzSvc *authz.Service` into the Handler struct (constructor).
- [ ] Step 4: focused + full suite green.
- [ ] Step 5: STOP and report.

### Task 7: Pluris Policy UI (list + matrix editor)

**Files:** Create `web/templates/pluris_policy.templ`, `web/templates/pluris_policy_helpers.go`, `web/lists/pluris_roles.go`. Modify `web/templates/menu.go` (sidebar child under Policy after Dependency Groups, key `policy-pluris`), `web/templates/pages.templ` (`PolicyTabs` gains "Pluris Policy" tab), `console/server/server_test.go` (mountPoints smoke case).

**Interfaces:** Consumes Task 6 handler data. Produces `PlurisPolicyPage(rows []PlurisRoleRow)` + `PlurisPolicyDetailPage(role db.Role, grants authz.Grants, members []db.ListIdentitiesForRoleRow, csrf string)`.

- [ ] Step 1: failing test — mountPoints case `{name:"pluris-policy", path:"/policy/pluris", expectStatus:200, expectTestID:`data-testid="page-pluris-policy"`}`.
- [ ] Step 2: run TestMountPoints — fails 404.
- [ ] Step 3: implement. LIST page: mirror `DependencyGroupsPage` EXACTLY (Layout+PageHeader+PolicyTabs wrapper, `<thead>` from `lists.FieldsFor(lists.ListIDPlurisRoles)` — INV-L, registered fields: Name/Type/Members/Permissions with DefaultVisible:true, Group:"main"), row-click nav script to `/policy/pluris/<id>`, Builtin/Custom chips; "+ New role" is a small inline clone form (source-role `<select>` of the 4 builtins + name input + `_csrf`, POST to `/policy/pluris/{sourceID}/clone`) — no separate /new page. DETAIL page: DetailShell (mirror `dependency_groups.templ` + its helpers file): hero = role name/slug + Builtin|Custom chip, delete form in `HeroSpec.DeleteForm` (disabled+tooltip for builtin or member-count>0); tabs = Permissions + Members. Permissions tab: `for _, d := range permissions.All()` one card per domain (`asset-detail-section card` style), one row per action (label + Description as title attr); scoped → `<select name={"perm_"+full}>` none/own/all; unscoped → checkbox `name={"perm_"+full}` value "yes"; whole matrix inside ONE `<form method="post" action=/policy/pluris/{id}>` with `_csrf` + Apply button; builtin → render selects/checkboxes `disabled` + banner + the clone form instead of Apply. Members tab: `@DetailTableFrame` with a NEW registered list id `pluris-role-members` (Username/Display name/Email columns) and `@DetailEmptyRow` when empty. Current-value rendering helper in `pluris_policy_helpers.go`: selected/checked from `grants[key]` defaulting none/no.
- [ ] Step 4: `make gen`; TestMountPoints passes; full suite green; gofmt clean.
- [ ] Step 5: STOP and report.

### Task 8: Field-update API (users + assets) + JS wiring

**Files:** Create `console/handlers/field_api.go`, `console/handlers/field_api_test.go`. Modify `pkg/services/identities.go` (UpdateFields + self-service allowlist), `pkg/services/assets.go` (UpdateFields), `db/queries/assets.sql` (UpdateAssetPayload if missing), `console/server/server.go` (routes), `web/static/detail.js` (fetch wiring per `docs/agent/Small agent output/inline-edit-save.md` §JS Wire-Up).

**Interfaces:** Consumes Task 5 helpers. Produces `POST /api/users/:id/fields`, `POST /api/assets/:subtype/:id/fields` with `{section, fields{key:value}}` → `{"updated":[...]}` | 400 `{"error":"..."}`; `IdentityService.UpdateFields(ctx, tenantID, id int64, section string, fields map[string]string) ([]string, error)`; `AssetService.UpdateFields(ctx, tenantID, assetID int64, subtype, section string, fields map[string]string) ([]string, error)`; `identities.SelfServiceEditableKeys` allowlist.

- [ ] Step 1: failing tests: admin-grants session updates another user's email+title → 200, DB reflects it; user-grants session updates OWN allowlisted field (phone_mobile) → 200; user updates own NON-allowlisted field (department) → 400/403; user updates ANOTHER user → 403; unknown section or key not in that section → 400; bool param with "notabool" → 400; cross-tenant target → 404; asset: admin updates `ram_mb` (int coercion into subtype payload JSON) → 200 and payload reflects; CSRF: the API routes are POSTs under the existing global CSRF middleware — tests must send the token header the same way existing handler tests do (read how server_test.go doCSRFPost extracts it; for echo.NewContext-style handler tests CSRF middleware is absent — fine, test at handler level like roles_test.go does).
- [ ] Step 2: run — fails.
- [ ] Step 3: implement. Services: identity UpdateFields = Get → validate section/keys via `params.SchemaByPathEntity("user")` sections → coerce per ParamDef.Type (string passthrough; bool strconv.ParseBool; int strconv.ParseInt; list = comma-split trim; skip TypeLink/readonly keys like id/tenant with 400 "not editable") → set struct fields via a `switch key` (write the REVERSE of `web/lists/identities.go:getIdentityParamValue` for the editable subset; unsupported key → 400) → `s.Update`. Asset UpdateFields = fetch (find the existing get-by-dbid pattern) → unmarshal SubtypePayload → validate keys against the subtype schema's section → coerce → merge into payload map → marshal → UpdateAssetPayload. Allowlist `SelfServiceEditableKeys` (in `catalog/identities/types.go`): display_name, given_name, surname, initials, email, phone_office, phone_mobile, phone_home, fax, office, street_address, city, state, postal_code, country, locale, timezone. Handler: parse JSON body, resolve target (reuse `resolveTenantIdentity` / asset equivalent — 404 cross-tenant), gate: `identity.update` scoped with ownerID=target id (assets: ownerID = asset owner id; unowned asset → Own never passes); if acting scope is Own (not All), reject keys outside the allowlist; call service; activity log `user_updated`/`asset_updated`; JSON responses per spec. detail.js: replace the console.log stub in `saveSectionEdit` with the fetch per inline-edit-save.md §JS Wire-Up (CSRF from `[name=_csrf]` input — verify one exists on detail pages; if not, add `<meta name="csrf-token">` to DetailShell and read that), on success run the existing span-update + remove the `.save-stub-note`; on !ok keep edit mode + alert error.
- [ ] Step 4: `make gen` (if .templ touched); focused + full suite green; gofmt clean.
- [ ] Step 5: STOP and report.

### Task 9: Avatar upload backend

**Files:** Create `console/handlers/avatar.go` + tests in `field_api_test.go` or own file. Modify `console/server/server.go` (route + static), `web/static/detail.js` (`applyAvatarFile` POSTs), `.gitignore` (`/data/`), `pkg/services/identities.go` (SetAvatarURL — check if Update already writes avatar_url; if yes reuse).

**Interfaces:** `POST /api/users/:id/avatar` multipart field `avatar`; png/jpeg/webp; ≤2MB; stores `data/avatars/<id>.<ext>` under the server working dir; sets `identities.avatar_url` to `/avatars/<id>.<ext>`; `GET /avatars/*` static route (echo `e.Static("/avatars","data/avatars")`). Gate `identity.update` scoped (own or all).

- [ ] Step 1: failing tests: multipart upload of a tiny valid PNG (embed 1x1 png bytes in the test) as self with user grants → 200, file exists under scratch dir, avatar_url set; wrong MIME (text/plain) → 400; >2MB (generate) → 400; other-user target with user grants → 403. Tests must chdir or configure the data dir to a t.TempDir() — add a package-level `AvatarDir` variable (default "data/avatars") the test overrides.
- [ ] Step 2: run — fails.
- [ ] Step 3: implement (sniff content type via http.DetectContentType on the first 512 bytes, not the client header; ext from detected type; os.MkdirAll AvatarDir 0o755; overwrite existing; delete old file when ext changes is nice-not-required). detail.js: in `applyAvatarFile`, after the local preview, POST the file via FormData + CSRF header to the endpoint; on failure alert + revert nothing (page reload shows truth).
- [ ] Step 4: focused + full suite green.
- [ ] Step 5: STOP and report.

### Task 10: E2E + docs + handoff

**Files:** Modify `docs/agent/HANDOFF.md`, `README.md`, `docs/agent/Small agent output/session.md` (mark backend-wiring done), `docs/UX_INVARIANTS.md` ONLY IF it hard-codes the old role matrix (grep "Role permission matrix" — update the section to point at the registry). E2E script in scratchpad (NOT repo).

- [ ] Step 1: full regen + build + suite + gofmt.
- [ ] Step 2: headless e2e (scratch dir server on :8094, model on the dependency-groups e2e): setup+login as super-admin → `/policy/pluris` 200 with all 4 builtin templates listed → POST clone of Technician ("Helpdesk L1") → 302; POST its matrix with `identity.delete` flipped to yes minus `endpoint_policy.manage_modules` → detail reflects; create a normal user, assign the user builtin role (existing roles UI POST), login as that user (setup a password path — check how test users get passwords; if only the setup admin has one, exercise the field API with the admin session for Own semantics via a second admin-created account with known password using UserCreateSubmit + whatever password flow exists; if no password-set flow exists for created users, do the self-service checks at handler-test level instead and note it) → user PATCHes own email via `/api/users/:id/fields` → 200; user calling DELETE-equivalent (UserDeleteSubmit) on another user → 403; avatar upload happy path → file served at /avatars/... Zero 5xx throughout.
- [ ] Step 3: docs — HANDOFF: new plan section, tasks 1-10 DONE with test names; README status line for Pluris Policy; session.md note.
- [ ] Step 4: STOP and report (owner commits; then manual browser pass).

## Self-review notes

- Spec coverage: registry+templates (T1), storage/resolution/clone (T2), session (T3), route layer+menu (T4), handler gates (T5), Pluris Policy backend+UI incl. self-lockout+clone+members (T6-7), field API+allowlist+JS (T8), avatar (T9), e2e+docs (T10). Out-of-scope items untouched.
- Type consistency: `authz.Grants`, `TemplateGrants`, `RoutePermissionKey`, `requirePermission(Scoped)`, `UpdateFields` signatures appear identically in shared reference and tasks.
- Known implementer-verification points (named in-task): CreateRoleParams shape, DeleteRole query existence, assets update query/column names, menu.go session threading, CSRF token availability on detail pages, password flow for created users (T10 fallback provided).