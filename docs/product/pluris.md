# Pluris

**What:** The Pluris family — an open-source, EU-sovereign alternative to the Microsoft AD + Group Policy + Intune stack, split into three interconnected sub-products.

**Related:** [[endpoint-management]], [[itsm]], [[os]], [[roadmap]]

## Mission

European organizations that want to move endpoints off Windows have no serious open-source replacement for the AD + Group Policy + Intune management stack, which makes the desktop migration itself impractical. Pluris closes that gap: one self-hosted console for identities, devices, policies and software, built with concepts familiar to Windows admins (OUs, groups, policies, enrollment) so the learning curve is shallow. It is built by a Windows sysadmin, developed in the open under AGPLv3, with heavy use of AI-assisted engineering.

Digital sovereignty is the driving principle: self-hosted, no mandatory cloud dependency, source available for audit, and license terms (AGPLv3) that keep it that way.

## The three sub-products

**Pluris Endpoint Management** is the console that exists today: multi-tenant identity and asset management, a canonical parameter registry, an endpoint policy catalog with Windows Group Policy mappings, dependency groups (a WMI-filter analog), and Pluris Policy — a zero-trust, GLPI-style permission system governing the console itself. This is the current and only build focus. See [[endpoint-management]].

**Pluris ITSM** is planned: tickets and incidents, a self-service portal (end users editing their own info, picking firewall settings from a catalog, filing tickets), and software assignment workflows. Not started — requirements are gathered, and the Pluris Policy permission registry already reserves room for an `itsm` domain so this can slot in without a rework. See [[itsm]].

**Pluris OS** is an early idea under consideration, not a commitment: a managed Linux OS image that ships with the Pluris agent pre-enrolled, aimed at reducing endpoint onboarding to "boot the image." See [[os]].

## How they interconnect

All three share one console, one identity model, and one asset model. Endpoint Management owns the canonical data: identities, assets, and the Pluris Policy permission registry that governs who can do what. ITSM, when built, consumes that inventory and identity data directly rather than duplicating it, and reuses the same permission registry pattern (new `itsm` domain, same grants engine). Pluris OS, if it proceeds, would simply ship the Endpoint Management agent preinstalled and pre-enrolled — it does not introduce a separate management plane.

## Current focus

Endpoint Management only. ITSM and OS are not being built yet; see [[roadmap]] for what ships next.
