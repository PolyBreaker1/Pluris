# List mass actions + Module Library standardization — design

**Date:** 2026-07-16
**Status:** approved (owner), ready for implementation
**Related:** [[2026-07-12-module-grants-and-ownership]], INV-L (`web/lists/lists.go`), `docs/development/handoff.md`

## Problem

1. The Module Library table (`policyModulesList` in `web/templates/pages.templ`) deviates
   from the platform list standard: it carries a `Management` column of per-row buttons
   (Pick / Clone / Disable / Delete with a native `confirm()`), plus a redundant
   "New module" button in its filter toolbar (the `PageHeader` action already links to
   `/policy/modules/new`). Every other list is clean `data-row-href` rows with no row
   buttons (see handoff "Global detail/list interaction standard").
2. The platform has no multi-select / bulk-operation capability on any list. Admins must
   act on items one at a time.

## Decision summary (owner-approved)

- Build selection + mass actions as a **global framework** (extend `web/static/lists.js`
  + shared templ components + a Go-side action registry in `web/lists/`), wire it into
  the **Module Library only** in this task. Other lists adopt it in follow-ups.
- **All rows selectable; actions filter.** Each toolbar action shows
  `Label (eligible of selected)` and acts only on eligible rows. Ineligible rows are
  listed with reasons in the confirmation dialog, never silently skipped.
- **Per-row buttons are removed entirely.** Clone → mass action "Duplicate";
  Delete → mass action "Delete"; Disable → module-specific mass action.
  **Pick is dropped from the list** — module detail / binding editors are its home
  (verify it exists on the module detail page; add there if missing).
- **Confirmation dialog** (styled `<dialog>`, matching `ConditionBuilderDialog` /
  `policyModulePickerDialog` patterns — never native `confirm()`) is REQUIRED for:
  any **delete** (any count), and **duplicate of more than one item**. Not required for
  single duplicate or disable.

## Design

### 1. Selection framework (frontend)

**Opt-in contract** (all `data-pluris-*`, consistent with existing filter/sort/row-nav
contracts in `lists.js`):

- Table opts in with `data-pluris-select="<listID>"` on the `<table>`.
- Shared templ components (new file `web/templates/list_mass_actions.templ`):
  - `ListSelectHeaderCell(listID)` — leftmost `<th>` with a select-all-visible checkbox.
  - `ListSelectCell(listID, itemID, caps)` — leftmost `<td>` checkbox;
    `caps` renders as `data-select-caps="duplicate,delete,disable"` (comma-joined keys
    of actions this row is eligible for).
  - `MassActionToolbar(listID, actions)` — hidden until selection ≥ 1; shows selected
    count and one button per registered action.
  - `MassActionConfirmDialog()` — one shared dialog mounted once per page.
- `lists.js` additions (no per-list JS, ~selection module):
  - Selection state per `data-list-id`, kept in memory (not persisted).
  - Header checkbox selects/deselects **visible** rows only (respects active filters);
    indeterminate state when partially selected.
  - Rows hidden by a filter STAY selected; the toolbar count shows
    `N selected` plus `(M hidden by filter)` when M > 0.
  - Checkbox cells are excluded from `data-row-href` row navigation (same nested-control
    exclusion mechanism already in `lists.js`).
  - Per-action eligibility = row's `data-select-caps` contains the action key.
  - Confirm flow: for delete (always) and duplicate (count > 1), open the dialog listing
    affected item names (first 10 + "and N more"), the ineligible items with reasons,
    and a danger-styled confirm for deletes. On confirm, POST the bulk endpoint with
    eligible ids; render per-item results (ok / failed+reason) back into the dialog;
    reload the list on close if anything succeeded.

**Action declaration (Go side, `web/lists/`):**

```go
// MassAction — one bulk operation a list offers.
type MassAction struct {
    Key    string // "duplicate" | "delete" | list-specific e.g. "disable"
    Label  string // "Duplicate"
    Icon   string // iconSVG name
    Danger bool   // renders red, dialog uses danger confirm
    URL    string // bulk endpoint, e.g. "/api/modules/bulk"
}
```

Registered per list next to its FieldDefs. The Module Library registers:
`duplicate` (POST clone), `disable` (Danger=false), `delete` (Danger=true), all → `/api/modules/bulk`.

### 2. Bulk endpoint (backend, modules first)

`POST /api/modules/bulk` (new handler in `console/handlers/`, route in
`console/server/server.go` beside the other `/api/modules` routes, same
`endpoint_policy.manage_modules` route gate):

Request: `{"action": "clone"|"disable"|"delete", "ids": ["..."], "_csrf": ...}` (form or
JSON — follow whichever the existing module action endpoints use).

Semantics: **best-effort per item**, reusing the existing single-item service methods in
`pkg/services/policymodules.go` (clone, disable, refcount-guarded delete). Per-item authz
via `pkg/authz.ModuleCanEdit/Admin` exactly as the single-item handlers do — bundled
modules: clonable, never disable/delete (mirrors current per-row visibility logic).

Response: `{"ok": ["id"...], "failed": [{"id": "...", "reason": "human-readable"}...]}`.
Best-effort is deliberate: module delete legitimately fails per item on refcount > 0.
No transaction across items; each item's operation keeps whatever transactionality the
service method already has.

### 3. Module Library table standardization

In `policyModulesList` (`web/templates/pages.templ`):

- Remove the `Management` `<th>`/`<td>` and all four per-row buttons + their forms.
- Remove the toolbar's "New module" field-action block (PageHeader action remains).
- Add `ListSelectHeaderCell` / `ListSelectCell` as the new first column. Per-row caps:
  bundled → `duplicate`; tenant/imported → `duplicate,disable,delete`.
- Mount `MassActionToolbar` between the filter toolbar and the table, and the shared
  confirm dialog once on the page.
- Keep: rich cells (chips, phase strip, deps details), `data-row-href` navigation,
  existing filter toolbar, `ConceptEmptyState`.
- Check the module detail page offers the Pick / picker entry point; if not, add it there
  (small, in-scope since the list loses it).

### 4. Scope guard

- Only the Module Library is wired up in this task. No checkbox column on any other list.
- No changes to `pkg/auth` route table (the known route-gate granularity caveat in the
  handoff stays as-is).
- CSRF: bulk POST carries the same `_csrf` token the existing module forms use.

## Testing

- **Handler tests** (`console/handlers/`): bulk clone/disable/delete happy path; mixed
  eligibility (bundled in a delete request → per-item failed with reason, eligible ones
  succeed); refcount-blocked delete reports failed; authz (non-manage_modules session
  → route-level reject); malformed action → 400.
- **Template tests**: `policyModulesList` renders no `pm-row-actions` buttons, renders the
  checkbox column with correct `data-select-caps` per origin, mounts toolbar + dialog.
- **JS contract**: rely on template-emitted attributes (as existing list features do);
  manual verification drives the flow end-to-end.
- **Definition of done** (AGENTS.md): `go build -buildvcs=false ./...` clean,
  `go test -buildvcs=false -count=1 ./...` green, `gofmt -l .` clean, `make gen` run
  after every `.templ` edit.
- **Manual drive** (owner/agent, server on :8081): select 2 tenant + 1 bundled module →
  Delete shows "(2 of 3)", dialog lists the bundled one as skipped with reason; confirm
  deletes the deletable ones; duplicate 2 modules → warning dialog appears; duplicate 1
  module → no dialog; header checkbox respects an active origin filter.

## Follow-ups (explicitly out of scope)

- Wire selection into groups, config groups, dependency groups, users, assets lists
  (each needs its own bulk endpoint + caps mapping).
- Category-specific actions for those lists.
- Route-table redesign for per-module grant reachability (existing handoff caveat).
