package services

import (
	"context"
	"database/sql"
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
