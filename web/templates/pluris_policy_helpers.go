package templates

import (
	"strconv"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/authz"
)

// Helpers for the Pluris Policy role detail page (Task 6/7), modeled on
// dependency_group_detail_helpers.go.

// plurisRoleHero builds the HeroSpec for a role: a parent-chain
// breadcrumb (chain is root-first, role's own row not included -- see
// roleParentChain), the Builtin/Custom chip, and member count as a hero
// Def. The ⋮ dropdown carries only the delete form; Clone moved to the
// Settings tab (see plurisRoleSettingsTab) with a plain link back to it
// here, keeping the dropdown free of a text-input form (see
// plurisRoleHeroMenu).
func plurisRoleHero(role db.Role, chain []db.Role, memberCount int, csrf string) HeroSpec {
	chip := Chip{Label: "Custom", Class: "asset-chip-enroll-pending"}
	if role.IsBuiltin {
		chip = Chip{Label: "Builtin", Class: "asset-chip-lifecycle"}
	}
	crumbs := []Crumb{
		{Label: "Policy", Href: "/policy/catalog"},
		{Label: "Pluris Policy", Href: "/policy/pluris"},
	}
	for _, ancestor := range chain {
		crumbs = append(crumbs, Crumb{Label: ancestor.Name, Href: "/policy/pluris/" + strconv.FormatInt(ancestor.ID, 10)})
	}
	crumbs = append(crumbs, Crumb{Label: role.Name})
	return HeroSpec{
		Crumbs:     crumbs,
		Name:       role.Name,
		ID:         role.Slug,
		Chips:      []Chip{chip},
		Defs:       []HeroDef{{Label: "Members", Value: strconv.Itoa(memberCount)}},
		DeleteForm: plurisRoleHeroMenu(role, memberCount, csrf),
	}
}

// plurisRoleTabs wires the Settings/Permissions/Members tabs of the role
// detail page. originByKey and inheritedByKey are handler-precomputed
// (see roleOriginByKey/roleInheritedGrants in
// console/handlers/pluris_policy.go) so the templ layer stays dumb: it
// only reads the maps, it never walks the chain itself.
func plurisRoleTabs(role db.Role, effective, own authz.Grants, chain []db.Role, members []db.ListIdentitiesForRoleRow, groups []db.ListGroupsForRoleRow, originByKey, inheritedByKey map[string]string, eligibleRoles []db.Role, csrf string) []TabSpec {
	return []TabSpec{
		{Slug: "settings", Label: "Settings", Body: plurisRoleSettingsTab(role, eligibleRoles, csrf)},
		{Slug: "permissions", Label: "Permissions", Body: plurisRolePermissionsTab(role, effective, originByKey, inheritedByKey, csrf)},
		{Slug: "members", Label: "Members", Body: plurisRoleMembersTab(members, groups)},
	}
}

// currentParentID returns role's parent id, or 0 when parentless (the
// RolePickerSelect "No parent" sentinel).
func currentParentID(role db.Role) int64 {
	if role.ParentRoleID.Valid {
		return role.ParentRoleID.Int64
	}
	return 0
}

// currentScope returns the stored scope value for a scoped permission
// key, defaulting to "none" when absent (authz.Grants.ScopeOf returns ""
// for a missing key; the matrix's <select> only offers none/own/all).
func currentScope(g authz.Grants, key string) string {
	if s := g.ScopeOf(key); s != "" {
		return s
	}
	return "none"
}

// isGranted reports whether an unscoped permission key's stored value is
// "yes" (checkbox checked).
func isGranted(g authz.Grants, key string) bool {
	return g.ScopeOf(key) == "yes"
}
