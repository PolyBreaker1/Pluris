package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
)

// Group-membership handlers for the detail-page Groups tabs (Task 9).
// Add/remove for both member kinds; every mutation appends an
// activity_log row and redirects back to the detail page's Groups tab.

// resolveTenantGroup parses and loads the group and verifies it belongs
// to the caller's tenant; cross-tenant group ids read as not-found.
func (h *Handler) resolveTenantGroup(c echo.Context, groupIDRaw string) (db.Group, error) {
	ctx := c.Request().Context()
	groupID, err := strconv.ParseInt(groupIDRaw, 10, 64)
	if err != nil {
		return db.Group{}, echo.NewHTTPError(http.StatusBadRequest, "invalid group id")
	}
	group, err := h.groupSvc.Get(ctx, groupID)
	if err != nil {
		return db.Group{}, echo.NewHTTPError(http.StatusNotFound, "group not found")
	}
	sess := auth.FromContext(ctx)
	if sess == nil || group.TenantID != sess.TenantID {
		return db.Group{}, echo.NewHTTPError(http.StatusNotFound, "group not found")
	}
	return group, nil
}

// logGroupEvent appends the group_added/group_removed activity row.
// Logging is best-effort: a failed insert never fails the mutation.
func (h *Handler) logGroupEvent(c echo.Context, entityType string, entityID int64, event, groupName string) {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	if sess == nil {
		return
	}
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID:        sess.TenantID,
		EntityType:      entityType,
		EntityID:        entityID,
		Event:           event,
		Detail:          sql.NullString{String: groupName, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
}

// AssetGroupAdd adds the asset to the group named by the group_id form
// value.
func (h *Handler) AssetGroupAdd(c echo.Context) error {
	if err := requirePermission(c, "asset.manage_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	subtype := c.Param("subtype")
	humanOrUUID := c.Param("id")

	assetID, err := h.assetSvc.ResolveDBID(ctx, humanOrUUID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	group, err := h.resolveTenantGroup(c, c.FormValue("group_id"))
	if err != nil {
		return err
	}
	if err := h.groupSvc.AddAssetMember(ctx, group.ID, assetID); err != nil {
		return err
	}
	h.logGroupEvent(c, "asset", assetID, "group_added", group.Name)
	return c.Redirect(http.StatusFound, "/assets/"+subtype+"/"+humanOrUUID+"#groups")
}

// AssetGroupRemove removes the asset from the group in the route.
func (h *Handler) AssetGroupRemove(c echo.Context) error {
	if err := requirePermission(c, "asset.manage_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	subtype := c.Param("subtype")
	humanOrUUID := c.Param("id")

	assetID, err := h.assetSvc.ResolveDBID(ctx, humanOrUUID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	group, err := h.resolveTenantGroup(c, c.Param("groupID"))
	if err != nil {
		return err
	}
	if err := h.groupSvc.RemoveAssetMember(ctx, group.ID, assetID); err != nil {
		// Rule-sourced memberships (Task 6.2) can't be removed by hand --
		// they're managed by the group's dynamic-membership rules.
		if errors.Is(err, services.ErrMemberNotDirect) {
			return echo.NewHTTPError(http.StatusConflict, "this membership is managed by the group's rules")
		}
		return err
	}
	h.logGroupEvent(c, "asset", assetID, "group_removed", group.Name)
	return c.Redirect(http.StatusFound, "/assets/"+subtype+"/"+humanOrUUID+"#groups")
}

// resolveTenantIdentity parses the :id route param and verifies the
// identity belongs to the caller's tenant.
func (h *Handler) resolveTenantIdentity(c echo.Context) (int64, error) {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	user, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	sess := auth.FromContext(ctx)
	if sess == nil || user.TenantID != sess.TenantID {
		return 0, echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	return id, nil
}

// UserGroupAdd adds the user to the group named by the group_id form
// value.
func (h *Handler) UserGroupAdd(c echo.Context) error {
	if err := requirePermission(c, "identity.assign_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	identityID, err := h.resolveTenantIdentity(c)
	if err != nil {
		return err
	}
	group, err := h.resolveTenantGroup(c, c.FormValue("group_id"))
	if err != nil {
		return err
	}
	if err := h.groupSvc.AddIdentityMember(ctx, group.ID, identityID); err != nil {
		return err
	}
	// Best-effort: group membership can carry group-assigned roles
	// (RBAC v2), so the identity's privilege cache may now be stale.
	_ = h.roleSvc.RecomputeForIdentity(ctx, identityID)
	h.logGroupEvent(c, "identity", identityID, "group_added", group.Name)
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(identityID, 10)+"#groups")
}

// UserGroupRemove removes the user from the group in the route.
func (h *Handler) UserGroupRemove(c echo.Context) error {
	if err := requirePermission(c, "identity.assign_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	identityID, err := h.resolveTenantIdentity(c)
	if err != nil {
		return err
	}
	group, err := h.resolveTenantGroup(c, c.Param("groupID"))
	if err != nil {
		return err
	}
	if err := h.groupSvc.RemoveIdentityMember(ctx, group.ID, identityID); err != nil {
		// Rule-sourced memberships (Task 6.2) can't be removed by hand.
		if errors.Is(err, services.ErrMemberNotDirect) {
			return echo.NewHTTPError(http.StatusConflict, "this membership is managed by the group's rules")
		}
		return err
	}
	// Best-effort: losing membership can drop group-assigned roles.
	_ = h.roleSvc.RecomputeForIdentity(ctx, identityID)
	h.logGroupEvent(c, "identity", identityID, "group_removed", group.Name)
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(identityID, 10)+"#groups")
}
