-- Module grants queries.
--
-- module_grants is the single, shared per-module access-grant table for
-- both policy modules and the future Scripts feature (a script is an
-- unpackaged module row in policy_modules) -- see db/schema/007's header
-- comment. subject_type/level are validated in Go, not the schema; see
-- pkg/authz/modules.go for the decision logic these rows feed.

-- name: ListGrantsForModule :many
SELECT * FROM module_grants
WHERE module_id = @module_id
ORDER BY subject_type, subject_id;

-- name: UpsertModuleGrant :one
-- Creates a grant, or -- if this (module, subject) pair already has a
-- grant row -- updates it in place to the new level, so a subject never
-- ends up with more than one grant per module.
INSERT INTO module_grants (
    module_id,
    subject_type,
    subject_id,
    level
) VALUES (
    @module_id,
    @subject_type,
    @subject_id,
    @level
)
ON CONFLICT(module_id, subject_type, subject_id)
DO UPDATE SET level = excluded.level
RETURNING *;

-- name: DeleteModuleGrant :exec
DELETE FROM module_grants
WHERE module_id = @module_id
  AND subject_type = @subject_type
  AND subject_id = @subject_id;
