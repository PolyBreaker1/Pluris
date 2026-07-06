package database

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/pluris/pluris/db"
)

func TestDatabaseBasicOperations(t *testing.T) {
	// Create temporary database
	dbPath := "test_pluris.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	// Open database
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	// Test: Create tenant
	tenant, err := database.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Test Org",
		Slug: "test-org",
	})
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}
	t.Logf("Created tenant: %+v", tenant)

	// Test: Create site
	site, err := database.Queries.CreateSite(ctx, db.CreateSiteParams{
		TenantID: tenant.ID,
		Name:     "HQ Office",
		Slug:     "hq",
	})
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}
	t.Logf("Created site: %+v", site)

	// Test: Create asset (computer)
	asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "550e8400-e29b-41d4-a716-446655440000",
		TenantID:        tenant.ID,
		SiteID:          sql.NullInt64{Int64: site.ID, Valid: true},
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"dev-laptop-001","os_family":"linux","ram_mb":16384}`,
		EnrollmentState: "pending",
		AgentVersion:    sql.NullString{String: "1.0.0", Valid: true},
		Labels:          sql.NullString{String: `{"env":"dev"}`, Valid: true},
	})
	if err != nil {
		t.Fatalf("Failed to create asset: %v", err)
	}
	t.Logf("Created asset: %+v", asset)

	// Test: List assets (now uses pagination params)
	assets, err := database.Queries.ListAssets(ctx, db.ListAssetsParams{
		TenantID: tenant.ID,
		Limit:    100,
		Offset:   0,
	})
	if err != nil {
		t.Fatalf("Failed to list assets: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("Expected 1 asset, got %d", len(assets))
	}
	t.Logf("Listed %d assets", len(assets))

	// Test: Get asset by UUID
	foundAsset, err := database.Queries.GetAssetByUUID(ctx, asset.Uuid)
	if err != nil {
		t.Fatalf("Failed to get asset by UUID: %v", err)
	}
	if foundAsset.ID != asset.ID {
		t.Fatalf("Expected asset ID %d, got %d", asset.ID, foundAsset.ID)
	}
	t.Logf("Found asset by UUID: %+v", foundAsset)

	// Test: Update asset enrollment
	err = database.Queries.UpdateAssetEnrollmentState(ctx, db.UpdateAssetEnrollmentStateParams{
		State: "enrolled",
		ID:    asset.ID,
	})
	if err != nil {
		t.Fatalf("Failed to update enrollment state: %v", err)
	}
	t.Logf("Updated asset enrollment state")

	// Test: Count assets
	count, err := database.Queries.CountAssetsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("Failed to count assets: %v", err)
	}
	if count != 1 {
		t.Fatalf("Expected count 1, got %d", count)
	}
	t.Logf("Asset count: %d", count)

	// Test: Search by hostname (JSON query)
	searchResults, err := database.Queries.SearchAssetsByHostname(ctx, db.SearchAssetsByHostnameParams{
		TenantID: tenant.ID,
		Search:   sql.NullString{String: "dev-laptop", Valid: true},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("Failed to search assets: %v", err)
	}
	if len(searchResults) != 1 {
		t.Fatalf("Expected 1 search result, got %d", len(searchResults))
	}
	t.Logf("Search found %d assets", len(searchResults))

	t.Logf("All tests passed!")
}
