# Development Setup

**What:** how to get the Pluris console building, running, and seeded locally, plus the owner's persistent :8081 dev-hosting convention.
**Related:** [[testing]] [[workflow]] [[invariants]]

## Prerequisites

- Go 1.22+, Make, SQLite3 (usually pre-installed on Linux).
- Ubuntu/Debian quick install: `sudo apt update && sudo apt install -y golang-go build-essential`.
- No other runtime dependencies — the stack is Go, Echo v4, Templ, sqlc, SQLite (modernc, pure-Go driver), vanilla JS. No Node/npm, no bundler.

This is about the **console** (the app in this repo, `cmd/console/`), not the Pluris OS Linux distro build (that lineage — its getting-started doc condensed from a now-deleted document, full text in git history, plus `pkglists/`, `scripts/build-*.sh` — is a separate, currently-dormant sub-product; see [[os]]).

## Make targets

Source of truth: `Makefile` at the repo root (`make help` prints the same list).

| Target | What it does |
|---|---|
| `make doctor` | Verifies `go` is installed and reports whether `templ` is installed. |
| `make tools` | `go install github.com/a-h/templ/cmd/templ@v0.2.793` — the only dev tool needed beyond the Go toolchain. |
| `make gen` | `templ generate ./web/templates` — regenerates every `*_templ.go` from the `.templ` sources. |
| `make dev` | Runs `make gen` then `go run ./cmd/console` — console at `http://localhost:8080`. |
| `make build` | Runs `make gen` then builds `bin/pluris-console`. |
| `make test` | Runs `make gen` then `go test ./...` (no `-buildvcs=false` baked in — pass it yourself, see [[testing]]). |
| `make vet` | Runs `make gen` then `go vet ./...`. |
| `make clean` | Removes `bin/` and every generated `*_templ.go` under `web/templates`. |

**sqlc is not a Makefile target.** After editing `db/queries/*.sql` or adding a `db/schema/00X_*.sql` migration, run `sqlc generate` directly (config: `sqlc.yaml`, generates into `db/`). SQL comments in those files must be plain ASCII — no em-dashes, no apostrophes/contractions — sqlc's parser breaks on them.

## Running the console

```bash
make dev          # gen + run at :8080
```

- **Port**: controlled by `PLURIS_HTTP_ADDR` (`cmd/console/main.go`); defaults to `:8080` if unset.
- **Database**: SQLite file `pluris.db`, created automatically in the process's current working directory on first run (`console/server.New()` opens `"pluris.db"` by default; `NewWithDB(path)` exists so tests and alternate instances can point elsewhere). WAL mode, zero-config, auto-migrated via the embedded migration tracker (`db/schema/embed.go`, `pkg/database/database.go`) — migrations are embedded in the binary, so a binary built from this repo always has its schema even when run from an arbitrary directory.
- **Never touch the repo-root `pluris.db*`** — it is the owner's live GUI-testing database (AGENTS.md rule 1). Point any throwaway run at a different working directory or pass a scratch path to `NewWithDB` if writing test/tooling code.
- First run lands on the setup wizard (`/setup`) to create the initial admin account and tenant.

## The owner's :8081 dev-hosting server

The owner runs a persistent instance on port 8081, exposed via Tailscale Funnel at a placeholder hostname (`https://YOUR-DEV-HOST.example.com/` — substitute the real hostname from the owner's private notes; never commit a real internal hostname into tracked docs). Architecture: `Internet → Tailscale Funnel (HTTPS :443) → localhost:8080 → pluris-console (Go)` — despite the `:8081` shorthand used in conversation, check the live systemd unit / `restart-dev.sh` script for the actual bound port before assuming 8080 vs 8081.

Two components keep it running:
1. **Tailscale Funnel** (`--bg` mode), persists across reboots as part of Tailscale's serve config.
2. **systemd user service** `pluris-dev.service` (`~/.config/systemd/user/pluris-dev.service`), auto-restart, enabled with lingering.

**Stale-binary warning**: this server does not pick up code changes automatically. After any change relevant to what's being tested, rebuild and restart:

```bash
./scripts/restart-dev.sh
# or manually:
systemctl --user restart pluris-dev
```

Other useful commands: `journalctl --user -u pluris-dev -f` (logs), `systemctl --user status pluris-dev`, `sudo tailscale funnel status` (funnel health). After editing the unit file itself: `systemctl --user daemon-reload && systemctl --user restart pluris-dev`.

This is the owner's personal convention for manual browser walkthroughs — agents should not assume they have access to it; use `make dev` / `go run` locally instead, and hand off to the owner for the :8081 walkthrough step when a plan calls for manual verification.

## Seeder

```bash
go run -buildvcs=false ./cmd/seed --db=./scratch.db --tenant=demo
```

`cmd/seed/main.go` populates comprehensive demo data (sites, groups, assets, identities, etc.) for a tenant. Flags:
- `--db` — database file to seed; **always point this at a scratch file**, never the live `pluris.db`, unless intentionally seeding the dev server.
- `--tenant` — slug to seed into; empty = first existing tenant; `demo` = create-or-reuse a standalone demo tenant.

The seeder is idempotent: re-running against the same database is safe (it treats SQLite `UNIQUE constraint failed` as "already seeded" and either uses `ON CONFLICT DO NOTHING` or checks existence first — see `isUniqueConstraintErr` and `TestSeedIsIdempotent` in `cmd/seed/main_test.go`). The setup wizard (`console/handlers/auth.go` `SetupSubmit`) also best-effort calls role/dependency-group builtin seeding (`RoleService.EnsureBuiltins`, `DependencyGroupService.EnsureBuiltins`) — separate from, and always run before, the demo-data seeder.

## gendocs

```bash
go run ./cmd/gendocs
```

Regenerates `docs/endpoint-management/windows-admins/concepts.md` and `glossary.md` from `web/orientation/orientation.go` and `web/glossary/glossary.go` (`cmd/gendocs/*.go`, `docsRoot = "docs/endpoint-management/windows-admins"`). **These two files are the only auto-generated docs in the tree; never hand-edit them** — edit the Go source data file, then regenerate. `README.md` and `cheatsheet.md` in that same directory are hand-authored and untouched by the generator. CI-equivalent check: `go run ./cmd/gendocs && git diff --exit-code docs/endpoint-management/windows-admins/concepts.md docs/endpoint-management/windows-admins/glossary.md` must be clean.

## sqlc / templ regeneration rules (summary)

- Edited a `.templ` file → `make gen` (never hand-edit `*_templ.go`; gitignored).
- Edited `db/queries/*.sql` or added a new `db/schema/00X_*.sql` → `sqlc generate` (never hand-edit `db/*.sql.go`).
- **Never edit an already-applied migration file.** New schema change = new numbered `db/schema/00X_*.sql`. Migrations run once via the `schema_migrations` tracker (`pkg/database/database.go`); PRAGMA-bearing migrations run non-transactionally — read that file before adding one.
- Do not run `templ fmt` (see [[layout-system]] for why).
