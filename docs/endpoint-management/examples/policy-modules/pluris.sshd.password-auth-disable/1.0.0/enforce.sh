#!/usr/bin/env bash
# enforce.sh — disable SSH password authentication. Idempotent.
#
# Inputs (env, populated by the agent from the module's `parameters` schema):
#   PLURIS_PARAM_ALLOW_ROOT   "true" | "false"  (default "false")
#
# Output: prints "changed" on stdout if the system state was modified, "unchanged" otherwise.
# Exit:   0 on success (changed or unchanged), non-zero on failure.

set -euo pipefail

ALLOW_ROOT="${PLURIS_PARAM_ALLOW_ROOT:-false}"
SNIPPET_PATH="/etc/ssh/sshd_config.d/50-pluris-password-auth.conf"

# Render desired content.
desired_content() {
    cat <<EOF
# Managed by Pluris module pluris.sshd.password-auth-disable. Do not edit.
PasswordAuthentication no
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin $([[ "$ALLOW_ROOT" == "true" ]] && echo "yes" || echo "no")
EOF
}

# Compare current vs desired. Idempotent short-circuit.
if [[ -f "$SNIPPET_PATH" ]] && diff -q <(desired_content) "$SNIPPET_PATH" >/dev/null 2>&1; then
    echo "unchanged"
    exit 0
fi

# Atomic write via temp file in the same dir.
TMP="$(mktemp "${SNIPPET_PATH}.XXXXXX")"
trap 'rm -f "$TMP"' EXIT
desired_content > "$TMP"
chmod 0644 "$TMP"
chown root:root "$TMP"

# Validate the resulting full sshd config before swapping in.
mv "$TMP" "$SNIPPET_PATH"
trap - EXIT

if ! sshd -t 2>/dev/null; then
    # Roll back this snippet immediately to avoid breaking SSH.
    rm -f "$SNIPPET_PATH"
    echo "sshd -t failed; rolled back" >&2
    exit 1
fi

# Reload sshd if it's running. Don't fail if the unit isn't installed (e.g. in a build chroot).
if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet ssh sshd 2>/dev/null; then
    systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || true
fi

echo "changed"
