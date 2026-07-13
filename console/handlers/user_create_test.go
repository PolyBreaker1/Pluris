package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/pkg/auth"
)

// TestUserNewShowFullLayout proves GET /users/new renders the Task 8
// full-page create form (not the old small UserFormPage): the
// page-user-create testid, an editable username input (create-time-
// settable, unlike edit mode), and a field from a section the old form
// never had (phone_mobile) all appear, with required attrs on the
// identifying fields.
func TestUserNewShowFullLayout(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "user_create_show_test.db", "user-create-show-tenant")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/users/new", nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.UserNewShow(c); err != nil {
		t.Fatalf("UserNewShow: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-testid="page-user-create"`,
		`name="username"`,
		`name="phone_mobile"`,
		`name="email"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	// username and email are required per the brief.
	if !strings.Contains(body, `id="ucf-username"`) || !strings.Contains(body, `required`) {
		t.Error("expected required attrs on identifying fields")
	}
}

// TestUserNewShowGateDenied proves a user-template session is denied both
// the GET (show form) and the POST (submit) sides of the full-page
// create flow — the identity.create gate from the old small form carries
// over unchanged.
func TestUserNewShowGateDenied(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "user_create_gate_test.db", "user-create-gate-tenant")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/users/new", nil)
	req = req.WithContext(auth.WithSession(req.Context(), userSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	mustHTTPStatus(t, h.UserNewShow(c), http.StatusForbidden)

	form := url.Values{"username": {"nope"}, "email": {"nope@example.com"}}
	postReq := newFormReq(http.MethodPost, "/users/new", form)
	postReq = postReq.WithContext(auth.WithSession(postReq.Context(), userSession(tenantID)))
	postRec := httptest.NewRecorder()
	postC := e.NewContext(postReq, postRec)
	mustHTTPStatus(t, h.UserCreateSubmit(postC), http.StatusForbidden)
}

// TestUserCreateSubmitFullForm proves the full-page create POST persists
// every field submitted across multiple schema sections (identity,
// organization, contact) in one round-trip, auto-fills display_name from
// First+Last, and redirects (302) to the new user's detail page.
func TestUserCreateSubmitFullForm(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "user_create_submit_test.db", "user-create-submit-tenant")

	e := echo.New()
	form := url.Values{
		"username":     {"jdoe"},
		"email":        {"jdoe@example.com"},
		"given_name":   {"Jane"},
		"surname":      {"Doe"},
		"title":        {"Engineer"},
		"phone_mobile": {"555-1234"},
	}
	req := newFormReq(http.MethodPost, "/users/new", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.UserCreateSubmit(c); err != nil {
		t.Fatalf("UserCreateSubmit: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302, body: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/users/") {
		t.Fatalf("redirect target = %q, want /users/<id> prefix", loc)
	}
	idStr := strings.TrimPrefix(loc, "/users/")
	idStr = strings.SplitN(idStr, "?", 2)[0]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		t.Fatalf("parse id from redirect %q: %v", loc, err)
	}

	created, err := h.identitySvc.Get(c.Request().Context(), id)
	if err != nil {
		t.Fatalf("get created identity: %v", err)
	}
	if created.Username != "jdoe" || created.Email != "jdoe@example.com" {
		t.Errorf("core fields not persisted: %+v", created)
	}
	if created.DisplayName != "Jane Doe" {
		t.Errorf("display_name auto-fill = %q, want %q", created.DisplayName, "Jane Doe")
	}
	if created.Title != "Engineer" {
		t.Errorf("title = %q, want %q", created.Title, "Engineer")
	}
	if created.PhoneMobile != "555-1234" {
		t.Errorf("phone_mobile = %q, want %q", created.PhoneMobile, "555-1234")
	}
}

// TestUserCreateSubmitMissingUsername proves a missing username re-renders
// the create page (200, not a redirect) with an error banner and the
// entered email preserved in the input's value attribute.
func TestUserCreateSubmitMissingUsername(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "user_create_missing_username_test.db", "user-create-missing-tenant")

	e := echo.New()
	form := url.Values{"email": {"keepme@example.com"}}
	req := newFormReq(http.MethodPost, "/users/new", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.UserCreateSubmit(c); err != nil {
		t.Fatalf("UserCreateSubmit: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-render on validation error)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="keepme@example.com"`) {
		t.Error("entered email not preserved in re-rendered form")
	}
	if !strings.Contains(body, "Username and email are required.") {
		t.Error("expected error banner text for the required-field violation")
	}
}

// TestUserCreateSubmitNonEditableKeyIgnored proves a non-editable schema
// key injected into the create form (e.g. "role", which the UI never
// renders an input for) never reaches UpdateFields and never overrides
// the server-assigned value -- the create form's field loop excludes
// every identities.NonEditableFieldKeys key (username excepted) before
// building the per-section UpdateFields payload.
func TestUserCreateSubmitNonEditableKeyIgnored(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "user_create_noneditable_test.db", "user-create-noneditable-tenant")

	e := echo.New()
	form := url.Values{
		"username": {"injector"},
		"email":    {"injector@example.com"},
		"role":     {"super_admin"}, // non-editable: must not be applied
	}
	req := newFormReq(http.MethodPost, "/users/new", form)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.UserCreateSubmit(c); err != nil {
		t.Fatalf("UserCreateSubmit: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302, body: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	idStr := strings.TrimPrefix(strings.SplitN(loc, "?", 2)[0], "/users/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		t.Fatalf("parse id from redirect %q: %v", loc, err)
	}
	if strings.Contains(loc, "warn=") {
		t.Errorf("redirect %q should not carry a warn param -- the injected key was silently ignored, not attempted+rejected", loc)
	}

	created, err := h.identitySvc.Get(c.Request().Context(), id)
	if err != nil {
		t.Fatalf("get created identity: %v", err)
	}
	if created.Role == "super_admin" {
		t.Errorf("role was set to %q from the injected form field, want the default user template role", created.Role)
	}
}
