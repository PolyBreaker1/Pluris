package handlers

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
)

func createDepsTestModule(t *testing.T, h *Handler, tenantID, ownerID int64, urn string) int64 {
	t.Helper()
	ctx := context.Background()
	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, urn, "M "+urn, "")
	if err != nil {
		t.Fatalf("create module: %v", err)
	}
	draft, err := h.moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	return draft.ID
}

func TestPolicyModuleDependencyAddRemoveConstraintRoundTrip(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.dep-main")
	createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.dep-base")
	vidStr := strconv.FormatInt(vid, 10)

	form := url.Values{"module_urn": {"tenant.acme.dep-base"}, "version_constraint": {">=1.0.0 <2.0.0"}}
	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.dep-main/versions/"+vidStr+"/deps", form, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.dep-main", vidStr})
	if err := h.PolicyModuleDependencyAddURN(c); err != nil {
		t.Fatalf("dep add: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("dep add status = %d", rec.Code)
	}

	vrow, err := d.Queries.GetPolicyModuleVersion(context.Background(), vid)
	if err != nil {
		t.Fatal(err)
	}
	fields := services.FieldsFromVersionRow(vrow)
	if len(fields.DependsOn) != 1 || fields.DependsOn[0].ModuleID != "tenant.acme.dep-base" || fields.DependsOn[0].VersionConstraint != ">=1.0.0 <2.0.0" {
		t.Fatalf("constraint not preserved: %+v", fields.DependsOn)
	}

	selfForm := url.Values{"module_urn": {"tenant.acme.dep-main"}}
	req2 := formRequest(http.MethodPost, "/x", selfForm, sess)
	c2, rec2 := newEchoCtx(req2)
	withParams(c2, []string{"id", "vid"}, []string{"tenant.acme.dep-main", vidStr})
	if err := h.PolicyModuleDependencyAddURN(c2); err != nil {
		t.Fatalf("self dep: %v", err)
	}
	if !strings.Contains(rec2.Header().Get("Location"), "cannot+depend+on+itself") {
		t.Fatalf("self-dependency should warn, got %q", rec2.Header().Get("Location"))
	}

	rmForm := url.Values{"module_urn": {"tenant.acme.dep-base"}}
	req3 := formRequest(http.MethodPost, "/x", rmForm, sess)
	c3, _ := newEchoCtx(req3)
	withParams(c3, []string{"id", "vid"}, []string{"tenant.acme.dep-main", vidStr})
	if err := h.PolicyModuleDependencyRemoveURN(c3); err != nil {
		t.Fatalf("dep remove: %v", err)
	}
	vrow, _ = d.Queries.GetPolicyModuleVersion(context.Background(), vid)
	if fields := services.FieldsFromVersionRow(vrow); len(fields.DependsOn) != 0 {
		t.Fatalf("dep not removed: %+v", fields.DependsOn)
	}
}

func TestPolicyModuleConflictAndConditionEndpoints(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.cfl-main")
	vidStr := strconv.FormatInt(vid, 10)

	post := func(handler echo.HandlerFunc, path string, form url.Values, params []string) (*echo.HTTPError, int, string) {
		req := formRequest(http.MethodPost, path, form, sess)
		c, rec := newEchoCtx(req)
		withParams(c, []string{"id", "vid", "cid"}[:len(params)], params)
		err := handler(c)
		if err != nil {
			if he, ok := err.(*echo.HTTPError); ok {
				return he, rec.Code, ""
			}
			t.Fatalf("handler error: %v", err)
		}
		return nil, rec.Code, rec.Header().Get("Location")
	}

	if he, code, _ := post(h.PolicyModuleConflictAdd, "/x", url.Values{"module_urn": {"tenant.acme.other"}}, []string{"tenant.acme.cfl-main", vidStr}); he != nil || code != http.StatusFound {
		t.Fatalf("conflict add: he=%v code=%d", he, code)
	}
	vrow, _ := d.Queries.GetPolicyModuleVersion(context.Background(), vid)
	if fields := services.FieldsFromVersionRow(vrow); len(fields.Conflicts) != 1 || fields.Conflicts[0] != "tenant.acme.other" {
		t.Fatalf("conflict round-trip: %+v", fields.Conflicts)
	}

	condForm := url.Values{
		"kind":          {"command"},
		"script_source": {"uname -r"},
		"operator":      {"contains"},
		"values":        {"3"},
	}
	if he, code, _ := post(h.PolicyModuleConditionAdd, "/x", condForm, []string{"tenant.acme.cfl-main", vidStr}); he != nil || code != http.StatusFound {
		t.Fatalf("condition add: he=%v code=%d", he, code)
	}
	conds, err := h.moduleSvc.ListVersionConditions(context.Background(), vid)
	if err != nil || len(conds) != 1 || conds[0].Kind != "command" {
		t.Fatalf("condition round-trip: n=%d err=%v", len(conds), err)
	}

	badForm := url.Values{"kind": {"script"}, "script_source": {"x"}, "script_ref": {"y"}, "operator": {"exists"}}
	if he, _, _ := post(h.PolicyModuleConditionAdd, "/x", badForm, []string{"tenant.acme.cfl-main", vidStr}); he == nil || he.Code != http.StatusBadRequest {
		t.Fatalf("ambiguous script subject should 400, got %v", he)
	}

	if he, code, _ := post(h.PolicyModuleConditionsMatchMode, "/x", url.Values{"match_mode": {"any"}}, []string{"tenant.acme.cfl-main", vidStr}); he != nil || code != http.StatusFound {
		t.Fatalf("match mode: he=%v code=%d", he, code)
	}
	vrow, _ = d.Queries.GetPolicyModuleVersion(context.Background(), vid)
	if vrow.ConditionsMatchMode != "any" {
		t.Fatalf("match mode not saved: %q", vrow.ConditionsMatchMode)
	}

	cidStr := strconv.FormatInt(conds[0].ID, 10)
	if he, code, _ := post(h.PolicyModuleConditionRemove, "/x", url.Values{}, []string{"tenant.acme.cfl-main", vidStr, cidStr}); he != nil || code != http.StatusFound {
		t.Fatalf("condition remove: he=%v code=%d", he, code)
	}

	// Published version: every mutation must reject.
	if _, err := h.moduleSvc.SetScript(context.Background(), vid, "apply", "apply.sh", "true"); err != nil {
		t.Fatal(err)
	}
	if err := h.moduleSvc.Publish(context.Background(), vid, ownerID); err != nil {
		t.Fatal(err)
	}
	if he, _, _ := post(h.PolicyModuleConditionAdd, "/x", condForm, []string{"tenant.acme.cfl-main", vidStr}); he == nil || he.Code != http.StatusBadRequest {
		t.Fatalf("condition add on published should 400, got %v", he)
	}
	if _, _, loc := post(h.PolicyModuleConflictAdd, "/x", url.Values{"module_urn": {"tenant.acme.x"}}, []string{"tenant.acme.cfl-main", vidStr}); !strings.Contains(loc, "warn=") {
		t.Fatalf("conflict add on published should redirect with warn, got %q", loc)
	}
}

func TestPolicyModuleDepsEndpointsRequireEdit(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.authz-mod")
	vidStr := strconv.FormatInt(vid, 10)

	strangerID := newEditorIdentity(t, d, tenantID, "stranger")
	stranger := &auth.UserSession{TenantID: tenantID, IdentityID: strangerID}

	req := formRequest(http.MethodPost, "/x", url.Values{"module_urn": {"tenant.acme.b"}}, stranger)
	c, _ := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.authz-mod", vidStr})
	err := h.PolicyModuleDependencyAddURN(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || (he.Code != http.StatusForbidden && he.Code != http.StatusNotFound) {
		t.Fatalf("stranger dep add: want 403/404, got %v", err)
	}
}

func TestPolicyModuleDraftDeleteAndScriptDelete(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.del-mod")
	vidStr := strconv.FormatInt(vid, 10)
	ctx := context.Background()

	if _, err := h.moduleSvc.SetScript(ctx, vid, "apply", "apply.sh", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.moduleSvc.SetScript(ctx, vid, "disable", "disable.sh", "true"); err != nil {
		t.Fatal(err)
	}

	req := jsonRequest(http.MethodPost, "/x", map[string]string{"phase": "disable"}, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.del-mod", vidStr})
	if err := h.PolicyModuleScriptDelete(c); err != nil {
		t.Fatalf("script delete: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("script delete status = %d", rec.Code)
	}
	scripts, _ := d.Queries.ListScriptsForVersion(ctx, vid)
	if len(scripts) != 1 || scripts[0].Name != "apply" {
		t.Fatalf("disable script should be gone: %+v", scripts)
	}

	badReq := jsonRequest(http.MethodPost, "/x", map[string]string{"phase": "bogus"}, sess)
	c2, _ := newEchoCtx(badReq)
	withParams(c2, []string{"id", "vid"}, []string{"tenant.acme.del-mod", vidStr})
	if err := h.PolicyModuleScriptDelete(c2); err == nil {
		t.Fatal("bogus phase should 400")
	}

	delReq := formRequest(http.MethodPost, "/x", url.Values{}, sess)
	c3, rec3 := newEchoCtx(delReq)
	withParams(c3, []string{"id", "vid"}, []string{"tenant.acme.del-mod", vidStr})
	if err := h.PolicyModuleVersionDelete(c3); err != nil {
		t.Fatalf("draft delete: %v", err)
	}
	if rec3.Code != http.StatusFound {
		t.Fatalf("draft delete status = %d", rec3.Code)
	}
	if _, err := d.Queries.GetPolicyModuleVersion(ctx, vid); err == nil {
		t.Fatal("draft version should be deleted")
	}

	vid2 := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.del-pub")
	vid2Str := strconv.FormatInt(vid2, 10)
	if _, err := h.moduleSvc.SetScript(ctx, vid2, "apply", "apply.sh", "true"); err != nil {
		t.Fatal(err)
	}
	if err := h.moduleSvc.Publish(ctx, vid2, ownerID); err != nil {
		t.Fatal(err)
	}
	delReq2 := formRequest(http.MethodPost, "/x", url.Values{}, sess)
	c4, rec4 := newEchoCtx(delReq2)
	withParams(c4, []string{"id", "vid"}, []string{"tenant.acme.del-pub", vid2Str})
	if err := h.PolicyModuleVersionDelete(c4); err != nil {
		t.Fatalf("published delete handler error: %v", err)
	}
	if !strings.Contains(rec4.Header().Get("Location"), "warn=") {
		t.Fatalf("published delete should warn, got %q", rec4.Header().Get("Location"))
	}
	if _, err := d.Queries.GetPolicyModuleVersion(ctx, vid2); err != nil {
		t.Fatal("published version must survive delete attempt")
	}
}

func TestPolicyModuleReportSchemaRoundTrip(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.report-mod")
	vidStr := strconv.FormatInt(vid, 10)

	schema := `{"type":"object","properties":{"drift":{"type":"boolean"}}}`
	body := map[string]any{"section": "report_schema", "fields": map[string]string{"report_schema": schema}}
	req := jsonRequest(http.MethodPost, "/x", body, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.report-mod", vidStr})
	if err := h.PolicyModuleVersionFieldUpdate(c); err != nil {
		t.Fatalf("report schema save: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("report schema save status = %d", rec.Code)
	}
	vrow, _ := d.Queries.GetPolicyModuleVersion(context.Background(), vid)
	if vrow.ReportSchema != schema {
		t.Fatalf("report schema round-trip: %q", vrow.ReportSchema)
	}

	bad := map[string]any{"section": "report_schema", "fields": map[string]string{"report_schema": "{not json"}}
	req2 := jsonRequest(http.MethodPost, "/x", bad, sess)
	c2, _ := newEchoCtx(req2)
	withParams(c2, []string{"id", "vid"}, []string{"tenant.acme.report-mod", vidStr})
	if err := h.PolicyModuleVersionFieldUpdate(c2); err == nil {
		t.Fatal("invalid report schema JSON should 400")
	}
}

func TestPolicyModuleExportAndImportHandlers(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.exp-mod")
	ctx := context.Background()
	if _, err := h.moduleSvc.SetScript(ctx, vid, "apply", "apply.sh", "true"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/policy/modules/tenant.acme.exp-mod/export", nil)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id"}, []string{"tenant.acme.exp-mod"})
	if err := h.PolicyModuleExport(c); err != nil {
		t.Fatalf("export: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d", rec.Code)
	}
	if cd := rec.Header().Get(echo.HeaderContentDisposition); !strings.Contains(cd, "tenant.acme.exp-mod.pmdl") {
		t.Fatalf("content disposition = %q", cd)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); !strings.Contains(ct, "application/gzip") {
		t.Fatalf("content type = %q", ct)
	}

	var mpBuf bytes.Buffer
	mw := multipart.NewWriter(&mpBuf)
	fw, err := mw.CreateFormFile("pmdl", "tenant.acme.exp-mod.pmdl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(rec.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	_ = mw.WriteField("as_copy", "1")
	_ = mw.Close()

	req2 := httptest.NewRequest(http.MethodPost, "/policy/modules/import", &mpBuf)
	req2.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	req2 = req2.WithContext(auth.WithSession(req2.Context(), sess))
	c2, rec2 := newEchoCtx(req2)
	if err := h.PolicyModuleImport(c2); err != nil {
		t.Fatalf("import: %v", err)
	}
	if rec2.Code != http.StatusFound {
		t.Fatalf("import status = %d", rec2.Code)
	}
	loc := rec2.Header().Get("Location")
	if !strings.Contains(loc, "tenant.acme.exp-mod-imported-1") {
		t.Fatalf("import redirect = %q", loc)
	}

	imported, err := d.Queries.GetPolicyModuleByURN(ctx, "tenant.acme.exp-mod-imported-1")
	if err != nil {
		t.Fatalf("imported module missing: %v", err)
	}
	if imported.Origin != "imported" {
		t.Fatalf("imported origin = %q", imported.Origin)
	}

	// Cross-tenant export must 404 before any authz decision.
	otherTen, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Other", Slug: "other"})
	if err != nil {
		t.Fatal(err)
	}
	otherID := newEditorIdentity(t, d, otherTen.ID, "otheradmin")
	otherSess := adminEditorSession(otherTen.ID, otherID)
	req3 := httptest.NewRequest(http.MethodGet, "/policy/modules/tenant.acme.exp-mod/export", nil)
	req3 = req3.WithContext(auth.WithSession(req3.Context(), otherSess))
	c3, _ := newEchoCtx(req3)
	withParams(c3, []string{"id"}, []string{"tenant.acme.exp-mod"})
	if err := h.PolicyModuleExport(c3); err == nil {
		t.Fatal("cross-tenant export should error")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant export: want 404, got %v", err)
	}
}
