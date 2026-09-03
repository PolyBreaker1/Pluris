package templates

import (
	"encoding/json"
	"strings"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/db"
)

// Helpers for the dependency group detail page (Task 6), modeled on
// policy_detail_helpers.go.

// dependencyGroupHero builds the HeroSpec for a dependency group.
func dependencyGroupHero(g dependencygroups.Group, csrf string) HeroSpec {
	chip := Chip{Label: "Custom", Class: "asset-chip-enroll-pending"}
	if g.Builtin {
		chip = Chip{Label: "Builtin", Class: "asset-chip-lifecycle"}
	}
	return HeroSpec{
		Crumbs: []Crumb{
			{Label: "Policy", Href: "/policy/catalog"},
			{Label: "Dependency Groups", Href: "/policy/dependency-groups"},
			{Label: g.Name},
		},
		Name:       g.Name,
		ID:         g.Slug,
		Chips:      []Chip{chip},
		DeleteForm: dependencyGroupDeleteAction(g, csrf),
	}
}

// dependencyGroupTabs wires the General/Conditions/Modules tabs of the
// dependency group detail page.
func dependencyGroupTabs(g dependencygroups.Group, conds []db.DependencyGroupCondition, csrf string) []TabSpec {
	return []TabSpec{
		{Slug: "general", Label: "General", Body: dependencyGroupGeneralTab(g, csrf)},
		{Slug: "conditions", Label: "Conditions", Body: dependencyGroupConditionsTab(g, conds, csrf)},
		{Slug: "modules", Label: "Modules", Body: dependencyGroupModulesTab()},
	}
}

// conditionValuesLabel renders a condition's ValueJson ([]string) as a
// comma-joined human string.
func conditionValuesLabel(valueJSON string) string {
	var vals []string
	if err := json.Unmarshal([]byte(valueJSON), &vals); err != nil {
		return valueJSON
	}
	return strings.Join(vals, ", ")
}

// conditionParamLabel resolves a condition's stored ParamPath to its
// catalog/params display label (e.g. "Package family") for the
// Conditions tab's human-readable row summary (Task 2.3). Paths that no
// longer resolve (a param removed from the registry after the condition
// was created) fall back to the raw path rather than erroring — the row
// must still render.
func conditionParamLabel(path string) string {
	_, _, def, err := params.ResolvePath(path)
	if err != nil || def == nil {
		return path
	}
	return def.Label
}

// conditionRowSummary renders one condition (db row) as its single
// human-readable summary in the standardized subject · operator · value
// shape (INV-TEST), e.g. "OS family · is any of · linux",
// "Bash · uname -r · contains · 3", "Script · custom.sh · contains ·
// example". Used by dependency groups, dynamic group rules, and module
// version tests — one summary renderer, never a fork.
func conditionRowSummary(cond db.DependencyGroupCondition) string {
	op := dependencygroups.Operator(cond.Operator).Label()
	var subject string
	switch cond.Kind {
	case string(dependencygroups.KindCommand):
		subject = "Bash · " + conditionScriptExcerpt(cond.ScriptSource)
	case string(dependencygroups.KindScript):
		if cond.ScriptRef != "" {
			subject = "Script · " + cond.ScriptRef
		} else {
			subject = "Script · " + conditionScriptExcerpt(cond.ScriptSource)
		}
	default:
		subject = conditionParamLabel(cond.ParamPath)
	}
	if cond.Operator == string(dependencygroups.OpExists) || cond.Operator == "" {
		return subject + " · " + op
	}
	return subject + " · " + op + " · " + conditionValuesLabel(cond.ValueJson)
}

// conditionScriptExcerptMaxRunes bounds the script excerpt so a huge
// one-line script can't blow out the table row's width.
const conditionScriptExcerptMaxRunes = 60

// conditionScriptExcerpt renders a script condition's first source line,
// ellipsized, for display in a <code> cell. It returns plain text —
// templ's { expr } text-node escaping (not templ.Raw) makes it
// textContent-safe, so no HTML-escaping happens here.
func conditionScriptExcerpt(source string) string {
	line := source
	if i := strings.IndexAny(source, "\n\r"); i >= 0 {
		line = source[:i]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "(empty script)"
	}
	r := []rune(line)
	if len(r) > conditionScriptExcerptMaxRunes {
		line = string(r[:conditionScriptExcerptMaxRunes]) + "…"
	}
	return line
}

// conditionPrefill is the JSON shape the condition-builder dialog expects
// on an Edit button's data-cb-prefill attribute (camelCase — see
// condition_builder.templ's doc comment).
type conditionPrefill struct {
	Kind         string   `json:"kind"`
	ParamPath    string   `json:"paramPath"`
	Operator     string   `json:"operator"`
	Values       []string `json:"values"`
	ScriptSource string   `json:"scriptSource"`
	ScriptRef    string   `json:"scriptRef"`
}

// conditionPrefillJSON builds the Edit button's data-cb-prefill payload
// for an existing condition. Rendered through a templ attribute
// expression, so the templ compiler HTML-escapes the resulting string
// for us (e.g. `"` -> `&#34;`) — callers must not templ.Raw this.
func conditionPrefillJSON(cond db.DependencyGroupCondition) string {
	var vals []string
	_ = json.Unmarshal([]byte(cond.ValueJson), &vals)
	kind := cond.Kind
	if kind == "" {
		kind = string(dependencygroups.KindParam)
	}
	b, err := json.Marshal(conditionPrefill{
		Kind: kind, ParamPath: cond.ParamPath, Operator: cond.Operator, Values: vals,
		ScriptSource: cond.ScriptSource, ScriptRef: cond.ScriptRef,
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ModuleDepsView is the per-module dependency-group summary rendered on
// the Modules → Library page (Task 7). Platforms are OR'd (module can run
// wherever ANY platform group passes); Requirements are AND'd (module
// needs ALL requirement groups to pass).
type ModuleDepsView struct {
	Platforms    []ModuleDepChip
	Requirements []ModuleDepChip
}

// ModuleDepChip is one dependency-group link rendered as a small chip with
// an (admin-gated) remove control.
type ModuleDepChip struct {
	GroupID int64
	Name    string
}
