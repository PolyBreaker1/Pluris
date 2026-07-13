package templates

import (
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

	CanEdit   bool
	CanAdmin  bool
	IsBundled bool

	OwnerLabel string
	CreatedAt  string

	Deps         ModuleDepsView
	AllDepGroups []dependencygroups.Group

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
}

// moduleEditorHero builds the HeroSpec for the module editor page.
func moduleEditorHero(d ModuleEditorData) HeroSpec {
	origin := "tenant"
	if d.IsBundled {
		origin = "bundled"
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
	if d.IsBundled {
		spec.Action = moduleCloneAction(d.Row.ModuleUrn, d.CSRF)
	}
	if d.CanAdmin && !d.IsBundled {
		spec.DeleteForm = moduleDeleteAction(d.Row.ModuleUrn, d.CSRF)
	}
	return spec
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
		{Slug: "sandbox", Label: "Sandbox", Body: moduleSandboxTab(d)},
		{Slug: "dependencies", Label: "Dependencies", Body: moduleDependenciesTab(d)},
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

func scriptSource(d ModuleEditorData, phase policymodules.LifecyclePhase) string {
	if sc, ok := d.Scripts[string(phase)]; ok {
		return sc.Source
	}
	return ""
}

func scriptFilename(d ModuleEditorData, phase policymodules.LifecyclePhase) string {
	if sc, ok := d.Scripts[string(phase)]; ok {
		return sc.Filename
	}
	return defaultScriptFilename(phase)
}

func defaultScriptFilename(phase policymodules.LifecyclePhase) string {
	if phase.Runtime() == policymodules.RuntimeWASM {
		return string(phase) + ".wasm"
	}
	return string(phase) + ".sh"
}
