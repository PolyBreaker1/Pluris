# Agent Rules — Pluris

Rules for ANY coding agent (Claude Code, opencode with Kimi/GLM/DeepSeek, etc.) working in this repo. These are strict. When unsure, stop and ask the owner instead of guessing.

## Absolute rules (never break)

1. **NEVER touch, delete, or write to `pluris.db*` in the repo root.** It is the owner's live GUI-testing database. All tests use `t.TempDir()` scratch paths.
2. **NEVER run git commands that change history or push** (`commit`, `push`, `checkout --`, `restore`, `stash`, etc.). The owner manages git manually. `git status` / `git diff` for reading are fine.
3. **All `go` commands need `-buildvcs=false`** (e.g. `go test -buildvcs=false -count=1 ./...`).
4. **After editing any `.templ` file, run `make gen`** (`templ generate ./web/templates`). Generated `*_templ.go` files are gitignored — never hand-edit them. Do **not** run `templ fmt`.
5. **After editing `db/queries/*.sql` or adding a `db/schema/*.sql` migration, run `sqlc generate`.** SQL comments must be plain ASCII — no em-dashes, no apostrophes/contractions (sqlc's parser breaks on them).
6. **Never edit an already-applied schema migration** (`db/schema/00X_*.sql`). New change = new numbered migration file. Migrations run once via the `schema_migrations` tracker (`pkg/database/database.go`); PRAGMA-bearing migrations run non-transactionally — read that file before adding one.
7. **Never hand-edit generated code**: `*_templ.go` (from `make gen`) and `db/*.sql.go` (from `sqlc generate`).
8. **Tests use `t.TempDir()` scratch databases only.**
9. **Definition of done**: `go build -buildvcs=false ./...` clean AND `go test -buildvcs=false -count=1 ./...` fully green AND `gofmt -l .` clean. Never leave the tree red.
10. **No new dependencies** without the owner's explicit OK. Stack is fixed: Go, Echo v4, Templ, sqlc, SQLite (modernc), vanilla JS — no new JS frameworks, no bundler.

## Architecture invariants

- **INV-CPP (Canonical Parameter Paths)**: every parameter is addressable as `entity/section/param` (e.g. `user/identity/email`, `computer/hardware/ram_mb`), derived from `catalog/params/` (`PathFor`, `ResolvePath`) — never hardcode or store paths in parallel.
- **INV-L (list registry)**: every table's columns come from `web/lists/` registrations — never write ad-hoc `<thead>` markup. Filters/sort/dividers go through `web/static/lists.js` (`data-pluris-*` attributes) — never a per-list script.
- **Detail pages**: every detail page uses `DetailShell` (`web/templates/detail_shell.templ`) — hero + tabs. Never invent a second detail layout.
- **Console authorization**: gated by the `catalog/permissions/` grant registry via `pkg/authz`/`pkg/auth`, not hardcoded role checks. Treat `pkg/auth`, `pkg/authz`, `catalog/permissions`, and `pkg/database` as strong-model-only territory — see `docs/development/workflow.md`.

## Doc map

Read `docs/INDEX.md` first. Concepts and UI reference live under `docs/endpoint-management/`. The dev process (spec → plan → execution, small-model task rules) is `docs/development/workflow.md`. Current shipped/in-flight state is `docs/development/handoff.md` — read it before starting or resuming any task. Setup/build is `docs/development/setup.md`; testing conventions are `docs/development/testing.md`.

Specs and plans for completed and in-flight work live under `docs/history/{specs,plans}/`, dated — permanent record, never deleted.

## Documentation rule (strict)

This doc tree does not accept new stray `.md` files. A new doc must fit the existing structure (`docs/endpoint-management/`, `docs/development/`, `docs/history/`, `docs/product/`) or it doesn't get written — raise it with the owner instead. Temporary working notes belong in `.superpowers/sdd/` (disposable), never loose in `docs/` or the repo root.

## Workflow

- Work one plan task at a time, in order, exactly as written. Do not improvise scope.
- Tests first when the plan says so.
- Before claiming done, re-check each requirement in the task against what you actually built.
- Update `docs/development/handoff.md` when you finish or abandon a task, so the next agent starts accurately.
- Run the dev server for manual checks with `PLURIS_HTTP_ADDR=:8081 go run -buildvcs=false ./cmd/console` (or `make dev` for the :8080 default).
