package services_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
)

func newCGSvc(t *testing.T) (*services.ConfigGroupService, *services.PolicyModuleService, *database.Database, int64) {
	t.Helper()
	d, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ten, err := d.Queries.CreateTenant(context.Background(), db.CreateTenantParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	moduleSvc := services.NewPolicyModuleService(d)
	return services.NewConfigGroupService(d, moduleSvc), moduleSvc, d, ten.ID
}

func newTestAsset(t *testing.T, d *database.Database, tenantID int64) int64 {
	t.Helper()
	a, err := d.Queries.CreateAsset(context.Background(), db.CreateAssetParams{
		Uuid: "uuid-" + t.Name(), TenantID: tenantID, Subtype: "computer",
		SubtypePayload: "{}", EnrollmentState: "enrolled",
	})
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func newTestIdentityCG(t *testing.T, d *database.Database, tenantID int64) int64 {
	t.Helper()
	ident, err := d.Queries.CreateIdentity(context.Background(), db.CreateIdentityParams{
		TenantID: tenantID, Username: "u-" + t.Name(), Email: t.Name() + "@acme.local",
		DisplayName: "Test User", Role: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ident.ID
}

// newPublishedModule creates a tenant module with one published version
// carrying the given parameters_schema JSON, returning its URN.
func newPublishedModule(t *testing.T, d *database.Database, moduleSvc *services.PolicyModuleService, tenantID int64, urn, schema string) string {
	t.Helper()
	return newPublishedModuleSatisfying(t, d, moduleSvc, tenantID, urn, schema, []string{"sec.test.policy"})
}

// newPublishedModuleSatisfying is newPublishedModule with an explicit
// Satisfies list, letting a test bind several policy URNs to the same
// schema (a binding's policy_urn is unique per group, so a matrix of
// validation cases needs distinct URNs all resolving to one module).
func newPublishedModuleSatisfying(t *testing.T, d *database.Database, moduleSvc *services.PolicyModuleService, tenantID int64, urn, schema string, satisfies []string) string {
	t.Helper()
	ctx := context.Background()
	mod, err := moduleSvc.CreateModule(ctx, &tenantID, nil, urn, "Test Module", "desc")
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := d.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenantID, Username: "pub-" + t.Name(), Email: t.Name() + "-pub@acme.local",
		DisplayName: "Publisher", Role: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{
		Version: "1.0.0", Scope: "machine", Satisfies: satisfies,
		ParametersSchema: schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Publishing requires an apply-phase script (INV-M3).
	if _, err := moduleSvc.SetScript(ctx, draft.ID, policymodules.PhaseApply, "enforce.sh", "#!/usr/bin/env bash\necho ok\n"); err != nil {
		t.Fatal(err)
	}
	if err := moduleSvc.Publish(ctx, draft.ID, publisher.ID); err != nil {
		t.Fatal(err)
	}
	return urn
}

func TestConfigGroupCRUDAndTenantIsolation(t *testing.T) {
	svc, _, _, ten1 := newCGSvc(t)
	ctx := context.Background()

	g, err := svc.Create(ctx, ten1, "Baseline", "desc", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if g.Disabled {
		t.Fatal("expected enabled group")
	}

	got, err := svc.Get(ctx, ten1, g.ID)
	if err != nil || got.Name != "Baseline" {
		t.Fatalf("Get: %v %+v", err, got)
	}

	// Cross-tenant read fails closed.
	if _, err := svc.Get(ctx, ten1+999, g.ID); !errors.Is(err, services.ErrConfigGroupNotFound) {
		t.Fatalf("expected ErrConfigGroupNotFound for cross-tenant read, got %v", err)
	}

	updated, err := svc.UpdateFields(ctx, ten1, g.ID, map[string]string{"name": "Renamed", "enabled": "false"})
	if err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("expected 2 updated fields, got %v", updated)
	}
	got, _ = svc.Get(ctx, ten1, g.ID)
	if got.Name != "Renamed" || !got.Disabled {
		t.Fatalf("update did not apply: %+v", got)
	}

	if _, err := svc.UpdateFields(ctx, ten1, g.ID, map[string]string{"bogus": "x"}); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for unknown field, got %v", err)
	}
	if _, err := svc.UpdateFields(ctx, ten1, g.ID, map[string]string{"name": ""}); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for empty name, got %v", err)
	}

	list, err := svc.ListByTenant(ctx, ten1)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByTenant: %v %+v", err, list)
	}

	if err := svc.Delete(ctx, ten1, g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, ten1, g.ID); !errors.Is(err, services.ErrConfigGroupNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestAssignmentKindMappingAndCRUD(t *testing.T) {
	svc, _, d, ten := newCGSvc(t)
	ctx := context.Background()
	g, err := svc.Create(ctx, ten, "G1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	assetID := newTestAsset(t, d, ten)

	tt, err := services.KindToTargetType(configgroups.KindComputer)
	if err != nil || tt != "asset" {
		t.Fatalf("KindToTargetType(computer) = %q, %v", tt, err)
	}
	if tt, _ := services.KindToTargetType(configgroups.KindUser); tt != "identity" {
		t.Fatalf("KindToTargetType(user) = %q", tt)
	}
	if tt, _ := services.KindToTargetType(configgroups.KindComputerGroup); tt != "group" {
		t.Fatalf("KindToTargetType(computer_group) = %q", tt)
	}
	if _, err := services.KindToTargetType(configgroups.KindConfigurationGroup); !errors.Is(err, services.ErrInvalidTargetType) {
		t.Fatalf("expected ErrInvalidTargetType for configuration_group kind, got %v", err)
	}

	a, err := svc.AddAssignment(ctx, ten, g.ID, "asset", assetID, 5, true)
	if err != nil {
		t.Fatalf("AddAssignment: %v", err)
	}
	if a.Priority != 5 || !a.Enforced {
		t.Fatalf("assignment fields not persisted: %+v", a)
	}

	if _, err := svc.AddAssignment(ctx, ten, g.ID, "asset", 999999, 0, false); !errors.Is(err, services.ErrTargetNotFound) {
		t.Fatalf("expected ErrTargetNotFound for missing asset, got %v", err)
	}
	if _, err := svc.AddAssignment(ctx, ten, g.ID, "bogus", assetID, 0, false); !errors.Is(err, services.ErrInvalidTargetType) {
		t.Fatalf("expected ErrInvalidTargetType, got %v", err)
	}

	// Idempotent repeat.
	again, err := svc.AddAssignment(ctx, ten, g.ID, "asset", assetID, 5, true)
	if err != nil {
		t.Fatalf("repeat AddAssignment should be idempotent, got %v", err)
	}
	if again.ID != a.ID {
		t.Fatalf("expected same assignment row back, got different id")
	}

	updated, err := svc.UpdateAssignment(ctx, ten, g.ID, a.ID, 9, false)
	if err != nil || updated.Priority != 9 || updated.Enforced {
		t.Fatalf("UpdateAssignment: %v %+v", err, updated)
	}

	list, err := svc.ListAssignments(ctx, ten, g.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListAssignments: %v %+v", err, list)
	}

	if err := svc.RemoveAssignment(ctx, ten, g.ID, a.ID); err != nil {
		t.Fatalf("RemoveAssignment: %v", err)
	}
	list, _ = svc.ListAssignments(ctx, ten, g.ID)
	if len(list) != 0 {
		t.Fatalf("expected 0 assignments after remove, got %d", len(list))
	}
}

func TestBindingParameterValidationMatrix(t *testing.T) {
	svc, moduleSvc, d, ten := newCGSvc(t)
	ctx := context.Background()
	g, err := svc.Create(ctx, ten, "G1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	schema := `{"type":"object","properties":{"length":{"type":"number"},"mode":{"type":"string","enum":["strict","relaxed"]},"on":{"type":"boolean"}},"required":["length"]}`
	satisfies := []string{"sec.test.policy", "sec.test.policy2", "sec.test.policy3", "sec.test.policy4", "sec.test.policy5", "sec.test.policy6", "sec.test.other", "sec.test.yet-another"}
	urn := newPublishedModuleSatisfying(t, d, moduleSvc, ten, "tenant.acme.test-module", schema, satisfies)

	// Valid values.
	b, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.policy", "", "", map[string]string{"length": "14", "mode": "strict", "on": "true"}, "enabled")
	if err != nil {
		t.Fatalf("AddBinding valid: %v", err)
	}
	if !b.ParameterValues.Valid {
		t.Fatal("expected parameter_values to be set")
	}

	// Unknown key.
	if _, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.policy2", "", "", map[string]string{"bogus": "x", "length": "1"}, "enabled"); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for unknown key, got %v", err)
	}

	// Missing required.
	if _, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.policy3", "", "", map[string]string{"mode": "strict"}, "enabled"); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for missing required, got %v", err)
	}

	// Wrong type.
	if _, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.policy4", "", "", map[string]string{"length": "not-a-number"}, "enabled"); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for wrong type, got %v", err)
	}

	// Enum violation.
	if _, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.policy5", "", "", map[string]string{"length": "1", "mode": "bogus"}, "enabled"); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for enum violation, got %v", err)
	}

	// Invalid state.
	if _, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.policy6", "", "", map[string]string{"length": "1"}, "bogus-state"); !errors.Is(err, services.ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}

	// Explicit module override.
	b2, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.other", urn, "", map[string]string{"length": "2"}, "enabled")
	if err != nil {
		t.Fatalf("AddBinding with override: %v", err)
	}
	if !b2.ModuleID.Valid {
		t.Fatal("expected module_id set from override")
	}

	// Unresolvable policy (no module satisfies it, no override) skips
	// validation entirely -- unknown keys are accepted.
	b3, err := svc.AddBinding(ctx, ten, g.ID, "policy.nothing.satisfies", "", "", map[string]string{"anything": "goes"}, "not_configured")
	if err != nil {
		t.Fatalf("AddBinding unresolvable policy should skip validation, got %v", err)
	}
	if b3.ModuleID.Valid {
		t.Fatal("expected module_id to stay NULL when no override given")
	}

	// Unknown override module name is a hard error.
	if _, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.yet-another", "no.such.module", "", nil, "enabled"); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for unknown override module, got %v", err)
	}

	// Duplicate policy_urn in the same group is rejected.
	if _, err := svc.AddBinding(ctx, ten, g.ID, "sec.test.policy", "", "", map[string]string{"length": "1"}, "enabled"); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for duplicate binding, got %v", err)
	}

	// Update + remove + list round-trip.
	upd, err := svc.UpdateBinding(ctx, ten, g.ID, b.ID, "", "", map[string]string{"length": "20"}, "disabled")
	if err != nil {
		t.Fatalf("UpdateBinding: %v", err)
	}
	if upd.State != "disabled" {
		t.Fatalf("expected updated state, got %q", upd.State)
	}

	list, err := svc.ListBindings(ctx, ten, g.ID)
	if err != nil || len(list) != 3 {
		t.Fatalf("ListBindings: %v (len=%d)", err, len(list))
	}

	if err := svc.RemoveBinding(ctx, ten, g.ID, b.ID); err != nil {
		t.Fatalf("RemoveBinding: %v", err)
	}
	list, _ = svc.ListBindings(ctx, ten, g.ID)
	if len(list) != 2 {
		t.Fatalf("expected 2 bindings after remove, got %d", len(list))
	}
}

func TestModuleCandidatesFiltersBySatisfies(t *testing.T) {
	svc, moduleSvc, d, ten := newCGSvc(t)
	ctx := context.Background()
	schema := `{"type":"object","properties":{}}`
	newPublishedModule(t, d, moduleSvc, ten, "tenant.acme.match", schema)

	cands, err := svc.ModuleCandidates(ctx, ten, "sec.test.policy")
	if err != nil {
		t.Fatalf("ModuleCandidates: %v", err)
	}
	if len(cands) != 1 || cands[0].ID != "tenant.acme.match" {
		t.Fatalf("expected 1 matching candidate, got %+v", cands)
	}

	none, err := svc.ModuleCandidates(ctx, ten, "sec.nothing.matches")
	if err != nil || len(none) != 0 {
		t.Fatalf("expected 0 candidates, got %v %+v", err, none)
	}
}

func TestAssignPolicyDirectFindOrCreateAndIdempotent(t *testing.T) {
	svc, _, d, ten := newCGSvc(t)
	ctx := context.Background()
	assetID := newTestAsset(t, d, ten)

	cg1, err := svc.AssignPolicyDirect(ctx, ten, "Direct - my-host", "Direct assignments for my-host", "machine", "sec.remote-access.ssh.password-auth", "asset", assetID)
	if err != nil {
		t.Fatalf("AssignPolicyDirect: %v", err)
	}
	// Repeat should find the same group and no-op the duplicate writes.
	cg2, err := svc.AssignPolicyDirect(ctx, ten, "Direct - my-host", "Direct assignments for my-host", "machine", "sec.remote-access.ssh.password-auth", "asset", assetID)
	if err != nil {
		t.Fatalf("repeat AssignPolicyDirect: %v", err)
	}
	if cg1.ID != cg2.ID {
		t.Fatalf("expected find-or-create to return the same group, got %d vs %d", cg1.ID, cg2.ID)
	}

	bindings, err := svc.ListBindings(ctx, ten, cg1.ID)
	if err != nil || len(bindings) != 1 {
		t.Fatalf("expected exactly 1 binding after repeat calls, got %v %+v", err, bindings)
	}
	assignments, err := svc.ListAssignments(ctx, ten, cg1.ID)
	if err != nil || len(assignments) != 1 {
		t.Fatalf("expected exactly 1 assignment after repeat calls, got %v %+v", err, assignments)
	}
}

func TestUserTargetIdentityKindMapping(t *testing.T) {
	svc, _, d, ten := newCGSvc(t)
	ctx := context.Background()
	g, err := svc.Create(ctx, ten, "G1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	identID := newTestIdentityCG(t, d, ten)
	a, err := svc.AddAssignment(ctx, ten, g.ID, "identity", identID, 0, false)
	if err != nil {
		t.Fatalf("AddAssignment identity: %v", err)
	}
	if a.TargetType != "identity" {
		t.Fatalf("expected target_type identity, got %q", a.TargetType)
	}
}

// publishModule creates+publishes a module owned by ownerTenant (pass
// nil for a bundled module, tenant_id NULL), satisfying policyURN, and
// returns its URN. The publisher identity is created under
// publisherTenant (a real tenant is required for the FK).
func publishModule(t *testing.T, d *database.Database, moduleSvc *services.PolicyModuleService, ownerTenant *int64, publisherTenant int64, urn, policyURN string) string {
	t.Helper()
	ctx := context.Background()
	mod, err := moduleSvc.CreateModule(ctx, ownerTenant, nil, urn, "Test Module", "desc")
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := d.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: publisherTenant, Username: "pub-" + urn, Email: urn + "-pub@acme.local",
		DisplayName: "Publisher", Role: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{
		Version: "1.0.0", Scope: "machine", Satisfies: []string{policyURN},
		ParametersSchema: `{"type":"object","properties":{"length":{"type":"number"}},"required":["length"]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := moduleSvc.SetScript(ctx, draft.ID, policymodules.PhaseApply, "enforce.sh", "#!/usr/bin/env bash\necho ok\n"); err != nil {
		t.Fatal(err)
	}
	if err := moduleSvc.Publish(ctx, draft.ID, publisher.ID); err != nil {
		t.Fatal(err)
	}
	return urn
}

// TestBindingModuleOverrideTenantIsolation locks the cross-tenant
// disclosure fix: a binding's module_urn override is resolved with
// tenant isolation (mirrors resolveTenantModuleByURN), so tenant A can
// never pin tenant B's tenant-owned module -- while a bundled module
// (legitimately cross-visible) and the caller's own module are accepted.
func TestBindingModuleOverrideTenantIsolation(t *testing.T) {
	svc, moduleSvc, d, tenA := newCGSvc(t)
	ctx := context.Background()

	tenB, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "TenantB", Slug: "tenant-b"})
	if err != nil {
		t.Fatal(err)
	}

	// A tenant-B-owned module and a bundled module, both satisfying the
	// same policy URN.
	foreignURN := publishModule(t, d, moduleSvc, &tenB.ID, tenB.ID, "tenant.b.secret-module", "sec.iso.policy")
	bundledURN := publishModule(t, d, moduleSvc, nil, tenA, "pluris.iso.bundled", "sec.iso.policy")
	ownURN := publishModule(t, d, moduleSvc, &tenA, tenA, "tenant.a.own-module", "sec.iso.policy")

	g, err := svc.Create(ctx, tenA, "Iso Group", "", true)
	if err != nil {
		t.Fatal(err)
	}

	// NEGATIVE: tenant A binds with tenant B's override URN -> rejected,
	// nothing persisted.
	if _, err := svc.AddBinding(ctx, tenA, g.ID, "sec.iso.policy", foreignURN, "", map[string]string{"length": "1"}, "enabled"); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("expected ErrFieldValidation for foreign-tenant override, got %v", err)
	}
	bindings, err := svc.ListBindings(ctx, tenA, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 0 {
		t.Fatalf("foreign-tenant override must not persist any binding, got %d", len(bindings))
	}

	// POSITIVE: bundled override accepted.
	b1, err := svc.AddBinding(ctx, tenA, g.ID, "sec.iso.policy", bundledURN, "", map[string]string{"length": "2"}, "enabled")
	if err != nil {
		t.Fatalf("bundled override should be accepted: %v", err)
	}
	if !b1.ModuleID.Valid {
		t.Fatal("bundled override should set module_id")
	}

	// POSITIVE: own-tenant override accepted. Own module satisfies
	// sec.iso.policy; bind it in a second group so the per-group
	// unique(policy_urn) constraint doesn't collide with b1.
	g2, err := svc.Create(ctx, tenA, "Iso Group 2", "", true)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := svc.AddBinding(ctx, tenA, g2.ID, "sec.iso.policy", ownURN, "", map[string]string{"length": "3"}, "enabled")
	if err != nil {
		t.Fatalf("own-tenant override should be accepted: %v", err)
	}
	if !b2.ModuleID.Valid {
		t.Fatal("own-tenant override should set module_id")
	}

	// NEGATIVE via UpdateBinding: retarget an accepted binding to the
	// foreign module -> rejected, binding unchanged.
	if _, err := svc.UpdateBinding(ctx, tenA, g.ID, b1.ID, foreignURN, "", map[string]string{"length": "4"}, "enabled"); !errors.Is(err, services.ErrFieldValidation) {
		t.Fatalf("UpdateBinding to foreign override must be rejected, got %v", err)
	}
	after, err := svc.ListBindings(ctx, tenA, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || !after[0].ModuleID.Valid {
		t.Fatalf("binding must remain on its bundled override after rejected update, got %+v", after)
	}
}
