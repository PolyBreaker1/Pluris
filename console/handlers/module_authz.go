package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

// moduleAccessInput builds a pkg/authz/modules.go ModuleAccessInput for
// sess against row, loading everything ModuleCanView/Edit/Admin need
// (session group/role memberships, the module's module_grants rows) so
// every module route can build one consistent decision input. Task 4.3
// is the first real caller of authz.ModuleAccessInput outside its own
// tests.
//
// RoleIDs is populated from the identity's DIRECTLY assigned roles only
// (db.Queries.ListRolesForIdentity), not roles reached transitively via
// group membership (RBAC v2's ListGroupRolesForIdentity) -- a known,
// documented gap: a module_grants row naming a role as subject only
// matches identities holding that role directly. Closing this fully
// would need the same union EffectiveGrants does internally; left as a
// follow-up since no route in this task grants roles module access by
// group-inherited role.
func (h *Handler) moduleAccessInput(ctx context.Context, sess *auth.UserSession, row db.PolicyModule) (authz.ModuleAccessInput, error) {
	in := authz.ModuleAccessInput{
		Grants:     sess.Grants,
		IdentityID: sess.IdentityID,
		IsBundled:  row.IsBundled,
	}
	if row.OwnerIdentityID.Valid {
		owner := row.OwnerIdentityID.Int64
		in.OwnerID = &owner
	}

	groups, err := h.db.Queries.ListGroupsForIdentity(ctx, sql.NullInt64{Int64: sess.IdentityID, Valid: true})
	if err != nil {
		return in, err
	}
	for _, g := range groups {
		in.GroupIDs = append(in.GroupIDs, g.ID)
	}

	roles, err := h.db.Queries.ListRolesForIdentity(ctx, sess.IdentityID)
	if err != nil {
		return in, err
	}
	for _, r := range roles {
		in.RoleIDs = append(in.RoleIDs, r.ID)
	}

	grants, err := h.db.Queries.ListGrantsForModule(ctx, row.ID)
	if err != nil {
		return in, err
	}
	for _, g := range grants {
		in.ModuleGrants = append(in.ModuleGrants, authz.ModuleGrant{
			SubjectType: g.SubjectType, SubjectID: g.SubjectID, Level: g.Level,
		})
	}
	return in, nil
}

// resolveTenantModule loads a policy_modules row by URN (:id route
// param) and verifies it's visible to sess's tenant: either bundled
// (tenant_id NULL, visible to every tenant) or owned by sess's own
// tenant. Cross-tenant modules 404, mirroring resolveTenantIdentity /
// resolveTenantAssetForFields's fail-closed shape. This is a coarser
// tenant-isolation check than the fine-grained ModuleCanView/Edit/Admin
// decision -- callers must still run the authz decision on top.
func (h *Handler) resolveTenantModule(c echo.Context) (db.PolicyModule, error) {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	if sess == nil {
		return db.PolicyModule{}, echo.NewHTTPError(http.StatusNotFound, "module not found")
	}
	return h.resolveTenantModuleByURN(ctx, sess, c.Param("id"))
}

// resolveTenantModuleByURN is resolveTenantModule's context-free core,
// factored out so callers that don't have a :id route param (e.g.
// ParamsAPI's ?module_id= query param, Task 4.4) can run the exact same
// tenant-isolation check.
func (h *Handler) resolveTenantModuleByURN(ctx context.Context, sess *auth.UserSession, urn string) (db.PolicyModule, error) {
	row, err := h.moduleSvc.GetModuleRow(ctx, urn)
	if err != nil {
		if errors.Is(err, services.ErrModuleNotFound) {
			return db.PolicyModule{}, echo.NewHTTPError(http.StatusNotFound, "module not found")
		}
		return db.PolicyModule{}, err
	}
	if !row.IsBundled && (!row.TenantID.Valid || row.TenantID.Int64 != sess.TenantID) {
		return db.PolicyModule{}, echo.NewHTTPError(http.StatusNotFound, "module not found")
	}
	return row, nil
}

// resolveModuleVersion loads a version row by its numeric :vid route
// param and verifies it belongs to mod (cross-module ids 404).
func (h *Handler) resolveModuleVersion(c echo.Context, mod db.PolicyModule) (db.PolicyModuleVersion, error) {
	ctx := c.Request().Context()
	vid, err := parseInt64Param(c, "vid")
	if err != nil {
		return db.PolicyModuleVersion{}, err
	}
	v, err := h.db.Queries.GetPolicyModuleVersion(ctx, vid)
	if err != nil {
		return db.PolicyModuleVersion{}, echo.NewHTTPError(http.StatusNotFound, "version not found")
	}
	if v.ModuleID != mod.ID {
		return db.PolicyModuleVersion{}, echo.NewHTTPError(http.StatusNotFound, "version not found")
	}
	return v, nil
}

func parseInt64Param(c echo.Context, name string) (int64, error) {
	v, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid "+name)
	}
	return v, nil
}
