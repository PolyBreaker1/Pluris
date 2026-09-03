# Data model

**What:** Schema tour of the SQLite database by domain, the 9 migration files, the migration-runner discipline, and sqlc conventions.

**Related:** [[overview]], [[decisions]], [[parameters]], [[identity-assets]]

Source of truth: `db/schema/001_initial.sql` through `011_module_tests_origin.sql`, embedded into the binary via `db/schema/embed.go` and applied by `pkg/database.Database.migrate()`.

## Migration discipline

- **Append-only.** New schema changes are new numbered files; existing files are never edited (except the one sanctioned rebuild in 003, below).
- **Run-once tracker.** A `schema_migrations(filename, applied_at)` table records which migration files have run; `migrate()` re-checks this on *every* `Open()` call (every process start), not just once ever — so migration files after 002 may safely contain non-idempotent statements (`ALTER TABLE`) precisely because the tracker guarantees each file executes exactly once per database. Migrations 001–002 are additionally idempotent via `CREATE TABLE IF NOT EXISTS` as defense in depth.
- **No `DROP TABLE`**, with one sanctioned exception: migration 003's `identities` rename-rebuild (SQLite cannot `ALTER` a `CHECK` constraint, so widening the `role` enum requires copy-new-table → drop-old → rename). This is safe *only* because the tracker guarantees it runs exactly once. An earlier version of migration 002 broke this rule (a `DROP TABLE identities` on every restart, wiping user data) — the incident is documented in that file's header comment as a permanent warning.
- **Embedded FS.** Migrations ship inside the compiled binary (`db/schema/embed.go`'s `Files embed.FS`), so a built console migrates correctly regardless of working directory. A missing embedded file is treated as a build defect and fails closed (returns an error), never silently skips.
- **PRAGMA handling.** SQLite cannot change `journal_mode` inside a transaction and silently ignores `foreign_keys` pragma changes mid-transaction. Migrations whose SQL text contains an actual `PRAGMA` statement (001, 003) run as bare `Exec` calls followed by a separate "record applied" `Exec` — not wrapped in a transaction, so the migration file must manage its own atomicity. PRAGMA-free migrations (002, 004–009) run schema + tracker-insert together inside one transaction, so a crash mid-migration rolls back cleanly and the migration re-runs on next boot.
- **Single-writer, in-process concurrency only.** Connection pool is capped at `MaxOpenConns(1)`; cross-*process* races are out of scope because Pluris deploys as a single server process.

## Per-migration contents

**001_initial.sql** — the core hierarchy and policy skeleton: `tenants`, `sites`, `groups`, `assets` (subtype-discriminated with JSON `subtype_payload`), `asset_links` (typed asset-to-asset relations), `group_memberships` (asset XOR identity per row via `CHECK`), `custom_policies`, `policy_modules` + `policy_module_versions` (immutable, signed), `configuration_groups` + `configuration_group_bindings` + `configuration_group_assignments`, `module_installations` + `module_installation_dependencies` (refcount edges). Note: this file intentionally does *not* create `identities` — the header comment explains why (002 owns that table; see below).

**002_identity_ad_compat.sql** — the full AD-attribute-compatible `identities` table (account/organization/contact/location/profile-and-scripts/security/Pluris-specific field groups — see [[identity-assets]] for the AD mapping), `identity_sessions` (token-hash based, tenant-scoped), `identity_audit_log`. `role` starts as a 3-value `CHECK` (`super_admin`, `admin`, `user_self_service`) — widened in 003.

**003_roles_software_logs.sql** — `roles` (per-tenant custom RBAC roles, `is_builtin`, `template_slug`, `permissions` JSON), `identity_roles` (identity↔role assignment), `installed_software`, `activity_log` (generic per-entity audit feed); adds `assets.description` and `assets.managed_by_identity_id`; adds `groups.group_category`/`groups.group_scope` (AD-style classification); and rebuilds `identities` to widen `role` to `super_admin | admin | technician | user` (renaming `user_self_service` → `user` via a `CASE` in the copy `INSERT`) — the one sanctioned `DROP TABLE`.

**004_dependency_groups.sql** — `dependency_groups` (named, reusable applicability filter, per tenant), `dependency_group_conditions` (one AND-ed predicate per row, keyed by canonical parameter path — see [[parameters]]), `module_dependency_links` (module→group link tagged `platform` or `requirement` role; `module_id` is a catalog/URN string, no FK — modules are identified by URN across the tenant/bundled split, not a single numeric id space).

**005_role_hierarchy_group_roles.sql** — adds `roles.parent_role_id` (self-referencing, `ON DELETE SET NULL`; builtin roles never have a parent, enforced in service code not the schema) and `group_roles` (group↔role assignment; group members inherit the group's roles in addition to any directly-assigned roles).

**006_condition_builder.sql** — widens `dependency_group_conditions` with `kind` (`param | script`, default `param`) and script fields (`script_source`, `script_expect`); widens `dependency_groups` with `match_mode` (`all | any`). Enum values validated in Go (`pkg/services/dependencygroups.go`), not a `CHECK` — SQLite's `ALTER TABLE` cannot add `CHECK` constraints to an existing table, the reasoning every migration from here on repeats in its own header.

**007_module_ownership_grants.sql** — `policy_modules.owner_identity_id` (nullable FK to `identities`; NULL = bundled/unowned) and `module_grants` (`module_id, subject_type, subject_id, level`; `subject_type` ∈ `identity|group|role`, `level` ∈ `view|edit|admin`, hierarchical). Deliberately the ONE grants table for both Policy Modules and the future Scripts feature (a script is modeled as an unpackaged module — same tables, no parallel `script_grants` table). See `docs/history/specs/2026-07-12-module-grants-and-ownership.md`.

**008_module_scripts.sql** — reconciles `policy_module_versions` (rename→create→copy-forward→drop, since SQLite can't just drop columns cleanly and the semantics changed): drops `runtime` (now derived per-phase in Go) and the single-script-per-phase columns (`enforce_script`/`validate_script`/`rollback_script`), replacing them with the child table `policy_module_scripts` (one row per `(version_id, phase, seq)`, multiple scripts per phase). Adds `sandbox_profile` (JSON) and `report_schema` (JSON Schema). `manifest_yaml` is kept but repurposed as a derived export artifact, not the source of truth. See `docs/history/specs/2026-07-12-module-persistence-and-param-injection.md`.

**009_group_kinds_rules.sql** — adds `groups.description`/`member_kind` (`asset|identity|mixed`)/`membership` (`static|dynamic`)/`rules_match_mode` (`all|any`, reusing 006's `match_mode` semantics), `group_memberships.source` (`direct|rule`), and the new table `group_membership_rules` (schema-parity with `dependency_group_conditions` — same condition-builder/eval machinery, reused wholesale rather than reinvented). See `docs/history/specs/2026-07-12-dynamic-groups.md`.

**010_soft_delete_retention.sql** — adds `deleted_at`/`deleted_by` soft-delete columns to every deletable entity plus the per-entity-kind `retention_settings` table (`purge_after_days`, `mode`). See the soft-delete spec under `docs/history/specs/`.

**011_module_tests_origin.sql** — the modular-module-system migration (2026-07-17 spec): adds `script_ref` to `dependency_group_conditions` and `group_membership_rules` (a script-kind condition may reference a library script instead of carrying inline source); rewrites legacy `script_expect` rows into the standardized operator/value expectation (the column is dead from 011 on — INV-TEST: every test is subject, operator, expected value, across `param | command | script` kinds); creates `module_version_conditions` (per-version module tests, column parity with the other two condition tables, `version_id` FK CASCADE); adds `policy_module_versions.conditions_match_mode` (`all|any`) and `policy_modules.origin` (`bundled|tenant|imported`, backfilled from `is_bundled` — makes imported modules representable). See `docs/history/specs/2026-07-17-modular-module-system-design.md`.

There is no `schema_migrations`-tracked migration beyond 011 as of this writing; `sessions` in the generic sense is `identity_sessions` (002).

## Domain tour

### Tenancy / sites / groups

`tenants` is the multi-tenant isolation root; every other table cascades from it. `sites` are geographic/network boundaries scoped to a tenant. `groups` are assignment-target collections (AD-style `group_category`/`group_scope` added in 003; `description`/`member_kind`/`membership`/`rules_match_mode` added in 009 — see below); `group_memberships` links either an asset or an identity (never both — enforced by a `CHECK`) into a group, with a `source` column (009: `direct | rule`) distinguishing hand-added members from rule-reconciled ones. `group_roles` (005) additionally lets a group carry roles that its members inherit. `group_membership_rules` (009) mirrors `dependency_group_conditions`' shape and drives dynamic-membership groups through the same condition-builder/eval engine as Dependency Groups — see `docs/history/specs/2026-07-12-dynamic-groups.md`.

### Identities

See [[identity-assets]] for the full AD-attribute mapping. Summary: `identities` carries account/organization/contact/location/profile-script/security field groups plus Pluris-specific fields (`role`, `avatar_url`, `locale`, `timezone`). `identity_sessions` and `identity_audit_log` are the auth-adjacent tables.

### Assets + JSON subtype payload pattern

`assets` has a `subtype` discriminator (`computer | server | printer | desk`, `CHECK`-constrained) and a `subtype_payload` JSON column holding subtype-specific fields (hostname/OS for computer/server, IP/consumables for printer, dock/monitor for desk). Column-vs-payload storage routing: fields that are frequently queried, joined, or filtered at the SQL level (tenant, site, subtype, enrollment_state, lifecycle_state, owner, timestamps, human_id) are dedicated columns with indexes; everything else lives in the JSON payload and is read/written through `catalog/assets`'s typed `SubtypePayload` structs (`ComputerPayload`, `ServerPayload`, `PrinterPayload`, `DeskPayload`), not raw JSON manipulation in handlers. `asset_links` models asset-to-asset relationships (`peripheral_of`, `docked_to`, `printed_via`, `hosts_vm`, `connected_to`, `other`) as one table with a `relation` enum rather than a table per relation type.

### Roles / identity_roles / group_roles / permissions JSON / parent_role_id

See [[decisions]] DEC-012/013/014 and [[authorization]] for the full RBAC model. Schema summary: `roles.permissions` stores only a role's *own* grant overrides as JSON (`"domain.action": "none|own|all|no|yes"`), not the resolved matrix; `roles.parent_role_id` (005) chains custom roles for inheritance; `identity_roles` and `group_roles` are the two assignment tables a session's effective grants are unioned from.

### Configuration groups / bindings / assignments

`configuration_groups` bind a set of policy-catalog entries to a target. `configuration_group_bindings` is the policy-URN + parameter-values + module-selection row (one per policy per group; `parameter_values` validated against the bound module's `parameters_schema` — single-source, values never define params). `configuration_group_assignments` is the polymorphic target link (`target_type` ∈ `asset|identity|group|site|tenant`, `target_id` referencing the matching table; `priority` + `enforced` mirror Windows GP semantics). **Real and DB-backed** (shipped in the 2026-07 overhaul, Task 5.2): `pkg/services/configgroups.go`'s `ConfigGroupService` does full CRUD against these tables — `catalog/configgroups` is now pure domain types only (the mock, `catalog/configgroups/mock.go`, was deleted along with the dialog UI it backed). See `docs/history/specs/2026-07-06-dependency-groups-design.md` for the sibling dependency-group model this pattern followed and [[handoff]] for current status.

### Dependency groups / conditions / module links

`dependency_groups` + `dependency_group_conditions` + `module_dependency_links` implement the applicability-filter model in `catalog/dependencygroups`: a dependency group is an AND-set of conditions over canonical parameter paths (e.g. `computer/hardware/os_package_family in [rpm]`), now authored through the shared `ConditionBuilderDialog` with a widened operator set and an optional script-condition kind (006 — see `docs/history/specs/2026-07-12-condition-builder-and-script-conditions.md`); a policy module links to one or more groups in a `platform` (match-any) or `requirement` (match-all) role. Fully wired: `pkg/services/dependencygroups.go`'s `DependencyGroupService` persists and evaluates these, with seeded builtins (`rpm-based`, `debian-based`, `arch-based`, `any-linux`, `windows`, disk-encryption variants) that are editable but not deletable (`ErrBuiltinProtected`).

### Policy modules tables

`policy_modules` (module identity: URN, tenant — null for bundled; `owner_identity_id` + `module_grants`, 007, for per-module access) + `policy_module_versions` (immutable once published: `manifest_yaml` now a derived export artifact rather than the source of truth, target_os, scope, `satisfies` URNs, dependency/conflict lists, sandbox_profile/report_schema JSON, Ed25519 signature fields — reconciled in 008) + `policy_module_scripts` (one row per version/phase/seq, replacing 001's single-script-per-phase columns). `module_installations` (per-asset installed module + pinned version + state) + `module_installation_dependencies` (edges for refcount-based safe uninstall, per ADR-007/INV-M1–M4; these two tables are not yet written by any service — no agent exists to report installations, see [[handoff]]). **Real and DB-backed** (Task 4.2–4.4): `pkg/services/policymodules.go`'s `PolicyModuleService` implements the full draft/publish/supersede/revoke state machine — `catalog/policymodules` is domain types + a `Catalog()` provider hook fed by the service, not a hardcoded mock. See `docs/history/specs/2026-07-12-module-persistence-and-param-injection.md` and `docs/history/specs/2026-07-12-module-grants-and-ownership.md`.

### installed_software, activity_log, schema_migrations, identity_sessions

`installed_software` — per-asset software inventory row (`pkg_type` CHECK across deb/rpm/appimage/flatpak/snap/wine/native/other). `activity_log` — generic `(tenant, entity_type, entity_id, event, detail, actor_identity_id)` audit feed, written by handlers like the field-update API on every successful mutation. `schema_migrations` — the migration tracker table described above. `identity_sessions` — the session table backing `pkg/auth.SessionManager` (token hash, not raw token, stored; `active_tenant_id` supports the super_admin tenant-switch flow).

## sqlc conventions

- **Named params.** Queries use `@name` placeholders (e.g. `WHERE id = @id`), not positional `?` or `$1` — `db/queries/*.sql` throughout.
- **ASCII comments only** in `.sql` files (no smart quotes/em-dashes) — consistent with the rest of the Go codebase's comment style.
- One `.sql` file per domain (`assets.sql`, `identities.sql`, `roles.sql`, `dependency_groups.sql`, `configuration_groups.sql`, `policy_modules.sql`, `module_installations.sql`, `module_grants.sql`, `group_rules.sql`, `group_members_detail.sql`, `tenants.sql`, `sessions.sql`, `activity_log.sql`, `identity_audit.sql`, `installed_software.sql`, `asset_links.sql`, `custom_policies.sql`, `group_detail.sql`), matching the schema domains above. `custom_policies.sql`'s generated CRUD (`CreateCustomPolicy`/`UpdateCustomPolicy`) has zero Go call sites today — `custom_policies.parameters_schema` is dead schema, documented in `docs/history/specs/2026-07-12-module-persistence-and-param-injection.md`.
- `sqlc.yaml` config: SQLite engine, `emit_interface: true` (generates the `Querier` interface `pkg/database.Database.Queries` satisfies), `emit_json_tags` with `snake_case`, nullable columns generate `sql.NullString`/`sql.NullInt64`/`sql.NullTime` (not pointer types — `emit_pointers_for_null_types: false`), enum-backed columns get a generated `Valid()`-style check (`emit_enum_valid_method`).
- Regeneration: edit `.sql`, run `sqlc generate` (not yet wired into `make gen` — see [[overview]]), commit the regenerated `db/*.sql.go`.
