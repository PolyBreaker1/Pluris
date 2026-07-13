package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
)

// createTestGroupForRoleTest creates a minimal group in tenantID for the
// group-role assignment tests below.
func createTestGroupForRoleTest(t *testing.T, h *Handler, tenantID int64, name string) db.Group {
	t.Helper()
	group, err := h.db.Queries.CreateGroup(context.Background(), db.CreateGroupParams{
		TenantID: tenantID,
		Name:     name,
		Slug:     name,
	})
	if err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	return group
}

// (a) Assign: role appears in ListRolesForGroup, and an existing member's
// identities.role cache is recomputed to reflect the newly-inherited role.
func TestGroupRoleAssignAndRecompute(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "group_role_assign_test.db", "grouprole-assign-tenant")
	ctx := context.Background()
	e := echo.New()
	// A real identity for the actor (not just adminSession's placeholder
	// id 1) so AssignRoleToGroup's assigned_by FK resolves, and so the
	// actor is a distinct row from the group member below -- otherwise
	// the self-escalation guard (Fix 3) would trip on an accidental id
	// collision rather than genuine group membership.
	actorID := createTestIdentityForPlurisTest(t, h, tenantID, "assign-actor")
	sess := adminSession(tenantID)
	sess.IdentityID = actorID

	group := createTestGroupForRoleTest(t, h, tenantID, "eng-group")
	member := createTestIdentityForPlurisTest(t, h, tenantID, "group-member1")
	if err := h.groupSvc.AddIdentityMember(ctx, group.ID, member); err != nil {
		t.Fatalf("add identity member: %v", err)
	}

	admin, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "admin"})
	if err != nil {
		t.Fatalf("get admin role: %v", err)
	}

	form := url.Values{"role_id": {strconv.FormatInt(admin.ID, 10)}}
	req := newFormReq(http.MethodPost, "/groups/x/roles", form)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(group.ID, 10))

	if err := h.GroupRoleAssign(c); err != nil {
		t.Fatalf("assign failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("assign status = %d, want 302", rec.Code)
	}

	roles, err := h.db.Queries.ListRolesForGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("list roles for group: %v", err)
	}
	found := false
	for _, r := range roles {
		if r.ID == admin.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("assigned role not present in ListRolesForGroup")
	}

	memberRow, err := h.identitySvc.Get(ctx, member)
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if memberRow.Role != "admin" {
		t.Errorf("member identities.role = %q, want admin (recomputed via group role)", memberRow.Role)
	}
}

// (b) Remove: role no longer listed and the member's privilege cache
// drops back down.
func TestGroupRoleRemove(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "group_role_remove_test.db", "grouprole-remove-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := adminSession(tenantID)

	group := createTestGroupForRoleTest(t, h, tenantID, "eng-group-2")
	member := createTestIdentityForPlurisTest(t, h, tenantID, "group-member2")
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
	if err := h.roleSvc.RecomputeForGroupMembers(ctx, group.ID); err != nil {
		t.Fatalf("seed recompute: %v", err)
	}

	req := newFormReq(http.MethodPost, "/groups/x/roles/y/remove", url.Values{})
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "roleID")
	c.SetParamValues(strconv.FormatInt(group.ID, 10), strconv.FormatInt(admin.ID, 10))

	if err := h.GroupRoleRemove(c); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("remove status = %d, want 302", rec.Code)
	}

	roles, err := h.db.Queries.ListRolesForGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("list roles for group: %v", err)
	}
	for _, r := range roles {
		if r.ID == admin.ID {
			t.Fatal("removed role still present in ListRolesForGroup")
		}
	}

	memberRow, err := h.identitySvc.Get(ctx, member)
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if memberRow.Role == "admin" {
		t.Error("member identities.role should have dropped after role removal")
	}
}

// (c) RBAC: user-template session gets 403 on both assign and remove.
func TestGroupRoleMutationsForbiddenForUser(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "group_role_rbac_test.db", "grouprole-rbac-tenant")
	ctx := context.Background()
	e := echo.New()
	sess := userSession(tenantID)

	group := createTestGroupForRoleTest(t, h, tenantID, "rbac-group")
	admin, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "admin"})
	if err != nil {
		t.Fatalf("get admin role: %v", err)
	}

	assignReq := newFormReq(http.MethodPost, "/groups/x/roles", url.Values{"role_id": {strconv.FormatInt(admin.ID, 10)}})
	assignReq = assignReq.WithContext(auth.WithSession(assignReq.Context(), sess))
	assignRec := httptest.NewRecorder()
	assignC := e.NewContext(assignReq, assignRec)
	assignC.SetParamNames("id")
	assignC.SetParamValues(strconv.FormatInt(group.ID, 10))
	if err := h.GroupRoleAssign(assignC); err == nil {
		t.Fatal("user assign should be forbidden")
	} else {
		mustHTTPStatus(t, err, http.StatusForbidden)
	}

	removeReq := newFormReq(http.MethodPost, "/groups/x/roles/y/remove", url.Values{})
	removeReq = removeReq.WithContext(auth.WithSession(removeReq.Context(), sess))
	removeRec := httptest.NewRecorder()
	removeC := e.NewContext(removeReq, removeRec)
	removeC.SetParamNames("id", "roleID")
	removeC.SetParamValues(strconv.FormatInt(group.ID, 10), strconv.FormatInt(admin.ID, 10))
	if err := h.GroupRoleRemove(removeC); err == nil {
		t.Fatal("user remove should be forbidden")
	} else {
		mustHTTPStatus(t, err, http.StatusForbidden)
	}
}

// (d) Cross-tenant group id resolves as not-found; cross-tenant role id
// resolves as not-found.
func TestGroupRoleCrossTenant404(t *testing.T) {
	h, tenantAID := setupPlurisTestDB(t, "group_role_crosstenant_test.db", "grouprole-cross-a")
	ctx := context.Background()
	e := echo.New()

	tenantB, err := h.db.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "grouprole-cross-b", Slug: "grouprole-cross-b"})
	if err != nil {
		t.Fatalf("create tenant b: %v", err)
	}
	if err := h.roleSvc.EnsureBuiltins(ctx, tenantB.ID); err != nil {
		t.Fatalf("ensure builtins tenant b: %v", err)
	}

	groupInA := createTestGroupForRoleTest(t, h, tenantAID, "cross-group-a")
	roleInA, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantAID, Slug: "admin"})
	if err != nil {
		t.Fatalf("get role in tenant a: %v", err)
	}

	sessB := adminSession(tenantB.ID)

	// Group belongs to tenant A; session is tenant B -> 404.
	req := newFormReq(http.MethodPost, "/groups/x/roles", url.Values{"role_id": {strconv.FormatInt(roleInA.ID, 10)}})
	req = req.WithContext(auth.WithSession(req.Context(), sessB))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(groupInA.ID, 10))
	err = h.GroupRoleAssign(c)
	mustHTTPStatus(t, err, http.StatusNotFound)

	// Group belongs to tenant B, but the role_id names a tenant-A role ->
	// still 404 (role resolves cross-tenant).
	groupInB := createTestGroupForRoleTest(t, h, tenantB.ID, "cross-group-b")
	req2 := newFormReq(http.MethodPost, "/groups/x/roles", url.Values{"role_id": {strconv.FormatInt(roleInA.ID, 10)}})
	req2 = req2.WithContext(auth.WithSession(req2.Context(), sessB))
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.FormatInt(groupInB.ID, 10))
	err = h.GroupRoleAssign(c2)
	mustHTTPStatus(t, err, http.StatusNotFound)
}

// (e) Open-redirect guard: groupRoleRedirectTarget must ignore a
// cross-origin Referer and fall back to the group detail page instead of
// ever bouncing the caller off-site (CWE-601).
func TestGroupRoleAssignRedirectIgnoresForeignReferer(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "group_role_redirect_test.db", "grouprole-redirect-tenant")
	ctx := context.Background()
	e := echo.New()
	// Real identity so AssignRoleToGroup's assigned_by FK resolves (see
	// TestGroupRoleAssignAndRecompute for why adminSession's placeholder
	// id 1 isn't safe to use directly here).
	actorID := createTestIdentityForPlurisTest(t, h, tenantID, "redirect-actor")
	sess := adminSession(tenantID)
	sess.IdentityID = actorID

	group := createTestGroupForRoleTest(t, h, tenantID, "redirect-group")
	admin, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "admin"})
	if err != nil {
		t.Fatalf("get admin role: %v", err)
	}

	form := url.Values{"role_id": {strconv.FormatInt(admin.ID, 10)}}
	req := newFormReq(http.MethodPost, "/groups/x/roles", form)
	req.Header.Set("Referer", "https://evil.example/x")
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(group.ID, 10))

	if err := h.GroupRoleAssign(c); err != nil {
		t.Fatalf("assign failed: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("assign status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	wantFallback := "/groups/" + strconv.FormatInt(group.ID, 10)
	if loc != wantFallback {
		t.Fatalf("redirect Location = %q, want fallback %q (forged Referer must not be honored)", loc, wantFallback)
	}
}

// (f) Self-escalation speed bump: an actor who is themselves a member of
// the target group cannot grant that group a role (they'd be raising
// their own effective access). An actor outside the group is unaffected,
// and a super_admin (bypass grant) in the group is exempt.
func TestGroupRoleAssignRefusesSelfMemberGroup(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "group_role_self_escalation_test.db", "grouprole-self-esc-tenant")
	ctx := context.Background()
	e := echo.New()

	admin, err := h.db.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "admin"})
	if err != nil {
		t.Fatalf("get admin role: %v", err)
	}

	doAssign := func(sess *auth.UserSession, group db.Group) (int, error) {
		form := url.Values{"role_id": {strconv.FormatInt(admin.ID, 10)}}
		req := newFormReq(http.MethodPost, "/groups/x/roles", form)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.FormatInt(group.ID, 10))
		err := h.GroupRoleAssign(c)
		return rec.Code, err
	}

	// Actor in the group -> 400.
	groupIn := createTestGroupForRoleTest(t, h, tenantID, "self-esc-in")
	actorID := createTestIdentityForPlurisTest(t, h, tenantID, "self-esc-actor")
	if err := h.groupSvc.AddIdentityMember(ctx, groupIn.ID, actorID); err != nil {
		t.Fatalf("add actor as member: %v", err)
	}
	actorSess := &auth.UserSession{
		TenantID: tenantID, IdentityID: actorID, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}
	if code, err := doAssign(actorSess, groupIn); err == nil {
		t.Fatal("actor assigning roles to their own group should be refused")
	} else {
		mustHTTPStatus(t, err, http.StatusBadRequest)
		_ = code
	}

	// Same actor, different group they don't belong to -> allowed.
	groupOut := createTestGroupForRoleTest(t, h, tenantID, "self-esc-out")
	if code, err := doAssign(actorSess, groupOut); err != nil {
		t.Fatalf("actor assigning roles to a group they don't belong to should succeed: %v", err)
	} else if code != http.StatusFound {
		t.Fatalf("assign status = %d, want 302", code)
	}

	// super_admin (bypass grant) in the group -> exempt, allowed.
	superID := createTestIdentityForPlurisTest(t, h, tenantID, "self-esc-super")
	if err := h.groupSvc.AddIdentityMember(ctx, groupIn.ID, superID); err != nil {
		t.Fatalf("add super_admin as member: %v", err)
	}
	superSess := &auth.UserSession{
		TenantID: tenantID, IdentityID: superID, Role: identities.RoleSuperAdmin,
		Grants: authz.Grants{authz.BypassKey: "yes"},
	}
	if code, err := doAssign(superSess, groupIn); err != nil {
		t.Fatalf("super_admin assigning roles to their own group should succeed: %v", err)
	} else if code != http.StatusFound {
		t.Fatalf("assign status = %d, want 302", code)
	}
}
