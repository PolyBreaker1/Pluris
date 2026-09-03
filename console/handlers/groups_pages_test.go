package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
)

// Task 6.2 handler tests: /groups list (+kind view), create, member
// add/remove (kind mapping + direct-only removal), rules
// (add/remove/match-mode/recalculate), the member-kind-conflict fields
// API, the RBAC-denied sweep, tenant isolation, and the detail page's
// picker/builder/source-chip markup.

// createGroupViaSvc creates a group with explicit member_kind/membership
// through the real service (slug derivation + validation included).
func createGroupViaSvc(t *testing.T, h *Handler, tenantID int64, name, memberKind, membership string) db.Group {
	t.Helper()
	g, err := h.groupSvc.Create(context.Background(), tenantID, name, "", memberKind, membership, "", "")
	if err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	return g
}

// createGroupTestAsset inserts a computer asset with the given payload.
func createGroupTestAsset(t *testing.T, h *Handler, tenantID int64, uuid, payload string) db.Asset {
	t.Helper()
	a, err := h.db.Queries.CreateAsset(context.Background(), db.CreateAssetParams{
		Uuid: uuid, TenantID: tenantID, Subtype: "computer", SubtypePayload: payload,
		EnrollmentState: "enrolled",
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	return a
}

// groupPageCtx builds an echo.Context for a group handler call.
func groupPageCtx(e *echo.Echo, req *http.Request, sess *auth.UserSession, params [][2]string) (echo.Context, *httptest.ResponseRecorder) {
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	names := make([]string, len(params))
	values := make([]string, len(params))
	for i, p := range params {
		names[i], values[i] = p[0], p[1]
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	return c, rec
}

// (1) List renders all groups; the kind query param picks the view
// (identity = identity+mixed, asset = asset+mixed).
func TestGroupsListKindViews(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_list_test.db", "groups-list-tenant")
	e := echo.New()

	createGroupViaSvc(t, h, tenantID, "only-users-grp", services.MemberKindIdentity, services.MembershipStatic)
	createGroupViaSvc(t, h, tenantID, "only-assets-grp", services.MemberKindAsset, services.MembershipStatic)
	createGroupViaSvc(t, h, tenantID, "mixed-grp", services.MemberKindMixed, services.MembershipStatic)

	cases := []struct {
		kind        string
		wantNames   []string
		absentNames []string
	}{
		{"", []string{"only-users-grp", "only-assets-grp", "mixed-grp"}, nil},
		{"identity", []string{"only-users-grp", "mixed-grp"}, []string{"only-assets-grp"}},
		{"asset", []string{"only-assets-grp", "mixed-grp"}, []string{"only-users-grp"}},
	}
	for _, tc := range cases {
		target := "/groups"
		if tc.kind != "" {
			target += "?kind=" + tc.kind
		}
		c, rec := groupPageCtx(e, httptest.NewRequest(http.MethodGet, target, nil), adminSession(tenantID), nil)
		if err := h.GroupsList(c); err != nil {
			t.Fatalf("GroupsList(kind=%q): %v", tc.kind, err)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-testid="page-groups"`) {
			t.Errorf("kind=%q: missing page-groups testid", tc.kind)
		}
		for _, name := range tc.wantNames {
			if !strings.Contains(body, name) {
				t.Errorf("kind=%q: body missing group %q", tc.kind, name)
			}
		}
		for _, name := range tc.absentNames {
			if strings.Contains(body, name) {
				t.Errorf("kind=%q: body should NOT contain group %q", tc.kind, name)
			}
		}
	}
}

// (2) Create redirects to the new group's detail page; validation
// failures re-render the form with the error.
func TestGroupCreateRedirectsToDetail(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_create_test.db", "groups-create-tenant")
	e := echo.New()

	form := url.Values{
		"name": {"Engineering Workstations"}, "description": {"All eng machines"},
		"member_kind": {"asset"}, "membership": {"dynamic"},
	}
	c, rec := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/new", form), adminSession(tenantID), nil)
	if err := h.GroupCreate(c); err != nil {
		t.Fatalf("GroupCreate: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/groups/") {
		t.Fatalf("redirect = %q, want /groups/<id>", loc)
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(loc, "/groups/"), 10, 64)
	if err != nil {
		t.Fatalf("redirect id parse: %v", err)
	}
	g, err := h.groupSvc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("created group not found: %v", err)
	}
	if g.MemberKind != "asset" || g.Membership != "dynamic" || g.Description != "All eng machines" {
		t.Errorf("created group meta wrong: %+v", g)
	}

	// Invalid member_kind re-renders the form with an error.
	badForm := url.Values{"name": {"x"}, "member_kind": {"bogus"}, "membership": {"static"}}
	c2, rec2 := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/new", badForm), adminSession(tenantID), nil)
	if err := h.GroupCreate(c2); err != nil {
		t.Fatalf("GroupCreate(bad kind): %v", err)
	}
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "invalid member kind") {
		t.Errorf("bad kind: expected re-rendered form with error, got %d", rec2.Code)
	}

	// Blank name likewise.
	nameless := url.Values{"name": {"  "}, "member_kind": {"mixed"}}
	c3, rec3 := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/new", nameless), adminSession(tenantID), nil)
	if err := h.GroupCreate(c3); err != nil {
		t.Fatalf("GroupCreate(no name): %v", err)
	}
	if rec3.Code != http.StatusOK || !strings.Contains(rec3.Body.String(), "name is required") {
		t.Errorf("no name: expected re-rendered form with error, got %d", rec3.Code)
	}
}

// (3) Member add maps picker kinds onto member rows (computer -> asset,
// user -> identity), enforces the group's member_kind, and 404s
// cross-tenant targets.
func TestGroupMemberAddKindMappingAndTenantIsolation(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_member_add_test.db", "groups-member-add-tenant")
	ctx := context.Background()
	e := echo.New()

	assetGroup := createGroupViaSvc(t, h, tenantID, "asset-only-grp", services.MemberKindAsset, services.MembershipStatic)
	gid := strconv.FormatInt(assetGroup.ID, 10)
	asset := createGroupTestAsset(t, h, tenantID, "aaaaaaaa-0000-0000-0000-000000000001", `{"os_family":"linux"}`)
	identityID := createTestIdentityForPlurisTest(t, h, tenantID, "member-add-user")

	// computer pick -> asset membership row.
	form := url.Values{"target_kind": {"computer"}, "target_ref": {strconv.FormatInt(asset.ID, 10)}}
	c, rec := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/members", form), adminSession(tenantID), [][2]string{{"id", gid}})
	if err := h.GroupMemberAdd(c); err != nil {
		t.Fatalf("GroupMemberAdd(computer): %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	members, err := h.groupSvc.ListMembers(ctx, assetGroup.ID)
	if err != nil || len(members) != 1 || members[0].Kind != "asset" || members[0].Source != "Direct" {
		t.Fatalf("expected 1 direct asset member, got %+v (err %v)", members, err)
	}

	// user pick on an asset-only group -> 400.
	userForm := url.Values{"target_kind": {"user"}, "target_ref": {strconv.FormatInt(identityID, 10)}}
	c2, _ := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/members", userForm), adminSession(tenantID), [][2]string{{"id", gid}})
	mustHTTPStatus(t, h.GroupMemberAdd(c2), http.StatusBadRequest)

	// Cross-tenant asset -> 404.
	tenantB, err := h.db.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "member-add-b", Slug: "member-add-b"})
	if err != nil {
		t.Fatalf("create tenant b: %v", err)
	}
	foreign := createGroupTestAsset(t, h, tenantB.ID, "bbbbbbbb-0000-0000-0000-000000000002", `{}`)
	foreignForm := url.Values{"target_kind": {"computer"}, "target_ref": {strconv.FormatInt(foreign.ID, 10)}}
	c3, _ := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/members", foreignForm), adminSession(tenantID), [][2]string{{"id", gid}})
	mustHTTPStatus(t, h.GroupMemberAdd(c3), http.StatusNotFound)
}

// (4) Direct members can be removed; rule-sourced members cannot (409),
// service-side as well as (absence of) the row's Remove button.
func TestGroupMemberRemoveDirectOnly(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_member_remove_test.db", "groups-member-remove-tenant")
	ctx := context.Background()
	e := echo.New()

	g := createGroupViaSvc(t, h, tenantID, "dyn-remove-grp", services.MemberKindAsset, services.MembershipDynamic)
	gid := strconv.FormatInt(g.ID, 10)
	linux := createGroupTestAsset(t, h, tenantID, "cccccccc-0000-0000-0000-000000000003", `{"os_family":"linux"}`)
	direct := createGroupTestAsset(t, h, tenantID, "dddddddd-0000-0000-0000-000000000004", `{"os_family":"windows"}`)
	if err := h.groupSvc.AddAssetMember(ctx, g.ID, direct.ID); err != nil {
		t.Fatalf("add direct member: %v", err)
	}
	// Rule matches the linux asset -> rule-sourced membership.
	if _, err := h.groupSvc.AddRule(ctx, g.ID, "param", "computer/hardware/os_family", "equals", []string{"linux"}, "", ""); err != nil {
		t.Fatalf("add rule: %v", err)
	}

	// Rule-sourced remove -> 409.
	form := url.Values{"kind": {"asset"}}
	c, _ := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/members/x/remove", form),
		adminSession(tenantID), [][2]string{{"id", gid}, {"mid", strconv.FormatInt(linux.ID, 10)}})
	mustHTTPStatus(t, h.GroupMemberRemove(c), http.StatusConflict)

	// Direct remove -> 302 and the row is gone.
	c2, rec2 := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/members/x/remove", form),
		adminSession(tenantID), [][2]string{{"id", gid}, {"mid", strconv.FormatInt(direct.ID, 10)}})
	if err := h.GroupMemberRemove(c2); err != nil {
		t.Fatalf("GroupMemberRemove(direct): %v", err)
	}
	if rec2.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec2.Code)
	}
	members, err := h.groupSvc.ListMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	for _, m := range members {
		if m.ID == direct.ID {
			t.Error("direct member should have been removed")
		}
	}
}

// (5) Rules: add via the condition-builder form contract, match-mode
// toggle, recalculate (counts in the redirect), remove; static groups
// reject rule adds; zero-rules recalculation is refused.
func TestGroupRulesLifecycle(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_rules_test.db", "groups-rules-tenant")
	ctx := context.Background()
	e := echo.New()

	g := createGroupViaSvc(t, h, tenantID, "dyn-rules-grp", services.MemberKindAsset, services.MembershipDynamic)
	gid := strconv.FormatInt(g.ID, 10)
	createGroupTestAsset(t, h, tenantID, "eeeeeeee-0000-0000-0000-000000000005", `{"os_family":"linux"}`)
	createGroupTestAsset(t, h, tenantID, "ffffffff-0000-0000-0000-000000000006", `{"os_family":"windows"}`)

	// Zero-rules recalculate -> 400 (the vacuous-AND guard).
	cz, _ := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/recalculate", url.Values{}), adminSession(tenantID), [][2]string{{"id", gid}})
	mustHTTPStatus(t, h.GroupRecalculate(cz), http.StatusBadRequest)

	// Add rule (form contract: repeated values= keys).
	form := url.Values{
		"kind": {"param"}, "param_path": {"computer/hardware/os_family"},
		"operator": {"equals"}, "values": {"linux"},
	}
	c, rec := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/rules", form), adminSession(tenantID), [][2]string{{"id", gid}})
	if err := h.GroupRuleAdd(c); err != nil {
		t.Fatalf("GroupRuleAdd: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("rule add status = %d, want 302", rec.Code)
	}
	rules, err := h.groupSvc.ListRules(ctx, g.ID)
	if err != nil || len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d (err %v)", len(rules), err)
	}
	// AddRule evaluates immediately: the linux asset is now a member.
	members, _ := h.groupSvc.ListMembers(ctx, g.ID)
	if len(members) != 1 || members[0].Source != "Dynamic" {
		t.Fatalf("expected 1 dynamic member after rule add, got %+v", members)
	}

	// Match-mode toggle.
	mm := url.Values{"match_mode": {"any"}}
	cm, recm := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/match-mode", mm), adminSession(tenantID), [][2]string{{"id", gid}})
	if err := h.GroupMatchModeUpdate(cm); err != nil {
		t.Fatalf("GroupMatchModeUpdate: %v", err)
	}
	if recm.Code != http.StatusFound {
		t.Fatalf("match-mode status = %d, want 302", recm.Code)
	}
	if g2, _ := h.groupSvc.Get(ctx, g.ID); g2.RulesMatchMode != "any" {
		t.Errorf("match mode = %q, want any", g2.RulesMatchMode)
	}
	badMM := url.Values{"match_mode": {"sometimes"}}
	cmb, _ := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/match-mode", badMM), adminSession(tenantID), [][2]string{{"id", gid}})
	mustHTTPStatus(t, h.GroupMatchModeUpdate(cmb), http.StatusBadRequest)

	// Recalculate: counts ride the redirect query.
	cr, recr := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/recalculate", url.Values{}), adminSession(tenantID), [][2]string{{"id", gid}})
	if err := h.GroupRecalculate(cr); err != nil {
		t.Fatalf("GroupRecalculate: %v", err)
	}
	loc := recr.Header().Get("Location")
	if !strings.Contains(loc, "recalc=1") || !strings.Contains(loc, "total=1") {
		t.Errorf("recalculate redirect %q missing counts", loc)
	}

	// Remove the rule: 302, rule gone, rule-sourced member reconciled away.
	rid := strconv.FormatInt(rules[0].ID, 10)
	crm, recrm := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/rules/x/remove", url.Values{}),
		adminSession(tenantID), [][2]string{{"id", gid}, {"rid", rid}})
	if err := h.GroupRuleRemove(crm); err != nil {
		t.Fatalf("GroupRuleRemove: %v", err)
	}
	if recrm.Code != http.StatusFound {
		t.Fatalf("rule remove status = %d, want 302", recrm.Code)
	}
	if rules, _ = h.groupSvc.ListRules(ctx, g.ID); len(rules) != 0 {
		t.Errorf("expected 0 rules after remove, got %d", len(rules))
	}

	// Static group rejects rule adds.
	static := createGroupViaSvc(t, h, tenantID, "static-rules-grp", services.MemberKindAsset, services.MembershipStatic)
	sgid := strconv.FormatInt(static.ID, 10)
	cs, _ := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+sgid+"/rules", form), adminSession(tenantID), [][2]string{{"id", sgid}})
	mustHTTPStatus(t, h.GroupRuleAdd(cs), http.StatusBadRequest)
}

// (6) The fields API surfaces a member-kind conflict as 409 and applies
// clean name/description edits.
func TestGroupFieldUpdateMemberKindConflict(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_fields_test.db", "groups-fields-tenant")
	ctx := context.Background()
	e := echo.New()

	g := createGroupViaSvc(t, h, tenantID, "fields-grp", services.MemberKindMixed, services.MembershipStatic)
	gid := strconv.FormatInt(g.ID, 10)
	asset := createGroupTestAsset(t, h, tenantID, "11111111-9999-0000-0000-000000000007", `{}`)
	if err := h.groupSvc.AddAssetMember(ctx, g.ID, asset.ID); err != nil {
		t.Fatalf("add asset member: %v", err)
	}

	// Narrowing to identity while an asset member exists -> 409.
	conflictReq := newJSONReq(http.MethodPost, "/api/groups/"+gid+"/fields",
		FieldUpdateRequest{Section: "meta", Fields: map[string]string{"member_kind": "identity"}})
	c, _ := groupPageCtx(e, conflictReq, adminSession(tenantID), [][2]string{{"id", gid}})
	mustHTTPStatus(t, h.GroupFieldUpdate(c), http.StatusConflict)

	// name + description save fine.
	okReq := newJSONReq(http.MethodPost, "/api/groups/"+gid+"/fields",
		FieldUpdateRequest{Section: "general", Fields: map[string]string{"name": "Renamed Grp", "description": "new desc"}})
	c2, rec2 := groupPageCtx(e, okReq, adminSession(tenantID), [][2]string{{"id", gid}})
	if err := h.GroupFieldUpdate(c2); err != nil {
		t.Fatalf("GroupFieldUpdate: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec2.Code)
	}
	g2, _ := h.groupSvc.Get(ctx, g.ID)
	if g2.Name != "Renamed Grp" || g2.Description != "new desc" {
		t.Errorf("fields not applied: %+v", g2)
	}

	// Unknown field key -> 400.
	badReq := newJSONReq(http.MethodPost, "/api/groups/"+gid+"/fields",
		FieldUpdateRequest{Section: "general", Fields: map[string]string{"slug": "nope"}})
	c3, _ := groupPageCtx(e, badReq, adminSession(tenantID), [][2]string{{"id", gid}})
	mustHTTPStatus(t, h.GroupFieldUpdate(c3), http.StatusBadRequest)
}

// (7) RBAC-denied sweep: a session from the "user" template (no group.*
// grants) gets 403 from every mutating group handler.
func TestGroupMutationsDeniedWithoutGrants(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_rbac_test.db", "groups-rbac-tenant")
	e := echo.New()
	g := createGroupViaSvc(t, h, tenantID, "rbac-grp", services.MemberKindMixed, services.MembershipDynamic)
	gid := strconv.FormatInt(g.ID, 10)
	sess := userSession(tenantID)

	cases := []struct {
		name    string
		handler func(echo.Context) error
		target  string
		params  [][2]string
	}{
		{"new", h.GroupNew, "/groups/new", nil},
		{"create", h.GroupCreate, "/groups/new", nil},
		{"delete", h.GroupDelete, "/groups/" + gid + "/delete", [][2]string{{"id", gid}}},
		{"member-add", h.GroupMemberAdd, "/groups/" + gid + "/members", [][2]string{{"id", gid}}},
		{"member-remove", h.GroupMemberRemove, "/groups/" + gid + "/members/1/remove", [][2]string{{"id", gid}, {"mid", "1"}}},
		{"rule-add", h.GroupRuleAdd, "/groups/" + gid + "/rules", [][2]string{{"id", gid}}},
		{"rule-remove", h.GroupRuleRemove, "/groups/" + gid + "/rules/1/remove", [][2]string{{"id", gid}, {"rid", "1"}}},
		{"match-mode", h.GroupMatchModeUpdate, "/groups/" + gid + "/match-mode", [][2]string{{"id", gid}}},
		{"recalculate", h.GroupRecalculate, "/groups/" + gid + "/recalculate", [][2]string{{"id", gid}}},
		{"fields", h.GroupFieldUpdate, "/api/groups/" + gid + "/fields", [][2]string{{"id", gid}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := groupPageCtx(e, newFormReq(http.MethodPost, tc.target, url.Values{}), sess, tc.params)
			mustHTTPStatus(t, tc.handler(c), http.StatusForbidden)
		})
	}
}

// (8) Tenant isolation on mutation routes: a cross-tenant group id reads
// as 404 (resolveTenantGroup), even for a fully-granted admin.
func TestGroupMutationsCrossTenant404(t *testing.T) {
	h, tenantAID := setupPlurisTestDB(t, "groups_crosstenant_test.db", "groups-cross-a")
	ctx := context.Background()
	e := echo.New()

	tenantB, err := h.db.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "groups-cross-b", Slug: "groups-cross-b"})
	if err != nil {
		t.Fatalf("create tenant b: %v", err)
	}
	g := createGroupViaSvc(t, h, tenantAID, "cross-grp", services.MemberKindMixed, services.MembershipStatic)
	gid := strconv.FormatInt(g.ID, 10)

	c, _ := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/delete", url.Values{}),
		adminSession(tenantB.ID), [][2]string{{"id", gid}})
	mustHTTPStatus(t, h.GroupDelete(c), http.StatusNotFound)
}

// (9) Reference integrity applies at permanent deletion, not soft deletion.
func TestGroupDeleteReferencedGuard(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_delete_test.db", "groups-delete-tenant")
	ctx := context.Background()
	e := echo.New()

	g := createGroupViaSvc(t, h, tenantID, "delete-grp", services.MemberKindMixed, services.MembershipStatic)
	gid := strconv.FormatInt(g.ID, 10)
	cg, err := h.configGroupSvc.Create(ctx, tenantID, "cg-referencing", "", true)
	if err != nil {
		t.Fatalf("create config group: %v", err)
	}
	if _, err := h.configGroupSvc.AddAssignment(ctx, tenantID, cg.ID, "group", g.ID, 0, false); err != nil {
		t.Fatalf("add assignment: %v", err)
	}

	c, _ := groupPageCtx(e, newFormReq(http.MethodPost, "/groups/"+gid+"/delete", url.Values{}),
		adminSession(tenantID), [][2]string{{"id", gid}})
	if err := h.GroupDelete(c); err != nil {
		t.Fatalf("soft delete referenced group: %v", err)
	}
	if _, err := h.groupSvc.Get(ctx, g.ID); err == nil {
		t.Fatal("soft-deleted group remains visible")
	}
	if err := h.groupSvc.PermanentlyDelete(ctx, tenantID, g.ID); !errors.Is(err, services.ErrGroupReferenced) {
		t.Fatalf("permanent delete = %v, want ErrGroupReferenced", err)
	}

	// Remove the assignment; permanent deletion now succeeds.
	assignments, err := h.configGroupSvc.ListAssignments(ctx, tenantID, cg.ID)
	if err != nil || len(assignments) != 1 {
		t.Fatalf("list assignments: %v (%d)", err, len(assignments))
	}
	if err := h.configGroupSvc.RemoveAssignment(ctx, tenantID, cg.ID, assignments[0].ID); err != nil {
		t.Fatalf("remove assignment: %v", err)
	}
	if err := h.groupSvc.PermanentlyDelete(ctx, tenantID, g.ID); err != nil {
		t.Fatalf("permanent delete after removing reference: %v", err)
	}
	if _, err := h.db.Queries.GetGroupIncludingDeleted(ctx, g.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("group remains after permanent delete: %v", err)
	}
}

// (10) Detail page markup: the target picker mounts with the allowed
// kinds matching the group's member_kind; the Rules tab mounts the
// condition builder for dynamic groups (explainer for static); member
// rows carry Direct/Dynamic source chips with remove only on direct
// rows; the zero-rules warning shows on a fresh dynamic group.
func TestGroupDetailMarkupPerKindAndMembership(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_detail_markup_test.db", "groups-markup-tenant")
	ctx := context.Background()
	e := echo.New()

	renderDetail := func(t *testing.T, g db.Group) string {
		t.Helper()
		gid := strconv.FormatInt(g.ID, 10)
		c, rec := groupPageCtx(e, httptest.NewRequest(http.MethodGet, "/groups/"+gid, nil), adminSession(tenantID), [][2]string{{"id", gid}})
		if err := h.GroupDetail(c); err != nil {
			t.Fatalf("GroupDetail(%s): %v", g.Name, err)
		}
		return rec.Body.String()
	}

	// Asset-kind dynamic group: computer-only picker, condition builder,
	// zero-rules warning + disabled recalculate.
	assetDyn := createGroupViaSvc(t, h, tenantID, "markup-asset-dyn", services.MemberKindAsset, services.MembershipDynamic)
	body := renderDetail(t, assetDyn)
	if !strings.Contains(body, `data-allowed-kinds="computer"`) {
		t.Error("asset group: picker should allow computer only")
	}
	if !strings.Contains(body, `id="condition-builder"`) {
		t.Error("dynamic group: condition builder dialog should mount")
	}
	if !strings.Contains(body, `data-testid="group-zero-rules-warning"`) {
		t.Error("fresh dynamic group: zero-rules warning missing")
	}
	if !strings.Contains(body, `disabled title="Add at least one rule first`) {
		t.Error("fresh dynamic group: recalculate should be disabled")
	}
	if !strings.Contains(body, `active">Groups`) && !strings.Contains(body, "assets-groups") {
		t.Error("asset group detail should highlight the assets-groups sidebar child")
	}

	// Identity-kind static group: user-only picker, static explainer, no
	// condition builder.
	identStatic := createGroupViaSvc(t, h, tenantID, "markup-ident-static", services.MemberKindIdentity, services.MembershipStatic)
	body = renderDetail(t, identStatic)
	if !strings.Contains(body, `data-allowed-kinds="user"`) {
		t.Error("identity group: picker should allow user only")
	}
	if !strings.Contains(body, `data-testid="group-rules-static-explainer"`) {
		t.Error("static group: rules explainer missing")
	}
	if strings.Contains(body, `id="condition-builder"`) {
		t.Error("static group: condition builder should NOT mount")
	}

	// Mixed dynamic group with one direct + one rule-sourced member:
	// both-kinds picker, Direct + Dynamic chips, remove only on direct.
	mixedDyn := createGroupViaSvc(t, h, tenantID, "markup-mixed-dyn", services.MemberKindMixed, services.MembershipDynamic)
	direct := createGroupTestAsset(t, h, tenantID, "22222222-9999-0000-0000-000000000008", `{"os_family":"windows"}`)
	if err := h.groupSvc.AddAssetMember(ctx, mixedDyn.ID, direct.ID); err != nil {
		t.Fatalf("add direct member: %v", err)
	}
	createGroupTestAsset(t, h, tenantID, "33333333-9999-0000-0000-000000000009", `{"os_family":"linux"}`)
	if _, err := h.groupSvc.AddRule(ctx, mixedDyn.ID, "param", "computer/hardware/os_family", "equals", []string{"linux"}, "", ""); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	body = renderDetail(t, mixedDyn)
	if !strings.Contains(body, `data-allowed-kinds="computer,user"`) {
		t.Error("mixed group: picker should allow computer and user")
	}
	if !strings.Contains(body, `data-member-source="Direct"`) || !strings.Contains(body, `data-member-source="Dynamic"`) {
		t.Error("members table should show both Direct and Dynamic source rows")
	}
	if !strings.Contains(body, "Managed by rules") {
		t.Error("rule-sourced row should show the managed-by-rules note instead of Remove")
	}
	if strings.Contains(body, `data-testid="group-zero-rules-warning"`) {
		t.Error("group with a rule should not show the zero-rules warning")
	}
}

// (11) The user/asset detail Groups tabs' Source column reflects the
// real membership source (the Task 6.2 carry-forward fix): rule-sourced
// memberships read "Dynamic", direct ones "Direct".
func TestGroupsTabSourceReflectsMembershipSource(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "groups_source_fix_test.db", "groups-source-tenant")
	ctx := context.Background()

	g := createGroupViaSvc(t, h, tenantID, "source-fix-grp", services.MemberKindAsset, services.MembershipDynamic)
	ruleAsset := createGroupTestAsset(t, h, tenantID, "44444444-9999-0000-0000-00000000000a", `{"os_family":"linux"}`)
	directAsset := createGroupTestAsset(t, h, tenantID, "55555555-9999-0000-0000-00000000000b", `{"os_family":"windows"}`)
	if err := h.groupSvc.AddAssetMember(ctx, g.ID, directAsset.ID); err != nil {
		t.Fatalf("add direct member: %v", err)
	}
	if _, err := h.groupSvc.AddRule(ctx, g.ID, "param", "computer/hardware/os_family", "equals", []string{"linux"}, "", ""); err != nil {
		t.Fatalf("add rule: %v", err)
	}

	ruleRows, err := h.groupSvc.ListForAsset(ctx, ruleAsset.ID)
	if err != nil || len(ruleRows) != 1 {
		t.Fatalf("ListForAsset(rule asset): %v (%d rows)", err, len(ruleRows))
	}
	if ruleRows[0].Source != "Dynamic" {
		t.Errorf("rule-sourced membership Source = %q, want Dynamic", ruleRows[0].Source)
	}
	directRows, err := h.groupSvc.ListForAsset(ctx, directAsset.ID)
	if err != nil || len(directRows) != 1 {
		t.Fatalf("ListForAsset(direct asset): %v (%d rows)", err, len(directRows))
	}
	if directRows[0].Source != "Direct" {
		t.Errorf("direct membership Source = %q, want Direct", directRows[0].Source)
	}
}
