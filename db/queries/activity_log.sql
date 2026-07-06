-- Activity log queries - generic per entity event feed backed by the
-- activity_log table in db/schema/003_roles_software_logs.sql.

-- name: InsertActivity :exec
INSERT INTO activity_log (tenant_id, entity_type, entity_id, event, detail, actor_identity_id)
VALUES (@tenant_id, @entity_type, @entity_id, @event, @detail, @actor_identity_id);

-- name: ListActivityForEntity :many
SELECT * FROM activity_log
WHERE tenant_id = @tenant_id
  AND entity_type = @entity_type
  AND entity_id = @entity_id
ORDER BY created_at DESC
LIMIT @limit;
