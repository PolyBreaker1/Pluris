package templates

// Plain-Go view models + hero/tab wiring for the Configuration Group
// pages (Task 5.2) — see config_groups.templ for the templ bodies.
// Mirrors policy_module_editor_helpers.go's split: templ stays
// declarative, everything computed lives here.

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/db"
)

// ConfigGroupRow is the list-view model for one Configuration Group.
type ConfigGroupRow struct {
	ID          int64
	Name        string
	Description string
	Enabled     bool
	Assignments int64
	Bindings    int64
	CreatedAt   time.Time
}

// ConfigGroupAssignmentRow is one row on the detail page's Assignments
// tab: a resolved target label plus the raw assignment fields.
type ConfigGroupAssignmentRow struct {
	ID         int64
	TargetType string // asset | identity | group | site | tenant
	TargetID   int64
	Label      string
	Priority   int64
	Enforced   bool
}

// ConfigGroupBindingRow is one row on the detail page's Policy Bindings
// tab.
type ConfigGroupBindingRow struct {
	ID                  int64
	PolicyURN           string
	State               string
	ModuleTitle         string // joined module title; "" = default (inherit)
	ModuleURN           string // module URN for the edit form's select; "" = default
	ModuleID            sql.NullInt64
	ModuleVersionPinned string
	ParameterValuesJSON string
}

// ConfigGroupDetailData carries everything ConfigGroupDetailPage renders.
type ConfigGroupDetailData struct {
	Group       db.ConfigurationGroup
	Assignments []ConfigGroupAssignmentRow
	Bindings    []ConfigGroupBindingRow
	Catalog     []policies.Policy
	Targets     []configgroups.Target
	// PolicyCandidatesJSON is the binding form's data island:
	// {"<policyURN>": [{"urn","title","schema"}...], ...} where schema is
	// the candidate module's parameters_schema JSON (string; may be "").
	// Candidates are ordered bundled-first so index 0 matches the
	// server's default-module pick (services.ConfigGroupService.
	// resolveParamSchema). Built by PolicyGroupDetail.
	PolicyCandidatesJSON string
	CSRF                 string
}

// configGroupDetailPath is the base URL for one group's detail page.
func configGroupDetailPath(id int64) string {
	return "/policy/groups/" + strconv.FormatInt(id, 10)
}

// configGroupSearchBlob — lowercase haystack for the list filter.
func configGroupSearchBlob(r ConfigGroupRow) string {
	return strings.ToLower(strings.Join([]string{
		r.Name, r.Description, strconv.FormatInt(r.ID, 10), configGroupEnabledAttr(r),
	}, " "))
}

// configGroupEnabledAttr — data-cg-enabled value for the quick filter.
func configGroupEnabledAttr(r ConfigGroupRow) string {
	if r.Enabled {
		return "enabled"
	}
	return "disabled"
}

// configGroupCreatedLabel formats the created timestamp for the list.
func configGroupCreatedLabel(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02")
}

// assignmentTypeLabel maps an assignment target_type onto the human
// label used in the Assignments table's Type column.
func assignmentTypeLabel(targetType string) string {
	switch targetType {
	case "asset":
		return "Computer"
	case "identity":
		return "User"
	case "group":
		return "Group"
	case "site":
		return "Site"
	case "tenant":
		return "Tenant"
	}
	return targetType
}

// assignmentKindChipClass reuses the target-kind chip color classes for
// the Type column (same hue family the TargetPickerDialog rows use).
func assignmentKindChipClass(targetType string) string {
	switch targetType {
	case "asset":
		return "cg-target-chip cg-target-computer"
	case "identity":
		return "cg-target-chip cg-target-user"
	case "group":
		return "cg-target-chip cg-target-computer"
	}
	return "cg-target-chip"
}

// assignmentIconKey — leading icon for an assignment row, mirroring
// configgroups.Target.IconKey's kind mapping.
func assignmentIconKey(targetType string) string {
	switch targetType {
	case "asset":
		return "target-computer"
	case "identity":
		return "target-user"
	case "group":
		return "target-computer-group"
	}
	return "inbox"
}

// bindingModuleLabel — the Module column: the override's title, or
// "Default (inherit)" when the binding carries no module pin.
func bindingModuleLabel(b ConfigGroupBindingRow) string {
	if b.ModuleTitle != "" {
		label := b.ModuleTitle
		if b.ModuleVersionPinned != "" {
			label += " @ " + b.ModuleVersionPinned
		}
		return label
	}
	return "Default (inherit)"
}

// bindingParamsSummary renders a binding's parameter_values JSON as a
// short "k=v, k=v" string for the Parameters column.
func bindingParamsSummary(raw string) string {
	if strings.TrimSpace(raw) == "" || raw == "{}" {
		return "—"
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	if len(m) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		b, _ := json.Marshal(m[k])
		parts = append(parts, k+"="+string(b))
	}
	return strings.Join(parts, ", ")
}

// configGroupAllowedTargetKinds — the TargetPickerDialog allow-list for
// the Assignments tab. KindConfigurationGroup is EXCLUDED: the
// configuration_group_assignments.target_type CHECK constraint
// (db/schema/001_initial.sql) only allows asset/identity/group/site/
// tenant — there is no "configuration_group" target_type, so
// group-composes-group targeting isn't representable at the assignment
// layer yet and the picker must not offer it (services.KindToTargetType
// rejects it server-side too).
func configGroupAllowedTargetKinds() []configgroups.TargetKind {
	return []configgroups.TargetKind{
		configgroups.KindComputer,
		configgroups.KindUser,
		configgroups.KindComputerGroup,
		configgroups.KindUserGroup,
	}
}

// configGroupHero builds the DetailShell hero.
func configGroupHero(d ConfigGroupDetailData) HeroSpec {
	enabledChip := Chip{Label: "Enabled", Class: "asset-chip-enroll-enrolled"}
	if d.Group.Disabled {
		enabledChip = Chip{Label: "Disabled", Class: "asset-chip-enroll-retired"}
	}
	return HeroSpec{
		Crumbs: []Crumb{
			{Label: "Policy", Href: "/policy/catalog"},
			{Label: "Configuration Groups", Href: "/policy/groups"},
			{Label: d.Group.Name},
		},
		Name: d.Group.Name,
		ID:   "cg-" + strconv.FormatInt(d.Group.ID, 10),
		Chips: []Chip{
			enabledChip,
			{Label: strconv.Itoa(len(d.Assignments)) + " assignment(s)", Class: ""},
			{Label: strconv.Itoa(len(d.Bindings)) + " binding(s)", Class: ""},
		},
		DeleteForm: configGroupDeleteAction(d),
	}
}

// configGroupTabs builds the DetailShell tab set.
func configGroupTabs(d ConfigGroupDetailData) []TabSpec {
	return []TabSpec{
		{Slug: "general", Label: "General", Body: configGroupGeneralTab(d)},
		{Slug: "assignments", Label: "Assignments", Body: configGroupAssignmentsTab(d)},
		{Slug: "bindings", Label: "Policy Bindings", Body: configGroupBindingsTab(d)},
	}
}
