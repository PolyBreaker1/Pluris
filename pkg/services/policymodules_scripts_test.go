package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/pkg/services"
)

// TestReferencedParams locks the pure param-token parser that feeds the
// security allow-list (spec section 6/8/9): distinct params in
// first-seen order, duplicates deduped, empty/malformed source yields
// nothing.
func TestReferencedParams(t *testing.T) {
	svc, _, _ := newModuleSvc(t)

	src := `echo {{ param "computer/hardware/ram_mb" }}
echo {{ param "user/identity/email" }}
echo {{ param "computer/hardware/ram_mb" }}
`
	got := svc.ReferencedParams(src)
	want := []string{"computer/hardware/ram_mb", "user/identity/email"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	if got := svc.ReferencedParams(""); len(got) != 0 {
		t.Fatalf("empty source: got %v, want none", got)
	}

	// Malformed tokens (missing quotes, wrong keyword) must be ignored.
	malformed := svc.ReferencedParams(`{{ param computer/hardware/ram_mb }} {{ paramx "a/b/c" }} {{param "a/b/c"}}`)
	if len(malformed) != 1 || malformed[0] != "a/b/c" {
		t.Fatalf("malformed handling: got %v", malformed)
	}
}

func setupDraftWithDefaults(t *testing.T, svc *services.PolicyModuleService, ten int64, urn string) int64 {
	t.Helper()
	ctx := context.Background()
	mod, err := svc.CreateModule(ctx, &ten, nil, urn, "Fixture", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SeedModuleDefaults(ctx, draft.ID); err != nil {
		t.Fatalf("SeedModuleDefaults: %v", err)
	}
	return draft.ID
}

// TestUpsertScript_ForksDefault is the fork-on-edit-of-default
// contract (spec section 7): editing a default-origin script must
// leave the pristine default row intact AND produce a custom-origin
// row carrying the edited source, coexisting under UNIQUE(version_id,
// name) via the reserved-name mechanism documented in
// pkg/services/policymodules_scripts.go.
func TestUpsertScript_ForksDefault(t *testing.T) {
	svc, _, ten := newModuleSvc(t)
	ctx := context.Background()
	versionID := setupDraftWithDefaults(t, svc, ten, "tenant.acme.fork-default")

	before, err := svc.ListScripts(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0].Name != "apply" || before[0].Origin != "default" {
		t.Fatalf("seed precondition: %+v", before)
	}
	pristineSource := before[0].Source

	edited, err := svc.UpsertScript(ctx, versionID, policymodules.Script{
		Name: "apply", Language: "sh", Source: "#!/bin/sh\necho edited\n",
	})
	if err != nil {
		t.Fatalf("UpsertScript: %v", err)
	}
	if edited.Origin != "custom" || edited.Source != "#!/bin/sh\necho edited\n" {
		t.Fatalf("edited script wrong: %+v", edited)
	}

	after, err := svc.ListScripts(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	var sawDefault, sawCustom bool
	for _, sc := range after {
		switch sc.Origin {
		case "default":
			sawDefault = true
			if sc.Source != pristineSource {
				t.Errorf("pristine default source changed: %+v", sc)
			}
			if sc.Name == "apply" {
				t.Errorf("default row must not keep the plain name after fork: %+v", sc)
			}
		case "custom":
			sawCustom = true
			if sc.Name != "apply" || sc.Source != "#!/bin/sh\necho edited\n" {
				t.Errorf("custom row wrong: %+v", sc)
			}
		}
	}
	if !sawDefault || !sawCustom {
		t.Fatalf("want both a default-origin and custom-origin row, got %+v", after)
	}
}

// TestRestoreDefaults_KeepsCustoms locks that RestoreDefaults re-points
// enforcement actions at the default wiring without deleting the
// edited custom script.
func TestRestoreDefaults_KeepsCustoms(t *testing.T) {
	svc, _, ten := newModuleSvc(t)
	ctx := context.Background()
	versionID := setupDraftWithDefaults(t, svc, ten, "tenant.acme.restore-defaults")

	if _, err := svc.UpsertScript(ctx, versionID, policymodules.Script{
		Name: "apply", Language: "sh", Source: "# edited",
	}); err != nil {
		t.Fatalf("UpsertScript: %v", err)
	}

	if err := svc.RestoreDefaults(ctx, versionID); err != nil {
		t.Fatalf("RestoreDefaults: %v", err)
	}

	scripts, err := svc.ListScripts(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	var customStillPresent bool
	for _, sc := range scripts {
		if sc.Name == "apply" && sc.Origin == "custom" && sc.Source == "# edited" {
			customStillPresent = true
		}
	}
	if !customStillPresent {
		t.Fatalf("edited custom script must survive RestoreDefaults, got %+v", scripts)
	}

	actions, err := svc.ListActions(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	var applyAction *policymodules.ModuleAction
	for i := range actions {
		if actions[i].Key == "apply" {
			applyAction = &actions[i]
		}
	}
	if applyAction == nil {
		t.Fatalf("apply action missing: %+v", actions)
	}
	if applyAction.Origin != "default" || applyAction.Value == "apply" {
		t.Fatalf("apply action must be re-pointed at the default wiring (not the custom script), got %+v", applyAction)
	}
}

// TestFullReset_DeletesCustoms locks that FullReset removes every
// custom script/action and the default set survives (or is reseeded).
func TestFullReset_DeletesCustoms(t *testing.T) {
	svc, _, ten := newModuleSvc(t)
	ctx := context.Background()
	versionID := setupDraftWithDefaults(t, svc, ten, "tenant.acme.full-reset")

	if _, err := svc.UpsertScript(ctx, versionID, policymodules.Script{
		Name: "apply", Language: "sh", Source: "# edited",
	}); err != nil {
		t.Fatalf("UpsertScript: %v", err)
	}
	if _, err := svc.UpsertAction(ctx, versionID, policymodules.ModuleAction{
		Key: "custom:healthcheck", Kind: "command", Value: "true",
	}); err != nil {
		t.Fatalf("UpsertAction: %v", err)
	}

	if err := svc.FullReset(ctx, versionID); err != nil {
		t.Fatalf("FullReset: %v", err)
	}

	scripts, err := svc.ListScripts(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range scripts {
		if sc.Origin == "custom" {
			t.Fatalf("custom script must be gone after FullReset, got %+v", scripts)
		}
	}
	var sawApplyDefault bool
	for _, sc := range scripts {
		if sc.Name == "apply" && sc.Origin == "default" {
			sawApplyDefault = true
		}
	}
	if !sawApplyDefault {
		t.Fatalf("default apply script must remain/be reseeded after FullReset, got %+v", scripts)
	}

	actions, err := svc.ListActions(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions {
		if a.Origin == "custom" {
			t.Fatalf("custom action must be gone after FullReset, got %+v", actions)
		}
		if a.Key == "custom:healthcheck" {
			t.Fatalf("custom:healthcheck action must be gone after FullReset, got %+v", actions)
		}
	}
}

// TestUpsertScript_NonDraftRejected locks that UpsertScript is
// draft-guarded: once a version is published, its scripts are immutable
// (ADR-007).
func TestUpsertScript_NonDraftRejected(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)

	versionID := setupDraftWithDefaults(t, svc, ten, "tenant.acme.upsert-nondraft")
	if err := svc.Publish(ctx, versionID, publisher); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if _, err := svc.UpsertScript(ctx, versionID, policymodules.Script{
		Name: "apply", Language: "sh", Source: "# malicious",
	}); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("want ErrVersionNotDraft, got %v", err)
	}
}

// TestFullReset_NonDraftRejected locks that FullReset is draft-guarded
// at the very top: calling it on a published (immutable, ADR-007)
// version must return ErrVersionNotDraft before any delete runs, so
// the frozen version's scripts/actions are left completely untouched.
func TestFullReset_NonDraftRejected(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)

	versionID := setupDraftWithDefaults(t, svc, ten, "tenant.acme.fullreset-nondraft")
	if _, err := svc.UpsertScript(ctx, versionID, policymodules.Script{
		Name: "apply", Language: "sh", Source: "# edited",
	}); err != nil {
		t.Fatalf("UpsertScript: %v", err)
	}
	before, err := svc.ListScripts(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	beforeActions, err := svc.ListActions(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Publish(ctx, versionID, publisher); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if err := svc.FullReset(ctx, versionID); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("want ErrVersionNotDraft, got %v", err)
	}

	after, err := svc.ListScripts(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("scripts must be unchanged after rejected FullReset: before=%+v after=%+v", before, after)
	}
	afterActions, err := svc.ListActions(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterActions) != len(beforeActions) {
		t.Fatalf("actions must be unchanged after rejected FullReset: before=%+v after=%+v", beforeActions, afterActions)
	}
}

// TestRestoreDefaults_NonDraftRejected locks that RestoreDefaults is
// draft-guarded: once a version is published, RestoreDefaults must
// reject it and leave the version's scripts/actions untouched.
func TestRestoreDefaults_NonDraftRejected(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	publisher := newTestIdentity(t, d, ten)

	versionID := setupDraftWithDefaults(t, svc, ten, "tenant.acme.restoredefaults-nondraft")
	if _, err := svc.UpsertScript(ctx, versionID, policymodules.Script{
		Name: "apply", Language: "sh", Source: "# edited",
	}); err != nil {
		t.Fatalf("UpsertScript: %v", err)
	}
	before, err := svc.ListScripts(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	beforeActions, err := svc.ListActions(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Publish(ctx, versionID, publisher); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if err := svc.RestoreDefaults(ctx, versionID); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("want ErrVersionNotDraft, got %v", err)
	}

	after, err := svc.ListScripts(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("scripts must be unchanged after rejected RestoreDefaults: before=%+v after=%+v", before, after)
	}
	afterActions, err := svc.ListActions(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterActions) != len(beforeActions) {
		t.Fatalf("actions must be unchanged after rejected RestoreDefaults: before=%+v after=%+v", beforeActions, afterActions)
	}
}

// TestActionCRUDRoundTrip exercises the actions table's guarded
// upsert/list/delete, including a custom (non-lifecycle) action key.
func TestActionCRUDRoundTrip(t *testing.T) {
	svc, _, ten := newModuleSvc(t)
	ctx := context.Background()
	versionID := setupDraftWithDefaults(t, svc, ten, "tenant.acme.action-crud")

	created, err := svc.UpsertAction(ctx, versionID, policymodules.ModuleAction{
		Key: "custom:healthcheck", Label: "Health check", Kind: "command", Value: "curl -f localhost/health",
	})
	if err != nil {
		t.Fatalf("UpsertAction: %v", err)
	}
	if created.Origin != "custom" || created.Kind != "command" {
		t.Fatalf("created action wrong: %+v", created)
	}

	actions, err := svc.ListActions(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range actions {
		if a.Key == "custom:healthcheck" {
			found = true
			if a.Value != "curl -f localhost/health" || a.Label != "Health check" {
				t.Errorf("action mismatch: %+v", a)
			}
		}
	}
	if !found {
		t.Fatalf("custom:healthcheck not found in %+v", actions)
	}

	if err := svc.DeleteAction(ctx, versionID, "custom:healthcheck"); err != nil {
		t.Fatalf("DeleteAction: %v", err)
	}
	afterDelete, err := svc.ListActions(ctx, versionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range afterDelete {
		if a.Key == "custom:healthcheck" {
			t.Fatalf("custom:healthcheck should be deleted, got %+v", afterDelete)
		}
	}
}
