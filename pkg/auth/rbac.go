package auth

import (
	"strings"

	"github.com/pluris/pluris/pkg/authz"
)

// routePermissionKey is the locked route-prefix -> Pluris Policy
// permission-key map. It replaces the old hardcoded role matrix: instead
// of asking "does this role's row say yes for this route", enforcement
// now asks "does this session's grants include the permission key this
// route requires". See
// docs/history/specs/2026-07-08-pluris-policy-authz-design.md
// section "3. Enforcement" for the design rationale.
//
// A value of "" means the route is open to any authenticated session
// (it carries no gated permission -- this mirrors the old matrix's
// all-roles-true rows for "/", "/profiles", "/wine", "/packages", and
// "/preferences").
//
// "/policy/modules" and "/scripts" both map to
// "endpoint_policy.manage_modules": in the old matrix both were
// technician-and-up (denied to plain users), and scripts are
// module-adjacent automation -- a script is effectively an unpackaged
// policy module lifecycle phase, so the same permission gates both.
var routePermissionKey = map[string]string{
	"/":       "",
	"/users":  "identity.view",
	"/assets": "asset.view",
	// "/groups" (Task 6.2): the standardized AD-style group list/create/
	// detail pages. Coarse gate is group.view; individual mutation routes
	// (create/delete/members/rules) call requirePermission with the more
	// specific group.* key inside the handler, same two-layer pattern as
	// "/policy/groups" + endpoint_policy.manage_config_groups.
	"/groups":            "group.view",
	"/policy":            "endpoint_policy.view",
	"/policy/pluris":     "console_access.view_roles",
	"/policy/modules":    "endpoint_policy.manage_modules",
	"/scripts":           "endpoint_policy.manage_modules",
	"/profiles":          "",
	"/wine":              "",
	"/packages":          "",
	"/preferences":       "",
	"/server-admin":      "server_admin.access",
	"/server-admin/data": "server_admin.manage_data",
	"/tenant-switch":     "server_admin.tenant_switch",

	// "/api/config-groups" (Task 5.2): the Configuration Group detail
	// page's General-tab inline-edit endpoint. Gated on the same key the
	// mutating /policy/groups handlers check (manage_config_groups) --
	// like "/api/modules" below, this API path doesn't share the
	// "/policy" prefix, so without this entry it would fall through to
	// the open "/" default.
	"/api/config-groups": "endpoint_policy.manage_config_groups",

	// "/api/modules" (Task 4.3): the module editor's field-update and
	// script-save endpoints. Gated on the same key as the
	// "/policy/modules" pages they serve, for parity -- these API paths
	// do NOT share that prefix, so without this entry they'd fall through
	// to the open "/" default. The per-module ModuleCanView/Edit/Admin
	// checks inside the handlers remain the fine-grained authorization;
	// this route key is the coarse defense-in-depth layer on top.
	"/api/modules": "endpoint_policy.manage_modules",

	// "/api/params" is deliberately open to any authenticated session
	// (Task 1.2): the parameter-registry feed is per-grant filtered
	// inside the handler (params.VisibleDefs), so a caller only ever
	// receives the subtree their own grants allow — gating the route on
	// a specific permission would just hide identity params from
	// identity-only users and asset params from asset-only users.
	"/api/params": "",

	// "/api/scripts" is the Scripts-library feed the condition builder's
	// script picker reads. Open to any authenticated session for the
	// same reason "/api/params" is: it returns only names/ids, and the
	// pages that consume it are themselves permission-gated. Returns []
	// until the Scripts library ships.
	"/api/scripts": "",
}

// RoutePermissionKey returns the Pluris Policy permission key that gates
// path, using longest-registered-prefix matching (so "/policy/modules"
// and "/policy/pluris" both win over the broader "/policy" entry, and
// "/scriptsfoo" still matches the "/scripts" entry -- copied from the old
// CanAccess's matching loop). Returns "" both when the longest matching
// prefix is registered as open, and when no registered prefix matches at
// all (e.g. a path that doesn't start with "/") -- deny-by-default for
// that latter case is enforced by CanAccessGrants's caller via
// RequireRole, not here.
func RoutePermissionKey(path string) string {
	bestPrefix := ""
	bestKey := ""
	found := false
	for prefix, key := range routePermissionKey {
		if strings.HasPrefix(path, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestKey = key
			found = true
		}
	}
	if !found {
		return ""
	}
	return bestKey
}

// CanAccessGrants reports whether grants g may access the route
// beginning with path. Semantics:
//   - RoutePermissionKey(path) == "" -> always true (open route, or no
//     registered prefix matches at all).
//   - otherwise, access is granted when g carries the bypass grant, or
//     when the key's stored value is a non-deny grant. Grants.Can
//     already implements exactly this for both scoped keys ("own"/"all"
//     count as allowed) and unscoped keys ("yes" counts as allowed), so
//     a single call covers both cases.
func CanAccessGrants(g authz.Grants, path string) bool {
	key := RoutePermissionKey(path)
	if key == "" {
		return true
	}
	return g.Can(key)
}
