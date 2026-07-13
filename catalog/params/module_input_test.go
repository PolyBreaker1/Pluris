package params

import "testing"

// Empty schema (module declares no inputs, the common case) -> nil,
// no error.
func TestModuleInputDefsEmptySchema(t *testing.T) {
	defs, err := ModuleInputDefs("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defs != nil {
		t.Fatalf("defs = %v, want nil", defs)
	}
}

// Full round trip: string/number/boolean/enum types, required flag,
// defaults, title-derived labels, key-derived label fallback, and
// "module/input/<key>" paths.
func TestModuleInputDefsParsesTypesEnumRequiredDefaults(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"hostname": {"type": "string", "title": "Hostname", "default": "localhost"},
			"port": {"type": "number", "title": "Port", "default": 22},
			"strict": {"type": "boolean", "title": "Strict mode", "default": true},
			"loglevel": {"type": "string", "enum": ["debug", "info", "error"], "title": "Log level"},
			"nolabel": {"type": "string"}
		},
		"required": ["hostname", "port"]
	}`
	defs, err := ModuleInputDefs(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 5 {
		t.Fatalf("got %d defs, want 5", len(defs))
	}

	byKey := map[string]ModuleInputDef{}
	for _, d := range defs {
		byKey[d.Def.Key] = d
	}

	host := byKey["hostname"]
	if host.Path != "module/input/hostname" {
		t.Errorf("hostname path = %q, want module/input/hostname", host.Path)
	}
	if host.Def.Type != TypeString || host.Def.Label != "Hostname" {
		t.Errorf("hostname = {Type:%q Label:%q}, want {string Hostname}", host.Def.Type, host.Def.Label)
	}
	if !host.Required {
		t.Error("hostname should be required")
	}
	if host.Default != `"localhost"` {
		t.Errorf("hostname default = %q, want \"localhost\"", host.Default)
	}
	if host.Def.Permission != "" {
		t.Errorf("hostname permission = %q, want empty (module-level gating only)", host.Def.Permission)
	}

	port := byKey["port"]
	if port.Def.Type != TypeFloat {
		t.Errorf("port type = %q, want float", port.Def.Type)
	}
	if !port.Required {
		t.Error("port should be required")
	}
	if port.Default != "22" {
		t.Errorf("port default = %q, want 22", port.Default)
	}

	strict := byKey["strict"]
	if strict.Def.Type != TypeBool {
		t.Errorf("strict type = %q, want bool", strict.Def.Type)
	}
	if strict.Required {
		t.Error("strict should not be required")
	}

	level := byKey["loglevel"]
	if level.Def.Type != TypeEnum {
		t.Errorf("loglevel type = %q, want enum", level.Def.Type)
	}
	wantEnum := []string{"debug", "info", "error"}
	if len(level.Def.EnumValues) != len(wantEnum) {
		t.Fatalf("loglevel enum = %v, want %v", level.Def.EnumValues, wantEnum)
	}
	for i, v := range wantEnum {
		if level.Def.EnumValues[i] != v {
			t.Errorf("loglevel enum[%d] = %q, want %q", i, level.Def.EnumValues[i], v)
		}
	}

	nolabel := byKey["nolabel"]
	if nolabel.Def.Label != "nolabel" {
		t.Errorf("nolabel label = %q, want key fallback %q", nolabel.Def.Label, "nolabel")
	}
	if nolabel.Default != "" {
		t.Errorf("nolabel default = %q, want empty", nolabel.Default)
	}

	// Deterministic order: sorted by key regardless of source JSON order.
	wantOrder := []string{"hostname", "loglevel", "nolabel", "port", "strict"}
	for i, d := range defs {
		if d.Def.Key != wantOrder[i] {
			t.Errorf("defs[%d].Key = %q, want %q (sorted order)", i, d.Def.Key, wantOrder[i])
		}
	}
}

// Malformed JSON must return an error, never panic.
func TestModuleInputDefsMalformedSchemaErrorsNotPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ModuleInputDefs panicked on malformed input: %v", r)
		}
	}()
	cases := []string{
		`{not valid json`,
		`"just a string"`,
		`42`,
		`["array", "not object"]`,
	}
	for _, c := range cases {
		if _, err := ModuleInputDefs(c); err == nil {
			t.Errorf("ModuleInputDefs(%q) = nil error, want error", c)
		}
	}
}

// An unrecognized "type" value degrades gracefully to TypeString rather
// than erroring the whole module's inputs over one unknown property.
func TestModuleInputDefsUnknownTypeFallsBackToString(t *testing.T) {
	defs, err := ModuleInputDefs(`{"type":"object","properties":{"weird":{"type":"frobnicate"}}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 1 || defs[0].Def.Type != TypeString {
		t.Fatalf("got %+v, want single TypeString def", defs)
	}
}
