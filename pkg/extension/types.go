// Package extension is the cross-cutting framework that every Pluris
// extensibility surface plugs into.
//
// Pluris ships with one extensibility surface today — Policy Modules,
// the signed-and-versioned bundles that implement Catalog policies on
// Linux endpoints (ADR-007). The roadmap adds more: themes, dashboard
// tile types, glossary packs, GP-import adapters, identity-provider
// adapters, update-channel curations, localizations. Every one of
// those families shares the same cross-cutting concerns:
//
//   - identity              (kind + id + version)
//   - provenance            (source + signature)
//   - lifecycle             (draft / published / disabled / revoked)
//   - tenant scoping        (bundled vs tenant-authored vs imported)
//   - canonical editor      (R1 — one editor per concept; never branch)
//
// Rather than re-design those concerns once per family, ADR-008
// promotes them into this package. A new "kind" (theme, glossary pack,
// dashboard tile, …) implements the Extension interface and registers
// itself via RegisterKind in its package's init(). The Sources page,
// the Library page chrome, the install / sign / revoke flows, and the
// future community-registry mechanism all read through this framework.
//
// What is NOT in this package (deferred to later slices):
//
//   - per-kind permission grammars (Slice B/C — declared & validated
//     here, but the grammar lives in each kind)
//   - refcount lift (today refcount lives in policymodules.ModuleInstallation;
//     when the second kind needs it, the helper graduates here)
//   - editor mounting / settings schema (Slice C)
//   - community registry fetch / signature verification chain (Slice D)
//
// Layering rule: this package depends on nothing inside pluris/.
// Concrete kinds (catalog/policymodules, future catalog/themes, etc.)
// depend on this package, never the reverse. That keeps the framework
// a true cross-cutting concern and prevents import cycles.
package extension

// ----------------------------------------------------------------------
// Kind — the discriminator for the family this extension belongs to.
// ----------------------------------------------------------------------

// Kind is a stable string identifier for a family of extensions. The
// value is used in:
//
//   - extension manifests (the `kind:` field a community author writes)
//   - registry catalog rows (one row per kind+id+version)
//   - URL routing (/policy/modules, /preferences/themes, …)
//   - audit log lines ("alice installed kind=policy-module id=…")
//
// Kinds register themselves via RegisterKind. New kinds must add their
// constant to this file (alphabetical) and require an ADR amendment.
type Kind string

const (
	// KindPolicyModule is the canonical first kind: signed bundles
	// containing apply / disable / uninstall / validate / report
	// scripts that implement Catalog policies on a Linux endpoint.
	// See catalog/policymodules and ADR-007.
	KindPolicyModule Kind = "policy-module"
)

// String returns the kind as a plain string (satisfies fmt.Stringer).
func (k Kind) String() string { return string(k) }

// ----------------------------------------------------------------------
// Source — where an extension version came from. Drives trust badges,
// permission gates, and the Sources page.
// ----------------------------------------------------------------------

// Source identifies where a particular extension originated. It is
// orthogonal to the lifecycle state: a tenant-authored extension can
// be in draft, published, or revoked state independent of its source.
type Source string

const (
	// SourceBundled — ships with the Pluris release, signed by the
	// Pluris release key. Read-only in every tenant; cloning produces
	// a SourceTenant copy.
	SourceBundled Source = "bundled"

	// SourceTenant — authored or cloned in this tenant via a wizard
	// or upload. Signed by the tenant's signing key. Editable by
	// users with the appropriate role.
	SourceTenant Source = "tenant"

	// SourceImported — fetched from another tenant's export bundle
	// or a vendor-supplied catalogue. Trust is per-source: the admin
	// approved the signing key when the source was added.
	SourceImported Source = "imported"

	// SourceCommunity — fetched from a public community registry.
	// Always sandboxed at install time; permissions are reviewed
	// before any code runs. (Slice D / community registry.)
	SourceCommunity Source = "community"
)

// IsValid reports whether s is one of the four canonical sources.
func (s Source) IsValid() bool {
	switch s {
	case SourceBundled, SourceTenant, SourceImported, SourceCommunity:
		return true
	}
	return false
}

// IsEditable returns true for sources whose extensions are mutable in
// the current tenant. Bundled / imported / community are read-only;
// only SourceTenant produces editable extensions.
func (s Source) IsEditable() bool { return s == SourceTenant }

// ----------------------------------------------------------------------
// LifecycleState — uniform lifecycle across all kinds. Per-kind state
// machines live in the concrete package (e.g. policymodules adds
// `superseded` for older versions still required by pinned bindings).
// ----------------------------------------------------------------------

// LifecycleState is the framework-level state of an extension version.
// Concrete kinds may add finer-grained sub-states; the framework's
// state is what the Sources page, the audit log, and the install
// gate reason about.
type LifecycleState string

const (
	// LifecycleDraft — under construction in an editor; not signed,
	// not deployable. Visible only to authors with edit permission.
	LifecycleDraft LifecycleState = "draft"

	// LifecyclePublished — signed and immutable. Eligible for binding,
	// install, and propagation to agents.
	LifecyclePublished LifecycleState = "published"

	// LifecycleSuperseded — a newer published version exists. The
	// version stays selectable for rollback and for installations
	// pinned to it; not offered as a default.
	LifecycleSuperseded LifecycleState = "superseded"

	// LifecycleDisabled — admin temporarily disabled this version.
	// Existing installations stop running it; new bindings cannot
	// pick it. Reversible.
	LifecycleDisabled LifecycleState = "disabled"

	// LifecycleRevoked — admin pulled this version (security issue,
	// signing-key compromise, …). Agents must refuse to run it.
	// Irreversible: revocation is a one-way trip.
	LifecycleRevoked LifecycleState = "revoked"
)

// IsDeployable returns true if a version in this state can be selected
// by a binding or kept on an agent. Used by the resolver and the
// install gate.
func (s LifecycleState) IsDeployable() bool {
	return s == LifecyclePublished || s == LifecycleSuperseded
}

// IsTerminal returns true if the state is irreversible.
func (s LifecycleState) IsTerminal() bool { return s == LifecycleRevoked }

// ----------------------------------------------------------------------
// Signature — every published version carries one. The framework holds
// the metadata; concrete kinds carry the wire bytes (for v1 the bytes
// are empty, mock-only).
// ----------------------------------------------------------------------

// Signature is the trust metadata attached to a published extension
// version. INV-X3 says every non-draft version has a Signature with a
// non-empty Signer and KeyID — even the bundled key is named.
type Signature struct {
	// Signer is the human-readable name shown in the trust chip.
	// Examples: "Pluris", "tenant:acme", "alice.chen@acme.local".
	Signer string

	// KeyID is the canonical identifier for the public key. Format:
	//   - bundled:    "pluris:bundled:<n>"
	//   - tenant:     "tenant:<tenant_id>:key:<key_id>"
	//   - community:  "community:<author>:<key_id>"
	KeyID string

	// Algo is the signature algorithm. v1 fixes this to "ed25519";
	// the field exists so a future migration can be detected.
	Algo string

	// Bytes is the base64-encoded signature over the manifest +
	// content hashes. Empty in the v1 mock; the trust chip renders
	// from Signer / KeyID / Algo.
	Bytes string
}

// IsZero reports whether s carries no signing metadata. Drafts always
// return true; published versions must always return false (INV-X3).
func (s Signature) IsZero() bool { return s.Signer == "" && s.KeyID == "" }

// ----------------------------------------------------------------------
// Manifest — universal header every extension declares.
// ----------------------------------------------------------------------

// Manifest carries the cross-kind identity for one extension entry
// (one Kind + one ID; multiple Versions hang off it via the Extension
// interface). Kind-specific fields (TargetOS for modules, CSS variables
// for themes, …) live in the concrete package's struct; this struct is
// what the Sources page, the audit log, and the registry agree on.
type Manifest struct {
	// Kind discriminates which family this is. Must be a registered
	// kind (RegisterKind); the framework refuses to serve manifests
	// for unknown kinds.
	Kind Kind

	// ID is the stable, dotted-slug identifier inside its kind.
	// Examples:
	//   - policy-module: "pluris.sshd.password-auth-disable"
	//   - theme:         "pluris.themes.dark-cyan"
	// Must not change for a given semantic extension; renames require
	// a new ID + a redirect entry.
	ID string

	// Title is the display name shown in lists and editor headers.
	Title string

	// Description is 1–3 sentences. The longer authoring guide lives
	// inside the bundle (README.md by convention).
	Description string

	// Source is where this extension entry came from. See Source.
	Source Source

	// TenantID is set for SourceTenant and SourceImported. Empty for
	// SourceBundled (no tenant — it ships with Pluris) and for
	// SourceCommunity until installed (then the receiving tenant's id
	// is recorded as the install-tenant, not the authoring one).
	TenantID string
}

// ----------------------------------------------------------------------
// Version — universal version row.
// ----------------------------------------------------------------------

// Version is the framework-level view of one published version of an
// extension. The concrete kind owns the full version struct (with its
// kind-specific fields like Sandbox or Scripts); this is the projection
// that cross-kind code (Sources, audit log, registry) reads.
type Version struct {
	// Version is the semver string. v1 mock treats it as opaque; the
	// resolver wires real semver in Phase 1.
	Version string

	// State is the framework-level lifecycle state. See LifecycleState.
	State LifecycleState

	// PublishedAt is ISO8601. Empty for drafts.
	PublishedAt string

	// PublishedBy is a human label ("alice.chen@acme.local"). Empty
	// for drafts.
	PublishedBy string

	// Signature is the trust metadata. IsZero() must be true for
	// drafts and false for every other state (INV-X3).
	Signature Signature
}

// ----------------------------------------------------------------------
// Extension — the cross-kind view every concrete extension implements.
// ----------------------------------------------------------------------

// Extension is the interface every concrete kind's entry type
// satisfies. Methods return projections, not the underlying mutable
// fields, so a future immutable backend can implement the same
// interface without exposing storage details.
//
// Implementations live next to the kind. For the policy-module kind
// see catalog/policymodules.Module.AsExtension().
type Extension interface {
	// Manifest returns the universal header. The Kind on the returned
	// Manifest must match the kind under which this Extension was
	// registered.
	Manifest() Manifest

	// Versions returns every version known for this extension, newest
	// first. The slice may be empty (a freshly created tenant entry
	// before its first publish); callers must tolerate that.
	Versions() []Version

	// LatestVersion returns the most recently published version, or
	// nil if no version has reached LifecyclePublished. Drafts are
	// not returned by this accessor — the Sources page and the
	// resolver want only deployable versions.
	LatestVersion() *Version
}
