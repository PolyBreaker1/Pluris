package lists

// ListIDPlurisRoles is the DOM/registry id for the Pluris Policy role
// list (Task 7). Registered here so the table's columns come from the
// shared field registry (INV-L).
const ListIDPlurisRoles = "pluris-roles"

// ListIDPlurisRoleMembers is the embedded-table id for the Members tab
// on the Pluris Policy role detail page (Task 7).
const ListIDPlurisRoleMembers = "pluris-role-members"

// ListIDPlurisRoleGroups is the embedded-table id for the "Groups
// holding this role" table on the Members tab (Task 6).
const ListIDPlurisRoleGroups = "pluris-role-groups"

func init() {
	Register(ListIDPlurisRoles, "Pluris Policy", detailTabGroups(), []FieldDef{
		{Key: "name", Label: "Name", Group: "main", DefaultVisible: true},
		{Key: "type", Label: "Type", Group: "main", DefaultVisible: true},
		{Key: "parent", Label: "Parent", Group: "main", DefaultVisible: true},
		{Key: "members", Label: "Members", Group: "main", DefaultVisible: true},
		{Key: "permissions", Label: "Permissions", Group: "main", DefaultVisible: true},
	})

	Register(ListIDPlurisRoleMembers, "Members", detailTabGroups(), []FieldDef{
		{Key: "username", Label: "Username", Group: "main", DefaultVisible: true},
		{Key: "display_name", Label: "Display name", Group: "main", DefaultVisible: true},
		{Key: "email", Label: "Email", Group: "main", DefaultVisible: true},
	})

	Register(ListIDPlurisRoleGroups, "Groups holding this role", detailTabGroups(), []FieldDef{
		{Key: "name", Label: "Group name", Group: "main", DefaultVisible: true},
	})
}
