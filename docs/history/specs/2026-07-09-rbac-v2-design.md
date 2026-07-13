# RBAC v2 — Role Inheritance, Group Roles, Standardized Role UX, Full-Page User Create

**Date:** 2026-07-09
**Author:** Claude (Opus 4.8) + Peter (owner)
**Status:** Approved (owner answered the three design questions; standing pre-approval to proceed)
**Builds on:** `2026-07-08-pluris-policy-authz-design.md` (registry, grants engine, matrix UI — all shipped)

## Owner requirements (verbatim intent)

- Roles are separate entities; one user can have multiple roles; roles apply to users OR groups.
- Role pages need the standardized layout + modern list UX (filters, search) like users/assets/policy.
- List page gets a Create button; Clone moves INTO the role detail.
- Flexible, HIERARCHICAL roles: GLPI/ServiceNow-style inheritance (owner's explicit pick).
- Dependency-groups list also gets the filter toolbar.
- /users/new becomes the FULL standardized user layout, blank and ready to fill (no small form).
- Enterprise-grade: versatile, new features must slot in.

## 1. Data model — migration `005_role_hierarchy_group_roles.sql` (append-only, no PRAGMA)

- `ALTER TABLE roles ADD COLUMN parent_role_id INTEGER REFERENCES roles(id) ON DELETE SET NULL;`
- New table `group_roles`: id, group_id FK groups ON DELETE CASCADE, role_id FK roles ON DELETE CASCADE, assigned_at, assigned_by (nullable FK identities SET NULL), UNIQUE(group_id, role_id).
- Rules: builtin roles NEVER have a parent (enforced in service, not schema). Custom roles may parent to a builtin or custom role in the same tenant. Max chain depth 5. Cycles rejected at write time.

## 2. Inheritance semantics (`pkg/authz`)

- **Storage = own overrides only.** A role's `permissions` JSON holds ONLY the keys it explicitly overrides. Roles without a parent (all builtins, standalone customs) store their full matrix as today — no data migration needed.
- **Resolution:** `ResolveRoleMatrix(role)` = walk parent chain root→leaf, merging each level's own keys over the previous (child overrides parent per key). Cycle/depth guarded (defensive at read too: stop at depth 5).
- **Override-diff save:** the matrix form still submits ALL 23 keys; for a role WITH a parent the server computes the parent's effective matrix and stores only the keys that DIFFER (equal values are dropped = "inherit"). For parentless roles, store all keys (today's behavior).
- **EffectiveGrants v2:** union over (direct roles ∪ roles assigned to any group the identity belongs to), each first resolved through inheritance. Rank rules unchanged (all>own>none, yes>no). Super-admin bypass unchanged.
- **Privilege cache (`identities.role`):** recompute considers direct + group roles; recompute triggers extend to: group-role assign/remove (all group members) and group-membership add/remove (that member).
- Self-lockout guard and the matrix editor operate on inheritance-RESOLVED effective values.
- Consolidate builtin slugs into `permissions.BuiltinSlugs()` (removes the duplicate lists — deferred Minor #1 from the last review).

## 3. Role management surfaces

### List `/policy/pluris` — standardized modern list
Search box + quick filters (All / Builtin / Custom, plus per-template-family chips) wired via the existing `data-pluris-filter` list infra (mirror the config-groups / policy-modules toolbars); columns via list registry: Name, Type, Parent, Members (direct+groups), Permissions summary. **Create role** button (replaces list-level Clone) → small inline panel or dedicated create step: name (required) + parent role select (default: none; choosing a builtin parent ≈ old clone-from-template but LIVE-inherited). Row-click → detail.

### Detail `/policy/pluris/:id` — DetailShell (existing) extended
- Hero: parent chain breadcrumb ("Technician → Helpdesk L1 → *this*"), Builtin/Custom chip, member count. ⋮ menu: Delete (existing rules) + **Clone** (moved here from the list; clone = new role with same parent + copied overrides).
- **Permissions tab:** matrix rows show the EFFECTIVE value; each row carries an origin badge — `own` (explicit override) vs `inherited from <role>` (or `default` for parentless). Editing a control marks the row own; a per-row "reset to inherited" control clears the override (parented roles only). One Apply as today.
- **Members tab:** direct identity members + a second table of groups holding this role.
- **Settings (General) tab:** name, description, parent selector (custom roles only; cycle-rejecting; changing parent re-renders matrix origins).

### Pickers — hierarchical role selection
Everywhere a role is picked (user Roles tab, group Roles tab, create-role parent select): a grouped/tree select — template families as groups, children indented under parents (depth-aware labels). Implementation: `<select>` with `<optgroup>` per family + indent prefixes (no JS tree dependency; enterprise-clean, keyboard-accessible).

## 4. Group roles

- Group detail page (or groups management surface — wherever groups render today; if no group detail page exists, add a minimal DetailShell group detail with General + Members + Roles tabs) gets a **Roles tab**: assign/remove roles (gate `console_access.manage_role_assignments`), hierarchical picker, activity rows `group_role_assigned/removed`.
- User detail Roles tab: existing direct-role rows + read-only "via group <name>" rows (remove action disabled with tooltip "Managed on the group").
- Cross-tenant: group and role must both resolve in-tenant (resolveTenant* pattern).

## 5. Dependency-groups list toolbar

Same search + quick filters treatment (All / Builtin / Custom) using the list-filter infra. Keep the existing Create button.

## 6. Full-page user create — `/users/new`

Replaces the small form: renders the SAME standardized user layout (DetailShell look) with every schema section's editable fields as OPEN inputs (create mode — no pencil toggling), avatar placeholder, and a sticky Create + Cancel action row. Required: username, email; display_name auto-fills from First+Last when blank (AD behavior). Submit = one POST to /users/new carrying all fields; server creates via IdentityService.Create then applies remaining fields through the same validation path as UpdateFields (shared coercion/editability rules; NonEditableFieldKeys refused). On validation error re-render with the entered values + error banner. After create → redirect to the real user detail. Roles/Groups/Policies tabs do not render in create mode. Gate `identity.create`. The old UserFormPage remains ONLY for edit (`/users/:id/edit`) until a later pass removes it.

## Out of scope (explicit)

Permission-level inheritance visualization beyond origin badges; role assignment to OUs/sites; approval workflows; the symmetric "raise-your-own-grants" guard (recorded as next-phase hardening); removing /users/:id/edit.

## Testing

Resolver: chain merge, override wins, depth cap, cycle rejection (set + write paths), parentless passthrough. EffectiveGrants v2: direct∪group union, group-role changes recompute cache (member gains/loses admin). Handlers: create-role w/ parent, parent-change cycle 400, override-diff save (equal values dropped), clone-in-detail, group role assign/remove + tenant isolation + RBAC, via-group rows read-only, full-page create (valid, missing username 400 re-render, non-editable key refused). UI render tests: origin badges, tree picker optgroups, toolbars filter attributes. E2E: create role w/ parent → edit override → assign to group → member's access changes → user created via full-page form edits own info.

## Invariants & constraints (unchanged)

No new deps; append-only migration 005 (ASCII comments, no PRAGMA); `-buildvcs=false`; t.TempDir() tests; never touch `pluris.db*`; `make gen`/`sqlc generate`; DetailShell + INV-L + resolveTenant* 404 pattern; owner owns git; suite green per task.
