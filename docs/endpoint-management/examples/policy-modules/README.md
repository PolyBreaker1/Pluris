# Policy Module — sample package

> Spec-by-example for the Policy Module format defined in ADR-006 (see [[decisions]]). **Not wired into the agent yet** — this directory exists only as a reference shape against which the runtime will be tested in Phase 2.

## Layout

```
docs/examples/policy-modules/
  pluris.sshd.password-auth-disable/
    1.0.0/
      module.yaml      — manifest; canonical metadata + parameters JSON Schema
      enforce.sh       — required, idempotent enforcement
      validate.sh      — optional, returns JSON of observed state for drift detection
      rollback.sh      — optional, undoes enforcement
    1.0.1/             — future revisions live as sibling directories
```

## Conventions used by the sample

- All scripts use `set -euo pipefail` (bash strict mode).
- Parameters are passed via `PLURIS_PARAM_<UPPERCASE_NAME>` env vars, populated by the agent from the validated parameter object.
- Files written under `/etc/` are atomic (temp file in the same directory, `mv` into place).
- Validation runs **before** the swap-in succeeds (`sshd -t`); failed validation rolls back the snippet immediately.
- `enforce.sh` and `rollback.sh` print `changed` or `unchanged` on stdout so the agent can report whether the run mutated state. This isn't required by ADR-006 but is a useful convention for runtime telemetry.

## Open questions for the format (defer to Phase 2)

- **Signing**: how is `module.yaml + scripts` signed and verified by the agent? Cosign? Detached PGP?
- **Manifest validation**: JSON Schema for the manifest itself (separate from the per-module parameter schema).
- **Runtime sandboxing**: today the agent will run scripts as root with no sandbox. Future: bwrap / seccomp profile.
- **Cross-module communication**: should modules be able to read state another module produced (e.g. a `sshd.base-config` module dropping `Include` directives that downstream modules rely on)? Initial answer: no — modules are independent and connected only via `depends_on` ordering.
