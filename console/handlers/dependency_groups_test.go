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

// TestDependencyGroupCreateAndRBAC covers Task 6 (and Task 5's permission
// migration): a plain "user" session is forbidden from creating a
// dependency group (the technician template now grants
// endpoint_policy.manage_dependency_groups, so it can no longer serve as
// the denied actor here), while an admin session succeeds and the group
// shows up via the service's tenant listing.
func TestDependencyGroupCreateAndRBAC(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_handler_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()

	newReq := func() *http.Request {
		form := url.Values{"name": {"My Group"}, "description": {"x"}}
		req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		return req
	}

	// Plain user is forbidden.
	req := newReq()
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 2, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.DependencyGroupCreate(c); err == nil {
		t.Fatal("user create should be forbidden")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("user create: want 403 HTTPError, got %v", err)
	}

	// Admin succeeds.
	req = newReq()
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := h.DependencyGroupCreate(c); err != nil {
		t.Fatalf("admin create failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("admin create status = %d, want 302", rec.Code)
	}

	groups, err := h.depGroupSvc.ListByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list by tenant: %v", err)
	}
	found := false
	for _, g := range groups {
		if g.Name == "My Group" {
			found = true
		}
	}
	if !found {
		t.Fatal("group not created")
	}
}

// TestDependencyGroupsListRenderToolbar (Task 5) covers the list page's
// standardized search + quick-filter toolbar infra: it must carry
// data-pluris-filter and the All/Builtin/Custom quick-filter control,
// same as the Pluris Policy roles list and the pm-toolbar it mirrors.
func TestDependencyGroupsListRenderToolbar(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_list_toolbar_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/policy/dependency-groups", nil)
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.DependencyGroups(c); err != nil {
		t.Fatalf("list render failed: %v", err)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "data-pluris-filter") {
		t.Error("dependency groups list missing data-pluris-filter toolbar infra")
	}
	if !strings.Contains(body, `data-filter-attr="dg-type"`) {
		t.Error("dependency groups list missing the All/Builtin/Custom quick filter")
	}
	if !strings.Contains(body, "New dependency group") {
		t.Error("dependency groups list should keep the existing \"New dependency group\" button")
	}
}

// TestDependencyGroupDeleteBuiltinProtected covers the builtin-protection
// path on Delete: seeding builtins then deleting one must fail with 400,
// while a custom group deletes cleanly.
func TestDependencyGroupDeleteBuiltinProtected(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_delete_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme2", Slug: "acme2"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()

	if err := h.depGroupSvc.EnsureBuiltins(ctx, tenant.ID); err != nil {
		t.Fatalf("ensure builtins: %v", err)
	}
	groups, err := h.depGroupSvc.ListByTenant(ctx, tenant.ID)
	if err != nil || len(groups) == 0 {
		t.Fatalf("expected seeded builtins, got %v / %d", err, len(groups))
	}
	builtinID := groups[0].ID

	adminSess := &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}

	doDelete := func(id int64) (*httptest.ResponseRecorder, error) {
		req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups/x/delete", nil)
		req = req.WithContext(auth.WithSession(req.Context(), adminSess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(id, 10))
		return rec, h.DependencyGroupDelete(c)
	}

	if _, err := doDelete(builtinID); err == nil {
		t.Fatal("deleting a builtin should fail")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("builtin delete: want 400 HTTPError, got %v", err)
	}

	custom, err := h.depGroupSvc.Create(ctx, tenant.ID, "Custom Group", "")
	if err != nil {
		t.Fatalf("create custom group: %v", err)
	}
	rec, err := doDelete(custom.ID)
	if err != nil {
		t.Fatalf("custom delete failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("custom delete status = %d, want 302", rec.Code)
	}
}

// TestDependencyGroupConditionAddValidatesRegistry (Task 1.3) covers the
// ConditionAdd handler's registry-backed validation: an unknown param path
// or an operator outside the eval engine's supported enum must 400 and
// must NOT create a condition row, while a valid registry path + a
// supported operator must succeed and persist.
func TestDependencyGroupConditionAddValidatesRegistry(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_cond_add_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme5", Slug: "acme5"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()

	adminSess := &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}

	group, err := h.depGroupSvc.Create(ctx, tenant.ID, "Test Group", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	post := func(paramPath, operator, values string) (*httptest.ResponseRecorder, error) {
		form := url.Values{"param_path": {paramPath}, "operator": {operator}, "values": {values}}
		req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups/x/conditions", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req = req.WithContext(auth.WithSession(req.Context(), adminSess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(group.ID, 10))
		return rec, h.DependencyGroupConditionAdd(c)
	}

	// Unknown param path rejected.
	if _, err := post("computer/hardware/not_a_real_param", "in", "x"); err == nil {
		t.Fatal("unknown param path should be rejected")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("unknown param path: want 400 HTTPError, got %v", err)
	}

	// Unsupported operator rejected. Task 2.1 widened the eval engine (and
	// this whitelist) from in/not_in/exists to also cover equals/contains/
	// starts_with/gt/matches/etc, so this now uses a key that will never
	// be a real operator rather than "equals" (which the pre-2.1 version
	// of this test rejected, but the widening explicitly makes valid).
	if _, err := post("computer/hardware/os_family", "not_a_real_operator", "linux"); err == nil {
		t.Fatal("unsupported operator should be rejected")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("unsupported operator: want 400 HTTPError, got %v", err)
	}

	// Neither rejected attempt should have created a condition.
	g, err := h.depGroupSvc.Get(ctx, group.ID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if len(g.Conditions) != 0 {
		t.Fatalf("rejected condition adds should not persist, got %d conditions", len(g.Conditions))
	}

	// Valid registry path + supported operator succeeds.
	rec, err := post("computer/hardware/os_family", "in", "linux")
	if err != nil {
		t.Fatalf("valid condition add failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("valid condition add status = %d, want 302", rec.Code)
	}
	g, err = h.depGroupSvc.Get(ctx, group.ID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if len(g.Conditions) != 1 || g.Conditions[0].ParamPath != "computer/hardware/os_family" {
		t.Fatalf("expected 1 condition on computer/hardware/os_family, got %+v", g.Conditions)
	}
}

// TestDependencyGroupDetailMountsConditionBuilder (Task 2.3) replaces
// Task 1.3's TestDependencyGroupDetailRendersRegistryDrivenConditionOptions,
// which asserted the OLD flat condition-add form's registry-driven
// <select>/<optgroup> markup. That form is gone — the Conditions tab now
// opens the reusable @ConditionBuilderDialog (which fetches /api/params
// client-side instead of server-rendering options), so this test guards
// the new contract instead: the dialog + its "Add condition" trigger are
// mounted, the old flat-form markup is fully removed, and an existing
// condition's Edit button carries a server-built, HTML-escaped
// data-cb-prefill JSON payload plus a human-readable row summary.
func TestDependencyGroupDetailMountsConditionBuilder(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_detail_render_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme6", Slug: "acme6"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()

	group, err := h.depGroupSvc.Create(ctx, tenant.ID, "Render Test Group", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := h.depGroupSvc.AddCondition(ctx, group.ID, "computer/hardware/os_family", "in", []string{"linux"}, "param", "", ""); err != nil {
		t.Fatalf("seed param condition: %v", err)
	}
	if err := h.depGroupSvc.AddCondition(ctx, group.ID, "", "exists", nil, "script", "#!/bin/sh\nexit 0", ""); err != nil {
		t.Fatalf("seed script condition: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/policy/dependency-groups/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(group.ID, 10))
	if err := h.DependencyGroupDetail(c); err != nil {
		t.Fatalf("detail render failed: %v", err)
	}
	body := rec.Body.String()

	// Dialog mounted once + its JS include + the Add trigger.
	if !strings.Contains(body, `id="condition-builder"`) {
		t.Error("Conditions tab should mount @ConditionBuilderDialog (missing #condition-builder)")
	}
	if !strings.Contains(body, "/static/condition-builder.js") {
		t.Error("mounting @ConditionBuilderDialog should include condition-builder.js")
	}
	if !strings.Contains(body, "data-condition-builder-open") {
		t.Error("Conditions tab should render a data-condition-builder-open trigger")
	}
	if !strings.Contains(body, ">Add condition</button>") {
		t.Error(`Conditions tab should render an "Add condition" button`)
	}

	// Match-mode control POSTs to the existing route.
	if !strings.Contains(body, "/match-mode") || !strings.Contains(body, "Match all conditions (AND)") {
		t.Error("Conditions tab should render the match-mode control")
	}

	// Old flat form markup is fully gone.
	if strings.Contains(body, `name="param_path"`) {
		t.Error(`old flat condition-add form (select name="param_path") should be removed`)
	}
	if strings.Contains(body, "comma,separated,values") {
		t.Error("old flat condition-add form's values placeholder should be removed")
	}

	// Existing condition rows: human-readable summary + Edit button with
	// HTML-escaped prefill JSON (templ escapes `"` to &#34; in attribute
	// expressions).
	if !strings.Contains(body, "OS family · is any of · linux") {
		t.Errorf("param condition row should render a human-readable summary; body lacks it")
	}
	if !strings.Contains(body, "Script · #!/bin/sh") {
		t.Error("script condition row should render the standardized script summary")
	}
	if !strings.Contains(body, "#!/bin/sh") {
		t.Error("script condition row should render a source excerpt (first line)")
	}
	if !strings.Contains(body, "data-cb-prefill=") {
		t.Error("existing condition rows should render Edit buttons carrying data-cb-prefill")
	}
	if !strings.Contains(body, "&#34;paramPath&#34;:&#34;computer/hardware/os_family&#34;") {
		t.Error("data-cb-prefill should carry the HTML-escaped condition JSON (paramPath)")
	}
	if strings.Contains(body, `data-cb-prefill="{"kind"`) {
		t.Error("data-cb-prefill JSON must be attribute-escaped, not raw")
	}
}

// TestModuleDependencyLinkRoundTrip covers Task 7: linking a module to a
// dependency group, listing that link, then unlinking it, all through the
// service methods reachable from the handler struct (guards the wiring
// ModuleDependencyAdd/Remove rely on).
func TestModuleDependencyLinkRoundTrip(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_module_link_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme3", Slug: "acme3"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	tenantID := tenant.ID

	if err := h.depGroupSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatal(err)
	}
	groups, _ := h.depGroupSvc.ListByTenant(ctx, tenantID)
	var anyLinux int64
	for _, g := range groups {
		if g.Slug == "any-linux" {
			anyLinux = g.ID
		}
	}
	if anyLinux == 0 {
		t.Fatal("any-linux builtin group not found")
	}
	if err := h.depGroupSvc.LinkModule(ctx, tenantID, "pluris.test.mod", anyLinux, "platform"); err != nil {
		t.Fatal(err)
	}
	links, _ := h.depGroupSvc.ListLinksForModule(ctx, tenantID, "pluris.test.mod")
	if len(links) != 1 || links[0].Role != "platform" {
		t.Fatalf("links=%+v", links)
	}
	if err := h.depGroupSvc.UnlinkModule(ctx, tenantID, "pluris.test.mod", anyLinux); err != nil {
		t.Fatal(err)
	}
	links, _ = h.depGroupSvc.ListLinksForModule(ctx, tenantID, "pluris.test.mod")
	if len(links) != 0 {
		t.Fatalf("expected 0 links after unlink, got %d", len(links))
	}
}

// TestModuleDependencyAddHandlerRBAC covers Task 7's RBAC gate: a plain
// "user" session calling ModuleDependencyAdd directly must get 403 (the
// technician template now grants endpoint_policy.manage_modules, so it
// can no longer serve as the denied actor here).
func TestModuleDependencyAddHandlerRBAC(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_module_link_rbac_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme4", Slug: "acme4"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()

	if err := h.depGroupSvc.EnsureBuiltins(ctx, tenant.ID); err != nil {
		t.Fatal(err)
	}
	groups, _ := h.depGroupSvc.ListByTenant(ctx, tenant.ID)
	var anyLinux int64
	for _, g := range groups {
		if g.Slug == "any-linux" {
			anyLinux = g.ID
		}
	}

	form := url.Values{"group_id": {strconv.FormatInt(anyLinux, 10)}, "role": {"platform"}}
	req := httptest.NewRequest(http.MethodPost, "/policy/modules/pluris.test.mod/dependencies", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 2, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("moduleID")
	c.SetParamValues("pluris.test.mod")

	if err := h.ModuleDependencyAdd(c); err == nil {
		t.Fatal("user ModuleDependencyAdd should be forbidden")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("user ModuleDependencyAdd: want 403 HTTPError, got %v", err)
	}
}

// TestDependencyGroupCrossTenantIsolation is a security regression test:
// tenant A must never be able to read, update, delete, or link a policy
// module to a dependency group that belongs to tenant B by guessing its
// integer id. All such by-id handlers must read cross-tenant ids as
// not-found (404), never leak or mutate the row.
func TestDependencyGroupCrossTenantIsolation(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_cross_tenant_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenantA, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "TenantA", Slug: "tenant-a"})
	if err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	tenantB, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "TenantB", Slug: "tenant-b"})
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	h := New(d)
	e := echo.New()

	// Seed builtins in tenant B and grab one of its group ids.
	if err := h.depGroupSvc.EnsureBuiltins(ctx, tenantB.ID); err != nil {
		t.Fatalf("ensure builtins for tenant B: %v", err)
	}
	groupsB, err := h.depGroupSvc.ListByTenant(ctx, tenantB.ID)
	if err != nil || len(groupsB) == 0 {
		t.Fatalf("expected seeded builtins for tenant B, got %v / %d", err, len(groupsB))
	}
	foreignID := groupsB[0].ID

	adminA := &auth.UserSession{
		TenantID: tenantA.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}

	wantNotFound := func(t *testing.T, label string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected error, got nil", label)
		}
		he, ok := err.(*echo.HTTPError)
		if !ok {
			t.Fatalf("%s: want *echo.HTTPError, got %T (%v)", label, err, err)
		}
		if he.Code != http.StatusNotFound {
			t.Fatalf("%s: want 404, got %d", label, he.Code)
		}
	}

	newIDReq := func(method, path string) (*http.Request, *httptest.ResponseRecorder, echo.Context) {
		req := httptest.NewRequest(method, path, nil)
		req = req.WithContext(auth.WithSession(req.Context(), adminA))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(foreignID, 10))
		return req, rec, c
	}

	// DependencyGroupDetail.
	_, _, c := newIDReq(http.MethodGet, "/policy/dependency-groups/x")
	wantNotFound(t, "DependencyGroupDetail", h.DependencyGroupDetail(c))

	// DependencyGroupUpdate.
	_, _, c = newIDReq(http.MethodPost, "/policy/dependency-groups/x")
	wantNotFound(t, "DependencyGroupUpdate", h.DependencyGroupUpdate(c))

	// DependencyGroupDelete.
	_, _, c = newIDReq(http.MethodPost, "/policy/dependency-groups/x/delete")
	wantNotFound(t, "DependencyGroupDelete", h.DependencyGroupDelete(c))

	// Confirm tenant B's group is untouched (still 404s the same way,
	// i.e. no accidental deletion happened).
	if _, err := h.depGroupSvc.Get(ctx, foreignID); err != nil {
		t.Fatalf("tenant B group should still exist after cross-tenant delete attempt: %v", err)
	}

	// ModuleDependencyAdd: a foreign group_id must 404 and must not
	// create a link.
	form := url.Values{"group_id": {strconv.FormatInt(foreignID, 10)}, "role": {"platform"}}
	req := httptest.NewRequest(http.MethodPost, "/policy/modules/pluris.test.mod/dependencies", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req = req.WithContext(auth.WithSession(req.Context(), adminA))
	rec := httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("moduleID")
	c.SetParamValues("pluris.test.mod")
	wantNotFound(t, "ModuleDependencyAdd", h.ModuleDependencyAdd(c))

	links, err := h.depGroupSvc.ListLinksForModule(ctx, tenantA.ID, "pluris.test.mod")
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected no link created via cross-tenant group_id, got %d", len(links))
	}
}
