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
SELECT * FROM identities WHERE id = @id LIMIT 1;

-- name: GetIdentityByEmail :one
SELECT * FROM identities
WHERE tenant_id = @tenant_id AND email = @email
LIMIT 1;

-- Used at login: the user only enters an email, not a tenant, so this
-- resolves across all tenants. Returns 0, 1, or (in the rare case two
-- tenants share an email) more than 1 row - callers MUST check the
-- length and treat >1 as an ambiguous-account error, never just take
-- the first result.
-- name: GetIdentityByEmailGlobal :many
SELECT * FROM identities WHERE email = @email;

-- name: ListIdentitiesByTenant :many
SELECT * FROM identities
WHERE tenant_id = @tenant_id
ORDER BY display_name
LIMIT @limit OFFSET @offset;

-- name: CountIdentitiesByTenant :one
SELECT COUNT(*) FROM identities WHERE tenant_id = @tenant_id;

-- Used by the setup-gate middleware: "does any identity exist anywhere?"
-- name: CountIdentitiesGlobal :one
SELECT COUNT(*) FROM identities;

-- name: SearchIdentities :many
SELECT * FROM identities
WHERE tenant_id = @tenant_id
  AND (display_name LIKE '%' || @search || '%'
       OR email LIKE '%' || @search || '%'
       OR username LIKE '%' || @search || '%'
       OR department LIKE '%' || @search || '%')
ORDER BY display_name
LIMIT @limit;

-- name: UpdateIdentity :one
UPDATE identities SET
    display_name = @display_name,
    given_name = @given_name,
    surname = @surname,
    email = @email,
    title = @title,
    department = @department,
    company = @company,
    employee_id = @employee_id,
    employee_type = @employee_type,
    manager_id = @manager_id,
    phone_office = @phone_office,
    phone_mobile = @phone_mobile,
    site_id = @site_id,
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
