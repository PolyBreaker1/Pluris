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

-- name: UpdateRolePermissions :exec
UPDATE roles SET permissions = @permissions, updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: ListIdentitiesForRole :many
-- Identities assigned to a given role, for the role detail page.
SELECT i.id, i.username, i.display_name, i.email
FROM identities i
JOIN identity_roles ir ON ir.identity_id = i.id
WHERE ir.role_id = @role_id AND i.deleted_at IS NULL
ORDER BY i.display_name;

-- name: DeleteRole :exec
-- Deletes a custom role. Callers must guard against deleting builtin
-- roles and roles that still have members (Task 6, Pluris Policy delete).
DELETE FROM roles WHERE id = @id;

-- name: UpdateRoleSettings :exec
-- Updates a custom role's name/description (Task 6 Settings tab rename).
-- Callers must guard builtin roles -- this query has no such check.
UPDATE roles SET name = @name, description = @description, updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateRoleParent :exec
-- Sets (or clears, if parent_role_id is NULL) a role's parent for
-- inheritance. Callers must guard cycles and max depth (service layer).
UPDATE roles SET parent_role_id = @parent_role_id, updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: ListRoleChildren :many
-- Direct children of a role in the inheritance chain.
SELECT * FROM roles WHERE parent_role_id = @id ORDER BY name;

-- name: AssignRoleToGroup :exec
INSERT OR IGNORE INTO group_roles (group_id, role_id, assigned_by)
VALUES (@group_id, @role_id, @assigned_by);

-- name: RemoveRoleFromGroup :exec
DELETE FROM group_roles
WHERE group_id = @group_id AND role_id = @role_id;

-- name: ListRolesForGroup :many
-- Roles assigned directly to a group, for the group detail Roles tab.
SELECT r.* FROM roles r
JOIN group_roles gr ON gr.role_id = r.id
WHERE gr.group_id = @group_id
ORDER BY r.name;

-- name: ListRolesForGroupDetail :many
-- Same as ListRolesForGroup but carries the assignment time (Task 7
-- group detail Roles tab "Assigned" column). Mirrors
-- ListRolesForIdentityDetail's relationship to ListRolesForIdentity.
SELECT r.*, gr.assigned_at FROM roles r
JOIN group_roles gr ON gr.role_id = r.id
WHERE gr.group_id = @group_id
ORDER BY r.name;

-- name: ListGroupsForRole :many
-- Groups holding a given role, for the role detail Members tab.
SELECT g.id, g.name FROM groups g
JOIN group_roles gr ON gr.group_id = g.id
WHERE gr.role_id = @role_id AND g.deleted_at IS NULL
ORDER BY g.name;

-- name: ListGroupRolesForIdentity :many
-- Roles an identity inherits via group membership (distinct across
-- however many groups grant the same role).
SELECT DISTINCT r.* FROM roles r
JOIN group_roles gr ON gr.role_id = r.id
JOIN group_memberships gm ON gm.group_id = gr.group_id
JOIN groups g ON g.id = gr.group_id
WHERE gm.identity_id = @identity_id AND g.deleted_at IS NULL
ORDER BY r.name;

-- name: ListGroupRolesForIdentityDetail :many
-- Roles an identity inherits via group membership, WITH the group each
-- one comes from (Task 7 user detail Roles tab "via <group>" rows). One
-- row per (role, group) pair -- an identity in two groups that both
-- grant the same role gets two rows, one per group, unlike the
-- DISTINCT-collapsed ListGroupRolesForIdentity above.
SELECT r.*, g.id AS group_id, g.name AS group_name
FROM roles r
JOIN group_roles gr ON gr.role_id = r.id
JOIN group_memberships gm ON gm.group_id = gr.group_id
JOIN groups g ON g.id = gr.group_id
WHERE gm.identity_id = @identity_id AND g.deleted_at IS NULL
ORDER BY r.name, g.name;
