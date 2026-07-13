# Pluris Management Platform — Research notes

> **Stripped 2026-05-05.** This file used to be the full landscape analysis (~430 lines). Most of it is now stale: architecture, IA, enforcement, policy model, and phase plans are owned by the canonical docs below. What remains is the short list of upstream tools we cite by name and the tiny piece of competitive positioning that hasn't been re-stated elsewhere. Full prior version is in git history.
>
> **Canonical sources (read these instead for current decisions):**
> - `docs/Pluris UX structure plan.md` — UX/IA spec.
> - `docs/UX_INVARIANTS.md` — formal IA contract.
> - `docs/endpoint-management/architecture/decisions.md` — ADR-001…006.
> - `docs/GROUP_POLICY_COMPATIBILITY.md` — GP catalog & translation tiers.

---

## Upstream tools we depend on or cite

| Tool | License | What we use it for | Doc reference |
|---|---|---|---|
| **OpenUEM** (Apache 2.0) | https://github.com/open-uem | `openuem-nats` as Go dependency; `openuem-agent` as the fork base for `pluris-agent`. | ADR-002 |
| **Kanidm** (MPL 2.0) | https://github.com/kanidm/kanidm | Identity provider (LDAP + OIDC + WebAuthn). | ADR-001 |
| **osquery** (Apache 2.0) | https://osquery.io | Device-state querying / compliance, deployed by `pluris-agent`. | ADR-003 |
| **Wazuh** (GPL) | https://wazuh.com | Optional security monitoring sidecar. | ADR-003 |
| **nmap** (custom OSS) | https://nmap.org | Network discovery for the Discovery service. | — |
| **NATS** (Apache 2.0) | https://nats.io | Messaging (JetStream + mTLS) between server and agents. | ADR-002 |
| **Microsoft GP Reference Spreadsheet** (free download) | https://www.microsoft.com/en-us/download/details.aspx?id=108395 | Seed data for the policy catalog (descriptions, supported-on, registry paths). | UX plan U2 |
| **GLPI** (GPL) | https://glpi-project.org | Reference / inspiration for asset taxonomy and CMDB-style fields (no code reuse). | ADR-005 |

---

## Short competitive note

| vs | Our differentiation |
|---|---|
| **Microsoft Intune** | Self-hosted, no per-device licensing; Linux-first; full AD-replacement bundled (Kanidm); GP compatibility. |
| **OpenUEM** | Built-in identity (Kanidm); per-user + per-asset policy with inheritance; GP catalog + Policy Module enforcement; asset-management platform; discovery. |
| **FleetDM** | Full management (write), not just visibility; identity included; policy enforcement, not just queries. |
| **Ansible / AWX** | Purpose-built endpoint management UI; per-user + per-asset policy model with inheritance; identity included; asset model; module ecosystem narrower but versioned and per-policy. |
| **GLPI** | Inherits its asset-management strengths; adds GP-compatible policy enforcement, per-user policy, integrated identity, real-time agent. |

---

## Commercial model (unchanged)

Open-source server (everything in this repo). Commercial offering: managed deployment, priority support, custom Policy Modules / compliance packs (CIS, HIPAA, SOC2), training, consulting, multi-tenant hosted SaaS.

Pricing axis: per-organization flat fee + tiered by managed-endpoint count. **Not** per-device licensing — that's a deliberate edge over Intune.
