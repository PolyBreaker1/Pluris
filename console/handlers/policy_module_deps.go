package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

// Structured editing for a draft version's module dependencies,
// conflicts, and tests (module_version_conditions) — the Dependencies
// tab rebuild (2026-07-17 spec, Task 3.1). Dependencies/conflicts are
// form-posted picker rows mutating the JSON columns through UpdateDraft
// (draft-guarded); tests go through PolicyModuleService's guarded
// condition CRUD and are authored by the shared ConditionBuilderDialog.

func (h *Handler) moduleVersionEditContext(c echo.Context) (db.PolicyModule, db.PolicyModuleVersion, error) {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantModule(c)
	if err != nil {
		return db.PolicyModule{}, db.PolicyModuleVersion{}, err
	}
	access, err := h.moduleAccessInput(ctx, sess, row)
	if err != nil {
		return db.PolicyModule{}, db.PolicyModuleVersion{}, err
	}
	if !authz.ModuleCanEdit(access) {
		return db.PolicyModule{}, db.PolicyModuleVersion{}, echo.NewHTTPError(http.StatusForbidden, "not allowed to edit this module")
	}
	vrow, err := h.resolveModuleVersion(c, row)
	if err != nil {
		return db.PolicyModule{}, db.PolicyModuleVersion{}, err
	}
	return row, vrow, nil
}

func moduleDepsRedirect(urn string, vid int64, warn string) string {
	u := "/policy/modules/" + urn + "?version=" + strconv.FormatInt(vid, 10)
	if warn != "" {
		u += "&warn=" + url.QueryEscape(warn)
	}
	return u + "#dependencies"
}

// PolicyModuleDependencyAddURN handles POST
// /policy/modules/:id/versions/:vid/deps — appends one structured
// dependency row {module URN, version constraint}.
func (h *Handler) PolicyModuleDependencyAddURN(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	depURN := strings.TrimSpace(c.FormValue("module_urn"))
	constraint := strings.TrimSpace(c.FormValue("version_constraint"))
	if constraint == "" {
		constraint = "*"
	}
	if depURN == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "module_urn is required")
	}
	if depURN == row.ModuleUrn {
		return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, "A module cannot depend on itself."))
	}

	fields := services.FieldsFromVersionRow(vrow)
	for _, dep := range fields.DependsOn {
		if dep.ModuleID == depURN {
			return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, "Dependency already listed: "+depURN))
		}
	}
	fields.DependsOn = append(fields.DependsOn, policymodules.Dependency{ModuleID: depURN, VersionConstraint: constraint})
	if _, err := h.moduleSvc.UpdateDraft(c.Request().Context(), vrow.ID, fields); err != nil {
		return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, "Could not add dependency: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// PolicyModuleDependencyRemoveURN handles POST
// /policy/modules/:id/versions/:vid/deps/remove.
func (h *Handler) PolicyModuleDependencyRemoveURN(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	depURN := strings.TrimSpace(c.FormValue("module_urn"))
	fields := services.FieldsFromVersionRow(vrow)
	kept := fields.DependsOn[:0]
	for _, dep := range fields.DependsOn {
		if dep.ModuleID != depURN {
			kept = append(kept, dep)
		}
	}
	fields.DependsOn = kept
	if _, err := h.moduleSvc.UpdateDraft(c.Request().Context(), vrow.ID, fields); err != nil {
		return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, "Could not remove dependency: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// PolicyModuleConflictAdd handles POST
// /policy/modules/:id/versions/:vid/conflicts.
func (h *Handler) PolicyModuleConflictAdd(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	conflictURN := strings.TrimSpace(c.FormValue("module_urn"))
	if conflictURN == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "module_urn is required")
	}
	fields := services.FieldsFromVersionRow(vrow)
	for _, u := range fields.Conflicts {
		if u == conflictURN {
			return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, "Conflict already listed: "+conflictURN))
		}
	}
	fields.Conflicts = append(fields.Conflicts, conflictURN)
	if _, err := h.moduleSvc.UpdateDraft(c.Request().Context(), vrow.ID, fields); err != nil {
		return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, "Could not add conflict: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// PolicyModuleConflictRemove handles POST
// /policy/modules/:id/versions/:vid/conflicts/remove.
func (h *Handler) PolicyModuleConflictRemove(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	conflictURN := strings.TrimSpace(c.FormValue("module_urn"))
	fields := services.FieldsFromVersionRow(vrow)
	kept := fields.Conflicts[:0]
	for _, u := range fields.Conflicts {
		if u != conflictURN {
			kept = append(kept, u)
		}
	}
	fields.Conflicts = kept
	if _, err := h.moduleSvc.UpdateDraft(c.Request().Context(), vrow.ID, fields); err != nil {
		return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, "Could not remove conflict: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// PolicyModuleConditionAdd handles POST
// /policy/modules/:id/versions/:vid/conditions — the module editor's
// condition:save wiring (same form contract as
// DependencyGroupConditionAdd: repeated values= keys).
func (h *Handler) PolicyModuleConditionAdd(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	formParams, err := c.FormParams()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid form data")
	}
	values := make([]string, len(formParams["values"]))
	for i, v := range formParams["values"] {
		values[i] = strings.TrimSpace(v)
	}
	_, err = h.moduleSvc.AddVersionCondition(c.Request().Context(), vrow.ID,
		c.FormValue("kind"), c.FormValue("param_path"), c.FormValue("operator"),
		values, c.FormValue("script_source"), c.FormValue("script_ref"))
	if err != nil {
		return moduleConditionHTTPError(err)
	}
	return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// PolicyModuleConditionRemove handles POST
// /policy/modules/:id/versions/:vid/conditions/:cid/remove.
func (h *Handler) PolicyModuleConditionRemove(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	cid, err := strconv.ParseInt(c.Param("cid"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid condition id")
	}
	if err := h.moduleSvc.RemoveVersionCondition(c.Request().Context(), vrow.ID, cid); err != nil {
		return moduleConditionHTTPError(err)
	}
	return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// PolicyModuleConditionsMatchMode handles POST
// /policy/modules/:id/versions/:vid/conditions-match-mode.
func (h *Handler) PolicyModuleConditionsMatchMode(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	if err := h.moduleSvc.SetConditionsMatchMode(c.Request().Context(), vrow.ID, c.FormValue("match_mode")); err != nil {
		return moduleConditionHTTPError(err)
	}
	return c.Redirect(http.StatusFound, moduleDepsRedirect(row.ModuleUrn, vrow.ID, ""))
}

func moduleConditionHTTPError(err error) error {
	switch err {
	case services.ErrInvalidConditionKind, services.ErrInvalidOperator,
		services.ErrParamPathRequired, services.ErrScriptSourceRequired,
		services.ErrScriptSourceAmbiguous, services.ErrInvalidMatchMode:
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case services.ErrVersionNotDraft:
		return echo.NewHTTPError(http.StatusBadRequest, "published versions are immutable -- create a draft")
	}
	return err
}

// PolicyModuleVersionDelete handles POST
// /policy/modules/:id/versions/:vid/delete -- draft versions only.
func (h *Handler) PolicyModuleVersionDelete(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	if err := h.moduleSvc.DeleteDraftVersion(c.Request().Context(), vrow.ID); err != nil {
		if err == services.ErrVersionNotDraft {
			return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"?warn="+url.QueryEscape("Only draft versions can be deleted -- revoke published versions instead.")+"#versions")
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"#versions")
}

// PolicyModuleScriptDelete handles POST
// /api/modules/:id/versions/:vid/scripts/delete.
func (h *Handler) PolicyModuleScriptDelete(c echo.Context) error {
	_, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	var req PolicyModuleScriptSaveRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	validPhase := false
	for _, ph := range policymodules.AllLifecyclePhases {
		if string(ph) == req.Phase {
			validPhase = true
			break
		}
	}
	if !validPhase {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid lifecycle phase")
	}
	if err := h.moduleSvc.DeleteScript(c.Request().Context(), vrow.ID, req.Phase); err != nil {
		if err == services.ErrVersionNotDraft {
			return echo.NewHTTPError(http.StatusBadRequest, "published versions are immutable -- create a draft")
		}
		return err
	}
	return c.NoContent(http.StatusOK)
}
