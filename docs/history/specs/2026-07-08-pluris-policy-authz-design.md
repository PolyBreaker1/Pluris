# Pluris Policy — Console Zero-Trust Authorization + User Management Backend

**Date:** 2026-07-08
**Author:** Claude (Opus 4.8) + Peter (owner)
**Status:** Approved (design approved verbally; plan pre-approved)

---

## Goal

Replace the hardcoded route-prefix RBAC with a real, flexible, zero-trust permission system for the Pluris console itself ("Pluris Policy"), and ship proper user creation/editing (the inline-edit backend) as its first enforced consumer.

Model: GLPI-style **domain → action → scope** grants, Jira-style named keys, no ServiceNow rule engine. Default deny; allow-only union across roles; `super_admin` bypasses in code.

## Decisions (locked with owner)

1. **Grant shape**: GLPI-style. Each permission is `domain.action`. *Scoped* actions grant **None / Own / All**; *unscoped* actions are **yes / no**.
2. **Templates**: the 4 builtin roles (Super Admin, Admin, Technician, User) are **read-only templates**; customizing = **clone into a custom role** and edit its matrix. Super Admin bypasses all checks in code (self-lockout impossible).
3. **Registry scope**: **enforced-only** — the matrix shows only domains with real enforcement points in this build. Future features (tickets, AD import, self-service catalog) add registry entries with their own builds.

## Planned future usage this must support (not build)

- Import/create users from AD.
- Admins solve incidents, assign software.
- End users edit their own info, choose firewall settings from a catalog, create tickets via a self-service portal.
The registry pattern must make each of these a small additive change (new domain + enforcement calls), never a schema/engine rework.

---

## 1. Permission registry — `catalog/permissions/` (pure Go, no DB imports)

```go
type ScopeLevel string // "none" | "own" | "all"  (scoped actions)
                       // "no" | "yes"            (unscoped actions)

type Action struct {
    Key         string // "update" — full key is domain.action
    Label       string // "Update account"
    Description string // one line, shown as row tooltip in the matrix
    Scoped      bool   // true → None/Own/All; false → yes/no
}

type Domain struct {
    Key     string // "identity"
    Label   string // "Identity Management"
    Actions []Action
}

func All() []Domain               // registration order = matrix order
func ActionByKey(full string) *Action // "identity.update" → *Action; nil if unknown
func AllKeys() []string           // every canonical "domain.action"
```

Registry `init()` panics on duplicate keys (mirrors params registry).

### v1 domains and actions

| Full key | Label | Scoped |
|---|---|---|
| `identity.view` | View accounts | yes |
| `identity.update` | Update account | yes |
| `identity.create` | Create account | no |
| `identity.delete` | Delete account | no |
| `identity.assign_roles` | Assign roles | no |
| `identity.assign_groups` | Manage group membership | no |
| `asset.view` | View assets | yes |
| `asset.update` | Update asset | yes |
| `asset.create` | Create asset | no |
| `asset.delete` | Delete asset | no |
| `asset.set_owner` | Set asset owner | no |
| `asset.manage_groups` | Manage asset group membership | no |
| `endpoint_policy.view` | View endpoint policy | no |
| `endpoint_policy.manage_catalog` | Manage policy catalog | no |
| `endpoint_policy.manage_config_groups` | Manage configuration groups | no |
| `endpoint_policy.manage_dependency_groups` | Manage dependency groups | no |
| `endpoint_policy.manage_modules` | Manage policy modules | no |
| `endpoint_policy.assign_policies` | Assign policies to targets | no |
| `console_access.view_roles` | View roles and permissions | no |
| `console_access.manage_role_assignments` | Assign/remove roles on users | no |
| `console_access.manage_permissions` | Create/edit/delete custom roles | no |
| `server_admin.access` | Access server administration | no |
| `server_admin.tenant_switch` | Switch tenants | no |

"Own" semantics: for `identity.*` scoped actions, Own = target identity id equals session identity id. For `asset.*` scoped actions, Own = the asset's owner (assigned identity) equals session identity id.

### Builtin template matrices (seeded)

- **Super Admin**: bypass in code; stored matrix = everything `all`/`yes` (display only).
- **Admin**: everything `all`/`yes` EXCEPT `server_admin.tenant_switch: no` (tenant switching stays super-admin-only, matching current behavior).
- **Technician**: identity view/update `all`, create `yes`, delete `no`, assign_roles `no`, assign_groups `yes`; asset view/update `all`, create/delete/set_owner/manage_groups `yes`; endpoint_policy all `yes` except `manage_modules: yes` (matches current technician = admin minus server-admin); console_access view_roles `yes`, manage_* `no`; server_admin all `no`.
- **User**: identity view `own`, update `own`, everything else `no`; asset view `own`, update `none`, others `no`; endpoint_policy view `yes` (read-only browsing, matches current nav), manage_* and assign `no`; console_access all `no`; server_admin all `no`.

## 2. Storage & resolution — `pkg/authz/`

**Storage**: existing `roles.permissions TEXT NOT NULL DEFAULT '{}'` column. JSON object, uniform string values:
`{"identity.update":"own","identity.create":"yes","asset.view":"all", ...}`
Missing key = deny (`none`/`no`). Unknown keys in stored JSON are ignored on read (forward compat). **No schema migration.**

**Seeding**: `AuthzService.EnsureBuiltinGrants(ctx, tenantID)` — idempotent; for each builtin role whose `permissions` is `'{}'` (or missing keys added since), writes/merges the template matrix. Runs at setup and lazily from the Pluris Policy page (mirrors `roleSvc.EnsureBuiltins`). Never overwrites a non-default value on merge (only fills absent keys).

**Resolution**:
```go
type Grants map[string]string // canonical key → "none|own|all|no|yes"

// EffectiveGrants: union of all roles assigned to the identity.
// Rank: all > own > none ; yes > no. Custom roles read from their JSON;
// super_admin short-circuits via Grants{superAdminBypass: ...} marker.
func (s *AuthzService) EffectiveGrants(ctx, identityID int64) (Grants, error)

func (g Grants) Can(key string) bool                       // unscoped: yes
func (g Grants) CanScoped(key string, ownerID, selfID int64) bool
    // all → true; own → ownerID == selfID; none → false
func (g Grants) ScopeOf(key string) string                 // for menus/UI
```
Grants are resolved **once per request** in the auth middleware and stored on the session context (`auth.Session` gains a `Grants` field, populated after session load). Session struct changes ripple through `auth.WithSession` test helpers — tests construct Grants directly.

**Update queries**: one new sqlc query `UpdateRolePermissions(id, permissions)`; plus `CloneRole` is service-level (CreateRole + copy JSON).

## 3. Enforcement — three layers

1. **Route middleware** (`pkg/auth/rbac.go` REPLACED): the prefix map values become permission keys, not role booleans:
   ```go
   var routePermission = map[string]string{
       "/users":         "identity.view",
       "/assets":        "asset.view",
       "/policy":        "endpoint_policy.view",
       "/policy/pluris": "console_access.view_roles",
       "/server-admin":  "server_admin.access",
       "/tenant-switch": "server_admin.tenant_switch",
       ...
   }
   ```
   Longest-prefix match kept. Scoped view keys pass at Own or All (row filtering stays in services). `identities.role` remains ONLY as the login/session bootstrap + super-admin marker.
2. **Handler gates**: new helper `requirePermission(c, key)` / `requirePermissionScoped(c, key, ownerID)` in `console/handlers`; existing `requireRoleAdmin` call sites migrate:
   - roles.go handlers → `console_access.manage_role_assignments`
   - dependency-group CRUD → `endpoint_policy.manage_dependency_groups`
   - module link handlers → `endpoint_policy.manage_modules`
   - policy picker/assign → `endpoint_policy.assign_policies`
   - user create/delete → `identity.create` / `identity.delete`
   - asset owner/groups → `asset.set_owner` / `asset.manage_groups`
   `requireRoleAdmin` is deleted when the last caller migrates.
3. **Service self-scoping**: field-update services take the acting session and enforce Own.

**Menu gating**: sidebar items render only when the session grants pass that route's key (menu.go consults the same routePermission map — single source of truth).

## 4. Pluris Policy UI — `/policy/pluris`

- Sidebar: Policy child "Pluris Policy" (after Dependency Groups), key `policy-pluris`; PolicyTabs gains the matching tab. Route gated by `console_access.view_roles`.
- **List page**: all roles in tenant — columns (INV-L registry list id `pluris-roles`): Name · Type (Builtin/Custom) · Members count · Permissions summary (e.g. "14 granted"). Row-click → detail. "+ New role (clone)" → picker of the 4 templates.
- **Detail page** (DetailShell):
  - Hero: role name, slug, Builtin/Custom chip; delete in ⋮ dropdown (custom only; builtin disabled with tooltip; also disabled while the role has members — remove members first, keeps semantics simple).
  - **Permissions tab** — the EDM matrix: one card per domain; each row = action label + description tooltip; scoped rows render a three-state selector (None/Own/All), unscoped a checkbox. Builtin: controls disabled + banner "Builtin template — clone to customize" + Clone button. Custom: editable, single Apply POSTs the whole matrix (read-modify-write of the JSON).
  - **Members tab** — identities assigned this role (read-only list; assignment stays on the user detail Roles tab).
- **Clone flow**: POST `/policy/pluris/:id/clone` with new name → creates custom role (template_slug = source slug, permissions = copy) → redirect to its detail.
- **Self-lockout guard**: saving a custom role's matrix is rejected (400, message) when the acting (non-super-admin) user holds that role and the new matrix drops `console_access.manage_permissions` from their effective grants.
- All mutations gated by `console_access.manage_permissions`; cross-tenant ids 404 via `resolveTenantRole`.

## 5. User management backend (flagship consumer)

Implements `docs/agent/Small agent output/inline-edit-save.md` (frontend contract already shipped), enforced by authz:

- `POST /api/users/:id/fields` — JSON `{section, fields{key:value}}`. Validates section+keys against the user schema (`catalog/params`), coerces types per ParamDef.Type, updates via `IdentityService.UpdateFields`. Gate: `identity.update` scoped (Own → only self, and Own additionally restricted to a **self-service field allowlist**: contact/personal params, never role/site/employment fields — allowlist defined next to the user schema). Cross-tenant 404. Response `{"updated":[...]}` / 400 with `{"error": "..."}`.
- `POST /api/assets/:subtype/:id/fields` — same shape for assets via `AssetService.UpdateFields` (JSON-payload merge for subtype params). Gate: `asset.update` scoped.
- `POST /api/users/:id/avatar` — multipart image (png/jpeg/webp, ≤2 MB), stored under `data/avatars/<id>.<ext>` (dir gitignored), sets `identities.avatar_url` to `/avatars/...` (static route added). Gate: `identity.update` scoped. Frontend `applyAvatarFile` wired to actually POST.
- `web/static/detail.js` `saveSectionEdit` fetch wiring per the spec doc; on success remove the "not saved" stub note.
- Existing user create/delete/role handlers migrate to `identity.create` / `identity.delete` / `identity.assign_roles`.

## 6. Testing

- Registry: unique keys, every builtin-template key exists in registry, every routePermission value exists in registry.
- Resolution: union ranking (all>own>none, yes>no) across two roles; super_admin bypass; unknown stored keys ignored; empty permissions = deny-all.
- Middleware: table-driven — each role template vs each route prefix (replaces the old rbac tests).
- Pluris Policy handlers: matrix save persists; builtin save 403/400; clone creates copy; self-lockout guard 400; cross-tenant 404; member-list renders.
- Field API: own-scope allows self-edit of allowlisted field, denies non-allowlisted; all-scope edits anyone; bad section/key/type 400; cross-tenant 404; technician-vs-user matrix.
- Avatar: happy path writes file + URL; wrong MIME/oversize 400.
- Headless e2e: login as seeded admin → open Pluris Policy → clone Technician → edit matrix → assign to a second user (via user detail) → verify that user's nav/allowed actions changed; user edits own email via inline-edit API; user cannot delete accounts.

## Constraints & invariants

- No new Go dependencies. No DB migration (JSON column exists). ASCII-only SQL comments. `-buildvcs=false`. Never touch repo-root `pluris.db*`; tests on `t.TempDir()`. `make gen` after `.templ`; `sqlc generate` after `.sql`. DetailShell + INV-L + INV-CPP respected. Owner owns git — no commits by agents.
- Session/RBAC changes are the highest-blast-radius area in the repo (`pkg/auth/`): the plan sequences them so every task leaves the suite green and login working.

## Out of scope

Tickets, AD import, self-service catalog UI, SP1 visual user-page redesign, field-level permissions, permission audit log (activity_log rows for role/permission changes ARE in scope — one line each, existing pattern).

## File map

**Create:** `catalog/permissions/{registry.go,registry_test.go,templates.go}` · `pkg/authz/{service.go,grants.go,service_test.go,grants_test.go}` · `console/handlers/{pluris_policy.go,pluris_policy_test.go,field_api.go,field_api_test.go,avatar.go}` · `web/templates/{pluris_policy.templ,pluris_policy_helpers.go}` · `web/lists/pluris_roles.go`
**Modify:** `pkg/auth/rbac.go` (replace) + `session.go` (Grants field) · `console/server/server.go` (routes, static avatars) · `console/handlers/{roles.go,dependency_groups.go,policy_picker.go,handlers.go,auth.go,groups.go}` (gate migration + seeding) · `pkg/services/{identities.go,assets.go,roles.go}` (UpdateFields, clone) · `db/queries/roles.sql` (UpdateRolePermissions) · `web/templates/menu.go` + `pages.templ` (nav gating, PolicyTabs) · `web/static/detail.js` (fetch wiring) · `.gitignore` (data/avatars)
