-- Module installation queries
-- Tracks which policy modules are installed on which assets

-- name: CreateModuleInstallation :one
INSERT INTO module_installations (
    asset_id,
    module_id,
    module_version_pinned,
    state
) VALUES (
    @asset_id,
    @module_id,
    @module_version_pinned,
    @state
) RETURNING *;

-- name: GetModuleInstallation :one
SELECT * FROM module_installations WHERE id = @id LIMIT 1;

-- name: GetInstallationByAssetAndModule :one
SELECT * FROM module_installations 
WHERE asset_id = @asset_id AND module_id = @module_id 
LIMIT 1;

-- name: ListInstallationsByAsset :many
SELECT 
    i.*,
    m.module_urn,
    m.title as module_title
FROM module_installations i
JOIN policy_modules m ON m.id = i.module_id
WHERE i.asset_id = @asset_id
ORDER BY m.title;

-- name: ListInstallationsByModule :many
SELECT 
    i.*,
    a.human_id as asset_human_id,
    json_extract(a.subtype_payload, '$.hostname') as asset_hostname
FROM module_installations i
JOIN assets a ON a.id = i.asset_id
WHERE i.module_id = @module_id
ORDER BY i.state, a.human_id;

-- name: ListInstallationsByState :many
SELECT 
    i.*,
    a.human_id as asset_human_id,
    m.title as module_title
FROM module_installations i
JOIN assets a ON a.id = i.asset_id
JOIN policy_modules m ON m.id = i.module_id
WHERE i.state = @state
ORDER BY i.updated_at DESC
LIMIT @limit;

-- name: ListPendingInstallations :many
SELECT 
    i.*,
    a.human_id as asset_human_id,
    m.module_urn
FROM module_installations i
JOIN assets a ON a.id = i.asset_id
JOIN policy_modules m ON m.id = i.module_id
WHERE i.state = 'pending'
ORDER BY i.created_at
LIMIT @limit;

-- name: ListFailingInstallations :many
SELECT 
    i.*,
    a.human_id as asset_human_id,
    m.title as module_title
FROM module_installations i
JOIN assets a ON a.id = i.asset_id
JOIN policy_modules m ON m.id = i.module_id
WHERE i.state = 'failing'
ORDER BY i.updated_at DESC
LIMIT @limit;

-- name: CountInstallationsByAsset :one
SELECT COUNT(*) FROM module_installations WHERE asset_id = @asset_id;

-- name: CountInstallationsByModule :one
SELECT COUNT(*) FROM module_installations WHERE module_id = @module_id;

-- name: CountInstallationsByState :one
SELECT COUNT(*) FROM module_installations WHERE state = @state;

-- name: UpdateModuleInstallation :one
UPDATE module_installations SET
    module_version_pinned = @module_version_pinned,
    state = @state,
    last_report = @last_report,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING *;

-- name: UpdateInstallationState :exec
UPDATE module_installations SET
    state = @state,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateInstallationApplied :exec
UPDATE module_installations SET
    state = 'applied',
    applied_at = CURRENT_TIMESTAMP,
    last_validated_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: DeleteModuleInstallation :exec
DELETE FROM module_installations WHERE id = @id;

-- name: DeleteInstallationsByAsset :exec
DELETE FROM module_installations WHERE asset_id = @asset_id;

-- name: DeleteInstallationsByModule :exec
DELETE FROM module_installations WHERE module_id = @module_id;

-- ============================================================================
-- Module Installation Dependencies
-- ============================================================================

-- name: CreateInstallationDependency :one
INSERT INTO module_installation_dependencies (
    installation_id,
    depends_on_installation_id
) VALUES (
    @installation_id,
    @depends_on_installation_id
) RETURNING *;

-- name: ListDependenciesForInstallation :many
SELECT 
    d.*,
    i.module_id as depends_on_module_id,
    m.title as depends_on_module_title
FROM module_installation_dependencies d
JOIN module_installations i ON i.id = d.depends_on_installation_id
JOIN policy_modules m ON m.id = i.module_id
WHERE d.installation_id = @installation_id;

-- name: ListDependentsOfInstallation :many
SELECT 
    d.*,
    i.module_id as dependent_module_id,
    m.title as dependent_module_title
FROM module_installation_dependencies d
JOIN module_installations i ON i.id = d.installation_id
JOIN policy_modules m ON m.id = i.module_id
WHERE d.depends_on_installation_id = @installation_id;

-- name: DeleteInstallationDependency :exec
DELETE FROM module_installation_dependencies WHERE id = @id;

-- name: DeleteDependenciesForInstallation :exec
DELETE FROM module_installation_dependencies WHERE installation_id = @installation_id;

-- name: DeleteDependenciesOnInstallation :exec
DELETE FROM module_installation_dependencies WHERE depends_on_installation_id = @installation_id;
