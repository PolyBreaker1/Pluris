#!/usr/bin/env bash
# validate.sh — report current state of the policy this module enforces.
#
# Output: a single JSON object on stdout describing observed state. The agent
# compares this to the desired state to detect drift.

set -euo pipefail

SNIPPET_PATH="/etc/ssh/sshd_config.d/50-pluris-password-auth.conf"

# `sshd -T` is the canonical way to read effective config (handles all Match blocks).
# Fall back to grepping the snippet if sshd isn't available.
if command -v sshd >/dev/null 2>&1 && sshd -T -C 'addr=127.0.0.1' 2>/dev/null > /tmp/.pluris-sshd-T.$$; then
    PASSWORD_AUTH=$(awk '$1=="passwordauthentication"{print $2; exit}' /tmp/.pluris-sshd-T.$$)
    PERMIT_ROOT=$(awk '$1=="permitrootlogin"{print $2; exit}' /tmp/.pluris-sshd-T.$$)
    rm -f /tmp/.pluris-sshd-T.$$
elif [[ -f "$SNIPPET_PATH" ]]; then
    PASSWORD_AUTH=$(awk '/^PasswordAuthentication/{print tolower($2); exit}' "$SNIPPET_PATH")
    PERMIT_ROOT=$(awk '/^PermitRootLogin/{print tolower($2); exit}' "$SNIPPET_PATH")
else
    PASSWORD_AUTH="unknown"
    PERMIT_ROOT="unknown"
fi

cat <<EOF
{
  "snippet_present": $([[ -f "$SNIPPET_PATH" ]] && echo "true" || echo "false"),
  "password_authentication": "${PASSWORD_AUTH:-unknown}",
  "permit_root_login": "${PERMIT_ROOT:-unknown}"
}
EOF
