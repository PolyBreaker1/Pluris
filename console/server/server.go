// Package server wires the Echo router for the Pluris management console.
//
// All routes registered here MUST correspond to entries in the locked
// sidebar (docs/endpoint-management/ui/invariants.md §VI). Mount-point tests in server_test.go
// assert every route renders the expected canonical-editor anchor.
package server

import (
	"context"
	"log"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/console/handlers"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
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

	// Pre-beta correctness over caching: force every response to skip the
	// browser cache so stale HTML/JS can never mask a fix. Revisit before
	// production (this defeats conditional requests and static-asset
	// caching wholesale).
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("Cache-Control", "no-store")
			return next(c)
		}
	})

	// Initialize database
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized successfully")

	// Wire database into handlers
	h := handlers.New(db)

	// Seed the bundled Policy Modules (idempotent) and install the live
	// catalog provider used by catalog/policymodules' pure helpers
	// (CandidatesForPolicy, the extension-framework loader, the Policy
	// Catalog table's candidate-module column) -- these run outside any
	// request's tenant context, so they read the tenant-agnostic bundled
	// set. Per-tenant reads (the Modules Library/Defaults pages) go
	// through the service directly from the handler, not this provider.
	moduleSvc := services.NewPolicyModuleService(db)
	if err := moduleSvc.SeedBundled(context.Background()); err != nil {
		log.Printf("warning: failed to seed bundled policy modules: %v", err)
	}
	policymodules.SetCatalogProvider(func() []policymodules.Module {
		mods, err := moduleSvc.ListBundled(context.Background())
		if err != nil {
			log.Printf("warning: failed to load bundled policy modules: %v", err)
			return nil
		}
		return mods
	})

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
		// Form lookup FIRST so existing HTML forms (login/setup/logout/
		// tenant-switch) behave identically. header:X-CSRF-Token is added so
		// JSON XHR/fetch callers (e.g. web/static/detail.js's inline-edit
		// save, which POSTs application/json and cannot carry a form field)
		// can also pass CSRF validation. Echo v4 CSRFConfig.TokenLookup
		// supports comma-separated multi-source lookup, trying each in order.
		TokenLookup: "form:_csrf,header:X-CSRF-Token",
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

	// 10 top-level sidebar items per docs/endpoint-management/ui/invariants.md §VI.

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
	// Enroll ("New asset") — minimal create-then-edit-inline form.
	// Registered before the /:subtype/:id wildcard below so "new" is
	// never captured as an id (same trick as the dependency-groups routes).
	e.GET("/assets/:subtype/new", h.AssetNewShow)
	e.POST("/assets/:subtype/new", h.AssetCreateSubmit)
	// Asset detail page — full page view of a single asset.
	e.GET("/assets/:subtype/:id", h.AssetDetail)
	e.POST("/assets/:subtype/:id/owner", h.AssetSetOwner)
	e.POST("/assets/:subtype/:id/groups", h.AssetGroupAdd)
	e.POST("/assets/:subtype/:id/groups/:groupID/remove", h.AssetGroupRemove)
	e.GET("/assets/:subtype/:id/policies/add", h.AssetPolicyAdd)
	e.POST("/assets/:subtype/:id/policies/add", h.AssetPolicyAddSubmit)

	// 4. Policy — sub-tabs (Catalog + Configuration Groups + Modules +
	// Dependency Groups).
	e.GET("/policy", h.PolicyRedirect)
	e.GET("/policy/catalog", h.PolicyCatalog)
	e.GET("/policy/catalog/:id", h.PolicyCatalogDetail)
	// Configuration Groups (Task 5.2) — registry-driven list + create +
	// DetailShell detail with Assignments/Bindings tab routes. "new" is
	// registered before the ":id" wildcard so it's never captured as an
	// id (same trick as the module/dependency-group routes below).
	e.GET("/policy/groups", h.PolicyGroups)
	e.GET("/policy/groups/new", h.PolicyGroupNew)
	e.POST("/policy/groups/new", h.PolicyGroupCreate)
	e.GET("/policy/groups/:id", h.PolicyGroupDetail)
	e.POST("/policy/groups/:id/delete", h.PolicyGroupDelete)
	e.POST("/policy/groups/:id/assignments", h.PolicyGroupAssignmentAdd)
	e.POST("/policy/groups/:id/assignments/:aid", h.PolicyGroupAssignmentUpdate)
	e.POST("/policy/groups/:id/assignments/:aid/remove", h.PolicyGroupAssignmentRemove)
	e.POST("/policy/groups/:id/bindings", h.PolicyGroupBindingAdd)
	e.POST("/policy/groups/:id/bindings/:bid", h.PolicyGroupBindingUpdate)
	e.POST("/policy/groups/:id/bindings/:bid/remove", h.PolicyGroupBindingRemove)
	e.GET("/policy/modules", h.PolicyModules)
	e.GET("/policy/modules/defaults", h.PolicyModulesDefaults)
	e.GET("/policy/modules/sources", h.PolicyModulesSources)
	// Module detail/create editor (Task 4.3). "new" is registered before
	// the ":id" wildcard so it's never captured as an id (same trick as
	// the asset-enroll and dependency-groups routes above).
	e.GET("/policy/modules/new", h.PolicyModuleNewShow)
	e.POST("/policy/modules/new", h.PolicyModuleCreateSubmit)
	e.GET("/policy/modules/:id", h.PolicyModuleDetail)
	e.POST("/policy/modules/:id/delete", h.PolicyModuleDeleteSubmit)
	e.POST("/policy/modules/:id/clone", h.PolicyModuleClone)
	e.POST("/policy/modules/:id/versions", h.PolicyModuleVersionCreate)
	e.POST("/policy/modules/:id/versions/:vid/publish", h.PolicyModuleVersionPublish)
	e.POST("/policy/modules/:id/versions/:vid/revoke", h.PolicyModuleVersionRevoke)
	e.GET("/policy/dependency-groups", h.DependencyGroups)
	e.GET("/policy/dependency-groups/new", h.DependencyGroupNew)
	e.POST("/policy/dependency-groups", h.DependencyGroupCreate)
	e.GET("/policy/dependency-groups/:id", h.DependencyGroupDetail)
	e.POST("/policy/dependency-groups/:id", h.DependencyGroupUpdate)
	e.POST("/policy/dependency-groups/:id/delete", h.DependencyGroupDelete)
	e.POST("/policy/dependency-groups/:id/conditions", h.DependencyGroupConditionAdd)
	e.POST("/policy/dependency-groups/:id/conditions/:condID/remove", h.DependencyGroupConditionRemove)
	e.POST("/policy/dependency-groups/:id/match-mode", h.DependencyGroupMatchModeUpdate)
	e.POST("/policy/modules/:moduleID/dependencies", h.ModuleDependencyAdd)
	e.POST("/policy/modules/:moduleID/dependencies/:groupID/remove", h.ModuleDependencyRemove)
	e.GET("/policy/pluris", h.PlurisPolicy)
	e.POST("/policy/pluris", h.PlurisPolicyCreate)
	e.GET("/policy/pluris/:id", h.PlurisPolicyDetail)
	e.POST("/policy/pluris/:id/clone", h.PlurisPolicyClone)
	e.POST("/policy/pluris/:id/parent", h.PlurisPolicySetParent)
	e.POST("/policy/pluris/:id/settings", h.PlurisPolicyRename)
	e.POST("/policy/pluris/:id", h.PlurisPolicySave)
	e.POST("/policy/pluris/:id/delete", h.PlurisPolicyDelete)

	// AD-style Groups (Task 6.2): the canonical /groups list (surfaced in
	// the sidebar under BOTH Users → "User Groups" and Assets → "Groups";
	// the kind query param picks the view), create form, and the upgraded
	// detail page (General/Members/Rules/Roles). Route-level RBAC:
	// "/groups" is gated on group.view in pkg/auth/rbac.go; each mutation
	// handler repeats its finer group.* key (create/update/delete/
	// manage_members). The role-assignment mutations (RBAC v2 Task 4)
	// stay gated on console_access.manage_role_assignments. "new" wins
	// over ":id" via echo's static-over-param routing.
	e.GET("/groups", h.GroupsList)
	e.GET("/groups/new", h.GroupNew)
	e.POST("/groups/new", h.GroupCreate)
	e.GET("/groups/:id", h.GroupDetail)
	e.POST("/groups/:id/delete", h.GroupDelete)
	e.POST("/groups/:id/members", h.GroupMemberAdd)
	e.POST("/groups/:id/members/:mid/remove", h.GroupMemberRemove)
	e.POST("/groups/:id/rules", h.GroupRuleAdd)
	e.POST("/groups/:id/rules/:rid/remove", h.GroupRuleRemove)
	e.POST("/groups/:id/match-mode", h.GroupMatchModeUpdate)
	e.POST("/groups/:id/recalculate", h.GroupRecalculate)
	e.POST("/groups/:id/roles", h.GroupRoleAssign)
	e.POST("/groups/:id/roles/:roleID/remove", h.GroupRoleRemove)

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

	// Inline-edit field-update API (Task 8, flagship authz consumer).
	// "/api/*" is not listed in pkg/auth/rbac.go's route-permission map,
	// so RoutePermissionKey longest-prefix-matches "/" -> "" (session
	// middleware only requires an authenticated session at the route
	// level); the handler-level requirePermissionScoped calls inside
	// UserFieldUpdate/AssetFieldUpdate are the actual authorization gate
	// here, same pattern as the Pluris Policy handlers.
	e.POST("/api/users/:id/fields", h.UserFieldUpdate)
	e.POST("/api/assets/:subtype/:id/fields", h.AssetFieldUpdate)

	// Policy Module editor field-update + script-save APIs (Task 4.3).
	// Route-level RBAC: "/api/modules" is registered in pkg/auth/rbac.go's
	// route-permission map with endpoint_policy.manage_modules (the same
	// key as the /policy/modules pages these serve) as coarse
	// defense-in-depth; the handler-level ModuleCanEdit checks remain the
	// fine-grained gate, mirroring UserFieldUpdate/AssetFieldUpdate's
	// handler-side pattern.
	e.POST("/api/modules/:id/fields", h.PolicyModuleFieldUpdate)
	e.POST("/api/modules/:id/versions/:vid/fields", h.PolicyModuleVersionFieldUpdate)
	e.POST("/api/modules/:id/versions/:vid/scripts", h.PolicyModuleScriptSave)

	// Configuration Group General-tab inline-edit API (Task 5.2). Route-
	// level RBAC: "/api/config-groups" is registered in pkg/auth/rbac.go
	// with endpoint_policy.manage_config_groups; the handler repeats the
	// same requirePermission check as the fine-grained layer.
	e.POST("/api/config-groups/:id/fields", h.ConfigGroupFieldUpdate)

	// Group General-tab inline-edit API (Task 6.2). Route-level RBAC:
	// "/api/*" falls through to the open "/" default, so the handler's
	// requirePermission(group.update) is the gate — same pattern as the
	// user/asset field APIs above.
	e.POST("/api/groups/:id/fields", h.GroupFieldUpdate)

	// Avatar upload (Task 9). Same auth shape as the field-update API
	// above: the route only requires an authenticated session, and
	// UserAvatarUpload's requirePermissionScoped(identity.update) call is
	// the real gate.
	e.POST("/api/users/:id/avatar", h.UserAvatarUpload)

	// Parameter registry API (Task 1.2): the single JSON feed for the
	// module editor parameter tree and dependency-condition builder.
	// Registered as "" (open to any authenticated session) in
	// pkg/auth/rbac.go's route-permission map — the response itself is
	// per-grant filtered inside ParamsAPI via params.VisibleDefs, so the
	// filtering IS the authorization, same shape as the /api routes above.
	e.GET("/api/params", h.ParamsAPI)

	// Avatar static files. Deliberately left BEHIND the full auth chain
	// (no isStaticPath/authExemptPaths exemption) rather than exempted
	// like /static -- avatars are private-by-default, and same-origin
	// <img> requests still send the session cookie, so authed pages that
	// reference /avatars/<id>.<ext> render fine without an exemption.
	e.Static("/avatars", handlers.AvatarDir)

	return e
}
