# Dependency Groups — Design Spec

**Date:** 2026-07-06
**Author:** Claude (Opus 4.8) + Peter (owner)
**Status:** Approved, ready for implementation plan
**Sub-project:** SP2 of the 2026-07-06 feature batch (SP1 editable detail pages + add flows, SP2 dependency groups, SP3 standardized extension pages). This spec covers SP2 only.

---

## Goal

Give policy modules a **clear, interconnected applicability schema** so the platform (and, later, the endpoint agent + resolver) can decide whether a given policy module is a valid choice for a given device. A **Dependency Group** is a named, reusable applicability filter — Pluris's equivalent of a Windows **WMI filter**.

## Concept

A Dependency Group is an AND-set of **conditions** over **canonical device-fact paths** (the same parameter registry that drives list columns and policy targeting — INV-CPP). A policy module references dependency groups in two roles:

- **platform** — the device must match **ANY** referenced platform group (empty = platform-agnostic).
- **requirement** — the device must match **ALL** referenced requirement groups.

The resolution graph is fully connected:

```
Module --link(role: platform|requirement)--> DependencyGroup --> Condition --> canonical param path --> device fact
```

This "typed: platform ANY + requirement ALL" model was chosen (over flat match-ALL or flat match-ANY) because it expresses the real cases directly: a module that runs on RPM **or** Debian **and** requires disk encryption.

## Non-goals (for this slice)

- **Live evaluation against real device inventory.** There is no endpoint agent yet, so devices do not report most facts. The evaluator is written and unit-tested against a synthetic facts map; when a fact is absent it returns **unknown** ("needs agent inventory"), never a false `eligible`.
- **Boolean-expression / nested-group logic.** YAGNI. Disjunction is expressed by set-membership operators inside a single group; conjunction by adding conditions or requirement groups.
- **Module authoring persistence.** Modules remain the in-memory catalog mock. Dependency links reference module IDs as soft text slugs.

---

## Data model — migration `db/schema/004_dependency_groups.sql`

This is the **only** migration in SP2. Append-only; ASCII-only comments.

### `dependency_groups`
| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | |
| tenant_id | INTEGER NOT NULL | FK tenants |
| slug | TEXT NOT NULL | stable id for builtins, e.g. `rpm-based` |
| name | TEXT NOT NULL | |
| description | TEXT | |
| builtin | INTEGER NOT NULL DEFAULT 0 | 1 = seeded template; delete-protected |
| created_at / updated_at | TEXT | ISO8601 |

`UNIQUE(tenant_id, slug)`.

### `dependency_group_conditions`
| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | |
| group_id | INTEGER NOT NULL | FK dependency_groups ON DELETE CASCADE |
| param_path | TEXT NOT NULL | canonical path, e.g. `computer/hardware/os_family` |
| operator | TEXT NOT NULL | `in` \| `not_in` \| `exists` |
| values | TEXT NOT NULL | JSON array of strings (empty `[]` for `exists`) |
| seq | INTEGER NOT NULL | ordering within group |

All conditions in a group are **AND**-combined.

### `module_dependency_links`
| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | |
| tenant_id | INTEGER NOT NULL | FK tenants |
| module_id | TEXT NOT NULL | catalog mock slug; **no FK** (modules not in DB yet) |
| group_id | INTEGER NOT NULL | FK dependency_groups ON DELETE CASCADE |
| role | TEXT NOT NULL | `platform` \| `requirement` |

`UNIQUE(tenant_id, module_id, group_id)`.

---

## Device-fact vocabulary — parameter registry (no asset migration)

Assets store subtype facts in a JSON payload (`SubtypePayload`), read as `payload["os_family"]`. New facts are therefore **param-registry + schema-section additions only** — no asset-table columns.

| Param key | Label | Enum values | Status |
|---|---|---|---|
| `os_family` | OS family | linux, windows, macos | exists |
| `os_package_family` | Package format | rpm, deb, arch, apk, other | **new** |
| `disk_encryption` | Disk encryption | none, bitlocker, luks, filevault, other | **new** |

Mounted on the computer (and server) schema sections so they receive canonical paths (`computer/hardware/os_family`, etc.). The seeder (`cmd/seed`) emits the two new keys so demo devices carry values.

---

## Bundled default templates

Seeded per tenant, `builtin=1`, via an idempotent `EnsureBuiltins` (mirrors `roleSvc.EnsureBuiltins`):

| slug | name | condition |
|---|---|---|
| `rpm-based` | RPM-based OS | `os_package_family in [rpm]` |
| `debian-based` | Debian-based OS | `os_package_family in [deb]` |
| `arch-based` | Arch-based OS | `os_package_family in [arch]` |
| `any-linux` | Any Linux | `os_family in [linux]` |
| `windows` | Windows | `os_family in [windows]` |
| `disk-encryption-active` | Disk encryption active | `disk_encryption not_in [none]` |
| `bitlocker` | BitLocker enabled | `disk_encryption in [bitlocker]` |
| `luks` | LUKS enabled | `disk_encryption in [luks]` |

Condition `param_path` values use the canonical computer paths. Default **module→group links** are seeded for the bundled mock modules (e.g. sshd modules → platform `any-linux`; a disk-encryption module → requirement `disk-encryption-active`).

---

## Operators

`in`, `not_in`, `exists`. Single-value `in` renders "is X"; multi-value renders "is one of X, Y". Booleans are expressed as enum values, so no dedicated boolean operator is required.

---

## Evaluator — pure, unit-tested

`catalog/dependencygroups/eval.go`:

```
type Status string // "eligible" | "ineligible" | "unknown"

type GroupResult struct {
    GroupID int64
    Slug    string
    Name    string
    Role    string // platform | requirement
    Pass    string // "pass" | "fail" | "unknown"
    Reason  string
}

type Result struct {
    Status       Status
    Platforms    []GroupResult
    Requirements []GroupResult
}

func Eligible(links []ModuleLink, groups map[int64]Group, facts map[string]string) Result
```

Rules:
- Evaluate each linked group against `facts`. A group passes if **every** condition passes; is `unknown` if any condition references a fact not present in `facts` (and no earlier condition failed); fails otherwise.
- **Platform** links: overall platform passes if **ANY** platform group passes; is `unknown` if none pass but at least one is unknown; fails if all fail. No platform links → platform passes (agnostic).
- **Requirement** links: overall requirement passes only if **ALL** requirement groups pass; `unknown` if none failed but at least one is unknown; fails if any fails.
- Overall `Status`: `eligible` if platform passes AND requirements pass; `ineligible` if platform fails OR any requirement fails; otherwise `unknown`.

Condition evaluation:
- `in`: fact value ∈ values.
- `not_in`: fact present AND value ∉ values (absent fact → unknown).
- `exists`: fact present and non-empty.

---

## Service + queries

`pkg/services/dependencygroups.go` (`DependencyGroupService`):

- `EnsureBuiltins(ctx, tenantID)` — idempotent seed of the 8 templates + their conditions + default module links.
- `ListByTenant`, `Get`, `Create`, `Update`, `Delete` (Delete refuses `builtin=1`).
- `AddCondition`, `RemoveCondition`.
- `LinkModule`, `UnlinkModule`, `ListLinksForModule`.
- `Evaluate(ctx, tenantID, moduleID, facts)` — loads links+groups, calls the pure evaluator.

sqlc queries in `db/queries/dependency_groups.sql`. ASCII-only comments.

---

## UI

### Sidebar
New item under **Policy**, below **Modules**: **"Dependency Groups"** → `/policy/dependency-groups`, key `policy-dependency-groups` (add to `web/templates/menu.go`).

### List page — `/policy/dependency-groups`
Standardized list layout consistent with Configuration Groups. Columns via the list registry (INV-L): **Name · Conditions (summary) · Used by (module link count) · Type (Builtin/Custom)**. `data-testid="page-dependency-groups"`. "+ New group" button → `/policy/dependency-groups/new`.

### Detail / editor — `/policy/dependency-groups/:id` on DetailShell
- **General** tab — name, description (editable; builtins editable too).
- **Conditions** tab — one row per condition: param-path picker (from registry), operator, value(s). Add/remove rows (admin).
- **Modules** tab — modules linked to this group, grouped by role.
- **"+ New group"** (`/policy/dependency-groups/new`) lands on the same editable layout in a blank state (SP1-consistent). Builtins: delete disabled with tooltip.

### Modules page — `/policy/modules`
Each module row becomes **expandable** to reveal its dependency groups grouped by role: **Platforms:** … / **Requires:** …. An admin **"Manage dependencies"** control opens the per-module link editor (add/remove group links with a role). This satisfies "every module must have its dependencies set."

---

## RBAC

CRUD (create/update/delete groups, add/remove conditions, link/unlink modules) gated to **admin | super_admin**, mirroring `requireRoleAdmin` in `console/handlers/roles.go`. Technicians and users get read-only views.

---

## Testing

- **Evaluator unit tests** (`catalog/dependencygroups/eval_test.go`): eligible / ineligible / unknown; platform ANY (one of several passes); requirement ALL (one fails → ineligible); empty platform → agnostic; unknown fact → unknown not eligible.
- **Service round-trip** (`pkg/services/dependencygroups_test.go`): EnsureBuiltins idempotent (run twice, same counts); create group + condition + link, list back; Delete refuses builtin.
- **Handler tests** (`console/handlers/dependency_groups_test.go`): list + detail pages 200 with testids; create writes rows; RBAC 403 for a technician session.
- **Route smoke** (`console/server/server_test.go`): `/policy/dependency-groups` → 200 with `data-testid="page-dependency-groups"`.

Scratch DBs only (`t.TempDir`). Never touch repo-root `pluris.db`.

---

## Invariants respected

- **INV-CPP** — conditions reference canonical parameter paths.
- **INV-L** — the new list gets its columns from `web/lists/`.
- **DetailShell** — the group detail/editor uses the one detail-page layout.
- Append-only migration (004); PRAGMA-free so it runs transactionally; ASCII-only SQL comments.
- `-buildvcs=false` on all go commands; `make gen` after `.templ`; `sqlc generate` after `.sql`.
- No new external dependencies. Owner manages all git operations.

---

## File map

**Create:**
- `db/schema/004_dependency_groups.sql`
- `db/queries/dependency_groups.sql`
- `catalog/dependencygroups/types.go`, `defaults.go`, `eval.go`, `eval_test.go`
- `pkg/services/dependencygroups.go`, `dependencygroups_test.go`
- `console/handlers/dependency_groups.go`, `dependency_groups_test.go`
- `web/templates/dependency_groups.templ` (list + detail + editor), helpers `.go`
- `web/lists/dependency_groups.go`

**Modify:**
- `catalog/params/definitions.go` (+ schema section mounts) — new facts
- `cmd/seed/*` — emit new fact keys
- `catalog/policymodules/defaults.go` — bundled module→group default links (data for the seeder)
- `web/templates/menu.go` — sidebar item + key label
- `web/templates/pages.templ` or modules template — expandable module rows
- `console/server/server.go` — routes
- `console/handlers/auth.go` (SetupSubmit) + `UserDetail`/policy handlers — call `EnsureBuiltins` where appropriate
