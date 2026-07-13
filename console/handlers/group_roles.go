package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
)

// Group-role assignment handlers (RBAC v2 Task 4, brief section 4). No
// group detail page exists yet (Task 7 builds it) -- these two routes are
// registered ahead of that UI so the backend surface is ready. Gated on
// console_access.manage_role_assignments, same permission as the user
// detail Roles tab's UserRoleAssign/UserRoleRemove (roles.go).

// logGroupRoleEvent appends the group_role_assigned/group_role_removed
// activity row (best-effort, never fails the mutation). Distinct from
// groups.go's logGroupEvent (entity_type "identity"/"asset") and
// pluris_policy.go's logRoleActivity (entity_type "role") -- this one
// hardcodes entity_type "group" with the group's own id, mirroring how
// logRoleActivity anchors role mutations to the role.
func (h *Handler) logGroupRoleEvent(c echo.Context, groupID int64, event, roleName string) {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	if sess == nil {
		return
	}
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID:        sess.TenantID,
		EntityType:      "group",
		EntityID:        groupID,
		Event:           event,
		Detail:          sql.NullString{String: roleName, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
}

// groupRoleRedirectTarget sends the caller back where they came from
// (Referer), falling back to the group detail page. Referer is
// client-supplied and not guaranteed same-origin (the CSRF token check
// already blocks forged cross-origin submissions, but this is defense in
// depth against ever redirecting off-site) -- same same-origin pattern as
// auth.go's TenantSwitchSubmit.
func groupRoleRedirectTarget(c echo.Context, groupID int64) string {
	fallback := "/groups/" + strconv.FormatInt(groupID, 10)
	referer := c.Request().Referer()
	if refURL, err := url.Parse(referer); err != nil || refURL.Host != c.Request().Host {
		return fallback
	}
	return referer
}

// GroupRoleAssign grants the role named by the role_id form value to the
// group in the route, then recomputes every member's identities.role
// privilege cache (a group-role change can raise or lower every member's
// effective access).
func (h *Handler) GroupRoleAssign(c echo.Context) error {
	if err := requirePermission(c, "console_access.manage_role_assignments"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	role, err := h.resolveTenantRole(c, c.FormValue("role_id"))
	if err != nil {
		return err
	}
	sess := auth.FromContext(ctx)
	// Self-escalation speed bump: an actor who already belongs to the
	// target group could grant themselves whatever the role carries.
	// This is not a full raise-guard (it doesn't compare grant sets the
	// way a wouldLockOutActor-style check would) -- it's parity with the
	// blanket self-service refusal UserRoleAssign applies (roles.go),
	// scoped to the group-membership case. super_admin (bypass grant)
	// is exempt, same as elsewhere.
	if sess != nil && !sess.Grants.Can(authz.BypassKey) {
		members, err := h.groupSvc.ListIdentityMembers(ctx, group.ID)
		if err != nil {
			return err
		}
		for _, m := range members {
			if m.ID == sess.IdentityID {
				return echo.NewHTTPError(http.StatusBadRequest, "you cannot assign roles to a group you belong to")
			}
		}
	}
	var assignedBy sql.NullInt64
	if sess != nil {
		assignedBy = sql.NullInt64{Int64: sess.IdentityID, Valid: true}
	}
	if err := h.db.Queries.AssignRoleToGroup(ctx, db.AssignRoleToGroupParams{
		GroupID:    group.ID,
		RoleID:     role.ID,
		AssignedBy: assignedBy,
	}); err != nil {
		return err
	}
	_ = h.roleSvc.RecomputeForGroupMembers(ctx, group.ID)
	h.logGroupRoleEvent(c, group.ID, "group_role_assigned", role.Name)
	return c.Redirect(http.StatusFound, groupRoleRedirectTarget(c, group.ID))
}

// GroupRoleRemove revokes the role in the route from the group in the
// route, then recomputes every member's identities.role privilege cache.
func (h *Handler) GroupRoleRemove(c echo.Context) error {
	if err := requirePermission(c, "console_access.manage_role_assignments"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	role, err := h.resolveTenantRole(c, c.Param("roleID"))
	if err != nil {
		return err
	}
	if err := h.db.Queries.RemoveRoleFromGroup(ctx, db.RemoveRoleFromGroupParams{
		GroupID: group.ID,
		RoleID:  role.ID,
	}); err != nil {
		return err
	}
	_ = h.roleSvc.RecomputeForGroupMembers(ctx, group.ID)
	h.logGroupRoleEvent(c, group.ID, "group_role_removed", role.Name)
	return c.Redirect(http.StatusFound, groupRoleRedirectTarget(c, group.ID))
}
