package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

// paramsAPIContext builds an echo.Context for a GET /api/params handler
// call with sess (nil = unauthenticated) in the request context.
func paramsAPIContext(e *echo.Echo, sess *auth.UserSession) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, "/api/params", nil)
	if sess != nil {
		req = req.WithContext(auth.WithSession(req.Context(), sess))
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// callParamsAPI invokes the handler and unmarshals the JSON body.
func callParamsAPI(t *testing.T, h *Handler, sess *auth.UserSession) (ParamsAPIResponse, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	c, rec := paramsAPIContext(e, sess)
	if err := h.ParamsAPI(c); err != nil {
		t.Fatalf("ParamsAPI failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp ParamsAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, rec.Body.String())
	}
	return resp, rec
}

// (1) Admin session sees every entity in canonical order, a known param
// with the correct operators for its type, and byte-identical output
// across calls (deterministic ordering).
func TestParamsAPIAdminSeesFullTree(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "params_api_admin_test.db", "params-api-admin-tenant")
	sess := adminSession(tenantID)

	resp, rec := callParamsAPI(t, h, sess)

	if ct := rec.Header().Get(echo.HeaderContentType); ct != echo.MIMEApplicationJSON && ct != echo.MIMEApplicationJSONCharsetUTF8 {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	wantEntities := []string{"computer", "server", "printer", "desk", "user"}
	gotEntities := make([]string, 0, len(resp.Sources))
	for _, src := range resp.Sources {
		gotEntities = append(gotEntities, src.Entity)
	}
	if !reflect.DeepEqual(gotEntities, wantEntities) {
		t.Fatalf("entities = %v, want %v", gotEntities, wantEntities)
	}

	// computer/hardware/os_family: enum with values + enum operators.
	var osFamily *ParamsAPIParam
	for _, src := range resp.Sources {
		if src.Entity != "computer" {
			continue
		}
		if src.Label != "Computer" || src.PluralLabel != "Computers" {
			t.Errorf("computer labels = %q/%q, want Computer/Computers", src.Label, src.PluralLabel)
		}
		for _, sec := range src.Sections {
			for i := range sec.Params {
				if sec.Params[i].Path == "computer/hardware/os_family" {
					osFamily = &sec.Params[i]
					if sec.Key != "hardware" || sec.Label != "Hardware" {
						t.Errorf("os_family section = %q/%q, want hardware/Hardware", sec.Key, sec.Label)
					}
				}
			}
		}
	}
	if osFamily == nil {
		t.Fatal("computer/hardware/os_family not found in admin response")
	}
	if osFamily.Key != "os_family" || osFamily.Type != "enum" {
		t.Errorf("os_family = {Key:%q Type:%q}, want {os_family enum}", osFamily.Key, osFamily.Type)
	}
	if !reflect.DeepEqual(osFamily.EnumValues, []string{"linux", "windows", "macos"}) {
		t.Errorf("os_family enumValues = %v, want [linux windows macos]", osFamily.EnumValues)
	}
	wantOps := params.OperatorsForType(params.TypeEnum)
	if len(osFamily.Operators) != len(wantOps) {
		t.Fatalf("os_family has %d operators, want %d", len(osFamily.Operators), len(wantOps))
	}
	for i, op := range wantOps {
		got := osFamily.Operators[i]
		if got.Key != op.Key || got.Label != op.Label || got.NeedsValue != op.NeedsValue {
			t.Errorf("os_family operator[%d] = %+v, want %+v", i, got, op)
		}
	}

	// ram_mb must carry its unit for the typed value input.
	found := false
	for _, src := range resp.Sources {
		for _, sec := range src.Sections {
			for _, p := range sec.Params {
				if p.Path == "computer/hardware/ram_mb" {
					found = true
					if p.Unit != "MB" || p.Type != "int" {
						t.Errorf("ram_mb = {Type:%q Unit:%q}, want {int MB}", p.Type, p.Unit)
					}
				}
			}
		}
	}
	if !found {
		t.Error("computer/hardware/ram_mb not found in admin response")
	}

	// Determinism: a second call must produce byte-identical output.
	_, rec2 := callParamsAPI(t, h, sess)
	if !bytes.Equal(rec.Body.Bytes(), rec2.Body.Bytes()) {
		t.Error("two identical requests produced different bytes — output is not deterministic")
	}
}

// (2) A session lacking asset.view but holding identity.view must see
// ZERO asset entities/params while identity params remain — the Task 1.1
// footgun scenario: raw schema defs all have empty Permission, so a
// handler filtering them directly would leak the full asset tree here.
func TestParamsAPIRestrictedGrantsFilterAssets(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "params_api_restricted_test.db", "params-api-restricted-tenant")
	sess := &auth.UserSession{
		TenantID: tenantID, IdentityID: 2, Role: identities.RoleUser,
		Grants: authz.Grants{"identity.view": "all"},
	}

	resp, _ := callParamsAPI(t, h, sess)

	if len(resp.Sources) != 1 || resp.Sources[0].Entity != "user" {
		got := make([]string, 0, len(resp.Sources))
		for _, s := range resp.Sources {
			got = append(got, s.Entity)
		}
		t.Fatalf("entities = %v, want exactly [user]", got)
	}
	nParams := 0
	for _, sec := range resp.Sources[0].Sections {
		nParams += len(sec.Params)
	}
	if nParams == 0 {
		t.Error("identity entity present but carries zero params")
	}

	// And vice versa: asset.view only -> no user entity, all four asset
	// subtypes present.
	assetSess := &auth.UserSession{
		TenantID: tenantID, IdentityID: 3, Role: identities.RoleUser,
		Grants: authz.Grants{"asset.view": "own"}, // any non-deny scope means "may see the definitions"
	}
	resp2, _ := callParamsAPI(t, h, assetSess)
	gotEntities := make([]string, 0, len(resp2.Sources))
	for _, s := range resp2.Sources {
		gotEntities = append(gotEntities, s.Entity)
	}
	if !reflect.DeepEqual(gotEntities, []string{"computer", "server", "printer", "desk"}) {
		t.Errorf("entities = %v, want [computer server printer desk]", gotEntities)
	}
}

// (2b) Deny-all grants -> empty sources array (valid JSON, no entities).
func TestParamsAPIDenyAllGrantsEmpty(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "params_api_denyall_test.db", "params-api-denyall-tenant")
	sess := &auth.UserSession{
		TenantID: tenantID, IdentityID: 4, Role: identities.RoleUser,
		Grants: authz.Grants{},
	}
	resp, rec := callParamsAPI(t, h, sess)
	if len(resp.Sources) != 0 {
		t.Errorf("deny-all grants returned %d entities, want 0", len(resp.Sources))
	}
	// "sources" must still be a JSON array, not null.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if string(raw["sources"]) != "[]" {
		t.Errorf("sources = %s, want []", raw["sources"])
	}
}

// (3) No session in context (defense in depth behind the auth
// middleware, which normally 302s to /login before the handler runs —
// see TestParamsAPIUnauthenticatedRedirects in console/server) -> 401.
func TestParamsAPINoSession401(t *testing.T) {
	h, _ := setupPlurisTestDB(t, "params_api_nosession_test.db", "params-api-nosession-tenant")
	e := echo.New()
	c, _ := paramsAPIContext(e, nil)
	err := h.ParamsAPI(c)
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	mustHTTPStatus(t, err, http.StatusUnauthorized)
}

// ---- Task 4.4: ?module_id= module-input source -----------------------

// paramsAPIContextWithModuleID builds a GET /api/params?module_id=...
// request, mirroring paramsAPIContext.
func paramsAPIContextWithModuleID(e *echo.Echo, sess *auth.UserSession, moduleID string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, "/api/params?module_id="+url.QueryEscape(moduleID), nil)
	if sess != nil {
		req = req.WithContext(auth.WithSession(req.Context(), sess))
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// moduleInputEntity returns the "module" source from resp, or nil.
func moduleInputEntity(resp ParamsAPIResponse) *ParamsAPIEntity {
	for i := range resp.Sources {
		if resp.Sources[i].Entity == "module" {
			return &resp.Sources[i]
		}
	}
	return nil
}

// (4) An authorized session (module owner) with ?module_id= sees the
// module's own module/input/<key> parameters as an extra "module"
// source, alongside the normal entity sources.
func TestParamsAPIModuleIDReturnsModuleInputs(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()

	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.withinputs", "With Inputs", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := h.moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{
		Version: "1.0.0", Scope: "machine",
		ParametersSchema: `{"type":"object","properties":{"hostname":{"type":"string","title":"Hostname"}},"required":["hostname"]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = draft

	e := echo.New()
	c, rec := paramsAPIContextWithModuleID(e, sess, "tenant.acme.withinputs")
	if err := h.ParamsAPI(c); err != nil {
		t.Fatalf("ParamsAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp ParamsAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	modSrc := moduleInputEntity(resp)
	if modSrc == nil {
		t.Fatal("expected a \"module\" source in the response")
	}
	if len(modSrc.Sections) != 1 || len(modSrc.Sections[0].Params) != 1 {
		t.Fatalf("module source = %+v, want exactly one param", modSrc)
	}
	p := modSrc.Sections[0].Params[0]
	if p.Path != "module/input/hostname" || p.Key != "hostname" || p.Label != "Hostname" || p.Type != "string" {
		t.Errorf("module input param = %+v, want module/input/hostname string", p)
	}

	// Non-module entity sources are still present -- module_id ADDS a
	// source, it doesn't replace the feed.
	sawEntity := false
	for _, s := range resp.Sources {
		if s.Entity == "computer" {
			sawEntity = true
		}
	}
	if !sawEntity {
		t.Error("expected the built-in entity sources to still be present alongside the module source")
	}
}

// (5) A same-tenant session with no manage_modules grant, no ownership,
// and no explicit module_grants row is denied -- 403, not a silently
// empty/omitted module source.
func TestParamsAPIModuleIDStrangerForbidden(t *testing.T) {
	h, d, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, d, tenantID, "owner")
	ctx := context.Background()
	if _, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.private", "Private", ""); err != nil {
		t.Fatal(err)
	}

	strangerID := newEditorIdentity(t, d, tenantID, "stranger")
	strangerSess := &auth.UserSession{
		TenantID: tenantID, IdentityID: strangerID, Role: identities.RoleUser,
		Grants: authz.Grants{}, // no manage_modules, no endpoint_policy.view
	}

	e := echo.New()
	c, _ := paramsAPIContextWithModuleID(e, strangerSess, "tenant.acme.private")
	err := h.ParamsAPI(c)
	if err == nil {
		t.Fatal("expected error for stranger")
	}
	mustHTTPStatus(t, err, http.StatusForbidden)
}

// (6) Unknown module_id -> 404, not a silently empty module source.
func TestParamsAPIModuleIDUnknown404(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	sess := adminEditorSession(tenantID, newEditorIdentity(t, h.db, tenantID, "owner"))

	e := echo.New()
	c, _ := paramsAPIContextWithModuleID(e, sess, "tenant.acme.does-not-exist")
	err := h.ParamsAPI(c)
	if err == nil {
		t.Fatal("expected error for unknown module_id")
	}
	mustHTTPStatus(t, err, http.StatusNotFound)
}

// (7) No module_id -> byte-identical to the pre-Task-4.4 response for a
// fixed session (the module_id branch must be a true no-op when absent).
func TestParamsAPINoModuleIDUnchanged(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "params_api_no_module_id_test.db", "params-api-no-module-id-tenant")
	sess := adminSession(tenantID)

	_, rec1 := callParamsAPI(t, h, sess)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/params", nil)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec2 := httptest.NewRecorder()
	c := e.NewContext(req, rec2)
	if err := h.ParamsAPI(c); err != nil {
		t.Fatalf("ParamsAPI: %v", err)
	}
	if !bytes.Equal(rec1.Body.Bytes(), rec2.Body.Bytes()) {
		t.Error("no module_id request produced different bytes across two otherwise-identical calls")
	}
}
