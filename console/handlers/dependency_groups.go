package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

// resolveTenantDepGroup loads a dependency group and verifies tenant
// ownership; cross-tenant ids read as not-found.
func (h *Handler) resolveTenantDepGroup(c echo.Context, idRaw string) (db.DependencyGroup, error) {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		return db.DependencyGroup{}, echo.NewHTTPError(http.StatusBadRequest, "invalid dependency group id")
	}
	row, err := h.db.Queries.GetDependencyGroup(ctx, id)
	if err != nil {
		return db.DependencyGroup{}, echo.NewHTTPError(http.StatusNotFound, "dependency group not found")
	}
	sess := auth.FromContext(ctx)
	if sess == nil || row.TenantID != sess.TenantID {
		return db.DependencyGroup{}, echo.NewHTTPError(http.StatusNotFound, "dependency group not found")
	}
	return row, nil
}

// DependencyGroups renders the browsable list of dependency groups
// (Task 5). EnsureBuiltins seeds the built-in OS/encryption groups for a
// fresh tenant so the list is never empty on first visit.
func (h *Handler) DependencyGroups(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	if err := h.depGroupSvc.EnsureBuiltins(ctx, sess.TenantID); err != nil {
		return err
	}
	deleted := c.QueryParam("state") == "deleted"
	var groups []dependencygroups.Group
	var err error
	if deleted {
		groups, err = h.depGroupSvc.ListDeletedByTenant(ctx, sess.TenantID)
	} else {
		groups, err = h.depGroupSvc.ListByTenant(ctx, sess.TenantID)
	}
	if err != nil {
		return err
	}
	rows := make([]templates.DependencyGroupRow, 0, len(groups))
	for _, g := range groups {
		count, err := h.depGroupSvc.CountLinks(ctx, g.ID)
		if err != nil {
			return err
		}
		rows = append(rows, templates.DependencyGroupRow{
			ID:               g.ID,
			Name:             g.Name,
			Slug:             g.Slug,
			Builtin:          g.Builtin,
			ConditionSummary: templates.DependencyConditionSummary(g),
			UsedBy:           count,
		})
	}
	setting, err := h.retentionSvc.GetSetting(ctx, services.EntityKindDependencyGroup)
	if err != nil {
		return err
	}
	return render(c, templates.DependencyGroupsPage(rows, deleted, services.RetentionDeleteCopy(setting, "dependency groups"), csrfTokenFrom(c)))
}

// DependencyGroupDetail renders a single dependency group on the
// standardized DetailShell (Task 6). It fetches both the pure catalog
// Group (for the name/description/conditions-as-values) and the raw DB
// condition rows (which carry the id ConditionRemove needs — the catalog
// Condition type intentionally has none).
func (h *Handler) DependencyGroupDetail(c echo.Context) error {
	ctx := c.Request().Context()
	row, err := h.resolveTenantDepGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	g, err := h.depGroupSvc.Get(ctx, row.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "dependency group not found")
	}
	conds, err := h.db.Queries.ListConditionsForGroup(ctx, row.ID)
	if err != nil {
		return err
	}
	return render(c, templates.DependencyGroupDetailPage(g, conds, csrfTokenFrom(c)))
}

// DependencyGroupNew renders the create form (admin-only).
func (h *Handler) DependencyGroupNew(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_dependency_groups"); err != nil {
		return err
	}
	return render(c, templates.DependencyGroupNewPage(csrfTokenFrom(c)))
}

// DependencyGroupCreate creates a new custom dependency group and
// redirects to its detail page.
func (h *Handler) DependencyGroupCreate(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_dependency_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	name := c.FormValue("name")
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	g, err := h.depGroupSvc.Create(ctx, sess.TenantID, name, c.FormValue("description"))
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(g.ID, 10))
}

// DependencyGroupUpdate updates a group's name/description. Builtins are
// editable (only deletion is protected).
func (h *Handler) DependencyGroupUpdate(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_dependency_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	row, err := h.resolveTenantDepGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	if err := h.depGroupSvc.Update(ctx, row.ID, c.FormValue("name"), c.FormValue("description")); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(row.ID, 10))
}

// DependencyGroupDelete deletes a custom dependency group; builtins map
// to a 400 via services.ErrBuiltinProtected.
func (h *Handler) DependencyGroupDelete(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_dependency_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	row, err := h.resolveTenantDepGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	sess := auth.FromContext(ctx)
	if err := h.depGroupSvc.Delete(ctx, sess.TenantID, row.ID, sess.IdentityID); err != nil {
		if errors.Is(err, services.ErrBuiltinProtected) {
			return echo.NewHTTPError(http.StatusBadRequest, "builtin groups cannot be deleted")
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups")
}

// dependencyGroupConditionOperatorAllowed reports whether op is one of the
// operators the dependency-group eval engine actually implements (see
// dependencygroups.AllOperators's doc comment — Task 2.1 widened this to
// the full string/numeric/regex set, whose keys are aligned with
// catalog/params' per-type operator sets where the concepts overlap).
func dependencyGroupConditionOperatorAllowed(op string) bool {
	for _, allowed := range dependencygroups.AllOperators() {
		if string(allowed) == op {
			return true
		}
	}
	return false
}

// DependencyGroupConditionAdd appends a condition to a group. The param
// path and the operator are validated against their registries
// (catalog/params.ResolvePath, dependencygroups.AllOperators) before
// hitting the DB — a stopgap guard originally from Task 1.3, since the
// flat condition form no longer constrains input to a fixed <select> the
// browser enforces (registry-driven options can still be bypassed by a
// hand-crafted POST). Task 2.1 widened the operator set this validates
// against (dependencygroups.AllOperators now covers string/numeric/regex
// operators, not just in/not_in/exists) and added an optional "kind" form
// field (default "param" when absent) plus optional script_source/
// script_ref fields for kind="script"/"command".
//
// Values encoding (Task 2.3): "values" is read via c.FormParams(), i.e.
// as REPEATED form keys — one "values=" pair per element — rather than a
// single comma-joined value. The condition-builder page script
// (web/templates/dependency_groups.templ's dependencyGroupConditionScript)
// POSTs it this way so a value containing a literal comma round-trips
// correctly; a single "values=x" still decodes to the one-element slice
// []string{"x"} exactly like the old comma-split did, so pre-2.3 callers
// (including this file's own tests) are unaffected.
func (h *Handler) DependencyGroupConditionAdd(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_dependency_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	row, err := h.resolveTenantDepGroup(c, c.Param("id"))
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
	scriptRef := c.FormValue("script_ref")

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
	if err := h.depGroupSvc.AddCondition(ctx, row.ID, paramPath, operator, values, kind, scriptSource, scriptRef); err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidConditionKind),
			errors.Is(err, services.ErrInvalidOperator),
			errors.Is(err, services.ErrParamPathRequired),
			errors.Is(err, services.ErrScriptSourceRequired),
			errors.Is(err, services.ErrScriptSourceAmbiguous):
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(row.ID, 10)+"#conditions")
}

// DependencyGroupMatchModeUpdate sets a group's match_mode ("all"/"any")
// (Task 2.1). Builtins are protected the same way builtin deletion is.
func (h *Handler) DependencyGroupMatchModeUpdate(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_dependency_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	row, err := h.resolveTenantDepGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	mode := c.FormValue("match_mode")
	if err := h.depGroupSvc.SetMatchMode(ctx, row.ID, mode); err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidMatchMode):
			return echo.NewHTTPError(http.StatusBadRequest, "invalid match mode")
		case errors.Is(err, services.ErrBuiltinMatchModeProtected):
			return echo.NewHTTPError(http.StatusBadRequest, "builtin groups cannot change match mode")
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(row.ID, 10)+"#conditions")
}

// ModuleDependencyAdd links a policy module to a dependency group under a
// given role ("platform" = match any; "requirement" = match all) (Task 7).
func (h *Handler) ModuleDependencyAdd(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_modules"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	moduleID := c.Param("moduleID")
	row, err := h.resolveTenantDepGroup(c, c.FormValue("group_id"))
	if err != nil {
		return err
	}
	role := c.FormValue("role")
	if role != "platform" && role != "requirement" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid role")
	}
	if err := h.depGroupSvc.LinkModule(ctx, sess.TenantID, moduleID, row.ID, role); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/modules")
}

// ModuleDependencyRemove unlinks a policy module from a dependency group
// (Task 7).
func (h *Handler) ModuleDependencyRemove(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_modules"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid group id")
	}
	if err := h.depGroupSvc.UnlinkModule(ctx, sess.TenantID, c.Param("moduleID"), groupID); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/modules")
}

// DependencyGroupConditionRemove removes one condition by its DB id.
func (h *Handler) DependencyGroupConditionRemove(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_dependency_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	row, err := h.resolveTenantDepGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	condID, err := strconv.ParseInt(c.Param("condID"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid condition id")
	}
	if err := h.depGroupSvc.RemoveCondition(ctx, row.ID, condID); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(row.ID, 10)+"#conditions")
}
