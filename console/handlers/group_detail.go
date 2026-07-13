package handlers

import (
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/web/templates"
)

// GroupDetail renders the standardized group detail page (Task 6.2:
// General/Members/Rules/Roles tabs on the DetailShell — upgraded from
// Task 7's minimal read-only page). Route-level RBAC gates "/groups" on
// group.view (pkg/auth/rbac.go); resolveTenantGroup (groups.go) 404s
// cross-tenant ids. The Roles tab's mutations stay gated on
// console_access.manage_role_assignments in group_roles.go; the other
// tabs' mutation routes carry their own group.* checks (groups_pages.go).
func (h *Handler) GroupDetail(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	members, err := h.groupSvc.ListMembers(ctx, group.ID)
	if err != nil {
		return err
	}
	// Builtin roles are seeded at setup; EnsureBuiltins here upgrades
	// tenants created before the roles feature existed (idempotent),
	// mirroring UserDetail's call.
	if err := h.roleSvc.EnsureBuiltins(ctx, sess.TenantID); err != nil {
		return err
	}
	roles, err := h.roleSvc.ListForGroupDetail(ctx, group.ID)
	if err != nil {
		return err
	}
	allRoles, err := h.roleSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	rules, err := h.groupSvc.ListRules(ctx, group.ID)
	if err != nil {
		return err
	}
	targets, err := h.targetSvc.Catalog(ctx, sess.TenantID, groupPickerKinds(group.MemberKind))
	if err != nil {
		return err
	}

	// Recalculate-result flash: the recalculate route redirects back here
	// with the reconciliation counts as query params (see GroupRecalculate).
	var recalc *templates.GroupRecalcFlash
	if c.QueryParam("recalc") == "1" {
		added, _ := strconv.Atoi(c.QueryParam("added"))
		removed, _ := strconv.Atoi(c.QueryParam("removed"))
		total, _ := strconv.Atoi(c.QueryParam("total"))
		recalc = &templates.GroupRecalcFlash{Added: added, Removed: removed, Total: total}
	}

	return render(c, templates.GroupDetailPage(templates.GroupDetailData{
		Group:    group,
		Members:  members,
		Roles:    roles,
		AllRoles: allRoles,
		Rules:    rules,
		Targets:  targets,
		Active:   templates.GroupDetailActiveKey(group.MemberKind),
		CSRF:     csrfTokenFrom(c),
		Recalc:   recalc,
	}))
}
