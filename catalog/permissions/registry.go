// Package permissions is the pure-Go Pluris Policy permission registry:
// domains, actions, canonical "domain.action" keys, and the builtin role
// template matrices. See docs/history/specs/2026-07-08-pluris-policy-authz-design.md
// section "1. Permission registry".
//
// Zero non-stdlib imports. Registry is immutable after init().
package permissions

// Action is one grantable action within a Domain. Key is the action's
// local name ("update"); the canonical key is "domain.action".
type Action struct {
	Key         string // "update" -- full key is domain.action
	Label       string // "Update account"
	Description string // one line, shown as row tooltip in the matrix
	Scoped      bool   // true -> None/Own/All; false -> yes/no
}

// Domain is a group of related Actions ("identity", "asset", ...).
type Domain struct {
	Key     string // "identity"
	Label   string // "Identity Management"
	Actions []Action
}

// domains holds the v1 registry in registration order. Registration order
// is the matrix display order.
var domains = []Domain{
	{
		Key:   "identity",
		Label: "Identity Management",
		Actions: []Action{
			{Key: "view", Label: "View accounts", Description: "View identity accounts", Scoped: true},
			{Key: "update", Label: "Update account", Description: "Update identity account fields", Scoped: true},
			{Key: "create", Label: "Create account", Description: "Create a new identity account", Scoped: false},
			{Key: "delete", Label: "Delete account", Description: "Delete an identity account", Scoped: false},
			{Key: "assign_roles", Label: "Assign roles", Description: "Assign or remove roles on an identity", Scoped: false},
			{Key: "assign_groups", Label: "Manage group membership", Description: "Manage an identity's group membership", Scoped: false},
		},
	},
	{
		Key:   "asset",
		Label: "Asset Management",
		Actions: []Action{
			{Key: "view", Label: "View assets", Description: "View assets", Scoped: true},
			{Key: "update", Label: "Update asset", Description: "Update asset fields", Scoped: true},
			{Key: "create", Label: "Create asset", Description: "Create a new asset", Scoped: false},
			{Key: "delete", Label: "Delete asset", Description: "Delete an asset", Scoped: false},
			{Key: "set_owner", Label: "Set asset owner", Description: "Set the owning identity of an asset", Scoped: false},
			{Key: "manage_groups", Label: "Manage asset group membership", Description: "Manage an asset's group membership", Scoped: false},
		},
	},
	{
		Key:   "endpoint_policy",
		Label: "Endpoint Policy",
		Actions: []Action{
			{Key: "view", Label: "View endpoint policy", Description: "View endpoint policy", Scoped: false},
			{Key: "manage_catalog", Label: "Manage policy catalog", Description: "Manage the endpoint policy catalog", Scoped: false},
			{Key: "manage_config_groups", Label: "Manage configuration groups", Description: "Manage endpoint configuration groups", Scoped: false},
			{Key: "manage_dependency_groups", Label: "Manage dependency groups", Description: "Manage endpoint dependency groups", Scoped: false},
			{Key: "manage_modules", Label: "Manage policy modules", Description: "Manage endpoint policy modules", Scoped: false},
			{Key: "assign_policies", Label: "Assign policies to targets", Description: "Assign endpoint policies to targets", Scoped: false},
		},
	},
	{
		Key:   "group",
		Label: "Group Management",
		Actions: []Action{
			{Key: "view", Label: "View groups", Description: "View AD-style asset/identity groups", Scoped: true},
			{Key: "create", Label: "Create group", Description: "Create a new group", Scoped: false},
			{Key: "update", Label: "Update group", Description: "Update group fields (description, member kind, membership mode, rules)", Scoped: false},
			{Key: "delete", Label: "Delete group", Description: "Delete a group", Scoped: false},
			{Key: "manage_members", Label: "Manage group members", Description: "Add or remove direct members and recalculate dynamic membership", Scoped: false},
		},
	},
	{
		Key:   "console_access",
		Label: "Console Access",
		Actions: []Action{
			{Key: "view_roles", Label: "View roles and permissions", Description: "View roles and permissions", Scoped: false},
			{Key: "manage_role_assignments", Label: "Assign/remove roles on users", Description: "Assign or remove roles on users", Scoped: false},
			{Key: "manage_permissions", Label: "Create/edit/delete custom roles", Description: "Create, edit, or delete custom roles", Scoped: false},
		},
	},
	{
		Key:   "server_admin",
		Label: "Server Administration",
		Actions: []Action{
			{Key: "access", Label: "Access server administration", Description: "Access server administration", Scoped: false},
			{Key: "tenant_switch", Label: "Switch tenants", Description: "Switch tenants", Scoped: false},
		},
	},
}

var (
	actionByKey map[string]*Action
	allKeys     []string
)

func init() {
	actionByKey = make(map[string]*Action)
	allKeys = make([]string, 0)
	for di := range domains {
		d := &domains[di]
		for ai := range d.Actions {
			a := &d.Actions[ai]
			full := d.Key + "." + a.Key
			if _, dup := actionByKey[full]; dup {
				panic("permissions: duplicate key " + full)
			}
			actionByKey[full] = a
			allKeys = append(allKeys, full)
		}
	}
}

// All returns the registered domains in registration order (matrix display
// order).
func All() []Domain {
	return domains
}

// ActionByKey returns the Action for a canonical "domain.action" key, or
// nil if unknown.
func ActionByKey(full string) *Action {
	return actionByKey[full]
}

// AllKeys returns every canonical "domain.action" key.
func AllKeys() []string {
	out := make([]string, len(allKeys))
	copy(out, allKeys)
	return out
}

// builtinSlugs are the four builtin role slugs, in privilege order
// (highest first). Single source of truth -- consumers that previously
// kept their own copy of this list (pkg/services/roles.go builtin seeding,
// pkg/authz.EnsureBuiltinGrants) call BuiltinSlugs() instead.
var builtinSlugs = []string{"super_admin", "admin", "technician", "user"}

// BuiltinSlugs returns the four builtin role slugs in privilege order.
func BuiltinSlugs() []string {
	out := make([]string, len(builtinSlugs))
	copy(out, builtinSlugs)
	return out
}
