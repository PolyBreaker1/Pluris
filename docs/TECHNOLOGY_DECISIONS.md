# Pluris - Critical Technology Decisions

## 1. Base OS Comparison

### Option A: Debian 12 (Bookworm) Stable ⭐ RECOMMENDED

**Pros:**
- **Enterprise-grade stability** - 5-year support cycle, extensively tested packages
- **Security** - Conservative update policy, security team responsiveness
- **Minimal by default** - Build exactly what you need, no bloat
- **Reproducible builds** - Critical for enterprise compliance
- **Universal architecture support** - Runs on anything from 10-year-old to latest hardware
- **No forced decisions** - You control every aspect of the system
- **Cost** - 100% free, no licensing concerns

**Cons:**
- **Older packages** - Software versions can be 1-3 years behind upstream
- **Hardware support** - May need backported drivers for very new hardware
- **Manual work** - Requires more configuration than Ubuntu/Fedora

**Best for:** Enterprises prioritizing stability, security, and long-term maintenance over bleeding-edge features.

---

### Option B: Ubuntu 24.04 LTS

**Pros:**
- **Hardware compatibility** - Excellent out-of-box driver support
- **HWE (Hardware Enablement)** - Backported kernels for newer hardware
- **Corporate backing** - Canonical's enterprise support options
- **Larger ecosystem** - More tutorials, community solutions
- **Snap/Flatpak** - Easy proprietary software installation
- **Landscape** - Built-in enterprise management (paid)
- **Pro subscriptions** - Extended security maintenance, live patching

**Cons:**
- **Snap controversy** - Forced snap usage, slower startup, centralized store
- **Phone-home** - Telemetry that must be disabled
- **Bloat** - Pre-installed software you'll need to remove
- **6-month release pressure** - Even LTS feels the churn

**Best for:** Companies wanting Canonical support or those with very diverse/new hardware.

---

### Option C: Fedora Workstation

**Pros:**
- **Bleeding edge** - Latest software versions, great for developer workstations
- **Wayland first** - Best Wayland implementation in Linux
- **Flatpak integration** - Native, non-controversial sandboxing
- **Security features** - SELinux by default, early adoption of new tech
- **Red Hat lineage** - Direct path to RHEL for servers

**Cons:**
- **13-month lifecycle** - Too short for enterprise desktops
- **Frequent updates** - Can break workflows, requires constant attention
- **No LTS equivalent** - Not designed for "install and forget" scenarios
- **Corporate desktop mismatch** - Built for developers, not typical office workers

**Best for:** Developer workstations, tech-forward companies, NOT general office use.

---

### Option D: Arch Linux

**Pros:**
- **Rolling release** - Always latest software
- **Complete control** - Build exactly what you want
- **Documentation** - Arch Wiki is the best Linux resource
- **AUR** - Massive user package repository

**Cons:**
- **No stability guarantees** - Updates can break systems
- **Manual maintenance** - Requires skilled Linux administrators
- **No enterprise tooling** - No AD integration, no management frameworks
- **Installation complexity** - CLI-based, expert-only

**Best for:** Power users, developers, enthusiasts. **NOT recommended for enterprise desktops.**

---

## Verdict: Debian 12 Stable

**Rationale:**
1. **Support timeline** - Debian 12 supported until ~2028, then can migrate to 13
2. **Predictability** - Critical for enterprise change management
3. **Security** - Conservative approach = fewer zero-day exposures
4. **Build flexibility** - Start minimal, add only what Pluris needs
5. **Backports available** - Can cherry-pick newer packages when needed
6. **No vendor lock-in** - Completely independent of Canonical/Red Hat decisions

---

## 2. Office 365 / Microsoft Office Compatibility Strategy

### The LibreOffice Problem
You're correct - LibreOffice UI feels dated and MS Office compatibility is imperfect:
- Complex documents lose formatting
- Macros don't work (VBA incompatible)
- Pivot tables, advanced Excel features break
- SharePoint/OneDrive integration absent
- Visual difference creates user friction

### Recommended Solutions (In Priority Order)

#### A. Microsoft Edge + Office 365 Web Apps (Primary) ⭐
**Implementation:**
```
- Install Microsoft Edge (Chromium-based, native Linux)
- Create PWA (Progressive Web Apps) for:
  - Outlook
  - Word
  - Excel
  - PowerPoint
  - Teams
- Pin to taskbar/dock like native apps
```

**Pros:**
- 100% feature parity with Windows Office 365
- Native Microsoft experience
- Auto-updates from Microsoft
- Full cloud integration (OneDrive, SharePoint)
- No Wine/translation layer needed

**Cons:**
- Requires internet (offline mode limited)
- Slightly different feel than desktop apps
- Heavy browser resource usage

**Reality check:** Most companies are already browser-first for Office 365. The desktop apps are increasingly irrelevant.

---

#### B. OnlyOffice Desktop Editors (Secondary)
**Installation:**
```
- Flatpak: flathub org.onlyoffice.desktopeditors
- Or native DEB package from onlyoffice.com
```

**Pros:**
- **Modern UI** - Ribbon interface like MS Office
- **Better compatibility** - Opens .docx, .xlsx, .pptx more accurately than LibreOffice
- **Free** - Open source, no licensing
- **Offline capable** - Full desktop application
- **Collaboration** - Connects to Nextcloud/ownCloud

**Cons:**
- Not 100% perfect (some advanced features missing)
- Smaller community than LibreOffice
- Plugin ecosystem weaker

---

#### C. SoftMaker Office (Commercial)
**Pricing:** ~$30/year or $100 perpetual

**Pros:**
- **Highest MS Office compatibility** - Specifically designed for this
- **Ribbon interface** - Familiar to Windows users
- **Polish language** - Excellent localization
- **Small footprint** - Fast, lightweight
- **FreeOffice** - Free version available (reduced features)

**Cons:**
- **Proprietary** - Closed source
- **Cost** - Not free
- **Smaller ecosystem** - Fewer templates/extensions

---

#### D. Wine + Microsoft Office (Not Recommended)
While possible to install real MS Office via Wine:

**Why avoid:**
- Licensing complexity (does company have transferable licenses?)
- Updates break frequently
- Performance overhead
- Security concerns (Windows code on Linux)
- Not supported by Microsoft
- Teams/OneDrive integration still problematic

---

### Recommended Hybrid Approach for Pluris

```
Tier 1: Microsoft Edge + Office 365 PWA (default for most users)
Tier 2: OnlyOffice Desktop (for offline-heavy users)
Tier 3: SoftMaker Office (optional paid tier for power users)
Fallback: LibreOffice (pre-installed, hidden from menu, emergency use)
```

---

## 3. Windows Application Compatibility (Beyond Office)

### Strategy: Layered Approach

#### Layer 1: Native Linux Replacements
Pre-install and configure Linux-native alternatives:
```
Windows App          Linux Replacement
----------------     -----------------
Notepad              Kate / Mousepad
Paint                KolourPaint / Pinta
Calculator           KCalc / GNOME Calc
Media Player         VLC
Photo Viewer         Gwenview
Terminal             Konsole
File Manager         Dolphin (Windows-like layout)
```

#### Layer 2: Wine + Bottles (For essential Windows apps)
```
- Install Bottles (modern Wine manager)
- Pre-configure common app environments:
  - .NET Framework bottle
  - Visual C++ Redistributables bottle
  - Legacy Windows app bottle
- Document which apps are certified to work
```

**Target success rate:** 80% of LOB (Line of Business) apps should work

#### Layer 3: Virtualization (For stubborn apps)
```
- VirtualBox or QEMU/KVM
- Pre-built Windows 10/11 VM template
- Seamless mode / spice for integration
- Shared folders, clipboard
```

#### Layer 4: Remote Desktop / Cloud PC
```
- Windows 365 Cloud PC integration
- Azure Virtual Desktop client
- RDP to company terminal servers
```

---

## 4. Desktop Environment: KDE Plasma 6 Deep Dive

### Why KDE over GNOME for Windows users?

| Feature | KDE Plasma | GNOME |
|---------|-----------|-------|
| Taskbar | ✅ Native, highly configurable | ❌ Requires extensions |
| Start Menu | ✅ Kickoff/Kicker like Windows | ❌ Activities overview |
| System Tray | ✅ Full support | ⚠️ Limited |
| Desktop icons | ✅ Native | ❌ Extension required |
| Right-click context | ✅ Full featured | ⚠️ Simplified |
| Window controls | ✅ Min/max/close right side | ⚠️ Left side default |
| Customization | ✅ Unlimited | ❌ Very limited |

### Recommended KDE Configuration for Pluris

```ini
# Layout: Windows 11 style
[Panel]
Position=bottom
Height=44px
Widgets=org.kde.plasma.kickoff, org.kde.plasma.icontasks, org.kde.plasma.systemtray, org.kde.plasma.digitalclock

# Task switcher: Thumbnail grid like Windows
[WindowSwitcher]
Layout=ThumbnailGrid

# Window decorations: Breeze with right-side buttons
[org.kde.kwin]
ButtonsOnRight=HIA__X
ButtonsOnLeft=

# Single-click disabled (Windows double-click behavior)
[KDE]
SingleClick=false
```

---

## 5. Enterprise Management Stack

### Active Directory Integration
```
Primary: realmd + sssd + oddjob-mkhomedir
    - Native AD join: realm join domain.com
    - User auth, group policies
    - Home directory creation

Alternative: PBIS Open (formerly Likewise)
    - More robust GPO support
    - Easier cross-domain trusts
    - GUI management tools
```

### Configuration Management
```
Tier 1: Ansible (recommended)
    - Agentless
    - YAML-based playbooks
    - Easy to audit

Tier 2: Landscape (if using Ubuntu base)
    - Canonical's commercial tool
    - Dashboard GUI

Tier 3: Puppet/Chef
    - If company already uses these
```

### Endpoint Management
```
- SSH + X11 forwarding: Remote admin
- KDE System Settings + KIOSK mode: Lock down desktops
- AppArmor: Enforce application confinement
- Firewalld: Centralized firewall management
```

---

## Summary: Technology Stack Decision

| Component | Selection | Alternative |
|-----------|-----------|-------------|
| **Base OS** | Debian 12 Stable | Ubuntu 24.04 LTS |
| **Desktop** | KDE Plasma 6 | GNOME 45 |
| **Office** | Edge + O365 PWA + OnlyOffice | SoftMaker Office |
| **Wine** | Bottles + Wine 9.0+ | PlayOnLinux |
| **AD Join** | realmd + sssd | PBIS Open |
| Management | Ansible + SSH | Landscape (paid) |
| **Security** | AppArmor + Firewalld | SELinux (Fedora) |
| **Updates** | unattended-upgrades | landscape-client |

---

## Next Actions

1. **Validate Office 365 PWA approach** - Test with actual company documents
2. **Test OnlyOffice** - Install and verify compatibility with your files
3. **Evaluate realmd vs PBIS** - Depends on your AD complexity
4. **Hardware audit** - Check if any hardware needs newer drivers (Debian vs Ubuntu decision point)
5. **Pilot group selection** - Identify 5-10 users for initial testing
