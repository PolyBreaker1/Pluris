// Package lists is the Pluris column / field registry for every tabular
// list rendered in the admin console (Policy Catalog, Configuration
// Groups, Computers, Users, future). It locks INV-L: every list shares
// the same field-definition shape, the same column-picker UI, and the
// same persistence model. Adding a new column to a list is a single
// FieldDef append; future lists adopt the framework by registering
// their fields and providing a per-list cell renderer.
//
// Why a registry, not per-list HTML:
//   - The user's column-picker dialog and the policy-detail dialog read
//     the SAME source of truth, so a field added once is exposable in
//     every place automatically (R1 — single source of truth UI).
//   - Future cross-list features (saved views, CSV export, search-as-
//     you-type column suggestions) plug into the registry without
//     touching individual templates.
//
// Render dispatch is per-list (see policy_catalog.go) because each list
// has a typed row (policies.Policy, configgroups.Group, …) and Go's
// generics are awkward inside templ. The trade-off is one switch
// statement per list — acceptable; it lives next to the FieldDef list
// it dispatches for, so adding a field is a co-located 2-edit change.
package lists

import (
	"sort"
	"strings"
)

// FieldDef — one possible column in a list's table AND one possible row
// in the same list's detail-dialog identity section.
type FieldDef struct {
	// Key — stable, dotted slug. Persisted in localStorage and (later)
	// per-user prefs. Must never change for a given semantic field;
	// renames must use a new key + a migration.
	Key string

	// Label — short human label; appears in the <th>, the column-picker
	// checkbox, and the detail dialog's <dt>.
	Label string

	// Description — long-form tooltip in the column picker. One sentence,
	// imperative voice. Shown so admins know what a column means before
	// they enable it.
	Description string

	// Group — bucket the picker groups checkboxes by. Keys defined per
	// list; see PolicyCatalogGroups for the policy list's groups.
	Group string

	// Width — CSS width hint applied to <th> when visible. Empty = auto.
	// Use percentages so the table reflows when columns toggle.
	Width string

	// DefaultVisible — included in the visible set when the user has no
	// saved preference. Keep this small (4–6 columns); everything else
	// is opt-in.
	DefaultVisible bool

	// DetailOnly — true means the field renders in the detail dialog
	// only, never as a list column. Used for fields whose value is too
	// verbose for a row (long descriptions, source previews).
	DetailOnly bool

	// Sort — sort comparator type. Empty = not sortable. Recognized:
	//   "alpha" — case-insensitive string compare, default direction asc
	//   "num"   — numeric compare (parseFloat), default direction desc
	// Per-list cell renderers must emit a parallel sort value via the
	// list's Render*CellSort function (read into <td data-sort=...>).
	Sort string
}

// IsSortable — convenience for templates.
func (f FieldDef) IsSortable() bool { return f.Sort != "" }

// IsListEligible — true if the field can appear in a list table (i.e.
// not detail-only). The picker hides ineligible entries; the detail
// dialog ignores this flag.
func (f FieldDef) IsListEligible() bool { return !f.DetailOnly }

// FieldGroup — the picker groups its checkboxes by these. Each list
// declares its own ordered groups.
type FieldGroup struct {
	Key   string
	Label string
	// Hint — one-line context line shown at the group head in the picker.
	Hint string
}

// listSpec — internal registry entry. One per list ID.
type listSpec struct {
	id     string
	title  string
	groups []FieldGroup
	fields []FieldDef
}

// Canonical DOM list IDs. Keep in sync with the templ data-pluris-list
// / data-list-id attributes. INV-L9 says every list driven by the
// unified engine (web/static/lists.js) declares its id here so call
// sites avoid string literals.
//
// NOTE — only ListIDPolicyCatalog has a full FieldDef registry today
// (see policy_catalog.go's init()). The other two IDs are DOM-only:
// the cg-table and pm-table templates render their columns directly
// in templ rather than going through Register / FieldsFor. When their
// columns become user-configurable, add a Register() call and they
// graduate to "registry-backed".
const (
	ListIDConfigGroups  = "config-groups"
	ListIDPolicyModules = "policy-modules"
	ListIDAssets        = "assets"
	ListIDIdentities    = "users"
)

var registry = map[string]*listSpec{}

// Register — called from each list's init() to publish its fields.
// The framework deduplicates field keys (panics on collision) so a
// stale rename can't silently shadow an active field.
func Register(id, title string, groups []FieldGroup, fields []FieldDef) {
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if _, dup := seen[f.Key]; dup {
			panic("lists: duplicate field key '" + f.Key + "' for list '" + id + "'")
		}
		seen[f.Key] = struct{}{}
	}
	groupSet := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		groupSet[g.Key] = struct{}{}
	}
	for _, f := range fields {
		if _, ok := groupSet[f.Group]; !ok {
			panic("lists: field '" + f.Key + "' references unknown group '" + f.Group + "' for list '" + id + "'")
		}
	}
	registry[id] = &listSpec{id: id, title: title, groups: groups, fields: fields}
}

// FieldsFor — every registered field for the list, in declaration order.
// Returns nil if the list isn't registered (the templ template renders
// an empty <thead>/<tbody>, which is the right behaviour for unknown
// IDs — fail visibly without crashing the page).
func FieldsFor(id string) []FieldDef {
	if s, ok := registry[id]; ok {
		return s.fields
	}
	return nil
}

// GroupsFor — ordered group descriptors for the list.
func GroupsFor(id string) []FieldGroup {
	if s, ok := registry[id]; ok {
		return s.groups
	}
	return nil
}

// TitleFor — the human title shown in the column-picker dialog header.
func TitleFor(id string) string {
	if s, ok := registry[id]; ok {
		return s.title
	}
	return id
}

// ListEligibleFields — every list-eligible (non-detail-only) field. Used
// by the picker to populate its checkboxes.
func ListEligibleFields(id string) []FieldDef {
	all := FieldsFor(id)
	out := make([]FieldDef, 0, len(all))
	for _, f := range all {
		if f.IsListEligible() {
			out = append(out, f)
		}
	}
	return out
}

// DefaultVisibleKeys — the seed visible set for a fresh user. Returned
// as a comma-separated string so it can ride in a data-* attribute.
func DefaultVisibleKeys(id string) string {
	all := FieldsFor(id)
	keys := make([]string, 0, len(all))
	for _, f := range all {
		if f.DefaultVisible && f.IsListEligible() {
			keys = append(keys, f.Key)
		}
	}
	return strings.Join(keys, ",")
}

// FieldsByGroup — order-preserving grouping for the picker UI.
func FieldsByGroup(id string) [][]FieldDef {
	groups := GroupsFor(id)
	idx := make(map[string]int, len(groups))
	for i, g := range groups {
		idx[g.Key] = i
	}
	out := make([][]FieldDef, len(groups))
	for _, f := range FieldsFor(id) {
		if !f.IsListEligible() {
			continue
		}
		i, ok := idx[f.Group]
		if !ok {
			continue
		}
		out[i] = append(out[i], f)
	}
	for i := range out {
		sort.SliceStable(out[i], func(a, b int) bool { return out[i][a].Label < out[i][b].Label })
	}
	return out
}

// RegisteredLists — every list ID currently registered. Useful for
// debugging and (later) the user's "default views" preference page.
func RegisteredLists() []string {
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
