# Pluris glossary — Linux & Pluris terms for Windows admins

> **Orientation content.** This describes the Pluris experience Windows admins are aiming for, including features still in development. For what is built TODAY see docs/product/endpoint-management.md.

Every term you'll meet in the console, with its Active-Directory or Windows-stack analogue. Hover the same term in the UI to see the same gloss inline.

> AUTO-GENERATED from `web/glossary/glossary.go`. Edit there, then `go run ./cmd/gendocs`.

## Authentication & identity

| Term | One-liner | Windows / AD equivalent |
|---|---|---|
| `pam` | Pluggable Authentication Modules — the Linux subsystem that login, sudo, ssh and screen-lock all consult to decide if a credential is valid. | ≈ LSA + Authentication Packages on Windows. |
| `pam_pwquality` | The PAM module that enforces password complexity (length, character classes, dictionary checks) before the password is accepted. | = Password Policy GPO (Account Policies → Password Policy). |
| `sssd` | Service that bridges the local system to a remote identity provider (LDAP / Kerberos / Kanidm) — caches credentials, resolves users and groups. | ≈ Netlogon + the AD client side of a domain join. |
| `kanidm` | The planned identity backend Pluris is designed to use by default — will replace what AD does for users, groups, and authentication. | ≈ AD DS (Active Directory Domain Services), without the SMB/Kerberos legacy. |
| `polkit` | The framework that decides whether a desktop app can elevate privilege for a single action (mount a disk, install a package). | ≈ UAC consent prompts, but per-action and per-app rather than per-process. |

## Policy & enforcement

| Term | One-liner | Windows / AD equivalent |
|---|---|---|
| `apparmor` | Per-application sandbox: each profile lists the files, network, and capabilities that one program is allowed to use. | ≈ AppLocker / WDAC, except scoped per-application instead of by publisher / hash. |
| `selinux` | Mandatory-access-control system that labels every file and process; the kernel enforces which labels can interact. | ≈ Mandatory Integrity Control on Windows, more comprehensive. |
| `dconf` | GNOME's configuration database — the equivalent of HKEY_CURRENT_USER for the GNOME desktop. | ≈ Per-user registry hive (HKCU); GP "User Configuration" mostly maps here. |
| `udev` | The kernel device manager — udev rules decide which USB / removable devices are allowed to appear and who can read them. | ≈ Removable Storage Access GP + Device Installation Restrictions. |

## Services & scheduling

| Term | One-liner | Windows / AD equivalent |
|---|---|---|
| `systemd` | The init + service manager. Every long-running thing on a Pluris host is a systemd unit. | ≈ Service Control Manager + Task Scheduler combined. |
| `systemd-resolved` | Local DNS resolver service — applies per-link DNS settings and DNS-over-TLS. | ≈ DNS Client service (Dnscache). |
| `journald` | Structured system log database — every service's output, indexed and queryable by field. | ≈ Windows Event Log. |
| `cron` | Scheduler for periodic jobs. On Pluris hosts, systemd timers are the modern replacement but cron still works. | ≈ Scheduled Tasks. |

## Filesystem & storage

| Term | One-liner | Windows / AD equivalent |
|---|---|---|
| `luks` | Linux's full-disk encryption layer — a passphrase-protected key wraps the disk encryption key. | ≈ BitLocker. |
| `autofs` | Auto-mounts network shares on first access; unmounts on idle. | ≈ GP Drive Maps + Offline Files (the on-demand mount part). |
| `fuse` | Lets a regular user-space program present itself as a filesystem (used by sshfs, gvfs, etc.). | ≈ no direct equivalent; closest is a SMB/WebDAV redirector that runs in user space. |

## Networking

| Term | One-liner | Windows / AD equivalent |
|---|---|---|
| `firewalld` | The host firewall service used by Pluris servers — zone-based, declarative. | ≈ Windows Defender Firewall with Advanced Security. |
| `nftables` | The kernel packet filter — what firewalld and ufw both ultimately program. | ≈ WFP (Windows Filtering Platform) at a lower level than the GUI firewall. |
| `networkmanager` | Per-host network configuration: wired/wireless, VPN, 802.1X, DNS overrides. | ≈ Network and Sharing Center + the MDM Wi-Fi/VPN profiles in Intune. |

## Package management

| Term | One-liner | Windows / AD equivalent |
|---|---|---|
| `apt` | Package manager for Debian / Ubuntu hosts. Install / upgrade / remove .deb packages from configured repositories. | ≈ WSUS + Software Distribution (the server-side approval + apply loop). |
| `dnf` | Package manager for Fedora / RHEL / Rocky hosts. Same role as apt for the .rpm world. | ≈ WSUS + Software Distribution (RPM family). |
| `snap` | Containerized application packages with auto-update. Each snap runs in a confined sandbox. | ≈ Microsoft Store / MSIX (the sandboxed-app delivery part). |
| `flatpak` | Containerized desktop application packages with portals for desktop integration. | ≈ Microsoft Store / MSIX, more desktop-Linux-focused. |

## Desktop & UI

| Term | One-liner | Windows / AD equivalent |
|---|---|---|
| `gsettings` | Command-line front-end to dconf — the way GNOME apps read and write their per-user configuration. | ≈ reg.exe for HKCU\Software. |
| `gvfs` | Virtual filesystem layer for the GNOME file manager — exposes SMB/SFTP/WebDAV/MTP as folders. | ≈ Explorer Network Locations + the WebClient service. |

## Pluris-specific concepts

| Term | One-liner | Windows / AD equivalent |
|---|---|---|
| `pluris-agent` | The endpoint daemon. Reports facts to the server over NATS, fetches the policy plan, runs Policy Modules, captures script output. | ≈ Intune MDM agent + Group Policy Client service combined. |
| `policy module` | A signed, versioned bundle (module.yaml + enforce / validate / rollback scripts) that implements ONE Policy Catalog setting on a Linux host. | ≈ Intune custom OMA-URI delivered as a structured package; no exact AD equivalent. |
| `configuration group` | The entity that binds Catalog settings (with values) to a target — a computer, user, group, or another Configuration Group's members. | = GPO + GP filtering (security filtering + WMI filter), one screen instead of three. |
| `profile` | A bundle of Configuration Groups, Scripts, Wine config, and Packages assigned to a target as one unit. Useful for role-based rollouts. | ≈ Intune Configuration Profile. |
| `update cycle` | A patch ring (Pilot / Broad / Critical) with its maintenance window and rollback policy. Profiles pin to a ring rather than approving each package. | = WSUS approval ring + maintenance window + auto-approval rule, in one screen. |

