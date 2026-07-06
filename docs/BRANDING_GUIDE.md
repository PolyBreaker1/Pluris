# Pluris Branding Guide

## Overview

Pluris uses a cohesive visual identity across all system components. This guide documents the branding assets and their usage.

---

## Color Palette

### Primary Colors

| Color | Hex | Usage |
|-------|-----|-------|
| **Deep Blue** | `#0a1628` | Backgrounds, panels |
| **Background Alt** | `#0d1f35` | Cards, elevated surfaces |
| **Background Hover** | `#122a45` | Hover states |
| **Cyan Accent** | `#00d4ff` | Primary accent, highlights |
| **Cyan Hover** | `#33ddff` | Hover accent |
| **Cyan Active** | `#0099cc` | Active/pressed states |
| **White** | `#ffffff` | Primary text |
| **Dimmed** | `#7eb8d1` | Secondary text |

### Semantic Colors

| Color | Hex | Usage |
|-------|-----|-------|
| Success | `#00d4aa` | Success states |
| Warning | `#ffaa00` | Warnings |
| Error | `#ff4757` | Errors, critical |
| Info | `#00d4ff` | Information |

---

## Logo System

### Primary Logo
- **File**: `branding/svg/pluris-logo.svg`
- **Size**: 512x512
- **Usage**: Application icons, about dialogs, installer

### Symbolic Logo
- **File**: `branding/svg/pluris-logo-symbolic.svg`
- **Size**: 128x128 (scalable)
- **Usage**: System tray, small icons, symbolic representations

### Banner
- **File**: `branding/svg/pluris-banner.svg`
- **Size**: 800x200
- **Usage**: Website headers, documentation headers

---

## Visual Identity

### Design Philosophy

**Clean & Minimalistic**
- Deep blue backgrounds reduce eye strain
- Cyan accents provide modern tech aesthetic
- White text ensures readability
- Grid patterns suggest structure and enterprise reliability

**Windows-Familiar**
- Taskbar at bottom (not top/side like default KDE)
- Right-side window controls
- Double-click to open
- System tray with standard icons

**Enterprise-Ready**
- Professional, not playful
- Consistent branding across boot, login, desktop
- Subtle animations (not distracting)

---

## Component Branding

### 1. Boot (Plymouth)

**Location**: `branding/plymouth/`

| File | Description |
|------|-------------|
| `pluris.plymouth` | Theme definition |
| `pluris.script` | Animation script |
| `logo.svg` | Boot logo |
| `spinner.svg` | Loading indicator |
| `wallpaper.svg` | Background |

**Features**:
- Deep blue background with subtle grid
- Spinning cyan spinner
- Pulsing logo animation
- Progress bar for updates

### 2. Bootloader (GRUB)

**Location**: `branding/grub/`

| File | Description |
|------|-------------|
| `theme.txt` | Theme configuration |
| `logo.svg` | Menu logo |
| `background.svg` | Menu background |

**Features**:
- Deep blue menu background
- Cyan selection highlight
- Logo centered above menu
- Clean typography

### 3. Login (SDDM)

**Location**: `branding/sddm/`

| File | Description |
|------|-------------|
| `Main.qml` | QtQuick theme |
| `theme.conf` | Theme settings |
| `logo.svg` | Login screen logo |
| `wallpaper.svg` | Background |

**Features**:
- Blurred wallpaper background
- Clock and date display
- Centered login panel
- Cyan accent on focused fields
- Session selector (Wayland/X11)
- Power controls (shutdown/reboot)

**Wayland Default**: Pluris defaults to Wayland sessions for modern applications.

### 4. Desktop (KDE Plasma)

**Location**: `config/desktop/`

| File | Purpose |
|------|---------|
| `kdeglobals` | Color scheme, fonts |
| `kwinrc` | Window manager settings |
| `kcmfonts` | Font configuration |
| `plasma-org.kde.plasma.desktop-appletsrc` | Panel layout |
| `plasma-windows-layout.kwinrules` | Window rules |
| `wayland-default.conf` | Wayland preferences |

**Layout Features**:
- Bottom panel (Windows 11 style)
- Icons-only task manager
- Kickoff start menu (bottom-left)
- System tray (bottom-right)
- Digital clock with date
- Right-side window controls

---

## Application Integration

### OnlyOffice (Primary Office Suite)

**Integration Script**: `/usr/local/bin/pluris-setup-onlyoffice`

**Features**:
- Set as default for .docx, .xlsx, .pptx
- MS Office format defaults
- Right-click "New Document" in file manager
- Dark theme pre-configured
- MS Core Fonts detection

### Microsoft Edge

**Configuration**:
- Set as default browser
- Office 365 PWA support
- Wayland native by default

### Browsers

| Browser | Display Server | Notes |
|---------|---------------|-------|
| Microsoft Edge | Wayland | Native Ozone platform |
| Firefox | Wayland | MOZ_ENABLE_WAYLAND=1 |
| Thunderbird | Wayland | Email client |

### Windows Applications (Wine/Bottles)

**Configuration**:
- XWayland for compatibility
- Bottles for management
- Certified app list maintained

---

## Wallpaper

### Default Wallpaper

**File**: `branding/wallpapers/pluris-default.svg`

**Design**:
- Deep blue gradient background
- Subtle cyan glow in corner
- Abstract geometric lines (suggesting network/structure)
- Grid overlay at low opacity
- Watermark logo in bottom-right

**Resolutions**: SVG (scalable to any resolution)

**4K Support**: Yes, SVG renders to any size

---

## Typography

### Primary Font: Inter

- **Weights**: Light (300), Regular (400), Medium (500), Bold (700)
- **Usage**: UI elements, menus, panels
- **Sizes**:
  - Small: 8-10pt (status, labels)
  - Regular: 10-12pt (menus, buttons)
  - Large: 14-18pt (headings)
  - Display: 24-64pt (clock, logos)

### Monospace: JetBrains Mono

- **Usage**: Terminal, code editors
- **Size**: 10pt default

### Installation

Fonts are installed via:
```bash
# In chroot
apt-get install fonts-inter fonts-jetbrains-mono
# Or manual install to /usr/share/fonts/
```

---

## Asset Locations in System

### System-wide

```
/usr/share/
├── pluris/                    # Pluris-specific assets
│   ├── icons/
│   ├── themes/
│   └── wallpapers/
├── icons/hicolor/scalable/
│   └── apps/pluris.svg        # Main logo
├── pixmaps/
│   └── pluris-logo.svg        # Logo for legacy apps
├── wallpapers/
│   └── pluris-default.svg     # Default wallpaper
├── sddm/themes/pluris/        # Login theme
│   ├── Main.qml
│   ├── theme.conf
│   ├── logo.svg
│   └── wallpaper.svg
├── plymouth/themes/pluris/    # Boot theme
│   ├── pluris.plymouth
│   ├── pluris.script
│   ├── logo.svg
│   └── spinner.svg
└── grub/themes/pluris/        # Bootloader theme
    ├── theme.txt
    ├── logo.svg
    └── background.svg
```

### User Configuration

```
~/.config/
├── kdeglobals              # Colors, fonts
├── kwinrc                  # Window manager
├── kcmfonts                # Font settings
├── plasma-*.rc             # Panel layout
└── kwinrulesrc             # Window rules
```

---

## Customization

### Changing Colors

Edit `branding/themes/pluris-colors.conf`:

```ini
[Colors]
Background=#0a1628        # Change this
Accent=#00d4ff            # Change this
```

Then regenerate or update `/etc/skel/.config/kdeglobals`.

### Changing Logo

1. Replace `branding/svg/pluris-logo.svg`
2. Keep same dimensions (512x512)
3. Maintain transparent background
4. Rebuild ISO

### Adding Wallpapers

1. Add SVG to `branding/wallpapers/`
2. Copy in `install_branding()` function
3. Update default in `plasma-org.kde.plasma.desktop-appletsrc`

### Custom SDDM Theme

Edit `branding/sddm/theme.conf`:

```ini
[General]
background=/path/to/custom-wallpaper.svg
accentColor=#custom-color
```

---

## Technical Notes

### SVG Usage

All branding assets use SVG for:
- **Scalability**: Renders perfectly at any resolution
- **File size**: Smaller than PNG at multiple resolutions
- **Future-proof**: Easy to modify

### Wayland Compatibility

All Qt/KDE themes are Wayland-native:
- SDDM uses `kwin_wayland` compositor
- No X11 dependencies for core UI
- XWayland available for legacy apps

### Installation During Build

Branding is applied via `install_branding()` in `install-desktop.sh`:

1. Copies SVG assets to system directories
2. Installs SDDM theme and sets as default
3. Installs Plymouth theme and sets as default
4. Installs GRUB theme configuration
5. Copies KDE configuration to `/etc/skel/`
6. Sets up Wayland defaults
7. Installs helper scripts

---

## Branding Checklist

### Build Verification

- [ ] Logo appears in application menu
- [ ] Login screen shows Pluris theme
- [ ] Boot animation displays correctly
- [ ] GRUB menu shows branding
- [ ] Desktop wallpaper applied
- [ ] Panel layout is Windows-style
- [ ] Colors match palette
- [ ] Fonts render correctly

### User Experience

- [ ] OnlyOffice opens .docx files by default
- [ ] Edge launches with Wayland
- [ ] Double-click opens files (not single-click)
- [ ] Window controls on right side
- [ ] Taskbar at bottom
- [ ] System tray shows network, volume, battery

---

## Resources

### SVG Tools
- Inkscape: Vector editing
- SVGOMG: Optimization
- Figma: Design (can export SVG)

### Color Resources
- Adobe Color: Palette generation
- Coolors.co: Palette exploration
- WebAIM: Contrast checking

### Testing
- QEMU: Test boot/login themes
- KDE Look: Theme inspiration
- GNOME Look: Cross-reference
