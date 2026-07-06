package params

import "testing"

func TestPathForAndResolveRoundTrip(t *testing.T) {
	p := PathFor("user", "email")
	if p != "user/identity/email" {
		t.Fatalf("got %q", p)
	}
	schema, sec, def, err := ResolvePath(p)
	if err != nil || schema.PathEntity != "user" || sec.Key != "identity" || def.Key != "email" {
		t.Fatalf("resolve failed: %v %v %v %v", schema, sec, def, err)
	}
	if PathFor("computer", "ram_mb") != "computer/hardware/ram_mb" {
		t.Fatal("computer path wrong")
	}
	if PathFor("computer", "email") != "" {
		t.Fatal("unmounted key must return empty")
	}
}

func TestResolvePathFailsClosed(t *testing.T) {
	for _, bad := range []string{"", "user", "user/identity", "ghost/identity/email", "user/ghost/email", "user/identity/ghost", "user/identity/email/extra"} {
		if _, _, _, err := ResolvePath(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestAllPathsUniqueAndComplete(t *testing.T) {
	paths := AllPaths()
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] {
			t.Fatalf("duplicate path %q", p)
		}
		seen[p] = true
	}
	for _, s := range Schemas {
		for _, sec := range s.Sections {
			for _, k := range sec.Params {
				if DefByKey(k) == nil {
					continue
				}
				if PathFor(s.PathEntity, k) == "" {
					t.Fatalf("no path for %s key %s", s.PathEntity, k)
				}
			}
		}
	}
}

func TestSchemaByPathEntity(t *testing.T) {
	if SchemaByPathEntity("user") != SchemaIdentity {
		t.Fatal("user must map to SchemaIdentity")
	}
	if SchemaByPathEntity("computer") != SchemaComputer {
		t.Fatal("computer must map to SchemaComputer")
	}
	if SchemaByPathEntity("ghost") != nil {
		t.Fatal("unknown entity must be nil")
	}
}

func TestTask4NewParamPathsResolve(t *testing.T) {
	for _, p := range []string{
		"computer/identity/description",
		"computer/identity/managed_by",
		"server/identity/managed_by",
		"printer/identity/description",
		"user/identity/initials",
	} {
		if _, _, _, err := ResolvePath(p); err != nil {
			t.Fatalf("expected %q to resolve: %v", p, err)
		}
	}
}
