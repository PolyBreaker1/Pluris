package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

// Task 4.3 — the module detail/create editor page: handlers, routes,
// and the mandatory SetScript draft-guard fix (pkg/services/policymodules.go).
//
// Save-mechanism choice (documented once here, not repeated per route):
//   - Module-level metadata (title/description) saves through
//     POST /api/modules/:id/fields, mirroring field_api.go's
//     {section, fields} JSON contract.
//   - EVERYTHING version-scoped (target_os/scope/satisfies, sandbox
//     profile, parameters_schema, depends_on/conflicts) saves through
//     ONE endpoint, POST /api/modules/:id/versions/:vid/fields, using
//     the same {section, fields} contract with a different section
//     name per tab. This endpoint enforces the version is a draft
//     (mirrors UpdateDraft's ADR-007 guard) before applying any change.
//   - Scripts get their own dedicated endpoint (POST
//     /api/modules/:id/versions/:vid/scripts) per the brief, since script
//     bodies aren't simple key/value fields.
//   - web/static/detail.js's shared per-section inline-edit JS is reused
//     for every section on this page via a small addition -- section.card
//     may set data-save-url to override the path-derived default, so one
//     page can have two different field endpoints without touching the
//     users/assets pages' behavior.

// PolicyModuleNewShow renders the create form (Task 4.3). Creating a
// module requires the same permission as reaching the Library at all.
func (h *Handler) PolicyModuleNewShow(c echo.Context) error {
	if err := requirePermission(c, authz.ActionManageModules); err != nil {
		return err
	}
	return render(c, templates.PolicyModuleNewPage(csrfTokenFrom(c), ""))
}

// PolicyModuleCreateSubmit creates a new tenant-owned module with a
// blank 0.1.0 draft version, owned by the creating identity, and
// redirects to its detail page.
func (h *Handler) PolicyModuleCreateSubmit(c echo.Context) error {
	if err := requirePermission(c, authz.ActionManageModules); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)

	urn := strings.TrimSpace(c.FormValue("urn"))
	title := strings.TrimSpace(c.FormValue("title"))
	description := c.FormValue("description")
	scope := c.FormValue("scope")
	if scope != "machine" && scope != "user" && scope != "both" {
		scope = "machine"
	}
	if urn == "" || title == "" {
		return render(c, templates.PolicyModuleNewPage(csrfTokenFrom(c), "URN and title are required."))
	}

	tenantID := sess.TenantID
	ownerID := sess.IdentityID
	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, urn, title, description)
	if err != nil {
		return render(c, templates.PolicyModuleNewPage(csrfTokenFrom(c), "Could not create module -- the URN may already be in use."))
	}
	if _, err := h.moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "0.1.0", Scope: scope}); err != nil {
		// The module itself was created; surface a warning rather than
		// losing the new module (same non-rollback shape as
		// UserCreateSubmit's second-pass warnings).
		return c.Redirect(http.StatusFound, "/policy/modules/"+urn+"?warn="+url.QueryEscape("Module created, but its initial draft version failed: "+err.Error()))
	}
	return c.Redirect(http.StatusFound, "/policy/modules/"+urn)
}

// PolicyModuleDetail renders the full module editor (Task 4.3's
// flagship page): metadata, version lifecycle, the two-pane scripts
// editor, structured parameters, sandbox, and dependencies -- all on one
// standardized DetailShell page.
func (h *Handler) PolicyModuleDetail(c echo.Context) error {
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

	mod, err := h.moduleSvc.GetModule(ctx, row.ID)
	if err != nil {
		return err
	}
	versionRows, err := h.db.Queries.ListVersionsByModule(ctx, row.ID)
	if err != nil {
		return err
	}

	data := templates.ModuleEditorData{
		Row:       row,
		Module:    mod,
		CanEdit:   authz.ModuleCanEdit(access),
		CanAdmin:  authz.ModuleCanAdmin(access),
		IsBundled: row.IsBundled,
		CSRF:      csrfTokenFrom(c),
		Warn:      c.QueryParam("warn"),
		CreatedAt: row.CreatedAt.Format("2006-01-02"),
	}
	if row.OwnerIdentityID.Valid {
		if owner, err := h.identitySvc.Get(ctx, row.OwnerIdentityID.Int64); err == nil {
			data.OwnerLabel = owner.DisplayName
		}
	}
	if data.OwnerLabel == "" && row.IsBundled {
		data.OwnerLabel = "Pluris (bundled)"
	}

	for _, v := range versionRows {
		scripts, _ := h.db.Queries.ListScriptsForVersion(ctx, v.ID)
		publishedAt := ""
		if v.PublishedAt.Valid {
			publishedAt = v.PublishedAt.Time.Format("2006-01-02")
		}
		data.Versions = append(data.Versions, templates.ModuleVersionRow{
			ID: v.ID, Version: v.Version, State: v.State,
			PublishedAt: publishedAt, PublishedBy: v.PublisherName.String,
			ScriptCount: len(scripts),
		})
	}

	// Selected version: ?version=<vid> if it belongs to this module,
	// else the most-recently-created draft, else the most recently
	// created version of any state, else none.
	var selected *db.ListVersionsByModuleRow
	if vidStr := c.QueryParam("version"); vidStr != "" {
		if vid, perr := strconv.ParseInt(vidStr, 10, 64); perr == nil {
			for i := range versionRows {
				if versionRows[i].ID == vid {
					selected = &versionRows[i]
					break
				}
			}
		}
	}
	if selected == nil {
		for i := range versionRows {
			if versionRows[i].State == "draft" {
				selected = &versionRows[i]
				break
			}
		}
	}
	if selected == nil && len(versionRows) > 0 {
		selected = &versionRows[0]
	}

	if selected != nil {
		vrow, err := h.db.Queries.GetPolicyModuleVersion(ctx, selected.ID)
		if err == nil {
			data.HasSelected = true
			data.SelectedID = vrow.ID
			data.Selected = vrow
			data.IsDraft = vrow.State == "draft"

			var targetOS []policymodules.TargetOS
			_ = json.Unmarshal([]byte(vrow.TargetOs), &targetOS)
			osStrs := make([]string, 0, len(targetOS))
			for _, o := range targetOS {
				osStrs = append(osStrs, string(o))
			}
			data.TargetOSCSV = strings.Join(osStrs, ", ")

			var satisfies []string
			_ = json.Unmarshal([]byte(vrow.Satisfies), &satisfies)
			data.SatisfiesCSV = strings.Join(satisfies, ", ")

			var sandbox policymodules.SandboxProfile
			_ = json.Unmarshal([]byte(vrow.SandboxProfile), &sandbox)
			data.FsReadCSV = strings.Join(sandbox.FsRead, ", ")
			data.FsWriteCSV = strings.Join(sandbox.FsWrite, ", ")
			data.NetEgressCSV = strings.Join(sandbox.NetEgress, ", ")
			data.SandboxUser = sandbox.User

			var deps []policymodules.Dependency
			if vrow.DependsOn.Valid {
				_ = json.Unmarshal([]byte(vrow.DependsOn.String), &deps)
			}
			depStrs := make([]string, 0, len(deps))
			for _, dep := range deps {
				depStrs = append(depStrs, dep.ModuleID)
			}
			data.DependsOnCSV = strings.Join(depStrs, ", ")

			var conflicts []string
			if vrow.Conflicts.Valid {
				_ = json.Unmarshal([]byte(vrow.Conflicts.String), &conflicts)
			}
			data.ConflictsCSV = strings.Join(conflicts, ", ")

			data.ParametersSchemaRaw = vrow.ParametersSchema.String

			scriptRows, err := h.db.Queries.ListScriptsForVersion(ctx, vrow.ID)
			if err != nil {
				return err
			}
			data.Scripts = make(map[string]db.PolicyModuleScript, len(scriptRows))
			for _, sc := range scriptRows {
				data.Scripts[sc.Phase] = sc
			}
		}
	}

	dv, groups, err := h.moduleDepsViewFor(ctx, sess.TenantID, row.ModuleUrn)
	if err != nil {
		return err
	}
	data.Deps = dv
	data.AllDepGroups = groups

	return render(c, templates.PolicyModuleDetailPage(data))
}

// moduleDepsViewFor loads a module's dependency-group links plus the
// full tenant group list (for the "link a group" picker), mirroring
// PolicyModules' inline computation so the Dependencies tab can reuse
// the exact same moduleDepsDetails templ component the Library page
// uses, rather than re-implementing the link/unlink UI (Task 4.3's
// dependency-tab placement choice: reuse in place, not duplicate --
// see the report).
func (h *Handler) moduleDepsViewFor(ctx context.Context, tenantID int64, urn string) (templates.ModuleDepsView, []dependencygroups.Group, error) {
	groups, err := h.depGroupSvc.ListByTenant(ctx, tenantID)
	if err != nil {
		return templates.ModuleDepsView{}, nil, err
	}
	byID := make(map[int64]dependencygroups.Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}
	links, err := h.depGroupSvc.ListLinksForModule(ctx, tenantID, urn)
	if err != nil {
		return templates.ModuleDepsView{}, groups, nil //nolint:nilerr // best-effort, mirrors PolicyModules' `continue` on error
	}
	var dv templates.ModuleDepsView
	for _, l := range links {
		g, ok := byID[l.GroupID]
		if !ok {
			continue
		}
		chip := templates.ModuleDepChip{GroupID: g.ID, Name: g.Name}
		if l.Role == "platform" {
			dv.Platforms = append(dv.Platforms, chip)
		} else {
			dv.Requirements = append(dv.Requirements, chip)
		}
	}
	return dv, groups, nil
}

// PolicyModuleDeleteSubmit deletes a module (blocked when referenced).
// Requires CanAdmin (never true for bundled modules).
func (h *Handler) PolicyModuleDeleteSubmit(c echo.Context) error {
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
	if !authz.ModuleCanAdmin(access) {
		return echo.NewHTTPError(http.StatusForbidden, "not allowed to delete this module")
	}
	if err := h.moduleSvc.DeleteModule(ctx, sess.TenantID, row.ID, row.ModuleUrn); err != nil {
		if err == services.ErrModuleReferenced {
			return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"?warn="+url.QueryEscape("Cannot delete: this module is still referenced (a dependency-group link, binding, or installation)."))
		}
		return err
	}
	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID: sess.TenantID, EntityType: "policy_module", EntityID: row.ID,
		Event: "module_deleted", Detail: sql.NullString{String: row.ModuleUrn, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
	return c.Redirect(http.StatusFound, "/policy/modules")
}

// PolicyModuleFieldUpdate handles POST /api/modules/:id/fields --
// module-level metadata only (title/description). See the file header
// for the save-mechanism split.
func (h *Handler) PolicyModuleFieldUpdate(c echo.Context) error {
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
	if !authz.ModuleCanEdit(access) {
		return echo.NewHTTPError(http.StatusForbidden, "not allowed to edit this module")
	}

	var req FieldUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	title := row.Title
	description := row.Description.String
	var updated []string
	if v, ok := req.Fields["title"]; ok {
		title = v
		updated = append(updated, "title")
	}
	if v, ok := req.Fields["description"]; ok {
		description = v
		updated = append(updated, "description")
	}
	if title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title cannot be empty")
	}
	if _, err := h.moduleSvc.UpdateModule(ctx, row.ID, title, description); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sort.Strings(updated)
	return c.JSON(http.StatusOK, FieldUpdateResponse{Updated: updated})
}

// PolicyModuleVersionFieldUpdate handles POST
// /api/modules/:id/versions/:vid/fields -- every version-scoped field on
// the editor page (general/sandbox/parameters/dependencies sections),
// draft-guarded: mutating a non-draft version is rejected the same way
// UpdateDraft rejects it. See the file header for the full mechanism.
func (h *Handler) PolicyModuleVersionFieldUpdate(c echo.Context) error {
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
	if !authz.ModuleCanEdit(access) {
		return echo.NewHTTPError(http.StatusForbidden, "not allowed to edit this module")
	}
	vrow, err := h.resolveModuleVersion(c, row)
	if err != nil {
		return err
	}
	if vrow.State != "draft" {
		return echo.NewHTTPError(http.StatusBadRequest, "only draft versions can be edited -- create a draft first")
	}

	var req FieldUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	fields := services.FieldsFromVersionRow(vrow)
	var updated []string
	for key, raw := range req.Fields {
		switch key {
		case "target_os":
			var os []policymodules.TargetOS
			for _, s := range splitCSV(raw) {
				os = append(os, policymodules.TargetOS(s))
			}
			fields.TargetOS = os
		case "scope":
			fields.Scope = raw
		case "satisfies":
			fields.Satisfies = splitCSV(raw)
		case "fs_read":
			fields.SandboxProfile.FsRead = splitCSV(raw)
		case "fs_write":
			fields.SandboxProfile.FsWrite = splitCSV(raw)
		case "net_egress":
			fields.SandboxProfile.NetEgress = splitCSV(raw)
		case "sandbox_user":
			fields.SandboxProfile.User = raw
		case "parameters_schema":
			if raw != "" && !json.Valid([]byte(raw)) {
				return echo.NewHTTPError(http.StatusBadRequest, "parameters_schema must be valid JSON")
			}
			fields.ParametersSchema = raw
		case "depends_on":
			var deps []policymodules.Dependency
			for _, s := range splitCSV(raw) {
				deps = append(deps, policymodules.Dependency{ModuleID: s, VersionConstraint: "*"})
			}
			fields.DependsOn = deps
		case "conflicts":
			fields.Conflicts = splitCSV(raw)
		default:
			continue
		}
		updated = append(updated, key)
	}
	if len(updated) == 0 {
		return c.JSON(http.StatusOK, FieldUpdateResponse{Updated: updated})
	}
	if _, err := h.moduleSvc.UpdateDraft(ctx, vrow.ID, fields); err != nil {
		return fieldUpdateHTTPError(err)
	}
	sort.Strings(updated)
	return c.JSON(http.StatusOK, FieldUpdateResponse{Updated: updated})
}

// splitCSV splits a comma-separated field-input list, trimming
// whitespace and dropping empty entries -- the v1 "simple repeatable
// text-input list" shape per the Task 4.3 brief.
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// PolicyModuleVersionCreate handles POST /policy/modules/:id/versions --
// a new draft, either forked from the latest published version or blank.
func (h *Handler) PolicyModuleVersionCreate(c echo.Context) error {
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
	if !authz.ModuleCanEdit(access) {
		return echo.NewHTTPError(http.StatusForbidden, "not allowed to edit this module")
	}
	version := strings.TrimSpace(c.FormValue("version"))
	if version == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "version is required")
	}
	var newVersion db.PolicyModuleVersion
	if c.FormValue("mode") == "blank" {
		newVersion, err = h.moduleSvc.CreateDraft(ctx, row.ID, services.ModuleVersionFields{Version: version, Scope: "machine"})
	} else {
		newVersion, err = h.moduleSvc.ForkLatestPublished(ctx, row.ID, version)
	}
	if err != nil {
		return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"?warn="+url.QueryEscape("Could not create draft: "+err.Error())+"#versions")
	}
	return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"?version="+strconv.FormatInt(newVersion.ID, 10)+"#versions")
}

// PolicyModuleVersionPublish handles POST
// /policy/modules/:id/versions/:vid/publish.
func (h *Handler) PolicyModuleVersionPublish(c echo.Context) error {
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
	if !authz.ModuleCanEdit(access) {
		return echo.NewHTTPError(http.StatusForbidden, "not allowed to edit this module")
	}
	vrow, err := h.resolveModuleVersion(c, row)
	if err != nil {
		return err
	}
	if err := h.moduleSvc.Publish(ctx, vrow.ID, sess.IdentityID); err != nil {
		return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"?warn="+url.QueryEscape("Publish failed: "+err.Error())+"#versions")
	}
	return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"#versions")
}

// PolicyModuleVersionRevoke handles POST
// /policy/modules/:id/versions/:vid/revoke.
func (h *Handler) PolicyModuleVersionRevoke(c echo.Context) error {
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
	if !authz.ModuleCanEdit(access) {
		return echo.NewHTTPError(http.StatusForbidden, "not allowed to edit this module")
	}
	vrow, err := h.resolveModuleVersion(c, row)
	if err != nil {
		return err
	}
	if err := h.moduleSvc.Revoke(ctx, vrow.ID); err != nil {
		return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"?warn="+url.QueryEscape("Revoke failed: "+err.Error())+"#versions")
	}
	return c.Redirect(http.StatusFound, "/policy/modules/"+row.ModuleUrn+"#versions")
}

// PolicyModuleScriptSaveRequest is the JSON body for
// POST /api/modules/:id/versions/:vid/scripts.
type PolicyModuleScriptSaveRequest struct {
	Phase    string `json:"phase"`
	Filename string `json:"filename"`
	Source   string `json:"source"`
}

// PolicyModuleScriptSave handles POST
// /api/modules/:id/versions/:vid/scripts. Draft-guarded by
// PolicyModuleService.SetScript itself (the Task 4.3 mandatory fix) --
// this handler's own draft check is defense in depth so the error
// message can be specific ("published versions are immutable...").
func (h *Handler) PolicyModuleScriptSave(c echo.Context) error {
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
	if !authz.ModuleCanEdit(access) {
		return echo.NewHTTPError(http.StatusForbidden, "not allowed to edit this module")
	}
	vrow, err := h.resolveModuleVersion(c, row)
	if err != nil {
		return err
	}

	var req PolicyModuleScriptSaveRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if _, err := h.moduleSvc.SetScript(ctx, vrow.ID, policymodules.LifecyclePhase(req.Phase), req.Filename, req.Source); err != nil {
		switch err {
		case services.ErrInvalidLifecyclePhase:
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		case services.ErrVersionNotDraft:
			return echo.NewHTTPError(http.StatusBadRequest, "published versions are immutable -- create a draft")
		default:
			return err
		}
	}
	return c.NoContent(http.StatusOK)
}

// PolicyModuleClone handles POST /policy/modules/:id/clone -- creates a
// tenant-owned copy of the source module (bundled or another tenant's),
// owned by the calling identity, and redirects to the new module's
// detail page. Only CanView of the source is required (not CanEdit --
// bundled modules are never editable, cloning is the intended path for
// changing their behavior).
func (h *Handler) PolicyModuleClone(c echo.Context) error {
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
		return echo.NewHTTPError(http.StatusForbidden, "not allowed to view this module")
	}

	src, err := h.moduleSvc.GetModule(ctx, row.ID)
	if err != nil {
		return err
	}
	newURN := row.ModuleUrn + ".clone-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	tenantID := sess.TenantID
	ownerID := sess.IdentityID
	newMod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, newURN, src.Title+" (clone)", src.Description)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "clone failed: "+err.Error())
	}

	// Copy the source's latest version (published preferred, else
	// whatever exists) forward as a fresh draft, scripts included.
	srcVersions, err := h.db.Queries.ListVersionsByModule(ctx, row.ID)
	if err == nil && len(srcVersions) > 0 {
		var srcVersionID int64
		for _, v := range srcVersions {
			if v.State == "published" {
				srcVersionID = v.ID
				break
			}
		}
		if srcVersionID == 0 {
			srcVersionID = srcVersions[0].ID
		}
		srcVersionRow, err := h.db.Queries.GetPolicyModuleVersion(ctx, srcVersionID)
		if err == nil {
			fields := services.FieldsFromVersionRow(srcVersionRow)
			draft, err := h.moduleSvc.CreateDraft(ctx, newMod.ID, fields)
			if err == nil {
				scripts, _ := h.db.Queries.ListScriptsForVersion(ctx, srcVersionID)
				for _, sc := range scripts {
					_, _ = h.db.Queries.UpsertModuleScript(ctx, db.UpsertModuleScriptParams{
						VersionID: draft.ID, Phase: sc.Phase, Filename: sc.Filename, Source: sc.Source, Seq: sc.Seq,
					})
				}
			}
		}
	}

	_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
		TenantID: sess.TenantID, EntityType: "policy_module", EntityID: newMod.ID,
		Event: "module_cloned", Detail: sql.NullString{String: "from " + row.ModuleUrn, Valid: true},
		ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
	})
	return c.Redirect(http.StatusFound, "/policy/modules/"+newURN)
}
