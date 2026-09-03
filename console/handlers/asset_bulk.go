package handlers

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
)

type AssetBulkRequest struct {
	Action string   `json:"action" form:"action"`
	IDs    []string `json:"ids" form:"ids"`
}

func (h *Handler) AssetBulk(c echo.Context) error {
	if err := requirePermission(c, "asset.delete"); err != nil {
		return err
	}
	var req AssetBulkRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Action != "delete" && req.Action != "restore" && req.Action != "purge" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid bulk asset action")
	}
	if len(req.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "at least one asset id is required")
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	resp := PolicyModuleBulkResponse{OK: []string{}, Failed: []PolicyModuleBulkFailure{}}
	for _, id := range req.IDs {
		row, lookupErr := h.db.Queries.GetAssetForDeletion(ctx, db.GetAssetForDeletionParams{TenantID: sess.TenantID, Identifier: sql.NullString{String: id, Valid: true}})
		if lookupErr != nil {
			resp.Failed = append(resp.Failed, PolicyModuleBulkFailure{ID: id, Reason: lookupErr.Error()})
			continue
		}
		var err error
		switch req.Action {
		case "delete":
			err = h.assetSvc.Delete(ctx, sess.TenantID, id, sess.IdentityID)
		case "restore":
			err = h.assetSvc.Restore(ctx, sess.TenantID, id)
		case "purge":
			err = h.assetSvc.PermanentlyDelete(ctx, sess.TenantID, id)
		}
		if err != nil {
			resp.Failed = append(resp.Failed, PolicyModuleBulkFailure{ID: id, Reason: err.Error()})
			continue
		}
		resp.OK = append(resp.OK, id)
		h.recordAssetBulkActivity(ctx, sess, row.ID, id, req.Action)
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) recordAssetBulkActivity(ctx context.Context, sess *auth.UserSession, entityID int64, identifier, action string) {
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID: sess.TenantID, EntityType: "asset", EntityID: entityID,
		Event: "asset_" + action, Detail: sql.NullString{String: identifier, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
}
