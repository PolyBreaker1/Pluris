# Pluris Linux - Development Plan

## Vision
A secure, manageable, and reliable Windows replacement for enterprise/company PCs with modern UI and Windows software compatibility.

## Core Pillars
1. **Security** - Hardened defaults, minimal attack surface, automated updates
2. **Manageability** - Centralized administration, domain join, policy enforcement
3. **Reliability** - Stable base, tested updates, rollback capabilities
4. **Modern UX** - Clean, intuitive interface familiar to Windows users
5. **Compatibility** - Seamless Windows app support via Wine/Proton

---

## Phase 1: Foundation & Environment (Week 1-2)

### 1.1 Base OS Selection
**Recommended: Debian 12 (Bookworm) Stable**
- Rationale: Enterprise-grade stability, 5-year support cycle, massive package repo
- Alternative: Ubuntu 24.04 LTS (better hardware support, more "modern")
- Alternative: openSUSE Leap (excellent enterprise tools, YaST)

### 1.2 Build Environment
```
Host requirements:
- 64GB+ free disk space
- 8GB+ RAM (16GB recommended)
- Internet connection
- Debian/Ubuntu host (recommended)
```

Tools to install:
- `debootstrap` - base system creation
- `live-build` - ISO generation
- `chroot` environment for building
- `squashfs-tools` - compression
- `xorriso` - ISO creation
- `qemu/kvm` - testing

### 1.3 Project Structure
```
pluris/
├── build/           # Build artifacts
├── config/          # System configs
│   ├── apt/         # Package sources
│   ├── desktop/     # DE customization
│   ├── security/    # Hardening configs
│   └── management/  # Enterprise tools
├── pkglists/        # Package selections
├── scripts/         # Build automation
├── docs/            # Documentation
├── iso/             # Output ISOs
└── testing/         # QA scripts
```

---

## Phase 2: Core System Design (Week 3-4)

### 2.1 Desktop Environment
**Primary: KDE Plasma 6**
- Modern, customizable, Windows-like layout possible
- Excellent Wayland support
- Strong enterprise features (KIOSK mode)
- Alternative: GNOME 45+ (simpler, more opinionated)

### 2.2 Windows Compatibility Stack
```
Priority packages:
- winehq-stable or winehq-staging
- winetricks
- bottles (modern Wine frontend)
- proton (for games if needed)
- playonlinux (optional)
- q4wine (Wine GUI)
```

### 2.3 Security Framework
```
Base hardening:
- firewalld / ufw (firewall)
- fail2ban (intrusion prevention)
- clamav (antivirus)
- apparmor (mandatory access control)
- usbguard (USB device control)
- secureboot support
- full disk encryption (LUKS) default
- automatic security updates (unattended-upgrades)
```

### 2.4 Management Stack
```
Enterprise integration:
- realmd / sssd (Active Directory join)
- freeipa-client (alternative to AD)
- ansible (configuration management)
- landscape-client (Canonical's tool)
- puppet-agent (alternative)
- SSH server for remote management
```

---

## Phase 3: Package Selection (Week 5-6)

### 3.1 Base System Packages
See: `pkglists/base-system.txt`

### 3.2 Office/Productivity
```
- libreoffice + MS Office compatibility fonts
- thunderbird / evolution (email)
- firefox-esr (stable browser)
- nextcloud-client (cloud sync)
- teams-for-linux / slack (if available)
```

### 3.3 Multimedia & Extras
```
- vlc / mpv
- gimp
- evince (PDF)
- printing support (cups, hplip)
- bluetooth support
```

---

## Phase 4: Customization (Week 7-8)

### 4.1 Visual Design
- Windows-like taskbar (KDE Panel: Icons-only + Task Manager)
- Default wallpaper/branding
- GTK/Qt theme consistency
- Plymouth boot theme
- GRUB2 theme

### 4.2 First Boot Experience
- OEM installer (calamares)
- Welcome wizard
- Windows migration assistant
- Domain join wizard

### 4.3 Default Configurations
- Pre-configured firewall rules
- Automatic update policy
- Power management optimized for desktops
- Network defaults (DHCP, no WiFi autoconnect)

---

## Phase 5: Build System (Week 9-10)

### 5.1 ISO Generation
- Debian Live Build or custom debootstrap
- SquashFS compression
- Hybrid ISO (BIOS+UEFI)
- Persistent live option

### 5.2 Update/Repo Strategy
- Custom APT repository for pluris-specific packages
- Mirror of Debian repos (optional)
- OTA update mechanism

---

## Phase 6: Testing & QA (Week 11-12)

### 6.1 Compatibility Testing
- Windows app compatibility matrix
- Hardware compatibility (HCL)
- Network/AD integration tests

### 6.2 Security Audit
- CIS benchmark compliance
- Vulnerability scanning
- Penetration testing

### 6.3 User Acceptance
- Pilot deployment
- Feedback collection
- Iteration

---

## Recommended Technologies Summary

| Component | Recommendation | Alternative |
|-----------|----------------|-------------|
| Base OS | Debian 12 Stable | Ubuntu 24.04 LTS |
| Desktop | KDE Plasma 6 | GNOME 45 |
| Display | Wayland default | X11 fallback |
| Installer | Calamares | Debian Installer |
| AD Join | realmd + sssd | PBIS/Open |
| Firewall | ufw | firewalld |
| AV | clamav | (optional) |
| Wine | winehq-stable | bottles |
| Updates | unattended-upgrades | landscape |

---

## Immediate Next Steps
1. Install build dependencies on host
2. Create debootstrap base
3. Define package lists
4. Set up chroot environment
5. Test initial build

---

## License & Governance
- Base: Debian (DFSG compliant)
- Custom components: GPL-3.0+
- Branding assets: Proprietary/company-owned
