package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

// CP2 of the Scripts+Enforcement redesign: handler tests for the
// Scripts tab's add/rename/delete row actions
// (console/handlers/policy_module_scripts.go), mirroring the
// deps/conflicts handler test shape (policy_module_deps_test.go).

func TestPolicyModuleScriptCreate_HappyPath(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-create")
	vidStr := strconv.FormatInt(vid, 10)

	form := url.Values{"name": {"custom-check.sh"}, "language": {"powershell"}}
	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-create/versions/"+vidStr+"/scripts", form, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.scr-create", vidStr})
	if err := h.PolicyModuleScriptCreate(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302: %s", rec.Code, rec.Body.String())
	}

	scripts, err := h.moduleSvc.ListScripts(context.Background(), vid)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sc := range scripts {
		if sc.Name == "custom-check.sh" {
			found = true
			if sc.Language != "powershell" {
				t.Errorf("language = %q, want powershell", sc.Language)
			}
			if sc.Origin != "custom" {
				t.Errorf("origin = %q, want custom", sc.Origin)
			}
		}
	}
	if !found {
		t.Fatal("created script not found in ListScripts")
	}
}

func TestPolicyModuleScriptCreate_InvalidLanguageRejected(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-badlang")
	vidStr := strconv.FormatInt(vid, 10)

	form := url.Values{"name": {"bad.sh"}, "language": {"ruby"}}
	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-badlang/versions/"+vidStr+"/scripts", form, sess)
	c, _ := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.scr-badlang", vidStr})
	err := h.PolicyModuleScriptCreate(c)
	if err == nil {
		t.Fatal("invalid language should be rejected")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400 HTTPError", err)
	}
}

func TestPolicyModuleScriptCreate_NonDraftRejected(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-published")
	vidStr := strconv.FormatInt(vid, 10)

	// INV-M3: publishing requires an apply-phase script/action.
	if _, err := h.moduleSvc.UpsertScript(context.Background(), vid, policymodules.Script{Name: "apply", Language: "sh", Source: "# apply"}); err != nil {
		t.Fatalf("seed apply script: %v", err)
	}
	if err := h.moduleSvc.Publish(context.Background(), vid, ownerID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	form := url.Values{"name": {"late.sh"}, "language": {"sh"}}
	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-published/versions/"+vidStr+"/scripts", form, sess)
	c, _ := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.scr-published", vidStr})
	err := h.PolicyModuleScriptCreate(c)
	if err == nil {
		t.Fatal("create on published version should fail")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400 HTTPError", err)
	}
}

func TestPolicyModuleScriptCreate_ForbiddenWithoutEditPermission(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-forbidden")
	vidStr := strconv.FormatInt(vid, 10)

	otherTenant, err := d.Queries.CreateTenant(context.Background(), db.CreateTenantParams{Name: "Other2", Slug: "other2"})
	if err != nil {
		t.Fatal(err)
	}
	strangerID := newEditorIdentity(t, d, otherTenant.ID, "stranger")
	strangerSess := &auth.UserSession{
		TenantID: otherTenant.ID, IdentityID: strangerID, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}

	form := url.Values{"name": {"x.sh"}, "language": {"sh"}}
	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-forbidden/versions/"+vidStr+"/scripts", form, strangerSess)
	c, _ := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.scr-forbidden", vidStr})
	err = h.PolicyModuleScriptCreate(c)
	if err == nil {
		t.Fatal("stranger should be forbidden")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || (he.Code != http.StatusForbidden && he.Code != http.StatusNotFound) {
		t.Fatalf("error = %v, want 403 or 404 HTTPError", err)
	}
}

func TestPolicyModuleScriptRename_RoundTrip(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-rename")
	vidStr := strconv.FormatInt(vid, 10)

	if _, err := h.moduleSvc.UpsertScript(ctx, vid, policymodules.Script{Name: "old-name.sh", Language: "sh"}); err != nil {
		t.Fatalf("seed script: %v", err)
	}

	form := url.Values{"new_name": {"new-name.sh"}}
	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-rename/versions/"+vidStr+"/scripts/old-name.sh/rename", form, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-rename", vidStr, "old-name.sh"})
	if err := h.PolicyModuleScriptRename(c); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302: %s", rec.Code, rec.Body.String())
	}

	scripts, err := h.moduleSvc.ListScripts(ctx, vid)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, sc := range scripts {
		names = append(names, sc.Name)
	}
	if !contains(names, "new-name.sh") || contains(names, "old-name.sh") {
		t.Fatalf("scripts after rename = %+v, want new-name.sh present and old-name.sh gone", names)
	}
}

func TestPolicyModuleScriptRemove_RoundTrip(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-remove")
	vidStr := strconv.FormatInt(vid, 10)

	if _, err := h.moduleSvc.UpsertScript(ctx, vid, policymodules.Script{Name: "gone.sh", Language: "sh"}); err != nil {
		t.Fatalf("seed script: %v", err)
	}

	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-remove/versions/"+vidStr+"/scripts/gone.sh/delete", url.Values{}, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-remove", vidStr, "gone.sh"})
	if err := h.PolicyModuleScriptRemove(c); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302: %s", rec.Code, rec.Body.String())
	}

	scripts, err := h.moduleSvc.ListScripts(ctx, vid)
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range scripts {
		if sc.Name == "gone.sh" {
			t.Fatal("script should have been deleted")
		}
	}
}

// CP3: handler tests for the standalone script editor page
// (PolicyModuleScriptEdit) and its source-save endpoint
// (PolicyModuleScriptSourceSave).

func getRequest(path string, sess *auth.UserSession) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if sess != nil {
		req = req.WithContext(auth.WithSession(req.Context(), sess))
	}
	return req
}

func TestPolicyModuleScriptEdit_HappyPathDraft(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-edit-draft")
	vidStr := strconv.FormatInt(vid, 10)

	if _, err := h.moduleSvc.UpsertScript(ctx, vid, policymodules.Script{
		Name: "check.ps1", Language: "powershell", Source: "Write-Host 'hi'",
	}); err != nil {
		t.Fatalf("seed script: %v", err)
	}

	req := getRequest("/policy/modules/tenant.acme.scr-edit-draft/versions/"+vidStr+"/scripts/check.ps1/edit", sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-edit-draft", vidStr, "check.ps1"})
	if err := h.PolicyModuleScriptEdit(c); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`value="check.ps1"`,
		`id="mse-language"`,
		`value="powershell" selected`,
		`id="pm-param-tree"`,
		`id="pm-param-tree-body"`,
		`id="pm-param-search"`,
		`id="mse-source"`,
		`Write-Host &#39;hi&#39;`,
		`id="mse-save"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("edit page body missing %q", want)
		}
	}
	if strings.Contains(body, "Read-only") {
		t.Errorf("draft version should not render the read-only badge")
	}
}

func TestPolicyModuleScriptEdit_ReadOnlyWhenNotDraft(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-edit-pub")
	vidStr := strconv.FormatInt(vid, 10)

	if _, err := h.moduleSvc.UpsertScript(ctx, vid, policymodules.Script{Name: "apply", Language: "sh", Source: "# apply"}); err != nil {
		t.Fatalf("seed apply script: %v", err)
	}
	if err := h.moduleSvc.Publish(ctx, vid, ownerID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	req := getRequest("/policy/modules/tenant.acme.scr-edit-pub/versions/"+vidStr+"/scripts/apply/edit", sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-edit-pub", vidStr, "apply"})
	if err := h.PolicyModuleScriptEdit(c); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Read-only") {
		t.Errorf("published version should render the read-only badge")
	}
	if strings.Contains(body, `id="mse-save"`) {
		t.Errorf("published version should not render a Save button")
	}
}

func TestPolicyModuleScriptEdit_MissingScript404(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-edit-missing")
	vidStr := strconv.FormatInt(vid, 10)

	req := getRequest("/policy/modules/tenant.acme.scr-edit-missing/versions/"+vidStr+"/scripts/nope.sh/edit", sess)
	c, _ := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-edit-missing", vidStr, "nope.sh"})
	err := h.PolicyModuleScriptEdit(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound {
		t.Fatalf("error = %v, want 404 HTTPError", err)
	}
}

func TestPolicyModuleScriptEdit_ForbiddenWithoutEditPermission(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-edit-forbidden")
	vidStr := strconv.FormatInt(vid, 10)
	if _, err := h.moduleSvc.UpsertScript(context.Background(), vid, policymodules.Script{Name: "x.sh", Language: "sh"}); err != nil {
		t.Fatalf("seed script: %v", err)
	}

	otherTenant, err := d.Queries.CreateTenant(context.Background(), db.CreateTenantParams{Name: "Other3", Slug: "other3"})
	if err != nil {
		t.Fatal(err)
	}
	strangerID := newEditorIdentity(t, d, otherTenant.ID, "stranger")
	strangerSess := &auth.UserSession{
		TenantID: otherTenant.ID, IdentityID: strangerID, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}

	req := getRequest("/policy/modules/tenant.acme.scr-edit-forbidden/versions/"+vidStr+"/scripts/x.sh/edit", strangerSess)
	c, _ := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-edit-forbidden", vidStr, "x.sh"})
	err = h.PolicyModuleScriptEdit(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || (he.Code != http.StatusForbidden && he.Code != http.StatusNotFound) {
		t.Fatalf("error = %v, want 403 or 404 HTTPError", err)
	}
}

func TestPolicyModuleScriptSourceSave_HappyPath(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-save")
	vidStr := strconv.FormatInt(vid, 10)

	if _, err := h.moduleSvc.UpsertScript(ctx, vid, policymodules.Script{Name: "check.sh", Language: "sh", Source: "# old"}); err != nil {
		t.Fatalf("seed script: %v", err)
	}

	body := map[string]string{"source": "# pluris:params\n# {{ param \"computer/hardware/os_family\" }}\n# pluris:end\n\necho hi", "language": "python"}
	req := jsonRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-save/versions/"+vidStr+"/scripts/check.sh", body, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-save", vidStr, "check.sh"})
	if err := h.PolicyModuleScriptSourceSave(c); err != nil {
		t.Fatalf("save: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	scripts, err := h.moduleSvc.ListScripts(ctx, vid)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sc := range scripts {
		if sc.Name == "check.sh" {
			found = true
			if sc.Language != "python" {
				t.Errorf("language = %q, want python", sc.Language)
			}
			if !strings.Contains(sc.Source, `{{ param "computer/hardware/os_family" }}`) {
				t.Errorf("source missing saved param header: %q", sc.Source)
			}
			if sc.Origin != "custom" {
				t.Errorf("origin = %q, want custom", sc.Origin)
			}
		}
	}
	if !found {
		t.Fatal("saved script not found in ListScripts")
	}
}

func TestPolicyModuleScriptSourceSave_NonDraftRejected(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-save-pub")
	vidStr := strconv.FormatInt(vid, 10)

	if _, err := h.moduleSvc.UpsertScript(ctx, vid, policymodules.Script{Name: "apply", Language: "sh", Source: "# apply"}); err != nil {
		t.Fatalf("seed apply script: %v", err)
	}
	if err := h.moduleSvc.Publish(ctx, vid, ownerID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	body := map[string]string{"source": "# new", "language": "sh"}
	req := jsonRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-save-pub/versions/"+vidStr+"/scripts/apply", body, sess)
	c, _ := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-save-pub", vidStr, "apply"})
	err := h.PolicyModuleScriptSourceSave(c)
	if err == nil {
		t.Fatal("save on published version should fail")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400 HTTPError", err)
	}
	if !strings.Contains(he.Message.(string), "immutable") {
		t.Errorf("message = %v, want mention of immutable draft guard", he.Message)
	}
	_ = services.ErrVersionNotDraft // sanity: the sentinel this path maps from
}

func TestPolicyModuleScriptSourceSave_InvalidLanguageRejected(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	vid := createDepsTestModule(t, h, tenantID, ownerID, "tenant.acme.scr-save-badlang")
	vidStr := strconv.FormatInt(vid, 10)

	if _, err := h.moduleSvc.UpsertScript(ctx, vid, policymodules.Script{Name: "check.sh", Language: "sh"}); err != nil {
		t.Fatalf("seed script: %v", err)
	}

	body := map[string]string{"source": "echo hi", "language": "ruby"}
	req := jsonRequest(http.MethodPost, "/policy/modules/tenant.acme.scr-save-badlang/versions/"+vidStr+"/scripts/check.sh", body, sess)
	c, _ := newEchoCtx(req)
	withParams(c, []string{"id", "vid", "name"}, []string{"tenant.acme.scr-save-badlang", vidStr, "check.sh"})
	err := h.PolicyModuleScriptSourceSave(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400 HTTPError", err)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
