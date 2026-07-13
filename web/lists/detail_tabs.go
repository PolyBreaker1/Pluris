// Package lists — registries for the embedded tables on the standardized
// asset detail page (Task 7, spec §2). Each detail tab that shows a table
// registers its columns here so DetailTableFrame can render the <thead>
// from the single source of truth (INV-L). These tables are small,
// server-rendered sets for now, so no Sort/Filter metadata yet — when a
// tab graduates to a full interactive list, extend its FieldDefs in place.
package lists

// Canonical list IDs for the asset detail tabs.
const (
	ListIDComputerGroups     = "computer-groups"
	ListIDAppliedPolicies    = "applied-policies"
	ListIDAssetConfiguration = "asset-configuration"
	ListIDInstalledSoftware  = "installed-software"
	ListIDAssetWineGroups    = "asset-wine-groups"
	ListIDAssetScripts       = "asset-scripts"
	ListIDAssetLogs          = "asset-logs"
)

// detailTabGroups — every detail-tab list uses a single "main" group;
// the column picker is not mounted on these embedded tables yet.
func detailTabGroups() []FieldGroup {
	return []FieldGroup{{Key: "main", Label: "Columns"}}
}

func init() {
	Register(ListIDComputerGroups, "Groups", detailTabGroups(), []FieldDef{
		{Key: "group", Label: "Group", Description: "Group the asset is a member of.", Group: "main", DefaultVisible: true},
		{Key: "category", Label: "Category", Description: "Group category (security, distribution, dynamic…).", Group: "main", DefaultVisible: true},
		{Key: "scope", Label: "Scope", Description: "Scope the group applies at (tenant, site…).", Group: "main", DefaultVisible: true},
		{Key: "source", Label: "Source", Description: "How the membership was established (manual, rule, sync).", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Membership actions (remove).", Group: "main", DefaultVisible: true},
	})

	Register(ListIDAppliedPolicies, "Applied Policies", detailTabGroups(), []FieldDef{
		{Key: "policy", Label: "Policy", Description: "Catalog policy applied to this asset.", Group: "main", DefaultVisible: true},
		{Key: "source", Label: "Source", Description: "Configuration Group the binding comes from.", Group: "main", DefaultVisible: true},
		{Key: "scope", Label: "Scope", Description: "Computer or user scope of the setting.", Group: "main", DefaultVisible: true},
		{Key: "value", Label: "Value", Description: "Effective value bound for this asset.", Group: "main", DefaultVisible: true},
		{Key: "status", Label: "Status", Description: "Application status reported by the agent.", Group: "main", DefaultVisible: true},
	})

	Register(ListIDAssetConfiguration, "Configuration", detailTabGroups(), []FieldDef{
		{Key: "setting", Label: "Setting", Description: "Managed configuration setting on this asset.", Group: "main", DefaultVisible: true},
		{Key: "policy_value", Label: "Policy value", Description: "Value the policy engine wants applied.", Group: "main", DefaultVisible: true},
		{Key: "reported_value", Label: "Reported value", Description: "Value last reported by the agent.", Group: "main", DefaultVisible: true},
		{Key: "status", Label: "Status", Description: "Compliance status: in sync, drifted, pending.", Group: "main", DefaultVisible: true},
		{Key: "action", Label: "Action", Description: "Remediation action available for this setting.", Group: "main", DefaultVisible: true},
	})

	Register(ListIDInstalledSoftware, "Installed Software", detailTabGroups(), []FieldDef{
		{Key: "name", Label: "Name", Description: "Package or application name from the agent inventory.", Group: "main", DefaultVisible: true},
		{Key: "version", Label: "Version", Description: "Installed version string.", Group: "main", DefaultVisible: true},
		{Key: "publisher", Label: "Publisher", Description: "Vendor or packager of the software.", Group: "main", DefaultVisible: true},
		{Key: "type", Label: "Type", Description: "Package type (deb, rpm, flatpak, snap, wine…).", Group: "main", DefaultVisible: true},
		{Key: "installed", Label: "Installed", Description: "Date the package was installed on the asset.", Group: "main", DefaultVisible: true},
		{Key: "size", Label: "Size", Description: "Installed size on disk.", Group: "main", DefaultVisible: true},
	})

	Register(ListIDAssetWineGroups, "Wine Groups", detailTabGroups(), []FieldDef{
		{Key: "wine_config_group", Label: "Wine Config Group", Description: "Wine configuration group assigned to this asset.", Group: "main", DefaultVisible: true},
		{Key: "wine_version", Label: "Wine version", Description: "Wine runtime version the group pins.", Group: "main", DefaultVisible: true},
		{Key: "arch", Label: "Arch", Description: "Wine prefix architecture (win32/win64).", Group: "main", DefaultVisible: true},
		{Key: "applications", Label: "Applications", Description: "Windows applications delivered by the group.", Group: "main", DefaultVisible: true},
		{Key: "assigned", Label: "Assigned", Description: "When the group was assigned to this asset.", Group: "main", DefaultVisible: true},
	})

	Register(ListIDAssetScripts, "Assigned Scripts", detailTabGroups(), []FieldDef{
		{Key: "script", Label: "Script", Description: "Automation script assigned to this asset.", Group: "main", DefaultVisible: true},
		{Key: "trigger", Label: "Trigger", Description: "Event or schedule that runs the script.", Group: "main", DefaultVisible: true},
		{Key: "last_run", Label: "Last run", Description: "When the script last executed on this asset.", Group: "main", DefaultVisible: true},
		{Key: "status", Label: "Status", Description: "Result of the last execution.", Group: "main", DefaultVisible: true},
		{Key: "next_run", Label: "Next run", Description: "Next scheduled execution, if any.", Group: "main", DefaultVisible: true},
	})

	Register(ListIDAssetLogs, "Logs", detailTabGroups(), []FieldDef{
		{Key: "time", Label: "Time", Description: "When the event was recorded.", Group: "main", DefaultVisible: true},
		{Key: "event", Label: "Event", Description: "Event type from the activity log.", Group: "main", DefaultVisible: true},
		{Key: "detail", Label: "Detail", Description: "Free-form event detail.", Group: "main", DefaultVisible: true},
		{Key: "actor", Label: "Actor", Description: "Identity that triggered the event, or System.", Group: "main", DefaultVisible: true},
	})
}

// Canonical list IDs for the user detail tabs (Task 8, spec §4).
const (
	ListIDUserGroups = "user-groups"
	ListIDUserRoles  = "user-roles"
)

func init() {
	Register(ListIDUserGroups, "Groups", detailTabGroups(), []FieldDef{
		{Key: "group", Label: "Group", Description: "Group the user is a member of.", Group: "main", DefaultVisible: true},
		{Key: "category", Label: "Category", Description: "Group category (security, distribution, dynamic…).", Group: "main", DefaultVisible: true},
		{Key: "scope", Label: "Scope", Description: "Scope the group applies at (tenant, site…).", Group: "main", DefaultVisible: true},
		{Key: "source", Label: "Source", Description: "How the membership was established (manual, rule, sync).", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Membership actions (remove).", Group: "main", DefaultVisible: true},
	})

	Register(ListIDUserRoles, "Roles", detailTabGroups(), []FieldDef{
		{Key: "role", Label: "Role", Description: "Role assigned to this user.", Group: "main", DefaultVisible: true},
		{Key: "type", Label: "Type", Description: "Built-in or custom role.", Group: "main", DefaultVisible: true},
		{Key: "description", Label: "Description", Description: "What the role grants.", Group: "main", DefaultVisible: true},
		{Key: "assigned", Label: "Assigned", Description: "When the role was assigned.", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Assignment actions (remove).", Group: "main", DefaultVisible: true},
	})
}

// Canonical list IDs for the group detail tabs (Task 7).
const (
	ListIDGroupMembers = "group-members"
	ListIDGroupRoles   = "group-roles"
)

func init() {
	// Task 6.2 reshaped the Members tab from identity-only columns
	// (username/display name/email) to mixed asset+identity member rows
	// with a source chip and a direct-only remove action.
	Register(ListIDGroupMembers, "Members", detailTabGroups(), []FieldDef{
		{Key: "member", Label: "Member", Description: "The asset or user that belongs to this group.", Group: "main", DefaultVisible: true},
		{Key: "kind", Label: "Kind", Description: "Member kind (asset or user).", Group: "main", DefaultVisible: true},
		{Key: "source", Label: "Source", Description: "Direct (added by an admin) or Dynamic (computed from the group's rules).", Group: "main", DefaultVisible: true},
		{Key: "added", Label: "Added", Description: "When the membership was established.", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Membership actions (remove; direct members only).", Group: "main", DefaultVisible: true},
	})

	Register(ListIDGroupRoles, "Roles", detailTabGroups(), []FieldDef{
		{Key: "role", Label: "Role", Description: "Role assigned to this group; members inherit it.", Group: "main", DefaultVisible: true},
		{Key: "type", Label: "Type", Description: "Built-in or custom role.", Group: "main", DefaultVisible: true},
		{Key: "assigned", Label: "Assigned", Description: "When the role was assigned to the group.", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Assignment actions (remove).", Group: "main", DefaultVisible: true},
	})
}

// Canonical list IDs for the policy detail tabs (Task 15).
const (
	ListIDPolicyModulesForPolicy = "policy-modules-for-policy"
	ListIDPolicyAssignments      = "policy-assignments"
)

func init() {
	Register(ListIDPolicyModulesForPolicy, "Modules", detailTabGroups(), []FieldDef{
		{Key: "module", Label: "Module", Description: "Policy module that can implement this policy.", Group: "main", DefaultVisible: true},
		{Key: "origin", Label: "Origin", Description: "bundled, tenant-authored, or imported.", Group: "main", DefaultVisible: true},
		{Key: "version", Label: "Version", Description: "Latest available module version.", Group: "main", DefaultVisible: true},
		{Key: "status", Label: "Scope", Description: "machine, user, or both.", Group: "main", DefaultVisible: true},
		{Key: "target_os", Label: "Target OS", Description: "Operating systems the module supports.", Group: "main", DefaultVisible: true},
	})

	Register(ListIDPolicyAssignments, "Assignments", detailTabGroups(), []FieldDef{
		{Key: "config_group", Label: "Configuration Group", Description: "Configuration group binding this policy.", Group: "main", DefaultVisible: true},
		{Key: "target", Label: "Target", Description: "Entity the group is assigned to.", Group: "main", DefaultVisible: true},
		{Key: "scope", Label: "Scope", Description: "Configuration group scope.", Group: "main", DefaultVisible: true},
		{Key: "status", Label: "Status", Description: "Assigned or Disabled.", Group: "main", DefaultVisible: true},
	})
}
