package templates

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
)

// Task 4.3 — the module detail/create editor page. This file holds the
// plain-Go data shape and helpers; policy_module_editor.templ holds the
// markup. Kept split the same way policy_detail.templ /
// policy_detail_helpers.go are, so templ-generated code stays purely
// declarative.

// ModuleVersionRow is one row of the Versions tab table -- the fields
// ListVersionsByModuleRow carries (id, version, state, published_at/by)
// plus a script count looked up per row by the handler.
type ModuleVersionRow struct {
	ID          int64
	Version     string
	State       string
	PublishedAt string
	PublishedBy string
	ScriptCount int
}

// ModuleEditorData bundles everything the module editor page needs to
// render, computed once by the handler (PolicyModuleDetail) so the templ
// side stays declarative. TargetOSCSV/SatisfiesCSV/etc are v1's "simple
// repeatable text-input list" (comma-joined) per the Task 4.3 brief --
// a real multi-picker is future work.
type ModuleEditorData struct {
	Row      db.PolicyModule
	Module   policymodules.Module
	Versions []ModuleVersionRow

	SelectedID  int64
	Selected    db.PolicyModuleVersion
	HasSelected bool
	IsDraft     bool
	Scripts     map[string]db.PolicyModuleScript // phase -> script; missing phase = zero value

	// ScriptList/ScriptUsedBy back the Scripts tab's INV-L list (CP2 of
	// the Scripts+Enforcement redesign): ScriptList is every named
	// script row for the selected version (services.ListScripts);
	// ScriptUsedBy maps a script name to the labels of every enforcement
	// action (services.ListActions, Kind=="script") that references it
	// by name, for the "Used by" column.
	ScriptList   []policymodules.Script
	ScriptUsedBy map[string][]string

	CanEdit   bool
	CanAdmin  bool
	IsBundled bool

	OwnerLabel string
	CreatedAt  string

	Deps         ModuleDepsView
	AllDepGroups []dependencygroups.Group

	DependsOn           []policymodules.Dependency
	Conflicts           []string
	Conditions          []db.ModuleVersionCondition
	ConditionsMatchMode string
	PickerModules       []ModulePickerOption

	CSRF string
	Warn string

	TargetOSCSV         string
	SatisfiesCSV        string
	FsReadCSV           string
	FsWriteCSV          string
	NetEgressCSV        string
	SandboxUser         string
	DependsOnCSV        string
	ConflictsCSV        string
	ParametersSchemaRaw string
	ReportSchemaRaw     string
}

// moduleEditorHero builds the HeroSpec for the module editor page.
func moduleEditorHero(d ModuleEditorData) HeroSpec {
	origin := d.Module.Origin
	if origin == "" {
		origin = "tenant"
		if d.IsBundled {
			origin = "bundled"
		}
	}
	chips := []Chip{{Label: origin, Class: "asset-chip-enroll-" + moduleOriginChipClass(origin)}}
	if d.Module.Scope != "" {
		chips = append(chips, Chip{Label: d.Module.Scope, Class: "asset-chip-lifecycle"})
	}
	if len(d.Module.TargetOS) > 0 {
		chips = append(chips, Chip{Label: moduleTargetOSLabel(d.Module), Class: "asset-chip-lifecycle"})
	}
	if v := d.Module.LatestVersion(); v != nil {
		chips = append(chips, Chip{Label: "published " + v.Version, Class: "asset-chip-enroll-enrolled"})
	}

	defs := []HeroDef{
		{Label: "Owner", Value: d.OwnerLabel},
		{Label: "Satisfies", Value: strconv.Itoa(len(d.Module.Satisfies)) + " polic" + pluralY(len(d.Module.Satisfies))},
		{Label: "Created", Value: d.CreatedAt},
	}

	spec := HeroSpec{
		Crumbs: []Crumb{
			{Label: "Policy", Href: "/policy/modules"},
			{Label: "Modules", Href: "/policy/modules"},
			{Label: d.Module.Title},
		},
		Name:  d.Module.Title,
		ID:    d.Row.ModuleUrn,
		Chips: chips,
		Defs:  defs,
	}
	if d.IsBundled || len(d.Module.Satisfies) > 0 {
		spec.Action = modulePickAction(d)
	}
	if d.CanAdmin && !d.IsBundled {
		spec.DeleteForm = moduleDeleteAction(d.Row.ModuleUrn, d.CSRF)
	}
	return spec
}

func modulePickerOS(m policymodules.Module) string {
	if len(m.TargetOS) == 0 {
		return "any"
	}
	return string(m.TargetOS[0])
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func moduleOriginChipClass(origin string) string {
	if origin == "bundled" {
		return "retired"
	}
	return "enrolled"
}

// moduleEditorTabs wires the 6 tabs of the module editor page.
func moduleEditorTabs(d ModuleEditorData) []TabSpec {
	return []TabSpec{
		{Slug: "general", Label: "General", Body: moduleGeneralTab(d)},
		{Slug: "versions", Label: "Versions", Body: moduleVersionsTab(d)},
		{Slug: "scripts", Label: "Scripts", Body: moduleScriptsTab(d)},
		{Slug: "parameters", Label: "Parameters", Body: moduleParametersTab(d)},
		{Slug: "report", Label: "Report", Body: moduleReportTab(d)},
		{Slug: "sandbox", Label: "Sandbox", Body: moduleSandboxTab(d)},
		{Slug: "dependencies", Label: "Dependencies", Body: moduleDependenciesTab(d)},
	}
}

// ModulePickerOption is one entry of the Dependencies tab's module
// picker <select> — every module the tenant can see except this one.
type ModulePickerOption struct {
	URN   string
	Title string
}

// moduleVersionActionURL builds the POST target for the Dependencies
// tab's structured actions (deps/conflicts/conditions routes).
func moduleVersionActionURL(d ModuleEditorData, action string) string {
	return "/policy/modules/" + d.Row.ModuleUrn + "/versions/" + strconv.FormatInt(d.SelectedID, 10) + "/" + action
}

// moduleConditionAsDGRow adapts a module_version_conditions row onto the
// dependency-condition row shape so the Tests section reuses
// conditionRowSummary / conditionPrefillJSON verbatim — the tables are
// column-parity mirrors by design (011's header comment), same adapter
// pattern as ruleAsCondition.
func moduleConditionAsDGRow(c db.ModuleVersionCondition) db.DependencyGroupCondition {
	return db.DependencyGroupCondition{
		ID: c.ID, ParamPath: c.ParamPath, Operator: c.Operator,
		ValueJson: c.ValueJson, Seq: c.Seq, Kind: c.Kind,
		ScriptSource: c.ScriptSource, ScriptRef: c.ScriptRef, ScriptExpect: c.ScriptExpect,
	}
}

// moduleVersionFieldsSaveURL is the shared version-scoped field-update
// endpoint every non-"meta" section on this page saves to (Task 4.3's
// "one clean mechanism" choice -- see the report). Empty when no version
// is selected (sections needing it are simply not editable then).
func moduleVersionFieldsSaveURL(d ModuleEditorData) string {
	if !d.HasSelected {
		return ""
	}
	return "/api/modules/" + d.Row.ModuleUrn + "/versions/" + strconv.FormatInt(d.SelectedID, 10) + "/fields"
}

func moduleMetaSaveURL(d ModuleEditorData) string {
	return "/api/modules/" + d.Row.ModuleUrn + "/fields"
}

func joinTargetOS(os []policymodules.TargetOS) string {
	parts := make([]string, 0, len(os))
	for _, o := range os {
		parts = append(parts, string(o))
	}
	return strings.Join(parts, ", ")
}

// scriptOriginLabel/scriptOriginChipClass render the Scripts tab's
// Origin column (CP2): "default" is the pristine bundled/seeded script
// (forked on first edit -- see policymodules_scripts.go's package
// comment), "custom" is tenant-authored or the edited working copy.
func scriptOriginLabel(origin string) string {
	if origin == "default" {
		return "(default)"
	}
	return "(custom)"
}

func scriptOriginChipClass(origin string) string {
	if origin == "default" {
		return "pm-status pm-status-superseded"
	}
	return "pm-status pm-status-published"
}

// scriptUsedByLabel renders the Scripts tab's "Used by" column: the
// comma-joined labels of every enforcement action referencing name, or
// an em dash when nothing references it yet.
func scriptUsedByLabel(d ModuleEditorData, name string) string {
	labels := d.ScriptUsedBy[name]
	if len(labels) == 0 {
		return "—"
	}
	return strings.Join(labels, ", ")
}

// scriptEditURL is the Scripts tab row's primary link -- the standalone
// script editor route (CP3). Uses the same :id/versions/:vid shape as
// its sibling CP2 routes (moduleVersionActionURL) so moduleVersionEditContext
// resolves tenant+module+version+permission identically.
func scriptEditURL(d ModuleEditorData, name string) string {
	return "/policy/modules/" + d.Row.ModuleUrn + "/versions/" + strconv.FormatInt(d.SelectedID, 10) + "/scripts/" + url.PathEscape(name) + "/edit"
}
