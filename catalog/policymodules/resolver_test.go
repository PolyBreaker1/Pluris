package policymodules

import "testing"

// Three regression tests, one per failure mode the resolver guards.
// These lock INV-M2 (no cycles), INV-M3 (missing deps rejected), and
// the conflict pass. The "happy path" test exercises topological order.

func TestResolve_HappyPath(t *testing.T) {
	plan, err := Resolve([]string{"pluris.sshd.password-auth-disable"}, AllModules())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Order) != 2 {
		t.Fatalf("want 2 modules in plan, got %d (%v)", len(plan.Order), plan.Order)
	}
	if plan.Order[0] != "pluris.sshd.base-config" {
		t.Fatalf("dep must come first; order=%v", plan.Order)
	}
	if len(plan.AddedTransitively) != 1 || plan.AddedTransitively[0] != "pluris.sshd.base-config" {
		t.Fatalf("base-config should be transitive; got %v", plan.AddedTransitively)
	}
}

func TestResolve_MissingDep(t *testing.T) {
	// Build a tiny catalog where one module depends on a non-existent ID.
	cat := []Module{
		{
			ID: "x.fake", Origin: "tenant",
			Versions: []ModuleVersion{
				{Version: "1.0.0", Status: "published",
					Dependencies: []Dependency{{ModuleID: "x.does-not-exist"}}},
			},
		},
	}
	_, err := Resolve([]string{"x.fake"}, cat)
	if err == nil || err.Code != "missing-dep" {
		t.Fatalf("want missing-dep error, got %+v", err)
	}
}

func TestResolve_Cycle(t *testing.T) {
	cat := []Module{
		{ID: "a", Versions: []ModuleVersion{{Version: "1", Status: "published",
			Dependencies: []Dependency{{ModuleID: "b"}}}}},
		{ID: "b", Versions: []ModuleVersion{{Version: "1", Status: "published",
			Dependencies: []Dependency{{ModuleID: "a"}}}}},
	}
	_, err := Resolve([]string{"a"}, cat)
	if err == nil || err.Code != "cycle" {
		t.Fatalf("want cycle error, got %+v", err)
	}
}

func TestResolve_Conflict(t *testing.T) {
	// password-auth-disable conflicts with password-auth-allow.
	// Pulling both should fail.
	_, err := Resolve(
		[]string{"pluris.sshd.password-auth-disable", "pluris.sshd.password-auth-allow"},
		AllModules(),
	)
	if err == nil || err.Code != "conflict" {
		t.Fatalf("want conflict error, got %+v", err)
	}
}

// Refcount lock — INV-M1 prerequisite. Just exercises the helper.
func TestModuleInstallation_Refcount(t *testing.T) {
	mi := &ModuleInstallation{
		Reasons: []InstallReason{{BindingID: "b1"}, {DependentInstall: "i2"}},
	}
	if got := mi.Refcount(); got != 2 {
		t.Fatalf("refcount want 2, got %d", got)
	}
	if !mi.IsLoadBearing() {
		t.Fatalf("refcount=2 should be load-bearing")
	}
	mi.Reasons = mi.Reasons[:1]
	if mi.IsLoadBearing() {
		t.Fatalf("refcount=1 should not be load-bearing")
	}
}
