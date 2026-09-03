# Roadmap

**What:** Shipped milestones, current phase, and next planned work for Pluris Endpoint Management.

**Related:** [[pluris]], [[endpoint-management]], [[itsm]], [[os]]

Snapshot of shipped scope lives in [[handoff]]; this file is the timeline.

## Shipped milestones

- **Core console + asset management** — multi-tenant data model (tenants/sites/groups) on SQLite/WAL; Computers/Servers/Printers/Desks list + detail; canonical parameter registry; Windows-GP-mapped policy catalog; modern list UX (search, quick filters, filter builder, column picker).
- **2026-07-04 — Users/Identity + real authentication.** AD-familiar identity schema, Argon2id local login, sessions, first-run setup wizard, account lockout, CSRF protection, asset↔owner pairing.
- **2026-07-05/06 — Standardized detail pages.** `DetailShell` hero+tabs pattern rolled out to computers, users, groups; role vocabulary (Super Admin/Admin/Technician/User) and RBAC matrix; policy assignment resolution (direct/group/site/tenant); add-policy-from-catalog flow; policy catalog detail pages.
- **2026-07-06/08 — Dependency groups.** WMI-filter analog: typed platform/requirement conditions gating which policy modules apply to which assets, replacing free-text module targeting.
- **2026-07-08/09 — Pluris Policy (console authorization v1).** Zero-trust `domain.action` permission registry, grants engine, per-request enforcement, matrix UI, field-update API for inline editing, avatar upload.
- **2026-07-09/10 — RBAC v2.** Hierarchical role inheritance (parent chain, override-diff storage), roles assignable to groups, standardized role/dependency-group lists, full-page user create (`/users/new`).
- **2026-07-10/12 — Overhaul: parameter registry, condition builder, real Configuration Groups, dynamic groups.** `GET /api/params` as the one permission-filtered param contract; one condition-builder dialog and one eval engine shared by dependency-group conditions, dynamic group membership rules and module tests; Configuration Groups moved off a mock onto real DB-backed `DetailShell` pages; module ownership/grants; CodeMirror 6 vendored (no CDN at runtime). Migrations 006–009.
- **2026-07-16 — Soft delete + retention, list mass actions.** Every deletable entity soft-deletes by default and is restorable, with per-entity-kind retention settings and an in-process purger enforcing reference guards. Shared row-selection/mass-action framework across lists, Module Library wired first. Migration 010.
- **2026-07-17 — Modular module system.** Unified test model (INV-TEST: subject/operator/expected value across param, command and script kinds), rebuilt dependencies tab with structured version constraints, `.pmdl` export/import with a property-tested round trip. Migration 011.
- **2026-07-18 — Module scripts + enforcement redesign (partial).** Scripts became first-class named rows (name + language + origin) with a separate actions table; Scripts tab rebuilt as a standard registry-driven list; standalone pop-out code editor. Migration 012. Checkpoints 4–5 outstanding — see Current phase.
- **2026-09-03 — Deployment and data-integrity fixes.** `web/static` embedded into the binary (it was served from a process-cwd-relative path, so every stylesheet and script 404'd whenever the binary ran from anywhere but the repo root, silently disabling all list/detail JS); removed two unused CDN-loaded frameworks that violated the fixed stack; `cmd/seed` now creates demo identities; Users list resolves site and manager display names instead of raw foreign-key ids.

## Current phase

Endpoint Management console is pre-beta: build, `go vet`, `gofmt` and the full
test suite are green, but nothing yet runs on a real managed device. The
console's own scope (identity, assets, endpoint-policy catalog, configuration
and dependency groups, policy modules, console authorization) is built and
DB-backed.

**In flight:** the module scripts + enforcement redesign
(`docs/history/plans/2026-07-18-module-scripts-enforcement-redesign.md`) is
three of five checkpoints done — CP4 (Enforcement tab + shared suggest widget)
and CP5 (defaults/reset dialog, tag flipping, security test) are not started,
and no final whole-feature review has run.

**Designed, not started:** unified device queries
(`docs/history/plans/2026-08-27-unified-device-queries.md`) — absorb Dependency
Groups into Configuration Groups, adding a nested query tree and a refresh
scheduler. XL effort, high risk (migrates live data, rewrites the shared eval
engine).

## Known structural debt

- **Policy Catalog is a static curated catalog**, not DB-backed — ~170 entries
  with Windows GP mappings live in `catalog/policies/`. This is deliberate (it
  is bundled product data, not tenant data). Configuration Groups, policy
  modules, dependency groups, groups, assets, identities and Pluris Policy
  roles are all fully database-backed.
- **Template monoliths**: a few files (`web/templates/pages.templ`,
  `layout.templ`, `menu.go`) have grown large and are candidates for splitting
  as features touch them.
- SQLite + sqlc replaced an earlier planned PostgreSQL + Ent stack; accepted as
  the working direction (zero-config install goal) but still has no formal
  architecture decision recorded in
  `docs/endpoint-management/architecture/decisions.md`.
- **Unbuilt sidebar sections**: Dashboard, Profiles, Scripts, Wine and Package
  Management render placeholder empty states. The navigation is deliberately
  complete ahead of the features, so the IA is reviewable early.

## Next milestones

1. **Linux endpoint agent MVP** — enrollment, inventory reporting, check-in. The beta-defining feature; nothing enforces on a device until this exists.
2. **Policy enforcement** — wire the policy catalog / configuration groups / dependency groups to actually apply settings via the agent once it exists.
3. **ITSM foundation** — tickets/incidents schema and first self-service-portal surfaces, once Endpoint Management's agent work has freed up capacity (see [[itsm]]).

Pluris OS (see [[os]]) remains a later, unscoped consideration and is not on this roadmap's near-term path.
