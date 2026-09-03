package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
)

type DependencyGroupBulkRequest struct {
	Action string   `json:"action" form:"action"`
	IDs    []string `json:"ids" form:"ids"`
}

func (h *Handler) DependencyGroupBulk(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_dependency_groups"); err != nil {
		return err
	}
	var req DependencyGroupBulkRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Action != "delete" && req.Action != "restore" && req.Action != "purge" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid bulk dependency group action")
	}
	if len(req.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "at least one dependency group id is required")
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	resp := PolicyModuleBulkResponse{OK: []string{}, Failed: []PolicyModuleBulkFailure{}}
	for _, rawID := range req.IDs {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err == nil {
			switch req.Action {
			case "delete":
				err = h.depGroupSvc.Delete(ctx, sess.TenantID, id, sess.IdentityID)
			case "restore":
				err = h.depGroupSvc.Restore(ctx, sess.TenantID, id)
			case "purge":
				err = h.depGroupSvc.PermanentlyDelete(ctx, sess.TenantID, id)
			}
		}
		if err != nil {
			resp.Failed = append(resp.Failed, PolicyModuleBulkFailure{ID: rawID, Reason: err.Error()})
			continue
		}
		resp.OK = append(resp.OK, rawID)
		h.recordDependencyGroupBulkActivity(ctx, sess, id, req.Action)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) recordDependencyGroupBulkActivity(ctx context.Context, sess *auth.UserSession, id int64, action string) {
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID: sess.TenantID, EntityType: "dependency_group", EntityID: id,
		Event: "dependency_group_" + action, Detail: sql.NullString{},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
}
