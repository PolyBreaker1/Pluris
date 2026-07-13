package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

// Task 8 (field-update API): the flagship consumer of the authz stack
// shipped in Tasks 1-7. It powers the per-section inline-edit UI
// (web/static/detail.js's saveSectionEdit) with a small JSON API per
// entity kind. See
// docs/history/specs/2026-07-08-pluris-policy-authz-design.md
// section "5. User management backend" and
// docs/endpoint-management/concepts/identity-assets.md (field API) for the frontend
// contract these handlers implement.

// FieldUpdateRequest is the JSON body both field-update endpoints bind:
// {"section": "identity", "fields": {"email": "new@example.com"}}.
type FieldUpdateRequest struct {
	Section string            `json:"section"`
	Fields  map[string]string `json:"fields"`
}

// FieldUpdateResponse is the 200 OK body: the param keys actually
// applied, sorted for deterministic output.
type FieldUpdateResponse struct {
	Updated []string `json:"updated"`
}

// UserFieldUpdate handles POST /api/users/:id/fields. Route-level RBAC
// only requires an authenticated session (see server.go's registration
// comment); this handler carries the actual authorization:
//   - identity.update, scoped to the target identity's own id via
//     requirePermissionScoped -- "all" scope may edit anyone, "own"
//     scope only the caller's own identity.
//   - When the caller's effective scope for identity.update is "own"
//     (and they don't hold the super_admin bypass), every submitted key
//     must additionally be in identities.SelfServiceEditableKeys --
//     self-service users cannot use this endpoint to change role,
//     employment, or security fields on themselves.
func (h *Handler) UserFieldUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)

	targetID, err := h.resolveTenantIdentity(c)
	if err != nil {
		return err
	}
	if err := requirePermissionScoped(c, "identity.update", targetID); err != nil {
		return err
	}

	var req FieldUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if sess != nil && sess.Grants.ScopeOf("identity.update") == "own" && !sess.Grants.Can(authz.BypassKey) {
		for key := range req.Fields {
			if !identities.SelfServiceEditableKeys[key] {
				return echo.NewHTTPError(http.StatusForbidden, "field not self-editable: "+key)
			}
		}
	}

	updated, err := h.identitySvc.UpdateFields(ctx, sess.TenantID, targetID, req.Section, req.Fields)
	if err != nil {
		return fieldUpdateHTTPError(err)
	}
	sort.Strings(updated)

	h.logFieldUpdateActivity(c, "identity", targetID, "user_updated", req.Section)
	return c.JSON(http.StatusOK, FieldUpdateResponse{Updated: updated})
}

// AssetFieldUpdate handles POST /api/assets/:subtype/:id/fields. Same
// gate shape as UserFieldUpdate but on asset.update, scoped to the
// asset's owning identity (0 for an unowned asset, so "own" scope never
// passes -- there is no self to compare against).
func (h *Handler) AssetFieldUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	subtype := c.Param("subtype")

	assetID, ownerID, err := h.resolveTenantAssetForFields(c, subtype)
	if err != nil {
		return err
	}
	if err := requirePermissionScoped(c, "asset.update", ownerID); err != nil {
		return err
	}

	var req FieldUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	updated, err := h.assetSvc.UpdateFields(ctx, sess.TenantID, assetID, subtype, req.Section, req.Fields)
	if err != nil {
		return fieldUpdateHTTPError(err)
	}
	sort.Strings(updated)

	h.logFieldUpdateActivity(c, "asset", assetID, "asset_updated", req.Section)
	return c.JSON(http.StatusOK, FieldUpdateResponse{Updated: updated})
}

// resolveTenantAssetForFields resolves the :id route param (numeric DB
// id or the human_id/UUID used on the detail-page URL) to the asset's
// numeric DB id and owning identity id, verifying tenant and subtype
// match. Cross-tenant, wrong-subtype, and unknown ids all fail closed
// as 404 -- mirroring resolveTenantIdentity's behavior for users.
func (h *Handler) resolveTenantAssetForFields(c echo.Context, subtype string) (assetID int64, ownerID int64, err error) {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	raw := c.Param("id")

	id, perr := strconv.ParseInt(raw, 10, 64)
	if perr != nil {
		id, perr = h.assetSvc.ResolveDBID(ctx, raw)
		if perr != nil {
			return 0, 0, echo.NewHTTPError(http.StatusNotFound, "asset not found")
		}
	}

	row, qerr := h.db.Queries.GetAsset(ctx, id)
	if qerr != nil {
		return 0, 0, echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	if sess == nil || row.TenantID != sess.TenantID || row.Subtype != subtype {
		return 0, 0, echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	if row.OwnerIdentityID.Valid {
		ownerID = row.OwnerIdentityID.Int64
	}
	return id, ownerID, nil
}

// fieldUpdateHTTPError maps a services.UpdateFields error to the HTTP
// status the field-update contract promises: ErrFieldNotFound -> 404,
// ErrFieldValidation (or anything else, defensively) -> 400.
func fieldUpdateHTTPError(err error) error {
	if errors.Is(err, services.ErrFieldNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return echo.NewHTTPError(http.StatusBadRequest, err.Error())
}

// logFieldUpdateActivity appends a best-effort activity_log row for a
// field-update mutation. detail records the edited section since the
// specific field keys are already visible in the 200 response.
func (h *Handler) logFieldUpdateActivity(c echo.Context, entityType string, entityID int64, event, section string) {
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
		Detail:          sql.NullString{String: "section: " + section, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
}
