package services

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
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

func TestAssetSoftDeleteRestoreImmediateAndPurge(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	svc := NewAssetService(database)
	retention := NewRetentionService(database)
	create := func(id, uuid string) db.Asset {
		t.Helper()
		row, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
			Uuid: uuid, TenantID: tenantID, Subtype: "computer", SubtypePayload: `{"hostname":"` + id + `"}`,
			EnrollmentState: "enrolled", HumanID: sql.NullString{String: id, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}

	soft := create("comp.soft", "asset-soft")
	if err := svc.Delete(ctx, tenantID, soft.HumanID.String, 77); err != nil {
		t.Fatal(err)
	}
	active, err := svc.ListBySubtype(ctx, tenantID, "computer")
	if err != nil || len(active) != 0 {
		t.Fatalf("active list after delete = %+v, %v", active, err)
	}
	deleted, err := svc.ListDeletedBySubtype(ctx, tenantID, "computer")
	if err != nil || len(deleted) != 1 {
		t.Fatalf("deleted list = %+v, %v", deleted, err)
	}
	if got, err := svc.GetByID(ctx, soft.HumanID.String); err != nil || got != nil {
		t.Fatalf("default get after delete = %+v, %v", got, err)
	}
	targets, err := NewTargetService(database).Catalog(ctx, tenantID, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Ref == strconv.FormatInt(soft.ID, 10) {
			t.Fatalf("soft-deleted asset leaked into target picker: %+v", target)
		}
	}
	if err := svc.Restore(ctx, tenantID, soft.HumanID.String); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.GetByID(ctx, soft.HumanID.String); err != nil || got == nil {
		t.Fatalf("get after restore = %+v, %v", got, err)
	}

	if _, err := retention.UpdateSetting(ctx, EntityKindAsset, RetentionModeImmediate, nil, 77); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, soft.HumanID.String, 77); err != nil {
		t.Fatal(err)
	}
	_, err = database.Queries.GetAssetForDeletion(ctx, db.GetAssetForDeletionParams{TenantID: tenantID, Identifier: soft.HumanID})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("immediate lookup = %v, want sql.ErrNoRows", err)
	}

	if _, err := retention.UpdateSetting(ctx, EntityKindAsset, RetentionModeSoft, nil, 77); err != nil {
		t.Fatal(err)
	}
	expired := create("comp.expired", "asset-expired")
	if err := svc.Delete(ctx, tenantID, expired.HumanID.String, 77); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Conn().ExecContext(ctx, "UPDATE assets SET deleted_at = datetime('now', '-2 days') WHERE id = ?", expired.ID); err != nil {
		t.Fatal(err)
	}
	results, err := retention.PurgeExpired(ctx)
	if err != nil || len(results) != 0 {
		t.Fatalf("NULL asset window purge = %+v, %v", results, err)
	}
	days := int64(1)
	if _, err := retention.UpdateSetting(ctx, EntityKindAsset, RetentionModeSoft, &days, 77); err != nil {
		t.Fatal(err)
	}
	results, err = retention.PurgeExpired(ctx)
	if err != nil || len(results) != 1 || results[0].EntityKind != EntityKindAsset || !results[0].Purged {
		t.Fatalf("asset purge = %+v, %v", results, err)
	}
}
