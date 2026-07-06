-- Custom policy queries
-- Custom policies are tenant-specific policy definitions (not from bundled catalog)

-- name: CreateCustomPolicy :one
INSERT INTO custom_policies (
    tenant_id,
    policy_urn,
    name,
    description,
    scope,
    category_path,
    parameters_schema
) VALUES (
    @tenant_id,
    @policy_urn,
    @name,
    @description,
    @scope,
    @category_path,
    @parameters_schema
) RETURNING *;

-- name: GetCustomPolicy :one
SELECT * FROM custom_policies WHERE id = @id LIMIT 1;

-- name: GetCustomPolicyByURN :one
SELECT * FROM custom_policies 
WHERE tenant_id = @tenant_id AND policy_urn = @policy_urn 
LIMIT 1;

-- name: ListCustomPoliciesByTenant :many
SELECT * FROM custom_policies 
WHERE tenant_id = @tenant_id 
ORDER BY category_path, name
LIMIT @limit OFFSET @offset;

-- name: ListCustomPoliciesByCategory :many
SELECT * FROM custom_policies 
WHERE tenant_id = @tenant_id AND category_path LIKE @category_prefix || '%'
ORDER BY name;

-- name: ListCustomPoliciesByScope :many
SELECT * FROM custom_policies 
WHERE tenant_id = @tenant_id AND scope = @scope
ORDER BY category_path, name;

-- name: CountCustomPoliciesByTenant :one
SELECT COUNT(*) FROM custom_policies WHERE tenant_id = @tenant_id;

-- name: UpdateCustomPolicy :one
UPDATE custom_policies SET
    name = @name,
    description = @description,
    scope = @scope,
    category_path = @category_path,
    parameters_schema = @parameters_schema
WHERE id = @id
RETURNING *;

-- name: DeleteCustomPolicy :exec
DELETE FROM custom_policies WHERE id = @id;

-- name: SearchCustomPolicies :many
SELECT * FROM custom_policies
WHERE tenant_id = @tenant_id
  AND (name LIKE '%' || @search || '%' OR policy_urn LIKE '%' || @search || '%')
ORDER BY category_path, name
LIMIT @limit;
