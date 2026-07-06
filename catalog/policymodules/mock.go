package policymodules

// Mock module catalog — populates the Scripts → Policy Modules list and
// the Configuration Group binding's module picker. Every module here is
// shaped exactly as the real backend will deliver it (modulo signature
// bytes which are stubbed). Editing this file is the v1 way to add new
// modules; once the Ent schemas land, this file becomes seed data.
//
// Selection criteria for what's mocked (so the UI exercises every code
// path):
//   - At least one module per major scope (machine / user).
//   - At least one with multiple versions (so the version picker shows).
//   - At least one with dependencies (resolver coverage).
//   - At least one with conflicts (block-binding path).
//   - At least one custom (tenant-origin) so the "Custom" badge shows.
//   - At least one bundled (read-only) so the gating works.
//   - At least one with each lifecycle phase combo to exercise the editor.

// AllModules — the catalog. Order is stable so list snapshots are
// deterministic; the UI re-sorts by (Origin, Title).
func AllModules() []Module {
	return []Module{
		// --- Bundled, machine scope, with conflicts -----------------
		{
			ID:          "pluris.sshd.password-auth-disable",
			Title:       "Disable SSH password authentication",
			Description: "Drops a snippet into /etc/ssh/sshd_config.d/ disabling password auth and reloads sshd. Idempotent; safe to re-apply.",
			TargetOS:    []TargetOS{OSLinux},
			Scope:       "machine",
			Satisfies: []string{
				"sec.remote-access.ssh.password-auth",
				"Computer/WindowsComponents/RemoteAccess/SSH/PasswordAuthDisable",
			},
			Origin: "bundled",
			Versions: []ModuleVersion{
				{
					Version: "1.2.0", PublishedAt: "2026-04-22T08:14:00Z", PublishedBy: "Pluris (bundled)",
					Sandbox: SandboxProfile{
						FsRead:  []string{"/etc/ssh/", "/proc/version"},
						FsWrite: []string{"/etc/ssh/sshd_config.d/"},
						User:    "root",
					},
					Dependencies: []Dependency{
						{ModuleID: "pluris.sshd.base-config", VersionConstraint: ">=1.0.0 <2.0.0"},
					},
					Conflicts: []string{"pluris.sshd.password-auth-allow"},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "" +
							"#!/usr/bin/env bash\nset -euo pipefail\n" +
							"# Disable password auth idempotently.\n" +
							"snippet=/etc/ssh/sshd_config.d/40-pluris-no-password.conf\n" +
							"want='PasswordAuthentication no'\n" +
							"echo \"$want\" > \"$snippet\"\nchmod 0644 \"$snippet\"\nsystemctl reload sshd\n"},
						{Phase: PhaseDisable, Filename: "disable.sh", Source: "" +
							"#!/usr/bin/env bash\nset -euo pipefail\n" +
							"snippet=/etc/ssh/sshd_config.d/40-pluris-no-password.conf\n" +
							"sed -i 's/^/# pluris-disabled: /' \"$snippet\" || true\nsystemctl reload sshd\n"},
						{Phase: PhaseUninstall, Filename: "rollback.sh", Source: "" +
							"#!/usr/bin/env bash\nset -euo pipefail\n" +
							"rm -f /etc/ssh/sshd_config.d/40-pluris-no-password.conf\nsystemctl reload sshd\n"},
						{Phase: PhaseValidate, Filename: "validate.wasm", Source: "(WASM module — read sshd_config, return JSON state)"},
					},
					ReportSchema: `{"type":"object","properties":{"sshd_running":{"type":"boolean"},"password_auth":{"type":"boolean"}}}`,
					Signature:    Signature{KeyID: "pluris:bundled:1", Algo: "ed25519", Signer: "Pluris (bundled)"},
					Status:       "published",
				},
				{
					Version: "1.1.0", PublishedAt: "2026-02-08T11:02:00Z", PublishedBy: "Pluris (bundled)",
					Sandbox:  SandboxProfile{FsWrite: []string{"/etc/ssh/sshd_config.d/"}, User: "root"},
					Status:   "superseded",
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "# 1.1.0 — superseded; kept for rollback"},
					},
					Signature: Signature{KeyID: "pluris:bundled:1", Algo: "ed25519", Signer: "Pluris (bundled)"},
				},
			},
		},

		// --- Bundled base dependency (no deps itself) ----------------
		{
			ID:          "pluris.sshd.base-config",
			Title:       "SSH base configuration",
			Description: "Ensures /etc/ssh/sshd_config.d/ exists and is loaded by sshd. Required by every other SSH-policy module.",
			TargetOS:    []TargetOS{OSLinux},
			Scope:       "machine",
			Satisfies:   []string{"sec.remote-access.ssh.base"},
			Origin:      "bundled",
			Versions: []ModuleVersion{
				{
					Version: "1.0.3", PublishedAt: "2026-03-18T16:40:00Z", PublishedBy: "Pluris (bundled)",
					Sandbox:  SandboxProfile{FsWrite: []string{"/etc/ssh/sshd_config.d/", "/etc/ssh/sshd_config"}, User: "root"},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "" +
							"#!/usr/bin/env bash\nset -euo pipefail\n" +
							"mkdir -p /etc/ssh/sshd_config.d\n" +
							"grep -q 'Include /etc/ssh/sshd_config.d/' /etc/ssh/sshd_config || " +
							"echo 'Include /etc/ssh/sshd_config.d/*.conf' >> /etc/ssh/sshd_config\n"},
					},
					Signature: Signature{KeyID: "pluris:bundled:1", Algo: "ed25519", Signer: "Pluris (bundled)"},
					Status:    "published",
				},
			},
		},

		// --- Bundled, conflicts with the disable module --------------
		{
			ID:          "pluris.sshd.password-auth-allow",
			Title:       "Allow SSH password authentication",
			Description: "Explicitly enables password auth (for migration windows). Conflicts with the disable module.",
			TargetOS:    []TargetOS{OSLinux},
			Scope:       "machine",
			Satisfies:   []string{"sec.remote-access.ssh.password-auth"},
			Origin:      "bundled",
			Versions: []ModuleVersion{
				{
					Version: "1.0.0", PublishedAt: "2026-01-12T09:00:00Z", PublishedBy: "Pluris (bundled)",
					Sandbox: SandboxProfile{FsWrite: []string{"/etc/ssh/sshd_config.d/"}, User: "root"},
					Dependencies: []Dependency{
						{ModuleID: "pluris.sshd.base-config", VersionConstraint: ">=1.0.0 <2.0.0"},
					},
					Conflicts: []string{"pluris.sshd.password-auth-disable"},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "echo 'PasswordAuthentication yes' > /etc/ssh/sshd_config.d/40-pluris-allow-password.conf"},
					},
					Signature: Signature{KeyID: "pluris:bundled:1", Algo: "ed25519", Signer: "Pluris (bundled)"},
					Status:    "published",
				},
			},
		},

		// --- Bundled, user scope (mounts a per-user systemd timer) ---
		{
			ID:          "pluris.user.screen-lock-timeout",
			Title:       "Set screen lock timeout (user)",
			Description: "Configures GNOME / Plasma idle-lock to a parameterised timeout. User-scope; runs as $target_user.",
			TargetOS:    []TargetOS{OSLinux},
			Scope:       "user",
			Satisfies:   []string{"sec.session.lock.idle-timeout"},
			Origin:      "bundled",
			Versions: []ModuleVersion{
				{
					Version: "2.0.1", PublishedAt: "2026-04-30T13:10:00Z", PublishedBy: "Pluris (bundled)",
					Sandbox: SandboxProfile{
						FsRead:  []string{"/etc/os-release"},
						FsWrite: []string{"$HOME/.config/dconf/", "$HOME/.config/plasma-locale-settings"},
						User:    "$target_user",
					},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "# read PLURIS_PARAMS_FD for idle-timeout, write dconf db"},
						{Phase: PhaseUninstall, Filename: "rollback.sh", Source: "# remove pluris dconf overrides"},
						{Phase: PhaseValidate, Filename: "validate.wasm", Source: "(WASM — return current idle timeout)"},
					},
					Signature: Signature{KeyID: "pluris:bundled:1", Algo: "ed25519", Signer: "Pluris (bundled)"},
					Status:    "published",
				},
			},
		},

		// --- Tenant-authored "custom" module --------------------------
		// Demonstrates: Origin=tenant, custom signer, draft state alongside
		// a published version. The UI gates "Edit" on Origin=tenant.
		{
			ID:          "tenant.acme.security-banner",
			Title:       "Corporate SSH login banner",
			Description: "Custom module authored by an Acme admin; drops a corporate banner into /etc/issue.net.",
			TargetOS:    []TargetOS{OSLinux},
			Scope:       "machine",
			Satisfies:   []string{"tenant.acme.ssh.banner"}, // points at a Custom Policy URN
			Origin:      "tenant",
			Versions: []ModuleVersion{
				{
					Version: "0.4.0", PublishedAt: "2026-05-01T17:21:00Z", PublishedBy: "alice.chen@acme.local",
					Sandbox: SandboxProfile{FsWrite: []string{"/etc/issue.net"}, User: "root"},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "" +
							"#!/usr/bin/env bash\nset -euo pipefail\n" +
							"banner=$(jq -r .banner_text < /proc/self/fd/3)\n" +
							"printf '%s\\n' \"$banner\" > /etc/issue.net\n"},
						{Phase: PhaseUninstall, Filename: "rollback.sh", Source: "rm -f /etc/issue.net"},
					},
					Signature: Signature{KeyID: "tenant:acme:key:1", Algo: "ed25519", Signer: "alice.chen@acme.local"},
					Status:    "published",
				},
				{
					Version: "0.5.0-draft", PublishedAt: "", PublishedBy: "alice.chen@acme.local",
					Status:    "draft",
					Sandbox:   SandboxProfile{FsWrite: []string{"/etc/issue.net"}, User: "root"},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "# draft — adds language detection"},
					},
					Signature: Signature{}, // unsigned; gates publish
				},
			},
		},

		// --- Bundled, network-egress example (phones home for CRL) ----
		{
			ID:          "pluris.cert.client-pin",
			Title:       "Pin corporate CA on workstations",
			Description: "Installs the corporate Root CA into the system trust store; periodically refreshes the CRL.",
			TargetOS:    []TargetOS{OSLinux},
			Scope:       "machine",
			Satisfies:   []string{"sec.tls.client.ca-pinning"},
			Origin:      "bundled",
			Versions: []ModuleVersion{
				{
					Version: "1.0.0", PublishedAt: "2026-04-12T10:33:00Z", PublishedBy: "Pluris (bundled)",
					Sandbox: SandboxProfile{
						FsRead:    []string{"/etc/os-release"},
						FsWrite:   []string{"/etc/pki/ca-trust/source/anchors/", "/usr/local/share/ca-certificates/"},
						NetEgress: []string{"https://crl.tenant.local/"}, // explicit allow
						User:      "root",
					},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "# install CA, refresh CRL"},
						{Phase: PhaseUninstall, Filename: "rollback.sh", Source: "# remove CA, regenerate trust store"},
						{Phase: PhaseReport, Filename: "report.wasm", Source: "(WASM — return CRL freshness JSON)"},
					},
					ReportSchema: `{"type":"object","properties":{"crl_age_seconds":{"type":"integer"},"ca_present":{"type":"boolean"}}}`,
					Signature:    Signature{KeyID: "pluris:bundled:1", Algo: "ed25519", Signer: "Pluris (bundled)"},
					Status:       "published",
				},
			},
		},
	}
}

// ByID — convenience accessor used by the dialog hydration path.
func ByID(id string) *Module {
	for _, m := range AllModules() {
		if m.ID == id {
			out := m
			return &out
		}
	}
	return nil
}

// CandidatesForPolicy — modules that satisfy the given catalog policy
// URN AND support at least one of the device's OSes. Used by the
// Configuration Group binding's module picker.
//
// `deviceOS` may be empty when the binding's target is a user/group
// without a specific device — in that case all OS-compatible modules
// are returned and the agent narrows at exec time.
func CandidatesForPolicy(urn string, deviceOS TargetOS) []Module {
	var out []Module
	for _, m := range AllModules() {
		if !m.SatisfiesURN(urn) {
			continue
		}
		if deviceOS != "" && !m.SupportsOS(deviceOS) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ----------------------------------------------------------------------
// Mock installations — used by Asset detail "Installed Modules" tab.
// ----------------------------------------------------------------------

// MockInstallations — three sample assets with overlapping module
// installations so the UI shows refcount > 1 (load-bearing modules) and
// dep-driven installations (e.g. base-config installed via dependency).
func MockInstallations() []ModuleInstallation {
	return []ModuleInstallation{
		// alice-laptop: SSH disable + its base-config dep.
		{
			ID: "install:1", AssetID: "asset:c2f1-alice-laptop",
			ModuleID: "pluris.sshd.password-auth-disable", ModuleVersion: "1.2.0",
			Reasons: []InstallReason{
				{BindingID: "binding:cg.baseline.workstations:0", AddedAt: "2026-04-25T09:00:00Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-04-25T09:00:34Z", LastValidated: "2026-05-06T08:00:00Z",
		},
		{
			ID: "install:2", AssetID: "asset:c2f1-alice-laptop",
			ModuleID: "pluris.sshd.base-config", ModuleVersion: "1.0.3",
			Reasons: []InstallReason{
				{DependentInstall: "install:1", AddedAt: "2026-04-25T09:00:00Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-04-25T09:00:31Z",
		},
		// build-server: SAME base-config (load-bearing — refcount=2 across
		// installations is misleading; the per-asset refcount on this row
		// is 2 because two reasons hold it: a direct binding AND a
		// dependent.
		{
			ID: "install:3", AssetID: "asset:aa01-build-server",
			ModuleID: "pluris.sshd.base-config", ModuleVersion: "1.0.3",
			Reasons: []InstallReason{
				{BindingID: "binding:cg.servers.ssh-baseline:0", AddedAt: "2026-04-20T07:30:00Z"},
				{DependentInstall: "install:4", AddedAt: "2026-04-20T07:30:02Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-04-20T07:30:10Z",
		},
		{
			ID: "install:4", AssetID: "asset:aa01-build-server",
			ModuleID: "pluris.sshd.password-auth-disable", ModuleVersion: "1.2.0",
			Reasons: []InstallReason{
				{BindingID: "binding:cg.servers.ssh-baseline:1", AddedAt: "2026-04-20T07:30:02Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-04-20T07:30:14Z",
		},
		// dev-workstation-12: corp banner (custom module).
		{
			ID: "install:5", AssetID: "asset:dd44-dev-ws-12",
			ModuleID: "tenant.acme.security-banner", ModuleVersion: "0.4.0",
			Reasons: []InstallReason{
				{BindingID: "binding:cg.acme.workstation-banners:0", AddedAt: "2026-05-02T12:11:00Z"},
			},
			State: InstStateApplied, AppliedAt: "2026-05-02T12:11:18Z",
		},
	}
}

// InstallationsForAsset — slice of installations on a given asset, in
// stable order. Used by the Asset detail "Installed Modules" tab.
func InstallationsForAsset(assetID string) []ModuleInstallation {
	var out []ModuleInstallation
	for _, mi := range MockInstallations() {
		if mi.AssetID == assetID {
			out = append(out, mi)
		}
	}
	return out
}
