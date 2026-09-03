// Package orientation is the single source of truth for how every
// admin-console concept is *introduced* to a Windows-trained admin.
//
// It powers two surfaces today:
//   - Empty-state copy for every list (templates.ConceptEmptyState reads
//     EmptyTitle / EmptyHelp / CreateLabel / CreateHref).
//   - The auto-generated docs/endpoint-management/windows-admins/concepts.md (run
//     `go run ./cmd/gendocs` to regenerate).
//
// Earlier iterations also drove a permanent orientation banner and
// sidebar subtitles; both were removed after review (2026-05-16) as
// redundant chrome. The richer fields (SidebarHint, ADEquivalent,
// LinuxMechanism, Summary) are retained for the docs generator and
// for future contextual surfaces (cell tooltips, detail dialogs).
//
// One concept = one row here. Adding a new top-level concept: append
// a Concept{} entry, reference its Key from the relevant page templ's
// @ConceptEmptyState call, then run `go run ./cmd/gendocs` to refresh
// the doc table. The same key is used in the docs URL fragment.
//
// Authoring rules (locked as INV-O1, INV-O3 in docs/endpoint-management/ui/invariants.md):
//   - ADEquivalent is short (≤ 60 chars). Use the *Windows admin's*
//     vocabulary: "AD OU + GP filtering", "Intune Configuration Profile",
//     "GPO". Say "No direct equivalent" when honest, never fudge.
//   - Summary is one sentence, ≤ 160 chars. Tells the admin what the
//     screen IS, not what it does.
//   - SidebarHint is ≤ 40 chars; gets clipped under the menu label.
//   - EmptyTitle starts with "No …"; EmptyHelp ends with the action the
//     CTA performs.
package orientation

// Concept describes one IA node — a sidebar item, a tab, or a logical
// "thing the admin manages". Every templ page that lives at a routable
// URL has exactly one Concept driving its orientation surfaces.
type Concept struct {
	// Key — stable slug. Matches the sidebar `Key`, the templ page's
	// orientation lookup, the docs anchor, and (later) the help-bot's
	// concept ID. Don't rename without a redirect map.
	Key string

	// Title — human title; same string the page header uses. Kept here
	// so the doc generator and the help bot don't have to reach into
	// the templ template to find it.
	Title string

	// SidebarHint — one short clarifying line under the sidebar label.
	// Empty for items that don't need one (Dashboard, Preferences).
	SidebarHint string

	// ADEquivalent — what this concept maps to in Microsoft Active
	// Directory / Group Policy / Intune vocabulary. Drives the right
	// chip in the orientation banner. "No direct equivalent — …" for
	// Pluris-specific concepts (Policy Module, Wine, Update Cycle).
	ADEquivalent string

	// Summary — one sentence shown in the orientation banner. Aimed at
	// a Windows admin who has never seen this screen before.
	Summary string

	// LinuxMechanism — one short line telling the admin "under the
	// hood, this is …". Optional; omit for high-level navigational
	// concepts (Dashboard, Preferences, Users).
	LinuxMechanism string

	// EmptyTitle / EmptyHelp — drive the empty-state component when a
	// list of this concept has zero rows. If empty, the empty-state is
	// suppressed (the underlying list doesn't naturally go to 0 — e.g.
	// the Policy Catalog ships seeded).
	EmptyTitle string
	EmptyHelp  string

	// CreateLabel / CreateHref — the action surfaced in the empty
	// state (and reused by the orientation banner's "Quick start"
	// link). Optional; lists with no admin-create flow leave these
	// blank (the catalog).
	CreateLabel string
	CreateHref  string

	// DocsAnchor — fragment under docs/endpoint-management/windows-admins/concepts.md
	// the "Learn more" link points to. Defaults to the Key when blank.
	DocsAnchor string
}

// All — every concept the admin console exposes. ORDER matches the
// sidebar so the auto-generated docs read top-to-bottom in the same
// shape an admin clicks the UI in.
var All = []Concept{
	{
		Key:          "dashboard",
		Title:        "Dashboard",
		SidebarHint:  "At-a-glance fleet status",
		ADEquivalent: "RSAT / Intune dashboard",
		Summary:      "Configurable tiles drawn from the Tenant → Site → Group → Asset hierarchy. Pick tile types and data sources per panel.",
	},

	{
		Key:            "users",
		Title:          "Users",
		SidebarHint:    "Identities + role assignment",
		ADEquivalent:   "AD Users and Computers (the user side)",
		Summary:        "Identity directory that will sync from Kanidm (planned identity backend) or added manually. Assign console roles, pair identities to assets, view per-user policy resolution.",
		LinuxMechanism: "Planned: Kanidm IDM (LDAP-compatible) + system PAM/SSSD on enrolled Linux hosts.",
		EmptyTitle:     "No users yet",
		EmptyHelp:      "Connect Kanidm (planned) or add a user manually. Once present, users can be targeted by policies and bound to assets.",
		CreateLabel:    "Add user",
		CreateHref:     "/users?new=1",
	},

	// Assets — single concept; subtype filter shows only one slice at a
	// time. AD vocab maps onto computers/servers as Computer accounts,
	// onto printers/desks as plain inventory CIs.
	{
		Key:            "assets",
		Title:          "Assets",
		SidebarHint:    "Computers, servers, printers, desks",
		ADEquivalent:   "AD Computer accounts + CMDB inventory",
		Summary:        "Single Asset entity with subtype filters. Same record powers policy targeting, software inventory, lifecycle, contract & warranty tracking.",
		LinuxMechanism: "pluris-agent on each enrolled host reports facts over NATS; static inventory rows for non-agent assets (printers, desks).",
		EmptyTitle:     "No assets enrolled",
		EmptyHelp:      "Enroll a host with the pluris-agent installer or add an inventory record for non-agent equipment (printers, desks).",
		CreateLabel:    "Enroll asset",
		CreateHref:     "/assets/computers?enroll=1",
	},

	{
		Key:          "policy",
		Title:        "Policy",
		SidebarHint:  "Settings + groups (≈ GPOs)",
		ADEquivalent: "Group Policy (GPMC)",
		Summary:      "Two surfaces: the Catalog (every available setting, GPEdit-style tree) and Configuration Groups (the rules that bind a setting to a target).",
	},
	{
		Key:            "policy-catalog",
		Title:          "Policy Catalog",
		SidebarHint:    "Every available setting",
		ADEquivalent:   "GPEdit setting browser",
		Summary:        "Browse every Windows GP setting alongside its Linux equivalent. Categorisation mirrors GPEdit so a search by Windows GP name finds it. Read-only; bind in a Configuration Group.",
		LinuxMechanism: "Each row carries its Linux mechanism column (PAM, AppArmor, systemd, dconf, …) and an optional Policy Module that performs the apply.",
		DocsAnchor:     "policy-catalog",
	},
	{
		Key:            "policy-groups",
		Title:          "Configuration Groups",
		SidebarHint:    "Bind settings to targets",
		ADEquivalent:   "GPO + GP Filtering / OU link",
		Summary:        "A Configuration Group binds one-or-more Catalog settings to a target — computer, user, group, or another Group's members.",
		LinuxMechanism: "Resolved at agent check-in: agent fetches the merged plan from the policy resolver and applies via the bound Policy Modules.",
		EmptyTitle:     "No configuration groups yet",
		EmptyHelp:      "Create one to bind catalog settings to a computer, user, or group. Equivalent of authoring a GPO and linking it.",
		CreateLabel:    "New configuration group",
		CreateHref:     "/policy/groups/new",
	},
	{
		Key:            "policy-modules",
		Title:          "Policy Modules — Library",
		SidebarHint:    "Signed enforcement bundles",
		ADEquivalent:   "No direct equivalent — closest is Intune custom OMA-URI",
		Summary:        "Versioned, signed bundles that translate a Catalog setting into actual Linux changes. Bundled modules ship with Pluris; tenant modules add or override.",
		LinuxMechanism: "module.yaml + enforce/validate/rollback scripts. Agent verifies the signature, runs the lifecycle phase, reports the result.",
		EmptyTitle:     "No tenant modules yet",
		EmptyHelp:      "Bundled modules cover the common settings out of the box. Add a tenant module only when you need a setting Pluris doesn't ship.",
		CreateLabel:    "New module",
		CreateHref:     "/policy/modules/new",
	},

	{
		Key:            "policy-modules-defaults",
		Title:          "Policy Modules — Tenant defaults",
		SidebarHint:    "Pick a module per policy",
		ADEquivalent:   "No direct equivalent — closest is a Group Policy Preference default",
		Summary:        "When more than one module satisfies the same policy, the tenant default decides which one is used by every binding (unless the binding explicitly overrides).",
		LinuxMechanism: "Resolution order at agent check-in: binding override → tenant default → Pluris default (first bundled module that satisfies). Implemented in catalog/policymodules.ResolveBindingModule (INV-M11).",
		EmptyTitle:     "No tenant defaults set",
		EmptyHelp:      "Pluris bundles a default module for every catalog policy it ships. You only need a tenant default when you have authored or imported an alternative.",
		CreateLabel:    "Set default",
		CreateHref:     "/policy/modules/defaults?new=1",
	},

	{
		Key:            "policy-modules-sources",
		Title:          "Policy Modules — Sources",
		SidebarHint:    "Where modules come from",
		ADEquivalent:   "No direct equivalent — closest is an Intune custom-detection script source",
		Summary:        "Sources are where the Library gets its modules from: Pluris-bundled (always on), Tenant-local (your tenant's own), and Imported registries (Phase 4; explicit fetch).",
		LinuxMechanism: "Adding a source doesn't import modules automatically — admin reviews and approves the diff before anything reaches the Library.",
		EmptyTitle:     "No imported registries yet",
		EmptyHelp:      "Pluris-bundled and Tenant-local are always present. Add an imported registry only if you want to pull modules from an external catalogue.",
		CreateLabel:    "Add source",
		CreateHref:     "/policy/modules/sources?new=1",
	},

	{
		Key:            "profiles",
		Title:          "Profiles",
		SidebarHint:    "Bundles applied to assets",
		ADEquivalent:   "Intune Configuration Profile",
		Summary:        "A Profile bundles Configuration Groups, Scripts, Wine configuration, and Packages. Assign one profile to a target instead of stitching components per host.",
		LinuxMechanism: "Server-side composition only; the agent receives the same merged settings whether you used Profiles or raw Configuration Groups.",
		EmptyTitle:     "No profiles yet",
		EmptyHelp:      "Create a profile to bundle groups, scripts, and packages and assign them as a unit. Useful for role-based rollouts (developer, kiosk, executive).",
		CreateLabel:    "New profile",
		CreateHref:     "/profiles?new=1",
	},

	{
		Key:            "scripts",
		Title:          "Scripts",
		SidebarHint:    "Ad-hoc admin scripts",
		ADEquivalent:   "GP Startup/Logon script + Intune script",
		Summary:        "Author or upload a script, choose a trigger (login, startup, schedule, on-demand), assign to a target. Bash, PowerShell (via pwsh), or Python.",
		LinuxMechanism: "Agent runs the script under a chosen user/system context with structured stdout capture for the run history.",
		EmptyTitle:     "No scripts yet",
		EmptyHelp:      "Add a script the next time you would have written a logon-script GPO or a one-off remediation in Intune.",
		CreateLabel:    "New script",
		CreateHref:     "/scripts?new=1",
	},

	{
		Key:            "wine",
		Title:          "Wine",
		SidebarHint:    "Run Windows apps on Linux",
		ADEquivalent:   "No direct equivalent — Pluris-specific",
		Summary:        "Manage Wine prefixes / Bottles / Proton runners centrally so Windows-only LOB apps run on Pluris Linux. Bind Wine Configuration Groups in a Profile.",
		LinuxMechanism: "Agent provisions the chosen Wine runtime, applies the prefix template, installs registry entries and shortcuts.",
		EmptyTitle:     "No Wine configurations yet",
		EmptyHelp:      "Create a Wine Configuration Group when you need to ship a Windows-only application — accounting suite, legacy CRM — to Pluris Linux endpoints.",
		CreateLabel:    "New Wine configuration",
		CreateHref:     "/wine?new=1",
	},

	{
		Key:          "packages",
		Title:        "Package Management",
		SidebarHint:  "Software inventory + delivery",
		ADEquivalent: "WSUS + Intune apps + Configuration Manager",
		Summary:      "Three surfaces: Package Managers (apt, dnf, snap, flatpak), Packages (the installed software inventory), Update Cycles (the rollout windows).",
	},
	{
		Key:            "packages-managers",
		Title:          "Package Managers",
		SidebarHint:    "apt / dnf / snap / flatpak",
		ADEquivalent:   "WSUS classifications + product list",
		Summary:        "Per-asset list of which package managers are enabled, their repos, signing keys, and the policy that gates them.",
		LinuxMechanism: "Agent reads /etc/apt/, /etc/yum.repos.d/, /var/lib/snapd, /var/lib/flatpak. Repos managed declaratively from policy.",
	},
	{
		Key:            "packages-packages",
		Title:          "Packages",
		SidebarHint:    "Installed software per asset",
		ADEquivalent:   "Intune Discovered apps",
		Summary:        "Cross-fleet software inventory. Filter by package, asset, manager, version. Schedule install / uninstall / hold.",
		LinuxMechanism: "Inventory built from each manager's local DB (dpkg, rpm, snap list, flatpak list); changes go through the Update Cycle gate.",
	},
	{
		Key:            "packages-cycles",
		Title:          "Update Cycles",
		SidebarHint:    "Rollout rings + windows",
		ADEquivalent:   "WSUS approval rings + maintenance windows",
		Summary:        "Pilot → Broad → Critical rings with their maintenance windows and rollback policy. Pin a Profile to a ring rather than approving each package individually.",
		LinuxMechanism: "Agent stays inside its ring's window for any apt/dnf/snap operation; out-of-window changes wait or, if marked critical, override.",
		EmptyTitle:     "No update cycles defined",
		EmptyHelp:      "Without cycles, every asset patches on its default schedule. Define rings (Pilot, Broad, Critical) to stagger rollouts and bound risk.",
		CreateLabel:    "New cycle",
		CreateHref:     "/packages/cycles?new=1",
	},

	{
		Key:          "server-admin",
		Title:        "Server Administration",
		SidebarHint:  "Tenant + console settings",
		ADEquivalent: "AD Sites & Services + Intune tenant settings",
		Summary:      "Tenant-wide configuration of the Pluris console itself: planned Kanidm connection, NATS endpoints, certificate authority, console roles, audit log retention.",
	},

	{
		Key:          "preferences",
		Title:        "Preferences",
		SidebarHint:  "",
		ADEquivalent: "MMC console preferences",
		Summary:      "Per-user UI preferences — default landing page, table density, saved column sets, keyboard shortcuts.",
	},
}

// Lookup returns the Concept for a given key. Returns the zero Concept
// when not found (callers should treat empty Title as "no orientation
// data" and skip the banner gracefully — failing visibly without
// crashing the page).
func Lookup(key string) Concept {
	for i := range All {
		if All[i].Key == key {
			return All[i]
		}
	}
	return Concept{}
}

// Found returns whether the key is registered. Used by templ
// conditionals to decide whether to render the banner.
func Found(key string) bool {
	return Lookup(key).Title != ""
}
