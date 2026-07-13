package permissions

import "testing"

// (a) AllKeys() has no duplicates and every key parses as "domain.action".
func TestAllKeys_NoDuplicatesAndWellFormed(t *testing.T) {
	keys := AllKeys()
	if len(keys) == 0 {
		t.Fatal("AllKeys() returned no keys")
	}
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate key %q", k)
		}
		seen[k] = true

		domain, action := "", ""
		for i := 0; i < len(k); i++ {
			if k[i] == '.' {
				domain = k[:i]
				action = k[i+1:]
				break
			}
		}
		if domain == "" || action == "" {
			t.Errorf("key %q does not parse as domain.action", k)
		}
	}
}

// (b) ActionByKey behavior.
func TestActionByKey(t *testing.T) {
	a := ActionByKey("identity.update")
	if a == nil {
		t.Fatal("ActionByKey(identity.update) = nil, want non-nil")
	}
	if !a.Scoped {
		t.Error("identity.update: Scoped = false, want true")
	}

	c := ActionByKey("identity.create")
	if c == nil {
		t.Fatal("ActionByKey(identity.create) = nil, want non-nil")
	}
	if c.Scoped {
		t.Error("identity.create: Scoped = true, want false")
	}

	if ActionByKey("nope.nope") != nil {
		t.Error("ActionByKey(nope.nope) != nil, want nil")
	}
}

// (c) Full coverage both ways for each builtin template.
func TestTemplateGrants_FullCoverage(t *testing.T) {
	registryKeys := make(map[string]bool)
	for _, k := range AllKeys() {
		registryKeys[k] = true
	}

	for _, slug := range []string{"super_admin", "admin", "technician", "user"} {
		grants := TemplateGrants(slug)
		if grants == nil {
			t.Fatalf("TemplateGrants(%q) = nil, want non-nil", slug)
		}
		for k := range grants {
			if !registryKeys[k] {
				t.Errorf("%s: grant key %q not in registry", slug, k)
			}
		}
		for k := range registryKeys {
			if _, ok := grants[k]; !ok {
				t.Errorf("%s: registry key %q missing from template", slug, k)
			}
		}
	}

	if TemplateGrants("nonexistent") != nil {
		t.Error("TemplateGrants(nonexistent) != nil, want nil")
	}
}

// (d) scoped actions carry none|own|all; unscoped carry no|yes, in every template.
func TestTemplateGrants_ValueDomains(t *testing.T) {
	for _, slug := range []string{"super_admin", "admin", "technician", "user"} {
		grants := TemplateGrants(slug)
		for key, val := range grants {
			a := ActionByKey(key)
			if a == nil {
				t.Fatalf("%s: unknown key %q in template", slug, key)
			}
			if a.Scoped {
				if val != "none" && val != "own" && val != "all" {
					t.Errorf("%s: scoped key %q has invalid value %q", slug, key, val)
				}
			} else {
				if val != "no" && val != "yes" {
					t.Errorf("%s: unscoped key %q has invalid value %q", slug, key, val)
				}
			}
		}
	}
}

// (e) spot-assert specific spec values.
func TestTemplateGrants_SpotValues(t *testing.T) {
	if got := TemplateGrants("technician")["identity.delete"]; got != "no" {
		t.Errorf("technician identity.delete = %q, want no", got)
	}
	if got := TemplateGrants("user")["identity.update"]; got != "own" {
		t.Errorf("user identity.update = %q, want own", got)
	}
	if got := TemplateGrants("admin")["server_admin.tenant_switch"]; got != "no" {
		t.Errorf("admin server_admin.tenant_switch = %q, want no", got)
	}
	if got := TemplateGrants("super_admin")["identity.delete"]; got != "yes" {
		t.Errorf("super_admin identity.delete = %q, want yes", got)
	}
}

// TemplateGrants must return a fresh copy each call.
func TestTemplateGrants_ReturnsFreshCopy(t *testing.T) {
	g1 := TemplateGrants("admin")
	g1["identity.view"] = "MUTATED"
	g2 := TemplateGrants("admin")
	if g2["identity.view"] == "MUTATED" {
		t.Error("TemplateGrants: mutation leaked between calls, want fresh copy")
	}
}
