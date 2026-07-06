# Work Handoff — current state and next steps

Last updated: 2026-07-06 (Claude Fable 5 session). Any agent continuing work: read `AGENTS.md` (repo root) first, then this file, then the plan file for your task.

**Active plan:** `docs/superpowers/plans/2026-07-05-standardized-detail-pages.md` (16 tasks).
**Spec:** `docs/superpowers/specs/2026-07-05-standardized-detail-pages-design.md`.

## Current state

| Task | Status |
|---|---|
| 1 migration tracker | DONE |
| 2 migration 003 + sqlc | DONE |
| 3 Canonical Parameter Paths | DONE |
| 4 new params + plumbing | DONE |
| 5 seeder | DONE |
| 6 DetailShell + detail.js | DONE |
| 7 Computer detail 8 tabs | DONE (2026-07-06, suite green incl. TestAssetDetailTabsRendered) |
| 8 User detail 4 tabs | DONE (2026-07-06, suite green incl. TestUserDetailTabsRendered). Reference pattern for future detail pages: `web/templates/asset_detail_helpers.go` + `user_detail_helpers.go` |
| 9 Groups tabs live | DONE (2026-07-06, suite green incl. TestGroupMembershipRoundTrip + TestUserGroupMembershipHandlers). GroupService in `pkg/services/groups.go`, handlers in `console/handlers/groups.go`, shared tab components in `web/templates/detail_groups.templ`. Owner manual browser check still pending |
| 10 Role vocabulary + RoleService | DONE (2026-07-06). 4-role vocabulary, RBAC matrix with technician, RoleService (`pkg/services/roles.go`) with cache recompute, setup wires EnsureBuiltins + super_admin assignment. TestRoleServiceLifecycle green. Remaining `user_self_service` grep hits are only the applied 002 migration (append-only, remapped by 003) |
| 11 Roles tab UI | DONE (2026-07-06). Assign/remove handlers (`console/handlers/roles.go`) with admin-only + no-self-modification guards; TestUserRoleAssignAuthorization green |
| 12 Assignment resolution | DONE (2026-07-06). AssignmentService (`pkg/services/assignments.go`) resolves direct/group/site/tenant scopes with binding dedupe; Applied Policies tabs live on both detail pages; TestResolveForTarget green |
| 13 Add-from-catalog flow | in progress this session — CHECK STATE: if `web/templates/policy_picker.templ` + routes `/assets/:subtype/:id/policies/add` exist and suite is green it may be done; otherwise finish per plan Task 13 |
| 14-16 | not started, do in order |

## Next steps

Tasks 8-16 are fully specified in the plan file — follow it verbatim, one task at a time, and keep the table above updated. See `docs/agent/SMALL-MODEL-TASKS.md` for which tasks are safe for smaller models.

## Publish-readiness checklist (parallel goal, no code needed)

Goal: repo clean and fund-worthy for publishing (NOT feature-complete). See `docs/funding/` (gitignored, owner-only).
- [x] README.md refreshed (pitch, honest pre-beta status, EU-sovereignty mission, AGPL, screenshots placeholder)
- [x] CONTRIBUTING.md created
- [x] LICENSE (AGPLv3) present
- [x] .gitignore covers `*.db`, generated templ files, `/docs/funding/`
- [x] Private data scrubbed: Tailscale hostname replaced with placeholder in all docs + scripts (2026-07-06); owner email only in gitignored docs/funding/
- [ ] Owner records demo GIF and takes screenshots into `docs/media/` (owner task, see `docs/funding/03-tasks-peter.md`)
- [ ] Owner drafts still pending from Claude: Anthropic credits text, NLnet application, launch posts (see `docs/funding/04-tasks-claude.md`)
