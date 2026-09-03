package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
)

// TestGroupMembershipRoundTrip verifies the Task 9 GroupService: add an
// asset and an identity to a group, list both directions, then remove
// and confirm the rows disappear. Adds are idempotent.
func TestGroupMembershipRoundTrip(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	svc := NewGroupService(database)

	group, err := database.Queries.CreateGroup(ctx, db.CreateGroupParams{
		TenantID: tenantID,
		Name:     "Engineering",
		Slug:     "engineering",
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		TenantID:        tenantID,
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"lt-grp-01"}`,
		EnrollmentState: "enrolled",
		HumanID:         sql.NullString{String: "comp.test.lt-grp-01", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateAsset failed: %v", err)
	}

	idSvc := NewIdentityService(database)
	user, err := idSvc.Create(ctx, tenantID, identities.Identity{
		Username:    "gmember",
		Email:       "gmember@example.com",
		DisplayName: "Group Member",
		Role:        identities.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("Create identity failed: %v", err)
	}

	// Add both member kinds; a duplicate add must not error.
	if err := svc.AddAssetMember(ctx, group.ID, asset.ID); err != nil {
		t.Fatalf("AddAssetMember failed: %v", err)
	}
	if err := svc.AddAssetMember(ctx, group.ID, asset.ID); err != nil {
		t.Fatalf("idempotent AddAssetMember failed: %v", err)
	}
	if err := svc.AddIdentityMember(ctx, group.ID, user.ID); err != nil {
		t.Fatalf("AddIdentityMember failed: %v", err)
	}

	assetRows, err := svc.ListForAsset(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListForAsset failed: %v", err)
	}
	if len(assetRows) != 1 {
		t.Fatalf("ListForAsset rows = %d, want 1 (idempotent add)", len(assetRows))
	}
	if assetRows[0].Name != "Engineering" || assetRows[0].Source != "Direct" {
		t.Fatalf("unexpected asset group row: %+v", assetRows[0])
	}
	if assetRows[0].AddedAt.IsZero() {
		t.Fatal("AddedAt should carry the membership creation time")
	}

	userRows, err := svc.ListForIdentity(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListForIdentity failed: %v", err)
	}
	if len(userRows) != 1 || userRows[0].ID != group.ID {
		t.Fatalf("unexpected identity group rows: %+v", userRows)
	}

	// Remove both and confirm empty.
	if err := svc.RemoveAssetMember(ctx, group.ID, asset.ID); err != nil {
		t.Fatalf("RemoveAssetMember failed: %v", err)
	}
	if err := svc.RemoveIdentityMember(ctx, group.ID, user.ID); err != nil {
		t.Fatalf("RemoveIdentityMember failed: %v", err)
	}
	assetRows, err = svc.ListForAsset(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListForAsset after remove failed: %v", err)
	}
	if len(assetRows) != 0 {
		t.Fatalf("asset still in %d groups after remove", len(assetRows))
	}
	userRows, err = svc.ListForIdentity(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListForIdentity after remove failed: %v", err)
	}
	if len(userRows) != 0 {
		t.Fatalf("identity still in %d groups after remove", len(userRows))
	}
}

func TestGroupSoftDeleteRestoreImmediateAndReferencedPurge(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	svc := NewGroupService(database)
	retention := NewRetentionService(database)
	group, err := svc.Create(ctx, tenantID, "Recycle group", "", MemberKindMixed, MembershipStatic, "security", "global")
	if err != nil {
		t.Fatal(err)
	}
	configSvc := NewConfigGroupService(database, NewPolicyModuleService(database))
	configGroup, err := configSvc.Create(ctx, tenantID, "References group", "", true)
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := configSvc.AddAssignment(ctx, tenantID, configGroup.ID, "group", group.ID, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, group.ID, 88); err != nil {
		t.Fatalf("referenced soft delete: %v", err)
	}
	if groups, err := svc.ListByTenant(ctx, tenantID); err != nil || len(groups) != 0 {
		t.Fatalf("active groups after delete = %+v, %v", groups, err)
	}
	if groups, err := svc.ListDeletedByTenant(ctx, tenantID); err != nil || len(groups) != 1 {
		t.Fatalf("deleted groups = %+v, %v", groups, err)
	}
	if err := svc.Restore(ctx, tenantID, group.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, group.ID, 88); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Conn().ExecContext(ctx, "UPDATE groups SET deleted_at = datetime('now', '-2 days') WHERE id = ?", group.ID); err != nil {
		t.Fatal(err)
	}
	days := int64(1)
	if _, err := retention.UpdateSetting(ctx, EntityKindGroup, RetentionModeSoft, &days, 88); err != nil {
		t.Fatal(err)
	}
	results, err := retention.PurgeExpired(ctx)
	if err != nil || len(results) != 1 || !results[0].Skipped || !errors.Is(results[0].Err, ErrGroupReferenced) {
		t.Fatalf("referenced group purge = %+v, %v", results, err)
	}
	if err := configSvc.RemoveAssignment(ctx, tenantID, configGroup.ID, assignment.ID); err != nil {
		t.Fatal(err)
	}
	results, err = retention.PurgeExpired(ctx)
	if err != nil || len(results) != 1 || !results[0].Purged {
		t.Fatalf("group purge after reference removal = %+v, %v", results, err)
	}

	immediate, err := svc.Create(ctx, tenantID, "Immediate group", "", MemberKindMixed, MembershipStatic, "security", "global")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retention.UpdateSetting(ctx, EntityKindGroup, RetentionModeImmediate, nil, 88); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, immediate.ID, 88); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Queries.GetGroupIncludingDeleted(ctx, immediate.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("immediate group lookup = %v", err)
	}
}
