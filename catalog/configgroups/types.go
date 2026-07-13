// Package configgroups holds the pure domain types for Configuration
// Groups — the Pluris concept formerly called "Policy Group" in early
// docs. A Configuration Group binds a set of policy-catalog entries
// (with values) to targets (computers, users, groups).
//
// Status: persistence is real (configuration_groups + bindings +
// assignments tables via pkg/services/configgroups.go, Task 5.2 — the
// old in-memory mock slice and its popup dialog are retired). This
// package keeps the DB-free domain vocabulary: TargetKind (the picker's
// kind enum + labels), Target (picker row shape), and the legacy
// ConfigurationGroup/Binding value types. The resolver that merges
// groups into an effective policy set at agent evaluation time lands in
// a later increment.
//
// Loopback semantics (locked):
//
//	A User-targeted Configuration Group MAY carry both user- AND
//	computer-scope bindings. Computer-scope bindings from a user-targeted
//	group apply to the host the user is currently logged into, for the
//	duration of the session (Windows-GP "loopback/merge" analogue).
//	The editor UI labels these explicitly as "Session-host settings" so
//	the admin cannot confuse them with machine-targeted settings.
//
// This package has zero dependencies on templ/echo so it stays usable
// from tests and from future gRPC/HTTP API code.
package configgroups

// TargetKind enumerates what a Configuration Group is bound to. Exactly
// one kind applies to a given group; the paired TargetRef carries the
// identifier (asset uuid, identity slug, group slug, CG slug, regex).
//
// Reserved kinds are listed here so the enum survives new targets without
// a breaking rename. `KindRegex` renders in the UI as a disabled option
// until the matcher lands.
type TargetKind string

const (
	KindUnspecified        TargetKind = ""
	KindComputer           TargetKind = "computer"
	KindUser               TargetKind = "user"
	KindComputerGroup      TargetKind = "computer_group"
	KindUserGroup          TargetKind = "user_group"
	KindConfigurationGroup TargetKind = "configuration_group"
	KindRegex              TargetKind = "regex"
)

// AllTargetKinds — source-order list for dialog radio buttons / picker.
// KindRegex is included so the UI can render it as "(coming soon)".
var AllTargetKinds = []TargetKind{
	KindComputer,
	KindUser,
	KindComputerGroup,
	KindUserGroup,
	KindConfigurationGroup,
	KindRegex,
}

// Label — human-readable name for a TargetKind.
func (k TargetKind) Label() string {
	switch k {
	case KindComputer:
		return "Computer"
	case KindUser:
		return "User"
	case KindComputerGroup:
		return "Computer group"
	case KindUserGroup:
		return "User group"
	case KindConfigurationGroup:
		return "Configuration group (members)"
	case KindRegex:
		return "Regex match"
	}
	return string(k)
}

// HelpText — short description shown under the radio in the dialog.
func (k TargetKind) HelpText() string {
	switch k {
	case KindComputer:
		return "Apply to a single computer asset."
	case KindUser:
		return "Apply to one user. User-scoped settings follow the user across hosts; computer-scoped settings in this group apply to the session host while the user is logged in."
	case KindComputerGroup:
		return "Apply to every computer that is a member of the chosen group."
	case KindUserGroup:
		return "Apply to every user that is a member of the chosen group."
	case KindConfigurationGroup:
		return "Apply to the targets of another Configuration Group — use to layer refinements on top of an existing group."
	case KindRegex:
		return "Match by regex on hostname, UPN, or label (planned — not yet implemented)."
	}
	return ""
}

// IsEnabled — false for kinds that are declared but not yet implemented.
func (k TargetKind) IsEnabled() bool {
	return k != KindRegex
}

// ContentScope — whether a Configuration Group carries machine settings,
// user settings, or both. Always derived from the bindings present, not
// stored directly — but cached here for quick badges in the list view.
type ContentScope string

const (
	ContentComputer ContentScope = "computer"
	ContentUser     ContentScope = "user"
	ContentBoth     ContentScope = "both"
)

// Binding is one policy-catalog entry inside a Configuration Group.
// Values are the admin-supplied parameters (typed JSON in the final
// schema; map[string]string is fine for the mock).
type Binding struct {
	PolicyID string            // matches policies.Policy.ID — the URN.
	Values   map[string]string // admin-supplied values (stringified for mock).
	Disabled bool              // admin can toggle without removing.

	// ModuleOverride — when set, pins this binding to a specific
	// module + version, overriding the tenant default for this policy
	// URN (and the Pluris default below that).
	//
	// Resolution order at agent check-in:
	//   1. Binding-level override (this field, if set)
	//   2. Tenant default for the policy URN (see policymodules.TenantDefaults)
	//   3. Pluris default = highest-priority bundled module that satisfies
	//
	// Empty ModuleID = inherit. Empty ModuleVersion + non-empty ModuleID
	// = "latest published version of this module".
	ModuleOverride ModuleRef
}

// ModuleRef — a module + optional version pin. Lives on Binding and on
// the tenant-default mapping. Decoupled from policymodules.Module so
// this package stays free of the policymodules import (consumer code
// dereferences via the resolver).
type ModuleRef struct {
	ModuleID      string // dotted slug, "" = no override
	ModuleVersion string // semver, "" = latest published of ModuleID
}

// IsSet — true when the binding carries an explicit module pick.
func (r ModuleRef) IsSet() bool { return r.ModuleID != "" }

// ConfigurationGroup — one row in the list, one form in the dialog.
type ConfigurationGroup struct {
	ID          string // stable slug, e.g. "cg.baseline.workstations"
	Name        string
	Description string

	TargetKind TargetKind
	TargetRef  string // target identifier (asset uuid, user slug, group slug, CG id, regex)

	Bindings []Binding
	Disabled bool

	// Cached for list view; derived from Bindings in a helper below.
	contentScope ContentScope

	// Audit / housekeeping (mock values).
	UpdatedBy string
	UpdatedAt string // RFC3339, kept as string for the mock to avoid time import in templs.
}

// ContentScope — returns the cached scope, computing it lazily from
// bindings if not yet set. In the real backend this is a derived Ent
// column refreshed on write.
func (g *ConfigurationGroup) ContentScope() ContentScope {
	if g.contentScope != "" {
		return g.contentScope
	}
	return ContentBoth // mock: most sample groups carry both
}
