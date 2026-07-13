package lists

// Task 6.2 — the canonical AD-style Groups list page (/groups) plus the
// group detail page's Rules tab table. The Members/Roles detail tabs'
// registries live in detail_tabs.go (Task 7; Members reshaped in 6.2).

// ListIDGroups is the /groups list page (surfaced in the sidebar under
// both Users → "User Groups" and Assets → "Groups").
const ListIDGroups = "groups"

// ListIDGroupRules is the embedded-table id for the Rules tab on the
// group detail page (dynamic-membership rules).
const ListIDGroupRules = "group-rules-tab"

func init() {
	Register(ListIDGroups, "Groups", detailTabGroups(), []FieldDef{
		{Key: "name", Label: "Name", Description: "Group name and description.", Group: "main", DefaultVisible: true},
		{Key: "kind", Label: "Member kind", Description: "What the group may contain: assets, users, or both (mixed).", Group: "main", DefaultVisible: true},
		{Key: "membership", Label: "Membership", Description: "Static (admin-managed members) or dynamic (rule-computed members).", Group: "main", DefaultVisible: true},
		{Key: "members", Label: "Members", Description: "Total member count (direct + rule-sourced).", Group: "main", DefaultVisible: true},
		{Key: "category", Label: "Category", Description: "Group category (e.g. security, distribution).", Group: "main", DefaultVisible: true},
		{Key: "scope", Label: "Scope", Description: "Group scope (e.g. global, site).", Group: "main", DefaultVisible: true},
		{Key: "created", Label: "Created", Description: "When the group was created.", Group: "main", DefaultVisible: true},
	})

	Register(ListIDGroupRules, "Rules", detailTabGroups(), []FieldDef{
		{Key: "rule", Label: "Rule", Description: "Human-readable rule summary (parameter · operator · values, or custom script).", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Rule actions (edit/remove).", Group: "main", DefaultVisible: true},
	})
}
