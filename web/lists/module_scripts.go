package lists

// ListIDModuleScripts is the embedded-table id for the Scripts tab on
// the Policy Module editor page (Scripts+Enforcement redesign, CP2).
// Replaces the old phase-tabbed inline script editor: scripts are now
// first-class named rows (name/language/origin), listed like any other
// INV-L table with add/rename/delete row actions.
const ListIDModuleScripts = "module-scripts"

func init() {
	Register(ListIDModuleScripts, "Scripts", detailTabGroups(), []FieldDef{
		{Key: "name", Label: "Name", Description: "The script's stable name, referenced by enforcement actions.", Group: "main", DefaultVisible: true},
		{Key: "language", Label: "Language", Description: "Interpreter the script runs under: sh, powershell, or python.", Group: "main", DefaultVisible: true},
		{Key: "origin", Label: "Origin", Description: "default (bundled/pristine, forked on first edit) or custom (tenant-authored or edited).", Group: "main", DefaultVisible: true},
		{Key: "used_by", Label: "Used by", Description: "Enforcement actions that reference this script by name.", Group: "main", DefaultVisible: true},
		{Key: "actions", Label: "", Description: "Row actions: rename and delete (draft + edit permission required).", Group: "main", DefaultVisible: true},
	})
}
