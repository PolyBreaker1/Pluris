package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/pkg/services"
)

// TestAddConditionParamValidation covers the "param" kind validation
// matrix: bad operator and empty path are rejected; a valid one succeeds
// and round-trips through Get.
func TestAddConditionParamValidation(t *testing.T) {
	svc, _, ten := newDGSvc(t)
	ctx := context.Background()
	g, err := svc.Create(ctx, ten, "Custom", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.AddCondition(ctx, g.ID, "computer/hardware/os_family", "bogus_operator", []string{"linux"}, "param", "", ""); !errors.Is(err, services.ErrInvalidOperator) {
		t.Fatalf("bad operator: want ErrInvalidOperator, got %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "in", []string{"linux"}, "param", "", ""); !errors.Is(err, services.ErrParamPathRequired) {
		t.Fatalf("empty path: want ErrParamPathRequired, got %v", err)
	}
	// Widened operator set: a new string operator (equals) must now be
	// accepted, and a new numeric one (gt) too.
	if err := svc.AddCondition(ctx, g.ID, "computer/hardware/os_family", "equals", []string{"linux"}, "param", "", ""); err != nil {
		t.Fatalf("equals should be accepted post-widening: %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "computer/hardware/cpu_count", "gt", []string{"2"}, "param", "", ""); err != nil {
		t.Fatalf("gt should be accepted post-widening: %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "computer/hardware/hostname", "matches", []string{"^web-"}, "param", "", ""); err != nil {
		t.Fatalf("matches should be accepted post-widening: %v", err)
	}

	got, err := svc.Get(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Conditions) != 3 {
		t.Fatalf("want 3 persisted conditions (only the valid adds), got %d", len(got.Conditions))
	}
}

// TestAddConditionKindValidation covers kind whitelisting and defaulting.
func TestAddConditionKindValidation(t *testing.T) {
	svc, _, ten := newDGSvc(t)
	ctx := context.Background()
	g, err := svc.Create(ctx, ten, "Custom", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.AddCondition(ctx, g.ID, "computer/hardware/os_family", "in", []string{"linux"}, "bogus_kind", "", ""); !errors.Is(err, services.ErrInvalidConditionKind) {
		t.Fatalf("bad kind: want ErrInvalidConditionKind, got %v", err)
	}
	// Empty kind defaults to "param".
	if err := svc.AddCondition(ctx, g.ID, "computer/hardware/os_family", "in", []string{"linux"}, "", "", ""); err != nil {
		t.Fatalf("empty kind should default to param and succeed: %v", err)
	}
	got, _ := svc.Get(ctx, g.ID)
	if len(got.Conditions) != 1 || got.Conditions[0].Kind != "param" {
		t.Fatalf("want 1 condition defaulted to kind=param, got %+v", got.Conditions)
	}
}

// TestAddConditionScriptValidation covers the script and command kind
// validation matrix under the standardized subject/operator/value shape:
// missing subject rejected, source+ref ambiguity rejected, operator
// required for every kind, valid inputs persist with script_ref intact.
func TestAddConditionScriptValidation(t *testing.T) {
	svc, _, ten := newDGSvc(t)
	ctx := context.Background()
	g, err := svc.Create(ctx, ten, "Custom", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.AddCondition(ctx, g.ID, "", "exists", nil, "script", "", ""); !errors.Is(err, services.ErrScriptSourceRequired) {
		t.Fatalf("empty script source+ref: want ErrScriptSourceRequired, got %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "exists", nil, "script", "exit 0", "lib-1"); !errors.Is(err, services.ErrScriptSourceAmbiguous) {
		t.Fatalf("both source and ref: want ErrScriptSourceAmbiguous, got %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "bogus_op", nil, "script", "exit 0", ""); !errors.Is(err, services.ErrInvalidOperator) {
		t.Fatalf("bad operator on script kind: want ErrInvalidOperator, got %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "contains", []string{"3"}, "command", "", ""); !errors.Is(err, services.ErrScriptSourceRequired) {
		t.Fatalf("empty command: want ErrScriptSourceRequired, got %v", err)
	}

	if err := svc.AddCondition(ctx, g.ID, "", "exists", nil, "script", "exit 0", ""); err != nil {
		t.Fatalf("valid inline script condition: %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "contains", []string{"example"}, "script", "", "custom-sh"); err != nil {
		t.Fatalf("valid script-ref condition: %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "contains", []string{"3"}, "command", "uname -r", ""); err != nil {
		t.Fatalf("valid command condition: %v", err)
	}

	got, err := svc.Get(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Conditions) != 3 {
		t.Fatalf("want 3 persisted conditions, got %d", len(got.Conditions))
	}
	if got.Conditions[1].ScriptRef != "custom-sh" {
		t.Fatalf("script_ref round-trip: got %q", got.Conditions[1].ScriptRef)
	}
	if got.Conditions[2].Kind != dependencygroups.KindCommand || got.Conditions[2].ScriptSource != "uname -r" {
		t.Fatalf("command round-trip: %+v", got.Conditions[2])
	}
}

// TestSetMatchMode covers match_mode validation and builtin protection.
func TestSetMatchMode(t *testing.T) {
	svc, _, ten := newDGSvc(t)
	ctx := context.Background()

	if err := svc.EnsureBuiltins(ctx, ten); err != nil {
		t.Fatal(err)
	}
	builtins, err := svc.ListByTenant(ctx, ten)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetMatchMode(ctx, builtins[0].ID, "any"); !errors.Is(err, services.ErrBuiltinMatchModeProtected) {
		t.Fatalf("builtin match mode change: want ErrBuiltinMatchModeProtected, got %v", err)
	}

	g, err := svc.Create(ctx, ten, "Custom", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetMatchMode(ctx, g.ID, "bogus"); !errors.Is(err, services.ErrInvalidMatchMode) {
		t.Fatalf("bad mode: want ErrInvalidMatchMode, got %v", err)
	}
	if err := svc.SetMatchMode(ctx, g.ID, "any"); err != nil {
		t.Fatalf("valid mode change failed: %v", err)
	}
	got, err := svc.Get(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MatchMode != "any" {
		t.Fatalf("want match_mode=any after SetMatchMode, got %q", got.MatchMode)
	}
}
