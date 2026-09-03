package lists

import "testing"

// CP2 registration test for the Scripts tab list: locks the 5 field
// keys the module editor's DetailTableFrame render + column picker
// depend on.

func TestModuleScriptsRegistered(t *testing.T) {
	fs := FieldsFor(ListIDModuleScripts)
	if len(fs) != 5 {
		t.Fatalf("module-scripts fields = %d, want 5: %+v", len(fs), fs)
	}
	wantKeys := []string{"name", "language", "origin", "used_by", "actions"}
	for i, want := range wantKeys {
		if fs[i].Key != want {
			t.Errorf("field %d key = %q, want %q", i, fs[i].Key, want)
		}
	}
}

func TestModuleScriptsDefaultsAllVisible(t *testing.T) {
	keys := DefaultVisibleKeys(ListIDModuleScripts)
	for _, want := range []string{"name", "language", "origin", "used_by", "actions"} {
		if !containsField(keys, want) {
			t.Errorf("default-visible keys %q missing %q", keys, want)
		}
	}
}

func containsField(csv, key string) bool {
	for _, part := range splitCSV(csv) {
		if part == key {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
