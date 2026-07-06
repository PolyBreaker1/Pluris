// Package server wires the Echo router for the Pluris management console.
//
// All routes registered here MUST correspond to entries in the locked
// sidebar (docs/UX_INVARIANTS.md §VI). Mount-point tests in server_test.go
// assert every route renders the expected canonical-editor anchor.
package server

import (
	"log"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/pluris/pluris/console/handlers"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/database"
)

// New returns a configured Echo router using the default "pluris.db" path.
func New() *echo.Echo {
	return NewWithDB("pluris.db")
}

// NewWithDB returns a configured Echo router using dbPath for the SQLite
// database. Exists so tests can point at an isolated, throwaway file
// instead of sharing the real dev database.
func NewWithDB(dbPath string) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(emw.Recover())
	e.Use(emw.Logger())
	e.Use(emw.Gzip())

	// Initialize database
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized successfully")

	// Wire database into handlers
	h := handlers.New(db)

	sessions := auth.NewSessionManager(db)

	// Full auth chain, in order: SetupGate (redirect to /setup until an
	// identity exists) -> RequireAuth (resolve session cookie, redirect
	// to /login otherwise) -> RequireRole (RBAC against the resolved
	// session) -> CSRF (validates state-changing requests).
	//
	// NOTE on CSRF Skipper: intentionally NOT skipping GET requests here.
	// Echo's CSRF middleware already only *validates* tokens on unsafe
	// methods (POST/PUT/PATCH/DELETE) internally — GET/HEAD/OPTIONS/TRACE
	// never get validated regardless of Skipper. But the middleware ALSO
	// issues the CSRF cookie and stashes the token in context (c.Set("csrf",
	// token)) as part of that same, non-skippable-without-losing-both code
	// path. A Skipper that skips GET would skip issuing the token too, so
	// the GET-rendered /setup and /login forms would embed an empty
	// hidden _csrf field, and the subsequent POST would always fail
	// validation (token never issued == never matches). Leaving Skipper
	// unset (default: never skip) is what makes the token round-trip work.
	e.Use(auth.SetupGate(db))
	e.Use(auth.RequireAuth(db, sessions))
	e.Use(auth.RequireRole())
	e.Use(emw.CSRFWithConfig(emw.CSRFConfig{
		TokenLookup: "form:_csrf",
	}))

	// Re-stash the CSRF token (set by the middleware above via c.Set,
	// which only lives on echo.Context) into the request's
	// context.Context, so templ components — which only ever see a
	// context.Context via their generated Render method — can render a
	// working hidden _csrf field (e.g. the header's logout / tenant-switch
	// forms) without every page handler threading the token through its
	// own function signature.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token, _ := c.Get("csrf").(string)
			c.SetRequest(c.Request().WithContext(auth.WithCSRFToken(c.Request().Context(), token)))
			return next(c)
		}
	})

	// Health check (not in sidebar).
	e.GET("/healthz", func(c echo.Context) error { return c.String(200, "ok") })

	// Static assets — shared lists.js / lists.css power every list table
	// (INV-L9). Served from web/static/. Path is process-cwd-relative;
	// the binary must be run from the repo root or via `make dev`.
	e.Static("/static", "web/static")

	// Auth (not in sidebar). Gated by SetupGate/RequireAuth/RequireRole/CSRF
	// above via authExemptPaths/setupExemptPaths in pkg/auth/middleware.go.
	e.GET("/setup", h.SetupShow)
	e.POST("/setup", h.SetupSubmit)
	e.GET("/login", h.LoginShow)
	e.POST("/login", h.LoginSubmit)
	e.POST("/logout", h.LogoutSubmit)
	e.POST("/tenant-switch", h.TenantSwitchSubmit)

	// 10 top-level sidebar items per docs/UX_INVARIANTS.md §VI.

	// 1. Dashboard
	e.GET("/", h.Dashboard)

	// 2. Users
	e.GET("/users", h.Users)
	e.GET("/users/new", h.UserNewShow)
	e.POST("/users/new", h.UserCreateSubmit)
	e.GET("/users/:id", h.UserDetail)
	e.GET("/users/:id/edit", h.UserEditShow)
	e.POST("/users/:id/edit", h.UserUpdateSubmit)
	e.POST("/users/:id/delete", h.UserDeleteSubmit)
	e.POST("/users/:id/groups", h.UserGroupAdd)
	e.POST("/users/:id/groups/:groupID/remove", h.UserGroupRemove)
	e.POST("/users/:id/roles", h.UserRoleAssign)
	e.POST("/users/:id/roles/:roleID/remove", h.UserRoleRemove)
	e.GET("/users/:id/policies/add", h.UserPolicyAdd)
	e.POST("/users/:id/policies/add", h.UserPolicyAddSubmit)

	// 3. Assets — 4 subtype tabs all mount the SAME canonical editor (INV-U2).
	e.GET("/assets", h.AssetsRedirect)
	e.GET("/assets/computers", h.AssetsComputers)
	e.GET("/assets/servers", h.AssetsServers)
	e.GET("/assets/printers", h.AssetsPrinters)
	e.GET("/assets/desks", h.AssetsDesks)
	// Asset detail page — full page view of a single asset.
	e.GET("/assets/:subtype/:id", h.AssetDetail)
	e.POST("/assets/:subtype/:id/owner", h.AssetSetOwner)
	e.POST("/assets/:subtype/:id/groups", h.AssetGroupAdd)
	e.POST("/assets/:subtype/:id/groups/:groupID/remove", h.AssetGroupRemove)
	e.GET("/assets/:subtype/:id/policies/add", h.AssetPolicyAdd)
	e.POST("/assets/:subtype/:id/policies/add", h.AssetPolicyAddSubmit)

	// 4. Policy — three sub-tabs (Catalog + Configuration Groups + Modules).
	e.GET("/policy", h.PolicyRedirect)
	e.GET("/policy/catalog", h.PolicyCatalog)
	e.GET("/policy/groups", h.PolicyGroups)
	e.GET("/policy/modules", h.PolicyModules)
	e.GET("/policy/modules/defaults", h.PolicyModulesDefaults)
	e.GET("/policy/modules/sources", h.PolicyModulesSources)

	// 5. Profiles
	e.GET("/profiles", h.Profiles)

	// 6. Scripts — single page (ad-hoc admin scripts only; Policy Modules
	//    moved to /policy/modules on 2026-05-16).
	e.GET("/scripts", h.Scripts)
	// Legacy paths — preserve external bookmarks.
	e.GET("/scripts/scripts", func(c echo.Context) error { return c.Redirect(301, "/scripts") })
	e.GET("/scripts/policy-modules", func(c echo.Context) error { return c.Redirect(301, "/policy/modules") })

	// 7. Wine
	e.GET("/wine", h.Wine)

	// 8. Package Management — 3 tabs.
	e.GET("/packages", h.PackagesRedirect)
	e.GET("/packages/managers", h.PackagesManagers)
	e.GET("/packages/packages", h.PackagesPackages)
	e.GET("/packages/cycles", h.PackagesCycles)

	// 9. Server Administration
	e.GET("/server-admin", h.ServerAdmin)

	// 10. User/Admin Preferences
	e.GET("/preferences", h.Preferences)

	return e
}
