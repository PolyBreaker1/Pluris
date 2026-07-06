# Pluris

Open-source **Microsoft Intune + Active Directory alternative** with a Windows Group Policy compatibility layer. Linux-first unified endpoint management with CMDB-style asset tracking.

**Why:** European organizations that want to move endpoints off Windows have no serious open-source replacement for the AD + Group Policy + Intune management stack — which makes the desktop migration itself impossible in practice. Pluris is built to close that gap: one self-hosted console for identities, devices, policies and software, with familiar concepts (OUs, groups, policies, enrollment) so Windows admins feel at home. Built by a Windows sysadmin who wanted the tool to exist, developed in the open under AGPLv3 with heavy use of AI-assisted engineering (Claude Code).

> ⚠️ **Project status: pre-beta.** The management console, data model and policy catalog are real and tested; the endpoint agent and policy enforcement are not built yet. Follow along or contribute — do not run this in production.

> **Key docs**: `docs/Pluris UX structure plan.md`, `docs/UX_INVARIANTS.md`, `docs/ARCHITECTURE_DECISIONS.md`

<!-- Screenshots: docs/media/ (demo GIF + console screenshots) -->

---

## Current Status

**Working today:**
- ✅ Authentication, sessions, CSRF protection, first-run setup flow
- ✅ Multi-tenant data model (tenants, sites, groups) on SQLite (WAL, zero-config, run-once migrations)
- ✅ Asset management (Computers, Servers, Printers, Desks) with list + standardized detail pages (hero + tabs)
- ✅ User/identity management with roles groundwork
- ✅ Parameter registry with **canonical parameter paths** (`computer/hardware/ram_mb`) — one addressing scheme for filtering, policy targeting and cross-feature references
- ✅ Policy catalog with Windows Group Policy equivalents mapped per policy
- ✅ Modern list UX: instant search, quick filters, advanced filter builder, column picker
- ✅ Full test suite (`go test ./...`) covering handlers, services, DB layer and templates

**In development (current plan: `docs/superpowers/plans/`):**
- 🔄 Standardized detail pages for all entities, live Groups/Policies/Roles tabs
- 🔄 Role model (Super Admin / Admin / Technician / User + custom roles)
- 🔄 Policy assignment resolution (direct / group / site / tenant)

**Planned next:**
- ⏳ Linux endpoint agent (enrollment, inventory, policy application)
- ⏳ Group Policy compatibility layer (apply Windows-equivalent policies to Linux)
- ⏳ Software deployment, scripts, Wine application groups

---

## Quickstart

### Prerequisites

**Ubuntu/Debian:**
```bash
sudo apt update && sudo apt install -y golang-go build-essential
```

**Other:**
- Go 1.22+ 
- Make
- SQLite3 (usually pre-installed)

### Run

```bash
make doctor       # Verify Go installation
make tools        # Install templ codegen + sqlc
make dev          # Run at http://localhost:8080
```

The database (`pluris.db`) is created automatically on first run.

---

## Project Layout

```
├── cmd/
│   ├── console/          # Main server binary
│   ├── seed/             # Database seeder
│   └── gendocs/          # Documentation generator
├── catalog/
│   ├── assets/           # Asset type definitions
│   ├── params/           # Parameter registry (single source of truth)
│   ├── policies/         # Policy catalog definitions
│   ├── policymodules/    # Policy module types
│   └── configgroups/     # Configuration group types
├── console/
│   ├── handlers/         # HTTP route handlers
│   └── server/           # Echo router + middleware
├── db/
│   ├── schema/           # SQLite migrations
│   ├── queries/          # SQL queries (sqlc)
│   └── *.go              # Generated Go code
├── pkg/
│   ├── database/         # Database wrapper
│   ├── services/         # Business logic layer
│   └── extension/        # Extension framework
├── web/
│   ├── templates/        # Templ components
│   ├── static/           # CSS/JS assets
│   └── lists/            # Table field definitions
└── docs/                 # Documentation
```

---

## Database

SQLite with WAL mode for concurrent access. Schema covers:

| Entity | Tables |
|--------|--------|
| Multi-tenancy | tenants, sites, groups |
| Assets | assets, asset_links, group_memberships |
| Users | identities |
| Policies | custom_policies, policy_modules, policy_module_versions |
| Configuration | configuration_groups, configuration_group_bindings, configuration_group_assignments |
| Deployment | module_installations, module_installation_dependencies |

All queries use [sqlc](https://sqlc.dev/) for type-safe generated Go code.

### Regenerate after schema changes

```bash
sqlc generate         # Regenerate db/*.go
make build            # Rebuild server
```

---

## Development

### Commands

```bash
make dev              # Run with hot reload
make build            # Build binary to bin/
make test             # Run all tests
make gen              # Regenerate templ + sqlc
```

### Agent-assisted development

This repo is developed with AI coding agents. `AGENTS.md` (repo root) holds the strict rules any agent must follow; `docs/agent/HANDOFF.md` tracks the current work state. Human contributors: see `CONTRIBUTING.md`.

---

## Architecture Decisions

Key decisions documented in `docs/ARCHITECTURE_DECISIONS.md` (ADR-001..009):

- **ADR-001**: Build the console fresh; do not fork OpenUEM wholesale
- **ADR-004**: UX invariants & Single Source of Truth UI
- **ADR-006**: Policy Module system for enforcement
- **ADR-008**: Extension framework (modules, profiles, scripts, wine, packages)

> Note: the SQLite + sqlc storage choice (replacing the originally planned
> PostgreSQL + Ent) has no ADR yet — see PROGRESS.md "Divergence from ADRs".

---

## Documentation

| Document | Purpose |
|----------|---------|
| `docs/Pluris UX structure plan.md` | Canonical IA spec (user-authored) |
| `docs/UX_INVARIANTS.md` | Formal contract for all UI changes |
| `docs/ARCHITECTURE_DECISIONS.md` | ADR-001 through ADR-009 |
| `docs/PARAMETER-REGISTRY.md` | Adding new parameters/columns |
| `docs/DATABASE-IMPLEMENTATION.md` | Database design and queries |
| `docs/MODERN-FILTER-SYSTEM.md` | Filter UI implementation |

---

## License

AGPLv3 — See LICENSE file.
