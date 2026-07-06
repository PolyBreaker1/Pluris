// Package glossary is the single source of truth for every Linux /
// Pluris-specific term that surfaces in the admin console UI.
//
// Why centralized:
//   - A Windows-trained admin meeting "pam_pwquality" or "AppArmor" for
//     the first time loses 30 seconds to context-switching. A short
//     in-product gloss with the AD/Windows analogue keeps them in flow.
//   - The same data drives docs/for-windows-admins/glossary.md (the
//     `go run ./cmd/gendocs` target reads this map and emits a markdown
//     table). One edit, two surfaces.
//   - lists.glossifyTokens (see web/lists/policy_catalog.go) wraps each
//     recognised token in a `<span class="term">` carrying the OneLine
//   - ADEquivalent in the `title` attribute. The lint test in
//     glossary_test.go asserts every key has non-empty OneLine and
//     ADEquivalent.
//
// Authoring rules (locked as INV-O3):
//   - OneLine: ONE sentence in plain English. Avoid Linux jargon
//     defining Linux jargon; the audience is a Windows L1.
//   - ADEquivalent: the closest Microsoft-stack analogue ("≈ AppLocker
//     rules", "≈ Password Policy GPO", "= local-only equivalent of
//     LSA Secrets"). When honest, say "No close equivalent — …".
//   - Keys are the bare lowercase token the admin sees in the UI
//     (`pam_pwquality`, `apparmor`, `dconf`, `systemd-resolved`). Use
//     the canonical spelling — the lookup is case-insensitive but
//     storage is canonical.
package glossary

import "strings"

// Term is one glossary entry. Renders as a tooltip in the UI and a
// markdown table row in the docs.
type Term struct {
	// Key — canonical lowercase identifier as shown in the UI.
	Key string

	// OneLine — one sentence definition, plain English.
	OneLine string

	// ADEquivalent — the Windows-stack analogue, prefixed with
	// "≈ " for approximate matches or "= " for exact matches.
	ADEquivalent string

	// Category — used for the docs table grouping. Choose from:
	// "auth", "policy", "process", "filesystem", "network", "service",
	// "package", "ui", "pluris" (Pluris-specific term).
	Category string
}

// All — every term the console exposes. Keep alphabetical within each
// category for readability; the docs generator preserves insertion
// order to allow editorial grouping.
var All = []Term{
	// ---- Authentication / identity --------------------------------
	{Key: "pam", Category: "auth",
		OneLine:      "Pluggable Authentication Modules — the Linux subsystem that login, sudo, ssh and screen-lock all consult to decide if a credential is valid.",
		ADEquivalent: "≈ LSA + Authentication Packages on Windows."},
	{Key: "pam_pwquality", Category: "auth",
		OneLine:      "The PAM module that enforces password complexity (length, character classes, dictionary checks) before the password is accepted.",
		ADEquivalent: "= Password Policy GPO (Account Policies → Password Policy)."},
	{Key: "sssd", Category: "auth",
		OneLine:      "Service that bridges the local system to a remote identity provider (LDAP / Kerberos / Kanidm) — caches credentials, resolves users and groups.",
		ADEquivalent: "≈ Netlogon + the AD client side of a domain join."},
	{Key: "kanidm", Category: "auth",
		OneLine:      "The identity provider Pluris uses by default — replaces what AD does for users, groups, and authentication.",
		ADEquivalent: "≈ AD DS (Active Directory Domain Services), without the SMB/Kerberos legacy."},
	{Key: "polkit", Category: "auth",
		OneLine:      "The framework that decides whether a desktop app can elevate privilege for a single action (mount a disk, install a package).",
		ADEquivalent: "≈ UAC consent prompts, but per-action and per-app rather than per-process."},

	// ---- Policy / enforcement -------------------------------------
	{Key: "apparmor", Category: "policy",
		OneLine:      "Per-application sandbox: each profile lists the files, network, and capabilities that one program is allowed to use.",
		ADEquivalent: "≈ AppLocker / WDAC, except scoped per-application instead of by publisher / hash."},
	{Key: "selinux", Category: "policy",
		OneLine:      "Mandatory-access-control system that labels every file and process; the kernel enforces which labels can interact.",
		ADEquivalent: "≈ Mandatory Integrity Control on Windows, more comprehensive."},
	{Key: "dconf", Category: "policy",
		OneLine:      "GNOME's configuration database — the equivalent of HKEY_CURRENT_USER for the GNOME desktop.",
		ADEquivalent: "≈ Per-user registry hive (HKCU); GP \"User Configuration\" mostly maps here."},
	{Key: "udev", Category: "policy",
		OneLine:      "The kernel device manager — udev rules decide which USB / removable devices are allowed to appear and who can read them.",
		ADEquivalent: "≈ Removable Storage Access GP + Device Installation Restrictions."},

	// ---- Process / service ----------------------------------------
	{Key: "systemd", Category: "service",
		OneLine:      "The init + service manager. Every long-running thing on a Pluris host is a systemd unit.",
		ADEquivalent: "≈ Service Control Manager + Task Scheduler combined."},
	{Key: "systemd-resolved", Category: "service",
		OneLine:      "Local DNS resolver service — applies per-link DNS settings and DNS-over-TLS.",
		ADEquivalent: "≈ DNS Client service (Dnscache)."},
	{Key: "journald", Category: "service",
		OneLine:      "Structured system log database — every service's output, indexed and queryable by field.",
		ADEquivalent: "≈ Windows Event Log."},
	{Key: "cron", Category: "service",
		OneLine:      "Scheduler for periodic jobs. On Pluris hosts, systemd timers are the modern replacement but cron still works.",
		ADEquivalent: "≈ Scheduled Tasks."},

	// ---- Filesystem / storage -------------------------------------
	{Key: "luks", Category: "filesystem",
		OneLine:      "Linux's full-disk encryption layer — a passphrase-protected key wraps the disk encryption key.",
		ADEquivalent: "≈ BitLocker."},
	{Key: "autofs", Category: "filesystem",
		OneLine:      "Auto-mounts network shares on first access; unmounts on idle.",
		ADEquivalent: "≈ GP Drive Maps + Offline Files (the on-demand mount part)."},
	{Key: "fuse", Category: "filesystem",
		OneLine:      "Lets a regular user-space program present itself as a filesystem (used by sshfs, gvfs, etc.).",
		ADEquivalent: "≈ no direct equivalent; closest is a SMB/WebDAV redirector that runs in user space."},

	// ---- Network --------------------------------------------------
	{Key: "firewalld", Category: "network",
		OneLine:      "The host firewall service used by Pluris servers — zone-based, declarative.",
		ADEquivalent: "≈ Windows Defender Firewall with Advanced Security."},
	{Key: "nftables", Category: "network",
		OneLine:      "The kernel packet filter — what firewalld and ufw both ultimately program.",
		ADEquivalent: "≈ WFP (Windows Filtering Platform) at a lower level than the GUI firewall."},
	{Key: "networkmanager", Category: "network",
		OneLine:      "Per-host network configuration: wired/wireless, VPN, 802.1X, DNS overrides.",
		ADEquivalent: "≈ Network and Sharing Center + the MDM Wi-Fi/VPN profiles in Intune."},

	// ---- Package management ---------------------------------------
	{Key: "apt", Category: "package",
		OneLine:      "Package manager for Debian / Ubuntu hosts. Install / upgrade / remove .deb packages from configured repositories.",
		ADEquivalent: "≈ WSUS + Software Distribution (the server-side approval + apply loop)."},
	{Key: "dnf", Category: "package",
		OneLine:      "Package manager for Fedora / RHEL / Rocky hosts. Same role as apt for the .rpm world.",
		ADEquivalent: "≈ WSUS + Software Distribution (RPM family)."},
	{Key: "snap", Category: "package",
		OneLine:      "Containerized application packages with auto-update. Each snap runs in a confined sandbox.",
		ADEquivalent: "≈ Microsoft Store / MSIX (the sandboxed-app delivery part)."},
	{Key: "flatpak", Category: "package",
		OneLine:      "Containerized desktop application packages with portals for desktop integration.",
		ADEquivalent: "≈ Microsoft Store / MSIX, more desktop-Linux-focused."},

	// ---- UI / desktop ---------------------------------------------
	{Key: "gsettings", Category: "ui",
		OneLine:      "Command-line front-end to dconf — the way GNOME apps read and write their per-user configuration.",
		ADEquivalent: "≈ reg.exe for HKCU\\Software."},
	{Key: "gvfs", Category: "ui",
		OneLine:      "Virtual filesystem layer for the GNOME file manager — exposes SMB/SFTP/WebDAV/MTP as folders.",
		ADEquivalent: "≈ Explorer Network Locations + the WebClient service."},

	// ---- Pluris-specific ------------------------------------------
	{Key: "pluris-agent", Category: "pluris",
		OneLine:      "The endpoint daemon. Reports facts to the server over NATS, fetches the policy plan, runs Policy Modules, captures script output.",
		ADEquivalent: "≈ Intune MDM agent + Group Policy Client service combined."},
	{Key: "policy module", Category: "pluris",
		OneLine:      "A signed, versioned bundle (module.yaml + enforce / validate / rollback scripts) that implements ONE Policy Catalog setting on a Linux host.",
		ADEquivalent: "≈ Intune custom OMA-URI delivered as a structured package; no exact AD equivalent."},
	{Key: "configuration group", Category: "pluris",
		OneLine:      "The entity that binds Catalog settings (with values) to a target — a computer, user, group, or another Configuration Group's members.",
		ADEquivalent: "= GPO + GP filtering (security filtering + WMI filter), one screen instead of three."},
	{Key: "profile", Category: "pluris",
		OneLine:      "A bundle of Configuration Groups, Scripts, Wine config, and Packages assigned to a target as one unit. Useful for role-based rollouts.",
		ADEquivalent: "≈ Intune Configuration Profile."},
	{Key: "update cycle", Category: "pluris",
		OneLine:      "A patch ring (Pilot / Broad / Critical) with its maintenance window and rollback policy. Profiles pin to a ring rather than approving each package.",
		ADEquivalent: "= WSUS approval ring + maintenance window + auto-approval rule, in one screen."},
}

// Lookup returns the term for a key (case-insensitive). Returns the
// zero Term if not found; callers should treat empty OneLine as
// "render the bare token, no tooltip" so a missing entry degrades
// gracefully rather than crashing.
func Lookup(key string) Term {
	low := strings.ToLower(strings.TrimSpace(key))
	for i := range All {
		if strings.ToLower(All[i].Key) == low {
			return All[i]
		}
	}
	return Term{}
}

// Found reports whether the key resolves to a defined term.
func Found(key string) bool { return Lookup(key).OneLine != "" }

// ByCategory groups terms in declaration order. Used by the docs
// generator to emit one table per category.
func ByCategory() map[string][]Term {
	out := map[string][]Term{}
	for _, t := range All {
		out[t.Category] = append(out[t.Category], t)
	}
	return out
}
