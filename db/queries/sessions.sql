-- Server-side session queries. The raw session token is never stored -
-- only its SHA-256 hash (token_hash). See pkg/auth/session.go (a later task).

-- name: CreateIdentitySession :one
INSERT INTO identity_sessions (
    identity_id, token_hash, ip_address, user_agent, expires_at
) VALUES (
    @identity_id, @token_hash, @ip_address, @user_agent, @expires_at
) RETURNING *;

-- name: GetActiveSessionByTokenHash :one
SELECT * FROM identity_sessions
WHERE token_hash = @token_hash
  AND revoked_at IS NULL
  AND expires_at > CURRENT_TIMESTAMP
LIMIT 1;

-- name: SetSessionActiveTenant :exec
UPDATE identity_sessions SET active_tenant_id = @active_tenant_id WHERE id = @id;

-- name: RevokeSession :exec
UPDATE identity_sessions SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash = @token_hash;

-- name: DeleteExpiredSessions :exec
DELETE FROM identity_sessions WHERE expires_at <= CURRENT_TIMESTAMP;
