package services_test

import (
	"context"
	"errors"
	"testing"

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

// TestAddConditionScriptValidation covers the "script" kind validation
// matrix: empty source rejected, bad script_expect JSON rejected, extra
// keys rejected, empty script_expect defaults, valid input persists.
func TestAddConditionScriptValidation(t *testing.T) {
	svc, _, ten := newDGSvc(t)
	ctx := context.Background()
	g, err := svc.Create(ctx, ten, "Custom", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.AddCondition(ctx, g.ID, "", "", nil, "script", "", ""); !errors.Is(err, services.ErrScriptSourceRequired) {
		t.Fatalf("empty script source: want ErrScriptSourceRequired, got %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "", nil, "script", "exit 0", "not json"); !errors.Is(err, services.ErrInvalidScriptExpect) {
		t.Fatalf("garbage JSON: want ErrInvalidScriptExpect, got %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "", nil, "script", "exit 0", `{"unexpected_key":1}`); !errors.Is(err, services.ErrInvalidScriptExpect) {
		t.Fatalf("disallowed key: want ErrInvalidScriptExpect, got %v", err)
	}
	if err := svc.AddCondition(ctx, g.ID, "", "", nil, "script", "exit 0", `{"exit_code":"not-a-number"}`); !errors.Is(err, services.ErrInvalidScriptExpect) {
		t.Fatalf("exit_code wrong type: want ErrInvalidScriptExpect, got %v", err)
	}
	// A quoted numeric string is still a string, not a JSON number —
	// json.Number decoding tolerates `"0"`, so the stricter token check
	// must reject it explicitly.
	if err := svc.AddCondition(ctx, g.ID, "", "", nil, "script", "exit 0", `{"exit_code":"0"}`); !errors.Is(err, services.ErrInvalidScriptExpect) {
		t.Fatalf("string-typed exit_code: want ErrInvalidScriptExpect, got %v", err)
	}

	// Empty script_expect defaults to {"exit_code":0}.
	if err := svc.AddCondition(ctx, g.ID, "", "", nil, "script", "exit 0", ""); err != nil {
		t.Fatalf("valid script condition with default expect: %v", err)
	}
	// Explicit valid expect with both allowed keys.
	if err := svc.AddCondition(ctx, g.ID, "", "", nil, "script", "grep -q ok /tmp/x", `{"exit_code":0,"output_equals":"ok"}`); err != nil {
		t.Fatalf("valid script condition with explicit expect: %v", err)
	}

	got, err := svc.Get(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Conditions) != 2 {
		t.Fatalf("want 2 persisted script conditions, got %d", len(got.Conditions))
	}
	if got.Conditions[0].ScriptExpect != `{"exit_code":0}` {
		t.Fatalf("want default script_expect, got %q", got.Conditions[0].ScriptExpect)
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
