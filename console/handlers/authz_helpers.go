package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/pkg/auth"
)

// requirePermission returns nil when the caller's session grants key
// (unscoped: "yes", or either scope value for a scoped key), else a 403
// echo.HTTPError naming the missing permission. A nil session (no auth
// middleware run, or /login-style routes) always denies.
func requirePermission(c echo.Context, key string) error {
	sess := auth.FromContext(c.Request().Context())
	if sess == nil || !sess.Grants.Can(key) {
		return echo.NewHTTPError(http.StatusForbidden, "missing required permission: "+key)
	}
	return nil
}

// requirePermissionScoped returns nil when the caller's session grants
// key at "all" scope, or at "own" scope and ownerID matches the caller's
// own identity, else a 403 echo.HTTPError naming the missing permission.
// A nil session always denies.
func requirePermissionScoped(c echo.Context, key string, ownerID int64) error {
	sess := auth.FromContext(c.Request().Context())
	if sess == nil || !sess.Grants.CanScoped(key, ownerID, sess.IdentityID) {
		return echo.NewHTTPError(http.StatusForbidden, "missing required permission: "+key)
	}
	return nil
}
