package services

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pluris/pluris/db"
)

// TestResolveForTarget covers Task 12: a policy bound in a configuration
// group reaches the asset both via a direct assignment and via a group
// assignment, dedupes to one row per binding, and a disabled binding
// reports status Disabled.
func TestResolveForTarget(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	svc := NewAssignmentService(database)

	asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "bbbbbbbb-cccc-dddd-eeee-ffffffffffff",
		TenantID:        tenantID,
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"lt-pol-01"}`,
		EnrollmentState: "enrolled",
		HumanID:         sql.NullString{String: "comp.test.lt-pol-01", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	group, err := database.Queries.CreateGroup(ctx, db.CreateGroupParams{
		TenantID: tenantID, Name: "Laptops", Slug: "laptops",
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	cg, err := database.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{
		TenantID: tenantID,
		Name:     "Baseline",
		Scope:    "machine",
	})
	if err != nil {
		t.Fatalf("CreateConfigurationGroup: %v", err)
	}
	binding, err := database.Queries.CreateConfigurationGroupBinding(ctx, db.CreateConfigurationGroupBindingParams{
		ConfigurationGroupID: cg.ID,
		PolicyUrn:            "sec.account.password.min-length",
		State:                "enabled",
		ParameterValues:      sql.NullString{String: `{"min_length":12}`, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateBinding: %v", err)
	}

	// Assign the same configuration group directly AND via the group.
	for _, target := range []struct {
		typ string
		id  int64
	}{{"asset", asset.ID}, {"group", group.ID}} {
		if _, err := database.Queries.CreateConfigurationGroupAssignment(ctx, db.CreateConfigurationGroupAssignmentParams{
			ConfigurationGroupID: cg.ID,
			TargetType:           target.typ,
			TargetID:             target.id,
		}); err != nil {
			t.Fatalf("CreateAssignment %s: %v", target.typ, err)
		}
	}

	rows, err := svc.ResolveForTarget(ctx, tenantID, "asset", asset.ID, []int64{group.ID}, 0)
	if err != nil {
		t.Fatalf("ResolveForTarget: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (dedupe by binding)", len(rows))
	}
	r := rows[0]
	if r.BindingID != binding.ID || r.SourceGroup != "Baseline" || r.Status != "Assigned" {
		t.Fatalf("unexpected row: %+v", r)
	}
	if r.PolicyName == "" || r.PolicyName == r.PolicyID {
		t.Logf("policy name fell back to raw id %q (catalog id not found) - acceptable fallback", r.PolicyID)
	}
	if r.ValueSummary != "min_length=12" {
		t.Fatalf("ValueSummary = %q, want min_length=12", r.ValueSummary)
	}

	// Disable the binding: status flips.
	if _, err := database.Queries.UpdateConfigurationGroupBinding(ctx, db.UpdateConfigurationGroupBindingParams{
		ID:              binding.ID,
		State:           "enabled",
		ParameterValues: binding.ParameterValues,
		ModuleID:        binding.ModuleID,
		Disabled:        true,
	}); err != nil {
		t.Fatalf("UpdateBinding: %v", err)
	}
	rows, err = svc.ResolveForTarget(ctx, tenantID, "asset", asset.ID, []int64{group.ID}, 0)
	if err != nil {
		t.Fatalf("ResolveForTarget after disable: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != "Disabled" {
		t.Fatalf("want one Disabled row, got %+v", rows)
	}
}

// TestResolveForTargetValues covers Task 14: one PolicyValue row per
// bound parameter, with the raw key preserved for the Configuration tab.
func TestResolveForTargetValues(t *testing.T) {
	database, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	svc := NewAssignmentService(database)

	asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "cccccccc-dddd-eeee-ffff-000000000000",
		TenantID:        tenantID,
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"lt-val-01"}`,
		EnrollmentState: "enrolled",
		HumanID:         sql.NullString{String: "comp.test.lt-val-01", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	cg, err := database.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{
		TenantID: tenantID, Name: "Values", Scope: "machine",
	})
	if err != nil {
		t.Fatalf("CreateConfigurationGroup: %v", err)
	}
	if _, err := database.Queries.CreateConfigurationGroupBinding(ctx, db.CreateConfigurationGroupBindingParams{
		ConfigurationGroupID: cg.ID,
		PolicyUrn:            "sec.account.password.min-length",
		State:                "enabled",
		ParameterValues:      sql.NullString{String: `{"min_length":12,"complexity":"high"}`, Valid: true},
	}); err != nil {
		t.Fatalf("CreateBinding: %v", err)
	}
	if _, err := database.Queries.CreateConfigurationGroupAssignment(ctx, db.CreateConfigurationGroupAssignmentParams{
		ConfigurationGroupID: cg.ID, TargetType: "asset", TargetID: asset.ID,
	}); err != nil {
		t.Fatalf("CreateAssignment: %v", err)
	}

	rows, err := svc.ResolveForTarget(ctx, tenantID, "asset", asset.ID, nil, 0)
	if err != nil {
		t.Fatalf("ResolveForTarget: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Values) != 2 {
		t.Fatalf("want 1 row with 2 values, got %+v", rows)
	}
	// Sorted by key: complexity, then min_length.
	if rows[0].Values[0].Key != "complexity" || rows[0].Values[0].Value != "high" {
		t.Fatalf("value[0] = %+v", rows[0].Values[0])
	}
	if rows[0].Values[1].Key != "min_length" || rows[0].Values[1].Value != "12" {
		t.Fatalf("value[1] = %+v", rows[0].Values[1])
	}
}
