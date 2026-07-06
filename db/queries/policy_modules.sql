-- Policy module queries
-- Policy modules are versioned, signed policy definitions

-- name: CreatePolicyModule :one
INSERT INTO policy_modules (
    module_urn,
    tenant_id,
    title,
    description,
    is_bundled
) VALUES (
    @module_urn,
    @tenant_id,
    @title,
    @description,
    @is_bundled
) RETURNING *;

-- name: GetPolicyModule :one
SELECT * FROM policy_modules WHERE id = @id LIMIT 1;

-- name: GetPolicyModuleByURN :one
SELECT * FROM policy_modules 
WHERE module_urn = @module_urn 
LIMIT 1;

-- name: ListPolicyModulesByTenant :many
SELECT * FROM policy_modules 
WHERE tenant_id = @tenant_id 
ORDER BY title
LIMIT @limit OFFSET @offset;

-- name: ListBundledModules :many
SELECT * FROM policy_modules 
WHERE is_bundled = TRUE
ORDER BY title;

-- name: CountPolicyModulesByTenant :one
SELECT COUNT(*) FROM policy_modules WHERE tenant_id = @tenant_id;

-- name: UpdatePolicyModule :one
UPDATE policy_modules SET
    title = @title,
    description = @description
WHERE id = @id
RETURNING *;

-- name: DeletePolicyModule :exec
DELETE FROM policy_modules WHERE id = @id;

-- name: SearchPolicyModules :many
SELECT * FROM policy_modules
WHERE (tenant_id = @tenant_id OR is_bundled = TRUE)
  AND (title LIKE '%' || @search || '%' OR module_urn LIKE '%' || @search || '%')
ORDER BY title
LIMIT @limit;

-- ============================================================================
-- Policy Module Versions
-- ============================================================================

-- name: CreatePolicyModuleVersion :one
INSERT INTO policy_module_versions (
    module_id,
    version,
    state,
    manifest_yaml,
    target_os,
    scope,
    runtime,
    satisfies,
    parameters_schema,
    depends_on,
    conflicts,
    enforce_script,
    validate_script,
    rollback_script
) VALUES (
    @module_id,
    @version,
    @state,
    @manifest_yaml,
    @target_os,
    @scope,
    @runtime,
    @satisfies,
    @parameters_schema,
    @depends_on,
    @conflicts,
    @enforce_script,
    @validate_script,
    @rollback_script
) RETURNING *;

-- name: GetPolicyModuleVersion :one
SELECT * FROM policy_module_versions WHERE id = @id LIMIT 1;

-- name: GetPolicyModuleVersionByNumber :one
SELECT * FROM policy_module_versions 
WHERE module_id = @module_id AND version = @version 
LIMIT 1;

-- name: GetLatestPublishedVersion :one
SELECT * FROM policy_module_versions 
WHERE module_id = @module_id AND state = 'published'
ORDER BY published_at DESC 
LIMIT 1;

-- name: ListVersionsByModule :many
SELECT v.*, i.display_name as publisher_name
FROM policy_module_versions v
LEFT JOIN identities i ON i.id = v.published_by
WHERE v.module_id = @module_id
ORDER BY v.created_at DESC;

-- name: CountVersionsByModule :one
SELECT COUNT(*) FROM policy_module_versions WHERE module_id = @module_id;

-- name: PublishModuleVersion :exec
UPDATE policy_module_versions SET
    state = 'published',
    published_at = CURRENT_TIMESTAMP,
    published_by = @published_by
WHERE id = @id;

-- name: RevokeModuleVersion :exec
UPDATE policy_module_versions SET
    state = 'revoked'
WHERE id = @id;

-- name: DeletePolicyModuleVersion :exec
DELETE FROM policy_module_versions WHERE id = @id;

-- name: GetModuleWithLatestVersion :one
SELECT 
    m.*,
    v.id as latest_version_id,
    v.version as latest_version,
    v.published_at as latest_published_at
FROM policy_modules m
LEFT JOIN policy_module_versions v ON v.module_id = m.id AND v.state = 'published'
WHERE m.id = @id
ORDER BY v.published_at DESC
LIMIT 1;
