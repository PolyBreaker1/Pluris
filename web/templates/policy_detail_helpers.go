package templates

import (
	"strings"

	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
)

// Helpers for the standardized policy detail page (Task 15).

// policyDetailHero builds the HeroSpec for a policy catalog entry.
func policyDetailHero(p policies.Policy) HeroSpec {
	chips := []Chip{{Label: policies.ScopeLabel(p.Scope), Class: "asset-chip-enroll-enrolled"}}
	if len(p.Category) > 0 {
		chips = append(chips, Chip{Label: p.Category[len(p.Category)-1], Class: "asset-chip-lifecycle"})
	}
	if p.Custom {
		chips = append(chips, Chip{Label: "Custom", Class: "asset-chip-enroll-pending"})
	}
	var defs []HeroDef
	if p.WinGPName != "" {
		defs = append(defs, HeroDef{Label: "Windows GP", Value: p.WinGPName, IconSVG: heroIconMonitor})
	}
	if p.WinGPPath != "" {
		defs = append(defs, HeroDef{Label: "GP path", Value: p.WinGPPath, IconSVG: heroIconFolder})
	}
	if p.LinuxImpl != "" {
		defs = append(defs, HeroDef{Label: "Linux mechanism", Value: p.LinuxImpl, IconSVG: heroIconTerminal})
	}
	spec := HeroSpec{
		Crumbs: []Crumb{
			{Label: "Policy", Href: "/policy/catalog"},
			{Label: "Catalog", Href: "/policy/catalog"},
			{Label: p.Name},
		},
		Name:  p.Name,
		ID:    p.ID,
		Chips: chips,
		Defs:  defs,
	}
	if p.Custom {
		spec.Action = policyEditAction(p)
	}
	return spec
}

// policyDetailTabs wires the 3 tabs of the policy detail page.
func policyDetailTabs(p policies.Policy, modules []policymodules.Module, assignments []db.ListAssignmentsByPolicyRow) []TabSpec {
	return []TabSpec{
		{Slug: "general", Label: "General", Body: policyGeneralTab(p)},
		{Slug: "modules", Label: "Modules", Body: policyModulesTab(p, modules)},
		{Slug: "assignments", Label: "Assignments", Body: policyAssignmentsTab(assignments)},
	}
}

// moduleLatestVersionLabel renders the latest version string, or a dash.
func moduleLatestVersionLabel(m policymodules.Module) string {
	if v := m.LatestVersion(); v != nil {
		return v.Version
	}
	return "—"
}

// moduleTargetOSLabel joins the module's target OS list.
func moduleTargetOSLabel(m policymodules.Module) string {
	if len(m.TargetOS) == 0 {
		return "any"
	}
	parts := make([]string, 0, len(m.TargetOS))
	for _, os := range m.TargetOS {
		parts = append(parts, string(os))
	}
	return strings.Join(parts, ", ")
}
