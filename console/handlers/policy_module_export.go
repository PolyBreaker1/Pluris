package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

// .pmdl export/import handlers (2026-07-17 spec, Part 4). Export is a
// VIEW-level action (bundled modules included -- they are public
// catalog content); import requires manage_modules and lands on the
// Sources tab.

// PolicyModuleExport handles GET /policy/modules/:id/export
// (?version=<vid>, ?all=1) and streams <urn>.pmdl.
func (h *Handler) PolicyModuleExport(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	row, err := h.resolveTenantModule(c)
	if err != nil {
		return err
	}
	access, err := h.moduleAccessInput(ctx, sess, row)
	if err != nil {
		return err
	}
	if !authz.ModuleCanView(access) {
		return echo.NewHTTPError(http.StatusNotFound, "module not found")
	}

	var explicitVID int64
	if v := c.QueryParam("version"); v != "" {
		explicitVID, _ = strconv.ParseInt(v, 10, 64)
	}
	ids, err := h.moduleSvc.ExportVersionIDs(ctx, row.ID, explicitVID, c.QueryParam("all") == "1")
	if err != nil {
		if errors.Is(err, services.ErrNoExportableVersion) {
			return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"?warn="+url.QueryEscape("Nothing to export: no matching version."))
		}
		return err
	}
	data, err := h.moduleSvc.ExportModuleBytes(ctx, row, ids)
	if err != nil {
		return err
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+row.ModuleUrn+`.pmdl"`)
	return c.Blob(http.StatusOK, "application/gzip", data)
}

// PolicyModuleImport handles POST /policy/modules/import (multipart
// file field "pmdl", optional as_copy=1).
func (h *Handler) PolicyModuleImport(c echo.Context) error {
	if err := requirePermission(c, authz.ActionManageModules); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)

	fh, err := c.FormFile("pmdl")
	if err != nil {
		return c.Redirect(http.StatusFound, "/policy/modules/sources?warn="+url.QueryEscape("Choose a .pmdl file to import."))
	}
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	parsed, err := services.ParsePmdl(f)
	if err != nil {
		return c.Redirect(http.StatusFound, "/policy/modules/sources?warn="+url.QueryEscape("Import failed: "+err.Error()))
	}
	mod, err := h.moduleSvc.ImportModule(ctx, parsed, sess.TenantID, sess.IdentityID, c.FormValue("as_copy") == "1")
	if err != nil {
		if errors.Is(err, services.ErrPmdlURNConflict) {
			return c.Redirect(http.StatusFound, "/policy/modules/sources?warn="+url.QueryEscape("A module with URN "+parsed.Module.URN+" already exists. Re-submit with \"import as copy\" to import under a new URN.")+"&conflict=1")
		}
		return c.Redirect(http.StatusFound, "/policy/modules/sources?warn="+url.QueryEscape("Import failed: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, "/policy/modules/"+mod.ModuleUrn)
}
