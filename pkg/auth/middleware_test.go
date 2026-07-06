package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func setupMiddlewareTestDB(t *testing.T) *database.Database {
	t.Helper()
	dbPath := "test_auth_middleware.db"
	t.Cleanup(func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-shm")
		os.Remove(dbPath + "-wal")
	})
	dbase, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { dbase.Close() })
	return dbase
}

func TestSetupGateRedirectsWhenNoIdentitiesExist(t *testing.T) {
	dbase := setupMiddlewareTestDB(t)
	e := echo.New()
	e.Use(SetupGate(dbase))
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "dashboard") })
	e.GET("/setup", func(c echo.Context) error { return c.String(http.StatusOK, "setup") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect to /setup, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("expected redirect to /setup, got %q", loc)
	}
}

func TestSetupGateAllowsThroughOnceAnIdentityExists(t *testing.T) {
	dbase := setupMiddlewareTestDB(t)
	ctx := context.Background()
	tenant, _ := dbase.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Org", Slug: "org-mw"})
	dbase.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "a", Email: "a@example.com",
		DisplayName: "A", Role: "super_admin",
	})

	e := echo.New()
	e.Use(SetupGate(dbase))
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "dashboard") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireAuthRedirectsToLoginWithoutSession(t *testing.T) {
	dbase := setupMiddlewareTestDB(t)
	sessions := NewSessionManager(dbase)

	e := echo.New()
	e.Use(RequireAuth(dbase, sessions))
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "dashboard") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect to /login, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireAuthPopulatesSessionOnValidCookie(t *testing.T) {
	dbase := setupMiddlewareTestDB(t)
	sessions := NewSessionManager(dbase)
	ctx := context.Background()

	tenant, _ := dbase.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Org", Slug: "org-mw2"})
	identity, _ := dbase.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "b", Email: "b@example.com",
		DisplayName: "B", Role: "admin",
	})
	rawToken, _, err := sessions.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	var gotIdentityID int64
	var gotRole identities.Role
	e := echo.New()
	e.Use(RequireAuth(dbase, sessions))
	e.GET("/", func(c echo.Context) error {
		sess := FromContext(c.Request().Context())
		if sess == nil {
			t.Fatal("expected session in context")
		}
		gotIdentityID = sess.IdentityID
		gotRole = sess.Role
		return c.String(http.StatusOK, "dashboard")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "pluris_session", Value: rawToken})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotIdentityID != identity.ID {
		t.Fatalf("expected identity id %d in context, got %d", identity.ID, gotIdentityID)
	}
	if gotRole != identities.RoleAdmin {
		t.Fatalf("expected role admin in context, got %q", gotRole)
	}
}

func TestRequireAuthIgnoresActiveTenantForNonSuperAdmin(t *testing.T) {
	dbase := setupMiddlewareTestDB(t)
	sessions := NewSessionManager(dbase)
	ctx := context.Background()

	homeTenant, _ := dbase.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Home", Slug: "org-home"})
	otherTenant, _ := dbase.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Other", Slug: "org-other"})
	identity, _ := dbase.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: homeTenant.ID, Username: "c", Email: "c@example.com",
		DisplayName: "C", Role: "admin",
	})
	rawToken, _, err := sessions.Create(ctx, identity.ID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Simulate a stale active-tenant switch (e.g. from before a role
	// demotion) directly on the session row.
	sess, err := sessions.Lookup(ctx, rawToken)
	if err != nil {
		t.Fatalf("failed to look up session: %v", err)
	}
	if err := sessions.SetActiveTenant(ctx, sess.ID, otherTenant.ID); err != nil {
		t.Fatalf("failed to set active tenant: %v", err)
	}

	var gotTenantID int64
	e := echo.New()
	e.Use(RequireAuth(dbase, sessions))
	e.GET("/", func(c echo.Context) error {
		userSess := FromContext(c.Request().Context())
		if userSess == nil {
			t.Fatal("expected session in context")
		}
		gotTenantID = userSess.TenantID
		return c.String(http.StatusOK, "dashboard")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "pluris_session", Value: rawToken})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotTenantID != homeTenant.ID {
		t.Fatalf("expected effective tenant to be identity's home tenant %d (non-super_admin must not honor ActiveTenantID), got %d", homeTenant.ID, gotTenantID)
	}
}

func TestRBACDeniesUserSelfServiceFromScripts(t *testing.T) {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.SetRequest(c.Request().WithContext(WithSession(c.Request().Context(), &UserSession{
				IdentityID: 1, Role: identities.RoleUser,
			})))
			return next(c)
		}
	})
	e.Use(RequireRole())
	e.GET("/scripts", func(c echo.Context) error { return c.String(http.StatusOK, "scripts") })

	req := httptest.NewRequest(http.MethodGet, "/scripts", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRBACAllowsAdminFromScripts(t *testing.T) {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.SetRequest(c.Request().WithContext(WithSession(c.Request().Context(), &UserSession{
				IdentityID: 1, Role: identities.RoleAdmin,
			})))
			return next(c)
		}
	})
	e.Use(RequireRole())
	e.GET("/scripts", func(c echo.Context) error { return c.String(http.StatusOK, "scripts") })

	req := httptest.NewRequest(http.MethodGet, "/scripts", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
