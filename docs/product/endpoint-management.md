# Pluris Endpoint Management

**What:** The management console — identity, asset, and endpoint-policy administration for Linux fleets, built as an AD + Group Policy + Intune replacement. This is the current and only build focus of Pluris.

**Related:** [[pluris]], [[roadmap]], [[authorization]], [[endpoint-policy]], [[parameters]], [[identity-assets]], [[invariants]]

## What it is

A self-hosted web console (Go + Echo + Templ, SQLite/WAL storage) covering multi-tenant identity management, asset/CMDB tracking, and an endpoint policy catalog with Windows Group Policy equivalents mapped per policy. It replaces the "AD + GPO + Intune" trio with one console and one data model, using vocabulary Windows admins already know (tenants, sites, groups, OUs, policies, enrollment).

## Who it's for

Windows admins migrating or considering migrating a fleet to Linux — people who know AD/GP/Intune concepts but are new to Linux endpoint management. The console is deliberately L1-friendly: standardized list/detail pages, inline editing, and a Windows-GP mapping on every policy so an admin can find the Linux equivalent of a setting they already know.

## Pillar features (as built)

**Identity and asset management.** `identities` carries AD-compatible attributes (username, UPN, org fields, contact, address, Windows profile/logon-script fields, security/login state). Assets (Computers, Servers, Printers, Desks) are tracked CMDB-style with owner-identity pairing. See [[identity-assets]].

**Canonical parameter system.** Every field on every entity is addressable by one canonical path (`computer/hardware/ram_mb`, `user/identity/email`), sourced from a single registry in `catalog/params/`. This one addressing scheme drives table columns, filters, detail-page fields, and policy targeting — never duplicated ad hoc. See [[parameters]].

**Endpoint policy.** A policy catalog with Windows GP equivalents mapped per policy; configuration groups (the GPO analog); dependency groups, a WMI-filter analog that gates which policy modules apply to which assets via typed platform/requirement conditions; and policy modules, the enforcement-unit abstraction (not yet wired to a real agent). See [[endpoint-policy]].

**Pluris Policy (console authorization).** A zero-trust, GLPI-style permission system for the console itself: a `domain.action` permission registry, a None/Own/All (or yes/no) grant matrix, four built-in role templates (Super Admin/Admin/Technician/User), custom roles cloned from templates, hierarchical role inheritance (parent chain, override-diff storage), roles assignable to users and to groups, and per-request enforcement on every route and handler. See [[authorization]].

**Standardized UI system.** One list engine (search, quick filters, advanced filter builder, column picker) shared by every entity list; one `DetailShell` (hero + tabs) shared by every detail page — no bespoke per-feature layouts. See [[invariants]].

**Inline editing and field API.** Field-level edit-in-place on detail pages backed by `POST /api/users/:id/fields` and `/api/assets/:subtype/:id/fields`, validated against the parameter schema and gated by the permission registry (including a self-service allowlist for "edit your own record" scope).

**Avatar upload.** Content-sniffed image upload (`POST /api/users/:id/avatar`), served back under `/avatars`, behind auth.

**Full-page create flows.** `/users/new` renders the full standardized detail layout open for input rather than a small modal form, matching the "one component per concept" rule.

## What is not built

This is pre-beta. Explicitly missing:
- The Linux endpoint agent — no enrollment, inventory collection, or check-in exists.
- Policy enforcement on devices — the policy catalog, configuration groups, and dependency groups define *what should apply*, but nothing pushes or enforces settings on a real endpoint yet.
- ADMX parsing / bulk Group Policy import.
- External identity provider sync (AD/Kanidm/FreeIPA) — deliberately deferred; the identity schema has no sync-tracking columns yet.

Do not represent Endpoint Management as managing live devices anywhere in user-facing copy — it manages the *data model and policy definitions* for a fleet that isn't enforced yet.
