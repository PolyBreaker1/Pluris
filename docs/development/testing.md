# Testing Conventions

**What:** the patterns every test in this repo follows — scratch databases, session injection, mount-point smoke tests, render assertions, headless e2e, and the definition of a green suite.
**Related:** [[setup]] [[workflow]] [[invariants]]

## Scratch databases only

Every test that touches SQLite uses `t.TempDir()` to build a throwaway path — **never** a repo-relative or `pluris.db` path:

```go
dbPath := filepath.Join(t.TempDir(), "seed_test.db")
if err := run(dbPath, "demo"); err != nil { ... }
```

or, for HTTP-layer tests, `server.NewWithDB(dbPath)` instead of `server.New()` (which defaults to `"pluris.db"`). This is an absolute rule (AGENTS.md #1) — the repo-root `pluris.db*` is the owner's live GUI-testing database and must never be written to, deleted, or even opened by a test run.

## `-buildvcs=false` everywhere

Every `go build` / `go test` / `go vet` invocation in this repo must pass `-buildvcs=false`:

```bash
go build -buildvcs=false ./...
go test -buildvcs=false -count=1 ./...
go vet -buildvcs=false ./...
```

(Note the `Makefile`'s `make test`/`make vet` targets do **not** bake this flag in — pass it explicitly when running tests yourself; CI-equivalent verification always includes it.)

## Handler-test session injection pattern

HTTP handler tests build an `*auth.UserSession` with pre-resolved grants and inject it into the request context, bypassing the login flow entirely:

```go
req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups/...", body)
req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
    TenantID: tenantID,
    UserID:   userID,
    Grants:   authz.Grants(permissions.TemplateGrants("admin")), // or "user", "technician", "super_admin"
}))
```

`permissions.TemplateGrants(slug)` (`catalog/permissions/templates.go`) returns the full builtin grant matrix for a role template, so tests can exercise both the happy path (`"admin"`) and the RBAC-denial path (`"user"`) without touching the database's role tables. See `console/handlers/dependency_groups_test.go`, `console/handlers/avatar_test.go`, `console/handlers/field_api_test.go` for the pattern across different handler families. Full-flow tests (through `/login`) exist too where the login/CSRF machinery itself is what's being tested (`console/server/server_test.go`'s `newTestServer` helper does a real login and returns the session cookie).

## `mountPoints` smoke tests

`console/server/server_test.go`'s `mountPoints` table is **the executable form of the sidebar/route contract** — one row per route with expected HTTP status, expected `data-testid` substring, and redirect target where applicable. `TestMountPoints` iterates the table and asserts each row. Adding a new top-level route or sub-route requires:
1. A row in `templates.Menu` (`web/templates/menu.go`).
2. Route registration in `console/server/server.go`.
3. A `data-testid="page-<slug>"` on the page's outermost wrapper.
4. A new row in `mountPoints`.

Related canonical-editor enforcement tests follow the same shape but assert a *component*, not just a status code — e.g. `TestAssetCanonicalEditorMountedOnEverySubtypeRoute` walks every `/assets/<subtype>` route and asserts they all mount the same `AssetEditor` component (INV-U2/R1 enforcement). New canonical editors get their own such test per INV-P2 ("mount-point tests").

## Render-assertion style

Templ component tests render to a string and assert on substrings/attributes rather than parsing the DOM:

```go
html := renderToString(t, DetailShell("assets-computers", "dev-1",
    templ.Attributes{"data-testid": "page-test"}, hero, tabs))
if !strings.Contains(html, `data-testid="page-test"`) { t.Fatal(...) }
```

(`web/templates/detail_shell_test.go` is the reference example.) This keeps template tests fast and focused on the HTML contract that JS and other tests depend on (`data-testid`, `data-tab`/`data-panel`, `data-pluris-*` attributes) rather than full-page snapshot comparison.

## Headless e2e pattern

For end-to-end verification that a real binary, real routing, and real JS-driven flows work together, sessions in this repo have used a **scratch-dir + CSRF setup flow** pattern, run outside the Go test suite (kept in the agent's scratchpad, not committed to the repo):

1. Build the console binary (or `go run`) pointed at a scratch SQLite path in a scratch working directory, on a free port.
2. Drive HTTP directly (Python `requests`/`httpx` or a bash+curl script): hit `/setup` first to read the CSRF token out of the form, POST the setup form to create the admin + tenant, then log in the same way for every subsequent authenticated request — the CSRF token must be threaded through every POST (`X-CSRF-Token` header or `_csrf` form field).
3. Assert on response status codes, redirect targets, and expected `data-testid`/`data-*` substrings in the HTML — same assertions style as the Go render tests, just against a live server.
4. Tear down (kill the process); the scratch dir/db are never committed.

This pattern verified, for example, the full RBAC v2 rollout (`docs/history/plans/2026-07-09-rbac-v2.md` task 9): setup → login → create a parented role → verify inheritance badges → rename → assign to a group → create a user → verify group-role visibility on the user's Roles tab — 32/32 assertions, zero 4xx/5xx. Write these as throwaway scripts per verification pass, not as a permanent addition to the repo.

## Suite-green definition

"Done" for any task means, from the repo root:

```bash
go build -buildvcs=false ./...
go test -buildvcs=false -count=1 ./...
gofmt -l .
```

`go build` and `go test` must both exit clean (`go test` fully green, not just non-crashing). `gofmt -l .` must print nothing (excluding generated `_templ.go` files, which templ owns the formatting of). Never leave the tree red — if a task can't reach green, say so explicitly rather than declaring completion.
