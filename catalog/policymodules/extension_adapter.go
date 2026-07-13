package policymodules

import (
	"github.com/pluris/pluris/pkg/extension"
)

// This file wires the policymodules package into the cross-kind
// extension framework introduced in ADR-008 / Slice A. It is purely
// additive — none of the existing types or functions in this package
// changed. The adapter exists so the Sources page, the audit log, the
// future community-registry surface, and any other "list every
// extension" site can read modules through pkg/extension without
// importing this package directly.
//
// Layering: this file imports pkg/extension; pkg/extension imports
// nothing from pluris/. That direction is the rule (see the package
// doc on extension/types.go).

// ----------------------------------------------------------------------
// Module → extension.Extension adapter
// ----------------------------------------------------------------------

// moduleAsExtension wraps a Module so that the cross-kind framework
// can read its manifest + versions without depending on this package's
// concrete types. The wrapper is value-typed because Module itself is
// (the mock returns Module values, not pointers); a pointer wrapper
// would surface aliasing bugs the moment the backend lands.
type moduleAsExtension struct {
	m Module
}

// Manifest projects a Module onto extension.Manifest. The kind is
// fixed to KindPolicyModule; that's the entire point of the adapter.
func (a moduleAsExtension) Manifest() extension.Manifest {
	return extension.Manifest{
		Kind:        extension.KindPolicyModule,
		ID:          a.m.ID,
		Title:       a.m.Title,
		Description: a.m.Description,
		Source:      mapOriginToSource(a.m.Origin),
		TenantID:    "", // populated when the backend tracks per-tenant ownership
	}
}

// Versions returns the framework projection of every version of this
// module, newest first (Module.Versions is already sorted that way).
func (a moduleAsExtension) Versions() []extension.Version {
	out := make([]extension.Version, 0, len(a.m.Versions))
	for _, v := range a.m.Versions {
		out = append(out, mapVersion(v))
	}
	return out
}

// LatestVersion returns the newest deployable version, or nil if none
// of the versions have reached LifecyclePublished. Drafts and revoked
// versions are skipped, matching the framework contract on
// extension.Extension.LatestVersion.
func (a moduleAsExtension) LatestVersion() *extension.Version {
	for _, v := range a.m.Versions {
		mapped := mapVersion(v)
		if mapped.State.IsDeployable() {
			return &mapped
		}
	}
	return nil
}

// AsExtension returns a framework-level view of this module. Concrete
// callers in this package keep using *Module; cross-kind callers go
// through the returned extension.Extension.
func (m Module) AsExtension() extension.Extension { return moduleAsExtension{m: m} }

// ----------------------------------------------------------------------
// String → enum mappings
//
// The mock's existing schema uses bare strings for Origin and Status
// (the historical types pre-date ADR-008). Mapping happens in one
// place so every adapter call sees the same translation.
// ----------------------------------------------------------------------

// mapOriginToSource translates Module.Origin into extension.Source.
// Unknown values default to SourceTenant — the safe fallback (treat
// as editable, scoped to a tenant) rather than SourceBundled which
// would grant trust the catalog never declared.
func mapOriginToSource(origin string) extension.Source {
	switch origin {
	case "bundled":
		return extension.SourceBundled
	case "tenant":
		return extension.SourceTenant
	case "imported":
		return extension.SourceImported
	case "community":
		return extension.SourceCommunity
	}
	return extension.SourceTenant
}

// mapStatusToLifecycle translates ModuleVersion.Status into
// extension.LifecycleState. The mock's "" / "draft" / "published" /
// "superseded" / "revoked" / "disabled" lexicon already matches the
// framework's enum 1:1; the switch exists so a future divergence
// (e.g. "deprecated" added to one side) lands in this one function.
func mapStatusToLifecycle(status string) extension.LifecycleState {
	switch status {
	case "published":
		return extension.LifecyclePublished
	case "superseded":
		return extension.LifecycleSuperseded
	case "disabled":
		return extension.LifecycleDisabled
	case "revoked":
		return extension.LifecycleRevoked
	case "draft", "":
		return extension.LifecycleDraft
	}
	return extension.LifecycleDraft
}

// mapVersion projects a ModuleVersion onto extension.Version.
func mapVersion(v ModuleVersion) extension.Version {
	return extension.Version{
		Version:     v.Version,
		State:       mapStatusToLifecycle(v.Status),
		PublishedAt: v.PublishedAt,
		PublishedBy: v.PublishedBy,
		Signature: extension.Signature{
			Signer: v.Signature.Signer,
			KeyID:  v.Signature.KeyID,
			Algo:   v.Signature.Algo,
			Bytes:  v.Signature.Bytes,
		},
	}
}

// ----------------------------------------------------------------------
// Kind registration
// ----------------------------------------------------------------------

// loadAllAsExtensions is the framework Loader for the policy-module
// kind. The framework calls this on demand from list / chrome paths;
// it must stay cheap. v1 wraps each Module in O(1); Catalog() reads
// through the provider installed by the service layer (Task 4.2).
func loadAllAsExtensions() []extension.Extension {
	mods := Catalog()
	out := make([]extension.Extension, 0, len(mods))
	for _, m := range mods {
		out = append(out, m.AsExtension())
	}
	return out
}

func init() {
	extension.RegisterKind(extension.KindSpec{
		Kind:        extension.KindPolicyModule,
		Title:       "Policy Modules",
		Description: "Signed, versioned bundles that implement Catalog policies on Linux endpoints. See ADR-007.",
		Loader:      loadAllAsExtensions,
	})
}
