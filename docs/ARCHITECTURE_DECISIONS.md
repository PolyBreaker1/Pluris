# Pluris — Architecture Decision Records

Lightweight ADRs. One section per decision. Each ADR is appended; never rewritten in place. Status: `proposed` | `accepted` | `superseded by ADR-XXX`.

---

## ADR-001 — Build the Pluris console fresh; do not fork OpenUEM wholesale

**Date**: May 3, 2026
**Status**: accepted
**Supersedes**: the original "fork OpenUEM" plan in `docs/FORK_STRATEGY.md` (kept for historical context).

### Context

`docs/FORK_STRATEGY.md` recommended forking the entire OpenUEM stack (console, agent, worker, ent, nats, ocsp-responder) and bolting Pluris's identity + Group Policy + compliance layers on top. Estimated savings: 6–9 months.

Investigation of the actual OpenUEM source (cloned `openuem-console`, `ent`, `openuem-agent`, `nats`):

1. **`Task` ent schema is a denormalized God-table** (~150 columns, hard-coded enum of ~40 task types covering winget/msi/apt/brew/registry/netbird/scripts/users/groups). Mapping a 3000+ setting GP catalog onto this either explodes the schema or bypasses it — in either case the OpenUEM model offers no reuse for our differentiator.
2. **Assignment model is `Profile → Tag → Agent`.** No org → site → group → device → user inheritance. No per-user policy. No GP-style merge/replace/loopback semantics. Pluris must build this regardless of fork.
3. **No end-user identity model.** Schemas have console-operator `User` and device `Agent`. Per-user policy enforcement requires a new identity entity; not inherited from OpenUEM.
4. **Console is ~33K LOC Go + ~20K LOC Templ.** Forking incurs a permanent rebranding tax across hundreds of files and inherits design constraints (`Profile`/`Task` model) that pull architectural gravity *away* from our GP differentiator.

### Decision

Build the Pluris console, Ent schema, policy engine, ADMX parser, and translation map **fresh**, designed around Group Policy semantics from day one. Reuse OpenUEM components selectively at the boundary, not at the core (see ADR-002).

### Consequences

- **Lose**: ~2–3 months relative to a clean fork (we rebuild console UI shell, auth flow, OIDC wiring, multi-tenancy plumbing).
- **Gain**: schema designed for GP not retrofitted onto it; smaller owned codebase (~5K LOC start vs. 53K inherited); no rebranding tax; same tech stack philosophy preserved (Go + Echo + Templ + HTMX + Ent + PostgreSQL + NATS).
- **Risk**: tempted to under-build inventory/remote-access features that OpenUEM ships free. Mitigation: those are deferred to post-v1 and partially covered by reusing `openuem-agent` (ADR-002).

---

## ADR-002 — Selective OpenUEM reuse: import `openuem-nats` as a dependency, fork only `openuem-agent`

**Date**: May 3, 2026
**Status**: accepted
**Related**: ADR-001, ADR-003.

### Context

Following ADR-001 (don't fork the console), the question is which OpenUEM pieces are still worth pulling in.

Survey of the OpenUEM repos:

| Repo | Reusable as-is? | Verdict |
|---|---|---|
| `openuem-console` | No — schema/UX bound to its own model | Build fresh |
| `ent` (schemas) | No — wrong model for GP | Build fresh |
| `openuem-worker` | No — orchestrates the wrong things | Build fresh |
| `openuem-ocsp-responder` | Maybe — narrow scope | Reimplement (small) |
| `openuem-nats-service` | Config only | Reference, not import |
| `nats` (Go module of message structs) | **Yes** — small, stable, Apache 2.0 | **Import as Go dependency** |
| `openuem-agent` | **Almost** — runs on Win/Linux/macOS, mTLS+NATS, multi-OS package mgmt, inventory | **Fork this one repo, add Pluris policy CSEs** |

### Decision

1. **`github.com/open-uem/nats`** — import as a Go module dependency. Extend with Pluris-specific message types in our own module. No fork.
2. **`openuem-agent`** — fork as `pluris-agent`. Track upstream as a `git remote` so we can cherry-pick fixes. Add the Pluris policy-applier subsystem (see ADR-003) alongside its existing inventory/winget/apt features. Preserve LICENSE + add NOTICE crediting OpenUEM per Apache 2.0.
3. **Cert/CA infra** — reimplement clean using stdlib `crypto/x509` (~few hundred LOC). Don't fork `openuem-ocsp-responder`.

### Consequences

- We inherit a working multi-OS endpoint courier (mTLS, NATS transport, install scripts for Windows/Linux/macOS) at the cost of one fork.
- Upstream divergence in `openuem-agent` is bounded to one repo; cherry-picking remains practical.
- `openuem-nats` becomes a soft contract — if upstream breaks compat we pin or vendor.

---

## ADR-003 — Linux GP enforcement architecture: extended agent + native CSEs + PAM session hook

**Date**: May 3, 2026
**Status**: accepted (§3 partially superseded by ADR-006)
**Related**: ADR-002, supersedes the SSH+Ansible enforcement plan in `docs/FORK_STRATEGY.md` §4.3.

> **Partial supersession (2026-05-05).** The hardcoded list of native Go CSEs in §3 (`file_cse`, `pam_cse`, `sshd_cse`, `sudoers_cse`, …) is **replaced by the Policy Module system in ADR-006**. The CSEs become the initial bundled modules in the module format. ADR-003's other elements (NATS bundle distribution, drift refresh, PAM session hook, mTLS, ansible-local as a script *runtime*) stand unchanged.

### Context

A Group Policy compatibility product must enforce policies on Linux endpoints with semantics that match Windows GP:

- **Per-machine policy** applied at boot.
- **Per-user policy** applied on user logon (`pam_open_session`).
- **Drift refresh** on a timer (Windows default: 90 min ± random).
- **Offline-tolerant** for laptops behind NAT or disconnected.

The original PROGRESS.md / FORK_STRATEGY.md plan was "agentless via SSH + Ansible." Analysis shows this is structurally insufficient:

- Ansible has no logon-time hook → no per-user policy on logon.
- SSH-from-server requires reachable endpoints → fails for roaming laptops.
- Drift only corrects when the server pushes again → endpoint can drift unbounded.
- Server-held SSH keys to every endpoint = larger credential blast radius than per-device mTLS certs.

Alternatives surveyed:

| Option | Outcome |
|---|---|
| Pure SSH + Ansible push | Rejected — see above. |
| `ansible-pull` on systemd timer | Solves drift + offline, still no per-user-on-logon. |
| Samba GP client (`samba-gpupdate` + CSEs) | Rejected as runtime — pulls Samba/Python/SMB into stack, requires SYSVOL-compatible policy share, GPLv3 client side. **Kept as algorithmic reference** — port their CSE logic where useful. |
| Himmelblau (Intune-on-Linux) | Rejected — wrong direction (pulls from Intune), GPLv3, narrow scope. |
| Build `pluris-agent` from scratch | Rejected — high effort given `openuem-agent` exists. |
| **Extend `openuem-agent` with Pluris CSEs + PAM session hook** | **Accepted.** |

### Decision

**Enforcement architecture (Option F):**

1. **Forked `pluris-agent`** (per ADR-002) runs on every managed endpoint (Linux/Windows/macOS).
2. **Policy bundles** are published from server to agent over a new NATS subject (`pluris.policy.<device-id>`), versioned, idempotent. Bundles describe the resolved policy for *this device* and *currently logged-in user(s)*.
3. **Native Go CSEs (Client-Side Extensions)** in `pluris-agent`, one module per setting class. Initial Tier-1 set:
   - `file_cse` — manage files with mode/owner/content.
   - `pam_cse` — idempotent edits to `/etc/security/*.conf`, `/etc/pam.d/*`.
   - `sshd_cse` — `/etc/ssh/sshd_config.d/pluris.conf`.
   - `sudoers_cse` — fragments in `/etc/sudoers.d/`, validated with `visudo -c`.
   - `firewalld_cse`
   - `sysctl_cse`
   - `apparmor_cse`
   - `dconf_cse` and `kde_kiosk_cse` — desktop lockdown.
   - `browser_policy_cse` — Firefox/Chrome managed-policy JSON drops.
   - `script_cse` — shell / PowerShell.
   - `ansible_local_cse` — escape hatch; runs `ansible-playbook --connection=local` for the long tail. Lets us reuse Samba's CSE algorithms by transcribing them into local playbooks.
4. **Drift refresh loop**: agent re-evaluates policy every 90 min ± jitter, and on signal-from-server. Each CSE is responsible for idempotent reconciliation.
5. **PAM session hook** (`pam_pluris.so`, ~200 LOC C, ours): on `pam_open_session`, opens a local Unix socket to `pluris-agent` and signals "user `X` logged in" → triggers per-user CSE pass for that user.
6. **osquery sidecar** (deployed via the agent) for drift detection and compliance reporting. Orthogonal to enforcement.

### Tier mapping (links to `docs/GROUP_POLICY_COMPATIBILITY.md`)

- **Tier 1** (direct): served by native CSEs above.
- **Tier 2** (approximate): mix of native CSEs (e.g. `apparmor_cse` for AppLocker) and `ansible_local_cse`.
- **Tier 3** (Windows-only passthrough): handled by the agent's existing Windows-side capabilities; no Linux work.
- **Tier 4** (Linux-native additions): native CSEs only.

### Consequences

- We ship a real GP-equivalent client for Linux (machine + user + drift), not a polling Ansible script.
- Enforcement code lives in our forked agent — bounded surface area.
- PAM module is a small piece of C, but it is C; it must be tested carefully and shipped per-distro.
- **`ansible-local` escape hatch** keeps us pragmatic without making Ansible the primary path.
- Server is now responsible for **policy resolution** (computing the effective bundle for a device+user from inheritance) — this becomes the central job of `pluris-policy`.

---

## ADR-004 — UX invariants & Single Source of Truth UI

**Date**: May 5, 2026
**Status**: accepted
**Related**: ADR-001. Codifies the constraints implied by `docs/Pluris UX structure plan.md`.

### Context

The user authored `docs/Pluris UX structure plan.md` defining the management console's information architecture: left-sidebar menu items (Dashboard, Users, Computers, Policy, Profiles, Scripts, Wine, Server Administration, User/Admin Preferences), tabbed Computer detail view, scope partitioning (Computer vs User Configuration like Windows GP), Qualys-style hierarchical search, and — most critically — the rule that **every concept has exactly one canonical editor reachable from every entry point**, with filtering rather than forking when contexts need different presentations.

This rule is non-trivial to honor. Naive implementation creates "the same thing" in two slightly different forms (e.g. a Policy Group rendered one way from the Policy menu and a "simplified Policy view" rendered from the Computer detail). Over time these diverge, drift in capability, and produce the "this dialog has a feature the other one doesn't" bug class.

### Decision

1. `docs/Pluris UX structure plan.md` is **canonical UX/IA source**. `docs/UX_INVARIANTS.md` is its formal extraction in checkable form. Together they outrank every other doc; conflicting docs are stale.
2. **Single Source of Truth UI** is enforced by:
   - One canonical editor component per concept, listed in `UX_INVARIANTS.md` §VII Concept Registry.
   - Multiple entry points mount the **same component instance** with at most a context filter / scope prop; no sibling "view-mode" or "lite" variants.
   - Mount-point tests assert each entry point routes to the canonical component.
3. **Hierarchy parity**: one shared model for `Tenant → Site → Group → (Device | Identity)`, consumed by every navigator, picker, search, and assignment selector.
4. **Scope is mandatory metadata**: Policy Group, Profile, Script, and PolicySetting carry `scope: machine | user | both`. Editors visually partition the two sections like Windows GP.
5. **Slow down before code**: new top-level menu items, entities, or editors require an IA contract row in `UX_INVARIANTS.md` §VII (entity, scope, canonical editor, all entry points + filters, AD/GP equivalent, role gates) **before** any handler or template is written.
6. **Operating rules** for AI assistants are in `.windsurf/rules/pluris-core.md` and apply on every turn.

### Consequences

- **Higher up-front design cost.** Adding a feature now starts with editing the Concept Registry, not creating files. This is intentional — the user explicitly asked us to slow down.
- **Lower long-term maintenance cost.** No drift between "the same thing in two places", no parallel implementations to keep in sync.
- **Refactor pressure.** Existing scaffolded code (`pluris/console/handlers/handlers.go`, `pluris/console/server/server.go`) is paused as draft pending IA-contract completion. Ent schemas (`pluris/db/schema/`) are aligned with INV-H1 but incomplete (missing Profile, Script, Wine, Role, DashboardTile, RoleAssignment); they remain as a draft starting point.
- **Test obligation.** Each canonical editor needs a mount-point test (table-driven over entry points). This is a phase-0 deliverable for the testing harness, not an afterthought.

### Non-decisions (deferred)

- Concrete Templ component layout for `editors/PolicyGroupEditor` etc. — to be filled in during Phase 1+ once entities are confirmed.
- Permission/role matrix details — captured per-concept in §VII as concepts land.
- Component library convention (e.g. how to express "filter prop") — pin during Phase 1 when first canonical editor is implemented.

---

## ADR-005 — Asset hierarchy (Asset replaces Computer at the top of the hardware hierarchy)

**Date**: 2026-05-05
**Status**: accepted
**Related**: ADR-001, ADR-004; codifies update U1 in `docs/Pluris UX structure plan.md`.

### Context

The original plan modelled "Computer" as the only managed-hardware entity. The user expanded scope: Pluris is also an asset-management platform, and the estate includes more than computers. Servers, printers, and "desks" (docks + monitors, with guest-connection profiles) are first-class managed entities. Modeling each as a parallel top-level concept would violate the hierarchy-parity rule (R3): every navigator and picker would need to enumerate sibling entities. Instead, we introduce a single root **Asset** with discriminated subtypes.

GLPI's asset model was reviewed for inspiration. Useful ideas adopted: subtype taxonomy (Computer, Server, Printer, Display, Peripheral, NetworkEquipment, Phone, Software-as-asset, Cartridge, Consumable), asset-to-asset relationships (peripheral ↔ host) via a single linking entity, location/site-aware fields, contract/warranty tracking, financial fields. Not adopted: GLPI's "global vs unitary" management toggle, GLPI's separate tables per subtype (we use single-table with discriminator + JSON subtype payload to keep the hierarchy join-free).

### Decision

1. The Ent schema gains an **Asset** entity. `subtype` is an enum: `computer | server | printer | desk | … (extensible)`. Subtype-specific fields live in a typed JSON payload column with per-subtype Go structs and JSON Schema validation. The previous standalone `Device` schema is **renamed/migrated to** `Asset` with `subtype=computer` for existing rows.
2. The shared hierarchy becomes **Tenant → Site → Group → (Asset | Identity)**, not "→ Device | Identity". All navigators, pickers, search, dashboard data sources, and policy assignments target Asset (with optional `subtype=` filter) rather than Computer.
3. Sidebar item "Computers" is renamed **Assets**. The Assets page tabs route by subtype (Computers / Servers / Printers / Desks). All four mount the same `editors/AssetEditor` with `subtype=…` filter.
4. Asset-to-asset relationships are modeled by a single `AssetLink` entity with typed `relation` enum (`peripheral_of`, `docked_to`, `printed_via`, `hosts_vm`, `etc`). One model, many relation types — no per-relation table.
5. Asset management platform features (lifecycle state, location, ownership, vendor, purchase date, warranty, depreciation, contracts) are added to Asset as optional fields. They are not gated behind a "CMDB module" flag — they are part of the base entity, surfaced or hidden by editor filter.
6. **Desks** carry a `guest_profile_id` reference to a Profile. When an end user logs into an asset that is `docked_to` a Desk with a `guest_profile_id`, the Profile-resolver merges that profile into the user's effective policy. (Detailed semantics deferred — `desk` UX is a TBD design session.)

### Consequences

- One canonical editor for all asset subtypes. Subtype tabs in the Assets page filter the same component.
- Existing `Device` schema and ADR-001 references to "Device" are **renamed to Asset** in Phase 0b. No production data exists yet, so this is a free rename.
- The mTLS device certificate's subject CN now binds to Asset.uuid rather than Device.uuid (same field, renamed entity).
- Adding new subtypes (Phone, NetworkSwitch, Display-as-distinct-from-Desk) is additive: new enum value, new JSON payload struct, new tab in the Assets page, no new editor component.

---

## ADR-006 — Policy Module system (supersedes ADR-003 native-CSE list)

**Date**: 2026-05-05
**Status**: accepted
**Supersedes (in part)**: ADR-003 §3 — the hardcoded list of native Go CSEs (`file_cse`, `pam_cse`, `sshd_cse`, …) is replaced by a uniform **module** mechanism. ADR-003's other elements (NATS bundle distribution, drift refresh, PAM session hook, mTLS) stand.
**Related**: codifies update U3 in `docs/Pluris UX structure plan.md`.

### Context

Hardcoding Linux equivalents of Windows policies inside `pluris-agent` (one Go CSE per setting class) makes the translation surface a closed, in-tree set. That is incompatible with several requirements the user articulated:

1. **No hardcoded translations** — translations must be data, not Go code.
2. **User-editable in the web UI** — admins must be able to read and modify the enforcement script for any policy without forking the agent or rebuilding it.
3. **Community contributions** — the bar to perfecting Linux policy coverage is community PRs; closed-coupled CSEs raise that bar prohibitively.
4. **Many-to-many policy↔script mapping** — one Bash script can satisfy several Windows policies (e.g. one `sshd_config` snippet covering several SSH-related GPOs); one policy can have several candidate enforcement scripts (admin chooses).
5. **Side-by-side UI** — admin sees the Windows policy text alongside the candidate Linux modules, with in-place script editing.

### Decision

A **Policy Module** is the unit of enforcement. It is a versioned package, not Go code in the agent.

#### Manifest (`module.yaml`)

```yaml
id: pluris.sshd.password-auth-disable
version: 1.2.0
title: Disable SSH password authentication
description: |
  Drops a snippet into /etc/ssh/sshd_config.d/ disabling password auth and reloads sshd.
target_os: [linux]                      # linux | windows | macos | any
scope: machine                          # machine | user | both
runtime: bash                           # bash | python | go | ansible-task
satisfies:                              # policy URNs this module implements (many)
  - "Computer/WindowsComponents/RemoteAccess/SSH/PasswordAuthDisable"
  - "pluris/ssh/password-auth"
parameters:                             # JSON Schema for inputs
  type: object
  properties:
    allow_root: { type: boolean, default: false }
  required: []
depends_on:                             # other modules required to run before this
  - id: pluris.sshd.base-config
    version: ">=1.0.0 <2.0.0"
conflicts: []                           # mutually-exclusive modules
files:
  enforce: enforce.sh                   # required, idempotent
  validate: validate.sh                 # optional, returns JSON state
  rollback: rollback.sh                 # optional
```

#### Layout

```
modules/
  pluris.sshd.password-auth-disable/
    1.2.0/
      module.yaml
      enforce.sh
      validate.sh
      rollback.sh
      README.md
    1.1.0/    # previous version, retained for rollback
      …
```

#### Distribution

- Pluris ships a curated set of bundled modules (the former ADR-003 CSEs, ported to module format).
- The server hosts a per-tenant module registry. Admins can upload modules, edit them in-browser (Monaco editor), or pull from a community git registry (analogous to Ansible Galaxy / Helm repos).
- Modules are signed; `pluris-agent` verifies signatures before execution.

#### Resolution & execution

1. Server computes the effective policy bundle for `(asset, user)`.
2. For each policy in the bundle that has a configured value, server picks the admin-selected module (default: the highest-priority compatible module if none chosen).
3. Server resolves the dependency graph across all selected modules, fails closed on cycles or missing deps.
4. Server publishes the bundle (modules + parameter values, in topological order) to the agent over NATS.
5. Agent executes modules in order; each `enforce` script is required to be idempotent. `validate` is run periodically for drift detection.

#### UI

- Canonical editor: `editors/PolicyModuleEditor`. Reachable from:
  - **Policy** menu → **Modules** sub-tab (route `/policy/modules`) — full module list, filtered by tenant + permission. Moved here from Scripts on 2026-05-16: a Policy Module IS-A policy concept, not an automation concept.
  - **Server Administration** → **Policy Enforcement Scripts** — admin view, includes install/upgrade/disable controls.
  - **Policy Group editor** side-by-side view — Windows policy on left, list of compatible modules on right, in-place edit pane below.
- **Compatibility filter is mandatory** at every entry point: only show modules whose `satisfies` matches and whose `target_os` matches the device's OS. Show a count of hidden incompatible modules with a "show all" toggle for advanced users.

#### Editing & versioning

- Edits to a module in the UI create a new immutable version. The previous version is retained.
- Production deploy of a module version is an explicit action (the policy bundle pins module versions). Edits do not auto-deploy.
- A test-deploy flow runs the module on an admin "testing workstation" (per UX plan section on User/Admin Preferences) before broad rollout.

### Consequences

- Translation becomes data, not code. The translation-tier system in `docs/GROUP_POLICY_COMPATIBILITY.md` becomes a *suggestion* for how to author modules, not a Go package taxonomy.
- The forked `pluris-agent` shrinks: it no longer hosts the long list of CSE Go packages. It becomes a module *runtime* (with the PAM hook + drift loop + NATS subscriber) plus a small set of trusted helpers (file-edit primitives, idempotency wrappers).
- Community contribution model needs infra: registry, signing, CI tests for module idempotency / rollback round-trip. Phase 2+ deliverable; out of scope for Phase 0.
- ADR-003 §3 (CSE list) is **partially superseded**. ADR-003's other parts (PAM session hook, NATS bundle distribution, drift loop, mTLS, ansible-local escape hatch as a *runtime option for module scripts*) stand.

### Non-decisions (deferred)

- Module signing scheme (Sigstore / Cosign / detached PGP) — pick during Phase 2.
- Module registry transport (git / OCI / NATS object store) — pick during Phase 2.
- Sandboxing of `enforce.sh` execution on the agent — initial cut runs as root with no sandbox; later add bwrap / seccomp if module ecosystem grows.

---

## ADR-007 — Policy Module engine: full-ephemeral runtime, bash+WASM, tenant Ed25519 signing, refcount-based uninstall safety

**Date**: 2026-05-06
**Status**: accepted
**Extends**: ADR-006 (does not supersede; ADR-006's manifest + UI integration stand)
**Closes**: ADR-006 §"Non-decisions (deferred)" — signing, sandbox, transport.

### Context

ADR-006 fixed the *shape* of a Policy Module (manifest, lifecycle scripts, UI placement) and deferred three high-stakes details:

1. How module bytes get from server to endpoint without exposing them to disk-resident-malware or wire eavesdroppers (the user's stated bar: "unencrypted code only on server, prevent reading sensitive data via pcap or on client machine if possible").
2. Which runtimes the agent must trust (ADR-006 listed `bash | python | go | ansible-task`).
3. How dependency relationships between modules are tracked so an uninstall never breaks an unrelated dependent (the user's stated requirement: "system keeps tracks of installed dependencies and usage, preventing duplicate install or uninstalling service that will break something else").

Plus the user introduced a new requirement after ADR-006: **custom policies** authored in the management UI by tenant admins, with a guided wizard exposing the same lifecycle (apply / disable / uninstall) plus optional report/validate.

### Decision summary

| Topic | Decision |
|---|---|
| Module bytes at rest on agent | **Never written to non-tmpfs**. Decrypted into `memfd_create()` / mlock'd buffer; per-exec tmpfs at `/run/pluris/exec/$exec_id`; both unmounted/zeroed at exit. |
| Wire encryption | mTLS over NATS (per ADR-003) **plus** AEAD-sealed bundle inside the channel (XChaCha20-Poly1305). Defense in depth against TLS-terminating proxies. |
| Per-execution key | Single-use, 60s TTL, delivered on a separate NATS subject scoped to `(tenant, asset, exec_id)`. Server-side revocable mid-flight. Compromised wire snapshot without fresh key delivery is useless. |
| Trusted runtimes on agent | **bash** (apply / disable / uninstall) **+ WASM via wasmtime** (validate / report only). No Python, no ansible-task in v1. |
| Sandbox | bwrap + Landlock + seccomp-bpf; profile declared in manifest `capabilities:`, compiled by server, enforced by agent runtime. Default deny on filesystem write outside `/run/pluris/exec/$id` and on network egress. |
| Signing | **Tenant-managed Ed25519** keypair generated at install. Private key on management server (HSM-backed when available). Signature embedded in bundle envelope. Agent ships its tenant's public key as part of enrollment payload; rejects bundles signed by any other key. Sigstore/Rekor remains a deferred opt-in for tenants with public-internet attestation requirements. |
| Custom policies | Tenant-private catalog entries co-resident with bundled policies in the same Policy Catalog list, marked with a `Custom` chip. Authored via `editors/CustomPolicyWizard` (multi-step). URN under `tenant.<tenant_id>.<slug>`. Always backed by at least one tenant module. |
| Dependency tracking | New `ModuleInstallation` entity + computed refcount. `uninstall` lifecycle script runs only when refcount drops to zero. Conflicts declared in manifest block installation up-front; cycles fail closed. |

### Threat model (what this defends against)

| Adversary capability | Defense |
|---|---|
| Passive pcap of NATS traffic | mTLS terminates plaintext at endpoints; AEAD bundle is opaque ciphertext even if TLS is decrypted by a relay; per-exec key is not in the captured traffic. |
| Compromised TLS-terminating relay (corp proxy, transparent middlebox) | AEAD bundle remains sealed; relay sees ciphertext only. Key is delivered on a separate request-reply round-trip the relay cannot replay. |
| Root-on-endpoint malware reading scripts off disk | Modules never land on disk. memfd + tmpfs only; tmpfs is unmounted post-exec; key buffer mlock'd + zeroed. A live root attacker can attach to the running process during the exec window — accepted residual risk; mitigated by the small exec window and the sandbox restricting exfiltration paths. |
| Stolen tenant signing key | Per-tenant key rotation procedure (planned ADR); existing installations re-sign on next periodic refresh; agent-side key pinning on enrollment limits blast radius to the affected tenant. |
| Malicious module uploaded by compromised admin | Sigstore-style audit log of every upload + edit + deploy. Sandbox declared in manifest is enforced regardless of script content; declared `capabilities.network.egress` defaults to empty. UI-level edit produces an immutable new version; production deploy is a separate gated action that requires re-sign. |
| Lateral movement: tenant A's module run on tenant B's endpoint | Agent only trusts its enrollment-time tenant public key. Server-side bundle signing and key-delivery subjects are tenant-scoped. |

Out of scope (for ADR-007): nation-state attackers with kernel-level endpoint compromise (TPM-attested boot is a future ADR), multi-tenant supply chain (no cross-tenant module sharing in v1).

### Module lifecycle (final shape)

Three required + two optional scripts. Same shape regardless of bundled vs custom origin.

| Phase | Required | Idempotency | Refcount semantics |
|---|---|---|---|
| `apply`     | yes | required idempotent | runs on every binding apply; cheap re-runs expected |
| `disable`   | optional | required idempotent | runs on binding disable; reversible — leaves files, disables effect |
| `uninstall` | optional | required idempotent | **runs only when refcount = 0** — see "Dependency tracking" below |
| `validate`  | optional | pure / read-only | periodic drift check; WASM runtime; outputs JSON state |
| `report`    | optional | pure / read-only | structured data return; WASM runtime; output validated against `report_schema` |

Standardized environment for every script:

```
PLURIS_PARAMS_FD=3            # parameter values JSON, file descriptor only
PLURIS_PHASE=apply|disable|uninstall|validate|report
PLURIS_PREVIOUS_VERSION       # for upgrade-aware apply
PLURIS_REPORT_FD=4            # write JSON report here (apply/validate/report)
PLURIS_TMPDIR=/run/pluris/exec/$exec_id   # tmpfs, auto-unmounted
PLURIS_LOG_FD=5               # structured log — agent forwards to server audit log
```

No env vars carry sensitive data; everything goes through fds the sandbox controls.

### Manifest extensions over ADR-006

```yaml
# All ADR-006 fields stand, plus:
capabilities:
  filesystem:
    write: ["/etc/ssh/sshd_config.d/"]   # absolute paths or globs
    read:  ["/etc/ssh/", "/proc/version"]
  network:
    egress: []                           # default-deny; non-empty = explicit allow-list
  syscalls: bash                         # bash | python(disallowed in v1) | restricted | custom
  user: root                             # root | $target_user | nobody
report_schema:                            # JSON Schema; report.sh output is server-validated
  type: object
  properties:
    sshd_running: { type: boolean }
lifecycle:
  apply:     enforce.sh
  disable:   disable.sh
  uninstall: rollback.sh
  validate:  validate.wasm                # WASM only for validate/report
  report:    report.wasm
signing:
  algo: ed25519
  signature: <base64>                     # tenant-key signature over manifest + script hashes
  signed_by: tenant:<tenant_id>:key:<key_id>
```

### Dependency tracking — the hard requirement

#### Entity: `ModuleInstallation`

One row per `(asset, module_version)` actually present on an asset.

| Field | Purpose |
|---|---|
| `id` | UUID |
| `asset` | edge to Asset |
| `module`, `module_version_pinned` | which module + immutable version |
| `installed_via` | edge to `ConfigurationGroupBinding` OR another `ModuleInstallation` (transitive) — the **reason** this is here |
| `state` | `pending | applied | disabled | failing | orphaned` |
| `applied_at`, `last_validated_at`, `last_report` | runtime status |

`installed_via` is a multi-edge: a module can be installed because three different bindings request it. **Refcount = count of incoming `installed_via` edges where the source is not orphaned.** Computed on read; not stored.

#### Operations

- **Install** (binding added or wider scope match): server resolves transitive deps, creates `ModuleInstallation` rows for every node in the closure, links `installed_via` edges, pushes bundles to agent in topological order. **If a node already has a row, only an edge is added — the apply script is not re-run.**
- **Uninstall** (binding removed):
  1. Remove the originating `installed_via` edge.
  2. For each affected node, recompute refcount.
  3. `refcount > 0` → leave installed; state stays `applied`. UI surfaces "still required by N other groups".
  4. `refcount = 0` → run `uninstall` script, remove row. Cascade: nodes that became zero only because their last dependent went away are uninstalled in reverse-topological order.
- **Conflict**: manifest `conflicts:` evaluated up-front; if any active installation conflicts, the binding is rejected. UI shows the conflict path.
- **Upgrade**: new version's `apply` runs; old version's `uninstall` runs only when no binding still pins the old version.

#### Invariants (enforced server-side, mirrored in tests)

- **INV-M1.** `uninstall` is invoked at most once per `ModuleInstallation` row, and only when refcount drops to zero.
- **INV-M2.** No cycles in the module dependency graph. Server rejects manifests that would create one.
- **INV-M3.** A binding cannot be saved if its declared module has unmet dependencies in the tenant's module catalog.
- **INV-M4.** A `ModuleInstallation` row's `installed_via` set is never empty (an empty set is the trigger to physically uninstall and delete the row).

### UI

- **Single Policy Catalog** with `Custom` chip on tenant-authored entries. Same search, same tree, same scope partitioning.
- **`+ New custom policy`** button on the Policy Catalog page → mounts `editors/CustomPolicyWizard` (multi-step):
  1. Identify (name, description, scope, category placement)
  2. Satisfies (link to existing catalog entry as override, or declare new tenant URN)
  3. Parameters (visual JSON-Schema builder)
  4. Dependencies (pick from existing modules; live conflict graph)
  5. Lifecycle scripts (Monaco; shellcheck on save; templates for systemd / sysctl / PAM / file edit)
  6. Sandbox profile (capability picker; sane defaults inferred from script content)
  7. Test (runs on admin's testing workstation, live log stream, round-trip apply→disable→uninstall verification)
  8. Sign & publish (tenant Ed25519 signature; publish creates first immutable version)
- **`editors/PolicyModuleEditor`** at Scripts → Policy Modules: list of all module versions, in-place script edit creates a new immutable version that requires re-sign.
- **Module picker inside `ConfigurationGroupDialog`**: same component pattern as the target picker (`@TargetPickerDialog`) but lists candidate modules per binding, filtered by `satisfies` ∩ catalog URN ∩ device OS. Reusable from Profile editor and Computer detail.
- **Computer detail → Installed Modules tab**: dep tree visualization, refcount per row, click-through to the binding that put each module there.

### Phasing

| Phase | Scope |
|---|---|
| 0 (this slice) | Manifest types in Go; mock module catalog; UI scaffolding for Policy Modules page + Custom Policy wizard skeleton; `Custom` chip on catalog rows; module picker inside CG dialog (UI only). |
| 1 | Backend Ent schemas: `PolicyModule`, `PolicyModuleVersion`, `ModuleInstallation`, `CustomPolicy`. Dep resolver + refcount engine. Tenant Ed25519 keypair generation at install. |
| 2 | Agent runtime: bwrap + Landlock + seccomp; AEAD bundle decode; per-exec key request-reply; tmpfs mount/unmount; bash exec harness; WASM (wasmtime) embedding for validate/report. |
| 3 | Custom Policy wizard end-to-end: Monaco editor; shellcheck on save; sandbox capability picker; testing-workstation round-trip. |
| 4 | Sigstore opt-in path; community module registry. |

### Consequences

- ADR-006's deferred decisions are now resolved. ADR-006's manifest schema and UI integration stand; this ADR adds capability declarations, signing fields, and the report schema.
- ADR-003 stands unchanged (mTLS, NATS bundles, drift loop, PAM session hook). The "ansible-local escape hatch" mentioned in ADR-003 is **deferred to phase 4+**; v1 ships without Ansible.
- Agent fork (per `docs/FORK_STRATEGY.md`): adds the AEAD/key-request runtime + bwrap/Landlock/seccomp harness + wasmtime embed. Removes the hardcoded CSE list (already done by ADR-006) and the ansible-local trampoline (deferred).
- Backend gains four new Ent schemas (Phase 1). Refcount logic is computed from edges, not stored — keeps schema simple and consistent.

### Non-decisions (newly deferred)

- Sigstore/Rekor opt-in path — Phase 4. Verification interface is a single function on the agent; swapping the trust root from Ed25519 pinned key to Rekor-rooted X.509 is a localized change.
- Module registry transport for community submissions (git / OCI / NATS object store) — Phase 4.
- TPM-attested boot for endpoint key sealing — separate ADR, post-Phase 4.
- Multi-tenant module sharing (a module published once, used across tenants) — out of scope; v1 is strictly per-tenant.

---

## ADR-008 — Extension Framework: unified abstraction for Policy Modules, Profiles, Scripts, Wine Configs, Packages

**Date**: 2026-05-17
**Status**: accepted
**Generalizes**: ADR-006, ADR-007 (Policy Modules become the first concrete `Kind` in a generic framework; ADR-006/007 manifests, signing, runtime semantics stand unchanged for `KindPolicyModule`).
**Related**: implements R2 (no parallel implementations) for the family of "user/community-supplied, signed, versioned, lifecycle-managed" content types referenced by `docs/Pluris UX structure plan.md`.

### Context

The UX plan repeatedly describes the same shape for several different concepts:

- Policy Modules (ADR-006/007).
- Profiles (assigned bundles of policy/scripts/wine config).
- Scripts (admin-authored automation).
- Wine Configurations (per-app DLL/registry overrides).
- Package recipes (Linux/Windows/Wine package install bundles).

Each of these has the same metadata skeleton: a stable id, a human title, a source (bundled / tenant / imported / community), a lifecycle state (draft / published / superseded / disabled), one or more semver versions, optional cryptographic signatures, and a per-tenant install/enable state. Every one of them needs the same Sources page treatment ("how many of each Source do you have?"), the same picker UX, the same audit-log surface, and the same import/export flow.

Implementing each as an isolated Go package risks five forks of the same code (browse, picker, lifecycle transitions, signature verification, audit, count aggregation). That violates **R2**.

### Decision

Introduce `pkg/extension/` — a generic framework that defines the shared shape and the cross-kind operations. Each concrete content type lives in its own package (e.g. `catalog/policymodules/`) and registers a `Kind` plus a `Loader` with the framework. Concrete entities provide an adapter that exposes their domain object as `extension.Extension`.

#### Core types (`pkg/extension/types.go`)

| Type | Purpose |
|---|---|
| `Kind` | string enum of registered kinds (`KindPolicyModule`, future: `KindProfile`, `KindScript`, `KindWineConfig`, `KindPackage`). Registration is explicit via `RegisterKind`; unknown kinds are rejected. |
| `Source` | provenance: `bundled` / `tenant` / `imported` / `community`. Drives the Sources page and the `IsEditable()` predicate (only `tenant` is editable in-place). |
| `LifecycleState` | `draft` / `published` / `superseded` / `disabled` / `revoked`. `IsDeployable()` is true for `published` and `superseded` (pinned bindings keep working). `IsTerminal()` is true for `revoked` only — revocation is one-way. |
| `Signature` | `Signer` + `KeyID` + `Algo` + base64 `Bytes`. v1 fixes `Algo` to `ed25519` and leaves `Bytes` empty (mock only); the trust chip renders from `Signer` / `KeyID`. INV-X3 requires `IsZero() == false` for every non-draft version. |
| `Manifest` | universal header: `Kind`, `ID`, `Title`, `Description`, `Source`, `TenantID`. Kind-specific fields stay on the concrete package's struct (no opaque `Payload` field — concrete types are reachable via type assertion when the caller knows the kind). |
| `Version` | `Version` (semver string), `State`, `PublishedAt`, `PublishedBy`, `Signature`. |
| `Extension` interface | three methods only: `Manifest() Manifest`, `Versions() []Version` (newest first), `LatestVersion() *Version` (most recently published, `nil` if none). Per-extension identity (`Kind`, `ID`, `Title`, `Source`) is read from the returned `Manifest`. |

#### Catalog (`pkg/extension/catalog.go`)

The framework's catalog is **only** a registry of `Kind → KindSpec`. It does not own storage; it delegates listing to the per-kind `Loader`. This keeps every concrete kind authoritative for its own data.

```go
type Loader func() []Extension

type KindSpec struct {
    Kind        Kind     // discriminator
    Title       string   // family name shown in chrome ("Policy Modules")
    Description string   // one-sentence intro for the empty state
    Loader      Loader   // returns every Extension of this kind
}

func RegisterKind(spec KindSpec)              // panics on dup or nil Loader
func LookupKind(k Kind) (KindSpec, bool)
func RegisteredKinds() []KindSpec             // sorted by Kind
func AllOfKind(k Kind) []Extension
func All() []Extension                        // union across kinds
func CountBySource(k Kind) map[Source]int     // pass "" for all kinds
```

Cross-kind queries (`All`, `CountBySource`) iterate registered loaders. Pages that present a unified view (Sources, global search, audit log filters) read through these functions, never through per-kind packages directly. `CountBySource` only counts extensions that have at least one published version — drafts have no source to attribute.

#### Adapter pattern

Each concrete package keeps its domain types and adds a thin adapter (e.g. `catalog/policymodules/extension_adapter.go`) that satisfies `extension.Extension`. The package's `init()` calls `extension.RegisterKind` with a `KindSpec` whose `Loader` returns the package's current entries. Tests assert interface satisfaction at compile time (`var _ extension.Extension = (*Module)(nil)`).

This means:

- **No data migration.** Existing `policymodules.Module` keeps its full domain API; the adapter is purely additive.
- **No god-package.** `pkg/extension` knows nothing about policy semantics, runtimes, or sandboxes. ADR-006/007 details remain encapsulated in `catalog/policymodules`.
- **Future kinds are mechanical.** Adding `Profile` is: define the Profile domain types in `catalog/profiles/`, write an adapter, register the kind. The Sources page, picker, and aggregation queries pick it up for free.

#### UI integration

- The Sources sub-page on `/policy/modules/sources` reads counts via `extension.CountBySource(extension.KindPolicyModule)` — one path, no per-source branching in the template.
- The Policy Module picker dialog (and future per-kind pickers) iterates `extension.AllOfKind(...)` filtered by `Source.IsEditable()` / `LifecycleState.IsDeployable()` predicates exposed on the framework. No picker has its own data-loading code.
- New invariants INV-X1..X4 in `docs/UX_INVARIANTS.md` codify: (X1) every "user-supplied content" kind goes through `pkg/extension`; (X2) Sources pages read through the framework; (X3) lifecycle/source semantics are framework-defined, not per-kind; (X4) cross-kind UI (search, audit, dashboard tiles for "extensions") iterates registered kinds.

### Consequences

- One canonical place to evolve the shared shape. Adding `KindProfile`, `KindScript`, `KindWineConfig`, `KindPackage` is now a small, repeatable change.
- ADR-006 and ADR-007 are not invalidated — `KindPolicyModule` is the first concrete kind. ADR-006/007 manifests stay on `policymodules.Module`; the adapter projects only the universal header into `extension.Manifest`. Callers that need policy-module specifics type-assert to the concrete type.
- The Sources page is no longer Policy-Module-specific in its data path; it can be reused for other kinds with a `Kind` parameter.
- Tests now have two layers: per-kind tests (existing) and a framework-level test that exercises `RegisterKind` / `Loader` / `CountBySource` against fakes. Both layers must pass for a kind to be considered "registered".
- **R2 enforcement**: a code review check looks for new `*_simple.go` / `*_v2.templ` siblings in catalog/* — they are forbidden; extend the canonical kind instead.

### Non-decisions (deferred)

- Per-kind verifier registration API for signatures (currently `Signature` is opaque; verification is per-kind). Will formalize when a second kind ships signed content.
- A persistence layer for cross-kind audit/history — Phase 1 deliverable, will likely be a single Ent schema referencing `(kind, id, version)` tuples.
- A web HTTP API for "list all extensions of a kind" — the in-process `Loader` interface is enough until the management console grows a public REST surface.

---

## ADR-009 — FreeIPA: integration target, not a foundation

**Date**: 2026-05-17
**Status**: accepted
**Related**: clarifies the identity / PKI strategy alongside the Kanidm direction in the UX plan and ADR-001's "no end-user identity model in OpenUEM" finding.

### Context

A reasonable question every few months: "FreeIPA looks similar — should we build on it or vendor it?" Answering once and recording the answer prevents re-litigation.

FreeIPA is an identity & authentication stack (389-DS LDAP + MIT Kerberos KDC with AD cross-realm trust + Dogtag CA + BIND DNS + SSSD on clients). It also provides HBAC rules, sudo rules, automount maps, ID views, and SELinux user maps. License is GPLv3 (Pluris is AGPLv3 — compatible at the boundary, not for in-tree vendoring).

What FreeIPA does **not** provide and Pluris does:

- Group Policy / setting enforcement model (Pluris's differentiator).
- Asset / CMDB management.
- Software / package / update orchestration, Wine config, Profiles, Scripts.
- Multi-tenant model (FreeIPA is one realm = one tenant).
- Drift detection and signed-bundle distribution to agents.
- Windows endpoint management (FreeIPA's AD trust is for AD users authenticating to Linux hosts, not for managing Windows endpoints).

The overlap is essentially the **Identity** entity, AD interop, and PKI for mTLS device certs — three of the ~25 concerns in `docs/Pluris UX structure plan.md`.

### Decision

**Do not** build on FreeIPA, vendor any FreeIPA component, or treat it as a substrate. Adopting 389-DS + Kerberos + Dogtag for the identity layer would inherit substantial operational complexity for a slice that Kanidm covers with a single binary.

**Do** treat FreeIPA as a first-class **integration target**, on the same footing as Active Directory:

1. **Identity backend adapter.** Pluris's `Identity` entity reads from a pluggable provider; FreeIPA joins the provider list (LDAP bind + SSSD-style group resolution + Kerberos SSO) alongside Kanidm and AD. Phase 2 work; not Phase 0.
2. **PKI option for mTLS device certs.** ADR-003/007 already keep the trust root pluggable. Allow Dogtag as a CA backend so existing IPA shops do not run two CAs.
3. **HBAC / sudo-rule import.** A one-shot migration helper that imports HBAC and sudo rules and renders them as Pluris policy bindings, read-only first, editable on user opt-in. No runtime dependency.

### Design influences (sanity-check only, not borrowed code)

- HBAC's `who × what × where × when` rule shape is a useful sanity check on the Pluris `PolicyGroup` assignment model. They already match in spirit.
- IPA "ID Views" (host-group-scoped attribute overlays) parallel Pluris **Profile overlays**. Worth a sentence in the Profile-merge-order docs when that lands.

### Consequences

- Pluris does not gain GPLv3 boundary issues — FreeIPA stays a peer, communicating over standard protocols (LDAP, Kerberos, ACME / certmonger, REST).
- The identity provider matrix grows by one row in Phase 2: `{Kanidm, AD, FreeIPA, generic OIDC}`.
- Customer-shop migrations from "AD + FreeIPA Linux side" to Pluris become a documented path: import HBAC, point SSSD-managed hosts at the Pluris agent, keep IPA as the auth provider until a later cutover.

### Non-decisions (deferred)

- The exact Identity provider abstraction (interface shape, capability negotiation) — Phase 2.
- Whether to also support **synthesizing** a FreeIPA-compatible LDAP/Kerberos surface from Pluris (so existing IPA-aware Linux hosts could join Pluris without reconfig). Out of scope for v1; revisit if customer demand surfaces.
- Reverse migration (Pluris → FreeIPA) — explicitly out of scope.

