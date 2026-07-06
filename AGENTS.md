# Agent Rules — Pluris

Rules for ANY coding agent (Claude Code, opencode with Kimi/GLM/DeepSeek, etc.) working in this repo. These are strict. When unsure, stop and ask the owner instead of guessing.

## Absolute rules (never break)

1. **NEVER touch, delete, or write to `pluris.db*` in the repo root.** It is the owner's live GUI-testing database. All tests must use `t.TempDir()` scratch paths (existing tests show the pattern).
2. **NEVER run git commands that change history or push.** The owner manages git manually. `git status` / `git diff` for reading is fine.
3. **All `go` commands need `-buildvcs=false`** (example: `go test -buildvcs=false -count=1 ./...`).
4. **After editing any `.templ` file run `make gen`** (templ is at `~/go/bin/templ`; Makefile uses it). Generated `*_templ.go` files are gitignored — never edit them by hand.
5. **After editing `db/queries/*.sql` or `db/schema/*.sql` run `sqlc generate`.** SQL comments must be plain ASCII: no em-dashes, no apostrophes/contractions — sqlc's parser breaks on them.
6. **Never edit applied schema migrations** (`db/schema/00X_*.sql` that already ran). New schema changes = new numbered migration file. Migrations run once via the `schema_migrations` tracker in `pkg/database/database.go`; PRAGMA-bearing migrations run non-transactionally (see that file before adding one).
7. **Definition of done for every task:** `go build -buildvcs=false ./...` clean AND `go test -buildvcs=false -count=1 ./...` fully green. Never leave the tree red.
8. **No new dependencies** without the owner's explicit OK. Stack is fixed: Go, Echo v4, Templ, sqlc, SQLite (modernc), vanilla JS.

## Architecture invariants

- **INV-CPP (Canonical Parameter Paths):** every parameter is addressable as `entity/section/param` (e.g. `user/identity/email`, `computer/hardware/ram_mb`). Paths are DERIVED from the registry in `catalog/params/` (`PathFor`, `ResolvePath`) — never hardcode or store them in parallel.
- **INV-L (list registry):** every table's columns come from `web/lists/` registrations — never write ad-hoc `<thead>` markup.
- **Detail pages:** all detail pages use `DetailShell` (`web/templates/detail_shell.templ`) — hero + tabs. Never invent a second detail layout.
- **UX invariants:** read `docs/UX_INVARIANTS.md` before UI work.

## Where things are

| What | Where |
|---|---|
| Current implementation plan (Tasks 1-16) | `docs/superpowers/plans/2026-07-05-standardized-detail-pages.md` |
| Approved design spec | `docs/superpowers/specs/2026-07-05-standardized-detail-pages-design.md` |
| Work state + exactly what to do next | `docs/agent/HANDOFF.md` |
| Which tasks fit smaller models | `docs/agent/SMALL-MODEL-TASKS.md` |
| Funding plan (docs work, no code) | `docs/funding/` |
| Param registry (source of truth) | `catalog/params/` |
| DB layer (sqlc generated + migrations) | `pkg/database/`, `db/` |
| HTTP handlers / routes | `console/handlers/`, `console/server/server.go` |
| Templates | `web/templates/` |

## Workflow

- Work ONE plan task at a time, in order, exactly as written in the plan file. Do not improvise scope.
- Tests first when the plan says so; the plan contains the test code and expected failures.
- Before claiming done, re-read the task's requirements in the plan and check each one.
- Update `docs/agent/HANDOFF.md` (the "Current state" section) when you finish or abandon a task, so the next agent starts accurately.
- Run the dev server for manual checks with: `PORT=8081 go run -buildvcs=false ./cmd/console` (owner tests GUI on :8081).
