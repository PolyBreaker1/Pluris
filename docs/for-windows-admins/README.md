# Pluris for Windows admins

> If you've used Active Directory + Group Policy + Intune, you already know 80% of Pluris. The rest is one tab per Linux concept and a hover for every term you don't recognize.

## Read in this order

| # | File | What it gives you | Time |
|---|---|---|---|
| 1 | [cheatsheet.md](cheatsheet.md) | "I want to do X in GP — where in Pluris?" One-row-per-task table. | 5 min |
| 2 | [concepts.md](concepts.md) | What every screen IS and the AD analogue. | 10 min |
| 3 | [glossary.md](glossary.md) | Every Linux/Pluris term you'll meet in the UI. Hover gives the same gloss in-product. | reference |

## How to navigate Pluris (sidebar map)

```
Dashboard            ← like a Configuration Manager dashboard
Users                ← AD Users (the user side)
Assets               ← AD Computer accounts + CMDB inventory
  ├ Computers
  ├ Servers
  ├ Printers
  └ Desks
Policy               ← Group Policy
  ├ Catalog          ← GPEdit setting browser (read-only)
  ├ Configuration Groups   ← GPO + GP filtering
  └ Modules          ← signed bundles that implement Catalog settings (Pluris-specific)
Profiles             ← Intune Configuration Profile
Scripts              ← GP Logon/Startup + Intune scripts (admin-authored, ad-hoc)
Wine                 ← run Windows apps on Linux (no AD analogue)
Package Management   ← WSUS + Intune apps + ConfigMgr
  ├ Package Managers ← apt / dnf / snap / flatpak
  ├ Packages         ← installed software inventory
  └ Update Cycles    ← rollout rings + maintenance windows
Server Administration   ← AD Sites & Services + Intune tenant
Preferences          ← MMC console preferences
```

## Mental model

| Windows | Pluris equivalent | Notes |
|---|---|---|
| Domain Controller | Pluris **server** + Kanidm | One Linux box; Kanidm is the LDAP-ish identity service. |
| Domain | Tenant | Top of every hierarchy. |
| Site | Site | Same word, same meaning. |
| OU + Security Filter | Configuration Group | One screen instead of three. |
| GPO | Configuration Group | A "GPO" in Pluris IS a Configuration Group. |
| GP Setting | Policy (in Catalog) + Policy Module | Catalog = the setting; Module = the thing that applies it on Linux. |
| Group Policy Client | `pluris-agent` | Daemon on every managed host. |
| WSUS approval ring | Update Cycle | Pilot / Broad / Critical, with windows. |
| Intune Configuration Profile | Profile | Bundle of Configuration Groups, scripts, packages. |
| Intune Endpoint Manager | Pluris admin console | This UI. |

## Two safe muscle-memory rules

1. **Single source of truth UI** — every concept has *one* canonical editor. If you're on Computer detail and click "Edit policy", you get the same screen you'd reach from Policy → Configuration Groups. No "simplified" duplicates anywhere. (Internally locked as `INV-U2` / `R1`.)
2. **Computer vs User scope is mandatory** — every Policy / Profile / Script / Setting carries `scope: machine | user | both`, and editors visually split the two like GPEdit does. If you don't see your setting, you may be on the wrong scope tab.

## When you get stuck

| Symptom | Where to look |
|---|---|
| "Where do I add this setting?" | Search the **Policy Catalog** (top filter). Searches Windows GP names, Linux mechanisms, descriptions. |
| "Why isn't it applying?" | Open the asset → **Policy** tab → resolution view. Shows the merged plan + which Configuration Group set each value. |
| "What is `pam_pwquality`?" | Hover any underlined term in the UI. Same content as [glossary.md](glossary.md). |
| "Is there a GP equivalent?" | Every concept page shows it in the orientation banner ("In Active Directory: …"). |
