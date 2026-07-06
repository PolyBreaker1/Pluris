package extension

import (
	"testing"
)

// ----------------------------------------------------------------------
// Enum invariants — every enum value present in the public API must
// satisfy its IsValid / IsDeployable / IsTerminal predicates as
// documented. If a future kind adds a state, this test forces an
// explicit decision rather than a silent default.
// ----------------------------------------------------------------------

func TestSourceIsValid(t *testing.T) {
	for _, s := range []Source{SourceBundled, SourceTenant, SourceImported, SourceCommunity} {
		if !s.IsValid() {
			t.Errorf("Source %q must be IsValid()", s)
		}
	}
	if Source("nonsense").IsValid() {
		t.Errorf("unknown Source must not be IsValid()")
	}
}

func TestSourceIsEditable(t *testing.T) {
	editable := map[Source]bool{
		SourceBundled:   false,
		SourceTenant:    true,
		SourceImported:  false,
		SourceCommunity: false,
	}
	for s, want := range editable {
		if got := s.IsEditable(); got != want {
			t.Errorf("Source(%q).IsEditable() = %v, want %v", s, got, want)
		}
	}
}

func TestLifecycleStateIsDeployable(t *testing.T) {
	deployable := map[LifecycleState]bool{
		LifecycleDraft:      false,
		LifecyclePublished:  true,
		LifecycleSuperseded: true, // pinned bindings still need it
		LifecycleDisabled:   false,
		LifecycleRevoked:    false,
	}
	for s, want := range deployable {
		if got := s.IsDeployable(); got != want {
			t.Errorf("LifecycleState(%q).IsDeployable() = %v, want %v", s, got, want)
		}
	}
}

func TestLifecycleRevokedIsTerminal(t *testing.T) {
	if !LifecycleRevoked.IsTerminal() {
		t.Errorf("LifecycleRevoked must be terminal (irreversible)")
	}
	for _, s := range []LifecycleState{LifecycleDraft, LifecyclePublished, LifecycleSuperseded, LifecycleDisabled} {
		if s.IsTerminal() {
			t.Errorf("LifecycleState(%q).IsTerminal() must be false", s)
		}
	}
}

func TestSignatureIsZero(t *testing.T) {
	if !(Signature{}).IsZero() {
		t.Errorf("zero Signature must be IsZero()")
	}
	if (Signature{Signer: "Pluris", KeyID: "pluris:bundled:1"}).IsZero() {
		t.Errorf("Signature with Signer + KeyID must NOT be IsZero()")
	}
}

// ----------------------------------------------------------------------
// Catalog / RegisterKind — duplicate-registration is a programmer
// error and must panic; lookups for unknown kinds return false; All()
// aggregates across kinds in the documented stable order.
// ----------------------------------------------------------------------

// fakeExt is a minimal Extension implementation for catalog tests. It
// lives only in the test binary; the production code path uses real
// adapters (e.g. policymodules.moduleAsExtension).
type fakeExt struct {
	kind   Kind
	id     string
	source Source
	latest *Version
}

func (f fakeExt) Manifest() Manifest {
	return Manifest{Kind: f.kind, ID: f.id, Title: f.id, Source: f.source}
}
func (f fakeExt) Versions() []Version {
	if f.latest == nil {
		return nil
	}
	return []Version{*f.latest}
}
func (f fakeExt) LatestVersion() *Version { return f.latest }

func TestRegisterKindPanicsOnDuplicate(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	RegisterKind(KindSpec{Kind: "k", Title: "K", Loader: func() []Extension { return nil }})

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on duplicate kind registration")
		}
	}()
	RegisterKind(KindSpec{Kind: "k", Title: "K", Loader: func() []Extension { return nil }})
}

func TestRegisterKindRejectsEmpty(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on empty Kind")
		}
	}()
	RegisterKind(KindSpec{Kind: "", Title: "x", Loader: func() []Extension { return nil }})
}

func TestRegisterKindRejectsNilLoader(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil Loader")
		}
	}()
	RegisterKind(KindSpec{Kind: "k", Title: "x", Loader: nil})
}

func TestLookupKindUnknown(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	if _, ok := LookupKind("no-such-kind"); ok {
		t.Errorf("LookupKind on unknown kind must return ok=false")
	}
}

func TestRegisteredKindsStableOrder(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	// Register out of alphabetical order on purpose.
	RegisterKind(KindSpec{Kind: "zzz", Title: "Z", Loader: func() []Extension { return nil }})
	RegisterKind(KindSpec{Kind: "aaa", Title: "A", Loader: func() []Extension { return nil }})
	RegisterKind(KindSpec{Kind: "mmm", Title: "M", Loader: func() []Extension { return nil }})

	got := RegisteredKinds()
	want := []Kind{"aaa", "mmm", "zzz"}
	if len(got) != len(want) {
		t.Fatalf("RegisteredKinds returned %d, want %d", len(got), len(want))
	}
	for i, k := range want {
		if got[i].Kind != k {
			t.Errorf("RegisteredKinds[%d] = %q, want %q", i, got[i].Kind, k)
		}
	}
}

func TestAllAggregatesAcrossKinds(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	pubV := &Version{Version: "1.0.0", State: LifecyclePublished}
	RegisterKind(KindSpec{
		Kind:  "alpha",
		Title: "Alpha",
		Loader: func() []Extension {
			return []Extension{fakeExt{kind: "alpha", id: "a1", source: SourceBundled, latest: pubV}}
		},
	})
	RegisterKind(KindSpec{
		Kind:  "beta",
		Title: "Beta",
		Loader: func() []Extension {
			return []Extension{
				fakeExt{kind: "beta", id: "b1", source: SourceTenant, latest: pubV},
				fakeExt{kind: "beta", id: "b2", source: SourceTenant, latest: pubV},
			}
		},
	})

	got := All()
	if len(got) != 3 {
		t.Errorf("All() returned %d, want 3 (1 from alpha + 2 from beta)", len(got))
	}
	// Stable ordering: alpha kind first (alphabetical), then beta.
	if got[0].Manifest().Kind != "alpha" || got[1].Manifest().Kind != "beta" || got[2].Manifest().Kind != "beta" {
		t.Errorf("All() order broken; got kinds %q %q %q",
			got[0].Manifest().Kind, got[1].Manifest().Kind, got[2].Manifest().Kind)
	}
}

func TestCountBySourceSkipsUnpublished(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	pub := &Version{Version: "1.0.0", State: LifecyclePublished}
	RegisterKind(KindSpec{
		Kind:  "k",
		Title: "K",
		Loader: func() []Extension {
			return []Extension{
				fakeExt{kind: "k", id: "bundled1", source: SourceBundled, latest: pub},
				fakeExt{kind: "k", id: "bundled2", source: SourceBundled, latest: pub},
				fakeExt{kind: "k", id: "tenant1", source: SourceTenant, latest: pub},
				fakeExt{kind: "k", id: "draft-only", source: SourceTenant, latest: nil}, // unpublished — must NOT count
				fakeExt{kind: "k", id: "imported1", source: SourceImported, latest: pub},
			}
		},
	})

	got := CountBySource("k")
	want := map[Source]int{
		SourceBundled:   2,
		SourceTenant:    1, // draft-only excluded
		SourceImported:  1,
		SourceCommunity: 0,
	}
	for src, w := range want {
		if got[src] != w {
			t.Errorf("CountBySource[%q] = %d, want %d", src, got[src], w)
		}
	}
}

func TestCountBySourceAggregatesAcrossKinds(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	pub := &Version{Version: "1.0.0", State: LifecyclePublished}
	RegisterKind(KindSpec{Kind: "k1", Title: "K1", Loader: func() []Extension {
		return []Extension{fakeExt{kind: "k1", id: "a", source: SourceBundled, latest: pub}}
	}})
	RegisterKind(KindSpec{Kind: "k2", Title: "K2", Loader: func() []Extension {
		return []Extension{fakeExt{kind: "k2", id: "b", source: SourceBundled, latest: pub}}
	}})

	got := CountBySource("") // "" — across every kind
	if got[SourceBundled] != 2 {
		t.Errorf("CountBySource('') across two kinds = %d, want 2", got[SourceBundled])
	}
}
