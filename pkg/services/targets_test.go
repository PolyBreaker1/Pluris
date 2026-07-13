package services

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// seedTargetFixtures creates one of everything in tenantID: a computer
// asset, a server asset, a printer asset (must be excluded from the
// catalog -- no agent), a desk asset (same), an identity, a group with
// both an asset and an identity member (dual-listed under both group
// kinds -- see TargetService.groupTargets' TODO), and an enabled + a
// disabled configuration group (only the enabled one should surface).
// suffix keeps uuids/human_ids/slugs unique across the two tenants the
// isolation test seeds.
func seedTargetFixtures(t *testing.T, d *database.Database, tenantID int64, suffix string) {
	t.Helper()
	ctx := context.Background()

	if _, err := d.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "11111111-1111-1111-1111-1111111111" + suffix,
		TenantID:        tenantID,
		Subtype:         "computer",
		SubtypePayload:  `{"hostname":"lobby-pc","os_family":"linux","os_distribution":"Ubuntu","os_version":"24.04"}`,
		EnrollmentState: "enrolled",
		HumanID:         sql.NullString{String: "comp.tgt.lobby-pc." + suffix, Valid: true},
	}); err != nil {
		t.Fatalf("create computer asset: %v", err)
	}
	if _, err := d.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "22222222-2222-2222-2222-2222222222" + suffix,
		TenantID:        tenantID,
		Subtype:         "server",
		SubtypePayload:  `{"hostname":"build-srv","os_family":"linux","os_distribution":"Debian","os_version":"12"}`,
		EnrollmentState: "enrolled",
		HumanID:         sql.NullString{String: "srv.tgt.build-srv." + suffix, Valid: true},
	}); err != nil {
		t.Fatalf("create server asset: %v", err)
	}
	if _, err := d.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "33333333-3333-3333-3333-3333333333" + suffix,
		TenantID:        tenantID,
		Subtype:         "printer",
		SubtypePayload:  `{"model":"LaserJet"}`,
		EnrollmentState: "enrolled",
		HumanID:         sql.NullString{String: "prn.tgt.lobby-printer." + suffix, Valid: true},
	}); err != nil {
		t.Fatalf("create printer asset: %v", err)
	}
	if _, err := d.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "44444444-4444-4444-4444-4444444444" + suffix,
		TenantID:        tenantID,
		Subtype:         "desk",
		SubtypePayload:  `{"location_label":"3F-12"}`,
		EnrollmentState: "pending",
		HumanID:         sql.NullString{String: "desk.tgt.3f-12." + suffix, Valid: true},
	}); err != nil {
		t.Fatalf("create desk asset: %v", err)
	}

	idSvc := NewIdentityService(d)
	user, err := idSvc.Create(ctx, tenantID, identities.Identity{
		Username:    "tgtuser" + suffix,
		Email:       "tgtuser." + suffix + "@example.com",
		DisplayName: "Target User " + suffix,
		Title:       "Engineer",
		Role:        identities.RoleUser,
	})
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}

	group, err := d.Queries.CreateGroup(ctx, db.CreateGroupParams{
		TenantID: tenantID,
		Name:     "Engineering " + suffix,
		Slug:     "engineering-tgt-" + suffix,
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	computerAsset, err := d.Queries.GetAssetByHumanID(ctx, sql.NullString{String: "comp.tgt.lobby-pc." + suffix, Valid: true})
	if err != nil {
		t.Fatalf("get computer asset back: %v", err)
	}
	groupSvc := NewGroupService(d)
	if err := groupSvc.AddAssetMember(ctx, group.ID, computerAsset.ID); err != nil {
		t.Fatalf("add asset member: %v", err)
	}
	if err := groupSvc.AddIdentityMember(ctx, group.ID, user.ID); err != nil {
		t.Fatalf("add identity member: %v", err)
	}

	if _, err := d.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{
		TenantID: tenantID,
		Name:     "Baseline " + suffix,
		Scope:    "both",
	}); err != nil {
		t.Fatalf("create enabled cg: %v", err)
	}
	disabledCG, err := d.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{
		TenantID: tenantID,
		Name:     "Retired baseline " + suffix,
		Scope:    "machine",
	})
	if err != nil {
		t.Fatalf("create disabled cg: %v", err)
	}
	if _, err := d.Queries.UpdateConfigurationGroup(ctx, db.UpdateConfigurationGroupParams{
		ID:       disabledCG.ID,
		Name:     disabledCG.Name,
		Scope:    disabledCG.Scope,
		Disabled: true,
	}); err != nil {
		t.Fatalf("disable cg: %v", err)
	}
}

// TestTargetCatalog covers the Task 5.1 backend: seeded fixtures produce
// the right kinds/refs/labels, subtype mapping excludes printer/desk,
// disabled configuration groups are excluded, groups are dual-listed
// under both group kinds, tenant isolation holds, and allowedKinds
// filters the result.
func TestTargetCatalog(t *testing.T) {
	d, tenantID := setupIdentityTestDB(t)
	ctx := context.Background()
	seedTargetFixtures(t, d, tenantID, "a")

	otherTenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Other Org", Slug: "other-org-target-svc",
	})
	if err != nil {
		t.Fatalf("create other tenant: %v", err)
	}
	seedTargetFixtures(t, d, otherTenant.ID, "b")

	svc := NewTargetService(d)

	t.Run("all kinds, subtype filtering, disabled cg excluded", func(t *testing.T) {
		targets, err := svc.Catalog(ctx, tenantID, nil)
		if err != nil {
			t.Fatalf("Catalog failed: %v", err)
		}

		byKind := map[configgroups.TargetKind][]configgroups.Target{}
		for _, tg := range targets {
			byKind[tg.Kind] = append(byKind[tg.Kind], tg)
		}

		// computer + server both surface as KindComputer; printer + desk
		// are excluded (no agent -- see Catalog's doc comment).
		if got := len(byKind[configgroups.KindComputer]); got != 2 {
			t.Fatalf("KindComputer rows = %d, want 2 (computer + server, printer/desk excluded)", got)
		}
		for _, tg := range byKind[configgroups.KindComputer] {
			if tg.Label == "" {
				t.Errorf("computer target has empty label: %+v", tg)
			}
			if _, err := strconv.ParseInt(tg.Ref, 10, 64); err != nil {
				t.Errorf("computer target ref %q is not a numeric id: %v", tg.Ref, err)
			}
		}

		if got := len(byKind[configgroups.KindUser]); got != 1 {
			t.Fatalf("KindUser rows = %d, want 1", got)
		}
		if want := "Target User a"; byKind[configgroups.KindUser][0].Label != want {
			t.Errorf("user label = %q, want %q", byKind[configgroups.KindUser][0].Label, want)
		}

		// Every group is dual-listed under both group kinds until
		// Phase 6's member_kind lands (see groupTargets' TODO), and
		// carries the 2-member count (1 asset + 1 identity).
		if got := len(byKind[configgroups.KindComputerGroup]); got != 1 {
			t.Fatalf("KindComputerGroup rows = %d, want 1", got)
		}
		if got := len(byKind[configgroups.KindUserGroup]); got != 1 {
			t.Fatalf("KindUserGroup rows = %d, want 1", got)
		}
		if byKind[configgroups.KindComputerGroup][0].Ref != byKind[configgroups.KindUserGroup][0].Ref {
			t.Errorf("dual-listed group refs differ: %q vs %q",
				byKind[configgroups.KindComputerGroup][0].Ref, byKind[configgroups.KindUserGroup][0].Ref)
		}

		// Only the enabled configuration group appears.
		if got := len(byKind[configgroups.KindConfigurationGroup]); got != 1 {
			t.Fatalf("KindConfigurationGroup rows = %d, want 1 (disabled cg excluded)", got)
		}
		if want := "Baseline a"; byKind[configgroups.KindConfigurationGroup][0].Label != want {
			t.Errorf("cg label = %q, want %q", byKind[configgroups.KindConfigurationGroup][0].Label, want)
		}
	})

	t.Run("tenant isolation", func(t *testing.T) {
		targets, err := svc.Catalog(ctx, tenantID, []configgroups.TargetKind{configgroups.KindComputer})
		if err != nil {
			t.Fatalf("Catalog failed: %v", err)
		}
		if len(targets) != 2 {
			t.Fatalf("KindComputer rows for tenant A = %d, want 2 (tenant B's assets must not leak in)", len(targets))
		}
		for _, tg := range targets {
			for _, tag := range tg.Tags {
				if tag == "srv.tgt.build-srv.b" || tag == "comp.tgt.lobby-pc.b" {
					t.Fatalf("tenant B asset leaked into tenant A catalog: %+v", tg)
				}
			}
		}
	})

	t.Run("allowedKinds filters to just the requested kind", func(t *testing.T) {
		targets, err := svc.Catalog(ctx, tenantID, []configgroups.TargetKind{configgroups.KindUser})
		if err != nil {
			t.Fatalf("Catalog failed: %v", err)
		}
		if len(targets) != 1 {
			t.Fatalf("filtered rows = %d, want 1", len(targets))
		}
		if targets[0].Kind != configgroups.KindUser {
			t.Errorf("filtered kind = %q, want %q", targets[0].Kind, configgroups.KindUser)
		}
	})
}
