#!/usr/bin/env bash
# Restart the Pluris dev server (regenerates templ, restarts Go app)
set -euo pipefail
systemctl --user restart pluris-dev
sleep 2
systemctl --user status pluris-dev --no-pager
echo ""
echo "Pluris dev server restarted at http://localhost:8080"
echo "Public URL: https://YOUR-DEV-HOST.example.com/"
