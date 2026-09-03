# Development Workflow

**What:** how work moves from idea to shipped code in this repo — spec → plan → subagent execution, where those documents live, the no-commit rule, and what's safe to hand to a small model.
**Related:** [[setup]] [[testing]] [[handoff]] [[invariants]]

## The process: spec → plan → subagent execution

New features go through three stages, using the `superpowers` skill workflow:

1. **Brainstorm + spec.** Explore intent, requirements, and design before implementation. The output is a design spec — `docs/history/specs/YYYY-MM-DD-<feature>-design.md`. Architecture-shaped work (new permission semantics, new schema entities, anything touching `pkg/auth`/`pkg/authz`/`pkg/database`) requires a strong model at this stage; do not let a small model architect a spec.
2. **Plan.** Once the spec is approved, break it into an ordered, numbered task list — `docs/history/plans/YYYY-MM-DD-<feature>.md`. Each task should be independently completable and independently testable.
3. **Execute.** Work the plan one task at a time, in order, exactly as written — no improvised scope. Each task is handed to an **implementer subagent**, then reviewed by **two independent review subagents** before being marked done (this applies even in unattended/loop-mode sessions — see the "Pluris: subagent-driven development" convention). Tests-first when the plan specifies test code and expected failures.

Both specs and plans are **historical records once done** — they live under `docs/history/` permanently; they are not deleted or moved after completion. `docs/development/handoff.md` is rewritten to reflect current state, but the spec/plan files that got there stay put as the audit trail.

## The no-commit rule

Agents never run `git commit`, `git push`, or any history-mutating git command in this repo. The owner reviews and commits manually. `git status` / `git diff` for reading the current state are fine; anything that changes repo history or remote state is not an agent's call to make, ever — this applies regardless of how confident the agent is that a change is correct.

## Snapshot-based review diffs

Because agents don't commit, code review during a plan's execution works off working-tree snapshots (before/after diffs of the actual files, not git commits) rather than PR diffs. A review subagent is handed the task description plus a diff of what changed and reports findings against that — not against a git log.

## Small-model sessions

The owner also runs tasks through smaller/free models (opencode with Kimi, GLM, DeepSeek, etc.) when a strong-model budget isn't warranted for the work. Full detail was condensed from a now-deleted document (full text in git history) into the summary below:

**Safe for a small model:**
- Manual QA support — diagnosing a pasted error/stack trace, one bug per session.
- Small, well-scoped UI polish: labels, spacing, empty-state copy, cosmetic template changes that don't touch field names, values, or handler logic.
- New detail-page fields/params, following `catalog/params/` conventions exactly (copy an existing param's pattern, don't invent a new shape).
- README/docs wiring for assets the owner drops in (e.g. screenshots into `docs/media/`).

**Off-limits for a small model — strong model only:**
- `pkg/auth/`, `pkg/authz/`, `catalog/permissions/` — permission semantics; a subtle grant-ranking or scope bug is a security hole, not a cosmetic bug.
- Role inheritance / group-role resolution (`pkg/authz/service.go`'s `ResolveRoleMatrix`/`SaveRoleOverrides`/`SetRoleParent`, `console/handlers/group_roles.go`, the `EffectiveGrants` direct∪group union) — cycle/depth-cap guards, diff-only override storage, and cache-recompute triggers are all subtle correctness-and-security surfaces.
- `pkg/database/database.go` and migrations — data-loss blast radius.
- Any new top-level feature architecture — plan it with a strong model first, then the resulting plan's individual tasks may be small-model-eligible per task.

**Protocol for a small-model session:** one task per session, fresh context each time (no multi-task chats). The session prompt names the exact task in the plan file, requires `go build -buildvcs=false ./...` and `go test -buildvcs=false -count=1 ./...` output before claiming done, and requires updating [[handoff]]'s status table. If a test fails and the fix isn't obvious from the failure message, the model should stop and report rather than thrash — a wrong "fix" costs more than an honest stop. A result is only accepted if the full suite is green; if it can't get there in a few attempts, the correct move is to revert and log a "Failed attempt" note in `HANDOFF.md`, not to force a merge.

## Documentation rules

This doc tree is **strict**. New documentation must fit the existing structure:

- `docs/endpoint-management/` — concepts, architecture, UI reference for the shipped product.
- `docs/development/` — this directory: setup, testing, workflow, handoff.
- `docs/history/{specs,plans}/` — the permanent record of what was designed and executed, dated.
- `docs/product/` — the three-product family pitch (Endpoint Management / ITSM / OS).
- `docs/funding/` — gitignored, owner-only.

**No scratch breadcrumb files.** Temporary working notes, in-progress checklists, and session scratch state belong in `.superpowers/sdd/` (disposable, not part of the doc tree) — never a stray `.md` at the repo root or loose in `docs/`. If a piece of information doesn't fit an existing doc's purpose, either extend the right doc or raise it with the owner before creating a new file; don't default to "just add a new markdown file."

## Resuming work

See [[handoff]] for the "how to resume" sequence (`AGENTS.md` → `docs/INDEX.md` → `docs/development/handoff.md`).
