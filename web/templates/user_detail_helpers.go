package templates

import (
	"strconv"

	"github.com/a-h/templ"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/services"
)

// Helpers for the standardized user detail page (Task 8): hero spec and
// tab wiring, mirroring the asset detail helpers.

// identityFieldPathAttrs returns the data-path attribute carrying the
// field's canonical parameter path (INV-CPP), or no attributes when the
// user entity does not mount the key.
func identityFieldPathAttrs(key string) templ.Attributes {
	if p := params.PathFor("user", key); p != "" {
		return templ.Attributes{"data-path": p}
	}
	return templ.Attributes{}
}

// userInitials picks the AD-style initials when set, else the first rune
// of the resolved display name.
func userInitials(user identities.Identity) string {
	if user.Initials != "" {
		return user.Initials
	}
	return firstRuneUpper(user.ResolvedDisplayName())
}

// userDetailHero builds the HeroSpec for a user detail page: name, UPN
// mono line, account-state chips, quick-fact defs, initials avatar and
// the Edit link as the single hero action.
func userDetailHero(user identities.Identity) HeroSpec {
	chips := []Chip{{Label: user.Role.Label(), Class: "asset-chip-role"}}
	if user.AccountEnabled {
		chips = append(chips, Chip{Label: "Enabled", Class: "asset-chip-enroll-enrolled"})
	} else {
		chips = append(chips, Chip{Label: "Disabled", Class: "asset-chip-enroll-retired"})
	}
	if user.AccountLocked {
		chips = append(chips, Chip{Label: "Locked", Class: "asset-chip-enroll-pending"})
	}

	var defs []HeroDef
	if user.Department != "" {
		defs = append(defs, HeroDef{Label: "Department", Value: user.Department})
	}
	if user.Title != "" {
		defs = append(defs, HeroDef{Label: "Title", Value: user.Title})
	}
	if user.Company != "" {
		defs = append(defs, HeroDef{Label: "Company", Value: user.Company})
	}
	if user.ManagerID != 0 {
		defs = append(defs, HeroDef{Label: "Manager", Value: "#" + strconv.FormatInt(user.ManagerID, 10)})
	}

	id := user.UserPrincipalName
	if id == "" {
		id = user.Email
	}

	return HeroSpec{
		Crumbs: []Crumb{
			{Label: "Users", Href: "/users"},
			{Label: user.ResolvedDisplayName()},
		},
		Name:   user.ResolvedDisplayName(),
		ID:     id,
		Chips:  chips,
		Defs:   defs,
		Visual: userAvatar(user),
		Action: userEditAction(user),
	}
}

// userDetailTabs wires the 4 standardized tabs (spec §4). Slugs are
// stable API for detail.js hash deep-links and the server test.
func userDetailTabs(user identities.Identity, assigned []assets.Asset, csrfToken string, groups []services.GroupRow, allGroups []db.Group, roles []db.ListRolesForIdentityDetailRow, allRoles []db.Role, applied []services.AppliedPolicy) []TabSpec {
	return []TabSpec{
		{Slug: "general", Label: "General", Body: userGeneralTab(user, assigned, csrfToken)},
		{Slug: "groups", Label: "Groups", Body: userGroupsTab(user, groups, allGroups, csrfToken)},
		{Slug: "policies", Label: "Applied Policies", Body: userPoliciesTab(user, applied)},
		{Slug: "roles", Label: "Roles", Body: userRolesTab(user, roles, allRoles, csrfToken)},
	}
}

// roleTypeLabel renders the Type column of the Roles tab.
func roleTypeLabel(isBuiltin bool) string {
	if isBuiltin {
		return "Built-in"
	}
	return "Custom"
}

// rolesNotAssigned filters tenant roles down to those not yet assigned
// (options for the add-role picker).
func rolesNotAssigned(all []db.Role, assigned []db.ListRolesForIdentityDetailRow) []db.Role {
	have := make(map[int64]bool, len(assigned))
	for _, r := range assigned {
		have[r.ID] = true
	}
	out := make([]db.Role, 0, len(all))
	for _, r := range all {
		if !have[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// groupsNotJoined filters the tenant's groups down to those the entity
// is not yet a member of (options for the add-to-group picker).
func groupsNotJoined(all []db.Group, joined []services.GroupRow) []db.Group {
	member := make(map[int64]bool, len(joined))
	for _, g := range joined {
		member[g.ID] = true
	}
	out := make([]db.Group, 0, len(all))
	for _, g := range all {
		if !member[g.ID] {
			out = append(out, g)
		}
	}
	return out
}

// appliedStatusChipClass maps an AppliedPolicy status to a chip class.
func appliedStatusChipClass(status string) string {
	if status == "Disabled" {
		return "asset-chip-enroll-retired"
	}
	return "asset-chip-enroll-enrolled"
}
