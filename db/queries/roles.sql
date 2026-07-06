-- Role queries - matches the roles and identity_roles tables in
-- db/schema/003_roles_software_logs.sql. Roles are per tenant RBAC
-- definitions beyond the builtin identities.role slugs; identity_roles
-- links them to identities.

-- name: CreateRole :one
INSERT INTO roles (tenant_id, slug, name, description, is_builtin, template_slug)
VALUES (@tenant_id, @slug, @name, @description, @is_builtin, @template_slug)
RETURNING *;

-- name: GetRoleBySlug :one
SELECT * FROM roles
WHERE tenant_id = @tenant_id AND slug = @slug
LIMIT 1;

-- name: ListRolesByTenant :many
SELECT * FROM roles
WHERE tenant_id = @tenant_id
ORDER BY is_builtin DESC, name;

-- name: AssignRoleToIdentity :exec
INSERT OR IGNORE INTO identity_roles (identity_id, role_id, assigned_by)
VALUES (@identity_id, @role_id, @assigned_by);

-- name: RemoveRoleFromIdentity :exec
DELETE FROM identity_roles
WHERE identity_id = @identity_id AND role_id = @role_id;

-- name: ListRolesForIdentity :many
SELECT r.* FROM roles r
JOIN identity_roles ir ON ir.role_id = r.id
WHERE ir.identity_id = @identity_id
ORDER BY r.name;

-- name: GetRole :one
SELECT * FROM roles WHERE id = @id;
