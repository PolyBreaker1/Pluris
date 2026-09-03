package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
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
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
)

// Task 4.3 handler tests for the module detail/create editor.

func newModuleEditorTestDB(t *testing.T) (*Handler, *database.Database, int64) {
	t.Helper()
	d, err := database.Open(filepath.Join(t.TempDir(), "policy_module_editor_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ten, err := d.Queries.CreateTenant(context.Background(), db.CreateTenantParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return New(d), d, ten.ID
}

// newEditorIdentity creates a real identity row so FK-referencing owner/
// published_by columns are satisfiable, and returns its id.
func newEditorIdentity(t *testing.T, d *database.Database, tenantID int64, username string) int64 {
	t.Helper()
	ident, err := d.Queries.CreateIdentity(context.Background(), db.CreateIdentityParams{
		TenantID: tenantID, Username: username, Email: username + "@acme.local",
		DisplayName: username, Role: "admin",
	})
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}
	return ident.ID
}

func adminEditorSession(tenantID, identityID int64) *auth.UserSession {
	return &auth.UserSession{
		TenantID: tenantID, IdentityID: identityID, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}
}

func newEchoCtx(req *http.Request) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func withParams(c echo.Context, names []string, values []string) {
	c.SetParamNames(names...)
	c.SetParamValues(values...)
}

func jsonRequest(method, path string, body any, sess *auth.UserSession) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if sess != nil {
		req = req.WithContext(auth.WithSession(req.Context(), sess))
	}
	return req
}

func formRequest(method, path string, form url.Values, sess *auth.UserSession) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if sess != nil {
		req = req.WithContext(auth.WithSession(req.Context(), sess))
	}
	return req
}

// TestPolicyModuleCreateThenDetail covers the create -> detail 200 round
// trip: creating a tenant module and immediately viewing its detail page.
func TestPolicyModuleCreateThenDetail(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)

	form := url.Values{"urn": {"tenant.acme.demo"}, "title": {"Demo Module"}, "description": {"a demo"}, "scope": {"machine"}}
	req := formRequest(http.MethodPost, "/policy/modules/new", form, sess)
	c, rec := newEchoCtx(req)
	if err := h.PolicyModuleCreateSubmit(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("create status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/policy/modules/tenant.acme.demo" {
		t.Fatalf("redirect = %q, want detail page", loc)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/policy/modules/tenant.acme.demo", nil)
	req2 = req2.WithContext(auth.WithSession(req2.Context(), sess))
	c2, rec2 := newEchoCtx(req2)
	withParams(c2, []string{"id"}, []string{"tenant.acme.demo"})
	if err := h.PolicyModuleDetail(c2); err != nil {
		t.Fatalf("detail: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", rec2.Code)
	}
	if body := rec2.Body.String(); !strings.Contains(body, "Demo Module") {
		t.Errorf("detail page missing title, body did not contain it")
	}
}

// TestPolicyModuleFieldUpdate_Metadata covers the module-level metadata
// inline-edit endpoint (POST /api/modules/:id/fields).
func TestPolicyModuleFieldUpdate_Metadata(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()

	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.meta", "Old Title", "old desc")
	if err != nil {
		t.Fatalf("CreateModule: %v", err)
	}

	req := jsonRequest(http.MethodPost, "/api/modules/tenant.acme.meta/fields",
		FieldUpdateRequest{Section: "meta", Fields: map[string]string{"title": "New Title"}}, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id"}, []string{"tenant.acme.meta"})
	if err := h.PolicyModuleFieldUpdate(c); err != nil {
		t.Fatalf("field update: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	row, err := h.moduleSvc.GetModuleRow(ctx, "tenant.acme.meta")
	if err != nil {
		t.Fatal(err)
	}
	if row.Title != "New Title" {
		t.Fatalf("title = %q, want %q", row.Title, "New Title")
	}
	if row.ID != mod.ID {
		t.Fatalf("sanity: row id mismatch")
	}
}

// TestPolicyModuleScriptSave_DraftGuard exercises the Task 4.3 mandatory
// fix end to end through the handler: a draft version's script saves
// fine; the SAME script save on the now-published version is rejected.
func TestPolicyModuleScriptSave_DraftGuard(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()

	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.scr", "Scr", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := h.moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	vid := strconv.FormatInt(draft.ID, 10)

	body := PolicyModuleScriptSaveRequest{Phase: string(policymodules.PhaseApply), Filename: "enforce.sh", Source: "# apply"}
	req := jsonRequest(http.MethodPost, "/api/modules/tenant.acme.scr/versions/"+vid+"/scripts", body, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.scr", vid})
	if err := h.PolicyModuleScriptSave(c); err != nil {
		t.Fatalf("draft script save: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("draft save status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	if err := h.moduleSvc.Publish(ctx, draft.ID, ownerID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	req2 := jsonRequest(http.MethodPost, "/api/modules/tenant.acme.scr/versions/"+vid+"/scripts", body, sess)
	c2, rec2 := newEchoCtx(req2)
	withParams(c2, []string{"id", "vid"}, []string{"tenant.acme.scr", vid})
	err = h.PolicyModuleScriptSave(c2)
	if err == nil {
		t.Fatal("script save on published version should fail")
	}
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("published script save error = %v, want 400 HTTPError", err)
	}
	_ = rec2
}

// TestPolicyModulePublish_RequiresApplyScript asserts the handler surfaces
// INV-M3's error (via redirect + warn) rather than a 500 when publishing
// a version with no apply script.
func TestPolicyModulePublish_RequiresApplyScript(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()

	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.noapply", "No Apply", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := h.moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	vid := strconv.FormatInt(draft.ID, 10)

	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.noapply/versions/"+vid+"/publish", url.Values{}, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.noapply", vid})
	if err := h.PolicyModuleVersionPublish(c); err != nil {
		t.Fatalf("publish handler: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (redirect with warn)", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "warn=") {
		t.Errorf("redirect missing warn param: %s", rec.Header().Get("Location"))
	}

	v, err := h.db.Queries.GetPolicyModuleVersion(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "draft" {
		t.Fatalf("version state = %q, want still draft after rejected publish", v.State)
	}
}

// TestPolicyModuleClone_BundledToTenant covers cloning a bundled module:
// the clone is tenant-owned by the calling session's identity, with the
// SAME title (plus a clone suffix) and a fresh draft version.
func TestPolicyModuleClone_BundledToTenant(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "cloner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()

	if err := h.moduleSvc.SeedBundled(ctx); err != nil {
		t.Fatalf("seed bundled: %v", err)
	}
	const bundledURN = "pluris.sshd.password-auth-disable"

	req := formRequest(http.MethodPost, "/policy/modules/"+bundledURN+"/clone", url.Values{}, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id"}, []string{bundledURN})
	if err := h.PolicyModuleClone(c); err != nil {
		t.Fatalf("clone: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/policy/modules/"+bundledURN+".clone-") {
		t.Fatalf("redirect = %q, want a clone URN under the bundled one", loc)
	}
	newURN := strings.TrimPrefix(loc, "/policy/modules/")

	row, err := h.moduleSvc.GetModuleRow(ctx, newURN)
	if err != nil {
		t.Fatalf("clone row not found: %v", err)
	}
	if row.IsBundled {
		t.Error("clone must not be bundled")
	}
	if !row.TenantID.Valid || row.TenantID.Int64 != tenantID {
		t.Errorf("clone tenant_id = %+v, want %d", row.TenantID, tenantID)
	}
	if !row.OwnerIdentityID.Valid || row.OwnerIdentityID.Int64 != ownerID {
		t.Errorf("clone owner = %+v, want %d", row.OwnerIdentityID, ownerID)
	}

	versions, err := h.db.Queries.ListVersionsByModule(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].State != "draft" {
		t.Fatalf("clone versions = %+v, want exactly one draft", versions)
	}
}

// TestPolicyModuleDelete_ReferencedSoftDeletes locks the rule that reference
// guards apply only at the permanent-delete boundary.
func TestPolicyModuleDelete_ReferencedSoftDeletes(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()

	if _, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.ref", "Ref", ""); err != nil {
		t.Fatal(err)
	}
	// Reference it via a dependency-group link.
	group, err := h.depGroupSvc.Create(ctx, tenantID, "Blocking Group", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.depGroupSvc.LinkModule(ctx, tenantID, "tenant.acme.ref", group.ID, "requirement"); err != nil {
		t.Fatal(err)
	}
	req := formRequest(http.MethodPost, "/policy/modules/tenant.acme.ref/delete", url.Values{}, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id"}, []string{"tenant.acme.ref"})
	if err := h.PolicyModuleDeleteSubmit(c); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if rec.Header().Get("Location") != "/policy/modules" {
		t.Errorf("redirect = %s, want module list", rec.Header().Get("Location"))
	}
	if _, err := h.moduleSvc.GetModuleRow(ctx, "tenant.acme.ref"); !errors.Is(err, services.ErrModuleNotFound) {
		t.Fatalf("default module read = %v, want hidden", err)
	}
	row, err := d.Queries.GetPolicyModuleByURNIncludingDeleted(ctx, "tenant.acme.ref")
	if err != nil || row.DeletedAt == nil {
		t.Fatalf("referenced module was not soft deleted: %+v, %v", row, err)
	}
}

// TestPolicyModuleDetail_StrangerForbidden covers the 403/404 path: a
// session from a DIFFERENT tenant (no manage_modules bypass, no
// ownership, no explicit module_grants row) must not see a tenant
// module -- mirrors resolveTenantIdentity's cross-tenant 404 shape.
func TestPolicyModuleDetail_StrangerForbidden(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "owner")
	ownerSess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()

	if _, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.private", "Private", ""); err != nil {
		t.Fatal(err)
	}

	otherTenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Other", Slug: "other"})
	if err != nil {
		t.Fatal(err)
	}
	strangerID := newEditorIdentity(t, d, otherTenant.ID, "stranger")
	strangerSess := &auth.UserSession{
		TenantID: otherTenant.ID, IdentityID: strangerID, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}

	req := httptest.NewRequest(http.MethodGet, "/policy/modules/tenant.acme.private", nil)
	req = req.WithContext(auth.WithSession(req.Context(), strangerSess))
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id"}, []string{"tenant.acme.private"})
	err = h.PolicyModuleDetail(c)
	if err == nil {
		t.Fatal("stranger should be denied")
	}
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusNotFound {
		t.Fatalf("stranger error = %v, want 404 HTTPError", err)
	}
	_ = rec
	_ = ownerSess
}

// TestPolicyModuleVersionFieldUpdate_ParametersRoundTrip covers the
// Parameters tab's save mechanism: POSTing a JSON Schema string through
// the version-fields endpoint persists it, and it reads back unchanged
// via GetModule/GetPolicyModuleVersion.
func TestPolicyModuleVersionFieldUpdate_ParametersRoundTrip(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()

	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.params", "Params", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := h.moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	vid := strconv.FormatInt(draft.ID, 10)

	schema := `{"type":"object","properties":{"timeout":{"type":"number","default":30}},"required":["timeout"]}`
	req := jsonRequest(http.MethodPost, "/api/modules/tenant.acme.params/versions/"+vid+"/fields",
		FieldUpdateRequest{Section: "parameters", Fields: map[string]string{"parameters_schema": schema}}, sess)
	c, rec := newEchoCtx(req)
	withParams(c, []string{"id", "vid"}, []string{"tenant.acme.params", vid})
	if err := h.PolicyModuleVersionFieldUpdate(c); err != nil {
		t.Fatalf("version field update: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	v, err := h.db.Queries.GetPolicyModuleVersion(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.ParametersSchema.String != schema {
		t.Fatalf("parameters_schema = %q, want %q", v.ParametersSchema.String, schema)
	}
}
