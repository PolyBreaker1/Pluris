package auth

import (
	"context"

	"github.com/pluris/pluris/catalog/identities"
)

// TenantOption is one entry in a super_admin's tenant switcher.
type TenantOption struct {
	ID   int64
	Name string
}

// UserSession is the resolved, request-scoped view of who is logged in.
// Populated once per request by the auth middleware (a later task) and
// read by handlers (for tenant scoping) and by templ components (for the
// nav user-menu and tenant switcher) via FromContext.
type UserSession struct {
	IdentityID  int64
	Email       string
	DisplayName string
	Role        identities.Role

	// TenantID is the EFFECTIVE tenant for this request: the super_admin's
	// switched ActiveTenantID if set, else the identity's own home tenant.
	TenantID int64

	// AvailableTenants is populated only when Role == identities.RoleSuperAdmin;
	// every other role never sees a switcher.
	AvailableTenants []TenantOption
}

type contextKey int

const (
	sessionContextKey contextKey = iota
	csrfContextKey
)

// WithSession returns a new context carrying sess.
func WithSession(ctx context.Context, sess *UserSession) context.Context {
	return context.WithValue(ctx, sessionContextKey, sess)
}

// FromContext returns the session stashed by WithSession, or nil if none
// is present (e.g. before the auth middleware has run, or on /login and
// /setup which never carry one).
func FromContext(ctx context.Context) *UserSession {
	sess, _ := ctx.Value(sessionContextKey).(*UserSession)
	return sess
}

// WithCSRFToken returns a new context carrying the current request's CSRF
// token. Echo's CSRF middleware only exposes the token via c.Set (on
// echo.Context, not context.Context), so a small middleware re-stashes it
// here — this is what lets templ components (which only ever see a
// context.Context, via the generated Render method) render forms with a
// working hidden _csrf field without every page handler having to thread
// the token through its own function signature.
func WithCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfContextKey, token)
}

// CSRFTokenFromContext returns the token stashed by WithCSRFToken, or ""
// if none is present.
func CSRFTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(csrfContextKey).(string)
	return token
}
