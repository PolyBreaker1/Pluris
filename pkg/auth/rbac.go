package auth

import (
	"strings"

	"github.com/pluris/pluris/catalog/identities"
)

// permission mirrors the locked role permission matrix in
// docs/UX_INVARIANTS.md (§ "Role permission matrix (v1, locked)"). It is
// intentionally coarse (route-prefix level): row-level self-scoping
// (e.g. the user role seeing only their own identity/assets) is
// applied inside the service layer using the session identity, not here.
// technician mirrors admin except /server-admin.
var permission = map[string]map[string]bool{
	"/":               {"super_admin": true, "admin": true, "technician": true, "user": true},
	"/users":          {"super_admin": true, "admin": true, "technician": true, "user": true},
	"/assets":         {"super_admin": true, "admin": true, "technician": true, "user": true},
	"/policy":         {"super_admin": true, "admin": true, "technician": true, "user": true},
	"/profiles":       {"super_admin": true, "admin": true, "technician": true, "user": true},
	"/scripts":        {"super_admin": true, "admin": true, "technician": true, "user": false},
	"/policy/modules": {"super_admin": true, "admin": true, "technician": true, "user": false},
	"/wine":           {"super_admin": true, "admin": true, "technician": true, "user": true},
	"/packages":       {"super_admin": true, "admin": true, "technician": true, "user": true},
	"/server-admin":   {"super_admin": true, "admin": true, "technician": false, "user": false},
	"/preferences":    {"super_admin": true, "admin": true, "technician": true, "user": true},
	"/tenant-switch":  {"super_admin": true, "admin": false, "technician": false, "user": false},
}

// CanAccess reports whether role may access the route beginning with
// path. It matches the longest registered prefix (so "/policy/modules"
// overrides the broader "/policy" entry for admin/user).
func CanAccess(role identities.Role, path string) bool {
	bestPrefix := ""
	for prefix := range permission {
		if strings.HasPrefix(path, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
		}
	}
	if bestPrefix == "" {
		return false
	}
	allowed, ok := permission[bestPrefix][string(role)]
	return ok && allowed
}
