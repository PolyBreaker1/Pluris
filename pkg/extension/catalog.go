package extension

import (
	"sort"
	"sync"
)

// ----------------------------------------------------------------------
// Kind registry — concrete kinds publish themselves at init() time.
// ----------------------------------------------------------------------

// Loader is a function that returns every Extension currently known
// for one Kind. The framework calls it on demand (no caching at this
// layer — concrete kinds may cache internally if they want).
//
// Keep loaders cheap: they're called from list-rendering paths and
// from the Sources page on every request. The v1 mock loaders return
// pre-built slices in O(N); when the backend lands, a loader becomes
// a single typed Ent query.
type Loader func() []Extension

// KindSpec is what a concrete package registers via RegisterKind. It
// carries enough metadata that cross-kind chrome (the Sources page,
// the future "Browse Extensions" surface) can render a row for the
// kind without importing the concrete package.
type KindSpec struct {
	// Kind — discriminator. Must be a constant declared in types.go.
	Kind Kind

	// Title is the human-readable family name shown in chrome
	// ("Policy Modules", "Themes", "Glossary Packs").
	Title string

	// Description is one sentence explaining the kind to an admin who
	// has never seen it before. Shown on the Sources page when no
	// extensions of this kind are installed.
	Description string

	// Loader returns every Extension currently known for this kind.
	// Required.
	Loader Loader
}

var (
	kindMu    sync.RWMutex
	kindSpecs = map[Kind]KindSpec{}
)

// RegisterKind publishes a concrete kind to the framework. Called from
// the concrete package's init(). Duplicate registration of the same
// kind panics — this is a programming error, not a runtime condition.
func RegisterKind(spec KindSpec) {
	kindMu.Lock()
	defer kindMu.Unlock()
	if spec.Kind == "" {
		panic("extension: RegisterKind called with empty Kind")
	}
	if spec.Loader == nil {
		panic("extension: RegisterKind called with nil Loader for kind " + string(spec.Kind))
	}
	if _, dup := kindSpecs[spec.Kind]; dup {
		panic("extension: kind " + string(spec.Kind) + " already registered")
	}
	kindSpecs[spec.Kind] = spec
}

// LookupKind returns the spec for kind k. The second return is false
// if no kind is registered under that key. Used by templates and tests
// that need to render a kind's family name without importing the
// concrete package.
func LookupKind(k Kind) (KindSpec, bool) {
	kindMu.RLock()
	defer kindMu.RUnlock()
	spec, ok := kindSpecs[k]
	return spec, ok
}

// RegisteredKinds returns every registered kind in a stable order
// (sorted by Kind string). Used by the Sources page to enumerate the
// kinds it knows about.
func RegisteredKinds() []KindSpec {
	kindMu.RLock()
	defer kindMu.RUnlock()
	out := make([]KindSpec, 0, len(kindSpecs))
	for _, spec := range kindSpecs {
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// ----------------------------------------------------------------------
// Cross-kind queries — the "what's installed?" view that powers chrome.
// ----------------------------------------------------------------------

// AllOfKind returns every Extension currently known for kind k. Returns
// nil if k is not registered.
func AllOfKind(k Kind) []Extension {
	spec, ok := LookupKind(k)
	if !ok {
		return nil
	}
	return spec.Loader()
}

// All returns every Extension across every registered kind. Order is
// kind-by-kind in RegisteredKinds order, with each kind's internal
// order preserved. Used by the Sources page's grand totals and by the
// audit log's "what changed" diff.
func All() []Extension {
	specs := RegisteredKinds()
	out := []Extension{}
	for _, spec := range specs {
		out = append(out, spec.Loader()...)
	}
	return out
}

// CountBySource is the breakdown shown on the Sources page header
// ("12 bundled · 4 tenant · 0 imported"). Counts are over the latest
// deployable version of each extension; an extension with no published
// version is not counted (it has no source to attribute to).
//
// Restrict to a single kind by passing it; pass empty Kind ("") to
// count across every registered kind.
func CountBySource(k Kind) map[Source]int {
	out := map[Source]int{
		SourceBundled:   0,
		SourceTenant:    0,
		SourceImported:  0,
		SourceCommunity: 0,
	}
	var exts []Extension
	if k == "" {
		exts = All()
	} else {
		exts = AllOfKind(k)
	}
	for _, ext := range exts {
		if ext.LatestVersion() == nil {
			continue
		}
		m := ext.Manifest()
		if m.Source.IsValid() {
			out[m.Source]++
		}
	}
	return out
}

// ----------------------------------------------------------------------
// Test seam — clear all registrations. Used by tests that re-register
// kinds with different loaders. NEVER call this from production code.
// ----------------------------------------------------------------------

// resetForTest is intentionally unexported. Tests in the extension
// package itself can call it. Tests in other packages that need a
// fresh registry should be re-architected to depend on the registry's
// behaviour rather than its emptiness.
func resetForTest() {
	kindMu.Lock()
	defer kindMu.Unlock()
	kindSpecs = map[Kind]KindSpec{}
}
