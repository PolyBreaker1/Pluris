package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
)

// (1) GET /groups/:id -> 200 + the page testid.
func TestGroupDetailRenders(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "group_detail_render_test.db", "group-detail-tenant")
	group := createTestGroupForRoleTest(t, h, tenantID, "eng-detail-group")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/groups/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(group.ID, 10))

	if err := h.GroupDetail(c); err != nil {
		t.Fatalf("group detail render failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-testid="page-group-detail"`) {
		t.Error("body missing page-group-detail testid")
	}
	if !strings.Contains(body, "eng-detail-group") {
		t.Error("body missing the group's name")
	}
}

// Cross-tenant group id resolves as not-found.
func TestGroupDetailCrossTenant404(t *testing.T) {
	h, tenantAID := setupPlurisTestDB(t, "group_detail_crosstenant_test.db", "group-detail-cross-a")
	ctx := context.Background()

	tenantB, err := h.db.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "group-detail-cross-b", Slug: "group-detail-cross-b"})
	if err != nil {
		t.Fatalf("create tenant b: %v", err)
	}
	if err := h.roleSvc.EnsureBuiltins(ctx, tenantB.ID); err != nil {
		t.Fatalf("ensure builtins tenant b: %v", err)
	}

	groupInA := createTestGroupForRoleTest(t, h, tenantAID, "cross-detail-group-a")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/groups/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantB.ID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(groupInA.ID, 10))

	err = h.GroupDetail(c)
	mustHTTPStatus(t, err, http.StatusNotFound)
}

// (2) Roles tab renders an assigned role after AssignRoleToGroup.
func TestGroupDetailRolesTabShowsAssignedRole(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "group_detail_roles_test.db", "group-detail-roles-tenant")
	ctx := context.Background()
	group := createTestGroupForRoleTest(t, h, tenantID, "roles-tab-group")

	admin, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "admin"})
	if err != nil {
		t.Fatalf("get admin role: %v", err)
	}
	if err := h.db.Queries.AssignRoleToGroup(ctx, db.AssignRoleToGroupParams{GroupID: group.ID, RoleID: admin.ID}); err != nil {
		t.Fatalf("seed group role: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/groups/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(group.ID, 10))

	if err := h.GroupDetail(c); err != nil {
		t.Fatalf("group detail render failed: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "group-roles") {
		t.Error("body missing the group-roles list id")
	}
	if !strings.Contains(body, admin.Name) {
		t.Errorf("body missing the assigned role name %q", admin.Name)
	}
	if !strings.Contains(body, `action="/groups/`+strconv.FormatInt(group.ID, 10)+`/roles/`+strconv.FormatInt(admin.ID, 10)+`/remove"`) {
		t.Error("body missing the per-row remove form for the assigned role")
	}
}

// (3) User detail body shows a "via <group>" row when the identity is a
// member of a group holding a role.
func TestUserDetailShowsViaGroupRoleRow(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "user_detail_via_group_test.db", "via-group-tenant")
	ctx := context.Background()

	member := createTestIdentityForPlurisTest(t, h, tenantID, "via-group-member")
	group := createTestGroupForRoleTest(t, h, tenantID, "via-group-holder")
	if err := h.groupSvc.AddIdentityMember(ctx, group.ID, member); err != nil {
		t.Fatalf("add identity member: %v", err)
	}
	admin, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "admin"})
	if err != nil {
		t.Fatalf("get admin role: %v", err)
	}
	if err := h.db.Queries.AssignRoleToGroup(ctx, db.AssignRoleToGroupParams{GroupID: group.ID, RoleID: admin.ID}); err != nil {
		t.Fatalf("seed group role: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/users/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(member, 10))

	if err := h.UserDetail(c); err != nil {
		t.Fatalf("user detail render failed: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "via <a") && !strings.Contains(body, "via-group-holder") {
		t.Error("body missing the \"via <group>\" attribution")
	}
	if !strings.Contains(body, `title="Managed on the group"`) {
		t.Error("body missing the disabled-remove tooltip for the via-group row")
	}
	if !strings.Contains(body, `href="/groups/`+strconv.FormatInt(group.ID, 10)+`"`) {
		t.Error("via-group row's group name should link to the group detail page")
	}
}

// (4) User roles picker body contains an <optgroup once there's more
// than one role family available.
func TestUserRolesPickerIsHierarchical(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "user_roles_picker_test.db", "roles-picker-tenant")
	ctx := context.Background()

	tech, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("get technician role: %v", err)
	}
	if _, err := h.authzSvc.CloneRole(ctx, tenantID, tech.ID, "Custom Tech Clone For Picker"); err != nil {
		t.Fatalf("clone role: %v", err)
	}

	member := createTestIdentityForPlurisTest(t, h, tenantID, "picker-user")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/users/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(member, 10))

	if err := h.UserDetail(c); err != nil {
		t.Fatalf("user detail render failed: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<optgroup") {
		t.Error("user roles picker should render an optgroup (hierarchical picker)")
	}
}

// (5) A group name in the user Groups tab renders as a link to
// /groups/:id.
func TestUserGroupsTabGroupNameLinks(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "user_groups_link_test.db", "groups-link-tenant")
	ctx := context.Background()

	member := createTestIdentityForPlurisTest(t, h, tenantID, "groups-link-user")
	group := createTestGroupForRoleTest(t, h, tenantID, "linked-group")
	if err := h.groupSvc.AddIdentityMember(ctx, group.ID, member); err != nil {
		t.Fatalf("add identity member: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/users/x", nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(member, 10))

	if err := h.UserDetail(c); err != nil {
		t.Fatalf("user detail render failed: %v", err)
	}
	body := rec.Body.String()
	want := `<a href="/groups/` + strconv.FormatInt(group.ID, 10) + `">linked-group</a>`
	if !strings.Contains(body, want) {
		t.Errorf("body missing linked group name; want substring %q", want)
	}
}
