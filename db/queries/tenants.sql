-- Tenant operations

-- name: CreateTenant :one
INSERT INTO tenants (name, slug)
VALUES (@name, @slug) RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = @id LIMIT 1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = @slug LIMIT 1;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY name;

-- name: UpdateTenant :one
UPDATE tenants SET 
    name = @name
WHERE id = @id
RETURNING *;

-- name: DeleteTenant :exec
DELETE FROM tenants WHERE id = @id;

-- name: CountTenants :one
SELECT COUNT(*) FROM tenants;

-- ============================================================================
-- Site operations
-- ============================================================================

-- name: CreateSite :one
INSERT INTO sites (tenant_id, name, slug)
VALUES (@tenant_id, @name, @slug) RETURNING *;

-- name: GetSite :one
SELECT * FROM sites WHERE id = @id LIMIT 1;

-- name: GetSiteBySlug :one
SELECT * FROM sites 
WHERE tenant_id = @tenant_id AND slug = @slug 
LIMIT 1;

-- name: ListSitesByTenant :many
SELECT * FROM sites
WHERE tenant_id = @tenant_id
ORDER BY name;

-- name: UpdateSite :one
UPDATE sites SET 
    name = @name
WHERE id = @id
RETURNING *;

-- name: DeleteSite :exec
DELETE FROM sites WHERE id = @id;

-- name: CountSitesByTenant :one
SELECT COUNT(*) FROM sites WHERE tenant_id = @tenant_id;

-- ============================================================================
-- Group operations
-- ============================================================================

-- name: CreateGroup :one
INSERT INTO groups (tenant_id, site_id, name, slug)
VALUES (@tenant_id, @site_id, @name, @slug) RETURNING *;

-- name: GetGroup :one
SELECT * FROM groups WHERE id = @id LIMIT 1;

-- name: GetGroupBySlug :one
SELECT * FROM groups 
WHERE tenant_id = @tenant_id AND slug = @slug 
LIMIT 1;

-- name: ListGroupsByTenant :many
SELECT * FROM groups
WHERE tenant_id = @tenant_id
ORDER BY name;

-- name: ListGroupsBySite :many
SELECT * FROM groups
WHERE tenant_id = @tenant_id AND site_id = @site_id
ORDER BY name;

-- name: UpdateGroup :one
UPDATE groups SET 
    name = @name, 
    site_id = @site_id
WHERE id = @id
RETURNING *;

-- name: DeleteGroup :exec
DELETE FROM groups WHERE id = @id;

-- name: CountGroupsByTenant :one
SELECT COUNT(*) FROM groups WHERE tenant_id = @tenant_id;

-- name: SearchGroups :many
SELECT * FROM groups
WHERE tenant_id = @tenant_id
  AND name LIKE '%' || @search || '%'
ORDER BY name
LIMIT @limit;

-- ============================================================================
-- Group membership operations
-- ============================================================================

-- name: AddAssetToGroup :exec
INSERT INTO group_memberships (group_id, asset_id)
VALUES (@group_id, @asset_id)
ON CONFLICT DO NOTHING;

-- name: AddIdentityToGroup :exec
INSERT INTO group_memberships (group_id, identity_id)
VALUES (@group_id, @identity_id)
ON CONFLICT DO NOTHING;

-- name: RemoveAssetFromGroup :exec
DELETE FROM group_memberships
WHERE group_id = @group_id AND asset_id = @asset_id;

-- name: RemoveIdentityFromGroup :exec
DELETE FROM group_memberships
WHERE group_id = @group_id AND identity_id = @identity_id;

-- name: ListGroupsForAsset :many
SELECT g.* FROM groups g
INNER JOIN group_memberships gm ON g.id = gm.group_id
WHERE gm.asset_id = @asset_id
ORDER BY g.name;

-- name: ListGroupsForIdentity :many
SELECT g.* FROM groups g
INNER JOIN group_memberships gm ON g.id = gm.group_id
WHERE gm.identity_id = @identity_id
ORDER BY g.name;

-- name: ListAssetsInGroup :many
SELECT a.* FROM assets a
INNER JOIN group_memberships gm ON a.id = gm.asset_id
WHERE gm.group_id = @group_id
ORDER BY a.human_id;

-- name: ListIdentitiesInGroup :many
SELECT i.* FROM identities i
INNER JOIN group_memberships gm ON i.id = gm.identity_id
WHERE gm.group_id = @group_id
ORDER BY i.display_name;

-- name: CountAssetsInGroup :one
SELECT COUNT(*) FROM group_memberships
WHERE group_id = @group_id AND asset_id IS NOT NULL;

-- name: CountIdentitiesInGroup :one
SELECT COUNT(*) FROM group_memberships
WHERE group_id = @group_id AND identity_id IS NOT NULL;

-- name: IsAssetInGroup :one
SELECT EXISTS(
    SELECT 1 FROM group_memberships 
    WHERE group_id = @group_id AND asset_id = @asset_id
) as is_member;

-- name: IsIdentityInGroup :one
SELECT EXISTS(
    SELECT 1 FROM group_memberships 
    WHERE group_id = @group_id AND identity_id = @identity_id
) as is_member;

-- name: ClearGroupMembers :exec
DELETE FROM group_memberships WHERE group_id = @group_id;
