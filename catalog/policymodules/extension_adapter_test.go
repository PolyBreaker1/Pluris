package policymodules

import (
	"testing"

	"github.com/pluris/pluris/pkg/extension"
)

// TestModuleSatisfiesExtension is a compile-time + runtime assertion
// that Module's framework projection truly implements extension.Extension.
// If a future refactor changes the projection's shape this test breaks
// loudly rather than the kind silently disappearing from cross-kind
// surfaces (Sources page, audit log, registry).
func TestModuleSatisfiesExtension(t *testing.T) {
	mods := testCatalog()
	if len(mods) == 0 {
		t.Fatalf("testCatalog() returned 0 — the rest of the test cannot run")
	}
	var _ extension.Extension = mods[0].AsExtension() // compile-time
}

func TestKindRegistration(t *testing.T) {
	spec, ok := extension.LookupKind(extension.KindPolicyModule)
	if !ok {
		t.Fatalf("policymodules init() failed to register KindPolicyModule with the framework")
	}
	if spec.Title == "" {
		t.Errorf("KindPolicyModule registered with empty Title")
	}
	if spec.Loader == nil {
		t.Errorf("KindPolicyModule registered with nil Loader")
	}
}

func TestLoaderReturnsAllModulesAsExtensions(t *testing.T) {
	prev := catalogProvider
	SetCatalogProvider(testCatalog)
	defer func() { catalogProvider = prev }()

	spec, ok := extension.LookupKind(extension.KindPolicyModule)
	if !ok {
		t.Fatal("kind not registered")
	}
	exts := spec.Loader()
	if len(exts) != len(testCatalog()) {
		t.Errorf("Loader returned %d extensions, want %d (one per Module)",
			len(exts), len(testCatalog()))
	}
	for _, ext := range exts {
		if ext.Manifest().Kind != extension.KindPolicyModule {
			t.Errorf("Loader returned an Extension whose Manifest.Kind is %q, want %q",
				ext.Manifest().Kind, extension.KindPolicyModule)
		}
	}
}

func TestOriginToSourceMapping(t *testing.T) {
	cases := map[string]extension.Source{
		"bundled":   extension.SourceBundled,
		"tenant":    extension.SourceTenant,
		"imported":  extension.SourceImported,
		"community": extension.SourceCommunity,
		"":          extension.SourceTenant, // safe-fallback documented behaviour
		"garbage":   extension.SourceTenant, // safe-fallback documented behaviour
	}
	for in, want := range cases {
		if got := mapOriginToSource(in); got != want {
			t.Errorf("mapOriginToSource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatusToLifecycleMapping(t *testing.T) {
	cases := map[string]extension.LifecycleState{
		"published":  extension.LifecyclePublished,
		"superseded": extension.LifecycleSuperseded,
		"disabled":   extension.LifecycleDisabled,
		"revoked":    extension.LifecycleRevoked,
		"draft":      extension.LifecycleDraft,
		"":           extension.LifecycleDraft,
		"weird":      extension.LifecycleDraft,
	}
	for in, want := range cases {
		if got := mapStatusToLifecycle(in); got != want {
			t.Errorf("mapStatusToLifecycle(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLatestVersionSkipsDraftsAndRevoked locks the framework contract
// that Extension.LatestVersion returns the newest *deployable* version,
// not the newest version overall.
func TestLatestVersionSkipsDraftsAndRevoked(t *testing.T) {
	m := Module{
		ID:    "test.module",
		Title: "Test",
		Versions: []ModuleVersion{
			{Version: "3.0.0", Status: "draft"},     // newer but not deployable
			{Version: "2.0.0", Status: "revoked"},   // newer but terminal
			{Version: "1.5.0", Status: "published"}, // FIRST deployable in newest-first order — should win
			{Version: "1.0.0", Status: "published"},
		},
	}
	got := m.AsExtension().LatestVersion()
	if got == nil {
		t.Fatal("LatestVersion() returned nil despite a published version existing")
	}
	if got.Version != "1.5.0" {
		t.Errorf("LatestVersion() = %q, want 1.5.0 (drafts + revoked must be skipped)", got.Version)
	}
}

func TestLatestVersionNilWhenNothingPublished(t *testing.T) {
	m := Module{
		ID:    "draft.only",
		Title: "Draft Only",
		Versions: []ModuleVersion{
			{Version: "0.1.0", Status: "draft"},
		},
	}
	if got := m.AsExtension().LatestVersion(); got != nil {
		t.Errorf("LatestVersion() on draft-only module = %+v, want nil", got)
	}
}

// TestCountBySourceMatchesMockOrigins is the cross-kind end-to-end:
// the framework's CountBySource should reflect the Origin distribution
// in the mock without policymodules-specific knowledge leaking into
// pkg/extension.
func TestCountBySourceMatchesMockOrigins(t *testing.T) {
	prev := catalogProvider
	SetCatalogProvider(testCatalog)
	defer func() { catalogProvider = prev }()

	got := extension.CountBySource(extension.KindPolicyModule)

	want := map[extension.Source]int{
		extension.SourceBundled:   0,
		extension.SourceTenant:    0,
		extension.SourceImported:  0,
		extension.SourceCommunity: 0,
	}
	for _, m := range testCatalog() {
		ext := m.AsExtension()
		if ext.LatestVersion() == nil {
			continue
		}
		want[mapOriginToSource(m.Origin)]++
	}

	for src, w := range want {
		if got[src] != w {
			t.Errorf("CountBySource[%q] = %d, want %d", src, got[src], w)
		}
	}
}
