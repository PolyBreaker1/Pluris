-- Asset link queries
-- Asset links define relationships between assets (e.g., desk-to-computer, server-to-server)

-- name: CreateAssetLink :one
INSERT INTO asset_links (
    tenant_id,
    from_asset_id,
    to_asset_id,
    relation,
    metadata
) VALUES (
    @tenant_id,
    @from_asset_id,
    @to_asset_id,
    @relation,
    @metadata
) RETURNING *;

-- name: GetAssetLink :one
SELECT * FROM asset_links WHERE id = @id LIMIT 1;

-- name: GetAssetLinkByAssets :one
SELECT * FROM asset_links 
WHERE from_asset_id = @from_asset_id 
  AND to_asset_id = @to_asset_id 
  AND relation = @relation
LIMIT 1;

-- name: ListLinksFromAsset :many
SELECT 
    l.*,
    a.human_id as to_asset_human_id,
    a.subtype as to_asset_subtype,
    json_extract(a.subtype_payload, '$.hostname') as to_asset_hostname,
    json_extract(a.subtype_payload, '$.name') as to_asset_name
FROM asset_links l
JOIN assets a ON a.id = l.to_asset_id
WHERE l.from_asset_id = @from_asset_id
ORDER BY l.relation, a.human_id;

-- name: ListLinksToAsset :many
SELECT 
    l.*,
    a.human_id as from_asset_human_id,
    a.subtype as from_asset_subtype,
    json_extract(a.subtype_payload, '$.hostname') as from_asset_hostname,
    json_extract(a.subtype_payload, '$.name') as from_asset_name
FROM asset_links l
JOIN assets a ON a.id = l.from_asset_id
WHERE l.to_asset_id = @to_asset_id
ORDER BY l.relation, a.human_id;

-- name: ListLinksByRelation :many
SELECT 
    l.*,
    fa.human_id as from_human_id,
    ta.human_id as to_human_id
FROM asset_links l
JOIN assets fa ON fa.id = l.from_asset_id
JOIN assets ta ON ta.id = l.to_asset_id
WHERE l.tenant_id = @tenant_id AND l.relation = @relation
ORDER BY fa.human_id, ta.human_id;

-- name: ListAllLinksForAsset :many
SELECT 
    l.*,
    CASE WHEN l.from_asset_id = @asset_id THEN 'outgoing' ELSE 'incoming' END as direction,
    CASE WHEN l.from_asset_id = @asset_id THEN ta.human_id ELSE fa.human_id END as related_asset_id,
    CASE WHEN l.from_asset_id = @asset_id THEN ta.subtype ELSE fa.subtype END as related_asset_subtype
FROM asset_links l
JOIN assets fa ON fa.id = l.from_asset_id
JOIN assets ta ON ta.id = l.to_asset_id
WHERE l.from_asset_id = @asset_id OR l.to_asset_id = @asset_id
ORDER BY l.relation;

-- name: CountLinksForAsset :one
SELECT COUNT(*) FROM asset_links 
WHERE from_asset_id = @asset_id OR to_asset_id = @asset_id;

-- name: UpdateAssetLink :one
UPDATE asset_links SET
    relation = @relation,
    metadata = @metadata
WHERE id = @id
RETURNING *;

-- name: DeleteAssetLink :exec
DELETE FROM asset_links WHERE id = @id;

-- name: DeleteLinksForAsset :exec
DELETE FROM asset_links 
WHERE from_asset_id = @asset_id OR to_asset_id = @asset_id;

-- name: DeleteLinksByRelation :exec
DELETE FROM asset_links 
WHERE tenant_id = @tenant_id AND relation = @relation;
