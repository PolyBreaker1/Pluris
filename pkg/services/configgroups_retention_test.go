package services_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/services"
)

func TestConfigurationGroupSoftDeleteRestoreImmediateAndPurge(t *testing.T) {
	svc, _, d, tenantID := newCGSvc(t)
	ctx := context.Background()
	actor, err := d.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenantID, Username: "config-group-retention", Email: "config-group-retention@example.com",
		DisplayName: "Configuration Group Retention", Role: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	assetID := newTestAsset(t, d, tenantID)
	group, err := svc.Create(ctx, tenantID, "Soft delete", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddAssignment(ctx, tenantID, group.ID, "asset", assetID, 0, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, group.ID, actor.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := svc.ListByTenant(ctx, tenantID); len(active) != 0 {
		t.Fatalf("active groups after delete = %d, want 0", len(active))
	}
	deleted, err := svc.ListDeletedByTenant(ctx, tenantID)
	if err != nil || len(deleted) != 1 {
		t.Fatalf("deleted groups = %+v, %v", deleted, err)
	}
	deletedBy, ok := deleted[0].DeletedBy.(int64)
	if !ok || deletedBy != actor.ID {
		t.Fatalf("deleted_by = %#v, want %d", deleted[0].DeletedBy, actor.ID)
	}
	if assigned, err := d.Queries.ListConfigGroupsForAsset(ctx, db.ListConfigGroupsForAssetParams{AssetID: assetID, SiteID: 0, TenantID: tenantID}); err != nil || len(assigned) != 0 {
		t.Fatalf("deleted group resolved for asset: %+v, %v", assigned, err)
	}
	if count, _ := d.Queries.CountAssignmentsByGroup(ctx, group.ID); count != 1 {
		t.Fatalf("assignment count after soft delete = %d, want 1", count)
	}
	if err := svc.Restore(ctx, tenantID, group.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := svc.ListByTenant(ctx, tenantID); len(active) != 1 {
		t.Fatalf("active groups after restore = %d, want 1", len(active))
	}

	if _, err := d.Queries.UpdateRetentionSetting(ctx, db.UpdateRetentionSettingParams{
		EntityKind: services.EntityKindConfigurationGroup, Mode: services.RetentionModeImmediate,
		PurgeAfterDays: nil, UpdatedBy: actor.ID,
	}); err != nil {
		t.Fatal(err)
	}
	immediate, err := svc.Create(ctx, tenantID, "Immediate", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, immediate.ID, actor.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Queries.GetConfigurationGroupIncludingDeleted(ctx, immediate.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("immediate delete lookup error = %v, want sql.ErrNoRows", err)
	}

	if _, err := d.Queries.UpdateRetentionSetting(ctx, db.UpdateRetentionSettingParams{
		EntityKind: services.EntityKindConfigurationGroup, Mode: services.RetentionModeSoft,
		PurgeAfterDays: int64(0), UpdatedBy: actor.ID,
	}); err != nil {
		t.Fatal(err)
	}
	purge, err := svc.Create(ctx, tenantID, "Purge", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, purge.ID, actor.ID); err != nil {
		t.Fatal(err)
	}
	results, err := services.NewRetentionService(d).PurgeExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, result := range results {
		if result.EntityKind == services.EntityKindConfigurationGroup && result.EntityID == purge.ID && result.Purged {
			found = true
		}
	}
	if !found {
		t.Fatalf("configuration group purge result missing: %+v", results)
	}
}
