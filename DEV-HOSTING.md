# Pluris Dev Hosting

## Public URL

**https://YOUR-DEV-HOST.example.com/**

Accessible from any network (internet-facing via Tailscale Funnel).

## Architecture

```
Internet --> Tailscale Funnel (HTTPS :443) --> localhost:8080 --> pluris-console (Go)
```

Two components keep this running:

1. **Tailscale Funnel** (`--bg` mode) - proxies HTTPS traffic from the public URL to localhost:8080. Persists across reboots as part of Tailscale's serve config.
2. **systemd user service** (`pluris-dev.service`) - runs the Go console app with auto-restart. Enabled with lingering so it survives logout.

## Quick Commands

| Action | Command |
|--------|---------|
| Restart dev server | `./scripts/restart-dev.sh` |
| View logs | `journalctl --user -u pluris-dev -f` |
| Stop server | `systemctl --user stop pluris-dev` |
| Start server | `systemctl --user start pluris-dev` |
| Check status | `systemctl --user status pluris-dev` |
| Disable funnel | `sudo tailscale funnel --https=443 off` |
| Re-enable funnel | `sudo tailscale funnel --bg 8080` |
| Full setup from scratch | `./scripts/setup-dev.sh` |

## After Code Changes

The server must be restarted to pick up changes (templ regeneration + Go rebuild):

```bash
./scripts/restart-dev.sh
```

Or manually:
```bash
systemctl --user restart pluris-dev
```

## Service File Location

`~/.config/systemd/user/pluris-dev.service`

After editing it, run:
```bash
systemctl --user daemon-reload
systemctl --user restart pluris-dev
```

## Troubleshooting

- **Port 8080 in use**: `lsof -i :8080` to find the process, kill it, then restart
- **Go not found**: Re-run `./scripts/setup-dev.sh` (installs Go to /tmp/opencode/go)
- **Funnel not working**: Check `sudo tailscale funnel status` and ensure the node is approved in the Tailscale admin panel
- **Service failing**: `journalctl --user -u pluris-dev --no-pager -n 50` for logs
