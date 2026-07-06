# Cheatsheet — "I want to do X in GP. Where in Pluris?"

> One row per task. Tables only. If your task isn't here, search the Policy Catalog by Windows GP name first.

## Account & authentication

| You want to… | In Group Policy | In Pluris | Linux mechanism |
|---|---|---|---|
| Force complex passwords | Computer → Windows Settings → Security → Account Policies → Password Policy | Policy → Catalog → search `password` | `pam_pwquality` |
| Lock out after N failures | Computer → … → Account Lockout Policy | Policy → Catalog → search `lockout` | `pam_faillock` |
| Force screensaver lock | User → Admin Templates → Control Panel → Personalization | Policy → Catalog → search `screen lock` | `gsettings` (GNOME) / KDE config |
| Disable local-admin password reuse (LAPS) | Computer → … → LAPS | Policy → Catalog → search `local admin password` | `pam` + Kanidm rotation |
| Map a Kanidm group to local sudoers | Computer → Restricted Groups | Policy → Catalog → search `sudoers` | `/etc/sudoers.d/` via Policy Module |

## Software & updates

| You want to… | In Group Policy / Intune | In Pluris | Linux mechanism |
|---|---|---|---|
| Push an MSI to all workstations | Computer → Software Settings → Software installation | Profile → Packages tab → Add package | `apt` / `dnf` / `snap` / `flatpak` |
| Stagger patches Pilot → Broad → Production | WSUS approval ring + maintenance window | Package Management → Update Cycles | agent honours window |
| Block app from running | AppLocker / WDAC | Policy → Catalog → search `applocker` (resolves to AppArmor profile) | `apparmor` |
| Allow only signed apps | WDAC code-integrity policy | Policy → Catalog → search `code integrity` | `apparmor` + signed-package policy |
| Inventory installed software | Intune → Discovered apps | Package Management → Packages | `dpkg` / `rpm` / `snap list` / `flatpak list` |
| Run a Windows-only LOB app on Linux | (no GP analogue) | Wine → New configuration group, then Profile assignment | `wine` / `bottles` / `proton` |

## Devices & peripherals

| You want to… | In Group Policy | In Pluris | Linux mechanism |
|---|---|---|---|
| Block USB mass storage | Computer → Admin Templates → System → Removable Storage | Policy → Catalog → search `removable storage` | `udev` rules + mount policy |
| Allow only specific USB IDs | Device Installation Restrictions | Policy → Catalog → search `device installation` | `udev` allow-list |
| Disable Bluetooth | Computer → … → Bluetooth | Policy → Catalog → search `bluetooth` | `bluetoothctl` + `systemd` |
| Configure a printer | Printer Connections preference | Asset → Printers tab → bind to a Profile | CUPS via Policy Module |

## Network & VPN

| You want to… | In Group Policy | In Pluris | Linux mechanism |
|---|---|---|---|
| Set DNS servers per site | Network → DNS Client | Policy → Catalog → search `dns` | `systemd-resolved` |
| Enforce a Wi-Fi profile | Wireless Network (IEEE 802.11) Policies | Policy → Catalog → search `wifi` | `networkmanager` |
| Always-on VPN | Intune VPN profile | Policy → Catalog → search `vpn` | `networkmanager` / `wg-quick` |
| Disable a network adapter | Adapter Settings GP | Policy → Catalog → search `adapter disable` | `networkmanager` connection state |
| Open a host firewall port | Firewall Policy | Policy → Catalog → search `firewall` | `firewalld` |

## Identity & directory

| You want to… | In AD | In Pluris | Linux mechanism |
|---|---|---|---|
| Add a user | AD Users and Computers → New User | Users → Add user (or sync from Kanidm) | Kanidm |
| Reset a password | ADUC right-click → Reset Password | User → Reset password | Kanidm |
| Add user to a group | Group → Members → Add | User → Group memberships | Kanidm |
| Disable an account | ADUC → Disable Account | User → Disable | Kanidm |
| Pair a user to their primary computer | (manual or scripted) | Computer detail → Assigned users | Pluris asset link |

## Auditing & compliance

| You want to… | In GP / Defender | In Pluris | Linux mechanism |
|---|---|---|---|
| Audit logon events | Audit Policy | Policy → Catalog → search `audit logon` | `journald` + `auditd` |
| Forward logs to SIEM | Wef / Defender for Endpoint | Server Administration → Log forwarding | `journald` upload |
| Run an osquery check across the fleet | (Defender Advanced Hunting) | Phase 3 — osquery integration |  |
| Snapshot compliance daily | Intune compliance policy | Scripts → bundled `compliance-snapshot-daily` |  |

## Scripts & automation

| You want to… | In GP / Intune | In Pluris | Linux mechanism |
|---|---|---|---|
| Run a script at startup | GP Startup Scripts | Scripts → New script, trigger=startup | systemd unit (`Type=oneshot`) |
| Run a script at logon | GP Logon Scripts | Scripts → New script, trigger=login | PAM session hook |
| Run a script on a schedule | Scheduled Tasks GP / Intune script | Scripts → New script, trigger=schedule | systemd timer |
| Test a script before deploying | (manual on a test VM) | Preferences → Admin testing workstations → manual-trigger | agent runs against a designated host |

---

## What doesn't have a GP equivalent

Some Pluris concepts are Linux- or Pluris-specific. They earn their own one-liner in [concepts.md](concepts.md).

| Concept | Why it's new | Where to start |
|---|---|---|
| **Policy Module** | Linux apply needs a versioned, signed package; Windows just edits the registry. | Scripts → Policy Modules |
| **Wine Configuration Group** | No Windows-on-Windows analogue. | Wine |
| **pluris-agent** | Combines GP Client + Intune agent in one daemon. | Server Administration → Agent enrollment |
| **Update Cycle** | Pluris bakes WSUS approval rings + maintenance windows + auto-approval into one entity. | Package Management → Update Cycles |
