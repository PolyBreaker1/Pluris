# Task Guide for Smaller Models (opencode: Kimi K2.6, GLM 5.2, DeepSeek V4)

The owner continues work with free models in opencode when large-model quota runs out. This file says which work is SAFE for them and how to run it. Smaller models: read `AGENTS.md` first, follow it literally, and stay inside the task text — do not redesign anything.

## How to run a session (owner instructions)

1. Start opencode in the repo root — it auto-reads `AGENTS.md`.
2. Give the model ONE task per session, phrased like:
   > Read AGENTS.md, docs/agent/HANDOFF.md, and Task N in docs/superpowers/plans/2026-07-05-standardized-detail-pages.md. Do only Task N, exactly as written. When done, run `go build -buildvcs=false ./...` and `go test -buildvcs=false -count=1 ./...` and show me the output. Then update the status table in docs/agent/HANDOFF.md.
3. **Accept the result only if the full test suite is green.** If the model cannot get it green in a few attempts, tell it to revert its changes and record what it tried in HANDOFF.md under a "Failed attempt" note.
4. New session (fresh context) per task — do not let one chat run multiple tasks.

## Task suitability

| Work | Fit for small model | Why |
|---|---|---|
| ~~Task 7~~ | DONE 2026-07-06 | Completed; its code is the reference pattern for Task 8. |
| ~~Task 8~~ | DONE 2026-07-06 | |
| ~~Task 9~~ | DONE 2026-07-06 | Owner manual browser check still pending. |
| **Task 11** (roles tab UI) | OK with care | Depends on Task 10 being done first. |
| **Task 15** (policy detail page) | OK with care | Mostly template work on DetailShell; deleting the popup requires careful grep for `pdd-` leftovers. |
| **Task 10** (role rename + RBAC enforcement) | **AVOID** | Cross-cutting rename touching auth/RBAC — a missed reference silently weakens security. Needs a strong model. |
| **Task 12** (assignment resolution) | **AVOID** | Multi-source policy resolution with dedupe logic; subtle correctness. |
| **Task 13** (add-from-catalog flow) | RISKY | Multi-step flow, find-or-create semantics; attempt only after 12 is done and only if desperate. |
| **Task 14** (configuration tab) | OK after 12 | Rendering rows from an existing service. |
| **Task 16** (end-to-end verification) | GOOD | Checklist execution, reporting only. |
| README/docs polish, CONTRIBUTING, comments | **GOOD** | Zero code risk. |
| Anything touching `pkg/auth/`, migrations, or `pkg/database/database.go` | **AVOID** | Security and data-loss blast radius. |

## Hard limits for small models (repeat of AGENTS.md, enforce strictly)

- Never touch root `pluris.db*`. Never git commit/push. Never edit `*_templ.go` by hand.
- `make gen` after `.templ` edits; `sqlc generate` after `.sql` edits; ASCII-only SQL comments.
- One task per session. Full test suite green before claiming done.
- If a test fails and the fix is not obvious from the failure message, STOP and report instead of thrashing — a wrong "fix" costs more than an honest stop.
