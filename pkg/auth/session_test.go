package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func setupAuthTestDB(t *testing.T) (*database.Database, db.Identity) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_auth_session.db")
	database, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	tenant, err := database.Queries.CreateTenant(context.Background(), db.CreateTenantParams{
		Name: "Test Org", Slug: "test-org-auth",
	})
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	identity, err := database.Queries.CreateIdentity(context.Background(), db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "jdoe", Email: "jdoe@example.com",
		DisplayName: "Jane Doe", Role: "admin",
	})
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}
	return database, identity
}

func TestCreateAndLookupSession(t *testing.T) {
	dbase, identity := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	ctx := context.Background()

	rawToken, _, err := mgr.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if rawToken == "" {
		t.Fatal("expected non-empty raw token")
	}

	sess, err := mgr.Lookup(ctx, rawToken)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if sess.IdentityID != identity.ID {
		t.Fatalf("expected identity id %d, got %d", identity.ID, sess.IdentityID)
	}
}

func TestLookupRejectsRevokedSession(t *testing.T) {
	dbase, identity := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	ctx := context.Background()

	rawToken, _, err := mgr.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := mgr.Revoke(ctx, rawToken); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	if _, err := mgr.Lookup(ctx, rawToken); err == nil {
		t.Fatal("expected Lookup to fail for a revoked session")
	}
}

func TestLookupRejectsUnknownToken(t *testing.T) {
	dbase, _ := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	if _, err := mgr.Lookup(context.Background(), "not-a-real-token"); err == nil {
		t.Fatal("expected Lookup to fail for an unknown token")
	}
}

func TestSessionExpiryIsThirtyDaysOut(t *testing.T) {
	dbase, identity := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	ctx := context.Background()

	_, expiresAt, err := mgr.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	wantMin := time.Now().Add(29 * 24 * time.Hour)
	wantMax := time.Now().Add(31 * 24 * time.Hour)
	if expiresAt.Before(wantMin) || expiresAt.After(wantMax) {
		t.Fatalf("expected expiry ~30 days out, got %v", expiresAt)
	}
}

func TestLookupRejectsExpiredSession(t *testing.T) {
	dbase, identity := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	ctx := context.Background()

	rawToken, _, err := mgr.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Directly backdate expires_at past now via raw SQL, since there's no
	// public API to create an already-expired session (by design). Use
	// UTC here to match what Create itself stores (see session.go) --
	// otherwise this write's local-offset formatting wouldn't compare
	// correctly against CURRENT_TIMESTAMP either, defeating the point of
	// the test.
	_, err = dbase.Conn().Exec(`UPDATE identity_sessions SET expires_at = ? WHERE token_hash = ?`,
		time.Now().UTC().Add(-1*time.Hour), hashToken(rawToken))
	if err != nil {
		t.Fatalf("failed to backdate session: %v", err)
	}

	if _, err := mgr.Lookup(ctx, rawToken); err == nil {
		t.Fatal("expected Lookup to fail for an expired session")
	}
}

func TestSetActiveTenantIsReflectedOnLookup(t *testing.T) {
	dbase, identity := setupAuthTestDB(t)
	mgr := NewSessionManager(dbase)
	ctx := context.Background()

	tenant2, err := dbase.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Second Org", Slug: "second-org-auth",
	})
	if err != nil {
		t.Fatalf("failed to create second tenant: %v", err)
	}

	rawToken, _, err := mgr.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	sess, err := mgr.Lookup(ctx, rawToken)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if err := mgr.SetActiveTenant(ctx, sess.ID, tenant2.ID); err != nil {
		t.Fatalf("SetActiveTenant failed: %v", err)
	}

	sessAfter, err := mgr.Lookup(ctx, rawToken)
	if err != nil {
		t.Fatalf("Lookup after SetActiveTenant failed: %v", err)
	}
	if sessAfter.ActiveTenantID != tenant2.ID {
		t.Fatalf("expected active tenant %d, got %d", tenant2.ID, sessAfter.ActiveTenantID)
	}
}
