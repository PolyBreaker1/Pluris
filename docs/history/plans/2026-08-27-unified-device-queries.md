# unified-device-queries - Work Plan

## TL;DR (For humans)

**What you'll get:** One "Configuration Group" that does everything — it holds a device query (nested AND/OR conditions, bash/script detection, reusing the existing parameter picker), refreshes its member list on a schedule you control (time-based, on-new-usage, or a manual button), and shows the computed devices in a Members tab. The separate "Dependency Groups" feature disappears from the sidebar and is fully absorbed.

**Why this approach:** The condition engine and the condition-builder dialog already exist and are shared by three features — this plan extends them rather than rebuilding. Merging into Configuration Groups (not renaming) keeps the URL space and mental model stable.

**What it will NOT do:** It does not touch the AD-style Groups page, does not build the endpoint agent (script conditions keep showing "unknown" until it exists), and adds no external scheduler or new dependencies.

**Effort:** XL
**Risk:** High — it migrates live data (existing dependency groups, module links) and rewrites the shared eval engine. Migration is idempotent and verified before old tables are dropped.
**Decisions to sanity-check:** CG gains `slug`/`is_builtin`/`match_mode` columns; query tree stored as adjacency-list nodes; `manage_dependency_groups` permission folds into `manage_config_groups`; nested groups are CG-only for now (dynamic group rules and module tests stay flat).

Your next move: review the mock (`docs/history/plans/2026-08-27-unified-device-queries-mock.html`), then start work with `/start-work`.

---

> TL;DR (machine): XL effort, high migration risk; delivers merged CG with nested query tree + refresh scheduler + computed members; DG family removed.

## Scope

### Must have
1. Migration `013_unified_queries.sql`: extend `configuration_groups` (`slug`, `is_builtin`, `match_mode`); create `configuration_group_query_nodes` (adjacency tree), `configuration_group_refresh_triggers`, `configuration_group_members`; migrate all DG rows/conditions/module links with ID remap; fold retention entity kind; then drop the three DG tables.
2. Recursive query engine: `catalog/dependencygroups` types gain a `Node` (condition | group) tree; `EvalGroup` walks it with ternary pass/fail/unknown per block, depth ≤ 5, nodes ≤ 100, cycle guard. Flat consumers (dynamic group rules, module version tests) keep working unchanged.
3. CG detail page: new **Query** tab (nested tree UI + shared condition builder), new **Refresh** tab (trigger list + add-trigger form + "Refresh now"), new **Members** tab (computed device list with last-refresh stamp).
4. Refresh scheduler: in-process goroutine (pattern of `startPurgeScheduler`), per-CG lease, `next_run_at`/`last_run_at`/`last_run_status`, failure retry with visible error state; on-usage trigger fires on binding/module-link/software-assignment creation (debounced); manual `POST /policy/groups/:id/refresh`.
5. Permission consolidation: `endpoint_policy.manage_dependency_groups` removed; existing grants migrated to `endpoint_policy.manage_config_groups`; module-link handlers stay under `manage_modules`.
6. Old-concept cleanup: `/policy/dependency-groups*` routes 302-redirect to `/policy/groups*`; DG menu item removed; DG service/handlers/templates/lists/tests deleted after a zero-unmigrated-rows verification.
7. Docs: spec at `docs/history/specs/2026-08-26-unified-device-queries.md`; update `docs/INDEX.md`, `docs/development/handoff.md`, `docs/endpoint-management/ui/invariants.md` (add INV-CGQ/CGR/CGM), and windows-admins glossary.
8. Mock already delivered: `docs/history/plans/2026-08-27-unified-device-queries-mock.html`.

### Must NOT have (guardrails, anti-slop, scope boundaries)
- NO changes to AD-style `/groups` dynamic membership (shares the engine but stays flat — engine changes must be backward-compatible).
- NO changes to module version tests UI semantics (they keep the flat condition builder).
- NO external cron / new Go dependencies / JS frameworks.
- NO endpoint agent work — script/bash conditions keep the "unknown until agent reports" contract (`script_result/<condition-id>`).
- NO rename of "Configuration Groups" and NO route change for `/policy/groups`.
- NO changes to the module publish/supersede/revoke state machine.
- NEVER touch `pluris.db*` in repo root; tests use `t.TempDir()` only.

## Verification strategy
> Zero human intervention — all verification is agent-executed.
- Test decision: **tests-after** (repo convention), framework: Go `testing` + existing suite conventions; template tests for new tabs.
- Definition of done (per AGENTS.md): `go build -buildvcs=false ./...` clean AND `go test -buildvcs=false -count=1 ./...` green AND `gofmt -l .` clean.
- After any `.templ` edit: `make gen`. After any `db/queries/*.sql` or schema edit: `sqlc generate`.
- Evidence: `.omo/evidence/task-<N>-unified-device-queries.txt` per todo.

## Execution strategy

### Parallel execution waves
- **Wave 1 (foundation, sequential):** todos 1-2 — schema + engine types. Everything depends on these.
- **Wave 2 (parallel):** todos 3 (migration logic), 4 (engine eval rewrite) — independent files.
- **Wave 3 (parallel):** todos 5 (CG service merge), 6 (refresh scheduler service), 7 (permission migration).
- **Wave 4 (parallel):** todos 8 (Query tab UI), 9 (Refresh tab UI), 10 (Members tab UI) — separate template files.
- **Wave 5 (parallel):** todos 11 (module-link retarget + routes), 12 (DG removal + redirects), 13 (docs + spec).
- **Wave 6 (final):** todo 14 — full verification.

### Dependency matrix
| Todo | Depends on | Blocks | Can parallelize with |
| --- | --- | --- | --- |
| 1. Schema migration 013 | — | 2,3,5,6,7 | — |
| 2. Node tree types | 1 | 4,8 | — |
| 3. DG→CG data migration | 1 | 12 | 4 |
| 4. Recursive eval engine | 2 | 5,8 | 3 |
| 5. CG service (query CRUD + members) | 4 | 8,9,10,11 | 6,7 |
| 6. Refresh scheduler service | 1 | 9 | 5,7 |
| 7. Permission fold + grant migration | 1 | 12 | 5,6 |
| 8. Query tab + nested builder | 2,5 | — | 9,10 |
| 9. Refresh tab UI | 6 | — | 8,10 |
| 10. Members tab UI | 5 | — | 8,9 |
| 11. Module link retarget | 5 | 12 | — |
| 12. DG removal + redirects | 3,7,11 | 14 | 13 |
| 13. Docs + spec | 11 | 14 | 12 |
| 14. Final verification | 12,13 | — | — |

## Todos
> Implementation + Test = ONE todo.

- [ ] 1. Schema migration `db/schema/013_unified_queries.sql`
  What to do: ALTER `configuration_groups` ADD `slug TEXT`, `is_builtin BOOLEAN NOT NULL DEFAULT FALSE`, `match_mode TEXT NOT NULL DEFAULT 'all'`; UNIQUE(tenant_id, slug) index (nullable slugs get generated `cg-<id>`). CREATE `configuration_group_query_nodes` (id, configuration_group_id FK CASCADE, parent_id NULL FK→self CASCADE, node_type CHECK('group'|'condition'), match_mode TEXT, seq INT, kind TEXT, param_path TEXT, operator TEXT, value_json TEXT, script_source TEXT, script_ref TEXT, created_at). CREATE `configuration_group_refresh_triggers` (id, configuration_group_id FK CASCADE, kind CHECK('interval'|'on_usage'), interval_value INT, interval_unit TEXT, at_time TEXT, enabled BOOL DEFAULT TRUE, next_run_at TIMESTAMP, last_run_at TIMESTAMP, last_run_status TEXT, last_run_error TEXT, created_at). CREATE `configuration_group_members` (configuration_group_id FK CASCADE, asset_id FK, matched_at, refresh_run_id; UNIQUE(group_id, asset_id)). CREATE `configuration_group_refresh_locks` (configuration_group_id PK, locked_until). Update sqlc queries file `db/queries/configuration_groups.sql` and new `db/queries/cg_query_nodes.sql`, `db/queries/cg_refresh.sql`; run `sqlc generate`.
  Must NOT: edit any earlier migration; no PRAGMA in this migration unless handled per `pkg/database/database.go` non-transactional path.
  Parallelization: Wave 1 | Blocked by: — | Blocks: 2,3,5,6,7
  References: `db/schema/001_initial.sql:201-250` (CG tables), `db/schema/004_dependency_groups.sql:8-46` (DG tables to absorb), `db/schema/009_group_kinds_rules.sql:51-61` (column-parity reference), `pkg/database/database.go` (migration runner), `db/queries/dependency_groups.sql` (query shapes).
  Acceptance criteria: `go build -buildvcs=false ./...` clean; new tables exist after boot against a scratch DB; `sqlc generate` produces no diff drift.
  QA scenarios: happy — boot with fresh DB, assert all 4 tables + 3 new columns exist (sqlite PRAGMA table_info). failure — boot twice, assert migration runs once (schema_migrations guard). Evidence: `.omo/evidence/task-1-unified-device-queries.txt`
  Commit: Y | feat(db): unified query schema for configuration groups

- [ ] 2. Query node domain types in `catalog/dependencygroups/types.go`
  What to do: Add `Node` struct {Type ('group'|'condition'), MatchMode, Children []Node, Condition *Condition} and `QueryTree` {Root Node}. Add `MaxQueryDepth = 5`, `MaxQueryNodes = 100` constants. Add `ValidateQueryTree(root)` (depth, count, cycle-safe by construction, per-condition payload validation via existing `validateConditionPayload` logic — move it from `pkg/services/dependencygroups.go:90-120` into the catalog package so it's shared). Keep flat `Group`/`Conditions` untouched for existing consumers. Unit tests: validation rejects depth 6, rejects 101 nodes, accepts valid tree.
  Must NOT: change `EvalGroup` signature yet; do not break `group_rules.go` or `policymodules_conditions.go` callers.
  Parallelization: Wave 1 | Blocked by: 1 | Blocks: 4,8
  References: `catalog/dependencygroups/types.go:106-199` (Condition/MatchMode/Group), `pkg/services/dependencygroups.go:90-120` (validation to move), `pkg/services/group_rules.go:255-267` (flat consumer to keep working).
  Acceptance criteria: `go test -buildvcs=false -count=1 ./catalog/dependencygroups/` green incl. new `TestValidateQueryTree*`.
  QA scenarios: happy — valid 3-level tree validates. failure — depth-6 tree and cycle-shaped input rejected with typed errors. Evidence: `.omo/evidence/task-2-unified-device-queries.txt`
  Commit: Y | feat(catalog): query node tree types with validation

- [ ] 3. DG→CG data migration (Go-side, in the same migration window)
  What to do: In `pkg/database` migration hook for 013 (or a dedicated `migrateUnifiedQueries` step run once, tracked in schema_migrations): for each tenant — insert CG row per DG (name, slug, is_builtin, match_mode, description; preserve deleted_at/deleted_by), build `(old_dg_id → new_cg_id)` map; insert query-node rows (root group node with DG match_mode; conditions as child nodes preserving kind/param/operator/value_json/script_source/script_ref/seq); rewrite `module_dependency_links.group_id` to new CG IDs; handle name collisions with suffix `-migrated`; fold `retention_settings` rows with entity_kind `dependency_group` into `configuration_group` (keep stricter of the two); verify zero unmigrated rows, then DROP `dependency_group_conditions`, `module_dependency_links` is KEPT but its FK target documented as CG (recreate table with FK to configuration_groups), DROP `dependency_groups`. Idempotent: skip if no DG table. Tests with scratch DB seeded with DG fixtures incl. builtin groups + soft-deleted row + collision case.
  Must NOT: lose any DG row; never touch repo-root pluris.db.
  Parallelization: Wave 2 | Blocked by: 1 | Blocks: 12 | With: 4
  References: `pkg/services/dependencygroups.go:149-166` (builtin seeds to re-home), `db/schema/004_dependency_groups.sql`, `db/schema/010_soft_delete_retention.sql:13-17`, `pkg/services/retention.go:14-24` (entity kinds).
  Acceptance criteria: `go test -buildvcs=false -count=1 ./pkg/database/ -run TestUnified` green: fixture with 3 DGs (1 builtin, 1 custom with 4 conditions, 1 soft-deleted) migrates to 3 CGs with correct node trees, links retargeted, retention folded, DG tables gone.
  QA scenarios: happy — full migration asserted row-by-row. failure — name collision gets `-migrated` suffix; rerun is a no-op. Evidence: `.omo/evidence/task-3-unified-device-queries.txt`
  Commit: Y | feat(db): migrate dependency groups into configuration groups

- [ ] 4. Recursive eval engine in `catalog/dependencygroups/eval.go`
  What to do: Add `EvalQueryTree(root Node, facts map[string]string) Status` implementing ternary logic: group ALL = fail if any child fails, pass if all pass, else unknown; group ANY = pass if any child passes, fail if all fail, else unknown; leaf conditions use existing `evalCondition`/`evalOperator`/`evalScriptCondition` unchanged. Keep `EvalGroup`/`Eligible` behavior identical for flat callers (wrap: flat Conditions → implicit root group). Tests: nested (linux AND rpm) OR (debian AND luks); fail-dominates-unknown in ALL; pass-dominates-unknown in ANY; empty group = vacuous per match mode; script unknown propagation; depth guard.
  Must NOT: change operator semantics or the script_result/<id> agent contract; no changes to `Eligible`'s platform/requirement framing (callers retarget in todo 11).
  Parallelization: Wave 2 | Blocked by: 2 | Blocks: 5,8 | With: 3
  References: `catalog/dependencygroups/eval.go:37-297` (current engine), `catalog/dependencygroups/eval_test.go`, `pkg/services/group_rules.go:223-298` (flat caller contract).
  Acceptance criteria: `go test -buildvcs=false -count=1 ./catalog/... ./pkg/services/...` green — existing flat tests untouched and passing plus new `TestEvalQueryTree*` cases.
  QA scenarios: happy — documented truth table cases all pass. failure — over-depth tree returns unknown+error, never panics. Evidence: `.omo/evidence/task-4-unified-device-queries.txt`
  Commit: Y | feat(catalog): recursive nested query evaluation

- [ ] 5. CG service: query CRUD + member computation in `pkg/services/configgroups.go` (+ new `cg_query.go`, `cg_members.go`)
  What to do: Methods: `GetQueryTree(ctx, cgID)`, `SaveQueryTree(ctx, cgID, tree)` (validate via todo 2, transactional replace of nodes), `EvaluateMembership(ctx, cgID)` — builds facts per asset via existing `FactsForAsset`, runs `EvalQueryTree`, reconciles `configuration_group_members` rows in one transaction, stamps `matched_at`/`refresh_run_id`, returns added/removed/total counts. `ListMembers(ctx, cgID)` joins members→assets for the Members tab. Builtin CGs: query read-only (same protection as DG builtins: `ErrBuiltinProtected`, match_mode locked for builtins). Absorb DG service's `EnsureBuiltins` as `EnsureBuiltinQueries` on the CG service (seed builtin query CGs: any-linux, rpm-based, debian-based, arch-based, windows, disk-encryption-active, bitlocker, luks — idempotent by slug).
  Must NOT: change assignment/binding methods; keep `FindOrCreateByName`/`AssignPolicyDirect` behavior for the policy picker.
  Parallelization: Wave 3 | Blocked by: 4 | Blocks: 8,9,10,11 | With: 6,7
  References: `pkg/services/configgroups.go:66-725` (existing service), `pkg/services/group_rules.go:223-330` (reconcile pattern to mirror), `pkg/services/facts.go:53-146` (FactsForAsset), `pkg/services/dependencygroups.go:149-216` (builtins).
  Acceptance criteria: `go test -buildvcs=false -count=1 ./pkg/services/ -run TestConfigGroup` green incl. new: save+reload tree round-trip; eval writes members; builtin query immutable.
  QA scenarios: happy — fixture assets produce expected member set after EvaluateMembership. failure — invalid tree rejected, members table unchanged (transaction rollback). Evidence: `.omo/evidence/task-5-unified-device-queries.txt`
  Commit: Y | feat(services): CG query storage and membership computation

- [ ] 6. Refresh scheduler: `pkg/services/cg_refresh.go` + registration in `console/server/server.go`
  What to do: `RefreshService` wrapping the CG service: `RefreshNow(ctx, cgID)` (manual), `RunDue(ctx)` (scheduler tick), `EnqueueUsageRefresh(cgID)` (debounced 30s window per CG, in-memory map). Ticker: `startQueryRefreshScheduler(db)` goroutine, 1-minute `time.Ticker`, registered next to `startPurgeScheduler` in `server.go`; per tick: select triggers where `enabled AND next_run_at <= now AND cg.deleted_at IS NULL`; per CG acquire lease via `configuration_group_refresh_locks` (skip if locked); run EvaluateMembership; update `last_run_at`, `next_run_at` (interval triggers), `last_run_status`/`last_run_error`; on error log + record status, retry next tick (no backoff beyond that). On-usage hook: called from binding-add, module-link-add, assignment-add service paths (single `EnqueueUsageRefresh` call each). `RefreshNow` runs synchronously for the manual button. Tests: past-due trigger fires and reschedules; lock prevents double-run; usage enqueue debounces (3 calls → 1 run); failure records last_run_error and CG stays intact.
  Must NOT: add dependencies; no cron library; do not run at boot for all CGs (only due ones).
  Parallelization: Wave 3 | Blocked by: 1 | Blocks: 9 | With: 5,7
  References: `console/server/server.go:82,382-418` (purger pattern to mirror), `pkg/services/retention.go:97-137` (per-item error isolation pattern), `pkg/services/group_rules.go:217-222` (the deferred-scheduler note this resolves).
  Acceptance criteria: `go test -buildvcs=false -count=1 ./pkg/services/ -run TestCGRefresh` green (ticker logic tested via direct RunDue call with seeded trigger rows; no real sleeping).
  QA scenarios: happy — due interval trigger computes members and stamps next_run_at. failure — EvaluateMembership error → last_run_status='error', next tick retries, no goroutine leak (test with cancel ctx). Evidence: `.omo/evidence/task-6-unified-device-queries.txt`
  Commit: Y | feat(services): query refresh scheduler with interval/usage/manual triggers

- [ ] 7. Permission fold: drop `manage_dependency_groups`
  What to do: Remove `endpoint_policy.manage_dependency_groups` from `catalog/permissions/registry.go` and role templates in `catalog/permissions/templates.go`. In migration 013 (todo 3's Go hook or a sibling step): rewrite every stored grant row (role_grants/overrides/group_role-derived caches — inspect `pkg/authz` storage) from `manage_dependency_groups` to `manage_config_groups`. Every handler currently gating on the DG key switches to `endpoint_policy.manage_config_groups`. Module link/unlink keeps gating on `manage_modules`. Tests: old key resolves to nothing; migrated grants grant CG management; module-link route still requires manage_modules.
  Must NOT: leave any reference to the old key; do not change the grant matrix UI structure.
  Parallelization: Wave 3 | Blocked by: 1 | Blocks: 12 | With: 5,6
  References: `catalog/permissions/registry.go:58-59`, `catalog/permissions/templates.go:30-141`, `console/handlers/dependency_groups.go:104` (current DG gate), `console/handlers/config_groups.go:94` (CG gate), `pkg/authz/` (grant storage).
  Acceptance criteria: `go test -buildvcs=false -count=1 ./pkg/authz/ ./console/handlers/ -run 'Permission|Grant'` green; grep for `manage_dependency_groups` returns zero hits.
  QA scenarios: happy — migrated technician role manages CGs. failure — unknown old key after migration is ignored, not an error. Evidence: `.omo/evidence/task-7-unified-device-queries.txt`
  Commit: Y | feat(authz): fold dependency-group permission into configuration groups

- [ ] 8. Query tab UI + nested builder (templates + JS)
  What to do: CG detail gains tabs order: General, **Query**, **Refresh**, **Members**, Assignments, Policy Bindings (`web/templates/config_groups_helpers.go` `configGroupTabs`). Query tab (`config_group_query.templ`): renders the node tree from `GetQueryTree` as nested group blocks (root + children, indent per level, per-group match-mode select, Add condition / Add group / Remove group controls — matching the approved mock); condition rows show status dot (param=evaluates-now green, bash/script=unknown grey) and kind chip. Extend `ConditionBuilderDialog` with a "Place into" selector (target group node id, or "new nested group") — the ONLY change to the shared dialog; dynamic-group rules and module tests mounts pass a flag that hides it (`data-cb-nesting="off"`). Save handler: `POST /policy/groups/:id/query/nodes` (add), `/nodes/:nid` (update/remove), `/nodes/:nid/match-mode`; all go through `SaveQueryTree` server-side so validation is single-pointed. Inline JS in the CG script block pattern (like existing `config_groups.templ` wiring). Template tests: tree renders nested; nesting-off consumers unchanged.
  Must NOT: fork the condition dialog into a second implementation; no new JS framework; do not change `dependency_groups.templ` (deleted in todo 12, not edited).
  Parallelization: Wave 4 | Blocked by: 2,5 | Blocks: — | With: 9,10
  References: mock `docs/history/plans/2026-08-27-unified-device-queries-mock.html` (Query tab + dialog), `web/templates/config_groups.templ:154-365` (tab pattern), `web/templates/condition_builder.templ:64-194`, `web/static/condition-builder.js:552-574` (save event), `web/templates/dependency_group_detail_helpers.go:36-42` (tabs being replaced).
  Acceptance criteria: `go test -buildvcs=false -count=1 ./web/templates/ ./console/handlers/ -run 'CGQuery|ConditionBuilder'` green; `make gen` run; manual check via dev server: add nested group + conditions, reload, tree persists.
  QA scenarios: happy — create 2-level query via dialog, tree renders nested after reload. failure — depth-limit rejection shows inline banner, no partial save. Evidence: `.omo/evidence/task-8-unified-device-queries.txt`
  Commit: Y | feat(web): CG query tab with nested condition builder

- [ ] 9. Refresh tab UI
  What to do: Refresh tab (`config_group_refresh.templ`) per mock: status strip (last refreshed, members delta, next run) + "Refresh now" button → `POST /policy/groups/:id/refresh` (redirect back with flash counts like `GroupRecalculate`); triggers table via `DetailTableFrame` (columns: Trigger kind, Configuration, Last triggered, Next run, On toggle, Remove); add-trigger form (kind select toggling interval fields [every N minutes/hours/days/weeks + at time] vs on-usage event checkboxes [binding added / module linked / software assigned]); toggle → `POST /policy/groups/:id/refresh-triggers/:tid/toggle`, remove → `/remove`, add → `POST .../refresh-triggers`. Error state: if `last_run_status='error'` show persistent inline banner with `last_run_error`. Handlers in `console/handlers/config_groups_refresh.go` gated by `manage_config_groups`.
  Must NOT: use browser alert() — persistent inline feedback only; no JS time-picking library (native `input[type=time]`).
  Parallelization: Wave 4 | Blocked by: 6 | Blocks: — | With: 8,10
  References: mock `docs/history/plans/2026-08-27-unified-device-queries-mock.html` (Refresh tab), `web/templates/config_groups.templ:213-274` (assignments tab pattern), `console/handlers/groups_pages.go:461-488` (recalc flash pattern), `web/templates/list_mass_actions.templ` (shared components).
  Acceptance criteria: `go test -buildvcs=false -count=1 ./console/handlers/ -run 'CGRefresh'` green; manual check: add interval trigger → row appears; Refresh now → members timestamp updates.
  QA scenarios: happy — full CRUD round-trip on a trigger. failure — invalid interval (0 minutes) rejected with inline error. Evidence: `.omo/evidence/task-9-unified-device-queries.txt`
  Commit: Y | feat(web): CG refresh tab with trigger management

- [ ] 10. Members tab UI
  What to do: Members tab (`config_group_members.templ`): `DetailTableFrame`-based table (columns: Name, OS, RAM, Last seen, Matched since) fed by `ListMembers`; header note "Computed from the query · refreshed <time>"; empty state via `ConceptEmptyState` variant ("No devices match yet — adjust the query or refresh"). Register columns in `web/lists/` registry per INV-L (new list ID `config-group-members`). Rows link to the asset detail (`data-row-href`).
  Must NOT: hand-write `<thead>` (registry only); no per-list JS (use `data-pluris-*`).
  Parallelization: Wave 4 | Blocked by: 5 | Blocks: — | With: 8,9
  References: mock (Members tab), `web/lists/detail_tabs.go` (registry pattern), `pkg/services/configgroups.go` ListMembers (todo 5), `web/templates/detail_shell.templ:163-183` (DetailTableFrame).
  Acceptance criteria: template test asserts member rows render from fixtures; list registry returns the new columns; `go test ./web/...` green.
  QA scenarios: happy — 4 fixture members render with links. failure — empty members shows the empty state, not a broken table. Evidence: `.omo/evidence/task-10-unified-device-queries.txt`
  Commit: Y | feat(web): CG members tab listing computed devices

- [ ] 11. Module applicability retarget (DG links → CG queries)
  What to do: Move `ModuleDependencyAdd`/`ModuleDependencyRemove` logic from `console/handlers/dependency_groups.go:275-312` into `console/handlers/policy_module_deps.go`, now resolving CGs via the CG service (module_dependency_links.group_id already points at CG IDs after todo 3). Module editor "Reusable dependency groups" section becomes "Reusable configuration group queries" — same chips UI, backed by CG list (`web/templates/policy_module_editor.templ:440-444`, `pages.templ:1522-1583` moduleDepsDetails). `DependencyGroupService.Evaluate/EvaluateForAsset` → CG service equivalents using `EvalQueryTree` (module eligible if its linked platform/requirement CG queries pass — roles preserved from `module_dependency_links.role`). Wire the on-usage hook: successful link add calls `EnqueueUsageRefresh(cgID)`.
  Must NOT: change `Eligible`'s platform-any/requirement-all semantics; no module publish-flow changes.
  Parallelization: Wave 5 | Blocked by: 5 | Blocks: 12 | With: —
  References: `console/handlers/dependency_groups.go:275-312`, `console/handlers/policy_module_deps.go`, `catalog/dependencygroups/eval.go:222-286` (Eligible), `web/templates/pages.templ:1522-1583`.
  Acceptance criteria: `go test -buildvcs=false -count=1 ./console/handlers/ ./pkg/services/ -run 'ModuleDep|Eligible'` green; module editor shows CG queries as chips; link add triggers usage refresh (assert via service spy in test).
  QA scenarios: happy — link module to CG query, chip appears, evaluation path returns expected verdict on fixture facts. failure — linking to nonexistent CG 404s cleanly. Evidence: `.omo/evidence/task-11-unified-device-queries.txt`
  Commit: Y | feat(policy): module gating on configuration group queries

- [ ] 12. DG removal + redirects
  What to do: Redirect handlers: `/policy/dependency-groups` → `/policy/groups`, `/policy/dependency-groups/new` → `/policy/groups/new`, `/policy/dependency-groups/:id` → resolve new CG id by old slug then 302 to `/policy/groups/:id` (fallback: list). Delete: `pkg/services/dependencygroups.go`, `console/handlers/dependency_groups.go`, `console/handlers/dependency_group_bulk.go`, `web/templates/dependency_groups.templ`, `web/templates/dependency_group_detail_helpers.go`, `web/lists/dependency_groups.go`, all DG test files; remove menu item in `web/templates/menu.go:62`; remove routes in `server.go:243-251,344`; retention service drops the DG entity-kind purge branch. Prerequisite guard in code review: verification query result (zero rows left referencing old tables) attached to evidence.
  Must NOT: delete anything before todo 3's verification passes; keep `catalog/dependencygroups` package (it hosts the shared engine — only the DG-specific service layer dies).
  Parallelization: Wave 5 | Blocked by: 3,7,11 | Blocks: 14 | With: 13
  References: `console/server/server.go:243-253,344`, `web/templates/menu.go:60-62`, Metis findings §5 (leak inventory).
  Acceptance criteria: grep `dependency_group` / `DependencyGroup` in non-generated code returns only migration 013 internals + catalog engine comments; `curl -s -o /dev/null -w '%{http_code}' http://localhost:8081/policy/dependency-groups` → 301/302; full build+test green.
  QA scenarios: happy — old URLs redirect, sidebar shows no DG entry. failure — unknown old DG id redirects to CG list (no 500). Evidence: `.omo/evidence/task-12-unified-device-queries.txt`
  Commit: Y | refactor(policy): remove dependency groups, redirect to configuration groups

- [ ] 13. Docs + spec
  What to do: Write `docs/history/specs/2026-08-26-unified-device-queries.md` (the design record: decisions, schema, engine semantics, refresh model). Update `docs/development/handoff.md` (what shipped; new caveats), `docs/INDEX.md` (link spec + update concepts), `docs/endpoint-management/concepts/endpoint-policy.md` (CG now owns queries), `docs/endpoint-management/architecture/data-model.md` (new tables), `docs/endpoint-management/ui/invariants.md` (add: INV-CGQ one query builder everywhere; INV-CGR one refresh scheduler; INV-CGM computed members surface), windows-admins glossary/cheatsheet (DG term removed, "query" term added). Update `docs/development/Plans/` copy of this plan with final state.
  Must NOT: create stray docs outside the existing structure; do not rewrite history specs.
  Parallelization: Wave 5 | Blocked by: 11 | Blocks: 14 | With: 12
  References: `docs/development/workflow.md` (spec convention), `docs/endpoint-management/ui/invariants.md:68-70` (nav invariant to edit), AGENTS.md doc rules.
  Acceptance criteria: docs mention no live "Dependency Groups" feature (only historical/migration notes); `docs/INDEX.md` links resolve.
  QA scenarios: happy — grep docs for stale concept. failure — broken wiki-link check by manual read. Evidence: `.omo/evidence/task-13-unified-device-queries.txt`
  Commit: Y | docs: unified device queries spec and handoff

- [ ] 14. Final verification
  What to do: Run the full definition of done: `go build -buildvcs=false ./...`, `go test -buildvcs=false -count=1 ./...`, `gofmt -l .` (expect empty), `make gen` produces no diff. Boot dev server on :8081 with a scratch DB, walk: CG list → new CG → build 2-level nested query → save → Refresh now → Members tab populated → add interval trigger → check row → visit /policy/dependency-groups → redirected. Record outputs.
  Parallelization: Wave 6 | Blocked by: 12,13 | Blocks: —
  References: AGENTS.md definition of done.
  Acceptance criteria: all commands exit 0; manual walkthrough passes each step.
  QA scenarios: as listed. Evidence: `.omo/evidence/task-14-unified-device-queries.txt`
  Commit: N

## Final verification wave
> Runs in parallel after ALL todos. ALL must APPROVE. Surface results and wait for the user's explicit okay before declaring complete.
- [ ] F1. Plan compliance audit — every Must have present, every Must NOT absent (grep + test evidence).
- [ ] F2. Code quality review — no dead DG code paths, no duplicated condition logic, migration idempotent.
- [ ] F3. Real manual QA — the todo-14 walkthrough executed against a running console.
- [ ] F4. Scope fidelity — no /groups changes, no agent work, no new dependencies.

## Commit strategy
One commit per todo, types as listed, in wave order. No history rewriting (owner manages git manually — worker only stages/commits when explicitly told; default: leave changes uncommitted per AGENTS.md rule 2 unless the start-work session says otherwise). Wait — AGENTS.md says agents must NEVER commit. So: **no commits at all**; each todo leaves a clean, tested working tree and the owner commits. (Commit lines above describe the suggested message only.)

## Success criteria
1. CG detail has General/Query/Refresh/Members/Assignments/Policy Bindings tabs; query tree nests to depth 5; builder dialog is the single shared one.
2. Refresh triggers (interval + on-usage + manual) compute `configuration_group_members` and surface status/errors in the Refresh tab.
3. All pre-existing DG data (incl. builtins, soft-deleted, module links) survives migration into CGs; DG tables/routes/nav/tests are gone with redirects in place.
4. `manage_dependency_groups` no longer exists; migrated grants work.
5. `go build`, `go test`, `gofmt` all green; existing flat-condition consumers (dynamic groups, module tests) unchanged and passing.
6. Mock delivered at `docs/history/plans/2026-08-27-unified-device-queries-mock.html`; spec delivered at `docs/history/specs/2026-08-26-unified-device-queries.md` (note: not found in the tree as of the 2026-09-03 doc cleanup — this plan is the authoritative source until/unless that spec resurfaces).
