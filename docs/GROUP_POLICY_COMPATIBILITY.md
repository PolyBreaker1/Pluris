> **Canonical-source notice (added 2026-05-05).** This document is the **research and architecture spec** for the GP translation layer (catalog, ADMX parser, translation tiers, ADR-003 enforcement). It is **not** authoritative on UI/IA. The GP editor and any UI element it touches must obey the invariants in:
>
> - `docs/Pluris UX structure plan.md` — UX/IA spec (user-authored).
> - `docs/UX_INVARIANTS.md` — formal IA contract.
>
> In particular: a Policy Group is one canonical concept with one canonical editor reachable from every entry point (Policy menu, Computer detail → Policy Groups tab, User detail → Policies, Profile editor, Server Admin → GP Import results). Any UI mockups in this document that imply forked / context-specific editors are **superseded** by the canonical-editor rule (INV-U1..U6).

---

# Group Policy Compatibility Layer - Research & Architecture

## 1. Why This Is a Killer Feature

**No one has done this properly yet.** The closest attempts:
- **Samba** has GP CSEs for Linux, but only works with a traditional AD DC, limited policy coverage, and no web UI
- **Himmelblau** enforces Intune policies on Linux, but only for Entra ID / Intune environments, not standalone
- **SSSD** can enforce some GPO access controls, but extremely limited scope

**Your opportunity**: Build the first standalone platform that speaks the GP language natively — import, translate, manage, and enforce GP-style policies across Linux AND Windows from a single web console. This alone could be the reason companies adopt your platform.

---

## 2. Existing Work to Build On

### 2.1 Samba Group Policy for Linux (CRITICAL FOUNDATION)

David Mulder (SUSE) has built a comprehensive GP framework for Linux in Samba:

**Already implemented Linux CSEs (Client Side Extensions):**

| GP Category | Linux Implementation | Config Target |
|------------|---------------------|---------------|
| Password policies | PAM / Kerberos | `/etc/security/`, krb5 |
| Firewall rules | firewalld | `firewall-cmd` |
| Sudoers rules | sudo config | `/etc/sudoers.d/` |
| Script execution | cron / systemd | Cron jobs, startup scripts |
| File deployment | File copy | Any path |
| Symlinks | Symlink creation | Any path |
| SSH configuration | sshd_config | `/etc/ssh/sshd_config.d/` |
| Firefox policies | JSON policy | `/etc/firefox/policies/` |
| Chrome/Chromium policies | JSON policy | `/etc/chromium/policies/` |
| GNOME settings | dconf | Desktop lockdown |
| PAM access | access.conf | `/etc/security/access.d/` |
| Certificate enrollment | certmonger | Auto-enroll certs |
| Login messages | MOTD/issue | `/etc/motd`, `/etc/issue` |

**Architecture**: Python-based CSEs inherit from `gp_ext` base class. Each CSE:
1. Reads policy data (from SYSVOL or Registry.pol files)
2. Applies settings to the Linux system
3. Tracks applied state (for rollback when policy is removed)
4. Non-tattooing — removing a GPO removes its effects

**Key insight**: These CSEs are modular Python files. We can extract the enforcement logic and drive it from our platform instead of requiring a Samba AD DC.

**Book resource**: https://dmulder.github.io/group-policy-book/ (complete reference)

### 2.2 Himmelblau (Intune on Linux)

- By the same developer (David Mulder)
- Rust-based, GPLv3
- Fork of Kanidm's auth stack, adapted for Entra ID
- Enforces Intune compliance policies: password, encryption, OS version, scripts
- PAM/NSS modules for auth
- Could potentially be integrated or referenced for Intune interop

### 2.3 ADMX/ADML File Format (GP Settings Definition)

ADMX files are **well-structured XML** that define every GP setting. They are the single source of truth for what the GP Editor displays.

**Structure of an ADMX file:**
```xml
<policyDefinitions>
  <policyNamespaces>
    <target prefix="appv" namespace="Microsoft.Policies.AppV" />
  </policyNamespaces>

  <categories>
    <category name="CAT_AppV" displayName="$(string.CAT_AppV)">
      <parentCategory ref="windows:CAT_WindowsComponents" />
    </category>
  </categories>

  <policies>
    <policy name="PasswordLength"
            class="Machine"
            displayName="$(string.PasswordLength)"
            explainText="$(string.PasswordLength_Help)"
            key="Software\Policies\..."
            valueName="MinPasswordLength">
      <parentCategory ref="CAT_Security" />
      <supportedOn ref="windows:SUPPORTED_Windows10" />
      <enabledValue><decimal value="1" /></enabledValue>
      <disabledValue><delete /></disabledValue>
      <elements>
        <decimal id="MinLen" valueName="MinLen"
                 minValue="8" maxValue="64" />
      </elements>
    </policy>
  </policies>
</policyDefinitions>
```

**ADML file (language strings):**
```xml
<policyDefinitionResources>
  <displayName>Security Settings</displayName>
  <resources>
    <stringTable>
      <string id="PasswordLength">Minimum Password Length</string>
      <string id="PasswordLength_Help">Sets the minimum number of characters...</string>
    </stringTable>
    <presentationTable>
      <presentation id="PasswordLength">
        <decimalTextBox refId="MinLen" defaultValue="8">
          Minimum characters:
        </decimalTextBox>
      </presentation>
    </presentationTable>
  </resources>
</policyDefinitionResources>
```

**Key elements in ADMX that map to UI controls:**
| ADMX Element | UI Control | Data Type |
|-------------|-----------|-----------|
| `<policy>` (enabled/disabled) | Toggle/checkbox | Boolean |
| `<text>` | Text input | String |
| `<decimal>` | Number spinner | Integer |
| `<boolean>` | Checkbox | Boolean |
| `<enum>` | Dropdown select | Enum |
| `<list>` | Key-value list | List |
| `<multiText>` | Multiline text | String[] |

**This means**: We can parse ADMX/ADML files and **automatically generate our web UI forms** for every GP setting. The ADMX file IS the UI definition.

---

## 3. Translation Layer Architecture

### 3.1 Core Concept

```
┌───────────────────────────────────────────────────┐
│           PLURIS POLICY CONSOLE (Web UI)           │
│                                                    │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────┐ │
│  │ GP-style    │  │ Linux-native│  │  Custom    │ │
│  │ categories  │  │ categories  │  │  policies  │ │
│  │ (familiar)  │  │ (new)       │  │  (YAML)   │ │
│  └──────┬──────┘  └──────┬──────┘  └─────┬─────┘ │
│         │                │               │        │
│         └────────┬───────┘───────────────┘        │
│                  ▼                                  │
│     ┌────────────────────────┐                     │
│     │   POLICY ENGINE        │                     │
│     │   (normalize all       │                     │
│     │    formats to internal │                     │
│     │    representation)     │                     │
│     └──────────┬─────────────┘                     │
│                │                                    │
├────────────────┼────────────────────────────────────┤
│                ▼                                    │
│     ┌────────────────────────┐                     │
│     │  TRANSLATION LAYER     │                     │
│     │                        │                     │
│     │  GP Setting ──────► Linux Enforcement        │
│     │                        │                     │
│     │  "Min password len=12" │                     │
│     │     ├──► PAM config    │                     │
│     │     ├──► Kanidm policy │                     │
│     │     └──► pwquality.conf│                     │
│     │                        │                     │
│     │  "Firewall: block 445" │                     │
│     │     └──► firewalld rule│                     │
│     │                        │                     │
│     │  "Disable USB storage" │                     │
│     │     └──► udev rule     │                     │
│     │         + kernel module│                     │
│     └──────────┬─────────────┘                     │
│                │                                    │
│     ┌──────────┴─────────────┐                     │
│     │  ENFORCEMENT BACKENDS  │                     │
│     │                        │                     │
│     │  Linux: Ansible/SSH    │                     │
│     │  Windows: native GPO   │                     │
│     │           or WinRM     │                     │
│     └────────────────────────┘                     │
└────────────────────────────────────────────────────┘
```

### 3.2 The GP-to-Linux Translation Map

This is the heart of the system — a curated mapping database:

```yaml
# Example translation map entries

- gp_category: "Computer Configuration > Security Settings > Account Policies > Password Policy"
  settings:
    - gp_name: "Minimum password length"
      gp_key: "Software\\Policies\\Microsoft\\Windows\\System\\MinimumPasswordLength"
      linux_translations:
        - target: pam
          action: "set_pam_pwquality"
          params:
            file: "/etc/security/pwquality.conf"
            key: "minlen"
          ansible_module: "community.general.pamd"
        - target: kanidm
          action: "api_call"
          endpoint: "/v1/domain/_attr/password_min_length"

    - gp_name: "Password must meet complexity requirements"
      linux_translations:
        - target: pam
          action: "set_pam_pwquality"
          params:
            dcredit: "-1"
            ucredit: "-1"
            lcredit: "-1"
            ocredit: "-1"

- gp_category: "Computer Configuration > Security Settings > Local Policies > Security Options"
  settings:
    - gp_name: "Interactive logon: Message text for users attempting to log on"
      linux_translations:
        - target: file
          action: "write_file"
          params:
            path: "/etc/issue"

    - gp_name: "Devices: Restrict removable storage access"
      linux_translations:
        - target: udev
          action: "udev_rule"
          params:
            rule: 'SUBSYSTEM=="usb", ATTR{bDeviceClass}=="08", ACTION=="add", RUN+="/bin/sh -c echo 0 > /sys/$devpath/authorized"'
        - target: kernel
          action: "modprobe_blacklist"
          params:
            modules: ["usb-storage", "uas"]

- gp_category: "Computer Configuration > Administrative Templates > Network > Firewall"
  settings:
    - gp_name: "Windows Firewall: Inbound rules"
      linux_translations:
        - target: firewalld
          action: "firewall_rule"
          ansible_module: "ansible.posix.firewalld"

- gp_category: "Computer Configuration > Administrative Templates > System > Logon"
  settings:
    - gp_name: "Run these programs at user logon"
      linux_translations:
        - target: autostart
          action: "xdg_autostart"
          params:
            dir: "/etc/xdg/autostart/"
        - target: systemd
          action: "systemd_user_service"

- gp_category: "User Configuration > Administrative Templates > Control Panel > Desktop"
  settings:
    - gp_name: "Desktop Wallpaper"
      linux_translations:
        - target: plasma
          action: "plasma_config"
          params:
            file: "plasma-org.kde.plasma.desktop-appletsrc"
            key: "Wallpaper/Image"
        - target: dconf
          action: "dconf_write"
          params:
            key: "/org/gnome/desktop/background/picture-uri"
```

### 3.3 Coverage Tiers

Not all GP settings have Linux equivalents. We categorize them:

**Tier 1 — Direct Translation (ship at launch)**
These have clear 1:1 Linux equivalents:

| GP Area | Linux Equivalent | Effort |
|---------|-----------------|--------|
| Password policies | PAM pwquality + Kanidm | Low |
| Account lockout | PAM faillock | Low |
| Firewall rules | firewalld | Low |
| SSH configuration | sshd_config.d | Low |
| Sudoers / privilege | sudoers.d | Low |
| Script execution | cron, systemd, autostart | Low |
| File deployment | File copy via Ansible | Low |
| Login banner/MOTD | /etc/issue, /etc/motd | Low |
| Firefox/Chrome policies | JSON policy files | Low |
| Certificate deployment | certmonger, trust anchors | Medium |
| Software installation | apt/dnf/flatpak | Medium |
| USB device control | udev + USBGuard | Medium |
| Disk encryption require | LUKS check via osquery | Medium |
| Auto-update policy | unattended-upgrades config | Low |

**Tier 2 — Approximate Translation (phase 2)**
These need adaptation but are conceptually similar:

| GP Area | Linux Approach |
|---------|---------------|
| Desktop lockdown | KDE KIOSK mode / dconf locks |
| Screen lock timeout | KDE/GNOME settings |
| Audit policy | auditd rules |
| AppLocker | AppArmor profiles |
| BitLocker | LUKS (different mechanism, same intent) |
| Windows Update | unattended-upgrades + custom |
| Folder redirection | Home directory structure + symlinks |
| Network drives | autofs / systemd mount units |
| Proxy settings | Environment vars + browser config |
| Power management | systemd power settings |

**Tier 3 — Windows-Only (passthrough for Windows clients)**
Applied natively to Windows devices, flagged as "Windows only" in UI:

| GP Area | Notes |
|---------|-------|
| Registry-based policies | Windows registry manipulation |
| COM/DCOM settings | No Linux equivalent |
| Windows Defender | Use ClamAV/Wazuh instead |
| IE/Edge specific | Use browser-neutral policies |
| .NET framework settings | N/A on Linux |
| ActiveX controls | N/A on Linux |

**Tier 4 — Custom / Linux-Native (our additions)**
Settings that don't exist in Windows GP but Linux admins need:

| Setting | Implementation |
|---------|---------------|
| AppArmor profile assignment | Per-app confinement |
| Flatpak permissions | Flatpak overrides |
| Snap store access | snapd config |
| Wayland/X11 selection | Display server config |
| Package repository sources | apt/dnf sources |
| Kernel parameters | sysctl.d |
| SELinux booleans | semanage |
| systemd service control | Enable/disable services |
| Cgroup resource limits | systemd resource control |

---

## 4. GPO Import / Migration

### 4.1 Import Formats

Companies can bring their existing policies in several ways:

**A. ADMX/ADML import (Template definitions)**
- Parse XML, generate our policy categories and UI
- Automatically creates the form fields in our web console
- No actual policy values — just the structure

**B. GPO Backup import (Actual policy values)**
- `Backup-GPO` PowerShell cmdlet exports GPOs to XML + Registry.pol
- Parse `GpoReport.xml` (full XML report of all settings)
- Parse `Registry.pol` (binary format, well-documented)
- Map each setting through our translation layer
- Show admin: "These X settings translated, Y need review, Z are Windows-only"

**C. Live AD sync (Bidirectional)**
- Connect to existing AD via LDAP
- Read GPOs from SYSVOL share
- Mirror policies in our platform
- Optionally push Linux-translated versions to Linux endpoints
- Keep Windows endpoints on native GP

**D. Intune policy import**
- Parse Intune configuration profiles (JSON via Graph API)
- Map to our internal representation
- Relevant for cloud-first organizations migrating away from Intune

### 4.2 Import Workflow UI

```
┌──────────────────────────────────────────────┐
│  IMPORT POLICIES                              │
│                                               │
│  Source:  ○ GPO Backup folder                │
│          ○ Connect to Active Directory        │
│          ○ ADMX template files                │
│          ○ Intune (Microsoft Graph)           │
│                                               │
│  [Browse...] or [Connect]                     │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │ Analysis Results                         │ │
│  │                                          │ │
│  │ ✅ 47 settings — direct translation     │ │
│  │ ⚠️  12 settings — approximate match     │ │
│  │ ❌  8 settings — Windows only           │ │
│  │ ℹ️   3 settings — manual review needed  │ │
│  │                                          │ │
│  │ [View Details] [Import All] [Select...]  │ │
│  └──────────────────────────────────────────┘ │
└──────────────────────────────────────────────┘
```

---

## 5. Web UI Design for Policy Editor

### 5.1 GP-Familiar Navigation

The web UI should mirror the GP Editor tree structure so Windows admins feel at home:

```
LEFT PANEL (tree):                    RIGHT PANEL (settings):
─────────────────                     ─────────────────────
📁 Computer Configuration            ┌─────────────────────────────┐
  📁 Security Settings               │ Minimum password length     │
    📁 Account Policies               │                             │
      📁 Password Policy        ──►  │ ○ Not Configured            │
      📁 Account Lockout              │ ● Enabled                   │
    📁 Local Policies                  │ ○ Disabled                  │
      📁 Audit Policy                  │                             │
      📁 Security Options             │ Minimum characters: [12]    │
  📁 Administrative Templates         │                             │
    📁 Network                         │ ┌─────────────────────────┐ │
      📁 Firewall                      │ │ Linux: PAM pwquality    │ │
    📁 System                          │ │ minlen = 12             │ │
      📁 Logon                         │ │                         │ │
      📁 Power Management             │ │ Windows: Registry       │ │
📁 User Configuration                │ │ MinimumPasswordLength=12│ │
  📁 Administrative Templates         │ └─────────────────────────┘ │
    📁 Desktop                         │                             │
    📁 Start Menu                      │ Applied to: [Device Group ▼]│
                                       │                             │
📁 Linux Extensions (NEW)            │ [Apply] [Test] [Revert]     │
  📁 AppArmor                         └─────────────────────────────┘
  📁 Systemd Services
  📁 Package Management
  📁 Kernel Parameters
```

### 5.2 Dual-Pane View

Each setting shows both the Windows GP name (familiar) and the Linux implementation (transparent):

```
┌─────────────────────────────────────────────┐
│ 🔒 Minimum Password Length                   │
│                                              │
│ GP Path: Computer Config > Security >        │
│          Account Policies > Password Policy  │
│                                              │
│ State:  ● Enabled  ○ Disabled  ○ Not Set    │
│ Value:  [12] characters                      │
│                                              │
│ ┌─ Platform Enforcement ──────────────────┐ │
│ │                                          │ │
│ │ 🐧 Linux:                               │ │
│ │   PAM: /etc/security/pwquality.conf      │ │
│ │   → minlen = 12                          │ │
│ │   Kanidm: password_min_length = 12       │ │
│ │   Status: ✅ Applied (3 devices)         │ │
│ │                                          │ │
│ │ 🪟 Windows:                              │ │
│ │   Registry: HKLM\...\MinPwdLength = 12  │ │
│ │   Status: ✅ Applied (7 devices)         │ │
│ │                                          │ │
│ │ 📊 Compliance: 10/10 devices (100%)     │ │
│ └──────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
```

---

## 6. Implementation Strategy

### 6.1 Phase approach

**Phase A: ADMX Parser + Translation Map (3-4 weeks)**
1. Build Go ADMX/ADML XML parser
2. Generate policy catalog from Microsoft's ADMX files (~3000+ settings)
3. Create initial translation map (Tier 1: ~50-80 most common settings)
4. Store in PostgreSQL: `gp_settings`, `gp_translations`, `gp_categories`
5. Web UI: browse GP tree, see which settings have Linux translations

**Phase B: Policy Editor UI (3-4 weeks)**
1. Auto-generate web forms from ADMX definitions
2. GP-style tree navigation
3. Enable/Disable/Not Configured states
4. Show Linux enforcement preview
5. Assign to devices/users/groups

**Phase C: Enforcement Engine (4-6 weeks)**
1. Generate Ansible playbooks from translated policies
2. Push via SSH to Linux endpoints
3. WinRM or native GPO push for Windows endpoints
4. Compliance checking via osquery
5. Rollback support (non-tattooing behavior)

**Phase D: GPO Import (2-3 weeks)**
1. Parse GPO backup XML
2. Parse Registry.pol binary format
3. Match settings against translation map
4. Import wizard with compatibility report
5. LDAP/AD live sync (stretch goal)

### 6.2 Database Schema (additions to OpenUEM fork)

```sql
-- GP settings catalog (parsed from ADMX)
CREATE TABLE gp_settings (
    id UUID PRIMARY KEY,
    admx_file TEXT,           -- e.g., "Windows.admx"
    policy_name TEXT,         -- e.g., "MinimumPasswordLength"
    display_name TEXT,        -- from ADML
    explain_text TEXT,        -- help text from ADML
    category_path TEXT,       -- full tree path
    class TEXT,               -- "Machine" or "User"
    registry_key TEXT,        -- Windows registry path
    value_name TEXT,
    supported_on TEXT,
    element_type TEXT,        -- boolean, decimal, text, enum, list
    element_definition JSONB, -- min/max values, enum items, etc.
    created_at TIMESTAMPTZ
);

-- Translation map: GP setting → Linux action
CREATE TABLE gp_translations (
    id UUID PRIMARY KEY,
    gp_setting_id UUID REFERENCES gp_settings(id),
    target_system TEXT,       -- "pam", "firewalld", "sshd", "kanidm", etc.
    action TEXT,              -- "set_config", "write_file", "api_call", etc.
    ansible_module TEXT,      -- which Ansible module to use
    ansible_params JSONB,     -- module parameters template
    tier INTEGER,             -- 1=direct, 2=approximate, 3=windows-only
    notes TEXT,
    verified BOOLEAN DEFAULT false
);

-- Policy instances (what admin configured)
CREATE TABLE policies (
    id UUID PRIMARY KEY,
    name TEXT,
    description TEXT,
    parent_id UUID REFERENCES policies(id),  -- inheritance
    created_by UUID,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

-- Individual policy settings values
CREATE TABLE policy_settings (
    id UUID PRIMARY KEY,
    policy_id UUID REFERENCES policies(id),
    gp_setting_id UUID REFERENCES gp_settings(id),
    state TEXT,               -- "enabled", "disabled", "not_configured"
    value JSONB,              -- the configured value
    updated_at TIMESTAMPTZ
);

-- Policy assignments
CREATE TABLE policy_assignments (
    id UUID PRIMARY KEY,
    policy_id UUID REFERENCES policies(id),
    target_type TEXT,         -- "device", "user", "group", "site", "org"
    target_id UUID,
    priority INTEGER,
    enforced BOOLEAN DEFAULT false,  -- like GP "enforced" flag
    created_at TIMESTAMPTZ
);

-- Compliance results
CREATE TABLE policy_compliance (
    id UUID PRIMARY KEY,
    device_id UUID,
    policy_id UUID REFERENCES policies(id),
    gp_setting_id UUID REFERENCES gp_settings(id),
    compliant BOOLEAN,
    actual_value JSONB,
    expected_value JSONB,
    checked_at TIMESTAMPTZ
);
```

---

## 7. Key Technical Considerations

### 7.1 ADMX Parsing is Straightforward
- ADMX files are well-formed XML with a documented schema
- Microsoft publishes all Windows ADMX files: https://learn.microsoft.com/en-us/troubleshoot/windows-client/group-policy/create-and-manage-central-store
- ~3000+ policy settings across ~150 ADMX files
- Go has excellent XML parsing (`encoding/xml`)

### 7.2 Registry.pol Parsing
- Binary format used in GPO backups
- Well-documented: header (PReg) + UTF-16LE key/value pairs
- Samba already has a Python parser: `samba.ndr.ndr_unpack(preg.file, data)`
- Easy to reimplement in Go

### 7.3 Samba CSE Code is Reusable
- Samba's Linux enforcement CSEs are Python files under GPLv3
- We can study the logic and reimplement in Go/Ansible
- Or call `samba-gpupdate` as subprocess on endpoints that have Samba installed
- Key CSE source: `samba/python/samba/gp/` in the Samba source tree

### 7.4 Translation Map is the Moat
- The curated GP→Linux translation database IS the product
- Start with top 80 most-used GP settings (covers 90% of real-world deployments)
- Community can contribute additional translations
- Each translation is verified and tested

---

## 8. Competitive Analysis

| Feature | Pluris (planned) | Samba GP | Himmelblau | Intune | GPMC |
|---------|-----------------|----------|------------|--------|------|
| Web UI for GP | ✅ | ❌ (GPMC only) | ❌ | ✅ | ✅ (MMC) |
| Linux enforcement | ✅ | ✅ | ✅ (limited) | ⚠️ (bad) | ❌ |
| Windows enforcement | ✅ | ✅ (via AD) | ❌ | ✅ | ✅ |
| Standalone (no AD) | ✅ | ❌ (needs AD) | ❌ (needs Entra) | ❌ | ❌ |
| GPO import | ✅ | N/A | ❌ | ❌ | N/A |
| GP-familiar UI | ✅ | N/A | ❌ | ⚠️ (different) | ✅ |
| Per-user policies | ✅ | ✅ | ❌ | ✅ | ✅ |
| Open source | ✅ | ✅ | ✅ | ❌ | ❌ |
| Translation map | ✅ | Implicit | ❌ | ❌ | N/A |
| Custom Linux policies | ✅ | ⚠️ | ❌ | ❌ | ❌ |

**Your unique position**: The only platform offering GP-compatible management WITHOUT requiring AD/Entra, working across Linux AND Windows, with a modern web UI.

---

## 9. Resources

| Resource | URL |
|----------|-----|
| Group Policy on Linux (book) | https://dmulder.github.io/group-policy-book/ |
| Samba GP wiki | https://wiki.samba.org/index.php/Group_Policy |
| Samba GP source (CSEs) | https://github.com/samba-team/samba/tree/master/python/samba/gp |
| ADMX schema reference | https://learn.microsoft.com/en-us/previous-versions/windows/desktop/policy/admx-schema |
| Microsoft ADMX downloads | https://learn.microsoft.com/en-us/troubleshoot/windows-client/group-policy/create-and-manage-central-store |
| Himmelblau | https://github.com/himmelblau-idm/himmelblau |
| Registry.pol format | https://learn.microsoft.com/en-us/previous-versions/windows/desktop/policy/registry-policy-file-format |
