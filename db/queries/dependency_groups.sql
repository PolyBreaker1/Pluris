-- Dependency group queries. Matches the tables in
-- db/schema/004_dependency_groups.sql. Groups are per tenant module
-- applicability filters; conditions AND within a group; module links
-- attach a group to a module in a role (platform or requirement).

-- name: CreateDependencyGroup :one
INSERT INTO dependency_groups (tenant_id, slug, name, description, is_builtin)
VALUES (@tenant_id, @slug, @name, @description, @is_builtin)
RETURNING *;

-- name: GetDependencyGroup :one
SELECT * FROM dependency_groups WHERE id = @id;

-- name: GetDependencyGroupBySlug :one
SELECT * FROM dependency_groups
WHERE tenant_id = @tenant_id AND slug = @slug
LIMIT 1;

-- name: ListDependencyGroupsByTenant :many
SELECT * FROM dependency_groups
WHERE tenant_id = @tenant_id
ORDER BY is_builtin DESC, name;

-- name: UpdateDependencyGroup :exec
UPDATE dependency_groups
SET name = @name, description = @description, updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateGroupMatchMode :exec
UPDATE dependency_groups
SET match_mode = @match_mode, updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: DeleteDependencyGroup :exec
DELETE FROM dependency_groups WHERE id = @id;

-- name: CreateDependencyGroupCondition :one
INSERT INTO dependency_group_conditions (group_id, param_path, operator, value_json, seq, kind, script_source, script_expect)
VALUES (@group_id, @param_path, @operator, @value_json, @seq, @kind, @script_source, @script_expect)
RETURNING *;

-- name: ListConditionsForGroup :many
SELECT * FROM dependency_group_conditions
WHERE group_id = @group_id
ORDER BY seq, id;

-- name: DeleteConditionsForGroup :exec
DELETE FROM dependency_group_conditions WHERE group_id = @group_id;

-- name: DeleteDependencyGroupCondition :exec
DELETE FROM dependency_group_conditions WHERE id = @id AND group_id = @group_id;

-- name: CreateModuleDependencyLink :exec
INSERT OR IGNORE INTO module_dependency_links (tenant_id, module_id, group_id, role)
VALUES (@tenant_id, @module_id, @group_id, @role);

-- name: DeleteModuleDependencyLink :exec
DELETE FROM module_dependency_links
WHERE tenant_id = @tenant_id AND module_id = @module_id AND group_id = @group_id;

-- name: ListLinksForModule :many
SELECT * FROM module_dependency_links
WHERE tenant_id = @tenant_id AND module_id = @module_id
ORDER BY role, group_id;

-- name: ListLinksForGroup :many
SELECT * FROM module_dependency_links
WHERE group_id = @group_id
ORDER BY module_id;

-- name: CountLinksForGroup :one
SELECT COUNT(*) FROM module_dependency_links WHERE group_id = @group_id;

-- name: CountLinksForModule :one
-- Used by the module service's DeleteModule guard: a module referenced
-- by any dependency-group link may not be deleted (see
-- pkg/services/policymodules.go).
SELECT COUNT(*) FROM module_dependency_links WHERE tenant_id = @tenant_id AND module_id = @module_id;
