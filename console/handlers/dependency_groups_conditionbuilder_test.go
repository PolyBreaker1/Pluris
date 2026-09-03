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

// TestDependencyGroupConditionAddAcceptsWidenedOperator covers Task 2.1's
// operator widening: "equals" (rejected pre-2.1) must now be accepted end
// to end through the handler, and "not_a_real_kind" as the kind form
// field must 400.
func TestDependencyGroupConditionAddAcceptsWidenedOperator(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_cond_builder_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme7", Slug: "acme7"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()

	adminSess := &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}

	group, err := h.depGroupSvc.Create(ctx, tenant.ID, "Widen Test Group", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	post := func(form url.Values) (*httptest.ResponseRecorder, error) {
		req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups/x/conditions", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req = req.WithContext(auth.WithSession(req.Context(), adminSess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(group.ID, 10))
		return rec, h.DependencyGroupConditionAdd(c)
	}

	// "equals" was rejected before Task 2.1; the eval engine widening
	// means it must now succeed with no "kind" field at all (defaults to
	// param).
	rec, err := post(url.Values{"param_path": {"computer/hardware/os_family"}, "operator": {"equals"}, "values": {"linux"}})
	if err != nil {
		t.Fatalf("equals operator should now be accepted: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("equals operator add status = %d, want 302", rec.Code)
	}

	// A garbage kind must 400 and must not create a condition.
	if _, err := post(url.Values{"kind": {"not_a_real_kind"}, "param_path": {"computer/hardware/os_family"}, "operator": {"equals"}, "values": {"linux"}}); err == nil {
		t.Fatal("garbage kind should be rejected")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("garbage kind: want 400 HTTPError, got %v", err)
	}

	g, err := h.depGroupSvc.Get(ctx, group.ID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if len(g.Conditions) != 1 {
		t.Fatalf("want exactly 1 persisted condition (the equals add), got %d", len(g.Conditions))
	}
	if g.Conditions[0].Operator != "equals" {
		t.Fatalf("want persisted operator equals, got %q", g.Conditions[0].Operator)
	}
}

// TestDependencyGroupMatchModeUpdate covers the new match-mode route:
// valid mode changes persist, invalid mode 400s, and builtins are
// protected.
func TestDependencyGroupMatchModeUpdate(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_match_mode_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme8", Slug: "acme8"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()
	adminSess := &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}

	postMode := func(groupID int64, mode string) (*httptest.ResponseRecorder, error) {
		form := url.Values{"match_mode": {mode}}
		req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups/x/match-mode", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req = req.WithContext(auth.WithSession(req.Context(), adminSess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(groupID, 10))
		return rec, h.DependencyGroupMatchModeUpdate(c)
	}

	group, err := h.depGroupSvc.Create(ctx, tenant.ID, "Match Mode Group", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	if _, err := postMode(group.ID, "bogus"); err == nil {
		t.Fatal("invalid match mode should be rejected")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("invalid match mode: want 400 HTTPError, got %v", err)
	}

	rec, err := postMode(group.ID, "any")
	if err != nil {
		t.Fatalf("valid match mode change failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("match mode change status = %d, want 302", rec.Code)
	}
	g, err := h.depGroupSvc.Get(ctx, group.ID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if g.MatchMode != "any" {
		t.Fatalf("want match_mode=any, got %q", g.MatchMode)
	}

	// Builtin protection.
	if err := h.depGroupSvc.EnsureBuiltins(ctx, tenant.ID); err != nil {
		t.Fatalf("ensure builtins: %v", err)
	}
	builtins, err := h.depGroupSvc.ListByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list by tenant: %v", err)
	}
	var builtinID int64
	for _, bg := range builtins {
		if bg.Builtin {
			builtinID = bg.ID
			break
		}
	}
	if builtinID == 0 {
		t.Fatal("expected at least one builtin group")
	}
	if _, err := postMode(builtinID, "any"); err == nil {
		t.Fatal("builtin match mode change should be rejected")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("builtin match mode change: want 400 HTTPError, got %v", err)
	}
}

// TestDependencyGroupConditionAddRepeatedValuesEncoding covers Task 2.3's
// values encoding contract: the condition-builder page JS sends
// `detail.values` as REPEATED `values=` form keys (one per element), and
// the handler must read them via FormParams() — so a multi-value "in"
// condition round-trips element-for-element, including a value that
// itself contains a comma (which the old comma-split encoding would have
// mangled into two values).
func TestDependencyGroupConditionAddRepeatedValuesEncoding(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_values_enc_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme9", Slug: "acme9"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()
	adminSess := &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}

	group, err := h.depGroupSvc.Create(ctx, tenant.ID, "Values Encoding Group", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	post := func(form url.Values) (*httptest.ResponseRecorder, error) {
		req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups/x/conditions", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req = req.WithContext(auth.WithSession(req.Context(), adminSess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(group.ID, 10))
		return rec, h.DependencyGroupConditionAdd(c)
	}

	// Multi-value enum "in" condition: three repeated values keys, one of
	// which contains a literal comma.
	rec, err := post(url.Values{
		"kind":       {"param"},
		"param_path": {"computer/hardware/os_package_family"},
		"operator":   {"in"},
		"values":     {"rpm", "deb", "weird,value"},
	})
	if err != nil {
		t.Fatalf("multi-value add failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("multi-value add status = %d, want 302", rec.Code)
	}

	g, err := h.depGroupSvc.Get(ctx, group.ID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if len(g.Conditions) != 1 {
		t.Fatalf("want 1 condition, got %d", len(g.Conditions))
	}
	got := g.Conditions[0].Values
	want := []string{"rpm", "deb", "weird,value"}
	if len(got) != len(want) {
		t.Fatalf("values round-trip: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("values[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestDependencyGroupConditionAddScriptViaForm covers adding a script
// condition end-to-end through the form POST the condition-builder page
// JS issues: kind=script with script_source/script_expect and NO
// param_path/operator/values must persist, and an empty script_source
// must 400.
func TestDependencyGroupConditionAddScriptViaForm(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "dep_groups_script_form_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme10", Slug: "acme10"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()
	adminSess := &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}

	group, err := h.depGroupSvc.Create(ctx, tenant.ID, "Script Form Group", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	post := func(form url.Values) (*httptest.ResponseRecorder, error) {
		req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups/x/conditions", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req = req.WithContext(auth.WithSession(req.Context(), adminSess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(group.ID, 10))
		return rec, h.DependencyGroupConditionAdd(c)
	}

	src := "#!/bin/sh\ncat /etc/foo.conf"
	rec, err := post(url.Values{
		"kind":          {"script"},
		"script_source": {src},
		"operator":      {"contains"},
		"values":        {"ok"},
	})
	if err != nil {
		t.Fatalf("script condition add failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("script condition add status = %d, want 302", rec.Code)
	}

	crec, err := post(url.Values{
		"kind":          {"command"},
		"script_source": {"uname -r"},
		"operator":      {"contains"},
		"values":        {"3"},
	})
	if err != nil {
		t.Fatalf("command condition add failed: %v", err)
	}
	if crec.Code != http.StatusFound {
		t.Fatalf("command condition add status = %d, want 302", crec.Code)
	}

	g, err := h.depGroupSvc.Get(ctx, group.ID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if len(g.Conditions) != 2 {
		t.Fatalf("want 2 conditions, got %d", len(g.Conditions))
	}
	cond := g.Conditions[0]
	if string(cond.Kind) != "script" || cond.ScriptSource != src || string(cond.Operator) != "contains" || len(cond.Values) != 1 || cond.Values[0] != "ok" {
		t.Fatalf("script condition round-trip mismatch: %+v", cond)
	}
	if string(g.Conditions[1].Kind) != "command" || g.Conditions[1].ScriptSource != "uname -r" {
		t.Fatalf("command condition round-trip mismatch: %+v", g.Conditions[1])
	}

	// Empty script source must 400.
	if _, err := post(url.Values{"kind": {"script"}, "script_source": {""}, "operator": {"exists"}}); err == nil {
		t.Fatal("empty script_source should be rejected")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("empty script_source: want 400 HTTPError, got %v", err)
	}
}
