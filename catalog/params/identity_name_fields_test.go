package params_test

import (
	"testing"

	"github.com/pluris/pluris/catalog/params"
)

// TestIdentityNameFieldsRegistered verifies that the AD-style First
// name / Last name fields (given_name / surname) are mounted on the
// identity schema alongside display_name, and resolve to the expected
// canonical paths (mirrors TestDependencyFactsRegistered's shape).
func TestIdentityNameFieldsRegistered(t *testing.T) {
	for _, key := range []string{"given_name", "surname"} {
		if d := params.DefByKey(key); d == nil {
			t.Fatalf("param %q not registered", key)
		}
	}
	if got := params.PathFor("user", "given_name"); got != "user/identity/given_name" {
		t.Fatalf("user given_name path = %q", got)
	}
	if got := params.PathFor("user", "surname"); got != "user/identity/surname" {
		t.Fatalf("user surname path = %q", got)
	}
}
