package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

// Task 5.2 — Configuration Group list + create + DetailShell detail
// pages, retiring the ConfigurationGroupDialog popup and the
// catalog/configgroups mock. Modeled directly on
// console/handlers/dependency_groups.go's list/create/detail/field
// shape; see web/templates/config_groups.templ for the page bodies.

// resolveTenantConfigGroup loads a Configuration Group and verifies
// tenant ownership via services.ConfigGroupService.Get, translating its
// sentinel error into a 404 (mirrors resolveTenantDepGroup).
func (h *Handler) resolveTenantConfigGroup(c echo.Context, idRaw string) (db.ConfigurationGroup, error) {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		return db.ConfigurationGroup{}, echo.NewHTTPError(http.StatusBadRequest, "invalid configuration group id")
	}
	row, err := h.configGroupSvc.Get(ctx, sess.TenantID, id)
	if err != nil {
		if errors.Is(err, services.ErrConfigGroupNotFound) {
			return db.ConfigurationGroup{}, echo.NewHTTPError(http.StatusNotFound, "configuration group not found")
		}
		return db.ConfigurationGroup{}, err
	}
	return row, nil
}

// PolicyGroups renders the browsable Configuration Groups list, reading
// real tenant-scoped rows via services.ConfigGroupService (previously
// catalog/configgroups' MockGroups).
func (h *Handler) PolicyGroups(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	groups, err := h.configGroupSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	rows := make([]templates.ConfigGroupRow, 0, len(groups))
	for _, g := range groups {
		assignCount, err := h.configGroupSvc.CountAssignments(ctx, g.ID)
		if err != nil {
			return err
		}
		bindCount, err := h.configGroupSvc.CountBindings(ctx, g.ID)
		if err != nil {
			return err
		}
		rows = append(rows, templates.ConfigGroupRow{
			ID:          g.ID,
			Name:        g.Name,
			Description: g.Description.String,
			Enabled:     !g.Disabled,
			Assignments: assignCount,
			Bindings:    bindCount,
			CreatedAt:   g.CreatedAt,
		})
	}
	return render(c, templates.ConfigGroupsPage(rows))
}

// PolicyGroupNew renders the create form.
func (h *Handler) PolicyGroupNew(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	return render(c, templates.ConfigGroupNewPage(csrfTokenFrom(c), ""))
}

// PolicyGroupCreate creates a new Configuration Group and redirects to
// its detail page.
func (h *Handler) PolicyGroupCreate(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	name := c.FormValue("name")
	if name == "" {
		return render(c, templates.ConfigGroupNewPage(csrfTokenFrom(c), "Name is required"))
	}
	g, err := h.configGroupSvc.Create(ctx, sess.TenantID, name, c.FormValue("description"), true)
	if err != nil {
		return render(c, templates.ConfigGroupNewPage(csrfTokenFrom(c), err.Error()))
	}
	return c.Redirect(http.StatusFound, "/policy/groups/"+strconv.FormatInt(g.ID, 10))
}

// PolicyGroupDetail renders the standardized DetailShell for one
// Configuration Group: General / Assignments / Policy Bindings tabs.
func (h *Handler) PolicyGroupDetail(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}

	assignmentRows, err := h.configGroupSvc.ListAssignments(ctx, sess.TenantID, row.ID)
	if err != nil {
		return err
	}
	assignments := make([]templates.ConfigGroupAssignmentRow, 0, len(assignmentRows))
	for _, a := range assignmentRows {
		assignments = append(assignments, templates.ConfigGroupAssignmentRow{
			ID:         a.ID,
			TargetType: a.TargetType,
			TargetID:   a.TargetID,
			Label:      h.targetSvc.LabelFor(ctx, a.TargetType, a.TargetID),
			Priority:   a.Priority,
			Enforced:   a.Enforced,
		})
	}

	// Module candidate index: for every tenant-visible module, its DB
	// row id -> URN mapping (bindings store numeric module_id) and its
	// latest published version's parameters_schema (for the
	// schema-generated binding forms' data island below).
	moduleRows, err := h.db.Queries.ListVisibleModules(ctx, sql.NullInt64{Int64: sess.TenantID, Valid: true})
	if err != nil {
		return err
	}
	idToURN := make(map[int64]string, len(moduleRows))
	for _, m := range moduleRows {
		idToURN[m.ID] = m.ModuleUrn
	}

	bindingRows, err := h.configGroupSvc.ListBindings(ctx, sess.TenantID, row.ID)
	if err != nil {
		return err
	}
	bindings := make([]templates.ConfigGroupBindingRow, 0, len(bindingRows))
	for _, b := range bindingRows {
		moduleURN := ""
		if b.ModuleID.Valid {
			moduleURN = idToURN[b.ModuleID.Int64]
		}
		bindings = append(bindings, templates.ConfigGroupBindingRow{
			ID:                  b.ID,
			PolicyURN:           b.PolicyUrn,
			State:               b.State,
			ModuleTitle:         b.ModuleTitle.String,
			ModuleURN:           moduleURN,
			ModuleID:            b.ModuleID,
			ModuleVersionPinned: b.ModuleVersionPinned.String,
			ParameterValuesJSON: b.ParameterValues.String,
		})
	}

	candidatesJSON, err := h.buildPolicyCandidatesJSON(ctx, sess.TenantID, moduleRows)
	if err != nil {
		return err
	}

	// Every catalog policy, for the "Add binding" select -- the same
	// data source the retired policy-picker page's confirm form used
	// (catalog/policies.Catalog()), just rendered as options instead of
	// a second GET/confirm round trip. See config_groups.templ's doc
	// comment on configGroupBindingsTab for why a flat <select> was
	// chosen over reusing PolicyPickerPage wholesale.
	catalog := policies.Catalog()

	targets, err := h.targetSvc.Catalog(ctx, sess.TenantID, configgroups.AllTargetKinds)
	if err != nil {
		return err
	}

	return render(c, templates.ConfigGroupDetailPage(templates.ConfigGroupDetailData{
		Group:                row,
		Assignments:          assignments,
		Bindings:             bindings,
		Catalog:              catalog,
		Targets:              targets,
		PolicyCandidatesJSON: candidatesJSON,
		CSRF:                 csrfTokenFrom(c),
	}))
}

// bindingCandidate is one entry in the detail page's
// #cg-policy-candidates data island (see ConfigGroupDetailData.
// PolicyCandidatesJSON).
type bindingCandidate struct {
	URN    string `json:"urn"`
	Title  string `json:"title"`
	Schema string `json:"schema"`
}

// buildPolicyCandidatesJSON assembles the policy-URN -> candidate-module
// map for the binding forms: every tenant-visible module appears under
// each policy URN its latest published version Satisfies, carrying that
// version's parameters_schema so the client can generate the
// parameter-value inputs. Bundled modules are listed first per policy,
// matching the server-side default-module pick
// (services.ConfigGroupService's schema resolution).
func (h *Handler) buildPolicyCandidatesJSON(ctx context.Context, tenantID int64, moduleRows []db.PolicyModule) (string, error) {
	type moduleInfo struct {
		urn, title, schema string
		bundled            bool
		satisfies          []string
	}
	infos := make([]moduleInfo, 0, len(moduleRows))
	for _, m := range moduleRows {
		ver, err := h.db.Queries.GetLatestPublishedVersion(ctx, m.ID)
		if err != nil {
			// No published version -- not a valid binding candidate.
			continue
		}
		var satisfies []string
		_ = json.Unmarshal([]byte(ver.Satisfies), &satisfies)
		if len(satisfies) == 0 {
			continue
		}
		infos = append(infos, moduleInfo{
			urn: m.ModuleUrn, title: m.Title, schema: ver.ParametersSchema.String,
			bundled: m.IsBundled, satisfies: satisfies,
		})
	}
	out := make(map[string][]bindingCandidate)
	// Two passes: bundled first, then tenant modules, so index 0 per
	// policy is the server's default pick.
	for _, pass := range []bool{true, false} {
		for _, mi := range infos {
			if mi.bundled != pass {
				continue
			}
			for _, urn := range mi.satisfies {
				out[urn] = append(out[urn], bindingCandidate{URN: mi.urn, Title: mi.title, Schema: mi.schema})
			}
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// PolicyGroupDelete deletes a Configuration Group (cascades to its
// bindings/assignments -- see the schema's ON DELETE CASCADE).
func (h *Handler) PolicyGroupDelete(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	if err := h.configGroupSvc.Delete(ctx, sess.TenantID, row.ID); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/groups")
}

// ConfigGroupFieldUpdate handles POST /api/config-groups/:id/fields --
// the General tab's inline-edit endpoint (name/description/enabled),
// following the same FieldUpdateRequest/Response contract as
// UserFieldUpdate/AssetFieldUpdate/PolicyModuleFieldUpdate
// (console/handlers/field_api.go).
func (h *Handler) ConfigGroupFieldUpdate(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}

	var req FieldUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	updated, err := h.configGroupSvc.UpdateFields(ctx, sess.TenantID, row.ID, req.Fields)
	if err != nil {
		return fieldUpdateHTTPError(err)
	}
	sort.Strings(updated)
	h.logFieldUpdateActivity(c, "configuration_group", row.ID, "config_group_updated", req.Section)
	return c.JSON(http.StatusOK, FieldUpdateResponse{Updated: updated})
}

// ---------------------------------------------------------------------
// Assignments tab
// ---------------------------------------------------------------------

// PolicyGroupAssignmentAdd handles POST /policy/groups/:id/assignments.
// The Add-target form (config_groups.templ's configGroupTargetPickerScript)
// submits target_kind + target_ref straight from the TargetPickerDialog's
// `target:pick` event payload -- the handler maps kind -> target_type via
// services.KindToTargetType rather than trusting a client-supplied
// target_type directly.
func (h *Handler) PolicyGroupAssignmentAdd(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}

	targetType, err := services.KindToTargetType(configgroups.TargetKind(c.FormValue("target_kind")))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported target kind")
	}
	targetID, err := strconv.ParseInt(c.FormValue("target_ref"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid target reference")
	}
	priority, _ := strconv.ParseInt(c.FormValue("priority"), 10, 64)
	enforced := c.FormValue("enforced") == "true" || c.FormValue("enforced") == "on"

	if _, err := h.configGroupSvc.AddAssignment(ctx, sess.TenantID, row.ID, targetType, targetID, priority, enforced); err != nil {
		return assignmentHTTPError(err)
	}
	return c.Redirect(http.StatusFound, "/policy/groups/"+strconv.FormatInt(row.ID, 10)+"#assignments")
}

// PolicyGroupAssignmentUpdate handles POST
// /policy/groups/:id/assignments/:aid -- the priority/enforced update
// route (see the task brief's routes section: "either a small POST
// route or fields-API style — pick one, document"). A dedicated route
// was chosen over the fields API because assignments aren't a
// section/dt-dd inline-edit panel, they're table rows with a small
// inline form per row -- the fields API's JSON contract doesn't fit
// that shape as naturally as a plain form POST does.
func (h *Handler) PolicyGroupAssignmentUpdate(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	assignmentID, err := strconv.ParseInt(c.Param("aid"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid assignment id")
	}
	priority, _ := strconv.ParseInt(c.FormValue("priority"), 10, 64)
	enforced := c.FormValue("enforced") == "true" || c.FormValue("enforced") == "on"

	if _, err := h.configGroupSvc.UpdateAssignment(ctx, sess.TenantID, row.ID, assignmentID, priority, enforced); err != nil {
		return assignmentHTTPError(err)
	}
	return c.Redirect(http.StatusFound, "/policy/groups/"+strconv.FormatInt(row.ID, 10)+"#assignments")
}

// PolicyGroupAssignmentRemove handles POST
// /policy/groups/:id/assignments/:aid/remove.
func (h *Handler) PolicyGroupAssignmentRemove(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	assignmentID, err := strconv.ParseInt(c.Param("aid"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid assignment id")
	}
	if err := h.configGroupSvc.RemoveAssignment(ctx, sess.TenantID, row.ID, assignmentID); err != nil {
		return assignmentHTTPError(err)
	}
	return c.Redirect(http.StatusFound, "/policy/groups/"+strconv.FormatInt(row.ID, 10)+"#assignments")
}

// assignmentHTTPError maps a ConfigGroupService assignment error to the
// right HTTP status.
func assignmentHTTPError(err error) error {
	switch {
	case errors.Is(err, services.ErrConfigGroupNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	case errors.Is(err, services.ErrTargetNotFound):
		return echo.NewHTTPError(http.StatusBadRequest, "target not found")
	case errors.Is(err, services.ErrInvalidTargetType):
		return echo.NewHTTPError(http.StatusBadRequest, "invalid target type")
	}
	return err
}

// ---------------------------------------------------------------------
// Policy Bindings tab
// ---------------------------------------------------------------------

// configGroupBindingParamValues reads every "param_<key>" form field
// into a plain map -- the binding form's parameter inputs are named
// with that prefix so they're distinguishable from the form's other
// fields (policy, module, state, version) without a nested/array form
// encoding.
func configGroupBindingParamValues(c echo.Context) (map[string]string, error) {
	formParams, err := c.FormParams()
	if err != nil {
		return nil, err
	}
	const prefix = "param_"
	out := make(map[string]string)
	for key, vals := range formParams {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix && len(vals) > 0 {
			out[key[len(prefix):]] = vals[0]
		}
	}
	return out, nil
}

// PolicyGroupBindingAdd handles POST /policy/groups/:id/bindings.
func (h *Handler) PolicyGroupBindingAdd(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	values, err := configGroupBindingParamValues(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid form data")
	}
	state := c.FormValue("state")
	if state == "" {
		state = "enabled"
	}
	if _, err := h.configGroupSvc.AddBinding(ctx, sess.TenantID, row.ID, c.FormValue("policy_urn"), c.FormValue("module_urn"), c.FormValue("module_version"), values, state); err != nil {
		return bindingHTTPError(err)
	}
	return c.Redirect(http.StatusFound, "/policy/groups/"+strconv.FormatInt(row.ID, 10)+"#bindings")
}

// PolicyGroupBindingUpdate handles POST
// /policy/groups/:id/bindings/:bid.
func (h *Handler) PolicyGroupBindingUpdate(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	bindingID, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid binding id")
	}
	values, err := configGroupBindingParamValues(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid form data")
	}
	state := c.FormValue("state")
	if state == "" {
		state = "enabled"
	}
	if _, err := h.configGroupSvc.UpdateBinding(ctx, sess.TenantID, row.ID, bindingID, c.FormValue("module_urn"), c.FormValue("module_version"), values, state); err != nil {
		return bindingHTTPError(err)
	}
	return c.Redirect(http.StatusFound, "/policy/groups/"+strconv.FormatInt(row.ID, 10)+"#bindings")
}

// PolicyGroupBindingRemove handles POST
// /policy/groups/:id/bindings/:bid/remove.
func (h *Handler) PolicyGroupBindingRemove(c echo.Context) error {
	if err := requirePermission(c, "endpoint_policy.manage_config_groups"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantConfigGroup(c, c.Param("id"))
	if err != nil {
		return err
	}
	bindingID, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid binding id")
	}
	if err := h.configGroupSvc.RemoveBinding(ctx, sess.TenantID, row.ID, bindingID); err != nil {
		return bindingHTTPError(err)
	}
	return c.Redirect(http.StatusFound, "/policy/groups/"+strconv.FormatInt(row.ID, 10)+"#bindings")
}

// bindingHTTPError maps a ConfigGroupService binding error to the right
// HTTP status: validation failures (unknown/missing/mistyped params,
// duplicate policy, unknown override module, bad state) are 400s;
// not-found is 404.
func bindingHTTPError(err error) error {
	switch {
	case errors.Is(err, services.ErrConfigGroupNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	case errors.Is(err, services.ErrFieldValidation), errors.Is(err, services.ErrInvalidState):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return err
}
