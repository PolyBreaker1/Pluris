package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
)

// Role-assignment handlers for the user detail Roles tab (Task 11).
// Assignment-only surface: the permission editor is a later feature.
// Both mutations require an admin-or-above actor, refuse self-service
// on the actor's own account (mirrors the self-delete guard), append an
// activity_log row and redirect back to the Roles tab.

// requireRoleAdmin returns nil when the caller may manage role
// assignments (admin or super_admin), else a 403.
func requireRoleAdmin(c echo.Context) error {
	sess := auth.FromContext(c.Request().Context())
	if sess == nil || (sess.Role != identities.RoleAdmin && sess.Role != identities.RoleSuperAdmin) {
		return echo.NewHTTPError(http.StatusForbidden, "managing roles requires an admin role")
	}
	return nil
}

// resolveTenantRole loads a role and verifies tenant ownership;
// cross-tenant role ids read as not-found.
func (h *Handler) resolveTenantRole(c echo.Context, roleIDRaw string) (db.Role, error) {
	ctx := c.Request().Context()
	roleID, err := strconv.ParseInt(roleIDRaw, 10, 64)
	if err != nil {
		return db.Role{}, echo.NewHTTPError(http.StatusBadRequest, "invalid role id")
	}
	role, err := h.db.Queries.GetRole(ctx, roleID)
	if err != nil {
		return db.Role{}, echo.NewHTTPError(http.StatusNotFound, "role not found")
	}
	sess := auth.FromContext(ctx)
	if sess == nil || role.TenantID != sess.TenantID {
		return db.Role{}, echo.NewHTTPError(http.StatusNotFound, "role not found")
	}
	return role, nil
}

// logRoleEvent appends the role_assigned/role_removed activity row
// (best-effort, never fails the mutation).
func (h *Handler) logRoleEvent(c echo.Context, identityID int64, event, roleName string) {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	if sess == nil {
		return
	}
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID:        sess.TenantID,
		EntityType:      "identity",
		EntityID:        identityID,
		Event:           event,
		Detail:          sql.NullString{String: roleName, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
}

// UserRoleAssign grants the role named by the role_id form value.
func (h *Handler) UserRoleAssign(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	identityID, err := h.resolveTenantIdentity(c)
	if err != nil {
		return err
	}
	sess := auth.FromContext(ctx)
	if sess.IdentityID == identityID {
		return echo.NewHTTPError(http.StatusBadRequest, "you cannot modify your own roles")
	}
	role, err := h.resolveTenantRole(c, c.FormValue("role_id"))
	if err != nil {
		return err
	}
	if err := h.roleSvc.Assign(ctx, identityID, role.ID, sess.IdentityID); err != nil {
		return err
	}
	h.logRoleEvent(c, identityID, "role_assigned", role.Name)
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(identityID, 10)+"#roles")
}

// UserRoleRemove revokes the role in the route.
func (h *Handler) UserRoleRemove(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	identityID, err := h.resolveTenantIdentity(c)
	if err != nil {
		return err
	}
	sess := auth.FromContext(ctx)
	if sess.IdentityID == identityID {
		return echo.NewHTTPError(http.StatusBadRequest, "you cannot modify your own roles")
	}
	role, err := h.resolveTenantRole(c, c.Param("roleID"))
	if err != nil {
		return err
	}
	if err := h.roleSvc.Remove(ctx, identityID, role.ID); err != nil {
		return err
	}
	h.logRoleEvent(c, identityID, "role_removed", role.Name)
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(identityID, 10)+"#roles")
}
