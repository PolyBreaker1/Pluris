package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

// CP2 of the Scripts+Enforcement redesign: the Scripts tab's
// add/rename/delete row actions. Mirrors moduleVersionEditContext's
// (policy_module_deps.go) resolve-plus-edit-permission-check shape and
// PolicyModuleScriptSave's (policy_module_editor.go) error-mapping --
// same draft guard, same 403/400 split -- reusing CP1's named-script
// service methods (ListScripts/UpsertScript/RenameScript/DeleteScript)
// rather than reimplementing any of their draft-guard or fork-on-edit
// logic.

func moduleScriptsRedirect(urn string, vid int64, warn string) string {
	u := "/policy/modules/" + urn + "?version=" + strconv.FormatInt(vid, 10)
	if warn != "" {
		u += "&warn=" + url.QueryEscape(warn)
	}
	return u + "#scripts"
}

// PolicyModuleScriptCreate handles POST
// /policy/modules/:id/versions/:vid/scripts -- adds a new named script
// (empty source; the standalone editor, CP3, is where source is
// authored) with the given name/language.
func (h *Handler) PolicyModuleScriptCreate(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	lang := c.FormValue("language")
	if lang == "" {
		lang = string(policymodules.LangSh)
	}
	if !policymodules.Language(lang).Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid language: "+lang)
	}

	if _, err := h.moduleSvc.UpsertScript(ctx, vrow.ID, policymodules.Script{Name: name, Language: lang}); err != nil {
		if err == services.ErrVersionNotDraft {
			return echo.NewHTTPError(http.StatusBadRequest, "published versions are immutable -- create a draft")
		}
		return c.Redirect(http.StatusFound, moduleScriptsRedirect(row.ModuleUrn, vrow.ID, "Could not add script: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, moduleScriptsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// PolicyModuleScriptRename handles POST
// /policy/modules/:id/versions/:vid/scripts/:name/rename.
func (h *Handler) PolicyModuleScriptRename(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	oldName := c.Param("name")
	newName := strings.TrimSpace(c.FormValue("new_name"))
	if newName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "new_name is required")
	}

	if err := h.moduleSvc.RenameScript(ctx, vrow.ID, oldName, newName); err != nil {
		if err == services.ErrVersionNotDraft {
			return echo.NewHTTPError(http.StatusBadRequest, "published versions are immutable -- create a draft")
		}
		return c.Redirect(http.StatusFound, moduleScriptsRedirect(row.ModuleUrn, vrow.ID, "Could not rename script: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, moduleScriptsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// PolicyModuleScriptRemove handles POST
// /policy/modules/:id/versions/:vid/scripts/:name/delete. Named
// "Remove" (not "Delete") to avoid colliding with the pre-existing
// phase-based PolicyModuleScriptDelete (policy_module_deps.go), which
// backs the old /api/modules/.../scripts/delete AJAX endpoint --
// unused by the new INV-L Scripts tab's markup but left in place per
// the brief (its cleanup is a deferred Minor for the final review).
func (h *Handler) PolicyModuleScriptRemove(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	name := c.Param("name")
	if err := h.moduleSvc.DeleteScript(ctx, vrow.ID, name); err != nil {
		if err == services.ErrVersionNotDraft {
			return echo.NewHTTPError(http.StatusBadRequest, "published versions are immutable -- create a draft")
		}
		return c.Redirect(http.StatusFound, moduleScriptsRedirect(row.ModuleUrn, vrow.ID, "Could not delete script: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, moduleScriptsRedirect(row.ModuleUrn, vrow.ID, ""))
}

// CP3 of the Scripts+Enforcement redesign: the standalone full-window
// script editor page and its source-save endpoint.
//
// PolicyModuleScriptEdit renders GET
// /policy/modules/:id/versions/:vid/scripts/:name/edit. Per the CP3
// brief this reuses moduleVersionEditContext as-is (NOT a softer
// view-only resolver): a caller lacking edit permission on the module
// entirely still gets moduleVersionEditContext's 403, same as every
// other route in this file. What's new here is that moduleVersionEditContext
// does not itself check the version's draft state (see its doc
// comment in policy_module_deps.go) -- so an edit-permitted caller
// viewing a published/superseded/revoked version reaches this handler
// fine, and the page renders READ-ONLY (no name/language editing, no
// Save, PlurisCodeEditor mounted with readOnly:true) rather than
// erroring, so browsing a shipped script's source still works.
func (h *Handler) PolicyModuleScriptEdit(c echo.Context) error {
	row, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	name := c.Param("name")
	scripts, err := h.moduleSvc.ListScripts(ctx, vrow.ID)
	if err != nil {
		return err
	}
	var script policymodules.Script
	found := false
	for _, sc := range scripts {
		if sc.Name == name {
			script = sc
			found = true
			break
		}
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "script not found")
	}

	data := templates.ModuleScriptEditorData{
		ModuleUrn: row.ModuleUrn,
		VersionID: vrow.ID,
		Name:      script.Name,
		Language:  script.Language,
		Source:    script.Source,
		ReadOnly:  vrow.State != "draft",
		CSRF:      csrfTokenFrom(c),
	}
	return render(c, templates.ModuleScriptEditorPage(data))
}

// moduleScriptSourceSaveBody is the JSON body PolicyModuleScriptSourceSave
// accepts: the standalone editor's Save button posts { source, language }.
type moduleScriptSourceSaveBody struct {
	Source   string `json:"source"`
	Language string `json:"language"`
}

// PolicyModuleScriptSourceSave handles POST
// /policy/modules/:id/versions/:vid/scripts/:name -- the CP3 standalone
// editor's Save action. Draft + edit-permission gated the same as every
// other mutating route in this file; unlike PolicyModuleScriptCreate
// (which adds a NEW script with empty source), this always UpsertScripts
// the named script's source+language, preserving its identity.
func (h *Handler) PolicyModuleScriptSourceSave(c echo.Context) error {
	_, vrow, err := h.moduleVersionEditContext(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	name := c.Param("name")
	var body moduleScriptSourceSaveBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	lang := body.Language
	if lang == "" {
		lang = string(policymodules.LangSh)
	}
	if !policymodules.Language(lang).Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid language: "+lang)
	}

	if _, err := h.moduleSvc.UpsertScript(ctx, vrow.ID, policymodules.Script{
		Name: name, Language: lang, Source: body.Source, Origin: "custom",
	}); err != nil {
		if err == services.ErrVersionNotDraft {
			return echo.NewHTTPError(http.StatusBadRequest, "published versions are immutable -- create a draft")
		}
		return err
	}
	return c.JSON(http.StatusOK, map[string]bool{"ok": true})
}
