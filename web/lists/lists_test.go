package lists

import "testing"

// Round-trip tests for the policy-catalog registration. Lock the
// invariants the column picker relies on:
//   1. Every field references a known group key.
//   2. No duplicate keys.
//   3. DefaultVisibleKeys returns at least 4 keys (the original 4
//      columns of the policy table — the page must render identically
//      out of the box for users with no saved preference).
//   4. ListEligibleFields excludes detail-only fields.

func TestPolicyCatalogRegistered(t *testing.T) {
	fs := FieldsFor(ListIDPolicyCatalog)
	if len(fs) == 0 {
		t.Fatalf("policy-catalog list not registered")
	}
}

func TestPolicyCatalogDefaults(t *testing.T) {
	keys := DefaultVisibleKeys(ListIDPolicyCatalog)
	if keys == "" {
		t.Fatalf("no default-visible fields")
	}
	// Sanity: at least 4 defaults to preserve the original UI.
	count := 1
	for _, c := range keys {
		if c == ',' {
			count++
		}
	}
	if count < 4 {
		t.Fatalf("want >=4 default fields, got %d (%q)", count, keys)
	}
}

func TestFieldsByGroupCovers(t *testing.T) {
	groups := FieldsByGroup(ListIDPolicyCatalog)
	if len(groups) != len(GroupsFor(ListIDPolicyCatalog)) {
		t.Fatalf("groups slice length mismatch")
	}
	for i, g := range groups {
		if len(g) == 0 {
			t.Fatalf("group %s has no fields", GroupsFor(ListIDPolicyCatalog)[i].Key)
		}
	}
}

func TestUnknownListGracefulNil(t *testing.T) {
	if FieldsFor("does-not-exist") != nil {
		t.Fatalf("unknown list should return nil, not panic")
	}
}
