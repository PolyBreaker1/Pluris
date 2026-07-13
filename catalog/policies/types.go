// Package policies is the Pluris Policy Catalog — the canonical, versioned
// list of settings exposed to admins in the Policy editor.
//
// Each entry is a Linux-native policy with the Windows Group Policy it
// corresponds to, so an admin migrating from AD/GP can find the same
// lever in the same place.
//
// This file only defines the structure. The catalog content lives in
// catalog_computer.go and catalog_user.go. The module engine (how a
// policy actually gets enforced on a device) is built on top in a later
// increment (see ADR-006 Policy Modules).
package policies

// Scope declares where a policy applies, matching Windows GP semantics:
//
//	ScopeComputer — applied at system level regardless of logged-in user
//	               (Computer Configuration branch).
//	ScopeUser     — applied per user session (User Configuration branch).
//	ScopeBoth     — meaningful on both branches; the UI shows it under
//	               both and enforcement picks the matching chain.
type Scope string

const (
	ScopeComputer Scope = "computer"
	ScopeUser     Scope = "user"
	ScopeBoth     Scope = "both"
)

// Policy is one row in the catalog.
type Policy struct {
	// ID is a stable slug, dotted, lowercase, unique across the catalog.
	// e.g. "sec.account.password.min-length".
	ID string

	// Name is the Pluris canonical name — phrased for Linux admins.
	Name string

	// WinGPName is the exact Windows Group Policy setting name. Used for
	// fuzzy match during GP import so legacy GPOs map onto Pluris policies
	// without manual rebind.
	WinGPName string

	// WinGPPath is the Windows GP tree location, pipe-separated, rooted at
	// "Computer Configuration" or "User Configuration". Used by GP import
	// and by the category navigator for parity with Windows GP.
	WinGPPath string

	// Category is the Pluris category path (leaf last). Parallels WinGPPath
	// but trimmed — no "Policies" / "Security Settings" duplicate rungs.
	Category []string

	// Scope — computer, user, or both.
	Scope Scope

	// Description is the admin-facing explanation (~1–3 sentences).
	// It must say what the policy does AND call out any Linux nuance.
	Description string

	// LinuxImpl is a short pointer at the Linux mechanism(s) the module
	// engine (Increment 5+) will use to enforce this policy. This is NOT
	// the implementation — it is a hint so admins know what tooling is
	// involved. Examples: "pam_pwquality", "faillock", "auditd",
	// "firewalld/nftables", "dconf lock", "systemd unit".
	LinuxImpl string

	// Custom marks tenant-authored catalog entries. Bundled (in-code)
	// entries are read-only; custom entries can be edited by the
	// originating tenant. There is no dedicated custom-policy authoring
	// UI today (the former Custom Policy Wizard was a pure stub — no
	// save path ever persisted — and was deleted; see
	// docs/history/specs/2026-07-12-module-persistence-and-param-injection.md).
	// The module editor (`/policy/modules/new`, `web/templates/policy_module_editor.templ`)
	// is the canonical place to author a policy module today; a
	// tenant-authored Catalog Policy entry would need a future editor of
	// its own. INV-M10: bundled and custom render in the same catalog
	// list, distinguished only by a small "Custom" chip.
	Custom bool

	// TenantID — non-empty only when Custom is true. Scopes the entry to
	// a single tenant; bundled entries are visible to all tenants.
	TenantID string
}

// Catalog is the full flat list of policies, in display order. Bundled
// entries (computer + user catalogs above) precede tenant-authored
// custom entries; the UI re-sorts within each category. INV-M10: the UI
// does not split bundled vs custom into separate tabs — they render in
// the same tree.
func Catalog() []Policy {
	out := make([]Policy, 0, len(computerPolicies)+len(userPolicies)+len(customPolicies))
	out = append(out, computerPolicies...)
	out = append(out, userPolicies...)
	out = append(out, customPolicies...)
	return out
}

// CategoryTree groups the catalog into a nested tree for the sidebar
// navigator. Order of traversal matches the catalog slice order.
type CategoryTree struct {
	Label    string
	Path     []string // full path from root
	Children []*CategoryTree
	Policies []Policy
}

// BuildTree assembles the tree from Catalog().
func BuildTree() *CategoryTree {
	root := &CategoryTree{Label: "Policies"}
	for _, p := range Catalog() {
		insertPolicy(root, p, p.Category)
	}
	return root
}

func insertPolicy(node *CategoryTree, p Policy, path []string) {
	if len(path) == 0 {
		node.Policies = append(node.Policies, p)
		return
	}
	head, rest := path[0], path[1:]
	for _, c := range node.Children {
		if c.Label == head {
			insertPolicy(c, p, rest)
			return
		}
	}
	child := &CategoryTree{
		Label: head,
		Path:  append(append([]string{}, node.Path...), head),
	}
	node.Children = append(node.Children, child)
	insertPolicy(child, p, rest)
}

// ScopeLabel — human label for a Scope.
func ScopeLabel(s Scope) string {
	switch s {
	case ScopeComputer:
		return "Computer"
	case ScopeUser:
		return "User"
	case ScopeBoth:
		return "Both"
	}
	return string(s)
}

// Counts summarises the catalog size for display.
type Counts struct {
	Total    int
	Computer int
	User     int
	Both     int
}

// CountBy returns summary counts.
func CountBy() Counts {
	var c Counts
	for _, p := range Catalog() {
		c.Total++
		switch p.Scope {
		case ScopeComputer:
			c.Computer++
		case ScopeUser:
			c.User++
		case ScopeBoth:
			c.Both++
		}
	}
	return c
}
