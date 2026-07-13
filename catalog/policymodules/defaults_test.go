package policymodules

import "testing"

// Test_ResolveBindingModule_PlurisDefault — no override, no tenant
// default: picks the first bundled module that satisfies + supports OS.
func Test_ResolveBindingModule_PlurisDefault(t *testing.T) {
	urn := "Computer/WindowsComponents/RemoteAccess/SSH/PasswordAuthDisable"
	got := ResolveBindingModule("", "", "no-such-tenant", urn, OSLinux, testCatalog())
	if !got.IsResolved() {
		t.Fatalf("expected a Pluris default, got %+v", got)
	}
	if got.Source != "pluris" {
		t.Errorf("want Source=pluris, got %q", got.Source)
	}
	if got.ModuleID != "pluris.sshd.password-auth-disable" {
		t.Errorf("want bundled SSH module, got %q", got.ModuleID)
	}
}

// Test_ResolveBindingModule_TenantDefault — seed sets a tenant default
// for the mock tenant ""; resolver picks it ahead of the bundled one
// (in this test they're the same module but Source must say "tenant").
func Test_ResolveBindingModule_TenantDefault(t *testing.T) {
	urn := "sec.remote-access.ssh.password-auth" // seeded in init()
	got := ResolveBindingModule("", "", "", urn, OSLinux, testCatalog())
	if got.Source != "tenant" {
		t.Errorf("want Source=tenant (seeded default), got %q (reason=%q)", got.Source, got.Reason)
	}
}

// Test_ResolveBindingModule_BindingOverride — binding pin wins over
// the seeded tenant default.
func Test_ResolveBindingModule_BindingOverride(t *testing.T) {
	urn := "sec.remote-access.ssh.password-auth"
	got := ResolveBindingModule("pluris.sshd.password-auth-disable", "1.1.0", "", urn, OSLinux, testCatalog())
	if got.Source != "binding" {
		t.Errorf("want Source=binding, got %q", got.Source)
	}
	if got.ModuleVersion != "1.1.0" {
		t.Errorf("want pinned 1.1.0, got %q", got.ModuleVersion)
	}
}

// Test_ResolveBindingModule_BindingOverrideStaleFallsThrough — a pin
// to a module that no longer satisfies the URN must fall through, not
// fail. UI surfaces the staleness separately.
func Test_ResolveBindingModule_BindingOverrideStaleFallsThrough(t *testing.T) {
	urn := "sec.remote-access.ssh.password-auth"
	// Bind to a module that exists but doesn't satisfy this URN.
	got := ResolveBindingModule("pluris.sshd.base-config", "", "", urn, OSLinux, testCatalog())
	if got.Source == "binding" {
		t.Errorf("stale binding override should not resolve as 'binding', got %+v", got)
	}
	if !got.IsResolved() {
		t.Errorf("should fall through to a valid source, got %+v", got)
	}
}

// Test_ResolveBindingModule_OSMismatch — no module supports the
// device's OS: returns unresolved.
func Test_ResolveBindingModule_OSMismatch(t *testing.T) {
	urn := "sec.remote-access.ssh.password-auth"
	got := ResolveBindingModule("", "", "", urn, OSWindows, testCatalog())
	if got.IsResolved() {
		t.Errorf("Windows-OS SSH binding should be unresolved, got %+v", got)
	}
}

// Test_SetAndClearTenantDefault — round-trips through the store.
func Test_SetAndClearTenantDefault(t *testing.T) {
	const urn = "test.urn.only-for-this-test"
	defer ClearTenantDefault("acme", urn) // cleanup if test fails mid-way

	if got := TenantDefaultFor("acme", urn); got.ModuleID != "" {
		t.Fatalf("precondition failed: %+v", got)
	}
	SetTenantDefault(TenantDefault{TenantID: "acme", PolicyURN: urn, ModuleID: "m1", ModuleVersion: "2.0.0"})
	got := TenantDefaultFor("acme", urn)
	if got.ModuleID != "m1" || got.ModuleVersion != "2.0.0" {
		t.Errorf("after Set, got %+v", got)
	}
	ClearTenantDefault("acme", urn)
	if got := TenantDefaultFor("acme", urn); got.ModuleID != "" {
		t.Errorf("after Clear, expected zero default, got %+v", got)
	}
}
