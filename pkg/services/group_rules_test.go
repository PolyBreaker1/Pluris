package services

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// setupGroupRulesTestDB opens a fresh on-disk test database (mirrors
// setupIdentityTestDB's pattern, identities_test.go) under a name unique
// to this file so parallel package test runs never collide.
func setupGroupRulesTestDB(t *testing.T) (*database.Database, int64) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_group_rules_service.db")
	d, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	tenant, err := d.Queries.CreateTenant(context.Background(), db.CreateTenantParams{
		Name: "Acme", Slug: "acme-group-rules",
	})
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	return d, tenant.ID
}

func mustCreateAsset(t *testing.T, d *database.Database, tenantID int64, uuid, payload string) db.Asset {
	t.Helper()
	a, err := d.Queries.CreateAsset(context.Background(), db.CreateAssetParams{
		Uuid: uuid, TenantID: tenantID, Subtype: "computer", SubtypePayload: payload,
		EnrollmentState: "enrolled",
	})
	if err != nil {
		t.Fatalf("CreateAsset failed: %v", err)
	}
	return a
}

func mustCreateIdentity(t *testing.T, d *database.Database, tenantID int64, username, email, department string) db.Identity {
	t.Helper()
	idSvc := NewIdentityService(d)
	ident, err := idSvc.Create(context.Background(), tenantID, identities.Identity{
		Username: username, Email: email, DisplayName: username, Role: identities.RoleUser,
		Department: department,
	})
	if err != nil {
		t.Fatalf("Create identity failed: %v", err)
	}
	row, err := d.Queries.GetIdentity(context.Background(), ident.ID)
	if err != nil {
		t.Fatalf("GetIdentity failed: %v", err)
	}
	return row
}

func mustCreateDynamicGroup(t *testing.T, d *database.Database, tenantID int64, name, memberKind string) db.Group {
	t.Helper()
	g, err := d.Queries.CreateGroup(context.Background(), db.CreateGroupParams{
		TenantID: tenantID, Name: name, Slug: name,
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if err := d.Queries.UpdateGroupMeta(context.Background(), db.UpdateGroupMetaParams{
		ID: g.ID, Description: "", MemberKind: memberKind, Membership: MembershipDynamic, RulesMatchMode: "all",
	}); err != nil {
		t.Fatalf("UpdateGroupMeta failed: %v", err)
	}
	g, err = d.Queries.GetGroup(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("GetGroup failed: %v", err)
	}
	return g
}

// TestMigration009AppliesFresh verifies the new columns/table exist and
// carry the documented defaults on a freshly migrated database.
func TestMigration009AppliesFresh(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	g, err := d.Queries.CreateGroup(context.Background(), db.CreateGroupParams{TenantID: tenantID, Name: "G", Slug: "g"})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if g.Description != "" || g.MemberKind != "mixed" || g.Membership != "static" || g.RulesMatchMode != "all" {
		t.Fatalf("unexpected defaults: %+v", g)
	}
	rules, err := d.Queries.ListRulesForGroup(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("ListRulesForGroup failed: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("want no rules on fresh group, got %d", len(rules))
	}
}

func TestDynamicMembershipExcludesSoftDeletedIdentity(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	ctx := context.Background()
	identity := mustCreateIdentity(t, d, tenantID, "deleted-dynamic", "deleted-dynamic@example.com", "IT")
	group := mustCreateDynamicGroup(t, d, tenantID, "dyn-deleted-identity", MemberKindIdentity)
	svc := NewGroupService(d)
	if _, err := svc.AddRule(ctx, group.ID, "param", "user/identity/email", string(dependencygroups.OpExists), nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if member, err := d.Queries.IsIdentityInGroup(ctx, db.IsIdentityInGroupParams{GroupID: group.ID, IdentityID: sql.NullInt64{Int64: identity.ID, Valid: true}}); err != nil || !member {
		t.Fatalf("identity was not initially added: member=%v err=%v", member, err)
	}
	if err := NewIdentityService(d).Delete(ctx, tenantID, identity.ID, 99); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EvaluateDynamicMembership(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if member, err := d.Queries.IsIdentityInGroup(ctx, db.IsIdentityInGroupParams{GroupID: group.ID, IdentityID: sql.NullInt64{Int64: identity.ID, Valid: true}}); err != nil || member {
		t.Fatalf("deleted identity remains a dynamic member: member=%v err=%v", member, err)
	}
}

// TestRuleCRUDValidationMatrix exercises AddRule against the same
// validation semantics DependencyGroupService.AddCondition applies
// (bad path/operator/kind/script subject all rejected the same way).
func TestRuleCRUDValidationMatrix(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	g := mustCreateDynamicGroup(t, d, tenantID, "dyn-valid", MemberKindAsset)

	cases := []struct {
		name                                            string
		kind, paramPath, operator, scriptSrc, scriptRef string
		wantErr                                         error
	}{
		{"missing param path", "param", "", string(dependencygroups.OpEquals), "", "", ErrParamPathRequired},
		{"bad operator", "param", "computer/hardware/os_family", "made_up_op", "", "", ErrInvalidOperator},
		{"bad kind", "bogus", "computer/hardware/os_family", string(dependencygroups.OpEquals), "", "", ErrInvalidConditionKind},
		{"missing script source", "script", "", string(dependencygroups.OpExists), "", "", ErrScriptSourceRequired},
		{"script source and ref both set", "script", "", string(dependencygroups.OpExists), "echo hi", "lib-1", ErrScriptSourceAmbiguous},
		{"missing command", "command", "", string(dependencygroups.OpContains), "", "", ErrScriptSourceRequired},
		{"valid param rule", "param", "computer/hardware/os_family", string(dependencygroups.OpEquals), "", "", nil},
		{"valid script rule", "script", "", string(dependencygroups.OpExists), "echo hi", "", nil},
		{"valid command rule", "command", "", string(dependencygroups.OpContains), "uname -r", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.AddRule(context.Background(), g.ID, tc.kind, tc.paramPath, tc.operator, []string{"linux"}, tc.scriptSrc, tc.scriptRef)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestStaticGroupRejectsRules verifies AddRule and EvaluateDynamicMembership
// both fail closed with ErrGroupNotDynamic on a static group.
func TestStaticGroupRejectsRules(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	g, err := d.Queries.CreateGroup(context.Background(), db.CreateGroupParams{TenantID: tenantID, Name: "static-g", Slug: "static-g"})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	// membership defaults to "static".
	_, err = svc.AddRule(context.Background(), g.ID, "param", "computer/hardware/os_family", string(dependencygroups.OpEquals), []string{"linux"}, "", "")
	if !errors.Is(err, ErrGroupNotDynamic) {
		t.Fatalf("want ErrGroupNotDynamic, got %v", err)
	}
	if _, err := svc.EvaluateDynamicMembership(context.Background(), g.ID); !errors.Is(err, ErrGroupNotDynamic) {
		t.Fatalf("want ErrGroupNotDynamic, got %v", err)
	}
}

// TestFactsForAssetParity proves the shared FactsForAsset helper
// produces the same dependency-group Evaluate verdicts a hand-built
// facts map would have (the pre-refactor calling convention every
// dependencygroups_test.go case still uses unmodified). It builds an
// asset whose subtype_payload carries the fields the builtin groups key
// off (os_family, os_package_family, disk_encryption), evaluates
// EvaluateForAsset (which internally calls FactsForAsset then the
// unchanged Evaluate), and cross-checks against Evaluate called with an
// equivalent hand-built facts map.
func TestFactsForAssetParity(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	dgSvc := NewDependencyGroupService(d)
	ctx := context.Background()
	if err := dgSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	if err := dgSvc.LinkModule(ctx, tenantID, "mod.parity", mustGroupIDBySlug(t, dgSvc, ctx, tenantID, "any-linux"), "requirement"); err != nil {
		t.Fatalf("LinkModule failed: %v", err)
	}

	payload := `{"os_family":"linux","os_package_family":"deb","disk_encryption":"luks","hostname":"parity-1"}`
	asset := mustCreateAsset(t, d, tenantID, "11111111-1111-1111-1111-111111111111", payload)

	handBuilt := map[string]string{"os_family": "linux", "os_package_family": "deb", "disk_encryption": "luks", "hostname": "parity-1"}
	wantResult, err := dgSvc.Evaluate(ctx, tenantID, "mod.parity", handBuilt)
	if err != nil {
		t.Fatalf("Evaluate (hand-built facts) failed: %v", err)
	}
	gotResult, err := dgSvc.EvaluateForAsset(ctx, tenantID, "mod.parity", asset)
	if err != nil {
		t.Fatalf("EvaluateForAsset failed: %v", err)
	}
	if gotResult.Status != wantResult.Status {
		t.Fatalf("parity mismatch: hand-built facts -> %v, FactsForAsset -> %v", wantResult.Status, gotResult.Status)
	}
	if gotResult.Status != dependencygroups.StatusEligible {
		t.Fatalf("want eligible for a linux asset against any-linux, got %v", gotResult.Status)
	}

	// FactsForAsset itself must expose the same trailing-key facts a
	// hand-built map would (the exact keys eval.go's paramKey resolves
	// against).
	facts := FactsForAsset(asset)
	for k, v := range handBuilt {
		if facts[k] != v {
			t.Fatalf("FactsForAsset[%q] = %q, want %q", k, facts[k], v)
		}
	}
}

func mustGroupIDBySlug(t *testing.T, svc *DependencyGroupService, ctx context.Context, tenantID int64, slug string) int64 {
	t.Helper()
	groups, err := svc.ListByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListByTenant failed: %v", err)
	}
	for _, g := range groups {
		if g.Slug == slug {
			return g.ID
		}
	}
	t.Fatalf("builtin group %q not found", slug)
	return 0
}

// TestFactsForIdentityRegistryPaths verifies FactsForIdentity's keys are
// exactly the trailing segments of catalog/params' registered "user/..."
// canonical paths (SchemaIdentity, PathEntity "user") -- the identity
// analogue of FactsForAsset's parity guarantee.
func TestFactsForIdentityRegistryPaths(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	ident := mustCreateIdentity(t, d, tenantID, "jdoe", "jdoe@example.com", "")

	facts := FactsForIdentity(ident)

	for _, key := range []string{"username", "email", "role", "display_name"} {
		path := params.PathFor("user", key)
		if path == "" {
			t.Fatalf("registry has no canonical path for user/%s", key)
		}
		if _, ok := facts[key]; !ok {
			t.Fatalf("FactsForIdentity missing key %q (canonical path %s)", key, path)
		}
	}
	if facts["username"] != "jdoe" || facts["email"] != "jdoe@example.com" {
		t.Fatalf("unexpected identity facts: %+v", facts)
	}
}

// TestEvaluateDynamicMembership_AssetRule: an asset dynamic group with a
// single os_family=linux rule gains the linux asset (source='rule') and
// excludes the windows one.
func TestEvaluateDynamicMembership_AssetRule(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	ctx := context.Background()
	g := mustCreateDynamicGroup(t, d, tenantID, "linux-boxes", MemberKindAsset)

	linux := mustCreateAsset(t, d, tenantID, "22222222-2222-2222-2222-222222222222", `{"os_family":"linux"}`)
	win := mustCreateAsset(t, d, tenantID, "33333333-3333-3333-3333-333333333333", `{"os_family":"windows"}`)

	if _, err := svc.AddRule(ctx, g.ID, "param", "computer/hardware/os_family", string(dependencygroups.OpEquals), []string{"linux"}, "", ""); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}

	members, err := d.Queries.ListAssetsInGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListAssetsInGroup failed: %v", err)
	}
	if len(members) != 1 || members[0].ID != linux.ID {
		t.Fatalf("want only linux asset as member, got %+v", members)
	}
	_ = win

	memberships, err := d.Queries.ListRuleSourcedAssetIDsForGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListRuleSourcedAssetIDsForGroup failed: %v", err)
	}
	if len(memberships) != 1 || !memberships[0].Valid || memberships[0].Int64 != linux.ID {
		t.Fatalf("want linux asset rule-sourced, got %+v", memberships)
	}
}

// TestEvaluateDynamicMembership_DirectMembersSurvive: a direct member
// that would NOT match the rule survives recalculation untouched.
func TestEvaluateDynamicMembership_DirectMembersSurvive(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	ctx := context.Background()
	g := mustCreateDynamicGroup(t, d, tenantID, "linux-boxes-2", MemberKindAsset)

	winDirect := mustCreateAsset(t, d, tenantID, "44444444-4444-4444-4444-444444444444", `{"os_family":"windows"}`)
	if err := svc.AddAssetMember(ctx, g.ID, winDirect.ID); err != nil {
		t.Fatalf("AddAssetMember failed: %v", err)
	}

	if _, err := svc.AddRule(ctx, g.ID, "param", "computer/hardware/os_family", string(dependencygroups.OpEquals), []string{"linux"}, "", ""); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	if _, err := svc.EvaluateDynamicMembership(ctx, g.ID); err != nil {
		t.Fatalf("EvaluateDynamicMembership failed: %v", err)
	}

	members, err := d.Queries.ListAssetsInGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListAssetsInGroup failed: %v", err)
	}
	found := false
	for _, m := range members {
		if m.ID == winDirect.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("direct windows member was removed by dynamic evaluation")
	}
}

// TestEvaluateDynamicMembership_StaleRemoved: a rule-sourced member that
// stops matching after its facts change is removed on the next
// evaluation.
func TestEvaluateDynamicMembership_StaleRemoved(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	ctx := context.Background()
	g := mustCreateDynamicGroup(t, d, tenantID, "linux-boxes-3", MemberKindAsset)

	asset := mustCreateAsset(t, d, tenantID, "55555555-5555-5555-5555-555555555555", `{"os_family":"linux"}`)
	if _, err := svc.AddRule(ctx, g.ID, "param", "computer/hardware/os_family", string(dependencygroups.OpEquals), []string{"linux"}, "", ""); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	if cnt, err := d.Queries.CountAssetsInGroup(ctx, g.ID); err != nil || cnt != 1 {
		t.Fatalf("want 1 member after first evaluation, got %d err=%v", cnt, err)
	}

	if err := d.Queries.UpdateAssetPayload(ctx, db.UpdateAssetPayloadParams{ID: asset.ID, Payload: `{"os_family":"windows"}`}); err != nil {
		t.Fatalf("UpdateAssetPayload failed: %v", err)
	}
	result, err := svc.EvaluateDynamicMembership(ctx, g.ID)
	if err != nil {
		t.Fatalf("EvaluateDynamicMembership failed: %v", err)
	}
	if result.Removed != 1 || result.Total != 0 {
		t.Fatalf("want removed=1 total=0, got %+v", result)
	}
}

// TestEvaluateDynamicMembership_MixedGroup: a mixed group's rule
// evaluates against both assets and identities independently. Using an
// "exists" rule on the "email" fact key (present on identities, never
// on assets) proves both candidate populations are actually walked: the
// identity is added, the asset is not.
func TestEvaluateDynamicMembership_MixedGroup(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	ctx := context.Background()
	g := mustCreateDynamicGroup(t, d, tenantID, "mixed-g", MemberKindMixed)

	asset := mustCreateAsset(t, d, tenantID, "66666666-6666-6666-6666-666666666666", `{"os_family":"linux"}`)
	ident := mustCreateIdentity(t, d, tenantID, "mixeduser", "mixeduser@example.com", "")

	if _, err := svc.AddRule(ctx, g.ID, "param", "user/identity/email", string(dependencygroups.OpExists), nil, "", ""); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}

	assetMembers, err := d.Queries.ListAssetsInGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListAssetsInGroup failed: %v", err)
	}
	if len(assetMembers) != 0 {
		t.Fatalf("want zero asset members (no email fact on assets), got %+v", assetMembers)
	}
	_ = asset

	identMembers, err := d.Queries.ListIdentitiesInGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListIdentitiesInGroup failed: %v", err)
	}
	if len(identMembers) != 1 || identMembers[0].ID != ident.ID {
		t.Fatalf("want the identity as a member, got %+v", identMembers)
	}
}

// TestEvaluateDynamicMembership_ScriptRuleZeroMembers: a script-kind
// rule alone never yields members -- "unknown" (the agent hasn't
// reported a script_result/<id> fact) is never treated as a pass.
func TestEvaluateDynamicMembership_ScriptRuleZeroMembers(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	ctx := context.Background()
	g := mustCreateDynamicGroup(t, d, tenantID, "script-only", MemberKindAsset)

	mustCreateAsset(t, d, tenantID, "77777777-7777-7777-7777-777777777777", `{"os_family":"linux"}`)

	if _, err := svc.AddRule(ctx, g.ID, "script", "", string(dependencygroups.OpExists), nil, "echo hi", ""); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}

	result, err := svc.EvaluateDynamicMembership(ctx, g.ID)
	if err != nil {
		t.Fatalf("EvaluateDynamicMembership failed: %v", err)
	}
	if result.Total != 0 || result.Added != 0 {
		t.Fatalf("want zero members from a lone script rule, got %+v", result)
	}
}

// TestSetGroupMetaMemberKindConflict verifies changing member_kind away
// from "mixed" is rejected when the group still has members of the kind
// being excluded.
func TestSetGroupMetaMemberKindConflict(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	ctx := context.Background()
	g, err := d.Queries.CreateGroup(ctx, db.CreateGroupParams{TenantID: tenantID, Name: "conflict-g", Slug: "conflict-g"})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	asset := mustCreateAsset(t, d, tenantID, "88888888-8888-8888-8888-888888888888", `{}`)
	if err := svc.AddAssetMember(ctx, g.ID, asset.ID); err != nil {
		t.Fatalf("AddAssetMember failed: %v", err)
	}
	err = svc.SetGroupMeta(ctx, g.ID, "", MemberKindIdentity, MembershipStatic, "all")
	if !errors.Is(err, ErrMemberKindConflict) {
		t.Fatalf("want ErrMemberKindConflict, got %v", err)
	}
	// Switching to the kind that already matches is fine.
	if err := svc.SetGroupMeta(ctx, g.ID, "d", MemberKindAsset, MembershipStatic, "all"); err != nil {
		t.Fatalf("unexpected error switching to matching kind: %v", err)
	}
}

// TestRemoveLastRuleClearsRuleSourcedMembers is the regression guard for
// the zero-rules mass-add bug: in the DEFAULT match_mode='all' (the
// dangerous case — 'any' is fail-closed on an empty condition set and
// hides the bug), removing the LAST rule of a dynamic group must clear
// its rule-sourced members to zero, NOT add every tenant candidate. A
// direct member (which never matched the rule) must survive.
func TestRemoveLastRuleClearsRuleSourcedMembers(t *testing.T) {
	d, tenantID := setupGroupRulesTestDB(t)
	svc := NewGroupService(d)
	ctx := context.Background()
	g := mustCreateDynamicGroup(t, d, tenantID, "zero-rules-clear", MemberKindAsset)
	if g.RulesMatchMode != "all" {
		t.Fatalf("precondition: want default match_mode 'all', got %q", g.RulesMatchMode)
	}

	// One rule matching only the linux box; plus a non-matching windows
	// box AND a direct windows member the rule never covers.
	linux := mustCreateAsset(t, d, tenantID, "a1a1a1a1-0000-0000-0000-000000000001", `{"os_family":"linux"}`)
	_ = mustCreateAsset(t, d, tenantID, "a2a2a2a2-0000-0000-0000-000000000002", `{"os_family":"windows"}`)
	directWin := mustCreateAsset(t, d, tenantID, "a3a3a3a3-0000-0000-0000-000000000003", `{"os_family":"windows"}`)
	if err := svc.AddAssetMember(ctx, g.ID, directWin.ID); err != nil {
		t.Fatalf("AddAssetMember(direct) failed: %v", err)
	}
	rule, err := svc.AddRule(ctx, g.ID, "param", "computer/hardware/os_family", string(dependencygroups.OpEquals), []string{"linux"}, "", "")
	if err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	// After AddRule's immediate evaluation: linux (rule) + directWin (direct) = 2.
	ruleIDs, err := d.Queries.ListRuleSourcedAssetIDsForGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListRuleSourcedAssetIDsForGroup failed: %v", err)
	}
	if len(ruleIDs) != 1 || !ruleIDs[0].Valid || ruleIDs[0].Int64 != linux.ID {
		t.Fatalf("want exactly the linux asset rule-sourced, got %+v", ruleIDs)
	}

	// Remove the last rule -> zero-rules path must clear rule-sourced
	// members, NOT mass-add every tenant asset.
	if err := svc.RemoveRule(ctx, g.ID, rule.ID); err != nil {
		t.Fatalf("RemoveRule failed: %v", err)
	}

	ruleIDs, err = d.Queries.ListRuleSourcedAssetIDsForGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListRuleSourcedAssetIDsForGroup (post-remove) failed: %v", err)
	}
	if len(ruleIDs) != 0 {
		t.Fatalf("removing the last rule must clear rule-sourced members, got %d still present", len(ruleIDs))
	}

	// The direct member survives; total is exactly the one direct member
	// (NOT the 3 tenant assets a vacuous-AND mass-add would have produced).
	total, err := d.Queries.CountAssetsInGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("CountAssetsInGroup failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("want 1 member (direct only) after removing last rule, got %d", total)
	}
	members, err := d.Queries.ListAssetsInGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListAssetsInGroup failed: %v", err)
	}
	if len(members) != 1 || members[0].ID != directWin.ID {
		t.Fatalf("want only the direct windows member to survive, got %+v", members)
	}
}
