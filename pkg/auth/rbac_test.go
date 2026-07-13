package auth

import (
	"testing"

	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/pkg/authz"
)

// grantsFor builds the authz.Grants for a builtin role template slug,
// mirroring how RequireAuth (middleware.go) constructs session grants:
// super_admin carries the bypass marker instead of the template matrix.
func grantsFor(t *testing.T, slug string) authz.Grants {
	t.Helper()
	if slug == "super_admin" {
		return authz.Grants{authz.BypassKey: "yes"}
	}
	tmpl := permissions.TemplateGrants(slug)
	if tmpl == nil {
		t.Fatalf("permissions.TemplateGrants(%q) = nil, want builtin template", slug)
	}
	return authz.Grants(tmpl)
}

func TestCanAccessGrantsMatchesLockedRouteMap(t *testing.T) {
	cases := []struct {
		role  string
		route string
		want  bool
	}{
		// user: can reach the open routes plus its own-scope view routes.
		{"user", "/", true},
		{"user", "/users", true},
		{"user", "/assets", true},
		{"user", "/policy", true},
		{"user", "/wine", true},
		{"user", "/packages", true},
		{"user", "/preferences", true},
		{"user", "/profiles", true},
		// user: cannot reach manage/admin-only routes.
		{"user", "/scripts", false},
		{"user", "/policy/modules", false},
		{"user", "/server-admin", false},
		{"user", "/tenant-switch", false},
		{"user", "/policy/pluris", false},

		// technician: everything except /server-admin and /tenant-switch.
		{"technician", "/", true},
		{"technician", "/users", true},
		{"technician", "/assets", true},
		{"technician", "/policy", true},
		{"technician", "/policy/pluris", true},
		{"technician", "/policy/modules", true},
		{"technician", "/scripts", true},
		{"technician", "/wine", true},
		{"technician", "/packages", true},
		{"technician", "/preferences", true},
		{"technician", "/profiles", true},
		{"technician", "/server-admin", false},
		{"technician", "/tenant-switch", false},

		// admin: everything except /tenant-switch.
		{"admin", "/", true},
		{"admin", "/users", true},
		{"admin", "/assets", true},
		{"admin", "/policy", true},
		{"admin", "/policy/pluris", true},
		{"admin", "/policy/modules", true},
		{"admin", "/scripts", true},
		{"admin", "/wine", true},
		{"admin", "/packages", true},
		{"admin", "/preferences", true},
		{"admin", "/profiles", true},
		{"admin", "/server-admin", true},
		{"admin", "/tenant-switch", false},

		// super_admin: everything (bypass).
		{"super_admin", "/", true},
		{"super_admin", "/users", true},
		{"super_admin", "/assets", true},
		{"super_admin", "/policy", true},
		{"super_admin", "/policy/pluris", true},
		{"super_admin", "/policy/modules", true},
		{"super_admin", "/scripts", true},
		{"super_admin", "/wine", true},
		{"super_admin", "/packages", true},
		{"super_admin", "/preferences", true},
		{"super_admin", "/profiles", true},
		{"super_admin", "/server-admin", true},
		{"super_admin", "/tenant-switch", true},
	}

	for _, c := range cases {
		g := grantsFor(t, c.role)
		got := CanAccessGrants(g, c.route)
		if got != c.want {
			t.Errorf("CanAccessGrants(%s grants, %q) = %v, want %v", c.role, c.route, got, c.want)
		}
	}
}

// TestOpenRoutesRequireNoPermissionKey asserts the routes that were "all
// roles" rows in the old matrix are wired as key == "" (open to any
// authenticated session) in the new route map, per the locked mapping.
func TestOpenRoutesRequireNoPermissionKey(t *testing.T) {
	openRoutes := []string{"/", "/wine", "/packages", "/preferences", "/profiles"}
	for _, r := range openRoutes {
		if key := RoutePermissionKey(r); key != "" {
			t.Errorf("RoutePermissionKey(%q) = %q, want \"\" (open route)", r, key)
		}
	}
}

func TestCanAccessGrantsDeniesWithNoGrants(t *testing.T) {
	var g authz.Grants
	if !CanAccessGrants(g, "/") {
		t.Error("expected open route \"/\" to be allowed even with nil grants")
	}
	if CanAccessGrants(g, "/users") {
		t.Error("expected nil grants to deny a scoped route")
	}
	if CanAccessGrants(g, "/server-admin") {
		t.Error("expected nil grants to deny an unscoped route")
	}
}

func TestRoutePermissionKeyLongestPrefixMatch(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/", ""},
		{"/users", "identity.view"},
		{"/assets", "asset.view"},
		{"/assets/computers", "asset.view"},
		// Task 6.2: the AD-style group pages (list/create/detail + the
		// query-string sidebar hrefs) all gate on group.view at the route
		// level; finer group.* keys are handler-side.
		{"/groups", "group.view"},
		{"/groups?kind=identity", "group.view"},
		{"/groups/5/rules", "group.view"},
		{"/policy", "endpoint_policy.view"},
		{"/policy/catalog", "endpoint_policy.view"},
		{"/policy/pluris", "console_access.view_roles"},
		{"/policy/modules", "endpoint_policy.manage_modules"},
		{"/scripts", "endpoint_policy.manage_modules"},
		{"/profiles", ""},
		{"/wine", ""},
		{"/packages", ""},
		{"/preferences", ""},
		{"/server-admin", "server_admin.access"},
		{"/tenant-switch", "server_admin.tenant_switch"},
		{"not-a-path", ""},
	}
	for _, c := range cases {
		got := RoutePermissionKey(c.path)
		if got != c.want {
			t.Errorf("RoutePermissionKey(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
