package templates

import (
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
// mono line, account-state chips, key detail defs, initials avatar,
// Edit action and Delete in the ⋮ dropdown. warn, when non-empty, is a
// message surfaced from the full-page create flow (Task 8): a field
// could not be applied via UpdateFields after the identity was already
// created (see console/handlers/handlers.go's UserCreateSubmit); it
// renders as a dismissible banner reusing hero.Action's slot.
func userDetailHero(user identities.Identity, csrfToken string, warn string) HeroSpec {
	chips := []Chip{{Label: user.Role.Label(), Class: "asset-chip-role"}}
	if user.AccountEnabled {
		chips = append(chips, Chip{Label: "Enabled", Class: "asset-chip-enroll-enrolled"})
	} else {
		chips = append(chips, Chip{Label: "Disabled", Class: "asset-chip-enroll-retired"})
	}
	if user.AccountLocked {
		chips = append(chips, Chip{Label: "Locked", Class: "asset-chip-enroll-pending"})
	}

	// Key details: always show Username, Email; add others when present
	defs := []HeroDef{
		{Label: "Username", Value: user.Username, IconSVG: heroIconUser},
		{Label: "Email", Value: user.Email, IconSVG: heroIconMail},
	}
	if user.Department != "" {
		defs = append(defs, HeroDef{Label: "Department", Value: user.Department, IconSVG: heroIconBuilding})
	}
	if user.Title != "" {
		defs = append(defs, HeroDef{Label: "Title", Value: user.Title, IconSVG: heroIconBadge})
	}
	if user.Office != "" {
		defs = append(defs, HeroDef{Label: "Office", Value: user.Office, IconSVG: heroIconMapPin})
	}

	id := user.UserPrincipalName
	if id == "" {
		id = user.Email
	}

	var action templ.Component
	if warn != "" {
		action = userWarnBanner(warn)
	}

	return HeroSpec{
		Crumbs: []Crumb{
			{Label: "Users", Href: "/users"},
			{Label: user.ResolvedDisplayName()},
		},
		Name:       user.ResolvedDisplayName(),
		ID:         id,
		Chips:      chips,
		Defs:       defs,
		Visual:     userAvatar(user),
		Action:     action,
		DeleteForm: userDeleteDropdownItem(user, csrfToken),
	}
}

// userDetailTabs wires the 4 standardized tabs (spec §4). Slugs are
// stable API for detail.js hash deep-links and the server test.
func userDetailTabs(user identities.Identity, assigned []assets.Asset, csrfToken string, groups []services.GroupRow, allGroups []db.Group, roles []db.ListRolesForIdentityDetailRow, allRoles []db.Role, viaGroupRoles []db.ListGroupRolesForIdentityDetailRow, applied []services.AppliedPolicy) []TabSpec {
	return []TabSpec{
		{Slug: "general", Label: "General", Body: userGeneralTab(user, assigned, csrfToken)},
		{Slug: "groups", Label: "Groups", Body: userGroupsTab(user, groups, allGroups, csrfToken)},
		{Slug: "policies", Label: "Applied Policies", Body: userPoliciesTab(user, applied)},
		{Slug: "roles", Label: "Roles", Body: userRolesTab(user, roles, allRoles, viaGroupRoles, csrfToken)},
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

// stringOrDash renders a possibly-empty string with a dash fallback.
func stringOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
