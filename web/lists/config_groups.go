package lists

// ListIDConfigGroupAssignments is the embedded-table id for the
// Assignments tab on the Configuration Group detail page (Task 5.2).
const ListIDConfigGroupAssignments = "config-group-assignments"

// ListIDConfigGroupBindings is the embedded-table id for the Policy
// Bindings tab on the Configuration Group detail page (Task 5.2).
const ListIDConfigGroupBindings = "config-group-bindings"

func init() {
	// ListIDConfigGroups already exists as a constant (lists.go) but
	// gained no FieldDef registry until Task 5.2 replaced the popup
	// dialog + cg-table hand-rolled columns with the standard registry-
	// driven list page.
	Register(ListIDConfigGroups, "Configuration Groups", detailTabGroups(), []FieldDef{
		{Key: "name", Label: "Name", Description: "Configuration group name.", Group: "main", DefaultVisible: true},
		{Key: "assignments", Label: "Assignments", Description: "Number of targets this group is assigned to.", Group: "main", DefaultVisible: true},
		{Key: "bindings", Label: "Bindings", Description: "Number of policy bindings in this group.", Group: "main", DefaultVisible: true},
		{Key: "enabled", Label: "Enabled", Description: "Whether the group is currently enabled.", Group: "main", DefaultVisible: true},
		{Key: "created", Label: "Created", Description: "When the group was created.", Group: "main", DefaultVisible: true},
	})

	Register(ListIDConfigGroupAssignments, "Assignments", detailTabGroups(), []FieldDef{
		{Key: "target", Label: "Target", Description: "The computer, user, or group this binding applies to.", Group: "main", DefaultVisible: true},
		{Key: "type", Label: "Type", Description: "Target kind (computer, user, group).", Group: "main", DefaultVisible: true},
		{Key: "priority", Label: "Priority", Description: "Higher priority wins when multiple groups target the same entity.", Group: "main", DefaultVisible: true},
		{Key: "enforced", Label: "Enforced", Description: "Enforced assignments cannot be overridden by a lower-priority group.", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Assignment actions (remove).", Group: "main", DefaultVisible: true},
	})

	Register(ListIDConfigGroupBindings, "Policy Bindings", detailTabGroups(), []FieldDef{
		{Key: "policy", Label: "Policy", Description: "Catalog policy URN this binding configures.", Group: "main", DefaultVisible: true},
		{Key: "module", Label: "Module", Description: "Module that implements the policy (default or override).", Group: "main", DefaultVisible: true},
		{Key: "state", Label: "State", Description: "enabled, disabled, or not_configured.", Group: "main", DefaultVisible: true},
		{Key: "values", Label: "Parameters", Description: "Summary of the configured parameter values.", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Binding actions (edit/remove).", Group: "main", DefaultVisible: true},
	})
}
