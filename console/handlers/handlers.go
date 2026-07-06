// Package handlers implements one HTTP handler per top-level sidebar
// route. Each handler renders a templ page from web/templates; the
// data-testid anchor on each page is asserted by the mount-point test
// in console/server/server_test.go.
package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

type Handler struct {
	db            *database.Database
	assetSvc      *services.AssetService
	identitySvc   *services.IdentityService
	groupSvc      *services.GroupService
	roleSvc       *services.RoleService
	assignmentSvc *services.AssignmentService
	sessions      *auth.SessionManager
}

func New(db *database.Database) *Handler {
	return &Handler{
		db:            db,
		assetSvc:      services.NewAssetService(db),
		identitySvc:   services.NewIdentityService(db),
		groupSvc:      services.NewGroupService(db),
		roleSvc:       services.NewRoleService(db),
		assignmentSvc: services.NewAssignmentService(db),
		sessions:      auth.NewSessionManager(db),
	}
}

// render writes a templ.Component as HTML.
func render(c echo.Context, comp templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	return comp.Render(c.Request().Context(), c.Response().Writer)
}

// Dashboard renders the top-level overview page (item 1 in the sidebar).
func (h *Handler) Dashboard(c echo.Context) error {
	return render(c, templates.DashboardPage())
}

// Users renders the identity directory page (item 2 in the sidebar).
// Mirrors assetsPage: fetch real rows scoped to the caller's effective
// tenant, then hand them to the templ page.
func (h *Handler) Users(c echo.Context) error {
	ctx := c.Request().Context()

	sess := auth.FromContext(ctx)
	tenantID := sess.TenantID

	rows, err := h.identitySvc.List(ctx, tenantID, 200, 0)
	if err != nil {
		return err
	}

	return render(c, templates.UsersPage(rows))
}

// UserDetail renders the full-page detail view of a single user, including
// the assets currently assigned to them.
func (h *Handler) UserDetail(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	user, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	assigned, err := h.identitySvc.ListAssignedAssets(ctx, sess.TenantID, id)
	if err != nil {
		return err
	}
	groups, err := h.groupSvc.ListForIdentity(ctx, id)
	if err != nil {
		return err
	}
	allGroups, err := h.groupSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	// Builtin roles are seeded at setup; EnsureBuiltins here upgrades
	// tenants created before the roles feature existed (idempotent).
	if err := h.roleSvc.EnsureBuiltins(ctx, sess.TenantID); err != nil {
		return err
	}
	roles, err := h.roleSvc.ListForIdentityDetail(ctx, id)
	if err != nil {
		return err
	}
	allRoles, err := h.roleSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	groupIDs := make([]int64, 0, len(groups))
	for _, g := range groups {
		groupIDs = append(groupIDs, g.ID)
	}
	applied, err := h.assignmentSvc.ResolveForTarget(ctx, sess.TenantID, "identity", id, groupIDs, 0)
	if err != nil {
		return err
	}
	return render(c, templates.UserDetailPage(user, assigned, csrfTokenFrom(c), groups, allGroups, roles, allRoles, applied))
}

// UserNewShow renders the empty "add user" form.
func (h *Handler) UserNewShow(c echo.Context) error {
	return render(c, templates.UserFormPage(identities.Identity{}, csrfTokenFrom(c), "", true))
}

// UserCreateSubmit creates a new identity from the "add user" form.
func (h *Handler) UserCreateSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	in := identities.Identity{
		Username:    c.FormValue("username"),
		Email:       c.FormValue("email"),
		DisplayName: c.FormValue("display_name"),
		Title:       c.FormValue("title"),
		Department:  c.FormValue("department"),
		Role:        identities.RoleUser,
	}
	if in.Username == "" || in.Email == "" || in.DisplayName == "" {
		return render(c, templates.UserFormPage(in, csrfTokenFrom(c), "Username, email, and display name are required.", true))
	}
	created, err := h.identitySvc.Create(ctx, sess.TenantID, in)
	if err != nil {
		log.Printf("user create failed: %v", err)
		return render(c, templates.UserFormPage(in, csrfTokenFrom(c), "Could not create user. Check the username/email aren't already taken.", true))
	}
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(created.ID, 10))
}

// UserEditShow renders the edit form pre-filled with the existing user.
func (h *Handler) UserEditShow(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	user, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	return render(c, templates.UserFormPage(user, csrfTokenFrom(c), "", false))
}

// UserUpdateSubmit writes back the editable fields of an existing user.
func (h *Handler) UserUpdateSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	existing, err := h.identitySvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	// Username is intentionally NOT taken from the form: UpdateIdentity's
	// SQL never writes it back (username-as-login-identifier changing
	// silently under a user is its own can of worms, left for later), and
	// UserFormPage renders it read-only in edit mode. existing.Username is
	// left as-is here so an accidental client-side edit is a no-op rather
	// than a silently-swallowed write.
	existing.Email = c.FormValue("email")
	existing.DisplayName = c.FormValue("display_name")
	existing.Title = c.FormValue("title")
	existing.Department = c.FormValue("department")

	if existing.Email == "" || existing.DisplayName == "" {
		return render(c, templates.UserFormPage(existing, csrfTokenFrom(c), "Email and display name are required.", false))
	}

	if _, err := h.identitySvc.Update(ctx, existing); err != nil {
		log.Printf("user update failed: %v", err)
		return render(c, templates.UserFormPage(existing, csrfTokenFrom(c), "Could not save changes.", false))
	}
	return c.Redirect(http.StatusFound, "/users/"+strconv.FormatInt(id, 10))
}

// UserDeleteSubmit permanently removes a user.
//
// Safety guard: a caller may not delete their OWN identity. Nothing else in
// this handler chain enforces row-level self-scoping yet (RBAC is
// route-prefix-only per pkg/auth/rbac.go's permission map, and
// "user"-role callers already reach every /users/* route), so
// without this check a logged-in user — self-service or admin — could
// delete their own active account out from under themselves via a stray
// POST. A "last remaining super_admin" guard was deliberately NOT added:
// it would require a new cross-tenant query IdentityService doesn't have
// today (super_admin status isn't scoped to the caller's tenant), which is
// more surface than a single simple check — left as a follow-up.
func (h *Handler) UserDeleteSubmit(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	if sess := auth.FromContext(ctx); sess != nil && sess.IdentityID == id {
		return echo.NewHTTPError(http.StatusBadRequest, "you cannot delete your own account")
	}
	if err := h.identitySvc.Delete(ctx, id); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/users")
}

// Assets — all four subtype tabs (computers / servers / printers / desks)
// render the SAME canonical editor with a `subtype=` filter. The redirect
// handler below points the bare /assets URL at the computers tab.

// AssetsRedirect 302-redirects /assets to /assets/computers (the default tab).
func (h *Handler) AssetsRedirect(c echo.Context) error {
	return c.Redirect(302, "/assets/computers")
}

func (h *Handler) AssetsComputers(c echo.Context) error {
	return h.assetsPage(c, "computers", "computer")
}

func (h *Handler) AssetsServers(c echo.Context) error {
	return h.assetsPage(c, "servers", "server")
}

func (h *Handler) AssetsPrinters(c echo.Context) error {
	return h.assetsPage(c, "printers", "printer")
}

func (h *Handler) AssetsDesks(c echo.Context) error {
	return h.assetsPage(c, "desks", "desk")
}

// assetsPage fetches assets from database and renders the page
func (h *Handler) assetsPage(c echo.Context, subtypeSlug, subtypeValue string) error {
	ctx := c.Request().Context()

	sess := auth.FromContext(ctx)
	tenantID := sess.TenantID

	// Fetch assets from database
	assets, err := h.assetSvc.ListBySubtype(ctx, tenantID, subtypeValue)
	if err != nil {
		return err
	}

	return render(c, templates.AssetsPage(subtypeSlug, assets))
}

// AssetDetail renders the full-page detail view of a single asset.
// The :subtype param provides breadcrumb context; :id is the asset's
// stable ID. If the asset doesn't exist, redirects to the subtype list.
func (h *Handler) AssetDetail(c echo.Context) error {
	ctx := c.Request().Context()
	subtype := c.Param("subtype")
	id := c.Param("id")

	// Fetch asset from database
	asset, err := h.assetSvc.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// If not found, show "not found" page
	if asset == nil {
		return render(c, templates.AssetDetailPage(subtype, id))
	}

	sess := auth.FromContext(ctx)
	owners, err := h.identitySvc.List(ctx, sess.TenantID, 200, 0)
	if err != nil {
		return err
	}

	// Software + Logs tab rows. The catalog Asset carries the human ID;
	// the inventory tables key on the numeric DB id, so resolve it. A
	// resolution failure is impossible here (GetByID just succeeded),
	// but fail soft to empty tabs rather than a 500 either way.
	var software []db.InstalledSoftware
	var logs []db.ActivityLog
	var groups []services.GroupRow
	var applied []services.AppliedPolicy
	if dbID, rerr := h.assetSvc.ResolveDBID(ctx, id); rerr == nil {
		if rows, qerr := h.db.Queries.ListSoftwareForAsset(ctx, dbID); qerr == nil {
			software = rows
		}
		if rows, qerr := h.db.Queries.ListActivityForEntity(ctx, db.ListActivityForEntityParams{
			TenantID:   sess.TenantID,
			EntityType: "asset",
			EntityID:   dbID,
			Limit:      100,
		}); qerr == nil {
			logs = rows
		}
		if rows, qerr := h.groupSvc.ListForAsset(ctx, dbID); qerr == nil {
			groups = rows
		}
		groupIDs := make([]int64, 0, len(groups))
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
		}
		var siteID int64
		if row, qerr := h.db.Queries.GetAsset(ctx, dbID); qerr == nil && row.SiteID.Valid {
			siteID = row.SiteID.Int64
		}
		if rows, qerr := h.assignmentSvc.ResolveForTarget(ctx, sess.TenantID, "asset", dbID, groupIDs, siteID); qerr == nil {
			applied = rows
		}
	}
	allGroups, err := h.groupSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}

	return render(c, templates.AssetDetailPageWithData(subtype, *asset, owners, csrfTokenFrom(c), software, logs, groups, allGroups, applied))
}

// AssetSetOwner assigns or clears the owning identity of an asset. An
// empty or "0" owner_id form value clears ownership; any other value
// must resolve to a real identity ID.
func (h *Handler) AssetSetOwner(c echo.Context) error {
	ctx := c.Request().Context()
	subtype := c.Param("subtype")
	humanOrUUID := c.Param("id")

	dbID, err := h.assetSvc.ResolveDBID(ctx, humanOrUUID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "asset not found")
	}

	ownerIDStr := c.FormValue("owner_id")
	if ownerIDStr == "" || ownerIDStr == "0" {
		if err := h.assetSvc.ClearOwner(ctx, dbID); err != nil {
			return err
		}
	} else {
		ownerID, err := strconv.ParseInt(ownerIDStr, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid owner_id")
		}
		// The owner picker's <select> is built from the caller's own
		// tenant (see AssetDetail above), but nothing stops a forged POST
		// from naming an identity ID belonging to a different tenant —
		// verify it resolves to an identity in the caller's own tenant
		// before writing it as the asset's owner.
		sess := auth.FromContext(ctx)
		owner, err := h.identitySvc.Get(ctx, ownerID)
		if err != nil || sess == nil || owner.TenantID != sess.TenantID {
			return echo.NewHTTPError(http.StatusForbidden, "owner must belong to your tenant")
		}
		if err := h.assetSvc.SetOwner(ctx, dbID, ownerID); err != nil {
			return err
		}
	}

	return c.Redirect(http.StatusFound, "/assets/"+subtype+"/"+humanOrUUID)
}

// Policy — three sub-tabs (Catalog / Configuration Groups / Modules) all
// live under /policy/* and share PolicyTabs in the page chrome. Modules
// moved here from /scripts/policy-modules on 2026-05-16: a Policy Module
// IS a policy concept, not an automation concept.

// PolicyRedirect 302-redirects /policy to /policy/catalog (the default tab).
func (h *Handler) PolicyRedirect(c echo.Context) error {
	return c.Redirect(302, "/policy/catalog")
}
func (h *Handler) PolicyCatalog(c echo.Context) error {
	return render(c, templates.PolicyCatalogPage())
}
func (h *Handler) PolicyGroups(c echo.Context) error {
	return render(c, templates.PolicyConfigurationGroupsPage())
}

// PolicyModules — Library view. Three sub-tabs (library / defaults /
// sources) share the same page chrome; the canonical editor remains
// editors/PolicyModuleEditor (R1: single source of truth).
func (h *Handler) PolicyModules(c echo.Context) error {
	return render(c, templates.PolicyModulesPage("library"))
}
func (h *Handler) PolicyModulesDefaults(c echo.Context) error {
	return render(c, templates.PolicyModulesPage("defaults"))
}
func (h *Handler) PolicyModulesSources(c echo.Context) error {
	return render(c, templates.PolicyModulesPage("sources"))
}

// Profiles renders the profile-list + editor page (item 5 in the sidebar).
func (h *Handler) Profiles(c echo.Context) error {
	return render(c, templates.ProfilesPage())
}

// Scripts renders the admin-authored automation-scripts page (item 6).
// Was previously a two-tab surface; the Policy Modules tab moved to
// /policy/modules on 2026-05-16.
func (h *Handler) Scripts(c echo.Context) error {
	return render(c, templates.ScriptsPage())
}

// Wine renders the Windows-app compatibility page (item 7 in the sidebar).
func (h *Handler) Wine(c echo.Context) error {
	return render(c, templates.WinePage())
}

// Package Management — three tabs (managers / packages / cycles).

// PackagesRedirect 302-redirects /packages to /packages/managers (default tab).
func (h *Handler) PackagesRedirect(c echo.Context) error {
	return c.Redirect(302, "/packages/managers")
}
func (h *Handler) PackagesManagers(c echo.Context) error {
	return render(c, templates.PackagesPage("managers"))
}
func (h *Handler) PackagesPackages(c echo.Context) error {
	return render(c, templates.PackagesPage("packages"))
}
func (h *Handler) PackagesCycles(c echo.Context) error {
	return render(c, templates.PackagesPage("cycles"))
}

// ServerAdmin renders the tenant/AD/GP-import admin page (item 9).
func (h *Handler) ServerAdmin(c echo.Context) error {
	return render(c, templates.ServerAdminPage())
}

// Preferences renders the per-user + admin preferences page (item 10).
func (h *Handler) Preferences(c echo.Context) error {
	return render(c, templates.PreferencesPage())
}
