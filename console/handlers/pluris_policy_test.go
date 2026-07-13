package handlers

import (
	"context"
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
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/database"
)

// setupPlurisTestDB opens a fresh temp-file database, creates a tenant,
// and seeds builtin roles + their Pluris Policy grant templates -- the
// baseline every Pluris Policy handler test starts from.
func setupPlurisTestDB(t *testing.T, dbName, tenantSlug string) (*Handler, int64) {
	t.Helper()
	d, err := database.Open(filepath.Join(t.TempDir(), dbName))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: tenantSlug, Slug: tenantSlug})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	if err := h.roleSvc.EnsureBuiltins(ctx, tenant.ID); err != nil {
		t.Fatalf("ensure builtins: %v", err)
	}
	if err := h.authzSvc.EnsureBuiltinGrants(ctx, tenant.ID); err != nil {
		t.Fatalf("ensure builtin grants: %v", err)
	}
	return h, tenant.ID
}

// adminSession returns a full-access session for tenantID (admin
// template), suitable as the "allowed" actor across these tests.
func adminSession(tenantID int64) *auth.UserSession {
	return &auth.UserSession{
		TenantID: tenantID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}
}

// userSession returns a deny-most session (user template), the "denied"
// actor for RBAC checks.
func userSession(tenantID int64) *auth.UserSession {
	return &auth.UserSession{
		TenantID: tenantID, IdentityID: 2, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}
}

func newFormReq(method, target string, form url.Values) *http.Request {
	var body strings.Reader
	if form != nil {
		body = *strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, &body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	return req
}

func mustHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("want *echo.HTTPError(%d), got %v (%T)", want, err, err)
	}
	if he.Code != want {
		t.Fatalf("status = %d, want %d (%v)", he.Code, want, he.Message)
	}
}

// (a) Clone: technician -> new custom role exists with copied grants.
func TestPlurisPolicyClone(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_clone_test.db", "clone-tenant")
	ctx := context.Background()
	e := echo.New()

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}

	form := url.Values{"name": {"My Custom Tech"}}
	req := newFormReq(http.MethodPost, "/policy/pluris/x/clone", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(tech.ID, 10))

	if err := h.PlurisPolicyClone(c); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("clone status = %d, want 302", rec.Code)
	}

	roles, err := h.db.Queries.ListRolesByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	var clone *db.Role
	for i := range roles {
		if roles[i].Name == "My Custom Tech" {
			clone = &roles[i]
		}
	}
	if clone == nil {
		t.Fatal("cloned role not found")
	}
	if clone.IsBuiltin {
		t.Error("cloned role should not be builtin")
	}
	if clone.Permissions != tech.Permissions {
		t.Errorf("cloned permissions %q != source %q", clone.Permissions, tech.Permissions)
	}
}

// (b) Save matrix on custom role persists; unknown keys are never written.
func TestPlurisPolicySaveMatrix(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_save_test.db", "save-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	clone, err := h.authzSvc.CloneRole(ctx, tenantID, tech.ID, "Save Target")
	if err != nil {
		t.Fatalf("clone role: %v", err)
	}

	form := url.Values{}
	for _, key := range permissions.AllKeys() {
		action := permissions.ActionByKey(key)
		if action.Scoped {
			form.Set("perm_"+key, "none")
		}
		// unscoped keys default to absent (=no) unless set below.
	}
	form.Set("perm_identity.view", "own")
	form.Set("perm_console_access.manage_permissions", "yes")
	form.Set("perm_totally.unknown.key", "yes") // must be ignored

	req := newFormReq(http.MethodPost, "/policy/pluris/x", form)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(clone.ID, 10))

	if err := h.PlurisPolicySave(c); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("save status = %d, want 302", rec.Code)
	}

	updated, err := h.db.Queries.GetRole(ctx, clone.ID)
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	grants := authz.Parse(updated.Permissions)
	if grants.ScopeOf("identity.view") != "own" {
		t.Errorf("identity.view = %q, want own", grants.ScopeOf("identity.view"))
	}
	if grants.ScopeOf("console_access.manage_permissions") != "yes" {
		t.Errorf("manage_permissions = %q, want yes", grants.ScopeOf("console_access.manage_permissions"))
	}
	if _, ok := grants["totally.unknown.key"]; ok {
		t.Error("unknown key was written, want dropped")
	}
	if grants.ScopeOf("asset.view") != "none" {
		t.Errorf("asset.view = %q, want none", grants.ScopeOf("asset.view"))
	}

	// Invalid scoped value -> 400, and role permissions left unchanged.
	badForm := url.Values{"perm_identity.view": {"bogus"}}
	req2 := newFormReq(http.MethodPost, "/policy/pluris/x", badForm)
	req2 = req2.WithContext(auth.WithSession(req2.Context(), sess))
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.FormatInt(clone.ID, 10))
	err = h.PlurisPolicySave(c2)
	mustHTTPStatus(t, err, http.StatusBadRequest)

	stillUpdated, err := h.db.Queries.GetRole(ctx, clone.ID)
	if err != nil {
		t.Fatalf("get role after bad save: %v", err)
	}
	if stillUpdated.Permissions != updated.Permissions {
		t.Error("rejected save should not have changed stored permissions")
	}
}

// (c) Save on a builtin role is rejected.
func TestPlurisPolicySaveBuiltinForbidden(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_save_builtin_test.db", "save-builtin-tenant")
	ctx := context.Background()
	e := echo.New()

	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}

	req := newFormReq(http.MethodPost, "/policy/pluris/x", url.Values{})
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(user.ID, 10))

	err = h.PlurisPolicySave(c)
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// (d) Delete rules: builtin -> 400; custom with members -> 400; empty
// custom -> 302.
func TestPlurisPolicyDeleteRules(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_delete_test.db", "delete-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	doDelete := func(id int64) error {
		req := newFormReq(http.MethodPost, "/policy/pluris/x/delete", url.Values{})
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(id, 10))
		err := h.PlurisPolicyDelete(c)
		if err == nil && rec.Code != http.StatusFound {
			t.Fatalf("delete status = %d, want 302", rec.Code)
		}
		return err
	}

	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}
	if err := doDelete(user.ID); err == nil {
		t.Fatal("deleting a builtin should fail")
	} else {
		mustHTTPStatus(t, err, http.StatusBadRequest)
	}

	custom, err := h.authzSvc.CloneRole(ctx, tenantID, user.ID, "Custom With Member")
	if err != nil {
		t.Fatalf("clone role: %v", err)
	}
	member := createTestIdentityForPlurisTest(t, h, tenantID, "member1")
	if err := h.roleSvc.Assign(ctx, member, custom.ID, 0); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	if err := doDelete(custom.ID); err == nil {
		t.Fatal("deleting a role with members should fail")
	} else {
		mustHTTPStatus(t, err, http.StatusBadRequest)
	}

	emptyCustom, err := h.authzSvc.CloneRole(ctx, tenantID, user.ID, "Empty Custom")
	if err != nil {
		t.Fatalf("clone role: %v", err)
	}
	if err := doDelete(emptyCustom.ID); err != nil {
		t.Fatalf("deleting an empty custom role should succeed: %v", err)
	}
}

// TestPlurisPolicyDeleteBlockedByChildren verifies a parent role with a
// child cannot be deleted (its children inherit grants through it, so
// deleting it out from under them would silently drop those grants) --
// deleting the child first clears the block.
func TestPlurisPolicyDeleteBlockedByChildren(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_delete_children_test.db", "delete-children-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	doDelete := func(id int64) error {
		req := newFormReq(http.MethodPost, "/policy/pluris/x/delete", url.Values{})
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(id, 10))
		err := h.PlurisPolicyDelete(c)
		if err == nil && rec.Code != http.StatusFound {
			t.Fatalf("delete status = %d, want 302", rec.Code)
		}
		return err
	}

	parent, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Parent Role", 0)
	if err != nil {
		t.Fatalf("create parent role: %v", err)
	}
	child, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Child Role", parent.ID)
	if err != nil {
		t.Fatalf("create child role: %v", err)
	}
	if err := h.authzSvc.SetRoleParent(ctx, child.ID, parent.ID); err != nil {
		t.Fatalf("set role parent: %v", err)
	}

	if err := doDelete(parent.ID); err == nil {
		t.Fatal("deleting a role with children should fail")
	} else {
		mustHTTPStatus(t, err, http.StatusBadRequest)
	}

	if err := doDelete(child.ID); err != nil {
		t.Fatalf("deleting the childless child should succeed: %v", err)
	}
	if err := doDelete(parent.ID); err != nil {
		t.Fatalf("deleting the now-childless parent should succeed: %v", err)
	}
}

// (e) Self-lockout: a non-super-admin actor holding a custom role whose
// ONLY source of console_access.manage_permissions is that role must not
// be able to save a matrix that drops it.
func TestPlurisPolicySelfLockoutGuard(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_lockout_test.db", "lockout-tenant")
	ctx := context.Background()
	e := echo.New()

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	custom, err := h.authzSvc.CloneRole(ctx, tenantID, tech.ID, "Perm Manager")
	if err != nil {
		t.Fatalf("clone role: %v", err)
	}
	// Grant manage_permissions on the custom role -- its only source for
	// the actor below.
	grants := authz.Parse(custom.Permissions)
	grants["console_access.manage_permissions"] = "yes"
	if err := h.authzSvc.SaveRolePermissions(ctx, custom.ID, grants); err != nil {
		t.Fatalf("seed custom role grants: %v", err)
	}

	actorID := createTestIdentityForPlurisTest(t, h, tenantID, "actor1")
	if err := h.roleSvc.Assign(ctx, actorID, custom.ID, 0); err != nil {
		t.Fatalf("assign custom role to actor: %v", err)
	}

	actorSess := &auth.UserSession{
		TenantID: tenantID, IdentityID: actorID, Role: identities.RoleTechnician,
		// Grants field mirrors what a real login would resolve: at least
		// manage_permissions=yes (from the assigned custom role), enough
		// to pass the handler-level requirePermission gate.
		Grants: authz.Grants{"console_access.manage_permissions": "yes"},
	}

	// Matrix that drops manage_permissions on the actor's own role.
	form := url.Values{}
	for _, key := range permissions.AllKeys() {
		if permissions.ActionByKey(key).Scoped {
			form.Set("perm_"+key, "none")
		}
	}
	// (manage_permissions omitted -> "no")

	req := newFormReq(http.MethodPost, "/policy/pluris/x", form)
	req = req.WithContext(auth.WithSession(req.Context(), actorSess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(custom.ID, 10))

	err = h.PlurisPolicySave(c)
	mustHTTPStatus(t, err, http.StatusBadRequest)

	// Grants must be unchanged.
	after, err := h.db.Queries.GetRole(ctx, custom.ID)
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	if authz.Parse(after.Permissions).ScopeOf("console_access.manage_permissions") != "yes" {
		t.Error("self-lockout guard should have left grants unchanged")
	}
}

// (f) Cross-tenant role id resolves as not-found.
func TestPlurisPolicyCrossTenantRole404(t *testing.T) {
	h, tenantAID := setupPlurisTestDB(t, "pluris_crosstenant_test.db", "cross-tenant-a")
	ctx := context.Background()
	e := echo.New()

	tenantB, err := h.db.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "cross-tenant-b", Slug: "cross-tenant-b"})
	if err != nil {
		t.Fatalf("create tenant b: %v", err)
	}

	roleInA, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantAID, Slug: "user"})
	if err != nil {
		t.Fatalf("get role in tenant a: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/policy/pluris/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenantB.ID, IdentityID: 99, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(roleInA.ID, 10))

	err = h.PlurisPolicyDetail(c)
	mustHTTPStatus(t, err, http.StatusNotFound)
}

// (g) All mutations are 403 for a user-template session.
func TestPlurisPolicyMutationsForbiddenForUser(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_rbac_test.db", "rbac-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := userSession(tenantID)

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	custom, err := h.authzSvc.CloneRole(ctx, tenantID, tech.ID, "RBAC Target")
	if err != nil {
		t.Fatalf("clone role (setup): %v", err)
	}

	newCtx := func(method, target string, form url.Values, id int64) echo.Context {
		req := newFormReq(method, target, form)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(id, 10))
		return c
	}

	if err := h.PlurisPolicyClone(newCtx(http.MethodPost, "/policy/pluris/x/clone", url.Values{"name": {"nope"}}, tech.ID)); err == nil {
		t.Fatal("user clone should be forbidden")
	} else {
		mustHTTPStatus(t, err, http.StatusForbidden)
	}
	if err := h.PlurisPolicySave(newCtx(http.MethodPost, "/policy/pluris/x", url.Values{}, custom.ID)); err == nil {
		t.Fatal("user save should be forbidden")
	} else {
		mustHTTPStatus(t, err, http.StatusForbidden)
	}
	if err := h.PlurisPolicyDelete(newCtx(http.MethodPost, "/policy/pluris/x/delete", url.Values{}, custom.ID)); err == nil {
		t.Fatal("user delete should be forbidden")
	} else {
		mustHTTPStatus(t, err, http.StatusForbidden)
	}
}

// TestPlurisPolicyListAndDetailRender is a smoke test for the two
// read-only handlers: both must render 200 and carry their data-testid
// anchors (the stub templ components Task 7 will replace).
func TestPlurisPolicyListAndDetailRender(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_render_test.db", "render-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	listReq := httptest.NewRequest(http.MethodGet, "/policy/pluris", nil)
	listReq = listReq.WithContext(auth.WithSession(listReq.Context(), sess))
	listRec := httptest.NewRecorder()
	listC := e.NewContext(listReq, listRec)
	if err := h.PlurisPolicy(listC); err != nil {
		t.Fatalf("list render failed: %v", err)
	}
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listRec.Code)
	}
	if !strings.Contains(listRec.Body.String(), `data-testid="page-pluris-policy"`) {
		t.Error("list page missing page-pluris-policy testid")
	}

	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}
	detailReq := httptest.NewRequest(http.MethodGet, "/policy/pluris/x", nil)
	detailReq = detailReq.WithContext(auth.WithSession(detailReq.Context(), sess))
	detailRec := httptest.NewRecorder()
	detailC := e.NewContext(detailReq, detailRec)
	detailC.SetParamNames("id")
	detailC.SetParamValues(strconv.FormatInt(user.ID, 10))
	if err := h.PlurisPolicyDetail(detailC); err != nil {
		t.Fatalf("detail render failed: %v", err)
	}
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", detailRec.Code)
	}
	if !strings.Contains(detailRec.Body.String(), `data-testid="page-pluris-policy-detail"`) {
		t.Error("detail page missing page-pluris-policy-detail testid")
	}
}

// (h) Detail render for a BUILTIN role: permission controls and the
// clone form are disabled/present; no Apply button.
func TestPlurisPolicyDetailRenderBuiltinReadOnly(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_render_builtin_test.db", "render-builtin-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/policy/pluris/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(user.ID, 10))

	if err := h.PlurisPolicyDetail(c); err != nil {
		t.Fatalf("detail render failed: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "disabled") {
		t.Error("builtin role detail should render disabled permission controls")
	}
	if !strings.Contains(body, "Builtin template") {
		t.Error("builtin role detail should render the read-only/clone banner")
	}
	if strings.Contains(body, ">Apply<") {
		t.Error("builtin role detail should NOT render an Apply button")
	}
}

// (i) Detail render for a CUSTOM role: Apply button present, and a scoped
// permission select carries the correct "selected" option for a grant
// set up in the test.
func TestPlurisPolicyDetailRenderCustomMatrix(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_render_custom_test.db", "render-custom-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	custom, err := h.authzSvc.CloneRole(ctx, tenantID, tech.ID, "Render Target")
	if err != nil {
		t.Fatalf("clone role: %v", err)
	}
	grants := authz.Parse(custom.Permissions)
	grants["identity.view"] = "own"
	if err := h.authzSvc.SaveRolePermissions(ctx, custom.ID, grants); err != nil {
		t.Fatalf("seed grants: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/policy/pluris/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(custom.ID, 10))

	if err := h.PlurisPolicyDetail(c); err != nil {
		t.Fatalf("detail render failed: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, ">Apply<") {
		t.Error("custom role detail should render an Apply button")
	}
	if !strings.Contains(body, `name="perm_identity.view"`) {
		t.Error("custom role detail should render the perm_identity.view select")
	}
	if !strings.Contains(body, `<option value="own" selected>Own</option>`) {
		t.Error(`expected identity.view select to have "own" marked selected`)
	}
}

// (j) Members tab: shows a member row when non-empty, and the empty-row
// message otherwise.
func TestPlurisPolicyDetailRenderMembersTab(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_render_members_test.db", "render-members-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}
	custom, err := h.authzSvc.CloneRole(ctx, tenantID, user.ID, "Members Target")
	if err != nil {
		t.Fatalf("clone role: %v", err)
	}

	renderDetail := func(id int64) string {
		req := httptest.NewRequest(http.MethodGet, "/policy/pluris/x", nil)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(id, 10))
		if err := h.PlurisPolicyDetail(c); err != nil {
			t.Fatalf("detail render failed: %v", err)
		}
		return rec.Body.String()
	}

	emptyBody := renderDetail(custom.ID)
	if !strings.Contains(emptyBody, "No members hold this role yet.") {
		t.Error("empty role should render the empty-row message")
	}

	member := createTestIdentityForPlurisTest(t, h, tenantID, "member-render")
	if err := h.roleSvc.Assign(ctx, member, custom.ID, 0); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	populatedBody := renderDetail(custom.ID)
	if !strings.Contains(populatedBody, "member-render") {
		t.Error("populated role should render the member's username")
	}
	if strings.Contains(populatedBody, "No members hold this role yet.") {
		t.Error("populated role should not render the empty-row message")
	}
}

// (k) Create: name-only -> parentless custom role, effective grants are
// deny-all (own permissions "{}").
func TestPlurisPolicyCreateNameOnly(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_create_nameonly_test.db", "create-nameonly-tenant")
	ctx := context.Background()
	e := echo.New()

	form := url.Values{"name": {"Fresh Role"}}
	req := newFormReq(http.MethodPost, "/policy/pluris", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.PlurisPolicyCreate(c); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("create status = %d, want 302", rec.Code)
	}

	roles, err := h.db.Queries.ListRolesByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	var created *db.Role
	for i := range roles {
		if roles[i].Name == "Fresh Role" {
			created = &roles[i]
		}
	}
	if created == nil {
		t.Fatal("created role not found")
	}
	if created.IsBuiltin {
		t.Error("created role should not be builtin")
	}
	if created.ParentRoleID.Valid {
		t.Error("name-only create should be parentless")
	}
	effective, err := h.authzSvc.ResolveRoleMatrix(ctx, *created)
	if err != nil {
		t.Fatalf("resolve matrix: %v", err)
	}
	if effective.Can("console_access.manage_permissions") {
		t.Error("fresh parentless role should be deny-all")
	}

	// Missing name -> 400.
	badReq := newFormReq(http.MethodPost, "/policy/pluris", url.Values{})
	badReq = badReq.WithContext(auth.WithSession(badReq.Context(), adminSession(tenantID)))
	badRec := httptest.NewRecorder()
	badC := e.NewContext(badReq, badRec)
	err = h.PlurisPolicyCreate(badC)
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// (l) Create with parent_id=technician -> effective grants equal the
// technician role's own effective matrix.
func TestPlurisPolicyCreateWithParent(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_create_parent_test.db", "create-parent-tenant")
	ctx := context.Background()
	e := echo.New()

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	techEffective, err := h.authzSvc.ResolveRoleMatrix(ctx, tech)
	if err != nil {
		t.Fatalf("resolve technician matrix: %v", err)
	}

	form := url.Values{"name": {"Child Of Tech"}, "parent_id": {strconv.FormatInt(tech.ID, 10)}}
	req := newFormReq(http.MethodPost, "/policy/pluris", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.PlurisPolicyCreate(c); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("create status = %d, want 302", rec.Code)
	}

	roles, err := h.db.Queries.ListRolesByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	var created *db.Role
	for i := range roles {
		if roles[i].Name == "Child Of Tech" {
			created = &roles[i]
		}
	}
	if created == nil {
		t.Fatal("created role not found")
	}
	if !created.ParentRoleID.Valid || created.ParentRoleID.Int64 != tech.ID {
		t.Fatal("created role should have technician as parent")
	}
	effective, err := h.authzSvc.ResolveRoleMatrix(ctx, *created)
	if err != nil {
		t.Fatalf("resolve matrix: %v", err)
	}
	for key, want := range techEffective {
		if effective[key] != want {
			t.Errorf("effective[%s] = %q, want %q (technician's)", key, effective[key], want)
		}
	}
}

// (m) SetParent: cycle rejected 400; builtin child rejected 400.
func TestPlurisPolicySetParentRejections(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_setparent_test.db", "setparent-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}
	parentless, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Cycle Root", 0)
	if err != nil {
		t.Fatalf("create parentless custom role: %v", err)
	}
	child, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Cycle Child", parentless.ID)
	if err != nil {
		t.Fatalf("create child custom role: %v", err)
	}

	doSetParent := func(roleID, parentID int64) error {
		form := url.Values{"parent_id": {strconv.FormatInt(parentID, 10)}}
		req := newFormReq(http.MethodPost, "/policy/pluris/x/parent", form)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(roleID, 10))
		return h.PlurisPolicySetParent(c)
	}

	// child's parent is already "parentless"; pointing "parentless" at
	// child would create a two-node cycle.
	if err := doSetParent(parentless.ID, child.ID); err == nil {
		t.Fatal("cycle should have been rejected")
	} else {
		mustHTTPStatus(t, err, http.StatusBadRequest)
	}

	// Builtin roles can never have a parent.
	if err := doSetParent(user.ID, tech.ID); err == nil {
		t.Fatal("builtin parent should have been rejected")
	} else {
		mustHTTPStatus(t, err, http.StatusBadRequest)
	}
}

// (n) SaveRoleOverrides on a parented role stores a diff-only (small) raw
// permissions JSON, not the full submitted matrix.
func TestPlurisPolicySaveOverridesDiffOnly(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_save_diff_test.db", "save-diff-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	techEffective, err := h.authzSvc.ResolveRoleMatrix(ctx, tech)
	if err != nil {
		t.Fatalf("resolve technician matrix: %v", err)
	}
	child, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Diff Child", tech.ID)
	if err != nil {
		t.Fatalf("create custom role: %v", err)
	}

	// Submit the FULL effective matrix, unchanged except for one flip.
	form := url.Values{}
	for _, key := range permissions.AllKeys() {
		action := permissions.ActionByKey(key)
		val := techEffective[key]
		if val == "" {
			val = defaultFormValueForKey(key)
		}
		if action.Scoped {
			form.Set("perm_"+key, val)
		} else if val == "yes" {
			form.Set("perm_"+key, "yes")
		}
	}
	// Flip one key so there's exactly one diff entry (technician's template
	// has asset.update = "all"; "own" is a genuine change).
	form.Set("perm_asset.update", "own")

	req := newFormReq(http.MethodPost, "/policy/pluris/x", form)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(child.ID, 10))

	if err := h.PlurisPolicySave(c); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("save status = %d, want 302", rec.Code)
	}

	updated, err := h.db.Queries.GetRole(ctx, child.ID)
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	raw := authz.Parse(updated.Permissions)
	if len(raw) != 1 {
		t.Fatalf("raw stored overrides = %d keys, want 1 (diff-only): %v", len(raw), raw)
	}
	if raw.ScopeOf("asset.update") != "own" {
		t.Errorf("asset.update override = %q, want own", raw.ScopeOf("asset.update"))
	}

	effective, err := h.authzSvc.ResolveRoleMatrix(ctx, updated)
	if err != nil {
		t.Fatalf("resolve matrix: %v", err)
	}
	if effective.ScopeOf("asset.update") != "own" {
		t.Errorf("effective asset.update = %q, want own", effective.ScopeOf("asset.update"))
	}
}

// defaultFormValueForKey mirrors authz.defaultForKey (unexported) for
// building a "no changes" full-matrix form submission in tests.
func defaultFormValueForKey(key string) string {
	if a := permissions.ActionByKey(key); a != nil && a.Scoped {
		return "none"
	}
	return "no"
}

// (o) Self-lockout guard still fires when the actor's only source of
// manage_permissions comes through an INHERITED (parented) role.
func TestPlurisPolicySelfLockoutGuardViaInheritance(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_lockout_inherit_test.db", "lockout-inherit-tenant")
	ctx := context.Background()
	e := echo.New()

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	// Parent grants manage_permissions; child inherits it with no own
	// overrides yet.
	grants := authz.Parse(tech.Permissions)
	grants["console_access.manage_permissions"] = "yes"
	if err := h.authzSvc.SaveRolePermissions(ctx, tech.ID, grants); err != nil {
		t.Fatalf("seed technician grants: %v", err)
	}
	child, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Inherit Lockout", tech.ID)
	if err != nil {
		t.Fatalf("create custom role: %v", err)
	}

	actorID := createTestIdentityForPlurisTest(t, h, tenantID, "inherit-actor")
	if err := h.roleSvc.Assign(ctx, actorID, child.ID, 0); err != nil {
		t.Fatalf("assign custom role to actor: %v", err)
	}

	actorSess := &auth.UserSession{
		TenantID: tenantID, IdentityID: actorID, Role: identities.RoleTechnician,
		Grants: authz.Grants{"console_access.manage_permissions": "yes"},
	}

	// Full matrix that drops manage_permissions as an explicit "no"
	// override on the child -- this should override the inherited "yes".
	form := url.Values{}
	for _, key := range permissions.AllKeys() {
		if permissions.ActionByKey(key).Scoped {
			form.Set("perm_"+key, "none")
		}
	}
	// manage_permissions omitted -> submitted as "no".

	req := newFormReq(http.MethodPost, "/policy/pluris/x", form)
	req = req.WithContext(auth.WithSession(req.Context(), actorSess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(child.ID, 10))

	err = h.PlurisPolicySave(c)
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// (p) List page render (Task 5): toolbar/search/quick-filter infra is
// present, the old list-level clone form is gone (replaced by a Create
// role panel posting to /policy/pluris), the Parent column header
// renders, and an optgroup appears in the parent picker once a custom
// role (with a distinct template family) exists.
func TestPlurisPolicyListRenderToolbarAndCreatePanel(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_list_toolbar_test.db", "list-toolbar-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	if _, err := h.authzSvc.CloneRole(ctx, tenantID, tech.ID, "Custom Tech Clone"); err != nil {
		t.Fatalf("clone role: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/policy/pluris", nil)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.PlurisPolicy(c); err != nil {
		t.Fatalf("list render failed: %v", err)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "data-pluris-filter") {
		t.Error("list page missing data-pluris-filter toolbar infra")
	}
	if !strings.Contains(body, "Create role") {
		t.Error("list page missing \"Create role\" trigger")
	}
	if !strings.Contains(body, `action="/policy/pluris"`) {
		t.Error("create panel form should post to /policy/pluris")
	}
	if strings.Contains(body, "Clone a builtin role") || strings.Contains(body, "pluris-clone-form") {
		t.Error("list-level clone-from-template form should be gone (Clone moved to role detail)")
	}
	if !strings.Contains(body, "<optgroup") {
		t.Error("parent picker should render an optgroup once a distinct-family custom role exists")
	}
	if !strings.Contains(body, ">Parent<") {
		t.Error("list page missing the Parent column header")
	}
}

// (q) Detail render origin badges: an inherited key shows "inherited
// from Technician" + a data-inherited attr on its control; a key with an
// own override shows the "own" badge. Also covers TDD item (5): the
// parent's name renders as a crumb link to its own detail page.
func TestPlurisPolicyDetailRenderOriginBadges(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_origin_badges_test.db", "origin-badges-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	child, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Origin Badge Child", tech.ID)
	if err != nil {
		t.Fatalf("create child role: %v", err)
	}
	// Give the child exactly one own override so both badge kinds appear.
	grants := authz.Parse(child.Permissions)
	grants["asset.update"] = "own"
	if err := h.authzSvc.SaveRolePermissions(ctx, child.ID, grants); err != nil {
		t.Fatalf("seed child override: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/policy/pluris/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(child.ID, 10))

	if err := h.PlurisPolicyDetail(c); err != nil {
		t.Fatalf("detail render failed: %v", err)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "inherited from Technician") {
		t.Error("detail body should show \"inherited from Technician\" for an inherited key")
	}
	if !strings.Contains(body, "data-inherited=") {
		t.Error("detail body should carry data-inherited attrs for reset-to-inherited")
	}
	if !strings.Contains(body, ">own<") {
		t.Error("detail body should show the \"own\" badge for an overridden key")
	}
	if !strings.Contains(body, `href="/policy/pluris/`+strconv.FormatInt(tech.ID, 10)+`"`) {
		t.Error("hero crumb should link to the parent role's own detail page")
	}
	if !strings.Contains(body, "Technician") {
		t.Error("hero crumb should render the parent role's name")
	}
}

// (r) Settings tab: parent selector renders for a custom role, and is
// replaced by static text for a builtin role.
func TestPlurisPolicyDetailRenderParentSelector(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_parent_selector_test.db", "parent-selector-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}
	custom, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Parent Selector Target", 0)
	if err != nil {
		t.Fatalf("create custom role: %v", err)
	}

	renderDetail := func(id int64) string {
		req := httptest.NewRequest(http.MethodGet, "/policy/pluris/x", nil)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(id, 10))
		if err := h.PlurisPolicyDetail(c); err != nil {
			t.Fatalf("detail render failed: %v", err)
		}
		return rec.Body.String()
	}

	customBody := renderDetail(custom.ID)
	if !strings.Contains(customBody, `name="parent_id"`) {
		t.Error("custom role settings tab should render the parent_id selector")
	}

	builtinBody := renderDetail(user.ID)
	if strings.Contains(builtinBody, `name="parent_id"`) {
		t.Error("builtin role settings tab should NOT render a parent_id selector")
	}
	if !strings.Contains(builtinBody, "Template root (builtins have no parent)") {
		t.Error("builtin role settings tab should render the static template-root text")
	}
}

// (s) Rename endpoint: renaming a custom role persists name+description;
// renaming a builtin role is rejected 400.
func TestPlurisPolicyRename(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_rename_test.db", "rename-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}
	custom, err := h.authzSvc.CreateCustomRole(ctx, tenantID, "Original Name", 0)
	if err != nil {
		t.Fatalf("create custom role: %v", err)
	}

	doRename := func(id int64, name, desc string) error {
		form := url.Values{"name": {name}, "description": {desc}}
		req := newFormReq(http.MethodPost, "/policy/pluris/x/settings", form)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(id, 10))
		return h.PlurisPolicyRename(c)
	}

	if err := doRename(custom.ID, "Renamed Role", "a new description"); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	updated, err := h.db.Queries.GetRole(ctx, custom.ID)
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	if updated.Name != "Renamed Role" {
		t.Errorf("name = %q, want %q", updated.Name, "Renamed Role")
	}
	if updated.Description.String != "a new description" {
		t.Errorf("description = %q, want %q", updated.Description.String, "a new description")
	}

	if err := doRename(user.ID, "New Name", ""); err == nil {
		t.Fatal("renaming a builtin role should be rejected")
	} else {
		mustHTTPStatus(t, err, http.StatusBadRequest)
	}
}

// (t) Groups-holding-this-role table renders after AssignRoleToGroup.
func TestPlurisPolicyDetailRenderGroupsHoldingRole(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "pluris_groups_holding_test.db", "groups-holding-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	user, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}
	custom, err := h.authzSvc.CloneRole(ctx, tenantID, user.ID, "Groups Holding Target")
	if err != nil {
		t.Fatalf("clone role: %v", err)
	}

	renderDetail := func() string {
		req := httptest.NewRequest(http.MethodGet, "/policy/pluris/x", nil)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(custom.ID, 10))
		if err := h.PlurisPolicyDetail(c); err != nil {
			t.Fatalf("detail render failed: %v", err)
		}
		return rec.Body.String()
	}

	beforeBody := renderDetail()
	if !strings.Contains(beforeBody, "No groups hold this role yet.") {
		t.Error("role with no groups should render the groups-tab empty state")
	}

	group, err := h.db.Queries.CreateGroup(ctx, db.CreateGroupParams{TenantID: tenantID, Name: "Holding Group", Slug: "holding-group"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := h.db.Queries.AssignRoleToGroup(ctx, db.AssignRoleToGroupParams{GroupID: group.ID, RoleID: custom.ID}); err != nil {
		t.Fatalf("assign role to group: %v", err)
	}

	afterBody := renderDetail()
	if !strings.Contains(afterBody, "Holding Group") {
		t.Error("populated groups table should render the group's name")
	}
	if strings.Contains(afterBody, "No groups hold this role yet.") {
		t.Error("populated groups table should not render the empty-row message")
	}
}

// createTestIdentityForPlurisTest creates a minimal identity in tenantID
// for role-assignment tests.
func createTestIdentityForPlurisTest(t *testing.T, h *Handler, tenantID int64, username string) int64 {
	t.Helper()
	ident, err := h.identitySvc.Create(context.Background(), tenantID, identities.Identity{
		Username:    username,
		Email:       username + "@example.com",
		DisplayName: username,
		Role:        identities.RoleUser,
	})
	if err != nil {
		t.Fatalf("create identity %s: %v", username, err)
	}
	return ident.ID
}
