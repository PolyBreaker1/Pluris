package services

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
)

func TestAssetOwnerPairing(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	identitySvc := NewIdentityService(database)
	assetSvc := NewAssetService(database)
	ctx := context.Background()

	owner, err := identitySvc.Create(ctx, tenantID, identities.Identity{
		Username: "owner1", Email: "owner1@example.com",
		// Migration 003 renamed the user_self_service role slug to "user"
		// in the identities CHECK constraint. The catalog/identities
		// constants still carry the old slug until the registry-update
		// task lands, so use the raw new slug here.
		DisplayName: "Owner One", Role: identities.Role("user"),
	})
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}

	asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "aaaaaaaa-e29b-41d4-a716-446655440000",
		TenantID:        tenantID,
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"pair-test"}`,
		EnrollmentState: "pending",
	})
	if err != nil {
		t.Fatalf("failed to create asset: %v", err)
	}

	if err := assetSvc.SetOwner(ctx, asset.ID, owner.ID); err != nil {
		t.Fatalf("SetOwner failed: %v", err)
	}

	var ownerID sql.NullInt64
	err = database.Conn().QueryRow(`SELECT owner_identity_id FROM assets WHERE id = ?`, asset.ID).Scan(&ownerID)
	if err != nil {
		t.Fatalf("failed to read owner_identity_id: %v", err)
	}
	if !ownerID.Valid || ownerID.Int64 != owner.ID {
		t.Fatalf("expected owner_identity_id %d, got %+v", owner.ID, ownerID)
	}

	assigned, err := identitySvc.ListAssignedAssets(ctx, tenantID, owner.ID)
	if err != nil {
		t.Fatalf("ListAssignedAssets failed: %v", err)
	}
	// Note: assets.Asset.ID is the human-readable stable id (falls back to
	// UUID when no human_id is set), not the numeric DB primary key, so we
	// compare against the asset's UUID rather than asset.ID (int64).
	if len(assigned) != 1 || assigned[0].UUID != asset.Uuid {
		t.Fatalf("expected 1 assigned asset with uuid %q, got %+v", asset.Uuid, assigned)
	}

	if err := assetSvc.ClearOwner(ctx, asset.ID); err != nil {
		t.Fatalf("ClearOwner failed: %v", err)
	}
	assignedAfter, err := identitySvc.ListAssignedAssets(ctx, tenantID, owner.ID)
	if err != nil {
		t.Fatalf("ListAssignedAssets after clear failed: %v", err)
	}
	if len(assignedAfter) != 0 {
		t.Fatalf("expected 0 assigned assets after clear, got %d", len(assignedAfter))
	}
}
