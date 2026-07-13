package params

import "testing"

// Existing entity-path behavior must be 100% unchanged by the introduction
// of the Source abstraction.
func TestEntitySourceRegisteredAndCovers(t *testing.T) {
	all := AllPaths()
	if len(all) == 0 {
		t.Fatal("expected AllPaths to be non-empty")
	}
	// Every path returned by AllPaths() must also resolve via ResolvePath,
	// confirming the entity source is wired into the same aggregation used
	// by consumers.
	for _, p := range all {
		if _, _, _, err := ResolvePath(p); err != nil {
			t.Errorf("path %q from AllPaths() failed to resolve: %v", p, err)
		}
	}
}

// module/ paths are reserved for a later task (dynamic per-module input
// resolution). For now they must resolve to a clear not-found error, never
// panic.
func TestModuleNamespaceReservedNotFoundNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ResolvePath panicked on reserved module/ namespace: %v", r)
		}
	}()
	_, _, _, err := ResolvePath("module/input/x")
	if err == nil {
		t.Fatal("expected error resolving module/input/x, got nil")
	}
}

// tenant/ paths are likewise reserved for a later task.
func TestTenantNamespaceReservedNotFoundNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ResolvePath panicked on reserved tenant/ namespace: %v", r)
		}
	}()
	_, _, _, err := ResolvePath("tenant/section/x")
	if err == nil {
		t.Fatal("expected error resolving tenant/section/x, got nil")
	}
}

// fakeSource is a minimal non-entity Source used to prove ResolveDef and
// EffectivePermission actually consult registered sources, closing the
// historic Paths()/ResolvePath asymmetry (Task 4.4).
type fakeSource struct {
	path string
	def  ParamDef
}

func (f fakeSource) Paths() []string { return []string{f.path} }
func (f fakeSource) Resolve(path string) (ParamDef, bool) {
	if path == f.path {
		return f.def, true
	}
	return ParamDef{}, false
}

// withFakeSource registers s for the duration of the test and restores
// the previous `sources` slice on cleanup, so this test's registration
// never leaks into other tests in the package.
func withFakeSource(t *testing.T, s Source) {
	t.Helper()
	prev := sources
	sources = append(append([]Source{}, prev...), s)
	t.Cleanup(func() { sources = prev })
}

// A registered non-entity source's path resolves via ResolveDef and
// contributes its Permission to EffectivePermission — the asymmetry that
// used to leave AllPaths()-listed non-entity paths unresolvable by any
// Source-generic entry point.
func TestResolveDefAndEffectivePermissionConsultRegisteredSources(t *testing.T) {
	fs := fakeSource{
		path: "tenant/custom/widget",
		def:  ParamDef{Key: "widget", Label: "Widget", Type: TypeString, Permission: "tenant.custom.view"},
	}
	withFakeSource(t, fs)

	def, ok := ResolveDef(fs.path)
	if !ok {
		t.Fatalf("ResolveDef(%q) = not found, want found", fs.path)
	}
	if def.Key != "widget" || def.Label != "Widget" {
		t.Errorf("ResolveDef(%q) = %+v, want Key=widget Label=Widget", fs.path, def)
	}

	if perm := EffectivePermission(fs.path); perm != "tenant.custom.view" {
		t.Errorf("EffectivePermission(%q) = %q, want tenant.custom.view", fs.path, perm)
	}

	// Unknown path (not owned by any registered source) still fails
	// closed, never panics.
	if _, ok := ResolveDef("tenant/custom/unknown"); ok {
		t.Error("ResolveDef of an unregistered path unexpectedly resolved")
	}
	if perm := EffectivePermission("tenant/custom/unknown"); perm != "" {
		t.Errorf("EffectivePermission of an unregistered path = %q, want empty", perm)
	}
}

// Entity paths must resolve identically via ResolveDef and the direct
// ResolvePath call — introducing the Source-generic entry point must not
// change entity resolution at all.
func TestResolveDefEntityPathsUnchanged(t *testing.T) {
	for _, p := range AllPaths() {
		wantSchema, wantSection, wantDef, wantErr := ResolvePath(p)
		gotDef, gotOK := ResolveDef(p)
		if wantErr != nil {
			if gotOK {
				t.Errorf("ResolveDef(%q) resolved but ResolvePath errored: %v", p, wantErr)
			}
			continue
		}
		if !gotOK {
			t.Errorf("ResolveDef(%q) = not found, want found (ResolvePath succeeded)", p)
			continue
		}
		if gotDef.Key != wantDef.Key || gotDef.Type != wantDef.Type {
			t.Errorf("ResolveDef(%q) = %+v, want %+v", p, gotDef, *wantDef)
		}
		_ = wantSchema
		_ = wantSection
	}
}
