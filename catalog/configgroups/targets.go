package configgroups

// Target — one row in the unified target picker. The picker is a
// reusable cross-entity component: from a Configuration Group it picks
// any kind; from a "computer-only" caller it filters to KindComputer +
// KindComputerGroup; etc. Same component, different `allowedKinds`
// prop (INV-U2 / INV-U3).
//
// Rows are assembled from real, tenant-scoped data by
// services.TargetService.Catalog (pkg/services/targets.go) — this
// package stays free of DB/service dependencies (see the package doc in
// types.go), so it only owns the shape and the presentation helpers
// below.
type Target struct {
	Kind  TargetKind
	Ref   string
	Label string
	// Meta — secondary line shown beneath the label in the picker. Free-form
	// (email, OS, member count, parent group). Populated per kind by
	// TargetService.Catalog; see that function's doc comment for the exact
	// content per kind.
	Meta string
	// Tags — extra strings included in the search blob (site, role,
	// hostname …) but not visually rendered. Lets admins find a target by
	// any keyword without crowding the row.
	Tags []string
}

// IconKey — Lucide key used by the picker to render the leading icon.
// Centralised here so colour/icon stay in sync with TargetKind.
func (t Target) IconKey() string {
	switch t.Kind {
	case KindComputer:
		return "target-computer"
	case KindUser:
		return "target-user"
	case KindComputerGroup:
		return "target-computer-group"
	case KindUserGroup:
		return "target-user-group"
	case KindConfigurationGroup:
		return "target-config-group"
	case KindRegex:
		return "target-regex"
	}
	return ""
}
