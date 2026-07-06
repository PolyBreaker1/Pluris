-- Asset CRUD operations

-- name: CreateAsset :one
INSERT INTO assets (
    uuid, tenant_id, site_id, subtype, subtype_payload,
    enrollment_state, agent_version, labels, human_id
) VALUES (
    @uuid, @tenant_id, @site_id, @subtype, @subtype_payload,
    @enrollment_state, @agent_version, @labels, @human_id
) RETURNING *;

-- name: GetAsset :one
SELECT * FROM assets
WHERE id = @id LIMIT 1;

-- name: GetAssetByUUID :one
SELECT * FROM assets
WHERE uuid = @uuid LIMIT 1;

-- name: GetAssetByHumanID :one
SELECT 
    a.*,
    t.slug as tenant_slug,
    s.name as site_name,
    i.display_name as owner_name,
    m.display_name as managed_by_name
FROM assets a
LEFT JOIN tenants t ON a.tenant_id = t.id
LEFT JOIN sites s ON a.site_id = s.id
LEFT JOIN identities i ON a.owner_identity_id = i.id
LEFT JOIN identities m ON a.managed_by_identity_id = m.id
WHERE a.human_id = @human_id
LIMIT 1;

-- name: GetAssetWithRelations :one
SELECT 
    a.*,
    t.slug as tenant_slug,
    t.name as tenant_name,
    s.name as site_name,
    s.slug as site_slug,
    i.display_name as owner_name,
    i.email as owner_email,
    m.display_name as managed_by_name
FROM assets a
LEFT JOIN tenants t ON a.tenant_id = t.id
LEFT JOIN sites s ON a.site_id = s.id
LEFT JOIN identities i ON a.owner_identity_id = i.id
LEFT JOIN identities m ON a.managed_by_identity_id = m.id
WHERE a.id = @id
LIMIT 1;

-- name: ListAssets :many
SELECT * FROM assets
WHERE tenant_id = @tenant_id
ORDER BY last_seen_at DESC
LIMIT @limit OFFSET @offset;

-- name: ListAssetsBySubtype :many
SELECT 
    a.*,
    t.slug as tenant_slug,
    s.name as site_name,
    i.display_name as owner_name,
    m.display_name as managed_by_name
FROM assets a
LEFT JOIN tenants t ON a.tenant_id = t.id
LEFT JOIN sites s ON a.site_id = s.id
LEFT JOIN identities i ON a.owner_identity_id = i.id
LEFT JOIN identities m ON a.managed_by_identity_id = m.id
WHERE a.tenant_id = @tenant_id AND a.subtype = @subtype
ORDER BY a.last_seen_at DESC
LIMIT @limit OFFSET @offset;

-- name: ListAssetsBySite :many
SELECT * FROM assets
WHERE tenant_id = @tenant_id AND site_id = @site_id
ORDER BY last_seen_at DESC
LIMIT @limit OFFSET @offset;

-- name: ListAssetsByOwner :many
SELECT * FROM assets
WHERE tenant_id = @tenant_id AND owner_identity_id = @owner_id
ORDER BY last_seen_at DESC;

-- name: UpdateAsset :one
UPDATE assets
SET site_id = @site_id,
    owner_identity_id = @owner_id,
    subtype_payload = @subtype_payload,
    labels = @labels,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING *;

-- name: UpdateAssetLastSeen :exec
UPDATE assets
SET last_seen_at = CURRENT_TIMESTAMP,
    agent_version = @agent_version,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateAssetEnrollmentState :exec
UPDATE assets
SET enrollment_state = @state,
    enrolled_at = CASE WHEN @state = 'enrolled' AND enrolled_at IS NULL THEN CURRENT_TIMESTAMP ELSE enrolled_at END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateAssetPayload :exec
UPDATE assets
SET subtype_payload = @payload,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateAssetCMDB :exec
UPDATE assets
SET lifecycle_state = @lifecycle_state,
    location = @location,
    vendor = @vendor,
    purchase_date = @purchase_date,
    purchase_price = @purchase_price,
    warranty_expires_at = @warranty_expires_at,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateAssetOwner :exec
-- Superseded by SetAssetOwner for new call sites (added in the owner-pairing
-- task); kept as-is here to avoid unrelated churn, not because both are needed.
UPDATE assets
SET owner_identity_id = @owner_id,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateAssetSite :exec
UPDATE assets
SET site_id = @site_id,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: DeleteAsset :exec
DELETE FROM assets WHERE id = @id;

-- name: CountAssetsByTenant :one
SELECT COUNT(*) FROM assets
WHERE tenant_id = @tenant_id;

-- name: CountAssetsBySubtype :one
SELECT COUNT(*) FROM assets
WHERE tenant_id = @tenant_id AND subtype = @subtype;

-- name: CountAssetsByEnrollmentState :one
SELECT COUNT(*) FROM assets
WHERE tenant_id = @tenant_id AND enrollment_state = @state;

-- Search assets by JSON payload fields (hostname)
-- name: SearchAssetsByHostname :many
SELECT * FROM assets
WHERE tenant_id = @tenant_id
  AND subtype IN ('computer', 'server')
  AND json_extract(subtype_payload, '$.hostname') LIKE '%' || @search || '%'
ORDER BY last_seen_at DESC
LIMIT @limit;

-- Search assets across multiple fields
-- name: SearchAssets :many
SELECT 
    a.*,
    t.slug as tenant_slug,
    s.name as site_name
FROM assets a
LEFT JOIN tenants t ON a.tenant_id = t.id
LEFT JOIN sites s ON a.site_id = s.id
WHERE a.tenant_id = @tenant_id
  AND (
    a.human_id LIKE '%' || @search || '%'
    OR json_extract(a.subtype_payload, '$.hostname') LIKE '%' || @search || '%'
    OR json_extract(a.subtype_payload, '$.name') LIKE '%' || @search || '%'
    OR a.uuid LIKE '%' || @search || '%'
  )
ORDER BY a.last_seen_at DESC
LIMIT @limit;

-- Filter assets by enrollment state
-- name: ListAssetsByEnrollmentState :many
SELECT * FROM assets
WHERE tenant_id = @tenant_id AND enrollment_state = @state
ORDER BY last_seen_at DESC
LIMIT @limit OFFSET @offset;

-- Get assets not seen since a timestamp (stale)
-- name: ListStaleAssets :many
SELECT * FROM assets
WHERE tenant_id = @tenant_id
  AND last_seen_at < @since
ORDER BY last_seen_at ASC
LIMIT @limit;

-- Get assets by lifecycle state
-- name: ListAssetsByLifecycleState :many
SELECT * FROM assets
WHERE tenant_id = @tenant_id AND lifecycle_state = @state
ORDER BY last_seen_at DESC
LIMIT @limit OFFSET @offset;

-- name: SetAssetOwner :exec
-- Narrow, single-column update: unlike UpdateAsset (which requires
-- re-supplying site_id/subtype_payload/labels), this only ever touches
-- ownership so callers cannot accidentally clobber unrelated fields.
UPDATE assets SET owner_identity_id = @owner_id, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: ClearAssetOwner :exec
UPDATE assets SET owner_identity_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetAssetDescription :exec
UPDATE assets SET description = @description, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- name: SetAssetManagedBy :exec
UPDATE assets SET managed_by_identity_id = @managed_by, updated_at = CURRENT_TIMESTAMP WHERE id = @id;

-- Summary statistics for dashboard
-- name: GetAssetStatsByTenant :one
SELECT 
    COUNT(*) as total,
    SUM(CASE WHEN enrollment_state = 'enrolled' THEN 1 ELSE 0 END) as enrolled,
    SUM(CASE WHEN enrollment_state = 'pending' THEN 1 ELSE 0 END) as pending,
    SUM(CASE WHEN enrollment_state = 'approved' THEN 1 ELSE 0 END) as approved,
    SUM(CASE WHEN subtype = 'computer' THEN 1 ELSE 0 END) as computers,
    SUM(CASE WHEN subtype = 'server' THEN 1 ELSE 0 END) as servers,
    SUM(CASE WHEN subtype = 'printer' THEN 1 ELSE 0 END) as printers,
    SUM(CASE WHEN subtype = 'desk' THEN 1 ELSE 0 END) as desks
FROM assets
WHERE tenant_id = @tenant_id;
