package database

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/pluris/pluris/db"
)

func TestIdentityQueriesCRUD(t *testing.T) {
	dbPath := "test_identity_queries.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	tenant, err := database.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Test Org", Slug: "test-org-idq",
	})
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	identity, err := database.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID:    tenant.ID,
		Username:    "jdoe",
		Email:       "jdoe@example.com",
		DisplayName: "Jane Doe",
		Role:        "admin",
	})
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}
	if identity.Username != "jdoe" {
		t.Fatalf("expected username jdoe, got %s", identity.Username)
	}

	got, err := database.Queries.GetIdentity(ctx, identity.ID)
	if err != nil {
		t.Fatalf("failed to get identity: %v", err)
	}
	if got.Email != "jdoe@example.com" {
		t.Fatalf("expected email jdoe@example.com, got %s", got.Email)
	}

	byEmail, err := database.Queries.GetIdentityByEmail(ctx, db.GetIdentityByEmailParams{
		TenantID: tenant.ID, Email: "jdoe@example.com",
	})
	if err != nil {
		t.Fatalf("failed to get identity by email: %v", err)
	}
	if byEmail.ID != identity.ID {
		t.Fatalf("expected id %d, got %d", identity.ID, byEmail.ID)
	}

	list, err := database.Queries.ListIdentitiesByTenant(ctx, db.ListIdentitiesByTenantParams{
		TenantID: tenant.ID, Limit: 100, Offset: 0,
	})
	if err != nil {
		t.Fatalf("failed to list identities: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(list))
	}

	err = database.Queries.SetIdentityPasswordHash(ctx, db.SetIdentityPasswordHashParams{
		ID:           identity.ID,
		PasswordHash: sql.NullString{String: "hashed", Valid: true},
	})
	if err != nil {
		t.Fatalf("failed to set password hash: %v", err)
	}

	afterPasswordSet, err := database.Queries.GetIdentity(ctx, identity.ID)
	if err != nil {
		t.Fatalf("failed to re-fetch identity after setting password hash: %v", err)
	}
	if !afterPasswordSet.PasswordHash.Valid || afterPasswordSet.PasswordHash.String != "hashed" {
		t.Fatalf("expected password hash %q, got %+v", "hashed", afterPasswordSet.PasswordHash)
	}

	session, err := database.Queries.CreateIdentitySession(ctx, db.CreateIdentitySessionParams{
		IdentityID: identity.ID,
		TokenHash:  "deadbeef",
		ExpiresAt:  time.Now().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	_ = session

	activeSession, err := database.Queries.GetActiveSessionByTokenHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("failed to get active session by token hash: %v", err)
	}
	if activeSession.IdentityID != identity.ID {
		t.Fatalf("expected session identity_id %d, got %d", identity.ID, activeSession.IdentityID)
	}

	err = database.Queries.InsertIdentityAuditLog(ctx, db.InsertIdentityAuditLogParams{
		TenantID:   sql.NullInt64{Int64: tenant.ID, Valid: true},
		IdentityID: sql.NullInt64{Int64: identity.ID, Valid: true},
		EventType:  "created",
	})
	if err != nil {
		t.Fatalf("failed to insert audit log: %v", err)
	}

	auditLog, err := database.Queries.ListIdentityAuditLogByTenant(ctx, db.ListIdentityAuditLogByTenantParams{
		TenantID: sql.NullInt64{Int64: tenant.ID, Valid: true},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("failed to list audit log: %v", err)
	}
	if len(auditLog) < 1 {
		t.Fatalf("expected at least 1 audit log entry, got %d", len(auditLog))
	}
	found := false
	for _, entry := range auditLog {
		if entry.EventType == "created" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an audit log entry with event_type %q, got %+v", "created", auditLog)
	}
}
