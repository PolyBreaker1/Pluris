-- Retention setting queries.

-- name: ListRetentionSettings :many
SELECT * FROM retention_settings ORDER BY entity_kind;

-- name: GetRetentionSetting :one
SELECT * FROM retention_settings WHERE entity_kind = @entity_kind;

-- name: UpdateRetentionSetting :one
UPDATE retention_settings
SET purge_after_days = @purge_after_days,
    mode = @mode,
    updated_at = CURRENT_TIMESTAMP,
    updated_by = @updated_by
WHERE entity_kind = @entity_kind
RETURNING *;
