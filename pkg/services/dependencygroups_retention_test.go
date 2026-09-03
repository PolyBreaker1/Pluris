package services_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/services"
)

func TestDependencyGroupSoftDeleteRestoreAndReferenceBoundary(t *testing.T) {
	svc, d, tenantID := newDGSvc(t)
	ctx := context.Background()
	group, err := svc.Create(ctx, tenantID, "Retention group", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkModule(ctx, tenantID, "pluris.retention.test", group.ID, "platform"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, group.ID, 41); err != nil {
		t.Fatal(err)
	}
	if active, _ := svc.ListByTenant(ctx, tenantID); len(active) != 0 {
		t.Fatalf("active dependency groups after delete = %d, want 0", len(active))
	}
	if deleted, _ := svc.ListDeletedByTenant(ctx, tenantID); len(deleted) != 1 {
		t.Fatalf("deleted dependency groups = %+v", deleted)
	}
	if links, err := svc.ListLinksForModule(ctx, tenantID, "pluris.retention.test"); err != nil || len(links) != 0 {
		t.Fatalf("deleted dependency group remained a module candidate: %+v, %v", links, err)
	}
	if err := svc.PermanentlyDelete(ctx, tenantID, group.ID); !errors.Is(err, services.ErrDependencyGroupReferenced) {
		t.Fatalf("permanent delete error = %v, want ErrDependencyGroupReferenced", err)
	}
	if err := svc.Restore(ctx, tenantID, group.ID); err != nil {
		t.Fatal(err)
	}
	if links, err := svc.ListLinksForModule(ctx, tenantID, "pluris.retention.test"); err != nil || len(links) != 1 {
		t.Fatalf("restored dependency group links = %+v, %v", links, err)
	}
	if err := svc.UnlinkModule(ctx, tenantID, "pluris.retention.test", group.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, group.ID, 41); err != nil {
		t.Fatal(err)
	}
	if err := svc.PermanentlyDelete(ctx, tenantID, group.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Queries.GetDependencyGroupIncludingDeleted(ctx, group.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("purged dependency group lookup error = %v, want sql.ErrNoRows", err)
	}
}

func TestDependencyGroupImmediateAndScheduledPurgeRespectReferences(t *testing.T) {
	svc, d, tenantID := newDGSvc(t)
	ctx := context.Background()
	if _, err := d.Queries.UpdateRetentionSetting(ctx, db.UpdateRetentionSettingParams{
		EntityKind: services.EntityKindDependencyGroup, Mode: services.RetentionModeImmediate, PurgeAfterDays: nil, UpdatedBy: int64(7),
	}); err != nil {
		t.Fatal(err)
	}
	linked, err := svc.Create(ctx, tenantID, "Immediate linked", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkModule(ctx, tenantID, "pluris.immediate.test", linked.ID, "requirement"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, linked.ID, 7); !errors.Is(err, services.ErrDependencyGroupReferenced) {
		t.Fatalf("immediate linked delete error = %v, want reference guard", err)
	}
	if _, err := svc.Get(ctx, linked.ID); err != nil {
		t.Fatalf("reference-blocked immediate delete hid group: %v", err)
	}

	if _, err := d.Queries.UpdateRetentionSetting(ctx, db.UpdateRetentionSettingParams{
		EntityKind: services.EntityKindDependencyGroup, Mode: services.RetentionModeSoft, PurgeAfterDays: int64(0), UpdatedBy: int64(7),
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, linked.ID, 7); err != nil {
		t.Fatal(err)
	}
	free, err := svc.Create(ctx, tenantID, "Purge free", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, tenantID, free.ID, 7); err != nil {
		t.Fatal(err)
	}
	results, err := services.NewRetentionService(d).PurgeExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var skippedLinked, purgedFree bool
	for _, result := range results {
		if result.EntityKind != services.EntityKindDependencyGroup {
			continue
		}
		if result.EntityID == linked.ID && result.Skipped && errors.Is(result.Err, services.ErrDependencyGroupReferenced) {
			skippedLinked = true
		}
		if result.EntityID == free.ID && result.Purged {
			purgedFree = true
		}
	}
	if !skippedLinked || !purgedFree {
		t.Fatalf("dependency group purge results = %+v", results)
	}
}
