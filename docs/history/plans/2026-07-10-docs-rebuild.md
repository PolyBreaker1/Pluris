# Docs Rebuild Plan (owner-mandated, 2026-07-10)

Owner mandate: Pluris = 3 interconnected sub-products (Endpoint Management = current work; ITSM = planned; OS = consideration). Rebuild ALL docs top-to-bottom into a strict, Obsidian-brain-like hierarchy: INDEX.md map-of-content, wiki-links `[[...]]`, one concept per file, product-first folders. DELETE everything that does not fit (esp. small-agent breadcrumbs, one-off session summaries). Project is backed up; owner authorized path changes and deletions. No git operations by agents.

## Target tree (final)
```
README.md AGENTS.md CONTRIBUTING.md            (root, rewritten)
docs/INDEX.md                                  (the brain)
docs/product/{pluris,endpoint-management,itsm,os,roadmap}.md
docs/endpoint-management/architecture/{overview,decisions,data-model}.md
docs/endpoint-management/concepts/{parameters,authorization,endpoint-policy,identity-assets}.md
docs/endpoint-management/ui/{invariants,layout-system}.md
docs/endpoint-management/windows-admins/{README,concepts,cheatsheet,glossary}.md   (moved)
docs/endpoint-management/examples/policy-modules/...                              (moved)
docs/development/{setup,workflow,testing,handoff}.md
docs/history/specs/*  docs/history/plans/*     (moved from docs/superpowers/)
```

## Doc conventions (every new file)
Title H1; one-line "**What:**" summary; "**Related:**" line of `[[wiki-links]]` (Obsidian-compatible, target = file base name); sections H2; concise, current-state (no changelog prose); code/file pointers as `path:line` or `path` references. INDEX.md = nested bullet tree of every doc with one-line hooks + a "Relations" section describing the cross-links.

## Source → target mapping (absorb = rewrite essence, then delete source)
- ARCHITECTURE_DECISIONS + TECHNOLOGY_DECISIONS → architecture/overview + decisions
- DATABASE-IMPLEMENTATION + DATABASE-INTEGRATION-COMPLETE → architecture/data-model
- PARAMETER-REGISTRY + ADDING-PARAMETERS-EXAMPLE → concepts/parameters
- Specs 2026-07-08-pluris-policy-authz + 2026-07-09-rbac-v2 (+ shipped code) → concepts/authorization
- GROUP_POLICY_COMPATIBILITY + dependency-groups spec + policymodules docs → concepts/endpoint-policy
- Pluris UX structure plan (owner-authored IA! preserve intent) + MANAGEMENT_INTERFACE → concepts/identity-assets + ui docs
- UX_INVARIANTS + MODERN-FILTER-SYSTEM + COLUMN-PICKER-SEARCH + BRANDING_GUIDE → ui/invariants + ui/layout-system
- QUICKSTART + DEV-HOSTING → development/setup
- AGENTS.md + RULES.md + docs/agent/SMALL-MODEL-TASKS + subagent conventions → AGENTS.md (root, tight) + development/workflow
- docs/agent/HANDOFF.md → development/handoff (rewritten: current state only)
- DEVELOPMENT_PLAN + PROGRESS → product/roadmap
- README.md → rewritten (3-product family framing)
- for-windows-admins/* → moved (light touch)
- superpowers specs/plans → moved to docs/history/ (unchanged content)

## DELETE list (after absorption)
docs/archive/* · docs/agent/Small agent output/* · AUDIT-REPORT · LAYOUT-IMPROVEMENTS · TABLE-NATURAL-FLOW · COLUMN-PICKER-SEARCH · MODERN-FILTER-SYSTEM · DATABASE-INTEGRATION-COMPLETE · MANAGEMENT_INTERFACE · BRANDING_GUIDE · QUICKSTART · DEVELOPMENT_PLAN · GROUP_POLICY_COMPATIBILITY · PARAMETER-REGISTRY · ADDING-PARAMETERS-EXAMPLE · DATABASE-IMPLEMENTATION · ARCHITECTURE_DECISIONS · TECHNOLOGY_DECISIONS · UX_INVARIANTS · "Pluris UX structure plan.md" · docs/agent/ (whole dir after moves) · root RULES.md, PROGRESS.md, DEV-HOSTING.md · .superpowers/sdd/task-*-brief.md + task-*-report.md + *-report.md (keep progress.md) · docs/superpowers/ (dir removed after move; future plans/specs go to docs/history/ — note in AGENTS.md)
KEEP UNTOUCHED: docs/funding/ (owner-private, gitignored), docs/media/ if present.

## Execution tasks
1. Scaffold dirs + move-only operations (windows-admins, examples, history) + delete .superpowers scratch. 
2. Writer A: product/* + README.md (reads old README/PROGRESS/DEVELOPMENT_PLAN/funding-free vision).
3. Writer B: architecture/* + concepts/parameters + concepts/identity-assets.
4. Writer C: concepts/authorization + concepts/endpoint-policy (from specs + code).
5. Writer D: ui/* + development/* + AGENTS.md rewrite + CONTRIBUTING touch.
6. INDEX.md (controller or writer E) + deletions per list + link-check pass (grep [[...]] targets resolve to existing basenames).
7. Final review subagent: structure coherence, no orphan links, no leftover deleted-doc references in code comments/README, suite still green (docs-only but AGENTS/HANDOFF paths are referenced in tests? grep first).

Code references to doc paths must be updated (grep "docs/" in *.go, Makefile, README).
