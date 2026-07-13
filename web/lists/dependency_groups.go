package lists

// ListIDDependencyGroups is the DOM/registry id for the Dependency
// Groups list (Task 5). Registered here so the table's columns come
// from the shared field registry (INV-L).
const ListIDDependencyGroups = "dependency-groups"

// ListIDDependencyGroupConditions is the embedded-table id for the
// Conditions tab on the dependency group detail page (Task 6).
const ListIDDependencyGroupConditions = "dependency-group-conditions"

func init() {
	Register(ListIDDependencyGroups, "Dependency Groups", detailTabGroups(), []FieldDef{
		{Key: "name", Label: "Name", Group: "main", DefaultVisible: true},
		{Key: "conditions", Label: "Conditions", Group: "main", DefaultVisible: true},
		{Key: "used_by", Label: "Used by", Group: "main", DefaultVisible: true},
		{Key: "type", Label: "Type", Group: "main", DefaultVisible: true},
	})

	Register(ListIDDependencyGroupConditions, "Conditions", detailTabGroups(), []FieldDef{
		{Key: "condition", Label: "Condition", Description: "The device-fact predicate or agent script this condition checks.", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Condition actions (edit/remove).", Group: "main", DefaultVisible: true},
	})
}
