package lists

import "testing"

// Round-trip tests for the Users (identity) list registration. Mirrors
// lists_test.go's policy-catalog tests. Lock the invariants the column
// picker relies on:
//   1. The list is registered with a non-empty field set (i.e. Register
//      didn't panic at init() time — every field's Group must match a
//      registered FieldGroup.Key, and those keys must equal
//      catalog/params.SchemaIdentity's section keys).
//   2. DefaultVisibleKeys returns at least 4 keys (the schema's own
//      DefaultColumns has 7 — this just checks the wiring works).
//   3. FieldsByGroup produces one slice per group, none empty.

func TestUsersListRegistered(t *testing.T) {
	fs := FieldsFor(ListIDIdentities)
	if len(fs) == 0 {
		t.Fatalf("users list not registered")
	}
}

func TestUsersListDefaults(t *testing.T) {
	keys := DefaultVisibleKeys(ListIDIdentities)
	if keys == "" {
		t.Fatalf("no default-visible fields")
	}
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

func TestUsersFieldsByGroupCovers(t *testing.T) {
	groups := FieldsByGroup(ListIDIdentities)
	if len(groups) != len(GroupsFor(ListIDIdentities)) {
		t.Fatalf("groups slice length mismatch")
	}
	for i, g := range groups {
		if len(g) == 0 {
			t.Fatalf("group %s has no fields", GroupsFor(ListIDIdentities)[i].Key)
		}
	}
}

// TestUsersGroupKeysMatchIdentitySchema is the explicit regression test
// for the risk flagged in Task 15: Register panics if a FieldDef.Group
// doesn't match a registered FieldGroup.Key. If this test file can even
// load (package init ran without panicking) that risk didn't materialize,
// but this test additionally asserts the expected section keys are all
// present so a future schema edit that silently drops one is caught.
func TestUsersGroupKeysMatchIdentitySchema(t *testing.T) {
	want := []string{
		"identity", "organization", "contact", "location",
		"profile", "security", "preferences", "metadata",
	}
	groups := GroupsFor(ListIDIdentities)
	got := map[string]bool{}
	for _, g := range groups {
		got[g.Key] = true
	}
	for _, k := range want {
		if !got[k] {
			t.Fatalf("expected group key %q to be registered for %q, groups=%v", k, ListIDIdentities, groups)
		}
	}
}
