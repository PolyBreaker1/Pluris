package configgroups

// Target — one row in the unified target picker. The picker is a
// reusable cross-entity component: from a Configuration Group it picks
// any kind; from a "computer-only" caller it filters to KindComputer +
// KindComputerGroup; etc. Same component, different `allowedKinds`
// prop (INV-U2 / INV-U3).
//
// Status: in-memory mock that stitches together the existing per-kind
// option lists into a single searchable catalog. The real backend
// query lives behind the same shape so the UI stays unchanged.
type Target struct {
	Kind  TargetKind
	Ref   string
	Label string
	// Meta — secondary line shown beneath the label in the picker. Free-form
	// (email, OS, member count, parent group). Mock values; real values come
	// from the backend slice.
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

// AllTargets — the unified mock catalog (every pickable thing).
// Hand-curated to exercise every kind the dialog supports. Order is
// stable so list-test assertions are deterministic; the picker re-sorts
// by relevance once a query is typed.
func AllTargets() []Target {
	out := []Target{
		// --- Computers --------------------------------------------------
		{Kind: KindComputer, Ref: "asset:b5e8-lobby-pc", Label: "lobby-pc",
			Meta: "Ubuntu 24.04 · HQ · last seen 3m ago",
			Tags: []string{"linux", "kiosk", "lobby"}},
		{Kind: KindComputer, Ref: "asset:c2f1-alice-laptop", Label: "alice-laptop",
			Meta: "Fedora 40 · alice.chen · last seen 12s ago",
			Tags: []string{"linux", "executive"}},
		{Kind: KindComputer, Ref: "asset:aa01-build-server", Label: "build-server",
			Meta: "Debian 12 · CI · last seen 1m ago",
			Tags: []string{"linux", "ci", "infra"}},
		{Kind: KindComputer, Ref: "asset:dd44-dev-ws-12", Label: "dev-workstation-12",
			Meta: "Arch Linux · bob.martin · last seen 8m ago",
			Tags: []string{"linux", "developer", "workstation"}},
		{Kind: KindComputer, Ref: "asset:ff21-accounting-pc", Label: "accounting-pc",
			Meta: "Windows 11 · finance · last seen 22m ago",
			Tags: []string{"windows", "finance"}},

		// --- Users ------------------------------------------------------
		{Kind: KindUser, Ref: "alice.chen", Label: "Alice Chen",
			Meta: "alice.chen@acme.local · CEO",
			Tags: []string{"executives", "admin"}},
		{Kind: KindUser, Ref: "bob.martin", Label: "Bob Martin",
			Meta: "bob.martin@acme.local · Engineering",
			Tags: []string{"engineering", "developer"}},
		{Kind: KindUser, Ref: "carol.singh", Label: "Carol Singh",
			Meta: "carol.singh@acme.local · Sales",
			Tags: []string{"sales"}},
		{Kind: KindUser, Ref: "dave.kim", Label: "Dave Kim",
			Meta: "dave.kim@acme.local · Engineering",
			Tags: []string{"engineering"}},
		{Kind: KindUser, Ref: "admin", Label: "admin",
			Meta: "admin@acme.local · Built-in administrator",
			Tags: []string{"system"}},

		// --- Computer groups -------------------------------------------
		{Kind: KindComputerGroup, Ref: "all-computers", Label: "All computers",
			Meta: "Built-in · 87 members",
			Tags: []string{"builtin", "everyone"}},
		{Kind: KindComputerGroup, Ref: "workstations", Label: "Workstations",
			Meta: "62 members · linux + windows",
			Tags: []string{"workstations"}},
		{Kind: KindComputerGroup, Ref: "servers", Label: "Servers",
			Meta: "21 members · infra",
			Tags: []string{"servers", "infra"}},
		{Kind: KindComputerGroup, Ref: "kiosks", Label: "Kiosks",
			Meta: "4 members · public-facing",
			Tags: []string{"kiosks", "public"}},

		// --- User groups -----------------------------------------------
		{Kind: KindUserGroup, Ref: "all-users", Label: "All users",
			Meta: "Built-in · 142 members",
			Tags: []string{"builtin", "everyone"}},
		{Kind: KindUserGroup, Ref: "engineering", Label: "Engineering",
			Meta: "38 members · linux primary",
			Tags: []string{"engineering", "developers"}},
		{Kind: KindUserGroup, Ref: "sales", Label: "Sales",
			Meta: "24 members · road warriors",
			Tags: []string{"sales", "mobile"}},
		{Kind: KindUserGroup, Ref: "executives", Label: "Executives",
			Meta: "6 members · loopback enabled",
			Tags: []string{"executives", "loopback"}},

		// --- Configuration groups (composition) -------------------------
		// Generated from the existing MockGroups so changes flow through.
	}
	for _, g := range MockGroups {
		out = append(out, Target{
			Kind:  KindConfigurationGroup,
			Ref:   g.ID,
			Label: g.Name,
			Meta:  "Configuration Group · " + string(g.TargetKind) + " → " + g.TargetRef,
			Tags:  []string{"configuration-group", string(g.TargetKind)},
		})
	}
	return out
}

// TargetByRef — resolves a (kind, ref) pair to the cached Target row.
// Returns nil when nothing matches, which is fine for the dialog: it
// renders a placeholder "No target selected" chip.
func TargetByRef(kind TargetKind, ref string) *Target {
	if ref == "" {
		return nil
	}
	for _, t := range AllTargets() {
		if t.Kind == kind && t.Ref == ref {
			return &t
		}
	}
	return nil
}
