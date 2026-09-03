package templates

import (
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/services"
)

// Helpers + view models for the group detail page (Task 6.2), modeled on
// config_groups_helpers.go / dependency_group_detail_helpers.go.

// GroupRecalcFlash carries EvaluateDynamicMembership's counts across the
// recalculate route's redirect (encoded as query params, decoded by the
// detail handler) into the Rules tab's result banner.
type GroupRecalcFlash struct {
	Added   int
	Removed int
	Total   int
}

// GroupDetailData is everything GroupDetailPage renders.
type GroupDetailData struct {
	Group    db.Group
	Members  []services.GroupMemberRow
	Roles    []db.ListRolesForGroupDetailRow
	AllRoles []db.Role
	Rules    []db.GroupMembershipRule
	Targets  []configgroups.Target
	Active   string // sidebar active key (users-groups / assets-groups)
	CSRF     string
	Recalc   *GroupRecalcFlash // non-nil right after a recalculation
}

// GroupDetailActiveKey picks the sidebar active key for a group's detail
// page from its member_kind: identity → users-groups, asset →
// assets-groups. Mixed groups appear under BOTH sidebar surfaces but a
// page can only highlight one tree — users-groups is chosen (mixed
// groups always contain the identity view), matching groupListActiveKey's
// default for the unfiltered list.
func GroupDetailActiveKey(memberKind string) string {
	if memberKind == "asset" {
		return "assets-groups"
	}
	return "users-groups"
}

// groupDetailPath is the base URL for one group's detail page.
func groupDetailPath(id int64) string {
	return "/groups/" + strconv.FormatInt(id, 10)
}

// groupFieldsPath is the General tab's fields-API endpoint.
func groupFieldsPath(id int64) string {
	return "/api/groups/" + strconv.FormatInt(id, 10) + "/fields"
}

// groupCrumbRoot picks the breadcrumb root for the group's sidebar
// surface.
func groupCrumbRoot(memberKind string) Crumb {
	if memberKind == "asset" {
		return Crumb{Label: "Assets", Href: "/assets/computers"}
	}
	return Crumb{Label: "Users", Href: "/users"}
}

// groupDetailHero builds the HeroSpec: name, slug, member-kind /
// membership / member-count chips, delete in the ⋮ dropdown.
func groupDetailHero(d GroupDetailData) HeroSpec {
	return HeroSpec{
		Crumbs: []Crumb{
			groupCrumbRoot(d.Group.MemberKind),
			{Label: "Groups", Href: groupListHref(d.Group.MemberKind)},
			{Label: d.Group.Name},
		},
		Name: d.Group.Name,
		ID:   d.Group.Slug,
		Chips: []Chip{
			{Label: groupKindLabel(d.Group.MemberKind), Class: chipSuffix(groupKindChipClass(d.Group.MemberKind))},
			{Label: groupMembershipLabel(d.Group.Membership), Class: chipSuffix(groupMembershipChipClass(d.Group.Membership))},
			{Label: strconv.Itoa(len(d.Members)) + " member(s)", Class: ""},
		},
		DeleteForm: groupDeleteAction(d),
	}
}

// chipSuffix strips the leading "asset-chip " prefix from the list-page
// chip-class helpers: HeroSpec.Chips prepends "asset-chip" itself.
func chipSuffix(full string) string {
	return strings.TrimSpace(strings.TrimPrefix(full, "asset-chip"))
}

// groupListHref links back to the list view matching the group's kind.
func groupListHref(memberKind string) string {
	switch memberKind {
	case "asset":
		return "/groups?kind=asset"
	case "identity":
		return "/groups?kind=identity"
	}
	return "/groups"
}

// groupDetailTabs wires the 4 tabs. Slugs are stable API for detail.js
// hash deep-links.
func groupDetailTabs(d GroupDetailData) []TabSpec {
	return []TabSpec{
		{Slug: "general", Label: "General", Body: groupGeneralTab(d)},
		{Slug: "members", Label: "Members", Body: groupMembersTab(d)},
		{Slug: "rules", Label: "Rules", Body: groupRulesTab(d)},
		{Slug: "roles", Label: "Roles", Body: groupRolesTab(d.Group, d.Roles, d.AllRoles, d.CSRF)},
	}
}

// groupAllowedTargetKinds restricts the shared TargetPickerDialog to the
// member kinds the group may contain: asset groups pick computers only,
// identity groups pick users only, mixed groups pick both. Group-of-group
// and configuration-group kinds are never offered — group_memberships
// rows hold exactly one asset_id or identity_id (001's CHECK).
func groupAllowedTargetKinds(memberKind string) []configgroups.TargetKind {
	switch memberKind {
	case "asset":
		return []configgroups.TargetKind{configgroups.KindComputer}
	case "identity":
		return []configgroups.TargetKind{configgroups.KindUser}
	}
	return []configgroups.TargetKind{configgroups.KindComputer, configgroups.KindUser}
}

// groupMemberKindLabel is the singular per-row form of groupKindLabel
// for the Members tab's Kind column.
func groupMemberKindLabel(kind string) string {
	switch kind {
	case "asset":
		return "Asset"
	case "identity":
		return "User"
	}
	return kind
}

// groupSourceChipClass colors the Members tab's source chip: Dynamic
// (rule-sourced) gets the amber "pending" tint, Direct stays neutral.
func groupSourceChipClass(source string) string {
	if source == "Dynamic" {
		return "asset-chip asset-chip-enroll-pending"
	}
	return "asset-chip"
}

// ruleAsCondition adapts a group_membership_rules row onto the
// dependency-condition row shape so the Rules tab reuses
// conditionRowSummary / conditionPrefillJSON verbatim — the two tables
// are column-for-column mirrors by design (009's header comment).
func ruleAsCondition(r db.GroupMembershipRule) db.DependencyGroupCondition {
	return db.DependencyGroupCondition{
		ID: r.ID, GroupID: r.GroupID, ParamPath: r.ParamPath, Operator: r.Operator,
		ValueJson: r.ValueJson, Seq: r.Seq, Kind: r.Kind,
		ScriptSource: r.ScriptSource, ScriptRef: r.ScriptRef, ScriptExpect: r.ScriptExpect,
	}
}

// groupRuleRowSummary renders one rule as its human-readable summary.
func groupRuleRowSummary(r db.GroupMembershipRule) string {
	return conditionRowSummary(ruleAsCondition(r))
}

// groupRulePrefillJSON builds the Edit button's data-cb-prefill payload.
func groupRulePrefillJSON(r db.GroupMembershipRule) string {
	return conditionPrefillJSON(ruleAsCondition(r))
}

// lowerJoin joins its arguments with spaces, lowercased — search-blob
// helper for the groups list.
func lowerJoin(parts ...string) string {
	return strings.ToLower(strings.Join(parts, " "))
}
