# Pluris ITSM

**What:** Planned ticketing, self-service, and software-assignment layer for Pluris; not started.

**Related:** [[pluris]], [[endpoint-management]], [[authorization]], [[roadmap]]

## Status

Not started. Requirements gathered from the owner's intended usage, no code, no schema, no UI.

## Planned scope

- **Tickets and incidents.** Admins log, track, and resolve issues raised against identities or assets.
- **Self-service portal.** End users edit their own account info and choose settings (e.g. firewall rules) from a catalog, and file tickets themselves. The self-service *editing* piece is already partly live today in Endpoint Management via the Pluris Policy self-service field-editing scope (`Own` grants on `identity.update`) — ITSM would extend this pattern to catalog-driven settings and ticket creation, not invent a new mechanism.
- **Software assignment.** Workflow for assigning software/packages to users or asset groups (builds on the endpoint policy module system once an enforcement path exists).

## Interconnection

ITSM will consume Endpoint Management's existing identity and asset inventory rather than keeping its own copy. The Pluris Policy permission registry (`catalog/permissions/`) is deliberately structured so a new `itsm` domain with its own actions can be added additively — same grants engine, same matrix UI, no schema rework. See [[authorization]].
