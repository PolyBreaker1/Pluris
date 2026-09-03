package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

type PolicyModuleBulkRequest struct {
	Action string   `json:"action" form:"action"`
	IDs    []string `json:"ids" form:"ids"`
}

type PolicyModuleBulkFailure struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type PolicyModuleBulkResponse struct {
	OK     []string                  `json:"ok"`
	Failed []PolicyModuleBulkFailure `json:"failed"`
}

// PolicyModuleBulk applies one module action independently to every id.
// One authorization or service failure never rolls back successful items.
func (h *Handler) PolicyModuleBulk(c echo.Context) error {
	if err := requirePermission(c, authz.ActionManageModules); err != nil {
		return err
	}
	var req PolicyModuleBulkRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Action != "clone" && req.Action != "revoke" && req.Action != "delete" && req.Action != "restore" && req.Action != "purge" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid bulk module action")
	}
	if len(req.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "at least one module id is required")
	}

	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	resp := PolicyModuleBulkResponse{OK: []string{}, Failed: []PolicyModuleBulkFailure{}}
	for _, urn := range req.IDs {
		var row db.PolicyModule
		var err error
		if req.Action == "restore" || req.Action == "purge" {
			row, err = h.resolveTenantModuleByURNIncludingDeleted(ctx, sess, urn)
		} else {
			row, err = h.resolveTenantModuleByURN(ctx, sess, urn)
		}
		if err == nil {
			var access authz.ModuleAccessInput
			access, err = h.moduleAccessInput(ctx, sess, row)
			if err == nil {
				switch req.Action {
				case "clone":
					if !authz.ModuleCanView(access) {
						err = errors.New("not allowed to duplicate this module")
					} else {
						_, err = h.clonePolicyModule(ctx, sess, row)
					}
				case "revoke":
					if !authz.ModuleCanEdit(access) {
						err = errors.New("not allowed to revoke versions for this module")
					} else {
						err = h.moduleSvc.RevokeVersions(ctx, row.ID)
						if err == nil {
							h.recordPolicyModuleActivity(ctx, sess, row, "module_versions_revoked")
						}
					}
				case "delete":
					if !authz.ModuleCanAdmin(access) {
						err = errors.New("not allowed to delete this module")
					} else {
						err = h.moduleSvc.DeleteModule(ctx, sess.TenantID, row.ID, row.ModuleUrn, sess.IdentityID)
						if err == nil {
							h.recordPolicyModuleActivity(ctx, sess, row, "module_deleted")
						}
					}
				case "restore":
					if !authz.ModuleCanAdmin(access) {
						err = errors.New("not allowed to restore this module")
					} else {
						err = h.moduleSvc.RestoreModule(ctx, row.ID)
						if err == nil {
							h.recordPolicyModuleActivity(ctx, sess, row, "module_restored")
						}
					}
				case "purge":
					if !authz.ModuleCanAdmin(access) {
						err = errors.New("not allowed to permanently delete this module")
					} else {
						err = h.moduleSvc.PermanentlyDeleteModule(ctx, sess.TenantID, row.ID, row.ModuleUrn)
						if err == nil {
							h.recordPolicyModuleActivity(ctx, sess, row, "module_purged")
						}
					}
				}
			}
		}
		if err != nil {
			resp.Failed = append(resp.Failed, PolicyModuleBulkFailure{ID: urn, Reason: policyModuleBulkReason(err)})
			continue
		}
		resp.OK = append(resp.OK, urn)
	}
	return c.JSON(http.StatusOK, resp)
}

func policyModuleBulkReason(err error) string {
	if errors.Is(err, services.ErrModuleReferenced) {
		return "Module is still referenced by a dependency group, binding, or installation."
	}
	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) {
		if message, ok := httpErr.Message.(string); ok {
			return message
		}
	}
	return err.Error()
}

func (h *Handler) recordPolicyModuleActivity(ctx context.Context, sess *auth.UserSession, row db.PolicyModule, event string) {
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID: sess.TenantID, EntityType: "policy_module", EntityID: row.ID,
		Event: event, Detail: sql.NullString{String: row.ModuleUrn, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
}
