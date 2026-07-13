# Architecture overview

**What:** The Pluris console's technology stack, repo layout, request flow, and the catalog-package pattern that makes every UI surface read from a single source of truth.

**Related:** [[decisions]], [[data-model]], [[parameters]], [[identity-assets]]

## Stack

- **Go 1.25**, single binary (`cmd/console`), no external services required to run.
- **Echo v4** — HTTP router and middleware chain (`console/server/server.go`).
- **Templ** (`github.com/a-h/templ`) — typed Go HTML templates, compiled to `*_templ.go` by `templ generate`.
- **sqlc** over **SQLite** (WAL mode) — type-safe generated query code; no ORM. See [[data-model]].
- **Vanilla JS** for interactivity (`web/static/detail.js`, `web/static/lists.js`, `web/static/filters-modern.js`) — no HTMX, no frontend framework, no npm build step.

This supersedes earlier planning docs (condensed from now-deleted documents, full text in git history — the original management-interface design doc and ADR-001/002) which described PostgreSQL + Ent + NATS + Kanidm + HTMX + FrankenUI. None of that is in the current code: there is no NATS, no agent fork, no Kanidm/external identity provider, no Ent. Identities, sessions, and password hashes are stored directly in the Pluris SQLite database (`pkg/auth/session.go`, `pkg/auth/password.go`). See [[decisions]] for the record of this drift.

## Repo layout

| Path | Contents |
|---|---|
| `cmd/console` | Main binary entrypoint. |
| `cmd/seed` | Test-data seeder. |
| `cmd/gendocs` | Regenerates `docs/endpoint-management/windows-admins/{concepts,glossary}.md` from `web/orientation` and `web/glossary`. |
| `console/server` | Echo router wiring (`server.go`) — the full middleware chain and route table. |
| `console/handlers` | HTTP handlers, one file per feature area (auth, assets, groups, roles, dependency groups, field API, avatar, Pluris Policy/RBAC editor). |
| `catalog/` | Pure-Go registries: `params`, `permissions`, `policies`, `policymodules`, `dependencygroups`, `configgroups`, `identities`, `assets`. See "Catalog packages" below. |
| `db/schema` | The 5 SQL migration files plus `embed.go` (embeds them into the binary). |
| `db/queries` | Hand-written `.sql` query files sqlc compiles into `db/*.sql.go`. |
| `pkg/database` | `Database` wrapper: `Open`, connection pragmas, migration runner. |
| `pkg/auth` | Session management, password hashing, the Echo middleware chain (`SetupGate`, `RequireAuth`, `RequireRole`), RBAC route-permission table. |
| `pkg/authz` | Grants resolution: `Grants` type, scope combinators (`Can`, `CanScoped`, `Union`), DB-backed `Service` that loads/saves `roles.permissions`. |
| `pkg/services` | Business-logic layer between handlers and sqlc queries (`AssetService`, `IdentityService`, `RoleService`, `GroupService`, `DependencyGroupService`, field-update validation). |
| `pkg/extension` | Generic framework for versioned/signed/lifecycle-managed content kinds (Policy Modules today; Profiles/Scripts/Wine Configs/Packages are documented future kinds). |
| `web/templates` | Templ source (`.templ`) and generated (`_templ.go`) page/component templates. |
| `web/lists` | List-view (table) rendering helpers shared by every entity list page. |
| `web/static` | Vanilla JS/CSS: `detail.js` (inline field editing), `lists.js`/`filters-modern.js` (table filtering/columns). |
| `web/orientation`, `web/glossary` | In-product help content; source of truth for the generated Windows-admin docs. |
| `docs/` | Documentation, including this tree. |

## Request flow

Every request passes through the middleware chain registered in `console/server/server.go`, in this order:

1. **Recover / Logger / Gzip** — Echo standard middleware.
2. **`no-store` Cache-Control** — forces every response to bypass browser cache (pre-beta correctness-over-caching tradeoff).
3. **`auth.SetupGate`** — redirects everything to `/setup` until at least one identity exists anywhere in the database (no seeded default account).
4. **`auth.RequireAuth`** — resolves the session cookie (`pluris_session`) via `SessionManager.Lookup`, loads the identity, and resolves **grants**: `super_admin` sessions get an unconditional bypass grant (`authz.BypassKey`); every other session calls `authz.Service.EffectiveGrants` to compute the identity's effective `authz.Grants` map (own role + inherited group roles + role-inheritance chain, see [[decisions]] and [[authorization]]) and stashes both the session and grants on the request context.
5. **`auth.RequireRole`** — checks the resolved grants against the route's required permission key (`pkg/auth/rbac.go`'s route→permission table, longest-prefix match) via `CanAccessGrants`. Despite the name, it is a permission check, not a hard-coded role check.
6. **Echo CSRF middleware** — form + `X-CSRF-Token` header lookup (the header path exists so `detail.js`'s JSON `fetch` calls can also pass validation).
7. Route handler. Handlers that need finer-grained, scope-aware checks (e.g. "own" vs "all") call `requirePermissionScoped` directly — the route-level gate is necessarily coarse (path-based), so field-level and record-level authorization happens in the handler (see `console/handlers/field_api.go`).

`/api/*` routes are not listed in the route-permission table, so route-level middleware only requires *some* authenticated session; the actual authorization is the handler's own `requirePermissionScoped` call. This is deliberate, not a gap — see [[authorization]].

## The catalog-packages pattern

Every domain concept that needs to be a single source of truth across list columns, detail pages, filters, and validation lives in a **pure Go package under `catalog/`** with zero (or near-zero) dependencies on echo/templ/sqlc:

- `catalog/params` — the canonical parameter registry (`ParamDef`, `SubtypeSchema`, canonical paths). See [[parameters]].
- `catalog/permissions` — the RBAC action registry (`Domain`/`Action`) and the four builtin role template matrices.
- `catalog/policies` — the Pluris Policy Catalog (Windows-GP-equivalent settings), bundled + tenant-custom entries in one flat list.
- `catalog/policymodules` — Policy Module manifests, defaults, and the `extension.Extension` adapter.
- `catalog/dependencygroups` — the pure applicability model (conditions over device fact keys) used to gate which modules apply to which assets.
- `catalog/configgroups` — Configuration Group target-kind model (still in-memory/mock; persistence is a later increment — see [[decisions]]).
- `catalog/identities`, `catalog/assets` — the canonical in-memory shapes (`Identity`, `Asset`) that `pkg/services` adapts database rows into, plus the editable/non-editable field allowlists that both the UI and the field-update service read (`NonEditableFieldKeys`, `SelfServiceEditableKeys`, `NonEditablePayloadKeys`).

The pattern: define a concept exactly once in its catalog package (type, registry, validation rules), and every consumer — list rendering, detail-page sections, filter dropdowns, the field-update API — reads through that single definition rather than re-declaring field lists. `pkg/extension` generalizes the shared shape (source/lifecycle/versioning/signature) across kinds that need it (Policy Modules today).

## Codegen workflow (`make gen`)

The Makefile's `gen` target currently runs `templ generate ./web/templates` only. The full codegen surface a contributor touches:

1. **`templ generate ./web/templates`** (`make gen`) — compiles `.templ` sources into `_templ.go`. Run before `make dev`/`make build`/`make test`.
2. **`sqlc generate`** (not yet wired into the Makefile; run manually, per `sqlc.yaml`) — regenerates `db/*.sql.go` from `db/schema/*.sql` + `db/queries/*.sql` whenever either changes.
3. **`go run ./cmd/gendocs`** (not yet wired into the Makefile) — regenerates `docs/endpoint-management/windows-admins/{concepts,glossary}.md` from `web/orientation` and `web/glossary`. CI is intended to enforce freshness via `go run ./cmd/gendocs && git diff --exit-code docs/` (see `cmd/gendocs/main.go`'s header comment), but no CI config currently exists to run it.

`make build` and `make test` both depend on `make gen` first.
