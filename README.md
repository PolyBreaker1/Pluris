# Pluris

Open-source, EU-sovereign alternative to the Microsoft **Active Directory + Group Policy + Intune** stack — built for organizations moving endpoints off Windows.

Pluris is a family of three interconnected sub-products:

- **Pluris Endpoint Management** — the console (this repo's current focus): identity/asset management, an endpoint policy catalog with Windows GP mappings, and zero-trust console authorization. Built and tested.
- **Pluris ITSM** *(planned)* — tickets/incidents, a self-service portal, software assignment. Not started; requirements gathered.
- **Pluris OS** *(under consideration)* — a managed Linux image with the agent pre-enrolled. No commitment.

See `docs/product/pluris.md` for the full family pitch.

> **Project status: pre-beta.** The management console, data model, and policy catalog are real, built, and tested. The endpoint agent and policy enforcement on real devices are **not built yet** — do not run this in production, and do not expect it to manage a live fleet today.

Built by a Windows sysadmin who wanted this tool to exist, developed in the open under AGPLv3 with heavy use of AI-assisted engineering (Claude Code).

---

## What works today (Endpoint Management)

- Authentication, sessions, CSRF protection, first-run setup wizard
- Multi-tenant data model (tenants, sites, groups) on SQLite (WAL, zero-config, auto-migrated)
- Asset management (Computers, Servers, Printers, Desks) with standardized list + detail pages
- Identity/user management with AD-familiar fields and a role model (Super Admin/Admin/Technician/User) including hierarchical role inheritance and group-assigned roles
- Canonical parameter registry — one addressing scheme (`computer/hardware/ram_mb`) for filtering, policy targeting, and cross-feature references
- Endpoint policy catalog with Windows Group Policy equivalents mapped per policy, plus dependency groups (a WMI-filter analog)
- Pluris Policy: zero-trust, GLPI-style console permission system with a full role/grant matrix UI
- Inline field editing, avatar upload, full-page create flows
- Full Go test suite covering handlers, services, the DB layer, and templates

**Not built yet:** the Linux endpoint agent, and any policy enforcement on a real device.

---

## Getting started

### Prerequisites

**Ubuntu/Debian:**
```bash
sudo apt update && sudo apt install -y golang-go build-essential
```

**Other:** Go 1.22+, Make, SQLite3 (usually pre-installed).

### Run

```bash
make doctor       # verify Go installation
make tools        # install templ codegen
make dev          # run at http://localhost:8080
```

The database (`pluris.db`) is created automatically on first run.

### Other commands

```bash
make build        # build console binary into bin/
make test         # run all tests
make gen          # regenerate templ codegen
make vet          # go vet
make clean        # remove generated code + binaries
```

---

## Project layout

```
├── cmd/console/          # Main server binary
├── catalog/              # Asset types, params, policy catalog, policy modules, dependency groups
├── console/               # HTTP handlers, Echo router + middleware
├── db/                    # SQLite schema migrations + sqlc-generated queries
├── pkg/                   # Database, services, auth/authz, extension framework
├── web/                   # Templ templates, static assets, list registry
└── docs/                  # Documentation
```

---

## Documentation

Start at **`docs/INDEX.md`** — the map of every doc in this repo, wiki-linked and organized by product. Key entry points:

- `docs/product/pluris.md` — the product family and mission
- `docs/product/endpoint-management.md` — this repo's product charter
- `docs/product/roadmap.md` — shipped milestones and what's next
- `AGENTS.md` — rules for any AI coding agent working in this repo

---

## License

AGPLv3 — see `LICENSE`.
