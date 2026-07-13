package params

import (
	"testing"

	"github.com/pluris/pluris/catalog/permissions"
)

// defsWithEffectivePermission resolves every canonical path to its
// ParamDef and stamps a copy's Permission field with EffectivePermission
// for that path — the contract FilterByGrants documents for callers that
// want schema defaults applied. One entry per mounting path, so a shared
// key (e.g. "tenant" mounted by both an asset schema and the identity
// schema) can appear more than once with different effective permissions.
func defsWithEffectivePermission() []ParamDef {
	paths := AllPaths()
	out := make([]ParamDef, 0, len(paths))
	for _, p := range paths {
		_, _, def, err := ResolvePath(p)
		if err != nil {
			continue
		}
		d := *def
		d.Permission = EffectivePermission(p)
		out = append(out, d)
	}
	return out
}

// Every canonical path must resolve to an effective permission that is
// either empty (visible to any authenticated user) or a valid key in the
// catalog/permissions registry.
func TestEffectivePermissionIsValidOrEmpty(t *testing.T) {
	for _, p := range AllPaths() {
		perm := EffectivePermission(p)
		if perm == "" {
			continue
		}
		if permissions.ActionByKey(perm) == nil {
			t.Errorf("path %q resolves to unknown permission key %q", p, perm)
		}
	}
}

// Asset subtypes (computer/server/printer/desk) default to asset.view;
// identity defaults to identity.view.
func TestEffectivePermissionSchemaDefaults(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"computer/hardware/ram_mb", "asset.view"},
		{"server/hardware/ram_mb", "asset.view"},
		{"printer/hardware/printer_model", "asset.view"},
		{"desk/hardware/desk_location", "asset.view"},
		{"user/identity/email", "identity.view"},
	}
	for _, c := range cases {
		if got := EffectivePermission(c.path); got != c.want {
			t.Errorf("EffectivePermission(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// FilterByGrants hides all computer/server/printer/desk params when
// asset.view is denied, but keeps identity params.
func TestFilterByGrantsDeniesAssetView(t *testing.T) {
	has := func(key string) bool { return key != "asset.view" }
	all := defsWithEffectivePermission()
	filtered := FilterByGrants(has, all)

	filteredKeys := map[string]bool{}
	for _, d := range filtered {
		filteredKeys[d.Key] = true
	}

	// Only check keys that are NOT also mounted by the identity schema:
	// a handful of keys (tenant, site) are shared across an asset schema
	// and identity, each with its own effective permission per mounting
	// path, so they may legitimately survive via their identity mount
	// even while hidden via their asset mount.
	for _, key := range SchemaComputer.AllParamKeys() {
		if SchemaIdentity.HasParam(key) {
			continue
		}
		if EffectivePermission(PathFor("computer", key)) == "asset.view" && filteredKeys[key] {
			t.Errorf("expected %q to be hidden when asset.view is denied", key)
		}
	}
	if !filteredKeys["email"] {
		t.Error("expected identity param email to remain visible when asset.view is denied")
	}
}

// FilterByGrants hides identity params when identity.view is denied, but
// keeps asset params.
func TestFilterByGrantsDeniesIdentityView(t *testing.T) {
	has := func(key string) bool { return key != "identity.view" }
	all := defsWithEffectivePermission()
	filtered := FilterByGrants(has, all)

	filteredKeys := map[string]bool{}
	for _, d := range filtered {
		filteredKeys[d.Key] = true
	}

	if filteredKeys["email"] {
		t.Error("expected identity param email to be hidden when identity.view is denied")
	}
	if !filteredKeys["ram_mb"] {
		t.Error("expected asset param ram_mb to remain visible when identity.view is denied")
	}
}

// EffectiveDefs must stamp every def's Permission with its effective
// permission — this closes the Task 1.1 footgun where raw schema defs all
// carry an empty Permission (schema default not resolved) and
// FilterByGrants silently treats everything as visible.
func TestEffectiveDefsStampsPermission(t *testing.T) {
	for _, md := range SchemaComputer.EffectiveDefs() {
		if md.Def.Permission != "asset.view" {
			t.Errorf("computer mounted def %q Permission = %q, want %q (stamped)", md.Path, md.Def.Permission, "asset.view")
		}
	}
	for _, md := range SchemaIdentity.EffectiveDefs() {
		if md.Def.Permission != "identity.view" {
			t.Errorf("identity mounted def %q Permission = %q, want %q (stamped)", md.Path, md.Def.Permission, "identity.view")
		}
	}
}

// EffectiveDefs preserves schema order: section order, then param order
// within the section, with SectionKey/SectionLabel matching the mount.
func TestEffectiveDefsSchemaOrder(t *testing.T) {
	defs := SchemaComputer.EffectiveDefs()
	i := 0
	for _, sec := range SchemaComputer.Sections {
		for _, key := range sec.Params {
			if DefByKey(key) == nil {
				continue
			}
			if i >= len(defs) {
				t.Fatalf("EffectiveDefs ran out at %d, expected %s/%s next", i, sec.Key, key)
			}
			md := defs[i]
			wantPath := SchemaComputer.PathEntity + "/" + sec.Key + "/" + key
			if md.Path != wantPath || md.Def.Key != key || md.SectionKey != sec.Key || md.SectionLabel != sec.Label {
				t.Errorf("defs[%d] = {Path:%q Key:%q Section:%q/%q}, want {%q %q %q/%q}",
					i, md.Path, md.Def.Key, md.SectionKey, md.SectionLabel, wantPath, key, sec.Key, sec.Label)
			}
			i++
		}
	}
	if i != len(defs) {
		t.Errorf("EffectiveDefs returned %d defs, schema mounts %d", len(defs), i)
	}
}

// VisibleDefs with a has denying asset.view hides EVERY computer param
// (they all inherit the schema default) while identity params remain —
// the exact API-feed scenario the footgun would have broken.
func TestVisibleDefsDeniesAssetView(t *testing.T) {
	has := func(key string) bool { return key != "asset.view" }
	if got := SchemaComputer.VisibleDefs(has); len(got) != 0 {
		t.Errorf("computer VisibleDefs(deny asset.view) = %d defs, want 0 (first: %+v)", len(got), got[0])
	}
	idVisible := SchemaIdentity.VisibleDefs(has)
	if len(idVisible) != len(SchemaIdentity.EffectiveDefs()) {
		t.Errorf("identity VisibleDefs(deny asset.view) = %d defs, want all %d",
			len(idVisible), len(SchemaIdentity.EffectiveDefs()))
	}
}

// ...and vice versa: denying identity.view hides every identity param
// while computer params remain.
func TestVisibleDefsDeniesIdentityView(t *testing.T) {
	has := func(key string) bool { return key != "identity.view" }
	if got := SchemaIdentity.VisibleDefs(has); len(got) != 0 {
		t.Errorf("identity VisibleDefs(deny identity.view) = %d defs, want 0", len(got))
	}
	compVisible := SchemaComputer.VisibleDefs(has)
	if len(compVisible) != len(SchemaComputer.EffectiveDefs()) {
		t.Errorf("computer VisibleDefs(deny identity.view) = %d defs, want all %d",
			len(compVisible), len(SchemaComputer.EffectiveDefs()))
	}
}

// OrderedSchemas returns every registered schema exactly once, in the
// canonical display order the JSON API promises.
func TestOrderedSchemasCoversRegistry(t *testing.T) {
	ordered := OrderedSchemas()
	if len(ordered) != len(Schemas) {
		t.Fatalf("OrderedSchemas returned %d schemas, registry has %d", len(ordered), len(Schemas))
	}
	want := []string{"computer", "server", "printer", "desk", "identity"}
	for i, s := range ordered {
		if s.Subtype != want[i] {
			t.Errorf("OrderedSchemas[%d] = %q, want %q", i, s.Subtype, want[i])
		}
	}
}

// VisiblePaths is consistent with AllPaths: allowing everything returns
// every path; denying everything returns only paths with empty effective
// permission (if any).
func TestVisiblePathsConsistency(t *testing.T) {
	allowAll := func(string) bool { return true }
	visible := VisiblePaths(allowAll)
	all := AllPaths()
	if len(visible) != len(all) {
		t.Fatalf("VisiblePaths(allowAll) = %d paths, want %d", len(visible), len(all))
	}

	denyAll := func(string) bool { return false }
	visible = VisiblePaths(denyAll)
	for _, p := range visible {
		if EffectivePermission(p) != "" {
			t.Errorf("path %q should not be visible when all permissions are denied", p)
		}
	}
}
