# Authorization — Pluris Policy

**What:** The console's zero-trust access-control system — GLPI-style `domain.action` grants, role inheritance, group roles, and the three enforcement layers that gate every route, handler, and service call.

**Related:** [[parameters]], [[identity-assets]], [[data-model]], [[endpoint-policy]]

> **Naming callout.** "Pluris Policy" (this document) is authorization for the **console itself** — who can view/edit users, assets, roles, and the policy admin surfaces. It is unrelated to **Endpoint Policy** ([[endpoint-policy]]), which is configuration pushed to **managed devices**. Same word, two different systems; see the disambiguation box at the top of [[endpoint-policy]].

## Model overview

GLPI-style **domain → action → scope** grants, Jira-style named keys, deny-by-default, allow-only union across roles. Each permission is a canonical key `domain.action` (e.g. `identity.update`). Two shapes:

- **Scoped** actions grant `none` / `own` / `all` (rank `all > own > none`).
- **Unscoped** actions grant `no` / `yes` (rank `yes > no`).

`super_admin` bypasses every check in code — never via a stored grant — so self-lockout of the platform's top role is structurally impossible.

## The registry — `catalog/permissions/`

Pure Go, zero non-stdlib imports, immutable after `init()`. This is the ITSM extension point: adding a feature's authorization surface means adding a `Domain`/`Action` here, nothing else in the engine changes.

```go
type Action struct {
    Key, Label, Description string
    Scoped bool // true -> None/Own/All; false -> yes/no
}
type Domain struct {
    Key, Label string
    Actions []Action
}
func All() []Domain                    // registration order = matrix display order
func ActionByKey(full string) *Action  // "identity.update" -> *Action, nil if unknown
func AllKeys() []string                // every canonical "domain.action"
func BuiltinSlugs() []string           // {"super_admin","admin","technician","user"}, privilege order
```

`init()` panics on a duplicate key, mirroring the params registry's fail-fast pattern.

### v1 domains (5 domains, 23 actions)

| Domain | Actions |
|---|---|
| `identity` | `view`\*, `update`\*, `create`, `delete`, `assign_roles`, `assign_groups` |
| `asset` | `view`\*, `update`\*, `create`, `delete`, `set_owner`, `manage_groups` |
| `endpoint_policy` | `view`, `manage_catalog`, `manage_config_groups`, `manage_dependency_groups`, `manage_modules`, `assign_policies` |
| `console_access` | `view_roles`, `manage_role_assignments`, `manage_permissions` |
| `server_admin` | `access`, `tenant_switch` |

\* scoped (`None`/`Own`/`All`); every other action is unscoped (`no`/`yes`). "Own" for `identity.*` means target identity id == session identity id; for `asset.*` it means the asset's owner identity == session identity id.

**Adding a permission domain:** append a `Domain` literal to `catalog/permissions/registry.go`'s `domains` slice, add the corresponding builtin-template rows in `templates.go`, then wire enforcement calls (`requirePermission`/`requirePermissionScoped`) at the new feature's handlers/services. No schema change, no engine change.

## Builtin templates

The 4 builtin roles are **read-only templates**; customizing means cloning into a custom role. `catalog/permissions/templates.go` seeds each:

| Domain | Super Admin | Admin | Technician | User |
|---|---|---|---|---|
| `identity.*` | all/yes everywhere | all/yes everywhere | view/update `all`; create `yes`; delete/assign_roles `no`; assign_groups `yes` | view/update `own`; everything else `no` |
| `asset.*` | all/yes everywhere | all/yes everywhere | view/update `all`; create/delete/set_owner/manage_groups `yes` | view `own`; update `none`; everything else `no` |
| `endpoint_policy.*` | yes everywhere | yes everywhere | yes everywhere | `view` only, rest `no` |
| `console_access.*` | yes everywhere | yes everywhere | `view_roles` yes; manage_* `no` | `no` everywhere |
| `server_admin.*` | yes everywhere | `access` yes, `tenant_switch` **no** | `no` everywhere | `no` everywhere |

Super Admin's stored matrix is display-only — the code bypass (below) is what actually grants access. `EnsureBuiltinGrants(ctx, tenantID)` idempotently fills any builtin role's absent keys from its template, merge-only (never overwrites a stored value); it runs at setup and lazily from the Pluris Policy list page.

## Role inheritance — `pkg/authz.Service`

Roles form a tree via `roles.parent_role_id`. **Builtins are always parentless roots** (`ErrBuiltinParent` guards this); custom roles may parent to a builtin or another custom role in the same tenant.

- **Storage = own overrides only.** A parented role's `permissions` JSON holds only the keys it explicitly overrides; a parentless role (all builtins, standalone customs) stores its full matrix, unchanged from pre-inheritance behavior.
- **`ResolveRoleMatrix(role)`** walks the parent chain root→leaf, merging each level's own keys over the previous — child overrides parent per key, missing keys fall through to the ancestor's value (or the registry default). Capped at `MaxRoleDepth` = 5 (counting the role itself), defensive against data ever landing deeper than `SetRoleParent` allows.
- **`SaveRoleOverrides(roleID, full)`** — the matrix form always submits all 23 keys. For a parentless role, the full matrix is stored verbatim. For a parented role, the parent's effective matrix is resolved and completed with `none`/`no` defaults for any registry key it has no opinion on, then only the keys that **differ** from that completed parent matrix are stored — equal values are dropped, meaning "inherit". This keeps the diff stable regardless of which keys happen to be set up the chain.
- **`SetRoleParent(roleID, parentID)`** guards, in order: role must exist and not be builtin (`ErrBuiltinParent`); clearing (`parentID == 0`) skips the rest; new parent must exist in the same tenant (else not-found); `parentID == roleID` or reachable by walking `parentID`'s own ancestor chain is a cycle (`ErrRoleCycle`); resulting chain depth over `MaxRoleDepth` is also `ErrRoleCycle`.
- **`CreateCustomRole(tenantID, name, parentID)`** creates a deny-all (`{}`) custom role and optionally sets its parent through `SetRoleParent`, so parent-assignment at creation gets the same guards as changing it later.

## Group roles

`group_roles` (migration 005) attaches roles to a `groups` row; every identity member inherits those roles in addition to any directly-assigned roles. `EffectiveGrants(ctx, identityID)` unions **direct ∪ group-via** roles: it fetches both `ListRolesForIdentity` and `ListGroupRolesForIdentity`, de-dupes by role ID (a role held both directly and via a group resolves only once), resolves each surviving role through `ResolveRoleMatrix`, and takes `Union` across the results.

`identities.role` is a denormalized privilege-cache column (login/session bootstrap + the super-admin marker only — never consulted for permission checks past that). Recompute triggers: direct role assign/remove, group-role assign/remove (`RoleService.RecomputeForGroupMembers`, all members), and group-membership add/remove (that one member).

## Enforcement — three layers

1. **Route middleware** (`pkg/auth/rbac.go`) — `routePermissionKey` maps a route prefix to a permission key (longest-prefix match, `RoutePermissionKey`); `""` means open to any authenticated session. `RequireRole()` (kept its old name to avoid churn in `server.go`'s middleware wiring) checks `CanAccessGrants(sess.Grants, path)` — a 403 on failure. Scoped view keys pass at `own` or `all`; row-level filtering to the caller's actual scope stays in the service layer.
2. **Handler gates** — `console/handlers/authz_helpers.go`'s `requirePermission(c, key)` and `requirePermissionScoped(c, key, ownerID)` are the per-mutation defense-in-depth layer; every Pluris Policy mutation (`console/handlers/pluris_policy.go`), group-role assignment (`console/handlers/group_roles.go`), and CRUD handler across identity/asset/endpoint-policy domains calls one of these before touching data. A nil session (no `RequireAuth` run) always denies.
3. **Service self-scoping** — field-update services (`IdentityService.UpdateFields`, `AssetService.UpdateFields`) receive the resolved scope and target owner, and enforce `own` at the row level; the handler resolves `CanScoped` once, the service never re-derives grants.

**Menu gating.** `web/templates/menu.go` consults the exact same `RoutePermissionKey`/`CanAccessGrants` pair the route middleware uses (see `pkg/auth/rbac.go`), so a sidebar item and its route are gated by one source of truth — no separate "show/hide" list to drift.

### Per-request resolution (`pkg/auth/middleware.go`)

Three steps, once per request, in `RequireAuth`:

1. **Session** — cookie → `SessionManager.Lookup` → identity row → `UserSession{IdentityID, Email, Role, TenantID, ...}`.
2. **Grants** — `super_admin` sessions get `Grants{authz.BypassKey: "yes"}` (no DB grant resolution at all); every other session calls `authzSvc.EffectiveGrants(ctx, identity.ID)`; a resolution error degrades to `Grants{}` (deny-all), logged, never a hard failure.
3. **Checks** — `sess.Grants` is stashed on the request context (`auth.WithSession`) for `RequireRole()`, every handler's `requirePermission*` call, and the menu renderer to read.

## UI — `/policy/pluris`

- **List page** — all tenant roles: Name · Type (Builtin/Custom) · Parent · Members (direct+group) · Permissions summary ("14 granted"). Search + quick filters (All/Builtin/Custom + per-template-family chips). **Create role** button (name + optional parent select).
- **Detail page** (`PlurisPolicyDetail`) — hero shows the parent-chain breadcrumb and Builtin/Custom chip; ⋮ menu carries Delete (builtin-protected, and blocked while the role has members or child roles) and Clone (copies the source's stored overrides + traces `template_slug` back to a builtin).
  - **Permissions tab** — the matrix editor: each row shows the role's **effective** value plus an **origin badge** — `own` (explicit override on this role), `inherited from <role>` (nearest ancestor whose own overrides carry the key), or `default` (nothing in the chain has an opinion). A per-row "reset to inherited" clears an override on parented roles, showing what `roleInheritedGrants` computes as the parent's completed effective matrix. Builtin roles: controls disabled, banner directs to Clone.
  - **Members tab** — direct identity members + groups holding this role.
  - **Settings tab** — name/description/parent selector (custom roles only; cycle-rejecting via `SetRoleParent`).
- **Hierarchical pickers** — every role-select surface (user Roles tab, group Roles tab, create-role parent picker) groups by template family with `<optgroup>` + indentation, no JS tree dependency.

## Security notes

- **`super_admin` code bypass.** Bypass is structural (`authz.BypassKey` marker set in `RequireAuth`, never a storable grant value), not a matrix row — a super_admin's stored matrix is cosmetic/display-only.
- **Self-lockout guard (lowering only).** `PlurisPolicySave` (`console/handlers/pluris_policy.go`'s `wouldLockOutActor`) rejects a matrix save that would leave the **acting**, non-super-admin identity without `console_access.manage_permissions` — resolving every OTHER role the actor holds (direct or via group) through its full inheritance chain, unioned with the candidate submission for the role being edited. Super_admin sessions are exempt (they always carry the bypass grant). This guard only prevents *lowering* your own access — it is not a general raise-guard.
- **Accepted caveats (verbatim-honest, not yet closed):**
  - `console_access.manage_role_assignments` is **effectively admin-equivalent** — a holder can assign any role (including admin-level custom roles) to any identity or group, which is a strictly more powerful lever than the permission's name suggests.
  - `SetRoleParent` can **raise** a `console_access.manage_permissions` holder's own effective grants (by re-parenting a role they hold onto a more privileged ancestor) — the self-lockout guard only catches the lowering direction; a symmetric raise-guard is recorded as next-phase hardening, not built.
  - `GroupRoleAssign` (`console/handlers/group_roles.go`) applies only a blanket "you cannot assign roles to a group you belong to" speed bump, not a real comparison of the role's grant set against the actor's own — parity with the pre-existing `UserRoleAssign` self-service refusal, not a full raise-guard.

## Code pointers

- Registry: `catalog/permissions/registry.go`, `catalog/permissions/templates.go`
- Grants combinators: `pkg/authz/grants.go`
- Service (resolution, inheritance, persistence): `pkg/authz/service.go`
- Route map + middleware glue: `pkg/auth/rbac.go`, `pkg/auth/middleware.go`
- Handler gates: `console/handlers/authz_helpers.go`
- Pluris Policy handlers: `console/handlers/pluris_policy.go`
- Group role assignment: `console/handlers/group_roles.go`
