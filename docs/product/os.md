# Pluris OS

**What:** Idea for a managed Linux image with the Pluris agent pre-enrolled — under consideration only, no commitment.

**Related:** [[pluris]], [[endpoint-management]]

## Status

Under consideration, no commitment. No code, no build pipeline, no design doc beyond early exploratory notes. Not a current priority.

## The idea

A Debian/Ubuntu-based managed Linux OS image, pre-configured and pre-enrolled with the Pluris endpoint agent, so deploying a new managed machine is closer to "boot the image" than "install Linux, then install and enroll an agent." Early exploratory notes considered a KDE-based, Windows-familiar desktop with a Wine/Proton compatibility layer for app migration, but none of that has been decided or scoped against the current Endpoint Management build.

## What would make it real

- A working Linux endpoint agent (Endpoint Management's current top planned milestone — see [[roadmap]]) — Pluris OS cannot exist before the agent does.
- Evidence that fleet admins want a turnkey image rather than "install the agent on our existing image," which has not been validated with users yet.
- A dedicated build/ISO pipeline, entirely separate from the console codebase.
