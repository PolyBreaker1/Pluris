package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/web/templates"
)

// Add-from-catalog flow (Task 13): GET renders the picker (list, or the
// confirm form when ?policy= names a catalog entry); POST binds the
// policy in a find-or-create "Direct - <entity>" configuration group and
// assigns that group to the entity.

// policyByID finds a catalog policy; nil when unknown.
func policyByID(id string) *policies.Policy {
	for _, p := range policies.Catalog() {
		if p.ID == id {
			pc := p
			return &pc
		}
	}
	return nil
}

// configGroupScope maps a policy scope to the configuration_groups CHECK
// vocabulary (machine/user/both).
func configGroupScope(s policies.Scope) string {
	switch s {
	case policies.ScopeComputer:
		return "machine"
	case policies.ScopeUser:
		return "user"
	}
	return "both"
}

// renderPolicyPicker renders both GET states.
func (h *Handler) renderPolicyPicker(c echo.Context, entityName, backHref, selfHref string) error {
	var selected *policies.Policy
	if pid := c.QueryParam("policy"); pid != "" {
		selected = policyByID(pid)
		if selected == nil {
			return echo.NewHTTPError(http.StatusNotFound, "unknown policy")
		}
	}
	return render(c, templates.PolicyPickerPage(entityName, backHref, selfHref, selected, csrfTokenFrom(c)))
}

// assignPolicyDirect performs the POST: find-or-create the direct
// configuration group, bind the policy, assign the group to the target
// and log the event. Idempotent on repeats. The actual find-or-create +
// binding + assignment writes live in services.ConfigGroupService.
// AssignPolicyDirect (Task 5.2 reconciliation) -- this handler only
// resolves the group name/description/scope and logs the activity row.
func (h *Handler) assignPolicyDirect(ctx context.Context, tenantID int64, targetType string, targetID int64, entityName string, pol *policies.Policy, actorID int64) error {
	groupName := "Direct - " + entityName
	if _, err := h.configGroupSvc.AssignPolicyDirect(ctx, tenantID, groupName, "Direct assignments for "+entityName, configGroupScope(pol.Scope), pol.ID, targetType, targetID); err != nil {
		return err
	}
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID:        tenantID,
		EntityType:      targetType,
		EntityID:        targetID,
		Event:           "policy_assigned",
		Detail:          sql.NullString{String: pol.Name, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: actorID, Valid: true},
	})
	return nil
}

// AssetPolicyAdd handles GET (picker/confirm) for assets.
func (h *Handler) AssetPolicyAdd(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.assign_policies"); err != nil {
		return err
	}
	subtype := c.Param("subtype")
	id := c.Param("id")
	asset, err := h.assetSvc.GetByID(c.Request().Context(), id)
	if err != nil || asset == nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	base := "/assets/" + subtype + "/" + id
	return h.renderPolicyPicker(c, asset.PrimaryHostname(), base+"#policies", base+"/policies/add")
}

// AssetPolicyAddSubmit handles the POST for assets.
func (h *Handler) AssetPolicyAddSubmit(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.assign_policies"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	subtype := c.Param("subtype")
	id := c.Param("id")
	asset, err := h.assetSvc.GetByID(ctx, id)
	if err != nil || asset == nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	dbID, err := h.assetSvc.ResolveDBID(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}
	pol := policyByID(c.FormValue("policy"))
	if pol == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "unknown policy")
	}
	sess := auth.FromContext(ctx)
	if err := h.assignPolicyDirect(ctx, sess.TenantID, "asset", dbID, asset.PrimaryHostname(), pol, sess.IdentityID); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/assets/"+subtype+"/"+id+"#policies")
}

// UserPolicyAdd handles GET (picker/confirm) for users.
func (h *Handler) UserPolicyAdd(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.assign_policies"); err != nil {
		return err
	}
	identityID, err := h.resolveTenantIdentity(c)
	if err != nil {
		return err
	}
	user, err := h.identitySvc.Get(c.Request().Context(), identityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	base := "/users/" + strconv.FormatInt(identityID, 10)
	return h.renderPolicyPicker(c, user.ResolvedDisplayName(), base+"#policies", base+"/policies/add")
}

// UserPolicyAddSubmit handles the POST for users.
func (h *Handler) UserPolicyAddSubmit(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.assign_policies"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	identityID, err := h.resolveTenantIdentity(c)
	if err != nil {
		return err
	}
	user, err := h.identitySvc.Get(ctx, identityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	pol := policyByID(c.FormValue("policy"))
	if pol == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "unknown policy")
	}
	sess := auth.FromContext(ctx)
	if err := h.assignPolicyDirect(ctx, sess.TenantID, "identity", identityID, user.ResolvedDisplayName(), pol, sess.IdentityID); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(identityID, 10)+"#policies")
}
