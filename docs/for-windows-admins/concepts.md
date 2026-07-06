# Pluris concepts — what each screen IS

Each row is one menu item or tab in the admin console. Click into the screen to act on it; this page tells you what you're looking at, the AD/GP analogue, and the Linux mechanism behind it.

> AUTO-GENERATED from `web/orientation/orientation.go`. Edit there, then `go run ./cmd/gendocs`.

## Top-level concepts

| Concept | In Active Directory | One-line description |
|---|---|---|
| [Dashboard](#dashboard) | RSAT / Intune dashboard | Configurable tiles drawn from the Tenant → Site → Group → Asset hierarchy. Pick tile types and data sources per panel. |
| [Users](#users) | AD Users and Computers (the user side) | Identity directory synced from Kanidm or added manually. Assign console roles, pair identities to assets, view per-user policy resolution. |
| [Assets](#assets) | AD Computer accounts + CMDB inventory | Single Asset entity with subtype filters. Same record powers policy targeting, software inventory, lifecycle, contract & warranty tracking. |
| [Policy](#policy) | Group Policy (GPMC) | Two surfaces: the Catalog (every available setting, GPEdit-style tree) and Configuration Groups (the rules that bind a setting to a target). |
| [Policy Catalog](#policy-catalog) | GPEdit setting browser | Browse every Windows GP setting alongside its Linux equivalent. Categorisation mirrors GPEdit so a search by Windows GP name finds it. Read-only; bind in a Configuration Group. |
| [Configuration Groups](#policy-groups) | GPO + GP Filtering / OU link | A Configuration Group binds one-or-more Catalog settings to a target — computer, user, group, or another Group's members. |
| [Policy Modules — Library](#policy-modules) | No direct equivalent — closest is Intune custom OMA-URI | Versioned, signed bundles that translate a Catalog setting into actual Linux changes. Bundled modules ship with Pluris; tenant modules add or override. |
| [Policy Modules — Tenant defaults](#policy-modules-defaults) | No direct equivalent — closest is a Group Policy Preference default | When more than one module satisfies the same policy, the tenant default decides which one is used by every binding (unless the binding explicitly overrides). |
| [Policy Modules — Sources](#policy-modules-sources) | No direct equivalent — closest is an Intune custom-detection script source | Sources are where the Library gets its modules from: Pluris-bundled (always on), Tenant-local (your tenant's own), and Imported registries (Phase 4; explicit fetch). |
| [Profiles](#profiles) | Intune Configuration Profile | A Profile bundles Configuration Groups, Scripts, Wine configuration, and Packages. Assign one profile to a target instead of stitching components per host. |
| [Scripts](#scripts) | GP Startup/Logon script + Intune script | Author or upload a script, choose a trigger (login, startup, schedule, on-demand), assign to a target. Bash, PowerShell (via pwsh), or Python. |
| [Wine](#wine) | No direct equivalent — Pluris-specific | Manage Wine prefixes / Bottles / Proton runners centrally so Windows-only LOB apps run on Pluris Linux. Bind Wine Configuration Groups in a Profile. |
| [Package Management](#packages) | WSUS + Intune apps + Configuration Manager | Three surfaces: Package Managers (apt, dnf, snap, flatpak), Packages (the installed software inventory), Update Cycles (the rollout windows). |
| [Package Managers](#packages-managers) | WSUS classifications + product list | Per-asset list of which package managers are enabled, their repos, signing keys, and the policy that gates them. |
| [Packages](#packages-packages) | Intune Discovered apps | Cross-fleet software inventory. Filter by package, asset, manager, version. Schedule install / uninstall / hold. |
| [Update Cycles](#packages-cycles) | WSUS approval rings + maintenance windows | Pilot → Broad → Critical rings with their maintenance windows and rollback policy. Pin a Profile to a ring rather than approving each package individually. |
| [Server Administration](#server-admin) | AD Sites & Services + Intune tenant settings | Tenant-wide configuration of the Pluris console itself: Kanidm connection, NATS endpoints, certificate authority, console roles, audit log retention. |
| [Preferences](#preferences) | MMC console preferences | Per-user UI preferences — default landing page, table density, saved column sets, keyboard shortcuts. |

---

## Per-concept reference

### Dashboard

<a id="dashboard"></a>

| | |
|---|---|
| **In Active Directory** | RSAT / Intune dashboard |
| **Summary** | Configurable tiles drawn from the Tenant → Site → Group → Asset hierarchy. Pick tile types and data sources per panel. |
| **Sidebar hint** | At-a-glance fleet status |

### Users

<a id="users"></a>

| | |
|---|---|
| **In Active Directory** | AD Users and Computers (the user side) |
| **On Linux** | Kanidm IDM (LDAP-compatible) + system PAM/SSSD on enrolled Linux hosts. |
| **Summary** | Identity directory synced from Kanidm or added manually. Assign console roles, pair identities to assets, view per-user policy resolution. |
| **Sidebar hint** | Identities + role assignment |
| **Create action** | `Add user` → `/users?new=1` |
| **Empty state** | _"No users yet"_ — Connect Kanidm or add a user manually. Once present, users can be targeted by policies and bound to assets. |

### Assets

<a id="assets"></a>

| | |
|---|---|
| **In Active Directory** | AD Computer accounts + CMDB inventory |
| **On Linux** | pluris-agent on each enrolled host reports facts over NATS; static inventory rows for non-agent assets (printers, desks). |
| **Summary** | Single Asset entity with subtype filters. Same record powers policy targeting, software inventory, lifecycle, contract & warranty tracking. |
| **Sidebar hint** | Computers, servers, printers, desks |
| **Create action** | `Enroll asset` → `/assets/computers?enroll=1` |
| **Empty state** | _"No assets enrolled"_ — Enroll a host with the pluris-agent installer or add an inventory record for non-agent equipment (printers, desks). |

### Policy

<a id="policy"></a>

| | |
|---|---|
| **In Active Directory** | Group Policy (GPMC) |
| **Summary** | Two surfaces: the Catalog (every available setting, GPEdit-style tree) and Configuration Groups (the rules that bind a setting to a target). |
| **Sidebar hint** | Settings + groups (≈ GPOs) |

### Policy Catalog

<a id="policy-catalog"></a>

| | |
|---|---|
| **In Active Directory** | GPEdit setting browser |
| **On Linux** | Each row carries its Linux mechanism column (PAM, AppArmor, systemd, dconf, …) and an optional Policy Module that performs the apply. |
| **Summary** | Browse every Windows GP setting alongside its Linux equivalent. Categorisation mirrors GPEdit so a search by Windows GP name finds it. Read-only; bind in a Configuration Group. |
| **Sidebar hint** | Every available setting |

### Configuration Groups

<a id="policy-groups"></a>

| | |
|---|---|
| **In Active Directory** | GPO + GP Filtering / OU link |
| **On Linux** | Resolved at agent check-in: agent fetches the merged plan from the policy resolver and applies via the bound Policy Modules. |
| **Summary** | A Configuration Group binds one-or-more Catalog settings to a target — computer, user, group, or another Group's members. |
| **Sidebar hint** | Bind settings to targets |
| **Create action** | `New configuration group` → `/policy/groups?new=1` |
| **Empty state** | _"No configuration groups yet"_ — Create one to bind catalog settings to a computer, user, or group. Equivalent of authoring a GPO and linking it. |

### Policy Modules — Library

<a id="policy-modules"></a>

| | |
|---|---|
| **In Active Directory** | No direct equivalent — closest is Intune custom OMA-URI |
| **On Linux** | module.yaml + enforce/validate/rollback scripts. Agent verifies the signature, runs the lifecycle phase, reports the result. |
| **Summary** | Versioned, signed bundles that translate a Catalog setting into actual Linux changes. Bundled modules ship with Pluris; tenant modules add or override. |
| **Sidebar hint** | Signed enforcement bundles |
| **Create action** | `New module from policy wizard` → `/policy/modules?new=1` |
| **Empty state** | _"No tenant modules yet"_ — Bundled modules cover the common settings out of the box. Add a tenant module only when you need a setting Pluris doesn't ship. |

### Policy Modules — Tenant defaults

<a id="policy-modules-defaults"></a>

| | |
|---|---|
| **In Active Directory** | No direct equivalent — closest is a Group Policy Preference default |
| **On Linux** | Resolution order at agent check-in: binding override → tenant default → Pluris default (first bundled module that satisfies). Implemented in catalog/policymodules.ResolveBindingModule (INV-M11). |
| **Summary** | When more than one module satisfies the same policy, the tenant default decides which one is used by every binding (unless the binding explicitly overrides). |
| **Sidebar hint** | Pick a module per policy |
| **Create action** | `Set default` → `/policy/modules/defaults?new=1` |
| **Empty state** | _"No tenant defaults set"_ — Pluris bundles a default module for every catalog policy it ships. You only need a tenant default when you have authored or imported an alternative. |

### Policy Modules — Sources

<a id="policy-modules-sources"></a>

| | |
|---|---|
| **In Active Directory** | No direct equivalent — closest is an Intune custom-detection script source |
| **On Linux** | Adding a source doesn't import modules automatically — admin reviews and approves the diff before anything reaches the Library. |
| **Summary** | Sources are where the Library gets its modules from: Pluris-bundled (always on), Tenant-local (your tenant's own), and Imported registries (Phase 4; explicit fetch). |
| **Sidebar hint** | Where modules come from |
| **Create action** | `Add source` → `/policy/modules/sources?new=1` |
| **Empty state** | _"No imported registries yet"_ — Pluris-bundled and Tenant-local are always present. Add an imported registry only if you want to pull modules from an external catalogue. |

### Profiles

<a id="profiles"></a>

| | |
|---|---|
| **In Active Directory** | Intune Configuration Profile |
| **On Linux** | Server-side composition only; the agent receives the same merged settings whether you used Profiles or raw Configuration Groups. |
| **Summary** | A Profile bundles Configuration Groups, Scripts, Wine configuration, and Packages. Assign one profile to a target instead of stitching components per host. |
| **Sidebar hint** | Bundles applied to assets |
| **Create action** | `New profile` → `/profiles?new=1` |
| **Empty state** | _"No profiles yet"_ — Create a profile to bundle groups, scripts, and packages and assign them as a unit. Useful for role-based rollouts (developer, kiosk, executive). |

### Scripts

<a id="scripts"></a>

| | |
|---|---|
| **In Active Directory** | GP Startup/Logon script + Intune script |
| **On Linux** | Agent runs the script under a chosen user/system context with structured stdout capture for the run history. |
| **Summary** | Author or upload a script, choose a trigger (login, startup, schedule, on-demand), assign to a target. Bash, PowerShell (via pwsh), or Python. |
| **Sidebar hint** | Ad-hoc admin scripts |
| **Create action** | `New script` → `/scripts?new=1` |
| **Empty state** | _"No scripts yet"_ — Add a script the next time you would have written a logon-script GPO or a one-off remediation in Intune. |

### Wine

<a id="wine"></a>

| | |
|---|---|
| **In Active Directory** | No direct equivalent — Pluris-specific |
| **On Linux** | Agent provisions the chosen Wine runtime, applies the prefix template, installs registry entries and shortcuts. |
| **Summary** | Manage Wine prefixes / Bottles / Proton runners centrally so Windows-only LOB apps run on Pluris Linux. Bind Wine Configuration Groups in a Profile. |
| **Sidebar hint** | Run Windows apps on Linux |
| **Create action** | `New Wine configuration` → `/wine?new=1` |
| **Empty state** | _"No Wine configurations yet"_ — Create a Wine Configuration Group when you need to ship a Windows-only application — accounting suite, legacy CRM — to Pluris Linux endpoints. |

### Package Management

<a id="packages"></a>

| | |
|---|---|
| **In Active Directory** | WSUS + Intune apps + Configuration Manager |
| **Summary** | Three surfaces: Package Managers (apt, dnf, snap, flatpak), Packages (the installed software inventory), Update Cycles (the rollout windows). |
| **Sidebar hint** | Software inventory + delivery |

### Package Managers

<a id="packages-managers"></a>

| | |
|---|---|
| **In Active Directory** | WSUS classifications + product list |
| **On Linux** | Agent reads /etc/apt/, /etc/yum.repos.d/, /var/lib/snapd, /var/lib/flatpak. Repos managed declaratively from policy. |
| **Summary** | Per-asset list of which package managers are enabled, their repos, signing keys, and the policy that gates them. |
| **Sidebar hint** | apt / dnf / snap / flatpak |

### Packages

<a id="packages-packages"></a>

| | |
|---|---|
| **In Active Directory** | Intune Discovered apps |
| **On Linux** | Inventory built from each manager's local DB (dpkg, rpm, snap list, flatpak list); changes go through the Update Cycle gate. |
| **Summary** | Cross-fleet software inventory. Filter by package, asset, manager, version. Schedule install / uninstall / hold. |
| **Sidebar hint** | Installed software per asset |

### Update Cycles

<a id="packages-cycles"></a>

| | |
|---|---|
| **In Active Directory** | WSUS approval rings + maintenance windows |
| **On Linux** | Agent stays inside its ring's window for any apt/dnf/snap operation; out-of-window changes wait or, if marked critical, override. |
| **Summary** | Pilot → Broad → Critical rings with their maintenance windows and rollback policy. Pin a Profile to a ring rather than approving each package individually. |
| **Sidebar hint** | Rollout rings + windows |
| **Create action** | `New cycle` → `/packages/cycles?new=1` |
| **Empty state** | _"No update cycles defined"_ — Without cycles, every asset patches on its default schedule. Define rings (Pilot, Broad, Critical) to stagger rollouts and bound risk. |

### Server Administration

<a id="server-admin"></a>

| | |
|---|---|
| **In Active Directory** | AD Sites & Services + Intune tenant settings |
| **Summary** | Tenant-wide configuration of the Pluris console itself: Kanidm connection, NATS endpoints, certificate authority, console roles, audit log retention. |
| **Sidebar hint** | Tenant + console settings |

### Preferences

<a id="preferences"></a>

| | |
|---|---|
| **In Active Directory** | MMC console preferences |
| **Summary** | Per-user UI preferences — default landing page, table density, saved column sets, keyboard shortcuts. |

