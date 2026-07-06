# Pluris - Quick Start Guide

## Prerequisites

- **Host OS**: Debian 12 or Ubuntu 22.04/24.04 LTS
- **Hardware**: 64GB+ disk space, 8GB+ RAM (16GB recommended)
- **Network**: Broadband internet for package downloads
- **Privileges**: sudo access (don't run scripts as root directly)

## Build Process Overview

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Setup Env     │ ──> │  Build Chroot   │ ──> │ Install Desktop │
│  (5 minutes)    │     │  (15 minutes)   │     │  (25 minutes)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
                                               ┌─────────────────┐
                                               │   Build ISO     │
                                               │  (10 minutes)   │
                                               └─────────────────┘
```

## Step-by-Step Instructions

### 1. Initial Setup

```bash
cd /home/dell/CascadeProjects/pluris

# Make scripts executable
chmod +x scripts/*.sh

# Install build tools
./scripts/setup-build-env.sh
```

This installs: `debootstrap`, `xorriso`, `qemu`, and other build dependencies.

### 2. Create Base System

```bash
./scripts/build-base-chroot.sh
```

Creates a minimal Debian 12 chroot at `build/chroot/`.

**What happens:**
- Downloads Debian base packages
- Installs Linux kernel and boot system
- Configures network, locale, timezone
- Creates default user account

### 3. Install Desktop & Apps

```bash
./scripts/install-desktop.sh
```

Installs KDE Plasma 6 with Windows-like layout and enterprise software.

**What gets installed:**
- **Desktop**: KDE Plasma 6, configured like Windows 11
- **Office**: Microsoft Edge + Office 365 PWA + OnlyOffice
- **Browser**: Edge + Firefox ESR
- **Wine**: WineHQ + Bottles for Windows apps
- **Enterprise**: realmd/sssd (AD join), Cockpit, security tools
- **Flatpak**: Signal, additional apps

### 4. Generate ISO

```bash
./scripts/build-iso.sh
```

Creates bootable hybrid ISO in `iso/` directory.

**Output:**
- `iso/pluris-1.0-amd64-YYYYMMDD.iso`
- `iso/pluris-1.0-amd64-YYYYMMDD.iso.sha256`
- `iso/pluris-1.0-amd64-YYYYMMDD.iso.md5`

### 5. Test (Optional)

The build script will offer to test with QEMU:

```bash
# Or test manually:
qemu-system-x86_64 -m 4096 -cdrom iso/pluris-1.0-amd64-*.iso -boot d
```

## First Boot After Install

1. **Install from ISO** to target system
2. **Complete first-boot wizard**:
   - Set hostname
   - Create user account
   - Configure timezone
   - Enable disk encryption (optional but recommended)
3. **Join domain** to Office 365** via Edge PWAs
4. **Sign in to Office 365** via Edge PWAs
5. **Install Windows apps** via Bottles if needed

## Customization

### Change Default Packages

Edit package lists in `pkglists/`:
- `base-system.txt` - Core system
- `desktop-environment.txt` - KDE Plasma
- `productivity.txt` - Office & browsers
- `windows-compatibility.txt` - Wine & Windows apps
- `enterprise-security.txt` - Security & management

### Modify Desktop Layout

Edit `scripts/pluris-apply-layout.sh` - Windows layout application script.

### Change Build Settings

Edit `config/build/pluris.conf`:
```bash
PLURIS_VERSION="1.0"           # Version string
DEBIAN_VERSION="bookworm"       # Debian release
ARCH="amd64"                   # Architecture
DESKTOP_ENVIRONMENT="kde"      # Desktop choice
```

### Add Custom APT Repos

Edit `config/apt/pluris.sources.list` - add company repositories.

## Common Issues

### Build fails with "debootstrap not found"
```bash
sudo apt-get install debootstrap live-build
```

### Chroot creation fails
- Check disk space: `df -h`
- Check network connectivity
- Try with verbose: `sudo debootstrap --verbose ...`

### Desktop packages fail
- Some packages may not exist in Debian stable
- Edit `pkglists/` to remove or replace them
- Check Debian packages: https://packages.debian.org/

### ISO too large
- Remove packages from lists
- Use more aggressive compression in `build-iso.sh`
- Remove documentation: `-e "usr/share/doc/*"` (already done)

### Windows apps don't work in Wine
- Use Bottles (installed via Flatpak) for easier management
- Check WineHQ AppDB for compatibility: https://appdb.winehq.org/
- Some apps may need Windows VM via KVM

## Enterprise Deployment

### Mass Deployment Options

1. **USB Sticks**: Create bootable USB with `dd`:
   ```bash
   sudo dd if=iso/pluris-*.iso of=/dev/sdX bs=4M status=progress
   ```

2. **PXE Boot**: Extract ISO to PXE server:
   ```bash
   # Extract vmlinuz, initrd.img, filesystem.squashfs
   # Configure PXE menu
   ```

3. **VM Template**: Install in VM, convert to template:
   ```bash
   # VMware/VirtualBox: Export as OVF
   # KVM: `virt-sparsify` + `qemu-img convert`
   ```

### Pre-seed Installation

For unattended install, create `preseed.cfg` and add to ISO boot.

### Group Policy

Use `sssd` with `ad_gpo_access_control = enforcing` for GPO support.

## Getting Help

- **Build logs**: Check terminal output, or add `2>&1 | tee build.log`
- **Chroot debugging**: `sudo chroot build/chroot bash`
- **Documentation**: See `docs/` directory
- **Upstream docs**:
  - Debian Live: https://wiki.debian.org/DebianLive
  - KDE Plasma: https://community.kde.org/
  - realmd/sssd: https://sssd.io/

## Next Steps

After successful build:

1. **Test on real hardware** - Check drivers, WiFi, graphics
2. **Pilot deployment** - 5-10 users for feedback
3. **Customize branding** - Add company wallpaper, logo
4. **Document internally** - Installation guide for your environment
5. **Plan training** - User training for new desktop
