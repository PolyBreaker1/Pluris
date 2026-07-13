// Package server tests.
//
// MOUNT-POINT TESTS (per docs/endpoint-management/ui/invariants.md INV-U6, INV-P2):
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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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
// It is the executable form of docs/endpoint-management/ui/invariants.md §VII Concept Registry.
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
	// AD-style groups (Task 6.2): one canonical list at /groups, surfaced
	// in the sidebar under both Users ("User Groups", ?kind=identity) and
	// Assets ("Groups", ?kind=asset).
	{name: "groups", path: "/groups", expectStatus: 200, expectTestID: `data-testid="page-groups"`},
	{name: "groups-kind-identity", path: "/groups?kind=identity", expectStatus: 200, expectTestID: `data-testid="page-groups"`},
	{name: "groups-kind-asset", path: "/groups?kind=asset", expectStatus: 200, expectTestID: `data-testid="page-groups"`},
	{name: "groups-new", path: "/groups/new", expectStatus: 200, expectTestID: `data-testid="page-group-new"`},
	{name: "policy-redirect", path: "/policy", expectStatus: 302, redirectsTo: "/policy/catalog"},
	{name: "policy-catalog", path: "/policy/catalog", expectStatus: 200, expectTestID: `data-testid="page-policy-catalog"`},
	{name: "policy-groups", path: "/policy/groups", expectStatus: 200, expectTestID: `data-testid="page-policy-groups"`},
	{name: "policy-modules", path: "/policy/modules", expectStatus: 200, expectTestID: `data-testid="page-policy-modules"`},
	{name: "policy-modules-defaults", path: "/policy/modules/defaults", expectStatus: 200, expectTestID: `data-testid="page-policy-modules-defaults"`},
	{name: "policy-modules-sources", path: "/policy/modules/sources", expectStatus: 200, expectTestID: `data-testid="page-policy-modules-sources"`},
	{name: "dependency-groups", path: "/policy/dependency-groups", expectStatus: 200, expectTestID: `data-testid="page-dependency-groups"`},
	{name: "pluris-policy", path: "/policy/pluris", expectStatus: 200, expectTestID: `data-testid="page-pluris-policy"`},
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

// TestParamsAPIUnauthenticatedRedirects proves GET /api/params behaves
// like every other /api route without a session: RequireAuth 302s to
// /login before the handler runs (Task 1.2).
func TestParamsAPIUnauthenticatedRedirects(t *testing.T) {
	e, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/params", nil) // no cookie
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code, "expected 302 redirect to /login without a session cookie")
	require.Equal(t, "/login", rec.Header().Get("Location"))
}

// TestParamsAPIAuthenticatedEndToEnd drives GET /api/params through the
// real middleware chain: 200, JSON content type, non-empty sources (the
// seeded super_admin bypasses every grant), and the global no-store
// cache header (no caching surprises for a permission-filtered feed).
func TestParamsAPIAuthenticatedEndToEnd(t *testing.T) {
	e, cookie := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/params", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var resp struct {
		Sources []struct {
			Entity string `json:"entity"`
		} `json:"sources"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Sources, 5, "super_admin must see all five entities")
	require.Equal(t, "computer", resp.Sources[0].Entity)
	require.Equal(t, "user", resp.Sources[4].Entity)
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
	// Task 7: the group name is a link into the new group detail page,
	// not a plain cell.
	require.Contains(t, page.Body.String(), `<td><a href="/groups/`+strconv.FormatInt(group.ID, 10)+`">Engineering</a></td>`, "membership row should render on the Groups tab")

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

// TestUserFullPageCreate proves the Task 8 full-page create flow end to
// end through the real Echo instance (CSRF middleware included): GET
// /users/new renders the standardized layout (not the old small form),
// POST persists every submitted field across sections and redirects
// (302) straight to the new user's detail page, and the edit-only
// endpoint (/users/:id/edit) still round-trips unchanged (the old
// UserFormPage is untouched by this task).
func TestUserFullPageCreate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_user_full_create.db")
	e, cookie := newTestServerAt(t, dbPath)

	// GET renders the full layout, not the small form.
	getReq := httptest.NewRequest(http.MethodGet, "/users/new", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	e.ServeHTTP(getRec, getReq)
	require.Equal(t, 200, getRec.Code)
	require.Contains(t, getRec.Body.String(), `data-testid="page-user-create"`)
	require.Contains(t, getRec.Body.String(), `name="phone_mobile"`)

	// POST creates across identity + organization + contact sections in
	// one round-trip.
	rec := doCSRFPost(t, e, "/users/new", "/users/new", url.Values{
		"username":     {"fullpage"},
		"email":        {"fullpage@test.local"},
		"given_name":   {"Full"},
		"surname":      {"Page"},
		"title":        {"Staff Engineer"},
		"phone_mobile": {"555-0100"},
	}, cookie)
	require.Equal(t, http.StatusFound, rec.Code, "create should redirect: %s", rec.Body.String())
	loc := rec.Header().Get("Location")
	require.True(t, strings.HasPrefix(loc, "/users/"), "redirect target = %q", loc)
	idStr := strings.TrimPrefix(strings.SplitN(loc, "?", 2)[0], "/users/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	require.NoError(t, err, "parse id from redirect %q", loc)

	d, err := database.Open(dbPath)
	require.NoError(t, err, "re-open test db for assertions")
	row, err := d.Queries.GetIdentity(context.Background(), id)
	require.NoError(t, err, "created identity should exist")
	require.Equal(t, "fullpage", row.Username)
	require.Equal(t, "fullpage@test.local", row.Email)
	require.Equal(t, "Full Page", row.DisplayName, "display_name should auto-fill from given+surname")
	require.Equal(t, "Staff Engineer", row.Title.String)
	require.Equal(t, "555-0100", row.PhoneMobile.String)
	require.NoError(t, d.Close())

	// The old edit-only form still works unchanged.
	editReq := httptest.NewRequest(http.MethodGet, "/users/"+idStr+"/edit", nil)
	editReq.AddCookie(cookie)
	editRec := httptest.NewRecorder()
	e.ServeHTTP(editRec, editReq)
	require.Equal(t, 200, editRec.Code)
	require.Contains(t, editRec.Body.String(), `data-testid="user-form"`)
}

// TestPolicyDetailPage proves Task 15: the catalog detail page renders
// on DetailShell, the old popup is gone from the list page, and unknown
// policy ids 404.
func TestPolicyDetailPage(t *testing.T) {
	e, cookie := newTestServerAt(t, filepath.Join(t.TempDir(), "test_policy_detail.db"))

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	// A bundled policy the catalog is known to ship.
	rec := get("/policy/catalog/sec.account.password.min-length")
	require.Equal(t, 200, rec.Code, "policy detail status")
	body := rec.Body.String()
	require.Contains(t, body, `data-testid="page-policy-detail"`)
	for _, slug := range []string{"general", "modules", "assignments"} {
		require.Contains(t, body, `data-tab="`+slug+`"`, "missing tab %q", slug)
	}

	// The popup is gone from the catalog list page.
	rec = get("/policy/catalog")
	require.Equal(t, 200, rec.Code)
	require.NotContains(t, rec.Body.String(), "pdd-dialog", "popup should be deleted")

	// Unknown id 404s.
	rec = get("/policy/catalog/does.not.exist")
	require.Equal(t, 404, rec.Code, "unknown policy id should 404")
}

// TestFieldUpdateCSRFAcceptsHeaderToken is the regression test for the
// review finding that the field-update JSON API was unreachable through
// the real middleware chain: web/static/detail.js's saveSectionEdit sends
// the CSRF token as an X-CSRF-Token header alongside a JSON body (it has
// no form to carry a "_csrf" field), but the CSRF middleware's TokenLookup
// used to be "form:_csrf" only, so the middleware itself -- never reaching
// the handler -- rejected every real save with 400. TokenLookup is now
// "form:_csrf,header:X-CSRF-Token" (see server.go), so this test drives
// the ACTUAL Echo instance via e.ServeHTTP (not a direct handler call, the
// way field_api_test.go's table exercises the handlers) to prove the
// middleware itself accepts a header-borne token, and still rejects a
// request with no token at all.
func TestFieldUpdateCSRFAcceptsHeaderToken(t *testing.T) {
	e, cookie := newTestServer(t)

	// GET any authenticated page to obtain a fresh CSRF cookie; Echo's
	// default CSRF middleware uses the double-submit pattern, so the
	// cookie's value IS the valid token regardless of which page issued it.
	getReq := httptest.NewRequest(http.MethodGet, "/users", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	e.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code)

	var csrfCookie *http.Cookie
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "_csrf" {
			csrfCookie = c
		}
	}
	require.NotNil(t, csrfCookie, "expected a _csrf cookie from GET /users")

	// admin identity seeded by newTestServer's /setup flow is id 1.
	body, err := json.Marshal(map[string]interface{}{
		"section": "identity",
		"fields":  map[string]string{"email": "updated-admin@test.local"},
	})
	require.NoError(t, err)

	postJSON := func(withToken bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/users/1/fields", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		req.AddCookie(csrfCookie)
		if withToken {
			req.Header.Set("X-CSRF-Token", csrfCookie.Value)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	// No token at all: the CSRF middleware must reject before the handler
	// ever runs.
	noTokenRec := postJSON(false)
	require.True(t, noTokenRec.Code == http.StatusBadRequest || noTokenRec.Code == http.StatusForbidden,
		"missing-token POST should be rejected by CSRF middleware, got %d: %s", noTokenRec.Code, noTokenRec.Body.String())

	// Header-borne token: must now pass the middleware AND reach the
	// handler successfully.
	okRec := postJSON(true)
	require.Equal(t, http.StatusOK, okRec.Code, "header-token POST should succeed end-to-end: %s", okRec.Body.String())
}
