package policymodules

// testCatalog is the pre-Task-4.2 mock catalog, kept as a test-only
// fixture. Production code no longer serves this literal (see
// catalog.go: the live catalog now comes from the DB via
// pkg/services/policymodules.go's SeedBundled + CatalogProvider) --
// but resolver_test.go, defaults_test.go, and extension_adapter_test.go
// need a stable, deterministic catalog shape to exercise dependency
// resolution, tenant-default resolution, and the extension-framework
// adapter without standing up a database. Constructing Module literals
// inline in tests is explicitly fine (see Task 4.2 brief); this is just
// shared across three test files instead of copy-pasted three times.
func testCatalog() []Module {
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
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "#!/usr/bin/env bash\nset -euo pipefail\necho 'PasswordAuthentication no'\n"},
						{Phase: PhaseDisable, Filename: "disable.sh", Source: "# disable"},
						{Phase: PhaseUninstall, Filename: "rollback.sh", Source: "# rollback"},
						{Phase: PhaseValidate, Filename: "validate.wasm", Source: "(WASM module — read sshd_config, return JSON state)"},
					},
					ReportSchema: `{"type":"object","properties":{"sshd_running":{"type":"boolean"},"password_auth":{"type":"boolean"}}}`,
					Signature:    Signature{KeyID: "pluris:bundled:1", Algo: "ed25519", Signer: "Pluris (bundled)"},
					Status:       "published",
				},
				{
					Version: "1.1.0", PublishedAt: "2026-02-08T11:02:00Z", PublishedBy: "Pluris (bundled)",
					Sandbox: SandboxProfile{FsWrite: []string{"/etc/ssh/sshd_config.d/"}, User: "root"},
					Status:  "superseded",
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
					Sandbox: SandboxProfile{FsWrite: []string{"/etc/ssh/sshd_config.d/", "/etc/ssh/sshd_config"}, User: "root"},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "mkdir -p /etc/ssh/sshd_config.d\n"},
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
		{
			ID:          "tenant.acme.security-banner",
			Title:       "Corporate SSH login banner",
			Description: "Custom module authored by an Acme admin; drops a corporate banner into /etc/issue.net.",
			TargetOS:    []TargetOS{OSLinux},
			Scope:       "machine",
			Satisfies:   []string{"tenant.acme.ssh.banner"},
			Origin:      "tenant",
			Versions: []ModuleVersion{
				{
					Version: "0.4.0", PublishedAt: "2026-05-01T17:21:00Z", PublishedBy: "alice.chen@acme.local",
					Sandbox: SandboxProfile{FsWrite: []string{"/etc/issue.net"}, User: "root"},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "printf '%s\\n' \"$banner\" > /etc/issue.net\n"},
						{Phase: PhaseUninstall, Filename: "rollback.sh", Source: "rm -f /etc/issue.net"},
					},
					Signature: Signature{KeyID: "tenant:acme:key:1", Algo: "ed25519", Signer: "alice.chen@acme.local"},
					Status:    "published",
				},
				{
					Version: "0.5.0-draft", PublishedAt: "", PublishedBy: "alice.chen@acme.local",
					Status:  "draft",
					Sandbox: SandboxProfile{FsWrite: []string{"/etc/issue.net"}, User: "root"},
					Scripts: []LifecycleScript{
						{Phase: PhaseApply, Filename: "enforce.sh", Source: "# draft — adds language detection"},
					},
					Signature: Signature{},
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
						NetEgress: []string{"https://crl.tenant.local/"},
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
