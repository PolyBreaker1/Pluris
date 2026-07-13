package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
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

// TestAssetCreateSubmitAndRBAC covers the "Enroll/Add asset" QA fix: an
// admin session posting Name (+ optional Hostname) to
// /assets/:subtype/new must create a real row (found by name in the
// tenant's computers) and 302 to its detail page, while a plain "user"
// session (asset.create = "no" in its permission template) is forbidden.
func TestAssetCreateSubmitAndRBAC(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "asset_new_handler_test.db"))
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
		form := url.Values{"name": {"new-laptop-099"}, "hostname": {"new-laptop-099"}}
		req := httptest.NewRequest(http.MethodPost, "/assets/computers/new", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		return req
	}
	withParam := func(c echo.Context) {
		c.SetParamNames("subtype")
		c.SetParamValues("computers")
	}

	// Plain user is forbidden.
	req := newReq()
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 2, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	withParam(c)
	if err := h.AssetCreateSubmit(c); err == nil {
		t.Fatal("user create should be forbidden")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("user create: want 403 HTTPError, got %v", err)
	}

	adminSess := &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}

	// Missing name re-renders the form with an error, 200.
	badForm := url.Values{"name": {""}}
	badReq := httptest.NewRequest(http.MethodPost, "/assets/computers/new", strings.NewReader(badForm.Encode()))
	badReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	badReq = badReq.WithContext(auth.WithSession(badReq.Context(), adminSess))
	rec = httptest.NewRecorder()
	c = e.NewContext(badReq, rec)
	withParam(c)
	if err := h.AssetCreateSubmit(c); err != nil {
		t.Fatalf("missing name should re-render, not error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("missing name status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Name is required") {
		t.Fatalf("missing name body should contain the error message, got: %s", rec.Body.String())
	}

	// Bad subtype 404s.
	badSubReq := newReq()
	badSubReq = badSubReq.WithContext(auth.WithSession(badSubReq.Context(), adminSess))
	rec = httptest.NewRecorder()
	c = e.NewContext(badSubReq, rec)
	c.SetParamNames("subtype")
	c.SetParamValues("bogus")
	if err := h.AssetCreateSubmit(c); err == nil {
		t.Fatal("bad subtype should 404")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusNotFound {
		t.Fatalf("bad subtype: want 404 HTTPError, got %v", err)
	}

	// Admin succeeds: 302 + row exists.
	req = newReq()
	req = req.WithContext(auth.WithSession(req.Context(), adminSess))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	withParam(c)
	if err := h.AssetCreateSubmit(c); err != nil {
		t.Fatalf("admin create failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("admin create status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/assets/computers/comp.acme.web.") {
		t.Fatalf("redirect location = %q, want /assets/computers/comp.acme.web.NNNN prefix", loc)
	}

	rows, err := h.assetSvc.ListBySubtype(ctx, tenant.ID, "computer")
	if err != nil {
		t.Fatalf("list by subtype: %v", err)
	}
	found := false
	for _, a := range rows {
		if a.PrimaryHostname() == "new-laptop-099" {
			found = true
		}
	}
	if !found {
		t.Fatalf("created asset not found among %d computers", len(rows))
	}
}

// TestAssetNewShowRBAC covers the GET form-render side: a plain "user"
// session is forbidden, an admin session renders the form page.
func TestAssetNewShowRBAC(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "asset_new_show_test.db"))
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

	req := httptest.NewRequest(http.MethodGet, "/assets/computers/new", nil)
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 2, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("subtype")
	c.SetParamValues("computers")
	if err := h.AssetNewShow(c); err == nil {
		t.Fatal("user AssetNewShow should be forbidden")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("user AssetNewShow: want 403 HTTPError, got %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/computers/new", nil)
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("subtype")
	c.SetParamValues("computers")
	if err := h.AssetNewShow(c); err != nil {
		t.Fatalf("admin AssetNewShow failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("admin AssetNewShow status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `data-testid="page-asset-new"`) {
		t.Fatalf("expected page-asset-new testid in body")
	}
}
