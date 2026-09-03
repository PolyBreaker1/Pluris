package templates

import (
	"testing"

	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
)

// findMenuItem locates a top-level Menu item by Key. Fails the test if absent.
func findMenuItem(t *testing.T, key string) MenuItem {
	t.Helper()
	for _, item := range Menu {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("menu item %q not found in Menu", key)
	return MenuItem{}
}

func TestDataManagementMenuPermission(t *testing.T) {
	serverAdmin := findMenuItem(t, "server-admin")
	if len(serverAdmin.Children) == 0 || serverAdmin.Children[0].Key != "server-admin-data" {
		t.Fatalf("Data Management must be the first Server Administration child: %+v", serverAdmin.Children)
	}
	child := serverAdmin.Children[0]
	if MenuItemVisible(&auth.UserSession{Grants: authz.Grants{}}, child) {
		t.Fatal("Data Management visible without grant")
	}
	if !MenuItemVisible(&auth.UserSession{Grants: authz.Grants{"server_admin.manage_data": "yes"}}, child) {
		t.Fatal("Data Management hidden with grant")
	}
}

// TestSidebarActiveMatching is table-driven over every `active` value
// actually passed to Layout across the app (see enumeration in the task
// brief), asserting which top-level item is active, whether its children
// expand, and which child (if any) is active.
func TestSidebarActiveMatching(t *testing.T) {
	tests := []struct {
		name         string
		topLevelKey  string // top-level Menu item to check isItemActive/expandChildren against
		active       string
		wantActive   bool   // isItemActive(topLevelItem, active)
		wantExpand   bool   // expandChildren(topLevelItem, active)
		wantChildKey string // if non-empty, the child key expected to satisfy isItemActive
	}{
		// --- Policy Modules bug: computed active "policy-modules-"+view ---
		{"policy-modules-library expands Policy, activates policy-modules child", "policy", "policy-modules-library", true, true, "policy-modules"},
		{"policy-modules-defaults expands Policy, activates policy-modules child", "policy", "policy-modules-defaults", true, true, "policy-modules"},
		{"policy-modules-sources expands Policy, activates policy-modules child", "policy", "policy-modules-sources", true, true, "policy-modules"},

		// --- Existing exact-match Policy children still work ---
		{"policy-catalog exact child match", "policy", "policy-catalog", true, true, "policy-catalog"},
		{"policy-groups exact child match", "policy", "policy-groups", true, true, "policy-groups"},
		{"policy-dependency-groups exact child match", "policy", "policy-dependency-groups", true, true, "policy-dependency-groups"},
		{"policy-pluris exact child match", "policy", "policy-pluris", true, true, "policy-pluris"},

		// --- AD-style groups sidebar children (Task 6.2) ---
		// /groups?kind=identity pages pass active "users-groups": Users
		// expands and its User Groups child lights up. /groups?kind=asset
		// (and asset-kind group detail pages) pass "assets-groups".
		{"users-groups expands Users, activates User Groups child", "users", "users-groups", true, true, "users-groups"},
		{"assets-groups expands Assets, activates Groups child", "assets", "assets-groups", true, true, "assets-groups"},
		// Cross-surface negatives: users-groups must not light the Assets
		// tree and vice versa.
		{"users-groups does not activate Assets", "assets", "users-groups", false, false, ""},
		{"assets-groups does not activate Users", "users", "assets-groups", false, false, ""},

		// --- Unrelated top-level items behave as today ---
		{"assets-computers exact child match", "assets", "assets-computers", true, true, "assets-computers"},
		{"packages-cycles exact child match", "packages", "packages-cycles", true, true, "packages-cycles"},
		// expandChildren returns true here (item.Key == active fallback);
		// Users gained a User Groups child in Task 6.2 so its children DO
		// render on its own page now.
		{"users plain leaf key", "users", "users", true, true, ""},
		{"dashboard plain leaf key", "dashboard", "dashboard", true, true, ""},

		// --- Negative / non-collision cases ---
		// active "policy-dependency-groups" must still only mark its own exact
		// child active; the sibling "policy-groups" child must NOT light up.
		{"policy-dependency-groups activates only its own child", "policy", "policy-dependency-groups", true, true, "policy-dependency-groups"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := findMenuItem(t, tt.topLevelKey)

			if got := isItemActive(item, tt.active); got != tt.wantActive {
				t.Errorf("isItemActive(%q item, active=%q) = %v, want %v", tt.topLevelKey, tt.active, got, tt.wantActive)
			}
			if got := expandChildren(item, tt.active); got != tt.wantExpand {
				t.Errorf("expandChildren(%q item, active=%q) = %v, want %v", tt.topLevelKey, tt.active, got, tt.wantExpand)
			}
			if tt.wantChildKey != "" {
				var child MenuItem
				found := false
				for _, c := range item.Children {
					if c.Key == tt.wantChildKey {
						child, found = c, true
						break
					}
				}
				if !found {
					t.Fatalf("child key %q not found under %q", tt.wantChildKey, tt.topLevelKey)
				}
				if !isItemActive(child, tt.active) {
					t.Errorf("isItemActive(child %q, active=%q) = false, want true", tt.wantChildKey, tt.active)
				}
			}
		})
	}
}

// TestPolicyDependencyGroupsDoesNotActivatePolicyGroupsSibling is the
// explicit negative case from the brief: active "policy-dependency-groups"
// must not mark the sibling child "policy-groups" as active, since
// "policy-dependency-groups" is not a '-'-boundary extension of
// "policy-groups" ("policy-dependency-groups" does not start with
// "policy-groups-").
func TestPolicyDependencyGroupsDoesNotActivatePolicyGroupsSibling(t *testing.T) {
	policy := findMenuItem(t, "policy")
	var policyGroupsChild MenuItem
	found := false
	for _, c := range policy.Children {
		if c.Key == "policy-groups" {
			policyGroupsChild, found = c, true
			break
		}
	}
	if !found {
		t.Fatal("policy-groups child not found under policy")
	}
	if isItemActive(policyGroupsChild, "policy-dependency-groups") {
		t.Error(`isItemActive(policy-groups child, active="policy-dependency-groups") = true, want false`)
	}
}

// TestKeyMatchesNoUnrelatedCollisions explicitly asserts the two
// near-miss cases the brief calls out: "policy-groups" vs
// "policy-dependency-groups" must not cross-match in either direction,
// since "policy-dependency-groups" does not start with "policy-groups-"
// and "policy-groups" does not start with "policy-dependency-groups-".
func TestKeyMatchesNoUnrelatedCollisions(t *testing.T) {
	if keyMatches("policy-groups", "policy-dependency-groups") {
		t.Error(`keyMatches("policy-groups", "policy-dependency-groups") = true, want false`)
	}
	if keyMatches("policy-dependency-groups", "policy-groups") {
		t.Error(`keyMatches("policy-dependency-groups", "policy-groups") = true, want false`)
	}
	// Sanity: the intended match direction works.
	if !keyMatches("policy-modules", "policy-modules-library") {
		t.Error(`keyMatches("policy-modules", "policy-modules-library") = false, want true`)
	}
	if !keyMatches("policy", "policy-modules-library") {
		t.Error(`keyMatches("policy", "policy-modules-library") = false, want true`)
	}
}
