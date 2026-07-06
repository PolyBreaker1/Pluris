---
trigger: always_on
description: Pluris core operating rules. Always apply. Override conflicting habits.
---

# Pluris Core Rules (read every turn)

## Canonical sources of truth

These two files outrank every other doc. If any other doc disagrees, fix the other doc.

1. **`docs/Pluris UX structure plan.md`** — user-authored UX/IA plan. Treat as immutable spec source unless the user edits it. Never paraphrase in a way that loses fidelity.
2. **`docs/UX_INVARIANTS.md`** — formal extraction of invariants from (1). Append-only; revise only on user direction.

ADRs in `docs/ARCHITECTURE_DECISIONS.md` are append-only and authoritative for backend/architecture decisions.

---

## Hard rules — violations are bugs

### R1. Single source of truth UI
- **One canonical editor per concept.** Concepts include: Policy Group, Profile, Script, Wine Configuration Group, User, Computer, Dashboard Tile.
- Multiple navigation paths to the same concept must mount the **same component instance** with at most a context filter / scope prop applied. Never a "view-mode" or "simplified" variant in a separate file.
- If a context needs **less**: pass a filter (`scope`, `readOnly`, `hideTabs`, …). Same component, same code path.
- If a context needs **more**: extend the canonical component. Never create a sibling component, even temporarily.
- Editor missing a feature? **Add it to the canonical editor.** Never branch.

### R2. No redundant services / no parallel implementations
- Before creating a new package, handler, schema entity, Templ component, or DB migration: search for an existing one and extend it.
- Misnamed/misplaced existing code → rename or relocate, never duplicate.
- "Quick" sibling files (`*_simple.go`, `*_v2.templ`, `*_lite.tsx`) are forbidden.

### R3. Hierarchy parity
- Every entity, navigation tree, dashboard tile picker, search filter, policy assignment selector, and audit log filter reads from one shared hierarchy: **Tenant → Site → Group → (Device | Identity)**.
- Don't introduce alternative groupings (e.g. "team", "department") unless the user requests it; map them onto the existing hierarchy.
- AD/GP compatibility is a first-class constraint. New entities must declare how they map to AD concepts (OU, Security Group, User, Computer, GPO).

### R4. User vs Computer scope is mandatory metadata
- Every Policy, Profile, Script, and Setting carries `scope: machine | user | both`.
- Editors visually partition the two like Windows GP ("Computer Configuration" / "User Configuration").
- Backend resolution treats them as independent inheritance chains that merge at evaluation time.

### R5. Slow down before code
- For any non-trivial new module (a new top-level menu item, a new entity, a new editor): write the IA contract **first** and confirm with the user before scaffolding code.
  - IA contract = entity name, fields, edges, screens it appears in, scope (machine/user/both), permission model, where its canonical editor lives, which navigation entry points filter into it.
- Add the entry to `docs/UX_INVARIANTS.md` § "Concept Registry" before writing handlers/templates.

### R6. Ask, don't assume
- When IA, scope, or hierarchy placement is ambiguous, ask the user before scaffolding.
- Default of "make it a separate page" is wrong — usually it's a tab, filter, or shared editor.

### R7. Documentation discipline
- Don't create new `.md` files unless the user asks or a rule requires it.
- Don't add or remove code comments unless asked.
- Existing canonical docs (`Pluris UX structure plan.md`, `UX_INVARIANTS.md`, `ARCHITECTURE_DECISIONS.md`) are the source of truth — update other docs to align with these, not the other way around.
- Keep `PROGRESS.md` as the project status log. Update it when phases change.

### R8. Bug-fix discipline
- Prefer minimal upstream fixes over downstream workarounds.
- For UI bugs that look like "this dialog is missing a feature" — first check if the canonical editor exists elsewhere; the bug is usually that the entry point isn't routing to it.

### R9. Branding consistency
- The management console UI uses the Pluris palette and typography defined in `docs/BRANDING_GUIDE.md` (deep blue `#0a1628`, cyan accent `#00d4ff`, Inter / JetBrains Mono). Don't introduce a parallel palette.

---

## Workflow rules

### W1. Plan visibility
- Maintain `todo_list` for any task touching more than one file.
- One step `in_progress` at a time. Mark complete promptly.

### W2. Confirm direction at decision points
- Before creating new top-level directories, new entities, new menu items, new dependencies: state the intent and proceed if obvious; ask if user-facing or architecturally novel.

### W3. Code style
- Go: idiomatic, no over-engineering. Prefer single Go module for the server side.
- Templates: Templ for HTML, HTMX for interactivity, FrankenUI/Tailwind for styling. No React, no SPA.
- Database: Ent schemas in `pluris/db/schema/`. Migrations generated, not hand-rolled.

### W4. Verification before "done"
- After non-trivial changes: `go vet ./...`, `go test ./...`, `templ generate --check` if templates changed.
- Provide copy-pastable commands when tools are unavailable in env.

---

## Concept-to-screen routing (must always hold)

When the same concept is reachable from multiple menu items, it routes to **the same editor** with these filters:

| Concept | Canonical editor | Reachable from | Filter applied |
|---|---|---|---|
| Policy Group | `editors/PolicyGroupEditor` | Policy menu, Computer detail → Policy Groups tab, User detail → Policies, Profile editor | scope filter; readOnly when launched from a non-owning context |
| Profile | `editors/ProfileEditor` | Profiles menu, Computer detail (assigned profiles), User detail | scope filter |
| Script | `editors/ScriptEditor` | Scripts menu, Computer detail → Assigned Scripts tab, Profile editor | trigger filter |
| Wine Config Group | `editors/WineConfigGroupEditor` | Wine menu → Configuration tab, Computer detail → Wine Groups tab, Profile editor | — |
| Computer | `editors/ComputerEditor` | Computers menu, search results, dashboard tiles, policy assignment targets | — |
| User (Identity) | `editors/UserEditor` | Users menu, Computer detail → assigned users, policy assignment targets | — |

This table is **enforced by tests** (each entry point asserts it mounts the canonical editor component). Update this table when adding concepts.
