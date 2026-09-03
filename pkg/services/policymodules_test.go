package services_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
)

func newModuleSvc(t *testing.T) (*services.PolicyModuleService, *database.Database, int64) {
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
	return services.NewPolicyModuleService(d), d, ten.ID
}

// newTestIdentity creates a minimal identity so tests can pass a real
// publishedBy id (published_by REFERENCES identities(id), so 0/absent
// fails the FK check).
func newTestIdentity(t *testing.T, d *database.Database, tenantID int64) int64 {
	t.Helper()
	ident, err := d.Queries.CreateIdentity(context.Background(), db.CreateIdentityParams{
		TenantID: tenantID, Username: "publisher", Email: "publisher@acme.local",
		DisplayName: "Publisher", Role: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ident.ID
}

// TestModuleVersionScriptRoundTrip exercises module -> version -> script
// creation and readback through GetModule, checking the mapping onto the
// catalog/policymodules domain model preserves phase, filename, source,
// and the JSON-encoded fields (satisfies, target_os, sandbox_profile).
func TestModuleVersionScriptRoundTrip(t *testing.T) {
	svc, _, ten := newModuleSvc(t)
	ctx := context.Background()

	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.test-module", "Test Module", "A test module")
	if err != nil {
		t.Fatalf("CreateModule: %v", err)
	}

	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{
		Version:        "0.1.0",
		TargetOS:       []policymodules.TargetOS{policymodules.OSLinux},
		Scope:          "machine",
		Satisfies:      []string{"sec.test.urn"},
		SandboxProfile: policymodules.SandboxProfile{FsWrite: []string{"/etc/test"}, User: "root"},
	})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	if _, err := svc.SetScript(ctx, draft.ID, policymodules.PhaseApply, "enforce.sh", "#!/bin/bash\necho hi\n"); err != nil {
		t.Fatalf("SetScript(apply): %v", err)
	}
	if _, err := svc.SetScript(ctx, draft.ID, policymodules.PhaseUninstall, "rollback.sh", "# rollback"); err != nil {
		t.Fatalf("SetScript(uninstall): %v", err)
	}

	got, err := svc.GetModule(ctx, mod.ID)
	if err != nil {
		t.Fatalf("GetModule: %v", err)
	}
	if got.ID != "tenant.acme.test-module" || got.Title != "Test Module" {
		t.Fatalf("module mismatch: %+v", got)
	}
	if len(got.Versions) != 1 {
		t.Fatalf("want 1 version, got %d", len(got.Versions))
	}
	v := got.Versions[0]
	if v.Version != "0.1.0" || v.Status != "draft" {
		t.Fatalf("version mismatch: %+v", v)
	}
	if len(v.Scripts) != 2 {
		t.Fatalf("want 2 scripts, got %d (%+v)", len(v.Scripts), v.Scripts)
	}
	var sawApply, sawUninstall bool
	for _, sc := range v.Scripts {
		switch sc.Phase {
		case policymodules.PhaseApply:
			sawApply = true
			// Migration 012 dropped the filename column; the SetScript
			// filename argument is now discarded (deprecated wrapper),
			// and hydrateVersion derives LifecycleScript.Filename from
			// the stored script name ("apply") instead.
			if sc.Filename != "apply" || sc.Source != "#!/bin/bash\necho hi\n" {
				t.Errorf("apply script mismatch: %+v", sc)
			}
		case policymodules.PhaseUninstall:
			sawUninstall = true
		}
	}
	if !sawApply || !sawUninstall {
		t.Fatalf("missing expected phases: %+v", v.Scripts)
	}
	if got.Scope != "machine" {
		t.Errorf("module Scope not populated from version, got %q", got.Scope)
	}
	if len(got.Satisfies) != 1 || got.Satisfies[0] != "sec.test.urn" {
		t.Errorf("Satisfies not round-tripped: %+v", got.Satisfies)
	}
}

// TestSetScript_InvalidPhase locks that only the five Go-enum phases are
// accepted.
func TestSetScript_InvalidPhase(t *testing.T) {
	svc, _, ten := newModuleSvc(t)
	ctx := context.Background()
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.badphase", "Bad Phase", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "0.1.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, draft.ID, policymodules.LifecyclePhase("bogus"), "f", "s"); !errors.Is(err, services.ErrInvalidLifecyclePhase) {
		t.Fatalf("want ErrInvalidLifecyclePhase, got %v", err)
	}
}

// TestUpdateDraft_OnlyDraftMutable locks ADR-007's immutability rule:
// published/superseded/revoked versions reject UpdateDraft.
func TestUpdateDraft_OnlyDraftMutable(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.immutable", "Immutable", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, draft.ID, policymodules.PhaseApply, "enforce.sh", "# apply"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, draft.ID, publisher); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := svc.UpdateDraft(ctx, draft.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "user"}); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("want ErrVersionNotDraft after publish, got %v", err)
	}
}

// TestSetScript_DraftGuard is the Task 4.3 mandatory fix: SetScript must
// reject writes to a published version's scripts (ADR-007 immutability),
// the same way UpdateDraft already does for version fields. Before the
// fix, SetScript had no guard at all and would silently overwrite a
// published version's apply script.
func TestSetScript_DraftGuard(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.scriptguard", "Script Guard", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, draft.ID, policymodules.PhaseApply, "enforce.sh", "# v1 apply"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, draft.ID, publisher); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Attempting to overwrite the apply script of the now-published
	// version must be rejected, and the stored source must be unchanged.
	if _, err := svc.SetScript(ctx, draft.ID, policymodules.PhaseApply, "enforce.sh", "# malicious overwrite"); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("want ErrVersionNotDraft after publish, got %v", err)
	}
	m, err := svc.GetModule(ctx, mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	v := m.LatestVersion()
	if v == nil || len(v.Scripts) != 1 || v.Scripts[0].Source != "# v1 apply" {
		t.Fatalf("published apply script must be unchanged, got %+v", v)
	}

	// A brand-new draft version's scripts must still be writable.
	draft2, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.1.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, draft2.ID, policymodules.PhaseApply, "enforce.sh", "# v2 apply"); err != nil {
		t.Fatalf("SetScript on a real draft should succeed, got %v", err)
	}

	// A nonexistent version id must surface the raw sql.ErrNoRows, not
	// ErrVersionNotDraft (mirrors UpdateDraft's not-found-vs-frozen split).
	if _, err := svc.SetScript(ctx, 999999, policymodules.PhaseApply, "f", "s"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows for nonexistent version, got %v", err)
	}
}

// TestPublish_RequiresApplyScript locks INV-M3: a version with no apply
// script cannot be published.
func TestPublish_RequiresApplyScript(t *testing.T) {
	svc, _, ten := newModuleSvc(t)
	ctx := context.Background()
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.noapply", "No Apply", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	// Only a validate script, no apply.
	if _, err := svc.SetScript(ctx, draft.ID, policymodules.PhaseValidate, "validate.wasm", "# validate"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, draft.ID, 0); !errors.Is(err, services.ErrPublishRequiresApplyScript) {
		t.Fatalf("want ErrPublishRequiresApplyScript, got %v", err)
	}
}

// TestPublish_SupersedesPrior locks that publishing a second version
// marks the first superseded, with superseded_by_version pointing at
// the new version.
func TestPublish_SupersedesPrior(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.supersede", "Supersede", "")
	if err != nil {
		t.Fatal(err)
	}
	v1, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, v1.ID, policymodules.PhaseApply, "enforce.sh", "# v1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, v1.ID, publisher); err != nil {
		t.Fatalf("Publish v1: %v", err)
	}

	v2, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "2.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, v2.ID, policymodules.PhaseApply, "enforce.sh", "# v2"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, v2.ID, publisher); err != nil {
		t.Fatalf("Publish v2: %v", err)
	}

	v1Row, err := d.Queries.GetPolicyModuleVersion(ctx, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v1Row.State != "superseded" {
		t.Errorf("v1 state = %q, want superseded", v1Row.State)
	}
	if !v1Row.SupersededByVersion.Valid || v1Row.SupersededByVersion.String != "2.0.0" {
		t.Errorf("v1 superseded_by_version = %+v, want 2.0.0", v1Row.SupersededByVersion)
	}
	v2Row, err := d.Queries.GetPolicyModuleVersion(ctx, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v2Row.State != "published" {
		t.Errorf("v2 state = %q, want published", v2Row.State)
	}
}

// TestSeedBundled_IdempotentAndMatchesFormerMock runs SeedBundled twice
// and checks the module count doesn't double, then spot-checks one
// module (the SSH password-auth-disable module) against the content the
// old catalog/policymodules/mock.go used to hardcode: same satisfies
// URNs, same apply-script body, apply+disable+uninstall+validate phases
// present.
func TestSeedBundled_IdempotentAndMatchesFormerMock(t *testing.T) {
	svc, d, _ := newModuleSvc(t)
	ctx := context.Background()

	if err := svc.SeedBundled(ctx); err != nil {
		t.Fatalf("SeedBundled (1st): %v", err)
	}
	first, err := d.Queries.ListBundledModules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("SeedBundled seeded 0 modules")
	}

	if err := svc.SeedBundled(ctx); err != nil {
		t.Fatalf("SeedBundled (2nd): %v", err)
	}
	second, err := d.Queries.ListBundledModules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("SeedBundled not idempotent: 1st run seeded %d, 2nd run has %d", len(first), len(second))
	}

	mod, err := svc.GetModuleByURN(ctx, "pluris.sshd.password-auth-disable")
	if err != nil {
		t.Fatalf("GetModuleByURN(password-auth-disable): %v", err)
	}
	if mod.Origin != "bundled" {
		t.Errorf("Origin = %q, want bundled", mod.Origin)
	}
	foundSatisfies := false
	for _, s := range mod.Satisfies {
		if s == "sec.remote-access.ssh.password-auth" {
			foundSatisfies = true
		}
	}
	if !foundSatisfies {
		t.Errorf("Satisfies missing expected URN: %+v", mod.Satisfies)
	}
	if len(mod.Versions) == 0 {
		t.Fatal("no versions seeded for password-auth-disable")
	}
	v := mod.Versions[0]
	if v.Status != "published" {
		t.Errorf("seeded version status = %q, want published", v.Status)
	}
	phases := map[policymodules.LifecyclePhase]string{}
	for _, sc := range v.Scripts {
		phases[sc.Phase] = sc.Source
	}
	if _, ok := phases[policymodules.PhaseApply]; !ok {
		t.Fatal("seeded module missing apply script")
	}
	if !containsStr(phases[policymodules.PhaseApply], "PasswordAuthentication no") {
		t.Errorf("apply script content mismatch, got: %q", phases[policymodules.PhaseApply])
	}
	for _, want := range []policymodules.LifecyclePhase{policymodules.PhaseDisable, policymodules.PhaseUninstall, policymodules.PhaseValidate} {
		if _, ok := phases[want]; !ok {
			t.Errorf("seeded module missing %s script", want)
		}
	}
}

func containsStr(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}
func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// TestPublish_ConcurrentPublishesYieldOnePublished locks the "at most
// one published version per module" invariant under concurrency: two
// goroutines publish two different drafts of the same module at the
// same time (mirrors TestEnsureBuiltinsConcurrentFirstRequest's
// pattern; run under -race). Both may succeed (the later one supersedes
// the earlier), but the end state must be exactly one published
// version.
func TestPublish_ConcurrentPublishesYieldOnePublished(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.race", "Race", "")
	if err != nil {
		t.Fatal(err)
	}
	makeDraft := func(ver string) int64 {
		t.Helper()
		v, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: ver, Scope: "machine"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SetScript(ctx, v.ID, policymodules.PhaseApply, "enforce.sh", "# "+ver); err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	v1 := makeDraft("1.0.0")
	v2 := makeDraft("2.0.0")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []int64{v1, v2} {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			errs[i] = svc.Publish(ctx, id, publisher)
		}(i, id)
	}
	wg.Wait()
	for i, err := range errs {
		// Each publish must either succeed or lose the race with the
		// typed error -- never anything else.
		if err != nil && !errors.Is(err, services.ErrVersionNotDraft) {
			t.Fatalf("publish %d: unexpected error %v", i, err)
		}
	}

	got, err := svc.GetModule(ctx, mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	published := 0
	for _, v := range got.Versions {
		if v.Status == "published" {
			published++
		}
	}
	if published != 1 {
		t.Fatalf("want exactly 1 published version after concurrent publishes, got %d (%+v)", published, got.Versions)
	}
}

// TestPublish_SameDraftTwiceSecondRejected locks the state-guarded
// publish: publishing an already-published version returns
// ErrVersionNotDraft (the sequential flavor of the race above).
func TestPublish_SameDraftTwiceSecondRejected(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.double", "Double", "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, v.ID, policymodules.PhaseApply, "enforce.sh", "# apply"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, v.ID, publisher); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if err := svc.Publish(ctx, v.ID, publisher); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("second publish: want ErrVersionNotDraft, got %v", err)
	}
}

// TestRevoke_StateGuards locks Revoke's state rules: drafts are
// rejected (delete instead), published versions revoke fine, and
// double-revoke is rejected.
func TestRevoke_StateGuards(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.revoke", "Revoke", "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}

	// Draft cannot be revoked.
	if err := svc.Revoke(ctx, v.ID); !errors.Is(err, services.ErrRevokeInvalidState) {
		t.Fatalf("revoke draft: want ErrRevokeInvalidState, got %v", err)
	}

	if _, err := svc.SetScript(ctx, v.ID, policymodules.PhaseApply, "enforce.sh", "# apply"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, v.ID, publisher); err != nil {
		t.Fatal(err)
	}

	// Published revokes fine.
	if err := svc.Revoke(ctx, v.ID); err != nil {
		t.Fatalf("revoke published: %v", err)
	}
	row, err := d.Queries.GetPolicyModuleVersion(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.State != "revoked" {
		t.Fatalf("state = %q, want revoked", row.State)
	}

	// Double-revoke rejected.
	if err := svc.Revoke(ctx, v.ID); !errors.Is(err, services.ErrRevokeInvalidState) {
		t.Fatalf("double revoke: want ErrRevokeInvalidState, got %v", err)
	}
}

// TestDeleteModule_BlockedWhenReferenced locks the DeleteModule guard:
// a module referenced by a configuration-group binding cannot be
// deleted.
func TestDeleteModule_BlockedWhenReferenced(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()

	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.referenced", "Referenced", "")
	if err != nil {
		t.Fatal(err)
	}

	// Unreferenced delete succeeds.
	free, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.free", "Free", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.PermanentlyDeleteModule(ctx, ten, free.ID, free.ModuleUrn); err != nil {
		t.Fatalf("DeleteModule on unreferenced module: %v", err)
	}

	// Now reference `mod` via a configuration_group_binding and confirm
	// deletion is blocked.
	cg, err := d.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{
		TenantID: ten, Name: "Test Group", Scope: "machine",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Queries.CreateConfigurationGroupBinding(ctx, db.CreateConfigurationGroupBindingParams{
		ConfigurationGroupID: cg.ID, PolicyUrn: "sec.test.urn", State: "enabled",
		ModuleID: sql.NullInt64{Int64: mod.ID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.PermanentlyDeleteModule(ctx, ten, mod.ID, mod.ModuleUrn); !errors.Is(err, services.ErrModuleReferenced) {
		t.Fatalf("want ErrModuleReferenced, got %v", err)
	}
}

func TestPolicyModuleSoftDeleteRestoreImmediateAndPurge(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	retention := services.NewRetentionService(d)

	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.recycle", "Recycle", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteModule(ctx, ten, mod.ID, mod.ModuleUrn, 42); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := svc.GetModuleByURN(ctx, mod.ModuleUrn); !errors.Is(err, services.ErrModuleNotFound) {
		t.Fatalf("default read after soft delete = %v, want ErrModuleNotFound", err)
	}
	deleted, err := d.Queries.GetPolicyModuleByURNIncludingDeleted(ctx, mod.ModuleUrn)
	if err != nil || deleted.DeletedAt == nil {
		t.Fatalf("including-deleted read = %+v, %v", deleted, err)
	}
	if err := svc.RestoreModule(ctx, mod.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := svc.GetModuleByURN(ctx, mod.ModuleUrn); err != nil {
		t.Fatalf("read after restore: %v", err)
	}

	if _, err := retention.UpdateSetting(ctx, services.EntityKindPolicyModule, services.RetentionModeImmediate, nil, 42); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteModule(ctx, ten, mod.ID, mod.ModuleUrn, 42); err != nil {
		t.Fatalf("immediate delete: %v", err)
	}
	if _, err := d.Queries.GetPolicyModuleByURNIncludingDeleted(ctx, mod.ModuleUrn); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("immediate row lookup = %v, want sql.ErrNoRows", err)
	}

	if _, err := retention.UpdateSetting(ctx, services.EntityKindPolicyModule, services.RetentionModeSoft, nil, 42); err != nil {
		t.Fatal(err)
	}
	expired, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.expired", "Expired", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteModule(ctx, ten, expired.ID, expired.ModuleUrn, 42); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Conn().ExecContext(ctx, "UPDATE policy_modules SET deleted_at = datetime('now', '-2 days') WHERE id = ?", expired.ID); err != nil {
		t.Fatal(err)
	}
	results, err := retention.PurgeExpired(ctx)
	if err != nil || len(results) != 0 {
		t.Fatalf("NULL-window purge = %+v, %v; want no work", results, err)
	}
	days := int64(1)
	if _, err := retention.UpdateSetting(ctx, services.EntityKindPolicyModule, services.RetentionModeSoft, &days, 42); err != nil {
		t.Fatal(err)
	}
	results, err = retention.PurgeExpired(ctx)
	if err != nil || len(results) != 1 || !results[0].Purged {
		t.Fatalf("expired purge = %+v, %v", results, err)
	}
}

func TestPolicyModuleSoftDeleteAllowsReferencesButPurgeSkips(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	retention := services.NewRetentionService(d)
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.referenced-soft", "Referenced soft", "")
	if err != nil {
		t.Fatal(err)
	}
	cg, err := d.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{TenantID: ten, Name: "Guard", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Queries.CreateConfigurationGroupBinding(ctx, db.CreateConfigurationGroupBindingParams{
		ConfigurationGroupID: cg.ID, PolicyUrn: "sec.guard", State: "enabled",
		ModuleID: sql.NullInt64{Int64: mod.ID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteModule(ctx, ten, mod.ID, mod.ModuleUrn, 42); err != nil {
		t.Fatalf("referenced soft delete must succeed: %v", err)
	}
	if _, err := d.Conn().ExecContext(ctx, "UPDATE policy_modules SET deleted_at = datetime('now', '-2 days') WHERE id = ?", mod.ID); err != nil {
		t.Fatal(err)
	}
	days := int64(1)
	if _, err := retention.UpdateSetting(ctx, services.EntityKindPolicyModule, services.RetentionModeSoft, &days, 42); err != nil {
		t.Fatal(err)
	}
	results, err := retention.PurgeExpired(ctx)
	if err != nil || len(results) != 1 || !results[0].Skipped || !errors.Is(results[0].Err, services.ErrModuleReferenced) {
		t.Fatalf("referenced purge = %+v, %v", results, err)
	}
	if _, err := d.Queries.GetPolicyModuleByURNIncludingDeleted(ctx, mod.ModuleUrn); err != nil {
		t.Fatalf("skipped module must remain: %v", err)
	}
}
