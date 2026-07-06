package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// SessionTTL is fixed (not sliding) per the approved design — every
// login issues a fresh 30-day session rather than extending an old one.
const SessionTTL = 30 * 24 * time.Hour

// Session is the resolved, in-memory shape of an identity_sessions row.
type Session struct {
	ID             int64
	IdentityID     int64
	ActiveTenantID int64 // 0 when unset
	ExpiresAt      time.Time
}

// Note: expired sessions are excluded from lookups (see
// GetActiveSessionByTokenHash's WHERE clause) but not actively purged.
// db/queries/sessions.sql's DeleteExpiredSessions query exists for a
// future periodic cleanup job — not wired up yet; rows just accumulate
// until that lands.

// SessionManager creates, looks up, and revokes server-side sessions.
type SessionManager struct {
	db *database.Database
}

func NewSessionManager(db *database.Database) *SessionManager {
	return &SessionManager{db: db}
}

// Create issues a new session for identityID and returns the raw token
// (to be set as the cookie value — never persisted) and its expiry.
func (m *SessionManager) Create(ctx context.Context, identityID int64, ip, userAgent string) (rawToken string, expiresAt time.Time, err error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	rawToken = hex.EncodeToString(tokenBytes)
	// UTC is required here, not cosmetic: GetActiveSessionByTokenHash's
	// SQL compares expires_at against SQLite's CURRENT_TIMESTAMP (always
	// UTC, no offset) as raw strings. go-sqlite3 serializes a time.Time
	// using its own Location, so a non-UTC local time would embed an
	// offset that breaks that lexicographic comparison near timezone
	// boundaries — silently mis-expiring sessions on non-UTC hosts.
	expiresAt = time.Now().UTC().Add(SessionTTL)

	_, err = m.db.Queries.CreateIdentitySession(ctx, db.CreateIdentitySessionParams{
		IdentityID: identityID,
		TokenHash:  hashToken(rawToken),
		IpAddress:  sql.NullString{String: ip, Valid: ip != ""},
		UserAgent:  sql.NullString{String: userAgent, Valid: userAgent != ""},
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return rawToken, expiresAt, nil
}

// Lookup resolves rawToken to an active (non-expired, non-revoked) session.
func (m *SessionManager) Lookup(ctx context.Context, rawToken string) (Session, error) {
	row, err := m.db.Queries.GetActiveSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return Session{}, fmt.Errorf("lookup session: %w", err)
	}
	sess := Session{
		ID:         row.ID,
		IdentityID: row.IdentityID,
		ExpiresAt:  row.ExpiresAt,
	}
	if row.ActiveTenantID.Valid {
		sess.ActiveTenantID = row.ActiveTenantID.Int64
	}
	return sess, nil
}

// SetActiveTenant records the super_admin's currently-switched tenant on
// an existing session (see pkg/auth's RBAC and the tenant-switch handler).
func (m *SessionManager) SetActiveTenant(ctx context.Context, sessionID, tenantID int64) error {
	if err := m.db.Queries.SetSessionActiveTenant(ctx, db.SetSessionActiveTenantParams{
		ID:             sessionID,
		ActiveTenantID: sql.NullInt64{Int64: tenantID, Valid: true},
	}); err != nil {
		return fmt.Errorf("set active tenant: %w", err)
	}
	return nil
}

// Revoke invalidates rawToken immediately (used by logout).
func (m *SessionManager) Revoke(ctx context.Context, rawToken string) error {
	if err := m.db.Queries.RevokeSession(ctx, hashToken(rawToken)); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// hashToken returns the hex-encoded SHA-256 of a raw session token. Only
// this hash is ever persisted — the raw token exists solely in the
// cookie, so a stolen database dump cannot be replayed as a live session.
// SHA-256 (not Argon2id like password.go) is correct here: the input is
// already a high-entropy random token, not a low-entropy human password,
// so a slow KDF would only add unnecessary latency with no security benefit.
func hashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}
