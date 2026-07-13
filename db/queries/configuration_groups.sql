-- Configuration group queries
-- Configuration groups bundle policy bindings for assignment

-- name: CreateConfigurationGroup :one
INSERT INTO configuration_groups (
    tenant_id,
    name,
    description,
    scope,
    disabled
) VALUES (
    @tenant_id,
    @name,
    @description,
    @scope,
    @disabled
) RETURNING *;

-- name: GetConfigurationGroup :one
SELECT * FROM configuration_groups WHERE id = @id LIMIT 1;

-- name: GetConfigurationGroupByName :one
SELECT * FROM configuration_groups 
WHERE tenant_id = @tenant_id AND name = @name 
LIMIT 1;

-- name: ListConfigurationGroupsByTenant :many
SELECT * FROM configuration_groups 
WHERE tenant_id = @tenant_id 
ORDER BY name
LIMIT @limit OFFSET @offset;

-- name: ListEnabledConfigurationGroups :many
SELECT * FROM configuration_groups 
WHERE tenant_id = @tenant_id AND disabled = FALSE
ORDER BY name;

-- name: CountConfigurationGroupsByTenant :one
SELECT COUNT(*) FROM configuration_groups WHERE tenant_id = @tenant_id;

-- name: UpdateConfigurationGroup :one
UPDATE configuration_groups SET
    name = @name,
    description = @description,
    scope = @scope,
    disabled = @disabled,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING *;

-- name: DeleteConfigurationGroup :exec
DELETE FROM configuration_groups WHERE id = @id;

-- name: SearchConfigurationGroups :many
SELECT * FROM configuration_groups
WHERE tenant_id = @tenant_id
  AND (name LIKE '%' || @search || '%' OR description LIKE '%' || @search || '%')
ORDER BY name
LIMIT @limit;

-- ============================================================================
-- Configuration Group Bindings (which policies are in a group)
-- ============================================================================

-- name: CreateConfigurationGroupBinding :one
INSERT INTO configuration_group_bindings (
    configuration_group_id,
    policy_urn,
    state,
    parameter_values,
    module_id,
    module_version_pinned,
    disabled
) VALUES (
    @configuration_group_id,
    @policy_urn,
    @state,
    @parameter_values,
    @module_id,
    @module_version_pinned,
    @disabled
) RETURNING *;

-- name: GetConfigurationGroupBinding :one
SELECT * FROM configuration_group_bindings WHERE id = @id LIMIT 1;

-- name: GetBindingByGroupAndPolicy :one
SELECT * FROM configuration_group_bindings 
WHERE configuration_group_id = @configuration_group_id AND policy_urn = @policy_urn
LIMIT 1;

-- name: ListBindingsByGroup :many
SELECT b.*, m.title as module_title
FROM configuration_group_bindings b
LEFT JOIN policy_modules m ON m.id = b.module_id
WHERE b.configuration_group_id = @configuration_group_id
ORDER BY b.policy_urn;

-- name: ListBindingsByModule :many
SELECT b.*, g.name as group_name
FROM configuration_group_bindings b
JOIN configuration_groups g ON g.id = b.configuration_group_id
WHERE b.module_id = @module_id;

-- name: CountBindingsByModule :one
-- Used by the module service's DeleteModule guard: a module referenced
-- by any configuration-group binding may not be deleted (see
-- pkg/services/policymodules.go).
SELECT COUNT(*) FROM configuration_group_bindings WHERE module_id = @module_id;

-- name: UpdateConfigurationGroupBinding :one
UPDATE configuration_group_bindings SET
    state = @state,
    parameter_values = @parameter_values,
    module_id = @module_id,
    module_version_pinned = @module_version_pinned,
    disabled = @disabled,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING *;

-- name: DeleteConfigurationGroupBinding :exec
DELETE FROM configuration_group_bindings WHERE id = @id;

-- name: DeleteBindingsByGroup :exec
DELETE FROM configuration_group_bindings WHERE configuration_group_id = @configuration_group_id;

-- ============================================================================
-- Configuration Group Assignments (who gets a group)
-- ============================================================================

-- name: CreateConfigurationGroupAssignment :one
INSERT INTO configuration_group_assignments (
    configuration_group_id,
    target_type,
    target_id,
    priority,
    enforced
) VALUES (
    @configuration_group_id,
    @target_type,
    @target_id,
    @priority,
    @enforced
) RETURNING *;

-- name: GetConfigurationGroupAssignment :one
SELECT * FROM configuration_group_assignments WHERE id = @id LIMIT 1;

-- name: UpdateConfigurationGroupAssignment :one
-- Updates priority/enforced on an existing assignment. target_type/
-- target_id/group are immutable after creation (remove + re-add to
-- retarget); this only covers the fields the detail-page assignments
-- table lets an admin tweak in place.
UPDATE configuration_group_assignments SET
    priority = @priority,
    enforced = @enforced
WHERE id = @id
RETURNING *;

-- name: ListAssignmentsByGroup :many
SELECT * FROM configuration_group_assignments
WHERE configuration_group_id = @configuration_group_id
ORDER BY target_type, target_id;

-- name: ListAssignmentsByTarget :many
SELECT a.*, g.name as group_name, g.scope as group_scope
FROM configuration_group_assignments a
JOIN configuration_groups g ON g.id = a.configuration_group_id
WHERE a.target_type = @target_type AND a.target_id = @target_id
ORDER BY a.priority DESC;

-- name: ListConfigGroupsForAsset :many
SELECT DISTINCT g.*
FROM configuration_groups g
JOIN configuration_group_assignments a ON a.configuration_group_id = g.id
WHERE g.disabled = FALSE
  AND ((a.target_type = 'asset' AND a.target_id = @asset_id)
   OR (a.target_type = 'group' AND a.target_id IN (
       SELECT gm.group_id FROM group_memberships gm WHERE gm.asset_id = @asset_id
   ))
   OR (a.target_type = 'site' AND a.target_id = @site_id)
   OR (a.target_type = 'tenant' AND a.target_id = @tenant_id))
ORDER BY a.priority DESC;

-- name: DeleteConfigurationGroupAssignment :exec
DELETE FROM configuration_group_assignments WHERE id = @id;

-- name: DeleteAssignmentsByGroup :exec
DELETE FROM configuration_group_assignments WHERE configuration_group_id = @configuration_group_id;

-- name: DeleteAssignmentsByTarget :exec
DELETE FROM configuration_group_assignments 
WHERE target_type = @target_type AND target_id = @target_id;

-- name: CountAssignmentsByGroup :one
SELECT COUNT(*) FROM configuration_group_assignments 
WHERE configuration_group_id = @configuration_group_id;
