# Platform-wide soft delete + retention — design

**Date:** 2026-07-16
**Status:** approved (owner), ready for implementation
**Related:** [[2026-07-16-list-mass-actions-design]], `catalog/permissions/`, `docs/development/handoff.md`

## Problem

Every delete in Pluris is immediate and permanent. The owner wants a recycle-bin model:
deletions mark items deleted; a configurable per-entity-type retention window purges them
later; deleted items are hidden from all UI unless explicitly filtered in. This also
resolves the review finding that the module "Disable" mass action performs an
irreversible revoke with no confirmation.

## Decisions (owner-approved)

- **Soft delete is the default for every deletable entity**, wired into ALL of them in
  this task: identities (users), assets, groups, configuration groups, dependency
  groups, policy modules.
- **Retention settings** live on a new **Data Management** page — the first child of
  Server Administration in the sidebar (`/server-admin/data`) — gated by a **new
  permission** registered in `catalog/permissions/` (follow existing domain/action
  naming conventions, e.g. a `server_admin` domain with a `manage_data` action;
  grantable via the `/policy/pluris` matrix like every other permission).
- Per entity kind: `purge_after_days` (NULL = never auto-purge — the default) and
  `mode` (`soft` default | `immediate` = keep today's hard-delete behavior).
- Deleted items are **hidden everywhere** (lists, pickers, candidate sets, dynamic
  group eval, module resolution) unless the list's standard filter explicitly selects
  "Deleted".
- The module Disable mass action is renamed **"Revoke versions"**, danger-styled,
  confirmation-required, with dialog text stating it cannot be undone (publish a new
  version to re-enable).

## Design

### 1. Schema — one migration (next number in db/schema/)

- Add `deleted_at TIMESTAMP NULL` and `deleted_by INTEGER NULL` (identity id, no FK
  cascade — informational) to: `identities`, `assets`, `groups`,
  `configuration_groups`, `dependency_groups`, `policy_modules`.
- New table `retention_settings`:
  `entity_kind TEXT PRIMARY KEY` (one row per kind above),
  `purge_after_days INTEGER NULL` (NULL = never auto-purge),
  `mode TEXT NOT NULL DEFAULT 'soft'` (`soft` | `immediate`),
  `updated_at`, `updated_by`.
  Seed one row per entity kind with defaults (NULL days, soft) in the migration.
- Never edit applied migrations (AGENTS.md rule 6); read `pkg/database/database.go`
  before writing the migration.

### 2. Service semantics

- Every existing `Delete<Entity>` service method becomes: look up the kind's
  `retention_settings.mode`; `immediate` → existing hard-delete path unchanged;
  `soft` (default) → set `deleted_at`/`deleted_by`.
- New per-entity `Restore<Entity>` (clears `deleted_at`/`deleted_by`) and shared purge
  logic `PurgeExpired(ctx)` that hard-deletes rows whose `deleted_at` is older than
  their kind's window.
- **Reference integrity moves to the hard boundary**: soft delete never blocks and
  never breaks references (fully restorable). Existing guards (e.g. module delete
  requires refcount 0) apply at hard delete/purge; purge skips still-referenced or
  guard-blocked items, logs the skip, and retries next cycle.
- Soft-deleted items must not participate in behavior: filter them out of list
  queries, pickers, candidate sets (`CandidatesForPolicy`), dynamic-membership eval,
  and module resolution. Default queries gain `WHERE deleted_at IS NULL`; add
  `include_deleted`-style variants only where the deleted view needs them.

### 3. Purge scheduler

- In-process ticker in the console startup path: run `PurgeExpired` once at boot,
  then hourly. Transactional per item; failures logged, never fatal. Record purges in
  the activity log (same mechanism the bulk module handler uses).

### 4. UI

- **List filter**: the standard filter toolbar gains a state selector where relevant
  (Active — default — / Deleted). Selecting Deleted shows only soft-deleted rows.
- **Mass actions on deleted rows**: Restore (no confirmation) and Delete permanently
  (danger, confirmation-required, "cannot be undone"). Live-row Delete keeps its
  confirmation but the dialog copy says items move to Deleted and how long they're
  kept (or "until deleted permanently" when the window is NULL); when the kind's mode
  is `immediate`, keep today's destructive warning copy.
- **Data Management page** (`/server-admin/data`): Server Administration gets its
  first sidebar child ("Data Management"). Page lists each entity kind with its
  retention window (days input or "never") and mode (soft/immediate), saved via the
  standard form/CSRF patterns. Route + page gated by the new permission. Follow
  DetailShell/list conventions as applicable; keep it a simple settings page.
- **Disable → Revoke versions** (modules): rename the mass action key/label, mark it
  Danger, require confirmation (extend the `needsConfirm` rule in `lists.js`), dialog
  text: revokes all published/superseded versions; cannot be undone; publish a new
  version to re-enable.

### 5. Out of scope

- No per-item retention overrides (per-kind only).
- No "empty recycle bin" bulk purge UI (purge is scheduler-driven; per-item Delete
  permanently exists).
- No changes to the auth route-table caveat from the handoff.

## Testing

- Migration applies on a fresh t.TempDir() DB; seeded retention rows present.
- Service tests per entity: soft delete hides from default list queries; restore
  round-trips; immediate mode hard-deletes; purge respects windows, skips referenced
  items (module refcount case), NULL window never purges.
- Handler tests: Data Management page permission-gated (403 without grant); settings
  save validates days ≥ 0/NULL; deleted filter returns deleted rows; restore and
  permanent-delete mass actions; revoke-versions requires nothing new server-side
  beyond the rename (bulk endpoint action key stays `disable` internally or is
  renamed consistently — pick one and be consistent across JS/Go/tests).
- Template tests: sidebar shows Data Management only with the grant; deleted-state
  filter renders; dialog copy contracts for the three delete flavors (soft, permanent,
  immediate-mode) and revoke-versions.
- Definition of done per AGENTS.md: build + full tests + gofmt clean; `make gen`
  after every .templ edit; sqlc generate after query/schema changes (ASCII-only SQL
  comments).

## Execution checkpoints (verification gate between each)

1. Migration + retention_settings + sqlc queries + service core (soft delete/restore/
   purge for ONE entity — policy modules — as the pattern proof).
2. Purge scheduler + Data Management page + new permission, end to end.
3. Remaining entities one checkpoint each: assets, identities, groups, configuration
   groups, dependency groups (queries, service, list filter, mass actions Restore /
   Delete permanently).
4. Disable → Revoke versions rename + confirmation + dialog copy.
