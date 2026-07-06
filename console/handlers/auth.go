// Package handlers — auth.go implements the first-run setup wizard and
// login/logout.
package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

// LoginFailureThreshold locks an account after this many consecutive
// failed login attempts. Matches the design spec's chosen value.
const LoginFailureThreshold = 10

// SetupShow renders the first-run wizard.
func (h *Handler) SetupShow(c echo.Context) error {
	return render(c, templates.SetupPage(csrfTokenFrom(c), ""))
}

// SetupSubmit creates the first tenant + first super_admin identity, then
// redirects to /login. Reachable only while SetupGate allows it through
// (zero identities in the DB) once Task 12 wires that middleware in.
func (h *Handler) SetupSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	orgName := c.FormValue("org_name")
	displayName := c.FormValue("display_name")
	email := c.FormValue("email")
	password := c.FormValue("password")

	if orgName == "" || displayName == "" || email == "" || len(password) < 8 {
		return render(c, templates.SetupPage(csrfTokenFrom(c), "All fields are required; password must be at least 8 characters."))
	}

	tenant, err := h.db.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: orgName,
		Slug: slugify(orgName),
	})
	if err != nil {
		log.Printf("setup: create tenant failed: %v", err)
		return render(c, templates.SetupPage(csrfTokenFrom(c), "Could not create the organization. Try a different name."))
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return render(c, templates.SetupPage(csrfTokenFrom(c), "Could not process password."))
	}

	username := email
	if at := strings.Index(email, "@"); at > 0 {
		username = email[:at]
	}

	created, err := h.identitySvc.Create(ctx, tenant.ID, identities.Identity{
		Username:    username,
		Email:       email,
		DisplayName: displayName,
		Role:        identities.RoleSuperAdmin,
	})
	if err != nil {
		log.Printf("setup: create identity failed: %v", err)
		return render(c, templates.SetupPage(csrfTokenFrom(c), "Could not create the admin account. Try a different email."))
	}

	// Seed the builtin roles and give the first admin a real super_admin
	// assignment (Task 10). Best-effort: the identities.role cache above
	// already grants access, so a failure here must not block setup.
	roleSvc := services.NewRoleService(h.db)
	if err := roleSvc.EnsureBuiltins(ctx, tenant.ID); err != nil {
		log.Printf("setup: ensure builtin roles failed: %v", err)
	} else if sa, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenant.ID, Slug: "super_admin"}); err == nil {
		if err := roleSvc.Assign(ctx, created.ID, sa.ID, created.ID); err != nil {
			log.Printf("setup: assign super_admin failed: %v", err)
		}
	}

	if err := h.db.Queries.SetIdentityPasswordHash(ctx, db.SetIdentityPasswordHashParams{
		ID:           created.ID,
		PasswordHash: nullString(hash),
	}); err != nil {
		log.Printf("setup: save password hash failed: %v", err)
		return render(c, templates.SetupPage(csrfTokenFrom(c), "Could not save the password. Please try again."))
	}

	return c.Redirect(http.StatusFound, "/login")
}

// LoginShow renders the login form.
func (h *Handler) LoginShow(c echo.Context) error {
	return render(c, templates.LoginPage(csrfTokenFrom(c), ""))
}

// LoginSubmit verifies credentials, locks the account after repeated
// failures, and issues a session cookie on success.
func (h *Handler) LoginSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	email := c.FormValue("email")
	password := c.FormValue("password")

	identity, err := h.identitySvc.GetByEmailGlobal(ctx, email)
	if err != nil {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Invalid email or password."))
	}
	if identity.AccountLocked {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "This account is locked. Contact an admin."))
	}
	if !identity.AccountEnabled {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Invalid email or password."))
	}

	dbIdentity, err := h.db.Queries.GetIdentity(ctx, identity.ID)
	if err != nil || !dbIdentity.PasswordHash.Valid {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Invalid email or password."))
	}

	ok, err := auth.VerifyPassword(password, dbIdentity.PasswordHash.String)
	if err != nil || !ok {
		h.db.Queries.RecordLoginFailure(ctx, identity.ID)
		h.db.Queries.LockIdentityIfThresholdExceeded(ctx, db.LockIdentityIfThresholdExceededParams{
			ID:        identity.ID,
			Threshold: LoginFailureThreshold,
		})
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Invalid email or password."))
	}

	h.db.Queries.RecordLoginSuccess(ctx, identity.ID)

	rawToken, expiresAt, err := h.sessions.Create(ctx, identity.ID, c.RealIP(), c.Request().UserAgent())
	if err != nil {
		return render(c, templates.LoginPage(csrfTokenFrom(c), "Could not start a session. Try again."))
	}

	c.SetCookie(&http.Cookie{
		Name:     "pluris_session",
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isRequestTLS(c),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})

	return c.Redirect(http.StatusFound, "/")
}

// TenantSwitchSubmit lets a super_admin change the active tenant for
// their current session. RBAC (pkg/auth.CanAccess, via the "/tenant-switch"
// entry in the permission matrix) already restricts this route to
// super_admin; this handler double-checks defensively.
func (h *Handler) TenantSwitchSubmit(c echo.Context) error {
	sess := auth.FromContext(c.Request().Context())
	if sess == nil || sess.Role != identities.RoleSuperAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "only super_admin can switch tenants")
	}

	tenantID, err := strconv.ParseInt(c.FormValue("tenant_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid tenant_id")
	}

	cookie, err := c.Cookie("pluris_session")
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "no active session")
	}
	sessionRow, err := h.sessions.Lookup(c.Request().Context(), cookie.Value)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "session not found")
	}

	if err := h.sessions.SetActiveTenant(c.Request().Context(), sessionRow.ID, tenantID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not switch tenant")
	}

	// Referer is client-supplied and not guaranteed same-origin (the CSRF
	// token check already blocks forged cross-origin submissions, but this
	// is defense in depth against ever redirecting off-site). Fall back to
	// "/" unless it parses and matches this request's own host.
	referer := c.Request().Referer()
	if refURL, err := url.Parse(referer); err != nil || refURL.Host != c.Request().Host {
		referer = "/"
	}
	return c.Redirect(http.StatusFound, referer)
}

// LogoutSubmit revokes the current session and clears the cookie.
func (h *Handler) LogoutSubmit(c echo.Context) error {
	if cookie, err := c.Cookie("pluris_session"); err == nil {
		h.sessions.Revoke(c.Request().Context(), cookie.Value)
	}
	c.SetCookie(&http.Cookie{
		Name: "pluris_session", Value: "", Path: "/", MaxAge: -1,
	})
	return c.Redirect(http.StatusFound, "/login")
}

func isRequestTLS(c echo.Context) bool {
	if c.Request().TLS != nil {
		return true
	}
	return c.Request().Header.Get("X-Forwarded-Proto") == "https"
}

// csrfTokenFrom reads the CSRF token Echo's CSRF middleware stashes in
// context under "csrf". Returns "" gracefully if that middleware isn't
// registered yet (it lands in Task 12) — forms just won't have a working
// token until then, which is fine since nothing enforces it yet either.
func csrfTokenFrom(c echo.Context) string {
	token, _ := c.Get("csrf").(string)
	return token
}

// nullString adapts a plain string into sql.NullString, treating "" as
// NULL. Mirrors the unexported helper of the same name in pkg/services,
// which isn't accessible from this package.
func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

// slugify makes a URL-safe tenant slug from a free-text org name: lowercase,
// non-alphanumeric runs collapsed to a single hyphen, trimmed. Falls back to
// a random "tenant-<hex>" slug when the name has no alphanumeric characters
// at all (e.g. "!!!" or emoji-only), so such names don't all collapse to the
// same empty string and collide against the tenants.slug UNIQUE constraint.
func slugify(s string) string {
	var b strings.Builder
	lastWasHyphen := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastWasHyphen = false
		default:
			if !lastWasHyphen && b.Len() > 0 {
				b.WriteRune('-')
				lastWasHyphen = true
			}
		}
	}
	slug := strings.TrimRight(b.String(), "-")
	if slug == "" {
		return "tenant-" + randomSuffix()
	}
	return slug
}

// randomSuffix returns a short random hex string for the slugify fallback.
// Not security-sensitive — it only needs to avoid collisions between
// punctuation/emoji-only org names, so crypto/rand is used here purely for
// its convenient uniform byte output, not for any secrecy guarantee.
func randomSuffix() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
