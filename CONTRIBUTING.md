# Contributing to Pluris

Thanks for your interest. Pluris is pre-beta and moving fast, so the most valuable contributions right now are: trying the quickstart and reporting what breaks, reviewing the data model and policy catalog against real-world AD/Intune experience, and small focused PRs.

## Build and test

```bash
make tools                                  # install templ + sqlc (once)
make gen                                    # regenerate templ templates
go build -buildvcs=false ./...
go test -buildvcs=false -count=1 ./...      # must be fully green before any PR
```

`make dev` runs the console at http://localhost:8080. The SQLite database is created automatically; delete your local `*.db` files to start fresh (never commit them).

## Ground rules

- **Generated code is never edited by hand:** `*_templ.go` comes from `make gen`, `db/*.sql.go` from `sqlc generate`. Edit the `.templ` / `.sql` sources.
- **Migrations are append-only.** Never modify an existing file in `db/schema/`; add a new numbered migration. SQL comments must be plain ASCII (sqlc parser limitation).
- **UI work must respect `docs/UX_INVARIANTS.md`**: table columns come from the `web/lists/` registry, detail pages use the `DetailShell` component, parameters are addressed by canonical paths (`entity/section/param`) derived from `catalog/params/`.
- **Tests use scratch databases** (`t.TempDir()`), never a repo-root path.
- No new dependencies without discussion in an issue first.

## Where to start

- `docs/ARCHITECTURE_DECISIONS.md` — why things are the way they are
- `docs/PARAMETER-REGISTRY.md` — how to add a parameter/column end to end
- `docs/superpowers/plans/` — the active implementation plan (open tasks)

## AI-assisted contributions

AI-generated PRs are welcome if you reviewed them yourself and the full test suite passes. Agents working in this repo must follow `AGENTS.md`.

## License

By contributing you agree your contributions are licensed under AGPLv3 (see LICENSE).
