# Pluris Project — Progress & Continuation Notes

**Last updated**: July 5, 2026 — after shipping the Users/Identity system + real
authentication (17-task implementation, see
`docs/superpowers/specs/2026-07-04-users-identity-login-design.md` and
`docs/superpowers/plans/2026-07-04-users-identity-login.md`). Everything below
was **verified against the actual code** (build + full test suite pass + live
end-to-end HTTP walkthrough), not carried over from earlier planning notes.

---

## What Pluris is

Open-source **Intune + Active Directory alternative** with a Group Policy
compatibility layer. Linux-first unified endpoint management with CMDB-style
asset tracking. Commercial model: AGPLv3 open source, paid support (paid tier
above ~500 devices under consideration).

## Canonical sources of truth (read in this order)

1. `docs/Pluris UX structure plan.md` — user-authored UX/IA spec (most important file)
2. `docs/UX_INVARIANTS.md` — Concept Registry + Single Source of Truth UI rules
3. `docs/ARCHITECTURE_DECISIONS.md` — ADR-001..009, append-only
4. `RULES.md` — operating rules
5. This file — current state and next steps

⚠️ `docs/MANAGEMENT_PLATFORM_RESEARCH.md`, `docs/FORK_STRATEGY.md`, and other
research docs contain **early planning that no longer matches the code**. Treat
them as historical context only.

---

## Actual technology stack (as built)

| Component | Technology | Status |
|-----------|-----------|--------|
| Backend | Go 1.22+ (Echo framework) | ✅ working |
| Web UI | Templ + HTMX-style server rendering | ✅ working |
| Database | **SQLite (WAL mode) + sqlc** — zero-config, auto-created | ✅ working |
| List/filter engine | `web/static/lists.{js,css}` + `filters-modern.{js,css}` | ✅ working |
| Messaging (NATS), Identity (Kanidm), agent, osquery, Wazuh | — | ❌ not started |

> **Divergence from ADRs, needs a decision**: the ADRs and old notes planned
> PostgreSQL + Ent ORM. The code uses SQLite + sqlc (a better fit for the
> "install = run one script" goal). No ADR records this switch. Either write
> ADR-010 accepting SQLite (with a documented SQLite→PostgreSQL growth path for
> large fleets), or migrate. Owner leans toward zero-config simplicity.

---

## Verified current state (what actually works)

- **Navigation shell**: 10 locked sidebar items, all routes mounted
  (`web/templates/menu.go` is the single source for the sidebar).
- **Authentication (NEW, July 2026)**: real local login — Argon2id password
  hashing, server-side sessions (`pkg/auth`), RBAC matching the locked role
  matrix in `docs/UX_INVARIANTS.md` (`super_admin`/`admin`/`user_self_service`),
  a first-run setup wizard (no default/shared credentials — `/setup` is forced
  until at least one identity exists), account lockout after 10 failed logins,
  CSRF protection on every mutating route, and a super_admin multi-tenant
  switcher. The console is **no longer open** — every route requires a valid
  session except `/setup`/`/login`/`/healthz`/`/static/*`.
- **Users directory (NEW, July 2026)**: `/users` is a real, database-backed
  list (same shared `web/lists` engine + column picker + filter builder as
  Assets — no bespoke UI), with full create/detail/edit/delete pages. Identity
  fields are rich and AD-familiar (username, UPN, org fields, contact, address,
  Windows profile/logon-script fields, security/login state) via a dedicated
  `catalog/params.SchemaIdentity` that reuses the same shared `tenant`/`site`
  params Assets already used.
- **Asset ↔ owner pairing (NEW, July 2026)**: Asset detail pages have a
  tenant-scoped owner picker; User detail pages show "Assigned assets".
  `owner_identity_id` on `assets` — previously an unused column — is now live.
- **Asset management**: Computers / Servers / Printers / Desks list + detail
  pages, wired to the real SQLite database through `pkg/services/AssetService`.
- **Parameter registry** (`catalog/params/`): 80+ parameters (assets + identity
  combined) — single source of truth for columns, filters, detail fields.
- **Unified list engine** (INV-L9): one filter/sort/search/count implementation
  shared by all list tables, including the new Users list.
- **Database layer**: identity schema rewritten (`db/schema/002_...sql`) with
  `identities` (rich AD-style), `identity_sessions`, `identity_audit_log`.
  ⚠️ A previous version of this migration `DROP TABLE`d and recreated the
  identity tables on **every app restart** (migrations re-run on every
  `Open()`, not just once) — found and fixed during this work. If you ever see
  user accounts mysteriously vanishing after a restart, check that no
  migration file has a `DROP TABLE` in it again; migrations here must be
  idempotent via `CREATE TABLE IF NOT EXISTS` only.
- **Extension framework** (`pkg/extension/`, ADR-008): types + catalog adapter,
  with tests.
- **Tests**: all tested packages pass (`go test ./...`), including a live,
  scripted end-to-end HTTP walkthrough (setup → restart-survival → login →
  lockout → user CRUD → asset pairing → tenant-switch absence with 1 tenant →
  logout → CSRF enforcement) run as the final verification step.

## Not built yet (do not assume otherwise)

- `pluris-agent`, NATS, enforcement of any kind
- ADMX parsing / GP catalog import (the main differentiator)
- Policy editor with syntax highlighting (ADR-006 UI)
- Policy Catalog / Configuration Groups / Modules pages are rendered from
  **hardcoded mock data in `catalog/`**, NOT from the database — the DB schema
  for them exists but the UI never touches it.
- External identity provider sync (Kanidm/AD/FreeIPA) — explicitly Phase 2 per
  ADR-009; the rich identity schema has no sync-tracking columns on purpose.
- Groups/OU assignment for identities — `group_memberships` supports it at the
  schema level, but there's no Groups UI/queries at all yet, so this wasn't
  wired up (would be building a picker for a feature that doesn't exist).
- **UX gap found during final verification**: a fresh `/setup` install has
  zero assets and currently **no in-app way to create one** — `cmd/seed`
  populates its own separate "Demo Organization" tenant, not the tenant a real
  admin creates via setup. Worth a doc note or an empty-state hint on
  `/assets/computers` until agent enrollment (or a manual "add asset" form)
  exists.
- **Design note, not a bug**: an account lockout blocks new login attempts but
  does not revoke sessions already issued to that account. Confirm this is the
  intended behavior before relying on lockout as an incident-response tool.

---

## Known structural debt (top refactor targets, in priority order)

1. **Mock-vs-DB split**: assets read from SQLite; policy/config-group/module
   pages read from Go mock catalogs. Two sources of truth — violates the
   project's own standardization rule. Unify behind the service layer.
2. **Template monoliths**: `web/templates/pages.templ` (~2,000 lines),
   `layout.templ` (~1,860), `menu.go` (~1,670). Split by feature area.
3. **Dead code** (verified with `go run golang.org/x/tools/cmd/deadcode@latest -test ./...`):
   21 unreachable functions remain, mostly in `web/templates/assets_helpers.go`
   (old formatting/icon helpers), `catalog/*/mock.go` (`ByID`,
   `CandidatesForPolicy`, `MockInstallations`, `InstallationsForAsset`), and
   `web/lists/lists.go`. Kept for now because upcoming UI work may rewire some;
   re-run the tool and delete whatever is still dead once the policy UI lands.
   Already deleted in the July 2026 cleanup: `web/static/filter-builder.js`
   (superseded filter engine, loaded by nothing), committed `pluris-console`
   binary, empty `paseo.json`.
4. **README ADR summaries were wrong** (fixed July 2026) — always cite ADRs
   from `docs/ARCHITECTURE_DECISIONS.md`, never from memory.

---

## How development works now (owner's workflow)

The owner (Peter) tests the running GUI and gives directions; the AI assistant
implements everything in code. Treat the owner as **an experienced Windows
admin building a platform he likes, not a developer** — explain technical
trade-offs in admin terms, keep the GUI reflecting real technical function,
and never add backend-only configuration (except scripts/modules).

Product principles from the owner (see `../dev_notes.txt`):
1. Easy migration from Windows GP — policy structure imported from Windows,
   enforcement via distro-aware **modules** with import/export and a proper
   script/parameter editor.
2. **One universal component per concept** — never two editors/lists/pickers
   for the same kind of thing. Structure and standardization above all.
3. Install must be "run a script, connect AD/Intune/SCCM" — scale from 3
   devices to very large fleets.
4. Community-friendly code: sensible comments, module development must be
   approachable for outside contributors.

## Next steps

1. Owner GUI testing round on the Users/login feature just shipped →
   instructions will follow from that session.
2. Consider closing the fresh-setup-has-no-assets UX gap noted above (either
   an in-app "add asset" form, or point `cmd/seed` at the setup-created tenant
   instead of its own separate demo tenant).
3. Unify policy/config-group/module pages onto the database (retire or demote
   mock catalogs to seed data).
4. Split the template monoliths as features get touched (don't big-bang it).
5. Decide ADR-010 (SQLite acceptance + growth path).
6. Then: standardized policy/module editor with syntax highlighting (ADR-006),
   and the ADMX/GP import pipeline.

## Build & run

```bash
make doctor            # verify Go
make tools             # install templ + sqlc
make gen               # regenerate templ + sqlc output (committed _templ.go not in repo)
make build             # binary → bin/pluris-console
make dev               # run at :8080 (PLURIS_HTTP_ADDR overrides)
go test ./...          # full test suite
```

Note: this working copy has no `.git` (downloaded archive); canonical history
lives on GitHub (owner: PolyBreaker1).
