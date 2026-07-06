#!/usr/bin/env bash
# Full setup: install Go, templ, enable systemd service + Tailscale funnel
set -euo pipefail

echo "=== Installing Go (if missing) ==="
if ! command -v go &>/dev/null && [ ! -f /tmp/opencode/go/bin/go ]; then
  curl -sL https://go.dev/dl/go1.22.4.linux-amd64.tar.gz -o /tmp/opencode/go.tar.gz
  mkdir -p /tmp/opencode
  tar -C /tmp/opencode -xzf /tmp/opencode/go.tar.gz
fi
export PATH="/tmp/opencode/go/bin:$HOME/go/bin:$PATH"
export GOPATH="$HOME/go"

echo "=== Installing templ ==="
go install github.com/a-h/templ/cmd/templ@v0.2.793

echo "=== Enabling systemd user service ==="
systemctl --user daemon-reload
systemctl --user enable pluris-dev
systemctl --user restart pluris-dev

echo "=== Enabling Tailscale Funnel ==="
sudo tailscale funnel --bg 8080

echo ""
echo "Done! Pluris is available at:"
echo "  Local:  http://localhost:8080"
echo "  Public: https://YOUR-DEV-HOST.example.com/"
