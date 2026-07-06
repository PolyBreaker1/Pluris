// Package server tests.
//
// MOUNT-POINT TESTS (per docs/UX_INVARIANTS.md INV-U6, INV-P2):
// every route in the locked sidebar must render with the documented
// `data-testid` anchor for its canonical editor (or page stub, in
// Increment 1). Adding a route requires adding a row to the testbed
// table here AND extending the Concept Registry §VII.
//
// In Increment 1 the canonical editors don't exist yet, so each route
// asserts the page-stub anchor. Increment 2+ will replace stub anchors
// with real editor anchors as they land.
package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// doCSRFPost performs a full CSRF-protected POST against e: it first GETs
// csrfPath to obtain a fresh CSRF cookie+token pair (CSRF middleware issues
// the cookie and stashes the token in context on every non-skipped request,
// including GET — see server.go's NewWithDB comment), then POSTs values to
// postPath with both the cookie and the token attached as the "_csrf" form
// field, exactly as a real browser session would (cookie automatically
// resent by the client, token value embedded in the hidden form field by
// the previously-rendered page).
//
// A bare POST with no _csrf field/cookie at all gets rejected by Echo's
// CSRF middleware with 400 "missing csrf token in the form parameter",
// since TokenLookup is "form:_csrf" and the field is fetched from the
// request, not the session — this helper exists so every test needing a
// state-changing POST (e.g. /setup, /login, and later /users CRUD) gets
// that round-trip right without repeating it inline.
//
// extraCookies are attached to both the GET and the POST (e.g. an existing
// session cookie, for POSTs made by an already-authenticated caller).
func doCSRFPost(t *testing.T, e *echo.Echo, csrfPath, postPath string, values url.Values, extraCookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	getReq := httptest.NewRequest(http.MethodGet, csrfPath, nil)
	for _, c := range extraCookies {
		getReq.AddCookie(c)
	}
	getRec := httptest.NewRecorder()
	e.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code, "GET %s should render the form", csrfPath)

	var csrfCookie *http.Cookie
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "_csrf" {
			csrfCookie = c
		}
	}
	require.NotNil(t, csrfCookie, "expected a _csrf cookie from GET %s", csrfPath)

	values.Set("_csrf", csrfCookie.Value)
	postReq := httptest.NewRequest(http.MethodPost, postPath, strings.NewReader(values.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(csrfCookie)
	for _, c := range extraCookies {
		postReq.AddCookie(c)
	}
	postRec := httptest.NewRecorder()
	e.ServeHTTP(postRec, postReq)
	return postRec
}

// newTestServer builds an Echo instance against an isolated, throwaway
// SQLite file (never the real dev pluris.db), seeds one tenant + one
// super_admin identity, and returns both the server and a ready-to-use
// session cookie for that identity. Every existing mount-point test needs
// this now that RequireAuth/RequireRole gate every route.
func newTestServer(t *testing.T) (*echo.Echo, *http.Cookie) {
	t.Helper()
	return newTestServerAt(t, filepath.Join(t.TempDir(), "test_server.db"))
}

// newTestServerAt is newTestServer with a caller-chosen SQLite path, so
// tests that need to seed extra rows can re-open the same file directly.
func newTestServerAt(t *testing.T, dbPath string) (*echo.Echo, *http.Cookie) {
	t.Helper()

	e := NewWithDB(dbPath)

	// Seed one identity by hitting the real /setup flow through the
	// server itself (not a side-channel DB write) — this exercises the
	// exact code path a real first-run admin would use, and guarantees
	// the seeded account has a working password hash or a fully-formed
	// session however SetupSubmit chooses to build one.
	setupRec := doCSRFPost(t, e, "/setup", "/setup", url.Values{
		"org_name":     {"Test Org"},
		"display_name": {"Test Admin"},
		"email":        {"admin@test.local"},
		"password":     {"testpassword123"},
	})
	if setupRec.Code != http.StatusFound {
		t.Fatalf("setup failed: expected 302, got %d, body: %s", setupRec.Code, setupRec.Body.String())
	}

	loginRec := doCSRFPost(t, e, "/login", "/login", url.Values{
		"email":    {"admin@test.local"},
		"password": {"testpassword123"},
	})
	if loginRec.Code != http.StatusFound {
		t.Fatalf("login failed: expected 302, got %d, body: %s", loginRec.Code, loginRec.Body.String())
	}

	cookies := loginRec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "pluris_session" {
			return e, c
		}
	}
	t.Fatal("expected pluris_session cookie after login, got none")
	return nil, nil
}

// mountPoints is the table of (route → expected data-testid).
// It is the executable form of docs/UX_INVARIANTS.md §VII Concept Registry.
var mountPoints = []struct {
	name         string
	path         string
	expectStatus int
	expectTestID string // substring expected in the response body
	redirectsTo  string // if non-empty, this path is a redirect to this target
}{
	{name: "dashboard", path: "/", expectStatus: 200, expectTestID: `data-testid="page-dashboard"`},
	{name: "users", path: "/users", expectStatus: 200, expectTestID: `data-testid="page-users"`},
	{name: "assets-redirect", path: "/assets", expectStatus: 302, redirectsTo: "/assets/computers"},
	{name: "assets-computers", path: "/assets/computers", expectStatus: 200, expectTestID: `data-testid="page-assets-computers"`},
	{name: "assets-servers", path: "/assets/servers", expectStatus: 200, expectTestID: `data-testid="page-assets-servers"`},
	{name: "assets-printers", path: "/assets/printers", expectStatus: 200, expectTestID: `data-testid="page-assets-printers"`},
	{name: "assets-desks", path: "/assets/desks", expectStatus: 200, expectTestID: `data-testid="page-assets-desks"`},
	{name: "policy-redirect", path: "/policy", expectStatus: 302, redirectsTo: "/policy/catalog"},
	{name: "policy-catalog", path: "/policy/catalog", expectStatus: 200, expectTestID: `data-testid="page-policy-catalog"`},
	{name: "policy-groups", path: "/policy/groups", expectStatus: 200, expectTestID: `data-testid="page-policy-groups"`},
	{name: "policy-modules", path: "/policy/modules", expectStatus: 200, expectTestID: `data-testid="page-policy-modules"`},
	{name: "policy-modules-defaults", path: "/policy/modules/defaults", expectStatus: 200, expectTestID: `data-testid="page-policy-modules-defaults"`},
	{name: "policy-modules-sources", path: "/policy/modules/sources", expectStatus: 200, expectTestID: `data-testid="page-policy-modules-sources"`},
	{name: "profiles", path: "/profiles", expectStatus: 200, expectTestID: `data-testid="page-profiles"`},
	{name: "scripts", path: "/scripts", expectStatus: 200, expectTestID: `data-testid="page-scripts"`},
	// Legacy paths preserved for external bookmarks (Policy Modules moved 2026-05-16).
	{name: "scripts-scripts-legacy", path: "/scripts/scripts", expectStatus: 301, redirectsTo: "/scripts"},
	{name: "scripts-policy-modules-legacy", path: "/scripts/policy-modules", expectStatus: 301, redirectsTo: "/policy/modules"},
	{name: "wine", path: "/wine", expectStatus: 200, expectTestID: `data-testid="page-wine"`},
	{name: "packages-redirect", path: "/packages", expectStatus: 302, redirectsTo: "/packages/managers"},
	{name: "packages-managers", path: "/packages/managers", expectStatus: 200, expectTestID: `data-testid="page-packages-managers"`},
	{name: "packages-packages", path: "/packages/packages", expectStatus: 200, expectTestID: `data-testid="page-packages-packages"`},
	{name: "packages-cycles", path: "/packages/cycles", expectStatus: 200, expectTestID: `data-testid="page-packages-cycles"`},
	{name: "server-admin", path: "/server-admin", expectStatus: 200, expectTestID: `data-testid="page-server-admin"`},
	{name: "preferences", path: "/preferences", expectStatus: 200, expectTestID: `data-testid="page-preferences"`},
}

func TestMountPoints(t *testing.T) {
	e, cookie := newTestServer(t)

	for _, mp := range mountPoints {
		t.Run(mp.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, mp.path, nil)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, mp.expectStatus, rec.Code, "status for %s", mp.path)

			if mp.redirectsTo != "" {
				require.Equal(t, mp.redirectsTo, rec.Header().Get("Location"), "redirect target for %s", mp.path)
				return
			}

			body := rec.Body.String()
			require.Contains(t, body, mp.expectTestID, "missing %s in %s body", mp.expectTestID, mp.path)
		})
	}
}

// TestAssetCanonicalEditorMountedOnEverySubtypeRoute is the R1 / INV-U2
// enforcement test: every /assets/<subtype> route must mount the SAME
// canonical AssetEditor component. If a future change forks the editor
// into per-subtype variants this test fails immediately.
func TestAssetCanonicalEditorMountedOnEverySubtypeRoute(t *testing.T) {
	e, cookie := newTestServer(t)
	for _, slug := range []string{"computers", "servers", "printers", "desks"} {
		t.Run(slug, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/assets/"+slug, nil)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			require.Equal(t, 200, rec.Code, "status for /assets/"+slug)
			body := rec.Body.String()
			require.Contains(t, body, `data-canonical-editor="AssetEditor"`,
				"AssetEditor not mounted on /assets/"+slug+" — R1/INV-U2 violation")
			require.Contains(t, body, `data-asset-subtype="`,
				"data-asset-subtype filter attribute missing on /assets/"+slug)
		})
	}
}

// TestAssetDetailTabsRendered proves the asset detail page renders on
// the standardized DetailShell (Task 7): all 8 tab slugs present, the
// page-asset-detail anchor preserved, and General-tab field values
// carry canonical parameter paths (INV-CPP) via data-path.
func TestAssetDetailTabsRendered(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_asset_detail.db")
	e, cookie := newTestServerAt(t, dbPath)

	// Seed one computer asset directly through the same SQLite file
	// (sequential open → insert → close, so the server and the test
	// never write concurrently).
	ctx := context.Background()
	d, err := database.Open(dbPath)
	require.NoError(t, err, "re-open test db for seeding")
	tenants, err := d.Queries.ListTenants(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, tenants, "setup flow should have created a tenant")
	_, err = d.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "11111111-2222-3333-4444-555555555555",
		TenantID:        tenants[0].ID,
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"test-host-01","os_distribution":"Ubuntu 24.04","os_version":"24.04","architecture":"x86_64"}`,
		EnrollmentState: "enrolled",
		HumanID:         sql.NullString{String: "comp.test.hq.0001", Valid: true},
	})
	require.NoError(t, err, "seed asset")
	require.NoError(t, d.Close())

	req := httptest.NewRequest(http.MethodGet, "/assets/computers/comp.test.hq.0001", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, "asset detail page status")
	body := rec.Body.String()

	require.Contains(t, body, `data-testid="page-asset-detail"`)
	require.Contains(t, body, `data-asset-id="comp.test.hq.0001"`)
	for _, slug := range []string{"general", "groups", "policies", "configuration", "software", "wine-groups", "scripts", "logs"} {
		require.Contains(t, body, `data-tab="`+slug+`"`, "missing tab slug %q", slug)
		require.Contains(t, body, `data-panel="`+slug+`"`, "missing panel for slug %q", slug)
	}
	// Canonical parameter path on the General tab's Name field (INV-CPP).
	require.Contains(t, body, `data-path="computer/identity/name"`)
}

func TestHealthz(t *testing.T) {
	e := NewWithDB(filepath.Join(t.TempDir(), "test_healthz.db"))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	require.Equal(t, "ok", strings.TrimSpace(rec.Body.String()))
}

// TestSetupGateRedirectsFreshDatabase proves the setup gate genuinely
// blocks every route (except /setup itself) until an identity exists.
func TestSetupGateRedirectsFreshDatabase(t *testing.T) {
	e := NewWithDB(filepath.Join(t.TempDir(), "test_fresh.db"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code, "expected 302 redirect to /setup on a fresh database")
	require.Equal(t, "/setup", rec.Header().Get("Location"))
}

// TestRequireAuthRedirectsWithoutSession proves an unauthenticated
// request (no cookie) to a protected route redirects to /login even
// after an identity already exists (distinguishing this from the
// setup-gate case above).
func TestRequireAuthRedirectsWithoutSession(t *testing.T) {
	e, _ := newTestServer(t)                             // identity already seeded by newTestServer's own setup+login flow
	req := httptest.NewRequest(http.MethodGet, "/", nil) // deliberately NOT attaching the cookie
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code, "expected 302 redirect to /login without a session cookie")
	require.Equal(t, "/login", rec.Header().Get("Location"))
}

// TestUserDetailTabsRendered proves the user detail page renders on the
// standardized DetailShell (Task 8): all 4 tab slugs present, the
// page-user-detail anchor preserved, and General-tab field values carry
// canonical parameter paths (INV-CPP) via data-path.
func TestUserDetailTabsRendered(t *testing.T) {
	e, cookie := newTestServerAt(t, filepath.Join(t.TempDir(), "test_user_detail.db"))

	// The /setup flow creates the first identity; it always gets ID 1.
	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, "user detail page status")
	body := rec.Body.String()

	require.Contains(t, body, `data-testid="page-user-detail"`)
	for _, slug := range []string{"general", "groups", "policies", "roles"} {
		require.Contains(t, body, `data-tab="`+slug+`"`, "missing tab slug %q", slug)
		require.Contains(t, body, `data-panel="`+slug+`"`, "missing panel for slug %q", slug)
	}
	// Canonical parameter path on the General tab's Email field (INV-CPP).
	require.Contains(t, body, `data-path="user/identity/email"`)
}

// TestUserGroupMembershipHandlers proves the Task 9 add/remove flow over
// HTTP with CSRF: POST adds the membership (row renders on the detail
// page and an activity row is written), POST remove takes it away again.
func TestUserGroupMembershipHandlers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_user_groups.db")
	e, cookie := newTestServerAt(t, dbPath)

	// Seed one group through the same SQLite file (sequential open,
	// insert, close - no concurrent writers).
	ctx := context.Background()
	d, err := database.Open(dbPath)
	require.NoError(t, err, "re-open test db for seeding")
	tenants, err := d.Queries.ListTenants(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, tenants)
	group, err := d.Queries.CreateGroup(ctx, db.CreateGroupParams{
		TenantID: tenants[0].ID,
		Name:     "Engineering",
		Slug:     "engineering",
	})
	require.NoError(t, err, "seed group")
	require.NoError(t, d.Close())

	// Add: the setup admin (identity 1) joins the group.
	rec := doCSRFPost(t, e, "/users/1", "/users/1/groups",
		url.Values{"group_id": {strconv.FormatInt(group.ID, 10)}}, cookie)
	require.Equal(t, http.StatusFound, rec.Code, "group add should redirect: %s", rec.Body.String())

	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	req.AddCookie(cookie)
	page := httptest.NewRecorder()
	e.ServeHTTP(page, req)
	require.Contains(t, page.Body.String(), "<td>Engineering</td>", "membership row should render on the Groups tab")

	// The mutation must leave an audit trail.
	d, err = database.Open(dbPath)
	require.NoError(t, err)
	activity, err := d.Queries.ListActivityForEntity(ctx, db.ListActivityForEntityParams{
		TenantID:   tenants[0].ID,
		EntityType: "identity",
		EntityID:   1,
		Limit:      10,
	})
	require.NoError(t, err)
	require.NoError(t, d.Close())
	found := false
	for _, a := range activity {
		if a.Event == "group_added" {
			found = true
		}
	}
	require.True(t, found, "group_added activity row should exist, got %+v", activity)

	// Remove again.
	rec = doCSRFPost(t, e, "/users/1", "/users/1/groups/"+strconv.FormatInt(group.ID, 10)+"/remove",
		url.Values{}, cookie)
	require.Equal(t, http.StatusFound, rec.Code, "group remove should redirect: %s", rec.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/users/1", nil)
	req.AddCookie(cookie)
	page = httptest.NewRecorder()
	e.ServeHTTP(page, req)
	require.NotContains(t, page.Body.String(), "<td>Engineering</td>", "membership row should be gone after remove")
}
