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

## Current phase

Endpoint Management console is feature-complete for its current scope (identity, assets, endpoint-policy catalog, console authorization) and pre-beta: build and test suite green, but nothing yet runs on a real managed device. Owner GUI walkthrough and repo-publish prep are the immediate housekeeping items; no new feature work is active as of this doc's writing.

## Known structural debt

- **Mock-vs-DB split**: Policy Catalog / Configuration Groups / Module pages still render from hardcoded mock data in `catalog/`, not the database — the schema exists but the UI doesn't touch it for those entities yet. Assets, identities, dependency groups, and Pluris Policy roles are fully database-backed.
- **Template monoliths**: a few generated-adjacent files (`web/templates/pages.templ`, `layout.templ`, `menu.go`) have grown large and are candidates for splitting as features touch them.
- SQLite + sqlc replaced an earlier planned PostgreSQL + Ent stack; this is accepted as the working direction (zero-config install goal) but has no formal architecture decision recorded yet.

## Next milestones

1. **Linux endpoint agent MVP** — enrollment, inventory reporting, check-in. The beta-defining feature; nothing enforces on a device until this exists.
2. **Policy enforcement** — wire the policy catalog / configuration groups / dependency groups to actually apply settings via the agent once it exists.
3. **ITSM foundation** — tickets/incidents schema and first self-service-portal surfaces, once Endpoint Management's agent work has freed up capacity (see [[itsm]]).

Pluris OS (see [[os]]) remains a later, unscoped consideration and is not on this roadmap's near-term path.
