# Identity & assets

**What:** The directory model — identities, assets, groups, sites/tenants — plus the create flows and inline-edit field API that operate on them.

**Related:** [[overview]], [[data-model]], [[parameters]], [[decisions]]

## The shared hierarchy

One hierarchy: **Tenant → Site → Group → (Asset | Identity)**. Every navigator, picker, list, and policy-assignment target reads through this model (ADR-004's R3). `tenants` is the multi-tenant isolation root; `sites` are geographic/network boundaries within a tenant; `groups` collect either assets or identities (never both in one membership row — see [[data-model]]) and can carry roles that members inherit.

## Identities — AD attribute parity

`identities` (migration 002, widened in 003) mirrors Active Directory's attribute set closely enough that an AD-migrating admin recognizes every field. Mapping (Pluris column → AD attribute):

| Pluris field | AD attribute | Notes |
|---|---|---|
| `username` | `sAMAccountName` (approx.) | Unique per tenant; not the login display value. |
| `user_principal_name` | `userPrincipalName` | `user@domain` form, optional. |
| `display_name` | `displayName` | Falls back to "GivenName Surname" or `username` if unset (`Identity.ResolvedDisplayName()`). |
| `given_name` | `givenName` | |
| `surname` | `sn` | |
| `initials` | `initials` | |
| `email` | `mail` | |
| `title`, `department`, `company` | `title`, `department`, `company` | |
| `employee_id`, `employee_type` | `employeeID`, `employeeType` | |
| `manager_id` | `manager` | Self-referencing FK into `identities`. |
| `phone_office`/`phone_mobile`/`phone_home`/`fax` | `telephoneNumber`/`mobile`/`homePhone`/`facsimileTelephoneNumber` | |
| `office`, `street_address`, `city`, `state`, `postal_code`, `country`, `country_code` | `physicalDeliveryOfficeName`, `streetAddress`, `l`, `st`, `postalCode`, `co`, `c` | |
| `home_directory`, `home_drive`, `profile_path`, `logon_script` | `homeDirectory`, `homeDrive`, `profilePath`, `scriptPath` | Windows-familiar profile/script fields. |
| `account_enabled`, `account_locked`, `account_expires_at` | `userAccountControl` bits, `accountExpires` | |
| `password_last_set_at`, `password_never_expires`, `must_change_password` | `pwdLastSet`, `UF_DONT_EXPIRE_PASSWD`, `pwdMustChange` | |
| `last_logon_at`, `logon_count`, `bad_password_count` | `lastLogon`, `logonCount`, `badPwdCount` | System-maintained; never editable via the field API. |

`role` is Pluris-specific (not AD): `super_admin | admin | technician | user` (widened from an original 2-role scheme in migration 003 — see [[decisions]] DEC-012). There is currently no live directory sync — identities are stored natively in the Pluris database, not read from AD/FreeIPA/Kanidm (see [[decisions]] DEC-015).

`catalog/identities.Identity` is the canonical in-memory shape every Users surface reads; `pkg/services.IdentityService` adapts `db.Identity` rows into it. See [[parameters]] for `NonEditableFieldKeys`/`SelfServiceEditableKeys`, which gate what the field-update API may change.

## Assets — 4 subtypes, human-id scheme, owner

`catalog/assets.Asset` is the canonical shape; `Subtype` discriminates `computer | server | printer | desk` (closed-extensible: a new subtype is additive — new constant, new `SubtypePayload` struct, new Assets-page tab, per ADR-005). Every subtype shares common fields (`EnrollmentState`, `LifecycleState`, `Site`, `Groups`, `Labels`, `OwnerIdentity`, `Location`, `Vendor`, warranty/purchase fields) plus a typed `Payload` matching its subtype (`ComputerPayload`, `ServerPayload`, `PrinterPayload`, `DeskPayload` — see [[data-model]]'s JSON-payload-routing note).

**Human-readable ID.** `assets.human_id` is a stable, readable identifier distinct from the row's numeric `id` and its agent-bound `uuid` — format `{prefix}.{tenant_slug}.{site_slug}.{seq:04d}`, e.g. `comp.acme.hq.0001`. Prefixes: `comp` (computer), `srv` (server), `prn` (printer), `desk` (desk). The web-based enroll flow (below) uses a literal `web` site segment (`comp.acme.web.0007`) since the minimal enroll form doesn't collect a site. Detail-page URLs and the field-update API accept either the human_id or the numeric DB id (`AssetService.ResolveDBID` / `resolveTenantAssetForFields` try numeric first, fall back to human_id/UUID lookup).

**Owner.** `OwnerIdentity` (column `owner_identity_id`, `ON DELETE SET NULL`) is the assigned user's identity, optional (empty for shared/pool assets). Setting/clearing owner is a dedicated action (`asset.set_owner` permission, `POST /assets/:subtype/:id/owner`), not a plain field-update — see [[overview]]'s request-flow section for how route-level vs handler-level authorization interact.

## Groups — identity + asset members, roles attachable

A `groups` row can hold members of either kind (asset or identity — enforced exclusive per membership row via a `CHECK`, so a group's overall membership is a mix of both kinds across rows), plus AD-style classification fields `group_category`/`group_scope` (migration 003). `GroupService.ListForAsset`/`ListForIdentity` return direct memberships only today (`Source: "Direct"` — inheritance via site/tenant rules or external sync is a documented future addition, not yet built).

Roles can attach to a group (`group_roles`, migration 005): every identity member inherits the group's roles in addition to any directly-assigned roles, folded into `identities.role`'s denormalized privilege cache by `RoleService.recomputeRoleCache`. See [[decisions]] DEC-014 and the authorization model.

## Sites / tenants

`sites` scope to a tenant (`UNIQUE(tenant_id, slug)`) and are referenced by both assets (`site_id`) and identities (`site_id`), and can constrain a group (`groups.site_id`). `tenants` is the top-level isolation boundary — every table with tenant-scoped data cascades from it, and `super_admin` sessions are the only ones that can switch active tenant mid-session (`POST /tenant-switch`, `identity_sessions.active_tenant_id`).

## Create flows

### Full-page user create

`GET /users/new` → `UserNewShow` renders `templates.UserCreatePage` with an empty `identities.Identity{}`. `POST /users/new` → `UserCreateSubmit`:

1. Requires `identity.create`.
2. Reads every text-valued schema field the form can submit (`identityFromCreateForm`) — this is the single source both the validation-failure re-render (which must echo back everything the admin typed) and the create call share.
3. Calls `IdentityService.Create` with only the "core" fields `CreateIdentityParams` actually accepts on insert (`userCreateCoreKeys`: username, email, display_name, given_name, surname, user_principal_name, title, department, company, employee_id, employee_type, phone_office, phone_mobile). `display_name` auto-fills from given+surname when blank, matching AD behavior.
4. Every other submitted, editable field is then applied through a second pass of `IdentityService.UpdateFields` — the *same* validation/coercion path the detail page's inline editor uses, so create never duplicates field-level logic.
5. A per-section `UpdateFields` failure after a successful `Create` does not roll back the new account; the handler redirects to the new user's detail page with a `warn` query param naming which fields were dropped.

### Minimal asset enroll, then inline-edit

`GET /assets/:subtype/new` → `AssetNewShow` renders a minimal `AssetNewPage` — no wizard. `POST /assets/:subtype/new` → `AssetCreateSubmit` creates a bare asset row (uuid, human_id, subtype, empty/default payload) and redirects straight to the real detail page, where every other field is filled in via the same inline per-section editor every other asset uses. This is a deliberate design choice ("fast create that lands on the real working detail page") documented in the handler's own comment, not a placeholder pending a fuller wizard.

## Inline editing + field API contract

The detail pages for both users and assets use one shared JS mechanism (`web/static/detail.js`) and one shared backend contract (`console/handlers/field_api.go`).

**Endpoints:**
- `POST /api/users/:id/fields`
- `POST /api/assets/:subtype/:id/fields`
- `POST /api/users/:id/avatar` (multipart upload, separate from the field API)

**Request/response shape.** Body: `{"section": "<section-key>", "fields": {"<param_key>": "<string value>", ...}}`. Response (200): `{"updated": ["<param_key>", ...]}` (sorted). Errors: 404 (`ErrFieldNotFound` — missing or cross-tenant) or 400 (`ErrFieldValidation` — unknown section/key, non-editable key, or coercion failure, with the offending key named in the body).

**Section-save, dirty-only.** Each detail-page section is edited as a unit: clicking "edit" on a section switches its rendered values to inputs, each stamped with `data-original="<current value>"`. Saving (`saveSectionEdit`) diffs every input's live value against its `data-original` and includes only the changed keys in the `fields` payload — the API never receives (and never needs to validate) untouched fields in that section.

**Authorization.** Route-level middleware only requires an authenticated session (`/api/*` has no entry in the route-permission table — see [[overview]]). The actual gate is the handler's own `requirePermissionScoped("identity.update"|"asset.update", targetID/ownerID)` call, which resolves the caller's scope (`none|own|all`) for that action and compares against the target's owner. When the caller's *own* identity-update scope is `own` (self-service, not the super_admin bypass), every submitted key must additionally appear in `identities.SelfServiceEditableKeys` — enforced in the handler before calling the service, so a self-service user cannot use this endpoint to touch role/employment/security fields on themselves even though they can view them.

**Avatar upload.** `POST /api/users/:id/avatar`: multipart form field `avatar`, ≤2MB, content-type sniffed off the actual bytes (never the client-supplied header) and restricted to `image/png`/`image/jpeg`/`image/webp`. Same authorization shape as the field API (`requirePermissionScoped(identity.update, ...)`). Files are served back from `/avatars/<id>.<ext>` — deliberately left behind the full auth chain (no static-path exemption) since avatars are private-by-default and same-origin `<img>` requests already carry the session cookie.

Both mutating endpoints append a best-effort `activity_log` row (`InsertActivity`) recording which section was touched.
