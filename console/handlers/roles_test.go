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
	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
)

// TestUserRoleAssignAuthorization covers the Task 11 guards directly at
// the handler level (sessions injected via auth.WithSession, no HTTP
// middleware stack needed): technician actors get 403, an admin cannot
// modify their own roles (400), and an admin assigning to someone else
// succeeds and recomputes the target's role cache.
func TestUserRoleAssignAuthorization(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "roles_handler_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "T", Slug: "t"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	idSvc := services.NewIdentityService(d)
	admin, err := idSvc.Create(ctx, tenant.ID, identities.Identity{
		Username: "admin", Email: "admin@example.com", DisplayName: "Admin", Role: identities.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	target, err := idSvc.Create(ctx, tenant.ID, identities.Identity{
		Username: "target", Email: "target@example.com", DisplayName: "Target", Role: identities.RoleUser,
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	roleSvc := services.NewRoleService(d)
	if err := roleSvc.EnsureBuiltins(ctx, tenant.ID); err != nil {
		t.Fatalf("ensure builtins: %v", err)
	}
	techRole, err := d.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenant.ID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}

	h := New(d)
	e := echo.New()

	do := func(actorID int64, actorRole identities.Role, targetID int64, roleID int64) (*httptest.ResponseRecorder, error) {
		form := url.Values{"role_id": {strconv.FormatInt(roleID, 10)}}
		req := httptest.NewRequest(http.MethodPost, "/users/"+strconv.FormatInt(targetID, 10)+"/roles",
			strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
			IdentityID: actorID, TenantID: tenant.ID, Role: actorRole,
			Grants: authz.Grants(permissions.TemplateGrants(string(actorRole))),
		}))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/users/:id/roles")
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(targetID, 10))
		return rec, h.UserRoleAssign(c)
	}

	// Technician actor: 403.
	_, err = do(admin.ID, identities.RoleTechnician, target.ID, techRole.ID)
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("technician actor: want 403 HTTPError, got %v", err)
	}

	// Admin modifying own roles: 400.
	_, err = do(admin.ID, identities.RoleAdmin, admin.ID, techRole.ID)
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("self-modification: want 400 HTTPError, got %v", err)
	}

	// Admin assigning to someone else: success, redirect, cache updated.
	rec, err := do(admin.ID, identities.RoleAdmin, target.ID, techRole.ID)
	if err != nil {
		t.Fatalf("admin assign failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("admin assign status = %d, want 302", rec.Code)
	}
	got, err := idSvc.Get(ctx, target.ID)
	if err != nil {
		t.Fatalf("get target: %v", err)
	}
	if got.Role != identities.RoleTechnician {
		t.Fatalf("target role cache = %q, want technician", got.Role)
	}
}

// TestUserPolicyAddSubmitCreatesRows covers Task 13: the POST creates
// the direct configuration group, the binding and the assignment, and
// the policy then resolves onto the target via ResolveForTarget.
func TestUserPolicyAddSubmitCreatesRows(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "policy_add_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "T2", Slug: "t2"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	idSvc := services.NewIdentityService(d)
	admin, err := idSvc.Create(ctx, tenant.ID, identities.Identity{
		Username: "padmin", Email: "padmin@example.com", DisplayName: "P Admin", Role: identities.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	target, err := idSvc.Create(ctx, tenant.ID, identities.Identity{
		Username: "ptarget", Email: "ptarget@example.com", DisplayName: "P Target", Role: identities.RoleUser,
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	policyID := policies.Catalog()[0].ID

	h := New(d)
	e := echo.New()
	form := url.Values{"policy": {policyID}}
	req := httptest.NewRequest(http.MethodPost, "/users/"+strconv.FormatInt(target.ID, 10)+"/policies/add",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		IdentityID: admin.ID, TenantID: tenant.ID, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/users/:id/policies/add")
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(target.ID, 10))
	if err := h.UserPolicyAddSubmit(c); err != nil {
		t.Fatalf("UserPolicyAddSubmit: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}

	// All three rows exist and the policy resolves onto the target.
	cg, err := d.Queries.GetConfigurationGroupByName(ctx, db.GetConfigurationGroupByNameParams{
		TenantID: tenant.ID, Name: "Direct - P Target",
	})
	if err != nil {
		t.Fatalf("direct configuration group missing: %v", err)
	}
	bindings, err := d.Queries.ListBindingsByGroup(ctx, cg.ID)
	if err != nil || len(bindings) != 1 || bindings[0].PolicyUrn != policyID {
		t.Fatalf("binding missing/wrong: %v %+v", err, bindings)
	}
	applied, err := services.NewAssignmentService(d).ResolveForTarget(ctx, tenant.ID, "identity", target.ID, nil, 0)
	if err != nil {
		t.Fatalf("ResolveForTarget: %v", err)
	}
	if len(applied) != 1 || applied[0].PolicyID != policyID || applied[0].SourceGroup != "Direct - P Target" {
		t.Fatalf("resolved rows wrong: %+v", applied)
	}

	// Repeat POST is idempotent (no 500 on UNIQUE violations).
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req, rec2)
	c2.SetPath("/users/:id/policies/add")
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.FormatInt(target.ID, 10))
	if err := h.UserPolicyAddSubmit(c2); err != nil {
		t.Fatalf("repeat UserPolicyAddSubmit: %v", err)
	}
}
