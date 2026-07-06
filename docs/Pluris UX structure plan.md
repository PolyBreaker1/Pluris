 ---
# menu items (left bar):

Dashboard
--
 - adjustable, all values can be added from hrierarchal dropnown (shared hierarchy with menu.) scaleble tiles, dispay type chose (graph, text, update view etc.)
Notes: dont forget to implement dynamic search, for value chose and also for endpoint selection if required (propper hierarchal logic is absolutel crusial, take inspiration from qualys)

Users
--
- company user list imported from AD or manualy added
- users have assigned devices (add semi-automatic pairing mechanism)
- users have difeernd roles that reflect policies including config and view of loged in user (Admin, super admin, user- self sevice)

Computers
--
- list of all connected endpoints in table view with filterig fow all values. (dafault: Name, OS, site, last seen, group)
- Computer view: tom right side displaying image of compter with os image on screen. left site will dispay general info like last seen, os version, group and specs
  - under general info on tob will be tab switchable module with tabs:
    - Policy groups: adpoted translated from ad
    - installed software: list view with type fiels (deb, appimage, wine)
    - wine groups
    - assigned scripts - with triger field
    - logs

Policy
--

- policy group list - opening polocy tab will reveal options to show/modify configuration (example: asgined software, wine gropus etc.) - !EVERYTHING DEVIDED TO USER AND COMPUTER, MAINTAN COMPATIBILITY WITH GP!


Profiles
--
 - allows setting up config profiles where can assign software, setup wine gropus, assign scripts 
   
Scripts
--
- list with all user-connfigured scripts with triger field. (predefine at least 5 for example update after user hitting shut down) ! DONT FORGET ADD DIALOG OR SECOND PAGE!
- script add/edit must gave propper script editor, test with manual triger and live log agent (on defined testing workstation), triger (shutdown, app open, policy change, custom, etc)

Wine
-- 
- tabs for applications and configuration groups
- groups are used for shared confugurations (like common wine container), configuration group must contain file editor, registry editor, regex path setup and everything nececary.

Server Admination
--
- contains AD connetion setting, GP import options, server logs (regarding application) etc.

User/Admin preferences
--
- contains per user seting like light/dark mode, profile picture change (doubled in header expaned user section), admins workstations (simplify seting, testing workstations a re uses as machines near admin, not nececerly admin owned machines)

Important plan strategy: all values must be cleverly sorted in hiearchyal logic with AD and GP compatibility in mind from start. this will alow admins to create big congirurations setups, all of them depend on each other. Make sure no logic or structure error, no separate definition and edit/view pages accesible from every nececary point (example group policy options in policy tab and also computer tab - evarything must point to same gui)
We want admins "freedom to move" but ist absolutely nececary to not divide same configs.
We need to ensure all setting will share theie editor, if our editor has too many options for specific intended task, simplify by filtering existing up, never by creating new, simplified or extended. if one feature missing, whole dialog/editor must be updated

---

# Updates — 2026-05-05 session

> The sections below capture decisions taken in chat after the initial plan was written. They are part of the canonical spec. Where they conflict with text above, these updates win.

## U1. Assets replaces Computers as a top-level menu item

The "Computers" menu item is renamed **Assets** and becomes the top-of-hierarchy for all managed hardware. Below Assets:

- **Computers** (workstations, laptops)
- **Servers**
- **Printers**
- **Desks** — docks and monitors. Profiles will simplify guest connection (a guest plugging into a desk inherits the desk's profile to set up peripherals/displays without per-user configuration).

Everything that previously linked to "Computer" (policy targets, dashboard tile data sources, search filters, group membership) now links to **Asset**, with subtype filtering when a context only wants a specific kind. The Asset detail view keeps the existing Computer-detail tab structure (Policy Groups / Installed Software / Wine Groups / Assigned Scripts / Logs) and extends it per subtype where needed (e.g. Printer asset shows queues and consumables; Server shows services and uptime).

Pluris is also positioned as an **asset management platform** (CMDB-style: lifecycle, location, ownership, financial info, peripherals, contracts, software inventory). Research GLPI for inspiration without copying its schema; pull in the parts that match our hierarchy and AD/GP-first model.

## U2. Windows policy catalog seed

We seed the policy catalog from Microsoft's official **Group Policy Settings Reference Spreadsheet** (each Windows release publishes one). The descriptions in that spreadsheet are the authoritative human-readable text shown in the Pluris policy editor.

## U3. Policy translation = Policy Module system

**Policy translation never hardcodes Linux equivalents.** Translation is implemented as a **module system**:

- A **Policy Module** is a versioned package containing: a manifest, an enforcement script, optional validate / rollback scripts, a parameters JSON schema, dependency declarations, and a `satisfies` list mapping the module to the Windows policy URNs (or Pluris-native setting URNs) it implements.
- One module can satisfy multiple policies; one policy can be satisfied by multiple modules (admin chooses which).
- Modules know their **dependencies**. A module can require other modules to be present and ordered before it. The agent resolves the dependency graph and runs in topological order.
- Modules are **user-editable in the web UI** (script editor side-by-side with the Windows policy that triggers it). Edits version a new revision. Production deploy is explicit, not implicit.
- Modules are **structured for community contribution**: simple file layout, idempotent scripts, validation tests. The bar to perfecting Linux policy coverage is community contributions.
- **Compatibility filter is mandatory in the UI**: when an admin binds a Windows policy, the editor only shows modules whose `satisfies` includes that policy and whose target OS matches the device's OS. No confusion from incompatible modules.

The Policy Module canonical editor is reachable from:
- **Scripts** menu, sub-tab **Policy Modules** (the regular Scripts tab continues to host user automation scripts).
- **Server Administration → Policy Enforcement Scripts** — admin view of all installed modules; this is the place to install/upgrade/disable modules at server level.
- Inside the **Policy Group editor** when binding a Windows policy: a side-by-side view (Windows policy on the left, list of compatible modules on the right, in-place script editor for the selected module).

All three entry points mount the same `editors/PolicyModuleEditor` (per the canonical-editor rule above), with different filters and embed flags.

> Architectural consequence: this supersedes the earlier idea of hardcoded native Go CSEs in `pluris-agent`. The CSE *concept* survives — they become the initial bundled modules in the same module format. See ADR-006.

## U4. Package Management as a new top-level menu item

A new top-level sidebar item **Package Management** with three tabs:

- **Package Managers** — definitions of supported package managers per OS (apt, dnf, pacman, flatpak, snap, winget, brew, …) and per-asset overrides.
- **Packages** — the catalog of packages we know about (cross-manager), installation status across the estate, version distribution, vulnerability state.
- **Update Cycles** — windows / schedules / staged rollouts. Connected to assets (which cycle they're on), policies (a Policy Group can mandate a cycle), and scripts (cycles can run pre-/post-update scripts).

Like everything else: connected to Assets (installed-software view), Policies (policy can require packages), Profiles (profiles bundle packages), Scripts (cycles can run scripts), and the Policy Module system (a module can declare a package dependency).

## U5. Cross-cutting reminder

**Everything must be connected to everything.** Asset → Policy → Profile → Script → Policy Module → Package — all reachable from each other via the canonical editors and shared hierarchical pickers. No isolated islands.

## U6. Doc discipline

Strip unimportant details from docs (long comparison tables, superseded plan iterations, scratch notes). Keep canonical spec compact. Don't strip from `Pluris UX structure plan.md` (this file), `UX_INVARIANTS.md`, `ARCHITECTURE_DECISIONS.md`, `BRANDING_GUIDE.md`, or any Linux-distro doc. Strip targets: `MANAGEMENT_PLATFORM_RESEARCH.md`, `FORK_STRATEGY.md`.