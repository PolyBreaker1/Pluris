# Users / Identity System + Real Login — Design Spec

**Date**: 2026-07-04
**Status**: approved, pending implementation plan

---

## 1. Problem & context

Pluris has a locked sidebar item "Users" that today renders an empty-state stub
(`web/templates/pages.templ` → `UsersPage()`). The database has only a minimal
`identities` table (`db/schema/001_initial.sql`: tenant_id, email, name,
password_hash, role) with a hardcoded `role` CHECK constraint, and no
authentication is wired anywhere — `console/handlers/handlers.go:74` hardcodes
`tenantID := int64(1)` with `// TODO: Get tenant ID from session/auth`.

A previous session (lost — the machine it was on died before the commit was
pushed) had drafted a much more ambitious identity schema,
`db/schema/002_identity_ad_compat.sql`: ~40 AD-style fields, an Organizational
Unit tree, AD-sync scaffolding, sessions, and an audit log. **This migration is
orphaned**: `pkg/database/database.go`'s `migrate()` only lists
`001_initial.sql`, so 002 has never been applied, and the sqlc queries in
`db/queries/identities.sql` were never updated to match it (they still target
the old 3-column shape).

This spec designs the real Users/Identity feature: a rich, AD-familiar
identity directory (matching Pluris's "easy switching from Windows" goal),
wired into the existing `catalog/params` registry the same way Assets are,
plus the app's first real authentication system and its first write-capable
(POST) code paths.

## 2. Goals

1. Replace the orphaned/broken identity schema with one that's actually
   migrated and matches the query layer.
2. Rich AD-style parameter set for identities, exposed through the same
   single-source-of-truth `catalog/params` registry Assets already use — one
   universal list/filter/detail system, not a parallel one.
3. Real local authentication: password login, server-side sessions, RBAC
   enforcing the locked role matrix already specified in
   `docs/UX_INVARIANTS.md`.
4. First-run setup wizard — no default/shared credentials ever exist.
5. A super_admin multi-tenant switcher (session-scoped active tenant),
   replacing the hardcoded `tenantID := int64(1)`.
6. Asset ↔ owner identity pairing, using the already-present but unused
   `owner_identity_id` FK and `ListAssetsByOwner` query.

## 3. Non-goals (explicit scope boundary)

- Groups/OU assignment for identities — `group_memberships` supports
  `identity_id` at the schema level, but there are no Groups queries or UI at
  all yet. Building a picker for a feature that doesn't functionally exist
  would be premature; revisit once Groups itself is built.
- AD / Kanidm / FreeIPA live directory sync — explicitly Phase 2 per ADR-009.
  No sync engine, no `ad_sync_configs`, no `source`/`source_guid` columns this
  pass.
- Custom roles beyond the locked three (`super_admin`, `admin`,
  `user_self_service`) — Phase 2+ per `UX_INVARIANTS.md`.
- Cross-tenant data merging beyond the session-scoped switcher (no shared
  cross-tenant reporting, no tenant hierarchy).
- SSO / OIDC / passwordless — local email+password only.

## 4. Data model

Replace `db/schema/002_identity_ad_compat.sql` in place (it has never been
applied to any real database, so this is not a breaking migration) with a
trimmed version:

### `identities`

Core: `id`, `tenant_id` (FK tenants), `site_id` (FK sites, nullable — mirrors
Asset hierarchy), `username`, `user_principal_name`, `email`.

Display: `display_name`, `given_name`, `surname`, `initials`.

Contact: `phone_office`, `phone_mobile`, `phone_home`, `fax`.

Organization: `title`, `department`, `company`, `employee_id`,
`employee_type`, `manager_id` (self-FK).

Location: `office`, `street_address`, `city`, `state`, `postal_code`,
`country`, `country_code`.

Windows-familiar profile fields: `home_directory`, `home_drive`,
`profile_path`, `logon_script`.

Account/security: `account_enabled` (bool), `account_locked` (bool),
`account_expires_at`, `password_hash` (Argon2id), `password_last_set_at`,
`password_never_expires`, `must_change_password`, `last_logon_at`,
`logon_count`, `bad_password_count`, `last_bad_password_at`.

Pluris-specific: `role` (`super_admin|admin|user_self_service` CHECK),
`avatar_url`, `locale`, `timezone`.

Metadata: `description`, `notes`, `created_at`, `updated_at`.

Dropped from the orphaned draft: `organizational_units` table entirely,
`ad_sync_configs` table entirely, and on `identities`/`groups`: `ou_id`,
`source`, `source_guid`, `source_sid`, `source_dn`, `last_synced_at`,
`sync_errors`, `custom_attributes`.

### `identity_sessions`

`id`, `identity_id` (FK), `token_hash` (SHA-256 of the raw session token — the
raw token is never persisted), `active_tenant_id` (FK tenants, nullable —
holds the super_admin's currently-switched tenant), `ip_address`,
`user_agent`, `created_at`, `expires_at`, `revoked_at`.

### `identity_audit_log`

`id`, `tenant_id`, `identity_id` (nullable — login failures may not resolve to
a known identity), `event_type` (`login_success|login_failure|logout|
password_change|account_locked|created|updated`), `ip_address`, `detail`
(free text), `created_at`.

`pkg/database/database.go`'s `migrate()` gets `002_identity_ad_compat.sql`
added to its migration list.

## 5. Query layer

Rewrite `db/queries/identities.sql` to match the new schema: Create, Get (by
id/email/username), List (by tenant, paginated), Search, Update, UpdateRole,
UpdatePassword, SetLocked/Enabled, Delete. Add `db/queries/sessions.sql`
(Create, GetByTokenHash, Revoke, DeleteExpired) and
`db/queries/identity_audit.sql` (Insert, ListByTenant). Regenerate via
`sqlc generate`.

## 6. Parameter registry (`catalog/params`)

Add a new `identity` `SubtypeSchema` to `catalog/params/schemas.go` (parallel
to `SchemaComputer` etc.), with sections: Account (username, UPN, email, role,
enabled/locked), Organization (title, department, company, manager), Contact
(phones, fax), Location (office, address fields), Security (last logon,
password state — display-only, not editable via the generic field editor).
Add the corresponding `ParamDef`s to `catalog/params/definitions.go`. This
reuses the exact mechanism already documented as the project's single source
of truth for columns/filters/detail display — no parallel field system.

## 7. Service layer

New `catalog/identities` package (types: `Identity` struct + `Role` enum,
mirroring `catalog/assets`). New `pkg/services/IdentityService`: `List`,
`Get(id)`, `GetByEmail`, `Create`, `Update`, `Delete`, `Search`,
`SetOwnerOnAsset` / `ClearOwnerOnAsset` (wraps the existing but currently
unused `owner_identity_id` column and `ListAssetsByOwner` query),
`ListAssignedAssets(identityID)`.

## 8. Authentication (`pkg/auth`, new package)

- Password hashing: Argon2id via `golang.org/x/crypto/argon2` (currently an
  indirect dependency — promote to direct in `go.mod`).
- Sessions: on login, generate a random 32-byte token, store
  `SHA-256(token)` in `identity_sessions`, set the raw token in a
  `Secure; HttpOnly; SameSite=Lax` cookie. Every request, middleware looks up
  the hash, checks `expires_at`/`revoked_at`, and loads the identity + active
  tenant into Echo's request context.
- Middleware chain (new, added to `console/server/server.go`):
  1. **Setup-gate**: if zero identities exist anywhere in the DB, redirect
     all routes except `/setup` to `/setup`.
  2. **Auth**: resolve session cookie → identity; redirect to `/login` if
     absent/expired, except for `/login`, `/setup`, `/static`, `/healthz`.
  3. **RBAC**: a Go map mirroring the locked permission matrix in
     `docs/UX_INVARIANTS.md` (§ role permission matrix), keyed by route
     prefix → required min role. 403 (own page, not a raw HTTP error) on
     violation. Row-level self-scoping (`user_self_service` limited to their
     own identity/assets) applied inside the service layer via the session
     identity, not the route middleware.
- CSRF: Echo's built-in CSRF middleware, applied globally to all POST routes
  — this feature introduces the app's first mutating requests, so this is
  the right point to add it once, for every write path that follows.
- Failed logins increment `bad_password_count` / write an audit row; on the
  **10th** consecutive failure, `account_locked` is set (locked account =
  explicit unlock by an admin via the Users editor, no auto-expiry in v1).
  `bad_password_count` resets to 0 on a successful login.
- Sessions expire **30 days** after creation (fixed, not sliding) and are
  looked up by `expires_at > now()`; login always issues a fresh session
  rather than extending an old one.
- The session cookie's `Secure` flag is set based on whether the incoming
  request was TLS (`c.Request().TLS != nil` or `X-Forwarded-Proto` behind a
  proxy) — plain-HTTP local dev (e.g. `http://localhost:8081`) still gets a
  working cookie; anything behind Tailscale/real TLS gets `Secure`.

## 9. First-run setup wizard

New route `/setup` (GET shows form, POST creates). Gated by the setup-gate
middleware above. Creates the first `tenant` row and the first `identities`
row as `super_admin`, then redirects to `/login`. No seeded/default account
ever exists in code.

## 10. Multi-tenant switcher

Only rendered for `role = super_admin`. A dropdown in the top nav listing all
tenants; selecting one POSTs to `/tenant-switch`, which sets
`identity_sessions.active_tenant_id` for the current session. All handlers
currently reading a hardcoded tenant id (starting with
`handlers.go:74`) switch to reading the resolved tenant from request context
(`active_tenant_id` if set, else the identity's own `tenant_id`). admin and
user_self_service never see the switcher; their effective tenant is always
their own `tenant_id`.

## 11. UI

- `/login` — email + password form, "invalid credentials" generic error
  (no user enumeration).
- `/setup` — first-run wizard (org name → creates tenant; admin
  name/email/password → creates super_admin).
- `/users` — rebuilt on the existing `web/lists` engine (same visual/filter
  pattern as `/assets/computers`), driven by the new `identity`
  `SubtypeSchema`. Replaces the current `EmptyState` stub.
- `/users/:id` — detail page (mirrors `AssetDetailPage`), plus an "Assigned
  assets" panel listing assets where `owner_identity_id = this identity`.
- Asset detail/editor gains an owner picker (assign/clear
  `owner_identity_id`), using the same hierarchical-picker component Assets
  already use for other link-type parameters.
- Top nav: user menu (current identity, logout) + tenant switcher
  (super_admin only).
- This is the first **write-capable** editor in the app (create/edit
  identity forms with CSRF tokens) — the form-handling pattern established
  here (standard HTML POST, not a JS SPA) is expected to be the template the
  Asset editor and others adopt later.

## 12. Testing

- `pkg/auth`: unit tests for hash/verify round-trip, session creation,
  expiry, revocation, failed-login counting/lockout.
- `pkg/services/IdentityService`: CRUD + search + owner-pairing tests
  (matching the existing `pkg/database/database_test.go` /
  `pkg/services` test conventions).
- `console/server`: extend `server_test.go` with the setup-gate redirect,
  login-required redirect, and one RBAC-denial case — following the existing
  mount-point-test convention (`TestAssetCanonicalEditorMountedOnEverySubtypeRoute`
  is the precedent to follow).
- Manual verification via the running dev server: fresh DB → forced to
  `/setup` → create admin → login → `/users` list renders → create a second
  user → assign as an asset's owner → confirm it shows on both sides.

## 13. Rollout note

Because this introduces mandatory login, the existing fully-open dev
experience changes: after this lands, a fresh `pluris.db` will force the
setup wizard before anything else is reachable. This is intentional per the
approved design (no default credentials).
