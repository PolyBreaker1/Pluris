package handlers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

// updateGroupNameParams builds the UpdateGroup params for a rename,
// keeping the group's site assignment unchanged.
func updateGroupNameParams(group db.Group, name string) db.UpdateGroupParams {
	return db.UpdateGroupParams{Name: name, SiteID: group.SiteID, ID: group.ID}
}

// Task 6.2 — the standardized AD-style group pages: /groups list,
// /groups/new create, plus the detail page's mutation routes (fields,
// members, rules, match-mode, recalculate, delete). Modeled on
// console/handlers/config_groups.go's list/create/detail/field shape.
//
// RBAC layering: the "/groups" route prefix is gated on group.view in
// pkg/auth/rbac.go (coarse layer); every mutating handler below repeats
// the finer group.* key via requirePermission (create/update/delete/
// manage_members), same two-layer pattern as the config-group handlers.

// groupPickerKinds maps a group's member_kind onto the TargetPickerDialog
// kinds its Members tab may offer: asset → computers only, identity →
// users only, mixed → both.
func groupPickerKinds(memberKind string) []configgroups.TargetKind {
	switch memberKind {
	case services.MemberKindAsset:
		return []configgroups.TargetKind{configgroups.KindComputer}
	case services.MemberKindIdentity:
		return []configgroups.TargetKind{configgroups.KindUser}
	}
	return []configgroups.TargetKind{configgroups.KindComputer, configgroups.KindUser}
}

// groupKindInView reports whether a group with memberKind belongs in the
// list page's kind view: the identity view shows identity+mixed groups,
// the asset view asset+mixed, and any other (or empty) view shows all.
func groupKindInView(memberKind, view string) bool {
	switch view {
	case services.MemberKindIdentity:
		return memberKind == services.MemberKindIdentity || memberKind == services.MemberKindMixed
	case services.MemberKindAsset:
		return memberKind == services.MemberKindAsset || memberKind == services.MemberKindMixed
	}
	return true
}

// GroupsList renders GET /groups. The kind query param picks the
// server-side member-kind view (see groupKindInView); mixed groups
// appear in both filtered views.
func (h *Handler) GroupsList(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	kind := c.QueryParam("kind")

	groups, err := h.groupSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	rows := make([]templates.GroupListRow, 0, len(groups))
	for _, g := range groups {
		if !groupKindInView(g.MemberKind, kind) {
			continue
		}
		count, err := h.groupSvc.CountMembers(ctx, g.ID)
		if err != nil {
			return err
		}
		rows = append(rows, templates.GroupListRow{
			ID:          g.ID,
			Name:        g.Name,
			Description: g.Description,
			MemberKind:  g.MemberKind,
			Membership:  g.Membership,
			Members:     count,
			Category:    g.GroupCategory,
			Scope:       g.GroupScope,
			Created:     g.CreatedAt.Format("2006-01-02"),
		})
	}
	return render(c, templates.GroupsPage(rows, kind))
}

// GroupNew renders the create form (GET /groups/new).
func (h *Handler) GroupNew(c echo.Context) error {
	if err := requirePermission(c, "group.create"); err != nil {
		return err
	}
	return render(c, templates.GroupNewPage(csrfTokenFrom(c), c.QueryParam("kind"), ""))
}

// GroupCreate creates a group and redirects to its detail page
// (POST /groups/new).
func (h *Handler) GroupCreate(c echo.Context) error {
	if err := requirePermission(c, "group.create"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	name := strings.TrimSpace(c.FormValue("name"))
	memberKind := c.FormValue("member_kind")
	membership := c.FormValue("membership")
	if membership == "" {
		membership = services.MembershipStatic
	}
	g, err := h.groupSvc.Create(ctx, sess.TenantID, name, c.FormValue("description"), memberKind, membership, c.FormValue("category"), c.FormValue("scope"))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrGroupNameRequired),
			errors.Is(err, services.ErrInvalidMemberKind),
			errors.Is(err, services.ErrInvalidMembershipMode):
			return render(c, templates.GroupNewPage(csrfTokenFrom(c), c.FormValue("member_kind"), err.Error()))
		}
		return err
	}
	h.logGroupEvent(c, "group", g.ID, "group_created", g.Name)
	return c.Redirect(http.StatusFound, "/groups/"+strconv.FormatInt(g.ID, 10))
}

// GroupDelete deletes a group (POST /groups/:id/delete). A group still
// targeted by Configuration Group assignments is protected
// (services.ErrGroupReferenced -> 409); memberships/rules/group-roles
// cascade via their FKs.
func (h *Handler) GroupDelete(c echo.Context) error {
	if err := requirePermission(c, "group.delete"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	if err := h.groupSvc.Delete(ctx, group.ID); err != nil {
		if errors.Is(err, services.ErrGroupReferenced) {
			return echo.NewHTTPError(http.StatusConflict, "this group is still targeted by configuration group assignments; remove those first")
		}
		return err
	}
	h.logGroupEvent(c, "group", group.ID, "group_deleted", group.Name)
	return c.Redirect(http.StatusFound, "/groups")
}

// GroupFieldUpdate handles POST /api/groups/:id/fields — the General
// tab's inline-edit endpoint. Accepted keys: name, description,
// member_kind, membership. member_kind/membership/description save
// through services.SetGroupMeta (merged with the group's current values,
// since the fields API sends only what changed); name goes through
// UpdateGroup. A member-kind conflict (group still holds members of the
// excluded kind) maps to 409 so the page can show the message inline.
func (h *Handler) GroupFieldUpdate(c echo.Context) error {
	if err := requirePermission(c, "group.update"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}

	var req FieldUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	description := group.Description
	memberKind := group.MemberKind
	membership := group.Membership
	name := group.Name
	updated := make([]string, 0, len(req.Fields))
	metaChanged := false
	for key, value := range req.Fields {
		switch key {
		case "name":
			if strings.TrimSpace(value) == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "name is required")
			}
			name = value
		case "description":
			description = value
			metaChanged = true
		case "member_kind":
			memberKind = value
			metaChanged = true
		case "membership":
			membership = value
			metaChanged = true
		default:
			return echo.NewHTTPError(http.StatusBadRequest, "unknown field: "+key)
		}
		updated = append(updated, key)
	}

	if metaChanged {
		if err := h.groupSvc.SetGroupMeta(ctx, group.ID, description, memberKind, membership, group.RulesMatchMode); err != nil {
			switch {
			case errors.Is(err, services.ErrMemberKindConflict):
				return echo.NewHTTPError(http.StatusConflict, err.Error())
			case errors.Is(err, services.ErrInvalidMemberKind),
				errors.Is(err, services.ErrInvalidMembershipMode):
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			return err
		}
	}
	if name != group.Name {
		if _, err := h.db.Queries.UpdateGroup(ctx, updateGroupNameParams(group, name)); err != nil {
			return err
		}
	}

	sort.Strings(updated)
	h.logFieldUpdateActivity(c, "group", group.ID, "group_updated", req.Section)
	return c.JSON(http.StatusOK, FieldUpdateResponse{Updated: updated})
}

// GroupMemberAdd handles POST /groups/:id/members. The Members tab's
// target picker submits target_kind ("computer"/"user") + target_ref
// (numeric asset/identity id, per TargetService.Catalog's ref contract).
// The kind must be allowed by the group's member_kind, and the target
// must belong to the caller's tenant.
func (h *Handler) GroupMemberAdd(c echo.Context) error {
	if err := requirePermission(c, "group.manage_members"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}

	kind := configgroups.TargetKind(c.FormValue("target_kind"))
	allowed := false
	for _, k := range groupPickerKinds(group.MemberKind) {
		if k == kind {
			allowed = true
			break
		}
	}
	if !allowed {
		return echo.NewHTTPError(http.StatusBadRequest, "member kind not allowed for this group")
	}
	ref, err := strconv.ParseInt(c.FormValue("target_ref"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid member reference")
	}

	switch kind {
	case configgroups.KindComputer:
		asset, err := h.db.Queries.GetAsset(ctx, ref)
		if err != nil || asset.TenantID != sess.TenantID {
			return echo.NewHTTPError(http.StatusNotFound, "asset not found")
		}
		if err := h.groupSvc.AddAssetMember(ctx, group.ID, ref); err != nil {
			return err
		}
		h.logGroupEvent(c, "asset", ref, "group_added", group.Name)
	case configgroups.KindUser:
		ident, err := h.identitySvc.Get(ctx, ref)
		if err != nil || ident.TenantID != sess.TenantID {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
		}
		if err := h.groupSvc.AddIdentityMember(ctx, group.ID, ref); err != nil {
			return err
		}
		// Group membership can carry group-assigned roles (RBAC v2), so
		// the identity's privilege cache may now be stale. Best-effort.
		_ = h.roleSvc.RecomputeForIdentity(ctx, ref)
		h.logGroupEvent(c, "identity", ref, "group_added", group.Name)
	}
	return c.Redirect(http.StatusFound, "/groups/"+strconv.FormatInt(group.ID, 10)+"#members")
}

// GroupMemberRemove handles POST /groups/:id/members/:mid/remove. The
// row's hidden "kind" field says whether :mid is an asset or identity
// id. Only DIRECT memberships may be removed: a rule-sourced row maps
// to 409 (services.ErrMemberNotDirect) — those rows are managed by the
// group's rules.
func (h *Handler) GroupMemberRemove(c echo.Context) error {
	if err := requirePermission(c, "group.manage_members"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	memberID, err := strconv.ParseInt(c.Param("mid"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid member id")
	}

	switch c.FormValue("kind") {
	case services.MemberKindAsset:
		err = h.groupSvc.RemoveAssetMember(ctx, group.ID, memberID)
		if err == nil {
			h.logGroupEvent(c, "asset", memberID, "group_removed", group.Name)
		}
	case services.MemberKindIdentity:
		err = h.groupSvc.RemoveIdentityMember(ctx, group.ID, memberID)
		if err == nil {
			_ = h.roleSvc.RecomputeForIdentity(ctx, memberID)
			h.logGroupEvent(c, "identity", memberID, "group_removed", group.Name)
		}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid member kind")
	}
	if err != nil {
		if errors.Is(err, services.ErrMemberNotDirect) {
			return echo.NewHTTPError(http.StatusConflict, "this member is managed by the group's rules and cannot be removed directly")
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/groups/"+strconv.FormatInt(group.ID, 10)+"#members")
}

// GroupRuleAdd handles POST /groups/:id/rules — the Rules tab's
// condition-builder save. Same form contract as
// DependencyGroupConditionAdd (repeated values= keys; param-path resolved
// against catalog/params; operator allow-listed against
// dependencygroups.AllOperators()).
func (h *Handler) GroupRuleAdd(c echo.Context) error {
	if err := requirePermission(c, "group.update"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	kind := c.FormValue("kind")
	if kind == "" {
		kind = string(dependencygroups.KindParam)
	}
	paramPath := c.FormValue("param_path")
	operator := c.FormValue("operator")
	scriptSource := c.FormValue("script_source")
	scriptExpect := c.FormValue("script_expect")

	if kind == string(dependencygroups.KindParam) {
		if _, _, _, err := params.ResolvePath(paramPath); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown parameter path")
		}
		if !dependencyGroupConditionOperatorAllowed(operator) {
			return echo.NewHTTPError(http.StatusBadRequest, "unsupported operator")
		}
	}
	formParams, err := c.FormParams()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid form data")
	}
	values := make([]string, len(formParams["values"]))
	for i, v := range formParams["values"] {
		values[i] = strings.TrimSpace(v)
	}

	if _, err := h.groupSvc.AddRule(ctx, group.ID, kind, paramPath, operator, values, scriptSource, scriptExpect); err != nil {
		switch {
		case errors.Is(err, services.ErrGroupNotDynamic):
			return echo.NewHTTPError(http.StatusBadRequest, "rules require a dynamic group")
		case errors.Is(err, services.ErrInvalidConditionKind),
			errors.Is(err, services.ErrInvalidOperator),
			errors.Is(err, services.ErrParamPathRequired),
			errors.Is(err, services.ErrScriptSourceRequired),
			errors.Is(err, services.ErrInvalidScriptExpect):
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/groups/"+strconv.FormatInt(group.ID, 10)+"#rules")
}

// GroupRuleRemove handles POST /groups/:id/rules/:rid/remove. The
// service re-evaluates membership immediately, so members that only
// matched via the removed rule disappear on the spot.
func (h *Handler) GroupRuleRemove(c echo.Context) error {
	if err := requirePermission(c, "group.update"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	ruleID, err := strconv.ParseInt(c.Param("rid"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid rule id")
	}
	if err := h.groupSvc.RemoveRule(ctx, group.ID, ruleID); err != nil {
		if errors.Is(err, services.ErrGroupNotDynamic) {
			return echo.NewHTTPError(http.StatusBadRequest, "rules require a dynamic group")
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/groups/"+strconv.FormatInt(group.ID, 10)+"#rules")
}

// GroupMatchModeUpdate handles POST /groups/:id/match-mode (all/any).
// Saved through SetGroupMeta with the group's other meta unchanged; a
// mode change re-evaluates membership immediately (same trigger
// semantics as rule save/delete) so the Members tab never shows a list
// computed under the previous mode.
func (h *Handler) GroupMatchModeUpdate(c echo.Context) error {
	if err := requirePermission(c, "group.update"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	mode := c.FormValue("match_mode")
	if err := h.groupSvc.SetGroupMeta(ctx, group.ID, group.Description, group.MemberKind, group.Membership, mode); err != nil {
		if errors.Is(err, services.ErrInvalidMatchMode) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid match mode")
		}
		return err
	}
	if group.Membership == services.MembershipDynamic {
		if _, err := h.groupSvc.EvaluateDynamicMembership(ctx, group.ID); err != nil {
			return err
		}
	}
	return c.Redirect(http.StatusFound, "/groups/"+strconv.FormatInt(group.ID, 10)+"#rules")
}

// GroupRecalculate handles POST /groups/:id/recalculate. Zero-rules
// guard (defense-in-depth behind the UI's disabled button): with no
// rules, a dynamic group's vacuous-AND evaluation would add EVERY
// eligible tenant candidate, so the request is rejected outright rather
// than confirmed — there is no legitimate zero-rules recalculation.
// On success the reconciliation counts ride the redirect's query params
// into the detail page's flash banner.
func (h *Handler) GroupRecalculate(c echo.Context) error {
	if err := requirePermission(c, "group.manage_members"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	group, err := h.resolveTenantGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	rules, err := h.groupSvc.ListRules(ctx, group.ID)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "this dynamic group has no rules; add at least one rule before recalculating")
	}
	result, err := h.groupSvc.EvaluateDynamicMembership(ctx, group.ID)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotDynamic) {
			return echo.NewHTTPError(http.StatusBadRequest, "only dynamic groups can be recalculated")
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/groups/"+strconv.FormatInt(group.ID, 10)+
		"?recalc=1&added="+strconv.Itoa(result.Added)+
		"&removed="+strconv.Itoa(result.Removed)+
		"&total="+strconv.Itoa(result.Total)+"#rules")
}
