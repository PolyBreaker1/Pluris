package policymodules

// Live module catalog access. Task 4.2 (backend slice) moved Policy
// Module persistence from a hardcoded in-memory slice to real DB
// tables (policy_modules / policy_module_versions / policy_module_scripts
// -- see db/schema/008_module_scripts.sql and
// pkg/services/policymodules.go). This package stays pure (no DB
// import, no import of pkg/services -- that would be a cycle, since the
// service imports this package for the domain types): it exposes a
// provider hook that the service installs once at startup, and the
// handful of helpers below that used to iterate a hardcoded literal now
// iterate whatever the provider currently returns.
//
// catalogProvider is nil until installed. Call sites that need the
// live catalog go through Catalog(); resolver.go's Resolve and
// defaults.go's ResolveBindingModule already take a []Module parameter
// directly (pure functions, unaffected by this file) -- callers fetch
// from the service and pass the slice in.
var catalogProvider func() []Module

// SetCatalogProvider installs the function the live module catalog is
// read through. Called once at server startup (see cmd/console) with
// something like policyModuleSvc.ListModules bound to a tenant. Tests
// that don't call this see Catalog() return nil, which every helper
// below treats as "no modules" rather than panicking.
func SetCatalogProvider(fn func() []Module) { catalogProvider = fn }

// Catalog returns the current module catalog via the installed
// provider, or nil if none has been installed yet (e.g. package tests
// that construct their own fixtures instead of relying on the provider).
func Catalog() []Module {
	if catalogProvider == nil {
		return nil
	}
	return catalogProvider()
}

// CandidatesForPolicy — modules in the live catalog that satisfy the
// given catalog policy URN AND support at least one of the device's
// OSes. Used by the Configuration Group binding's module picker.
//
// `deviceOS` may be empty when the binding's target is a user/group
// without a specific device — in that case all OS-compatible modules
// are returned and the agent narrows at exec time.
func CandidatesForPolicy(urn string, deviceOS TargetOS) []Module {
	var out []Module
	for _, m := range Catalog() {
		if !m.SatisfiesURN(urn) {
			continue
		}
		if deviceOS != "" && !m.SupportsOS(deviceOS) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ----------------------------------------------------------------------
// Mock installations — used by Asset detail "Installed Modules" tab.
// ----------------------------------------------------------------------

// MockInstallations — three sample assets with overlapping module
// installations so the UI shows refcount > 1 (load-bearing modules) and
// dep-driven installations (e.g. base-config installed via dependency).
func MockInstallations() []ModuleInstallation {
	return []ModuleInstallation{
		// alice-laptop: SSH disable + its base-config dep.
		{
			ID: "install:1", AssetID: "asset:c2f1-alice-laptop",
			ModuleID: "pluris.sshd.password-auth-disable", ModuleVersion: "1.2.0",
			Reasons: []InstallReason{
				{BindingID: "binding:cg.baseline.workstations:0", AddedAt: "2026-04-25T09:00:00Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-04-25T09:00:34Z", LastValidated: "2026-05-06T08:00:00Z",
		},
		{
			ID: "install:2", AssetID: "asset:c2f1-alice-laptop",
			ModuleID: "pluris.sshd.base-config", ModuleVersion: "1.0.3",
			Reasons: []InstallReason{
				{DependentInstall: "install:1", AddedAt: "2026-04-25T09:00:00Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-04-25T09:00:31Z",
		},
		// build-server: SAME base-config (load-bearing — refcount=2 across
		// installations is misleading; the per-asset refcount on this row
		// is 2 because two reasons hold it: a direct binding AND a
		// dependent.
		{
			ID: "install:3", AssetID: "asset:aa01-build-server",
			ModuleID: "pluris.sshd.base-config", ModuleVersion: "1.0.3",
			Reasons: []InstallReason{
				{BindingID: "binding:cg.servers.ssh-baseline:0", AddedAt: "2026-04-20T07:30:00Z"},
				{DependentInstall: "install:4", AddedAt: "2026-04-20T07:30:02Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-04-20T07:30:10Z",
		},
		{
			ID: "install:4", AssetID: "asset:aa01-build-server",
			ModuleID: "pluris.sshd.password-auth-disable", ModuleVersion: "1.2.0",
			Reasons: []InstallReason{
				{BindingID: "binding:cg.servers.ssh-baseline:1", AddedAt: "2026-04-20T07:30:02Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-04-20T07:30:14Z",
		},
		// dev-workstation-12: corp banner (custom module).
		{
			ID: "install:5", AssetID: "asset:dd44-dev-ws-12",
			ModuleID: "tenant.acme.security-banner", ModuleVersion: "0.4.0",
			Reasons: []InstallReason{
				{BindingID: "binding:cg.acme.workstation-banners:0", AddedAt: "2026-05-02T12:11:00Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-05-02T12:11:18Z",
		},
	}
}

// InstallationsForAsset — slice of installations on a given asset, in
// stable order. Used by the Asset detail "Installed Modules" tab.
func InstallationsForAsset(assetID string) []ModuleInstallation {
	var out []ModuleInstallation
	for _, mi := range MockInstallations() {
		if mi.AssetID == assetID {
			out = append(out, mi)
		}
	}
	return out
}
