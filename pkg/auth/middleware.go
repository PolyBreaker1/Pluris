package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/pkg/database"
)

const sessionCookieName = "pluris_session"

// setupExemptPaths are reachable even with zero identities in the DB.
var setupExemptPaths = map[string]bool{
	"/setup":   true,
	"/healthz": true,
}

// authExemptPaths are reachable without a valid session.
var authExemptPaths = map[string]bool{
	"/login":   true,
	"/setup":   true,
	"/healthz": true,
}

func isStaticPath(path string) bool {
	return len(path) >= 8 && path[:8] == "/static/"
}

// SetupGate redirects every request to /setup until at least one
// identity exists anywhere in the database. No default/shared account is
// ever seeded — this is how the very first super_admin gets created.
func SetupGate(dbase *database.Database) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if setupExemptPaths[path] || isStaticPath(path) {
				return next(c)
			}
			count, err := dbase.Queries.CountIdentitiesGlobal(c.Request().Context())
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to check setup state")
			}
			if count == 0 {
				return c.Redirect(http.StatusFound, "/setup")
			}
			return next(c)
		}
	}
}

// RequireAuth resolves the session cookie into a UserSession and stashes
// it in the request context (see WithSession/FromContext). Requests
// without a valid session are redirected to /login, except for the
// exempt paths above.
func RequireAuth(dbase *database.Database, sessions *SessionManager) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if authExemptPaths[path] || isStaticPath(path) {
				return next(c)
			}

			cookie, err := c.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				return c.Redirect(http.StatusFound, "/login")
			}

			sess, err := sessions.Lookup(c.Request().Context(), cookie.Value)
			if err != nil {
				return c.Redirect(http.StatusFound, "/login")
			}

			identity, err := dbase.Queries.GetIdentity(c.Request().Context(), sess.IdentityID)
			if err != nil {
				return c.Redirect(http.StatusFound, "/login")
			}

			effectiveTenant := identity.TenantID
			if identities.Role(identity.Role) == identities.RoleSuperAdmin && sess.ActiveTenantID != 0 {
				effectiveTenant = sess.ActiveTenantID
			}

			userSess := &UserSession{
				IdentityID:  identity.ID,
				Email:       identity.Email,
				DisplayName: identity.DisplayName,
				Role:        identities.Role(identity.Role),
				TenantID:    effectiveTenant,
			}

			if userSess.Role == identities.RoleSuperAdmin {
				tenants, err := dbase.Queries.ListTenants(c.Request().Context())
				if err == nil {
					opts := make([]TenantOption, 0, len(tenants))
					for _, t := range tenants {
						opts = append(opts, TenantOption{ID: t.ID, Name: t.Name})
					}
					userSess.AvailableTenants = opts
				}
			}

			c.SetRequest(c.Request().WithContext(WithSession(c.Request().Context(), userSess)))
			return next(c)
		}
	}
}

// RequireRole enforces the RBAC matrix (CanAccess) against the session
// stashed by RequireAuth. Must run after RequireAuth in the middleware
// chain. Renders a plain 403 — page-level styling of this response is a
// follow-up, not required for the auth system to be correct.
func RequireRole() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if authExemptPaths[path] || isStaticPath(path) {
				return next(c)
			}
			sess := FromContext(c.Request().Context())
			if sess == nil {
				return echo.NewHTTPError(http.StatusForbidden, "no active session")
			}
			if !CanAccess(sess.Role, path) {
				return echo.NewHTTPError(http.StatusForbidden, "not permitted for your role")
			}
			return next(c)
		}
	}
}
