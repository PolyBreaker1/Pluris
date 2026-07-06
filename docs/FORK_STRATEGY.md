# Pluris Fork Strategy — historical note

> **Stripped 2026-05-05.** This file used to be a 310-line "fork all of OpenUEM" plan. That plan is **superseded** by ADR-001 (build console fresh) and ADR-002 (selective reuse — fork only `openuem-agent`, depend on `openuem-nats` as a Go module). What remains here is the Apache 2.0 mechanics, since they are still operative for the agent fork. Full prior version is in git history.
>
> Canonical: `docs/ARCHITECTURE_DECISIONS.md` (ADR-001, ADR-002, ADR-003, ADR-006).

---

## What we actually fork

`openuem-agent` only. Tracked as a git remote on the fork at `pluris/pluris-agent/` so we can cherry-pick upstream fixes. We rename, add LICENSE + NOTICE, and replace its enforcement subsystem with the Policy Module runtime (ADR-006).

`openuem-nats` is imported as a Go module dependency, not forked. If upstream breaks compatibility we pin or vendor.

Everything else (`openuem-console`, `openuem-worker`, `ent`, `openuem-ocsp-responder`, `openuem-nats-service`) is **not** forked. We build fresh per ADR-001.

---

## Apache 2.0 fork mechanics (what we MUST do)

For `pluris-agent/`:

1. **Include the original LICENSE file** — preserve OpenUEM's `LICENSE` as `pluris-agent/PLURIS-AGENT-UPSTREAM-LICENSE`.
2. **Include any original NOTICE** as `pluris-agent/PLURIS-AGENT-UPSTREAM-NOTICE`.
3. **State changes** — the project-root `NOTICE` file documents that `pluris-agent/` is derived from `openuem-agent`, modifications attributed to The Pluris Authors.
4. **Keep copyright notices** in original source files. New files we write get a Pluris copyright header.

What we MAY do (and choose to):

- Rename, rebrand, redistribute under our own name.
- Sell support/services around it commercially.
- Stay Apache 2.0 (consistent with upstream; simplest).
- Not contribute changes back upstream (we may; not required).

---

## What OpenUEM's agent gives us (worth the fork)

Concretely the parts of `openuem-agent` we want to inherit and not rebuild:

- Multi-OS endpoint support: Windows, Linux (Debian/RedHat), macOS (Intel + Apple Silicon).
- mTLS enrollment + certificate-based identity.
- NATS subscriber wiring with reconnection / drift handling.
- Hardware + software inventory collection.
- Multi-OS package-manager helpers (Winget, Flatpak, Homebrew) — these become Package Manager modules per ADR-006.
- Install scripts and packaging (msi, deb, pkg).

What we **do not** keep:

- Its rigid `Task` / `Profile` / `Tag` model — we build a Policy Module runtime instead (ADR-006).
- Its hardcoded Linux enforcement (`Task` types) — replaced by user-editable Policy Modules.

---

## Risk on this fork (the only real one)

Upstream `openuem-agent` could refactor in ways that conflict with our Policy Module runtime. Mitigation: track upstream as a remote, cherry-pick fixes only, accept that we own the divergence. We're not trying to upstream the Policy Module runtime; it's our differentiator.
