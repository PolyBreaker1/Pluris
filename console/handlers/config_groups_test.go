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
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
)

// seedCGAsset creates a computer asset and returns its DB id.
func seedCGAsset(t *testing.T, h *Handler, tenantID int64, hostname string) int64 {
	t.Helper()
	a, err := h.db.Queries.CreateAsset(context.Background(), db.CreateAssetParams{
		Uuid: "cg-" + hostname, TenantID: tenantID, Subtype: "computer",
		SubtypePayload: `{"hostname":"` + hostname + `","os_family":"linux"}`, EnrollmentState: "enrolled",
	})
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func seedCGIdentity(t *testing.T, h *Handler, tenantID int64, name string) int64 {
	t.Helper()
	ident, err := h.identitySvc.Create(context.Background(), tenantID, identities.Identity{
		Username: name, Email: name + "@example.com", DisplayName: name, Role: identities.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ident.ID
}

// TestPolicyGroupCreateRedirectsToDetail: POST /policy/groups/new
// creates the group and 302s to its detail page.
func TestPolicyGroupCreateRedirectsToDetail(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "cg_create_test.db", "cg-create-tenant")
	e := echo.New()

	form := url.Values{"name": {"My Group"}, "description": {"desc"}}
	req := newFormReq(http.MethodPost, "/policy/groups/new", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.PolicyGroupCreate(c); err != nil {
		t.Fatalf("PolicyGroupCreate: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/policy/groups/") {
		t.Fatalf("redirect = %q, want /policy/groups/<id>", loc)
	}
	groups, _ := h.configGroupSvc.ListByTenant(context.Background(), tenantID)
	if len(groups) != 1 || groups[0].Name != "My Group" {
		t.Fatalf("group not persisted: %+v", groups)
	}
}

// TestPolicyGroupDetailPage: the detail page mounts the shared
// TargetPickerDialog with real tenant-scoped targets (the Task 5.1
// assertion, relocated from the retired list-page mount), renders the
// three tabs, and carries no retired cg-dialog markup.
func TestPolicyGroupDetailPage(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "cg_detail_test.db", "cg-detail-tenant")
	ctx := context.Background()

	seedCGAsset(t, h, tenantID, "pg-lobby-pc")
	seedCGIdentity(t, h, tenantID, "carol-pg")

	tenantB := createSecondTenant(t, h, "cg-detail-tenant-b")
	seedCGAsset(t, h, tenantB, "tenant-b-only-pc")

	g, err := h.configGroupSvc.Create(ctx, tenantID, "Detail Group", "", true)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/policy/groups/"+strconv.FormatInt(g.ID, 10), nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(g.ID, 10))

	if err := h.PolicyGroupDetail(c); err != nil {
		t.Fatalf("PolicyGroupDetail: %v", err)
	}
	body := rec.Body.String()

	if !strings.Contains(body, `data-testid="page-policy-group-detail"`) {
		t.Fatal("body missing page-policy-group-detail testid")
	}
	if !strings.Contains(body, `id="target-picker"`) {
		t.Fatal("detail page missing the mounted TargetPickerDialog")
	}
	if !strings.Contains(body, "pg-lobby-pc") {
		t.Error("picker missing the seeded asset's hostname (real row not rendered)")
	}
	if !strings.Contains(body, "carol-pg") {
		t.Error("picker missing the seeded identity (real row not rendered)")
	}
	if strings.Contains(body, "tenant-b-only-pc") {
		t.Error("tenant B's asset leaked into tenant A's picker")
	}
	// configuration_group is not a legal assignment target_type: the
	// picker's allow-list must exclude that kind.
	if strings.Contains(body, `data-allowed-kinds="computer,user,computer_group,user_group,configuration_group"`) {
		t.Error("picker allow-list must not include configuration_group")
	}
	if !strings.Contains(body, `data-allowed-kinds="computer,user,computer_group,user_group"`) {
		t.Error("picker allow-list missing/unexpected")
	}
	for _, tab := range []string{`data-tab="general"`, `data-tab="assignments"`, `data-tab="bindings"`} {
		if !strings.Contains(body, tab) {
			t.Errorf("detail page missing tab %s", tab)
		}
	}
	if strings.Contains(body, `id="cg-dialog"`) || strings.Contains(body, "cg:save") {
		t.Error("detail page contains retired cg-dialog markup")
	}
	// Cross-tenant detail read is a 404.
	reqB := httptest.NewRequest(http.MethodGet, "/policy/groups/"+strconv.FormatInt(g.ID, 10), nil)
	reqB = reqB.WithContext(auth.WithSession(reqB.Context(), adminSession(tenantB)))
	recB := httptest.NewRecorder()
	cB := e.NewContext(reqB, recB)
	cB.SetParamNames("id")
	cB.SetParamValues(strconv.FormatInt(g.ID, 10))
	mustHTTPStatus(t, h.PolicyGroupDetail(cB), http.StatusNotFound)
}

// TestPolicyGroupAssignmentRoutes: add (with picker-kind mapping),
// update priority/enforced, and remove.
func TestPolicyGroupAssignmentRoutes(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "cg_assign_test.db", "cg-assign-tenant")
	ctx := context.Background()
	e := echo.New()

	assetID := seedCGAsset(t, h, tenantID, "assign-pc")
	g, err := h.configGroupSvc.Create(ctx, tenantID, "Assign Group", "", true)
	if err != nil {
		t.Fatal(err)
	}
	gid := strconv.FormatInt(g.ID, 10)

	// Add: kind "computer" maps to target_type "asset".
	form := url.Values{
		"target_kind": {"computer"},
		"target_ref":  {strconv.FormatInt(assetID, 10)},
		"priority":    {"3"},
		"enforced":    {"true"},
	}
	req := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/assignments", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(gid)
	if err := h.PolicyGroupAssignmentAdd(c); err != nil {
		t.Fatalf("AssignmentAdd: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("add status = %d, want 302", rec.Code)
	}
	list, _ := h.configGroupSvc.ListAssignments(ctx, tenantID, g.ID)
	if len(list) != 1 || list[0].TargetType != "asset" || list[0].Priority != 3 || !list[0].Enforced {
		t.Fatalf("assignment not persisted with kind mapping: %+v", list)
	}
	aid := strconv.FormatInt(list[0].ID, 10)

	// configuration_group kind is rejected with 400.
	badForm := url.Values{"target_kind": {"configuration_group"}, "target_ref": {"1"}}
	reqBad := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/assignments", badForm)
	reqBad = reqBad.WithContext(auth.WithSession(reqBad.Context(), adminSession(tenantID)))
	cBad := e.NewContext(reqBad, httptest.NewRecorder())
	cBad.SetParamNames("id")
	cBad.SetParamValues(gid)
	mustHTTPStatus(t, h.PolicyGroupAssignmentAdd(cBad), http.StatusBadRequest)

	// Update priority/enforced.
	upForm := url.Values{"priority": {"9"}}
	reqUp := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/assignments/"+aid, upForm)
	reqUp = reqUp.WithContext(auth.WithSession(reqUp.Context(), adminSession(tenantID)))
	recUp := httptest.NewRecorder()
	cUp := e.NewContext(reqUp, recUp)
	cUp.SetParamNames("id", "aid")
	cUp.SetParamValues(gid, aid)
	if err := h.PolicyGroupAssignmentUpdate(cUp); err != nil {
		t.Fatalf("AssignmentUpdate: %v", err)
	}
	list, _ = h.configGroupSvc.ListAssignments(ctx, tenantID, g.ID)
	if list[0].Priority != 9 || list[0].Enforced {
		t.Fatalf("assignment update not applied: %+v", list[0])
	}

	// Remove.
	reqRm := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/assignments/"+aid+"/remove", url.Values{})
	reqRm = reqRm.WithContext(auth.WithSession(reqRm.Context(), adminSession(tenantID)))
	recRm := httptest.NewRecorder()
	cRm := e.NewContext(reqRm, recRm)
	cRm.SetParamNames("id", "aid")
	cRm.SetParamValues(gid, aid)
	if err := h.PolicyGroupAssignmentRemove(cRm); err != nil {
		t.Fatalf("AssignmentRemove: %v", err)
	}
	list, _ = h.configGroupSvc.ListAssignments(ctx, tenantID, g.ID)
	if len(list) != 0 {
		t.Fatalf("assignment not removed: %+v", list)
	}
}

// seedPublishedModuleForHandler publishes a tenant module whose latest
// version satisfies the given policy URN and carries schema.
func seedPublishedModuleForHandler(t *testing.T, h *Handler, tenantID int64, urn, policyURN, schema string) {
	t.Helper()
	ctx := context.Background()
	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, nil, urn, "Handler Module", "")
	if err != nil {
		t.Fatal(err)
	}
	pub := seedCGIdentity(t, h, tenantID, "publisher-"+urn)
	draft, err := h.moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{
		Version: "1.0.0", Scope: "machine", Satisfies: []string{policyURN}, ParametersSchema: schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.moduleSvc.SetScript(ctx, draft.ID, "apply", "enforce.sh", "#!/bin/sh\ntrue\n"); err != nil {
		t.Fatal(err)
	}
	if err := h.moduleSvc.Publish(ctx, draft.ID, pub); err != nil {
		t.Fatal(err)
	}
}

// TestPolicyGroupBindingRoutes: add with valid params, 400 on invalid
// params, inline update, and remove.
func TestPolicyGroupBindingRoutes(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "cg_binding_test.db", "cg-binding-tenant")
	ctx := context.Background()
	e := echo.New()

	schema := `{"type":"object","properties":{"length":{"type":"number"}},"required":["length"]}`
	seedPublishedModuleForHandler(t, h, tenantID, "tenant.cgbind.mod", "sec.cgbind.policy", schema)

	g, err := h.configGroupSvc.Create(ctx, tenantID, "Bind Group", "", true)
	if err != nil {
		t.Fatal(err)
	}
	gid := strconv.FormatInt(g.ID, 10)

	// Valid add.
	form := url.Values{"policy_urn": {"sec.cgbind.policy"}, "state": {"enabled"}, "param_length": {"14"}}
	req := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/bindings", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(gid)
	if err := h.PolicyGroupBindingAdd(c); err != nil {
		t.Fatalf("BindingAdd: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("add status = %d, want 302", rec.Code)
	}
	bindings, _ := h.configGroupSvc.ListBindings(ctx, tenantID, g.ID)
	if len(bindings) != 1 {
		t.Fatalf("binding not persisted: %+v", bindings)
	}
	if !strings.Contains(bindings[0].ParameterValues.String, `"length":14`) {
		t.Fatalf("parameter_values not coerced to number: %q", bindings[0].ParameterValues.String)
	}
	bid := strconv.FormatInt(bindings[0].ID, 10)

	// Invalid: missing required param -> 400.
	badForm := url.Values{"policy_urn": {"sec.cgbind.policy2"}, "state": {"enabled"}}
	// Same module satisfies only sec.cgbind.policy; use the same URN to
	// hit schema validation (duplicate check happens after validation,
	// so use a wrong-typed value on a fresh policy instead).
	badForm = url.Values{"policy_urn": {"sec.cgbind.policy"}, "state": {"enabled"}, "param_length": {"NaN-ish"}}
	reqBad := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/bindings", badForm)
	reqBad = reqBad.WithContext(auth.WithSession(reqBad.Context(), adminSession(tenantID)))
	cBad := e.NewContext(reqBad, httptest.NewRecorder())
	cBad.SetParamNames("id")
	cBad.SetParamValues(gid)
	mustHTTPStatus(t, h.PolicyGroupBindingAdd(cBad), http.StatusBadRequest)

	// Update state + values.
	upForm := url.Values{"state": {"disabled"}, "param_length": {"20"}}
	reqUp := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/bindings/"+bid, upForm)
	reqUp = reqUp.WithContext(auth.WithSession(reqUp.Context(), adminSession(tenantID)))
	recUp := httptest.NewRecorder()
	cUp := e.NewContext(reqUp, recUp)
	cUp.SetParamNames("id", "bid")
	cUp.SetParamValues(gid, bid)
	if err := h.PolicyGroupBindingUpdate(cUp); err != nil {
		t.Fatalf("BindingUpdate: %v", err)
	}
	bindings, _ = h.configGroupSvc.ListBindings(ctx, tenantID, g.ID)
	if bindings[0].State != "disabled" || !strings.Contains(bindings[0].ParameterValues.String, `"length":20`) {
		t.Fatalf("binding update not applied: %+v", bindings[0])
	}

	// Remove.
	reqRm := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/bindings/"+bid+"/remove", url.Values{})
	reqRm = reqRm.WithContext(auth.WithSession(reqRm.Context(), adminSession(tenantID)))
	cRm := e.NewContext(reqRm, httptest.NewRecorder())
	cRm.SetParamNames("id", "bid")
	cRm.SetParamValues(gid, bid)
	if err := h.PolicyGroupBindingRemove(cRm); err != nil {
		t.Fatalf("BindingRemove: %v", err)
	}
	bindings, _ = h.configGroupSvc.ListBindings(ctx, tenantID, g.ID)
	if len(bindings) != 0 {
		t.Fatalf("binding not removed: %+v", bindings)
	}
}

// TestConfigGroupFieldUpdate: the General tab's inline-edit endpoint
// applies name/description/enabled and 400s unknown fields.
func TestConfigGroupFieldUpdate(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "cg_fields_test.db", "cg-fields-tenant")
	ctx := context.Background()
	e := echo.New()

	g, err := h.configGroupSvc.Create(ctx, tenantID, "Fields Group", "", true)
	if err != nil {
		t.Fatal(err)
	}
	gid := strconv.FormatInt(g.ID, 10)

	body := `{"section":"general","fields":{"name":"Renamed","enabled":"false"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/config-groups/"+gid+"/fields", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(gid)
	if err := h.ConfigGroupFieldUpdate(c); err != nil {
		t.Fatalf("ConfigGroupFieldUpdate: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got, _ := h.configGroupSvc.Get(ctx, tenantID, g.ID)
	if got.Name != "Renamed" || !got.Disabled {
		t.Fatalf("field update not applied: %+v", got)
	}

	badBody := `{"section":"general","fields":{"bogus":"x"}}`
	reqBad := httptest.NewRequest(http.MethodPost, "/api/config-groups/"+gid+"/fields", strings.NewReader(badBody))
	reqBad.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	reqBad = reqBad.WithContext(auth.WithSession(reqBad.Context(), adminSession(tenantID)))
	cBad := e.NewContext(reqBad, httptest.NewRecorder())
	cBad.SetParamNames("id")
	cBad.SetParamValues(gid)
	mustHTTPStatus(t, h.ConfigGroupFieldUpdate(cBad), http.StatusBadRequest)
}

// TestPolicyGroupDelete: delete hides the group, preserves its children
// until the purge boundary, and redirects to the active list.
func TestPolicyGroupDelete(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "cg_delete_test.db", "cg-delete-tenant")
	ctx := context.Background()
	e := echo.New()

	assetID := seedCGAsset(t, h, tenantID, "del-pc")
	g, err := h.configGroupSvc.Create(ctx, tenantID, "Delete Group", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.configGroupSvc.AddAssignment(ctx, tenantID, g.ID, "asset", assetID, 0, false); err != nil {
		t.Fatal(err)
	}
	gid := strconv.FormatInt(g.ID, 10)

	req := newFormReq(http.MethodPost, "/policy/groups/"+gid+"/delete", url.Values{})
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(gid)
	if err := h.PolicyGroupDelete(c); err != nil {
		t.Fatalf("PolicyGroupDelete: %v", err)
	}
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/policy/groups" {
		t.Fatalf("delete redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if _, err := h.configGroupSvc.Get(ctx, tenantID, g.ID); err == nil {
		t.Fatal("group still exists after delete")
	}
	// References survive soft deletion and are removed only at purge.
	n, err := h.db.Queries.CountAssignmentsByGroup(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("assignment count after soft delete = %d, want 1", n)
	}
}

// TestConfigGroupRBACDenied: every mutating route 403s for a session
// lacking endpoint_policy.manage_config_groups (the restricted-session
// pattern from the params_api tests).
func TestConfigGroupRBACDenied(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "cg_rbac_test.db", "cg-rbac-tenant")
	ctx := context.Background()
	e := echo.New()

	g, err := h.configGroupSvc.Create(ctx, tenantID, "RBAC Group", "", true)
	if err != nil {
		t.Fatal(err)
	}
	gid := strconv.FormatInt(g.ID, 10)
	sess := userSession(tenantID) // "user" template: manage_config_groups = "no"

	cases := []struct {
		name    string
		handler func(echo.Context) error
		path    string
		params  [][2]string
	}{
		{"create-page", h.PolicyGroupNew, "/policy/groups/new", nil},
		{"create", h.PolicyGroupCreate, "/policy/groups/new", nil},
		{"delete", h.PolicyGroupDelete, "/policy/groups/" + gid + "/delete", [][2]string{{"id", gid}}},
		{"assignment-add", h.PolicyGroupAssignmentAdd, "/policy/groups/" + gid + "/assignments", [][2]string{{"id", gid}}},
		{"assignment-update", h.PolicyGroupAssignmentUpdate, "/policy/groups/" + gid + "/assignments/1", [][2]string{{"id", gid}, {"aid", "1"}}},
		{"assignment-remove", h.PolicyGroupAssignmentRemove, "/policy/groups/" + gid + "/assignments/1/remove", [][2]string{{"id", gid}, {"aid", "1"}}},
		{"binding-add", h.PolicyGroupBindingAdd, "/policy/groups/" + gid + "/bindings", [][2]string{{"id", gid}}},
		{"binding-update", h.PolicyGroupBindingUpdate, "/policy/groups/" + gid + "/bindings/1", [][2]string{{"id", gid}, {"bid", "1"}}},
		{"binding-remove", h.PolicyGroupBindingRemove, "/policy/groups/" + gid + "/bindings/1/remove", [][2]string{{"id", gid}, {"bid", "1"}}},
		{"field-update", h.ConfigGroupFieldUpdate, "/api/config-groups/" + gid + "/fields", [][2]string{{"id", gid}}},
	}
	for _, tc := range cases {
		req := newFormReq(http.MethodPost, tc.path, url.Values{})
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		c := e.NewContext(req, httptest.NewRecorder())
		var names, vals []string
		for _, p := range tc.params {
			names = append(names, p[0])
			vals = append(vals, p[1])
		}
		if len(names) > 0 {
			c.SetParamNames(names...)
			c.SetParamValues(vals...)
		}
		err := tc.handler(c)
		he, ok := err.(*echo.HTTPError)
		if !ok || he.Code != http.StatusForbidden {
			t.Errorf("%s: want 403, got %v", tc.name, err)
		}
	}
}
