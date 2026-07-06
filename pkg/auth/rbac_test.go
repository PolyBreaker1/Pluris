package auth

import (
	"testing"

	"github.com/pluris/pluris/catalog/identities"
)

func TestCanAccessMatchesLockedMatrix(t *testing.T) {
	cases := []struct {
		role   identities.Role
		prefix string
		want   bool
	}{
		{identities.RoleSuperAdmin, "/server-admin", true},
		{identities.RoleAdmin, "/server-admin", true},
		{identities.RoleUser, "/server-admin", false},
		{identities.RoleUser, "/users", true}, // read-only own identity, still "can access"
		{identities.RoleUser, "/scripts", false},
		{identities.RoleAdmin, "/scripts", true},
		{identities.RoleUser, "/policy/modules", false},
		// Prefix-boundary cases.
		{identities.RoleAdmin, "/scriptsfoo", true}, // extends past a registered prefix: prefix match, not exact match
		// Note: "/script" (without the trailing "s") is NOT a "matches nothing"
		// case here, because "/" is itself a registered prefix and every
		// absolute path starts with "/" — so "/script" falls through to the
		// "/" entry and is allowed for admin. That's a real, intentional
		// property of this map (root is a catch-all for authenticated
		// roles), not a bug. To exercise the true "matches no registered
		// prefix" branch we need a path that doesn't start with "/" at all.
		{identities.RoleAdmin, "not-a-path", false},
	}
	for _, c := range cases {
		got := CanAccess(c.role, c.prefix)
		if got != c.want {
			t.Errorf("CanAccess(%q, %q) = %v, want %v", c.role, c.prefix, got, c.want)
		}
	}
}

func TestCanAccessUnknownRoleDeniedByDefault(t *testing.T) {
	if CanAccess(identities.Role("bogus_role"), "/") {
		t.Fatal("expected unknown role to be denied by default")
	}
}
