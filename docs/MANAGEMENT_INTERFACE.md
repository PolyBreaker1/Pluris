# Pluris Management Interface

> **Note**: This document was previously a survey of third-party Linux management tools (Foreman, Rudder, Ansible). That content is **superseded** — Pluris is now its own management platform. The old content is preserved at the bottom of this file under "Historical: third-party tooling survey" for reference, but it does not describe the product.
>
> **Canonical sources for this topic:**
> - `docs/Pluris UX structure plan.md` — UX/IA spec (user-authored, canonical).
> - `docs/UX_INVARIANTS.md` — formal IA contract derived from the plan.
> - `docs/ARCHITECTURE_DECISIONS.md` — ADR-001/002/003/004.
>
> If anything below contradicts those, the above wins. Fix this file.

---

## Overview

The **Pluris Management Console** is the web UI for administering Pluris-managed estates. It is the product itself — not an integration with Foreman/Rudder/Landscape. It runs as `pluris-console` (Go + Echo + Templ + HTMX + FrankenUI), backed by PostgreSQL and NATS, with `pluris-agent` on every endpoint and Kanidm as the identity provider.

This document is the **operator-facing summary** of the console's information architecture. The **normative** version lives in `docs/UX_INVARIANTS.md`.

---

## Top-level navigation (left sidebar)

| # | Menu item | Purpose |
|---|---|---|
| 1 | Dashboard | Adjustable scalable tiles. Tile data sources picked from the shared hierarchy. Display types: graph, text, update view, table. |
| 2 | Users | Identity list (AD-imported or manually added). Role assignment. Semi-automatic asset pairing. |
| 3 | **Assets** | Unified hardware list with subtype tabs: **Computers / Servers / Printers / Desks**. All four mount the same `editors/AssetEditor` with `subtype=` filter. Detail view: tabbed sub-modules (**Policy Groups, Installed Software, Wine Groups, Assigned Scripts, Logs**) plus subtype-specific tabs (e.g. Printer queues, Server services). Asset-management fields (lifecycle, location, vendor, warranty, contracts) inspired by GLPI. |
| 4 | Policy | Policy Groups list and editor. **Every section partitioned into Computer Configuration and User Configuration**, mirroring Windows GP. When a Windows policy is bound, side-by-side view shows compatible Policy Modules. |
| 5 | Profiles | Profile list and editor. Profiles bundle assigned software, Wine groups, scripts, and packages. |
| 6 | Scripts | Page with two tabs: **Scripts** (user automation, with trigger field, manual test against testing workstation, live log) and **Policy Modules** (translation packages — see ADR-006). |
| 7 | Wine | Tabs: Applications, Configuration Groups. Configuration Group editor includes file editor, registry editor, regex path setup. |
| 8 | **Package Management** | Three tabs: **Package Managers** (apt/dnf/pacman/flatpak/snap/winget/brew), **Packages** (cross-manager catalog, install status, vulnerabilities), **Update Cycles** (windows/schedules/staged rollouts). |
| 9 | Server Administration | AD connection settings, GP import wizard, **Policy Enforcement Scripts** (admin view of `editors/PolicyModuleEditor`), server (application) logs. |
| 10 | User/Admin Preferences | Per-user (light/dark, profile picture). Admin-specific (admin testing workstations — references existing Assets). |

Adding a new top-level menu item requires explicit user approval and a Concept Registry entry in `docs/UX_INVARIANTS.md` §VII.

---

## Architectural rules the UI must obey

These are the rules from `docs/UX_INVARIANTS.md` summarized for human reading. Refer to that document for the normative version.

### 1. One canonical editor per concept

Every concept (Policy Group, Profile, Script, Wine Configuration Group, Computer, User, Dashboard Tile, …) has **exactly one** editor component. Every navigation path that lands on that concept mounts the **same component**, with at most a filter / scope prop applied.

> Example: a Policy Group opened from the Policy menu, from a Computer's "Policy Groups" tab, or from inside a Profile editor all open `editors/PolicyGroupEditor`. None of these is a "lite version" or a "view-only twin" — they are the same component.

### 2. Filter, never fork

If a context needs **less** than the canonical editor exposes, pass a filter or scope prop (`hideTabs`, `readOnly`, `scope=machine`). If a context needs **more**, extend the canonical editor. Never create a sibling component.

### 3. Shared hierarchy everywhere

There is exactly one hierarchy in Pluris: **Tenant → Site → Group → (Asset | Identity)**. Asset has subtypes (`computer`, `server`, `printer`, `desk`, …). Every navigator, picker, search input, dashboard tile data source, and policy assignment target reads from this same model. AD/GP compatibility is built in at this layer (Site↔Site, Group↔Security Group/OU, Asset(subtype=computer)↔Computer, Identity↔User, Policy Group↔GPO).

### 4. Computer vs User scope is mandatory

Every Policy Group, Profile, Script, **Policy Module**, and PolicySetting carries `scope: machine | user | both`. Editors visually partition the two sections, mirroring Windows Group Policy ("Computer Configuration" / "User Configuration"). Backend resolution treats them as independent inheritance chains that merge at evaluation time on the agent.

### 5. Hierarchical typeahead search

Wherever an entity is selected, the picker is the same hierarchical search component (Qualys-inspired typeahead with breadcrumbs and lazy expansion). Picker scope can be filtered (`only=device`, `under=site:berlin`) but the component is unchanged.

### 6. Self-service uses admin editors with `readOnly`

End-user self-service views render the same editor components as admin views, with `readOnly=true` and a scope-of-self filter. There is no separate "user portal" component branch.

---

## Concept-to-screen routing

This is a summary; see `docs/UX_INVARIANTS.md` §VII for the normative table.

| Concept | Reachable from | Filter passed |
|---|---|---|
| Asset (any subtype) | Assets menu (all tabs), search, dashboard tiles, policy assignment target picker, Discovery results | `subtype=` filter per tab |
| Policy Group | Policy menu, Asset detail → Policy Groups tab, User detail → Policies, Profile editor, Server Admin → GP Import results | `targetAsset` / `targetUser` / `embedded` as appropriate |
| Profile | Profiles menu, Asset detail, User detail, Desk asset (Guest Profile reference) | scope filter |
| Script | Scripts menu → Scripts tab, Asset detail → Assigned Scripts tab, Profile editor, Update Cycle (pre/post hooks) | trigger filter |
| Policy Module | Scripts menu → Policy Modules tab, Server Admin → Policy Enforcement Scripts, Policy Group editor side-by-side view | compatibility filter (`satisfies` ∩ policy URN; `target_os` ∩ device OS) |
| Wine Configuration Group | Wine → Configuration tab, Asset detail → Wine Groups tab, Profile editor | — |
| Wine Application | Wine → Applications tab, Asset detail → Installed Software (Wine type) | — |
| User (Identity) | Users menu, Asset detail → Assigned Users, policy assignment target picker | — |
| Package | Package Management → Packages tab, Asset detail → Installed Software, Profile editor, Policy Group editor, Policy Module manifest | — |
| Package Manager | Package Management → Package Managers tab, Asset detail (overrides) | — |
| Update Cycle | Package Management → Update Cycles tab, Asset detail, Policy Group | — |

Adding a routing path means adding a row to the `UX_INVARIANTS.md` Concept Registry **and** extending the mount-point test for that editor.

---

## Process: how to add a feature without violating invariants

1. **Find the concept.** Is the feature about an existing concept (Policy Group, Profile, …) or a new one?
2. **Existing concept** → modify the canonical editor referenced in `UX_INVARIANTS.md` §VII. If the change is contextual, add a filter/scope prop. **Never branch into a sibling component.**
3. **New concept** → write the IA contract row in `UX_INVARIANTS.md` §VII first (entity, scope, canonical editor path, every entry point + filter, AD/GP equivalent, role gates). Confirm with the user. *Then* scaffold the schema and editor.
4. **New entry point for an existing concept** → add the row to §VII, extend the editor's mount-point test, then implement the navigation.

---

## Operator-facing topics covered elsewhere

- Visual styling, palette, typography → `docs/BRANDING_GUIDE.md`.
- Endpoint enforcement (CSEs, PAM hook, drift refresh) → `docs/ARCHITECTURE_DECISIONS.md` ADR-003 and `docs/GROUP_POLICY_COMPATIBILITY.md`.
- Backend architecture (Go modules, Ent, NATS, mTLS) → `docs/ARCHITECTURE_DECISIONS.md` ADR-001/002, `pluris/README.md`.
- AD / Group Policy compatibility translation → `docs/GROUP_POLICY_COMPATIBILITY.md`.

---

## Historical: third-party tooling survey (superseded)

The earlier version of this document surveyed Foreman + Katello, Rudder, Canonical Landscape, Ansible+SSH, etc. as candidate management tools. These are no longer the recommendation: Pluris is its own platform. The historical content is removed for clarity; see git history if you need to reference it. Decisions that came out of that survey are captured in `docs/ARCHITECTURE_DECISIONS.md`.
