#!/usr/bin/env bash
# rollback.sh — remove the snippet and restore default sshd password-auth behavior.
# Idempotent.

set -euo pipefail

SNIPPET_PATH="/etc/ssh/sshd_config.d/50-pluris-password-auth.conf"

if [[ ! -f "$SNIPPET_PATH" ]]; then
    echo "unchanged"
    exit 0
fi

rm -f "$SNIPPET_PATH"

if ! sshd -t 2>/dev/null; then
    echo "sshd -t failed after rollback; manual intervention required" >&2
    exit 1
fi

if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet ssh sshd 2>/dev/null; then
    systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || true
fi

echo "changed"
