-- Audit trail for authentication and identity-management events.

-- name: InsertIdentityAuditLog :exec
INSERT INTO identity_audit_log (
    tenant_id, identity_id, event_type, ip_address, detail
) VALUES (
    @tenant_id, @identity_id, @event_type, @ip_address, @detail
);

-- name: ListIdentityAuditLogByTenant :many
SELECT * FROM identity_audit_log
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @limit;
