package services

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
)

// TestAssetDescriptionAndManagedByRoundTrip verifies the Task 4 columns
// (assets.description, assets.managed_by_identity_id) flow from the
// SetAssetDescription/SetAssetManagedBy queries through AssetService
// reads: GetByID resolves both, and ListBySubtype rows carry them too.
func TestAssetDescriptionAndManagedByRoundTrip(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()

	// Create the identity that will manage the asset.
	idSvc := NewIdentityService(database)
	admin, err := idSvc.Create(ctx, tenantID, identities.Identity{
		Username:    "asmith",
		Email:       "asmith@example.com",
		DisplayName: "Alice Smith",
		Role:        identities.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("Create identity failed: %v", err)
	}

	// Create an asset.
	created, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "11111111-2222-3333-4444-555555555555",
		TenantID:        tenantID,
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"lt-0001"}`,
		EnrollmentState: "enrolled",
		HumanID:         sql.NullString{String: "comp.test.lt-0001", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateAsset failed: %v", err)
	}

	// Set the new columns via the generated queries.
	if err := database.Queries.SetAssetDescription(ctx, db.SetAssetDescriptionParams{
		ID:          created.ID,
		Description: sql.NullString{String: "Primary dev laptop", Valid: true},
	}); err != nil {
		t.Fatalf("SetAssetDescription failed: %v", err)
	}
	if err := database.Queries.SetAssetManagedBy(ctx, db.SetAssetManagedByParams{
		ID:        created.ID,
		ManagedBy: sql.NullInt64{Int64: admin.ID, Valid: true},
	}); err != nil {
		t.Fatalf("SetAssetManagedBy failed: %v", err)
	}

	// Read back via the service (GetAssetByHumanID JOIN path).
	assetSvc := NewAssetService(database)
	got, err := assetSvc.GetByID(ctx, "comp.test.lt-0001")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil asset")
	}
	if got.Description != "Primary dev laptop" {
		t.Fatalf("expected Description %q, got %q", "Primary dev laptop", got.Description)
	}
	if got.ManagedBy != "Alice Smith" {
		t.Fatalf("expected ManagedBy %q, got %q", "Alice Smith", got.ManagedBy)
	}

	// List path (ListAssetsBySubtype JOIN) must carry them too.
	list, err := assetSvc.ListBySubtype(ctx, tenantID, "computer")
	if err != nil {
		t.Fatalf("ListBySubtype failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(list))
	}
	if list[0].Description != "Primary dev laptop" {
		t.Fatalf("list row: expected Description %q, got %q", "Primary dev laptop", list[0].Description)
	}
	if list[0].ManagedBy != "Alice Smith" {
		t.Fatalf("list row: expected ManagedBy %q, got %q", "Alice Smith", list[0].ManagedBy)
	}
}
