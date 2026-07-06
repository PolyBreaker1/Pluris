package params

import "testing"

func TestAllSchemaParamsHaveDefinitions(t *testing.T) {
	for subtype, schema := range Schemas {
		for _, sec := range schema.Sections {
			for _, key := range sec.Params {
				if _, ok := Definitions[key]; !ok {
					t.Errorf("schema %q section %q references undefined param %q", subtype, sec.Key, key)
				}
			}
		}
		for _, key := range schema.DefaultColumns {
			if _, ok := Definitions[key]; !ok {
				t.Errorf("schema %q default column references undefined param %q", subtype, key)
			}
		}
	}
}

func TestAllLinksHaveDefinitions(t *testing.T) {
	for _, link := range allLinks {
		if _, ok := Definitions[link.ParamKey]; !ok {
			t.Errorf("link %q references undefined param key", link.ParamKey)
		}
		def := Definitions[link.ParamKey]
		if def.Type != TypeLink {
			t.Errorf("link %q points to param with type %q, expected %q", link.ParamKey, def.Type, TypeLink)
		}
	}
}

func TestNoDuplicateDefinitionKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range allDefs {
		if seen[d.Key] {
			t.Errorf("duplicate definition key %q", d.Key)
		}
		seen[d.Key] = true
	}
}

func TestSharedWithFindsCorrectSubtypes(t *testing.T) {
	// ram_mb is mounted by both computer and server, not printer or desk.
	shared := SharedWith("ram_mb")
	has := map[string]bool{}
	for _, s := range shared {
		has[s] = true
	}
	if !has["computer"] {
		t.Error("expected computer to have ram_mb")
	}
	if !has["server"] {
		t.Error("expected server to have ram_mb")
	}
	if has["printer"] {
		t.Error("printer should not have ram_mb")
	}
	if has["desk"] {
		t.Error("desk should not have ram_mb")
	}
}

func TestSchemaHasParam(t *testing.T) {
	if !SchemaComputer.HasParam("hostname") {
		t.Error("computer schema should have hostname")
	}
	if SchemaComputer.HasParam("printer_ip") {
		t.Error("computer schema should NOT have printer_ip")
	}
	if !SchemaPrinter.HasParam("printer_ip") {
		t.Error("printer schema should have printer_ip")
	}
}
