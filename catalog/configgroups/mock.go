package configgroups

// MockGroups — sample Configuration Groups used by the list page and the
// edit dialog until the Ent schema + resolver land. Chosen to cover every
// TargetKind except Regex (which renders as disabled).
var MockGroups = []ConfigurationGroup{
	{
		ID:          "cg.baseline.workstations",
		Name:        "Baseline — all workstations",
		Description: "Security baseline applied to every enrolled computer. Locks password length, enables firewall, enforces screen-lock on idle.",
		TargetKind:  KindComputerGroup,
		TargetRef:   "all-computers",
		Bindings: []Binding{
			{PolicyID: "sec.account.password.min-length", Values: map[string]string{"length": "14"}},
			{PolicyID: "sec.firewall.domain.state", Values: map[string]string{"state": "on"}},
			{PolicyID: "sec.firewall.private.state", Values: map[string]string{"state": "on"}},
			{PolicyID: "sec.firewall.public.state", Values: map[string]string{"state": "on"}},
		},
		UpdatedBy:    "admin@acme.local",
		UpdatedAt:    "2026-05-02T11:20:00Z",
		contentScope: ContentComputer,
	},
	{
		ID:          "cg.dev-team.loopback",
		Name:        "Dev team — developer stations",
		Description: "For the Engineering user group. Carries user-scope settings (desktop theme, Start menu) AND computer-scope settings that apply to the machine they are logged into (relaxed firewall, USB storage allowed).",
		TargetKind:  KindUserGroup,
		TargetRef:   "engineering",
		Bindings: []Binding{
			{PolicyID: "adm.desktop.wallpaper", Values: map[string]string{"path": "/usr/share/pluris/wp/dev.png"}},
			{PolicyID: "sec.firewall.public.state", Values: map[string]string{"state": "on"}},
			{PolicyID: "adm.system.removable-storage.deny-write", Values: map[string]string{"enabled": "false"}},
		},
		UpdatedBy:    "admin@acme.local",
		UpdatedAt:    "2026-05-03T09:05:00Z",
		contentScope: ContentBoth,
	},
	{
		ID:          "cg.kiosk.lobby-pc",
		Name:        "Kiosk — lobby PC",
		Description: "Single-asset lock-down for the reception kiosk. Removes Start-menu clutter and disables USB mass storage entirely.",
		TargetKind:  KindComputer,
		TargetRef:   "asset:b5e8-lobby-pc",
		Bindings: []Binding{
			{PolicyID: "adm.start.disable-context-menu", Values: map[string]string{"enabled": "true"}},
			{PolicyID: "adm.system.removable-storage.deny-all", Values: map[string]string{"enabled": "true"}},
		},
		UpdatedBy:    "admin@acme.local",
		UpdatedAt:    "2026-04-28T15:41:00Z",
		contentScope: ContentComputer,
	},
	{
		ID:          "cg.ceo.personal",
		Name:        "CEO — personal overrides",
		Description: "Single-user overrides applied wherever the CEO signs in. Includes a session-host machine setting that relaxes idle-lock timeout for board-room meetings.",
		TargetKind:  KindUser,
		TargetRef:   "alice.chen",
		Bindings: []Binding{
			{PolicyID: "adm.desktop.wallpaper", Values: map[string]string{"path": "/home/alice/wallpapers/board.png"}},
			{PolicyID: "sec.localpol.security-options.interactive-logon-machine-inactivity-limit", Values: map[string]string{"seconds": "3600"}},
		},
		UpdatedBy:    "admin@acme.local",
		UpdatedAt:    "2026-05-04T18:11:00Z",
		contentScope: ContentBoth,
	},
	{
		ID:          "cg.dev-team.plus-cli-power-tools",
		Name:        "Dev team — +CLI power tools",
		Description: "Extends the Dev team group with extra Linux CLI capabilities (sudoers tweaks, SSH keepalives). Targets the members of another Configuration Group.",
		TargetKind:  KindConfigurationGroup,
		TargetRef:   "cg.dev-team.loopback",
		Bindings: []Binding{
			{PolicyID: "adm.system.ssh-client-keepalive", Values: map[string]string{"seconds": "120"}},
		},
		UpdatedBy:    "admin@acme.local",
		UpdatedAt:    "2026-05-05T07:22:00Z",
		contentScope: ContentComputer,
	},
}

// All returns the full list (mock). Keeps the caller decoupled from the
// underlying storage decision (slice today, Ent query tomorrow).
func All() []ConfigurationGroup {
	out := make([]ConfigurationGroup, len(MockGroups))
	copy(out, MockGroups)
	return out
}

// ByID returns a group by slug, or nil if not found.
func ByID(id string) *ConfigurationGroup {
	for i := range MockGroups {
		if MockGroups[i].ID == id {
			g := MockGroups[i]
			return &g
		}
	}
	return nil
}

// Blank returns a new empty group suitable for the "New" dialog.
func Blank() ConfigurationGroup {
	return ConfigurationGroup{
		TargetKind: KindComputerGroup, // sensible default — covers 80% of real cases
	}
}

// MockTargetOptions — the choices the target-picker dropdown offers for
// a given TargetKind. Real implementation queries the backend; mock here
// keeps the dialog renderable.
func MockTargetOptions(k TargetKind) []TargetOption {
	switch k {
	case KindComputer:
		return []TargetOption{
			{Value: "asset:b5e8-lobby-pc", Label: "lobby-pc  (b5e8…)"},
			{Value: "asset:c2f1-alice-laptop", Label: "alice-laptop  (c2f1…)"},
			{Value: "asset:aa01-build-server", Label: "build-server  (aa01…)"},
		}
	case KindUser:
		return []TargetOption{
			{Value: "alice.chen", Label: "Alice Chen  (alice.chen@acme.local)"},
			{Value: "bob.martin", Label: "Bob Martin  (bob.martin@acme.local)"},
			{Value: "admin", Label: "admin  (admin@acme.local)"},
		}
	case KindComputerGroup:
		return []TargetOption{
			{Value: "all-computers", Label: "All computers"},
			{Value: "workstations", Label: "Workstations"},
			{Value: "servers", Label: "Servers"},
			{Value: "kiosks", Label: "Kiosks"},
		}
	case KindUserGroup:
		return []TargetOption{
			{Value: "all-users", Label: "All users"},
			{Value: "engineering", Label: "Engineering"},
			{Value: "sales", Label: "Sales"},
			{Value: "executives", Label: "Executives"},
		}
	case KindConfigurationGroup:
		out := make([]TargetOption, 0, len(MockGroups))
		for _, g := range MockGroups {
			out = append(out, TargetOption{Value: g.ID, Label: g.Name})
		}
		return out
	}
	return nil
}

// TargetOption is one row in a target-picker dropdown.
type TargetOption struct {
	Value string
	Label string
}

// LookupTargetLabel — resolves a TargetRef to the human-readable label
// used in the list view. Falls back to the raw ref when nothing matches
// (helpful while the backend is still mock).
func LookupTargetLabel(kind TargetKind, ref string) string {
	for _, o := range MockTargetOptions(kind) {
		if o.Value == ref {
			return o.Label
		}
	}
	return ref
}
