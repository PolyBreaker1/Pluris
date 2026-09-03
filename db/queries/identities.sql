-- Identity management queries - matches the schema in
-- db/schema/002_identity_ad_compat.sql. Identities are both the
-- end-user directory (owner of assets) and the accounts that log into
-- the console (role-gated).

-- name: CreateIdentity :one
INSERT INTO identities (
    tenant_id, site_id, username, user_principal_name, email, display_name,
    given_name, surname, title, department, company, employee_id,
    employee_type, manager_id, phone_office, phone_mobile, role
) VALUES (
    @tenant_id, @site_id, @username, @user_principal_name, @email, @display_name,
    @given_name, @surname, @title, @department, @company, @employee_id,
    @employee_type, @manager_id, @phone_office, @phone_mobile, @role
) RETURNING *;

-- name: GetIdentity :one
SELECT * FROM identities WHERE id = @id AND deleted_at IS NULL LIMIT 1;

-- name: GetIdentityIncludingDeleted :one
SELECT * FROM identities WHERE id = @id LIMIT 1;

-- name: GetIdentityByEmail :one
SELECT * FROM identities
WHERE tenant_id = @tenant_id AND email = @email AND deleted_at IS NULL
LIMIT 1;

-- Used at login: the user only enters an email, not a tenant, so this
-- resolves across all tenants. Returns 0, 1, or (in the rare case two
-- tenants share an email) more than 1 row - callers MUST check the
-- length and treat >1 as an ambiguous-account error, never just take
-- the first result.
-- name: GetIdentityByEmailGlobal :many
SELECT * FROM identities WHERE email = @email AND deleted_at IS NULL;

-- name: ListIdentitiesByTenant :many
SELECT * FROM identities
WHERE tenant_id = @tenant_id
  AND deleted_at IS NULL
ORDER BY display_name
LIMIT @limit OFFSET @offset;

-- name: ListDeletedIdentitiesByTenant :many
SELECT * FROM identities
WHERE tenant_id = @tenant_id AND deleted_at IS NOT NULL
ORDER BY display_name
LIMIT @limit OFFSET @offset;

-- Same rows as ListIdentitiesByTenant plus the resolved site name, for the
-- Users list page (INV-L). A separate query rather than widening the query
-- above: ListIdentitiesByTenant is also called directly by group_rules.go
-- and targets.go, which do not need the join and should not pay for it.
-- name: ListIdentitiesByTenantWithSite :many
SELECT sqlc.embed(identities), s.name AS site_name
FROM identities
LEFT JOIN sites s ON identities.site_id = s.id
WHERE identities.tenant_id = @tenant_id
  AND identities.deleted_at IS NULL
ORDER BY identities.display_name
LIMIT @limit OFFSET @offset;

-- name: ListDeletedIdentitiesByTenantWithSite :many
SELECT sqlc.embed(identities), s.name AS site_name
FROM identities
LEFT JOIN sites s ON identities.site_id = s.id
WHERE identities.tenant_id = @tenant_id
  AND identities.deleted_at IS NOT NULL
ORDER BY identities.display_name
LIMIT @limit OFFSET @offset;

-- name: CountIdentitiesByTenant :one
SELECT COUNT(*) FROM identities WHERE tenant_id = @tenant_id AND deleted_at IS NULL;

-- Used by the setup-gate middleware: "does any identity exist anywhere?"
-- name: CountIdentitiesGlobal :one
SELECT COUNT(*) FROM identities WHERE deleted_at IS NULL;

-- name: SearchIdentities :many
SELECT * FROM identities
WHERE tenant_id = @tenant_id
  AND deleted_at IS NULL
  AND (display_name LIKE '%' || @search || '%'
       OR email LIKE '%' || @search || '%'
       OR username LIKE '%' || @search || '%'
       OR department LIKE '%' || @search || '%')
ORDER BY display_name
LIMIT @limit;

-- name: UpdateIdentity :one
-- Writes every field the Users UI (detail-page editor + inline-edit field
-- API, see console/handlers/field_api.go) can set on an identity. Kept as
-- one wide UPDATE (rather than narrow SetIdentityX statements per field)
-- so IdentityService.Update stays the single write path callers use --
-- ID/AccountEnabled/AccountLocked/Role/password fields have their own
-- narrower statements below for flows that must not clobber unrelated
-- columns.
UPDATE identities SET
    display_name = @display_name,
    given_name = @given_name,
    surname = @surname,
    initials = @initials,
    email = @email,
    title = @title,
    department = @department,
    company = @company,
    employee_id = @employee_id,
    employee_type = @employee_type,
    manager_id = @manager_id,
    phone_office = @phone_office,
    phone_mobile = @phone_mobile,
    phone_home = @phone_home,
    fax = @fax,
    office = @office,
    street_address = @street_address,
    city = @city,
    state = @state,
    postal_code = @postal_code,
    country = @country,
    country_code = @country_code,
    home_directory = @home_directory,
    home_drive = @home_drive,
    profile_path = @profile_path,
    logon_script = @logon_script,
    account_enabled = @account_enabled,
    account_locked = @account_locked,
    account_expires_at = @account_expires_at,
    password_never_expires = @password_never_expires,
    must_change_password = @must_change_password,
    locale = @locale,
    timezone = @timezone,
    description = @description,
    notes = @notes,
    site_id = @site_id,
    avatar_url = @avatar_url,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING *;

-- name: UpdateIdentityRole :exec
UPDATE identities SET role = @role, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetIdentityEnabled :exec
UPDATE identities SET account_enabled = @account_enabled, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetIdentityLocked :exec
UPDATE identities SET account_locked = @account_locked, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetIdentityPasswordHash :exec
UPDATE identities SET
    password_hash = @password_hash,
    password_last_set_at = CURRENT_TIMESTAMP,
    must_change_password = FALSE,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: RecordLoginSuccess :exec
UPDATE identities SET
    last_logon_at = CURRENT_TIMESTAMP,
    logon_count = logon_count + 1,
    bad_password_count = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: RecordLoginFailure :exec
UPDATE identities SET
    bad_password_count = bad_password_count + 1,
    last_bad_password_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: LockIdentityIfThresholdExceeded :exec
UPDATE identities SET account_locked = TRUE
WHERE id = @id AND bad_password_count >= @threshold;

-- name: DeleteIdentity :exec
DELETE FROM identities WHERE id = @id;

-- name: SoftDeleteIdentity :execrows
UPDATE identities
SET deleted_at = CURRENT_TIMESTAMP, deleted_by = @deleted_by
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: RestoreIdentity :execrows
UPDATE identities
SET deleted_at = NULL, deleted_by = NULL
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NOT NULL;

-- name: ListExpiredIdentities :many
SELECT * FROM identities
WHERE deleted_at IS NOT NULL AND deleted_at <= @cutoff
ORDER BY deleted_at, id;
