#!/usr/bin/env bash
# Pluris Test Console — single GUI controller (KDE/kdialog, zenity fallback).
#
# Replaces the separate start/stop scripts: start, stop and live status are
# all handled from one control window. The server keeps running in the
# background after the panel is closed; open the panel again to stop it.
#
#   ./scripts/pluris-gui.sh            open the control panel
#   ./scripts/pluris-gui.sh --start    start server, terminal output only
#   ./scripts/pluris-gui.sh --stop     stop server, terminal output only
#   ./scripts/pluris-gui.sh --status   print status, terminal output only
#   ./scripts/pluris-gui.sh --notify   with --start/--stop: add a KDE notification
#   (plain 'start'/'stop'/'status' work too; --help prints full usage)
#
# Non-GUI modes never open dialogs; they print status to stdout and set a
# meaningful exit code (0 = server running/stopped, 1 = failure).
#
# GUI: pick Start/Stop/Status/Quit from a kdialog menu. Starting runs in the
# background and shows a popup notice when the server is up (or an error dialog).
# The server is launched detached (setsid) so closing the panel or launcher
# never kills it.
#
# Env: PLURIS_HTTP_ADDR (default :8080), PLURIS_RUN_DIR, PLURIS_SEED=1 (demo data).
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT_ADDR="${PLURIS_HTTP_ADDR:-:8080}"
PORT="${PORT_ADDR##*:}"
RUN_DIR="${PLURIS_RUN_DIR:-"$HOME/.local/state/pluris-test"}"
BIN="$REPO_DIR/bin/pluris-console"
PIDFILE="$RUN_DIR/pluris-console.pid"
LOG="$RUN_DIR/server.log"
HEALTH_URL="http://localhost:${PORT}/healthz"
URL="http://localhost:${PORT}/"

have_kdialog() { command -v kdialog >/dev/null 2>&1; }
have_zenity() { command -v zenity >/dev/null 2>&1; }

is_running() { curl -fsS "$HEALTH_URL" >/dev/null 2>&1; }

process_alive() {
  [[ -f "$PIDFILE" ]] || return 1
  local pid
  pid="$(tr -d '[:space:]' < "$PIDFILE")"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  [[ -d "/proc/$pid" ]] || return 1
  tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -q "pluris-console"
}

find_go() {
  if command -v go >/dev/null 2>&1; then
    command -v go
    return 0
  fi
  for cand in "$HOME/.local/opt/go/bin/go" /usr/local/go/bin/go /tmp/opencode/go/bin/go; do
    if [[ -x "$cand" ]]; then
      echo "$cand"
      return 0
    fi
  done
  return 1
}

ensure_binary() {
  if [[ "${PLURIS_REBUILD:-0}" == "1" || ! -x "$BIN" ]]; then
    local gobin
    gobin="$(find_go)" || { echo "ERROR: 'go' not found (install it or set PATH)." >&2; return 1; }
    echo "Building $BIN ..."
    (cd "$REPO_DIR" && "$gobin" build -buildvcs=false -o "$BIN" ./cmd/console)
  fi
}

wait_ready() {
  local tries=0
  while ! curl -fsS "$HEALTH_URL" >/dev/null 2>&1; do
    tries=$((tries + 1))
    [[ $tries -ge 40 ]] && return 1
    sleep 0.5
  done
  return 0
}

seed_demo() {
  local gobin
  gobin="$(find_go)" || { echo "ERROR: 'go' not found; cannot seed." >&2; return 1; }
  echo "Seeding demo data into $RUN_DIR/pluris.db ..."
  (cd "$REPO_DIR" && "$gobin" run -buildvcs=false ./cmd/seed --db="$RUN_DIR/pluris.db" --tenant=demo)
}

start_now() {
  if is_running; then
    echo "Already running: $(cat "$PIDFILE" 2>/dev/null || true) on $PORT_ADDR"
    return 0
  fi
  mkdir -p "$RUN_DIR"
  rm -f "$PIDFILE"
  ensure_binary || return 1
  if [[ "${PLURIS_SEED:-0}" == "1" ]]; then seed_demo || return 1; fi
  cd "$RUN_DIR"
  if command -v setsid >/dev/null 2>&1; then
    PLURIS_HTTP_ADDR="$PORT_ADDR" setsid "$BIN" >>"$LOG" 2>&1 </dev/null &
  else
    PLURIS_HTTP_ADDR="$PORT_ADDR" nohup "$BIN" >>"$LOG" 2>&1 &
  fi
  local pid=$!
  echo "$pid" > "$PIDFILE"
  if wait_ready; then
    echo "Started: pid $pid at $URL"
    return 0
  fi
  echo "ERROR: server did not become healthy at $HEALTH_URL." >&2
  tail -n 10 "$LOG" >&2
  kill "$pid" 2>/dev/null || true
  rm -f "$PIDFILE"
  return 1
}

stop_now() {
  if ! process_alive; then
    rm -f "$PIDFILE"
    echo "Not running."
    return 0
  fi
  local pid
  pid="$(tr -d '[:space:]' < "$PIDFILE")"
  echo "Sending SIGTERM to pid $pid"
  kill "$pid" 2>/dev/null || true
  local i
  for i in $(seq 1 20); do
    [[ -d "/proc/$pid" ]] || break
    sleep 0.25
  done
  if [[ -d "/proc/$pid" ]]; then
    echo "pid $pid still alive after 5s — sending SIGKILL"
    kill -9 "$pid" 2>/dev/null || true
    sleep 0.5
  fi
  rm -f "$PIDFILE"
  echo "Stopped."
}

status_summary() {
  local health="STOPPED"
  if is_running; then health="RUNNING (HTTP 200)"; fi
  printf 'State:      %s\n' "$health"
  printf 'URL:        %s\n' "$URL"
  if process_alive; then
    printf 'Process:    yes (pid %s)\n' "$(tr -d '[:space:]' < "$PIDFILE")"
  else
    printf 'Process:    no\n'
  fi
  printf 'Database:   %s/pluris.db\n' "$RUN_DIR"
  if [[ -f "$LOG" ]]; then printf 'Last log:   %s\n' "$(tail -n 1 "$LOG")"; fi
  echo
}

msgbox() { # title text
  if have_kdialog; then kdialog --title "$1" --msgbox "$2"; return; fi
  if have_zenity; then zenity --info --title "$1" --text="$2"; return; fi
  echo "$2"
}

msgbox_error() {
  if have_kdialog; then kdialog --title "$1" --error "$2"; return; fi
  if have_zenity; then zenity --error --title "$1" --text="$2"; return; fi
  echo "$2" >&2
}

panel_dialog() {
  local text
  text="$(status_summary)"
  printf 'Pluris test server is %s.\n\n%s\nClosing this window does NOT stop the server.' \
    "$(is_running && echo RUNNING || echo STOPPED)" "$text"
}

start_gui() {
  local out rc code last
  out="$(mktemp)"; rc="$(mktemp)"
  if have_kdialog; then
    kdialog --title "Pluris Test Console" --passivepopup "Starting the Pluris test server... (first run may take a minute to build)" 3 >/dev/null 2>&1 || true
  fi
  ( start_now >"$out" 2>&1; echo "$?" >"$rc" ) &
  wait "$!" || true
  code="$(cat "$rc")"
  last="$(tail -n 6 "$out")"
  rm -f "$out" "$rc"
  if [[ "$code" == "0" ]]; then
    if have_kdialog; then
      kdialog --title "Pluris Test Console" --passivepopup "Server is running: $URL" 6 >/dev/null 2>&1 || true
    else
      msgbox "Pluris Test Console" "Server started.\n\n$(status_summary)"
    fi
  else
    msgbox_error "Pluris Test Console" "Start failed (exit $code).\n\n$last"
  fi
}

stop_gui() {
  local out rc code
  out="$(mktemp)"; rc="$(mktemp)"
  ( stop_now >"$out" 2>&1; echo "$?" >"$rc" ) &
  wait "$!" || true
  code="$(cat "$rc")"
  rm -f "$out" "$rc"
  if [[ "$code" == "0" ]]; then
    msgbox "Pluris Test Console" "Server stopped.\n\n$(status_summary)"
  else
    msgbox_error "Pluris Test Console" "Stop failed (exit $code).\n\n$(cat "$out")"
  fi
}

panel() {
  local choice
  while :; do
    if have_kdialog; then
      choice="$(kdialog --title "Pluris Test Console" --menu "$(panel_dialog)" \
        Start "Start the server" Stop "Stop the server" \
        Status "Refresh / show status" Quit "Close (server keeps running)" 2>/dev/null || true)"
    elif have_zenity; then
      choice="$(zenity --list --title "Pluris Test Console" --text "$(panel_dialog)" \
        --column Action Start Stop Status Quit --hide-header 2>/dev/null || true)"
    else
      echo "No kdialog or zenity; install one to use the GUI panel." >&2
      return 1
    fi
    case "$choice" in
      Start) start_gui ;;
      Stop) stop_gui ;;
      Status) msgbox "Pluris Test Console" "$(status_summary)" ;;
      *) return 0 ;;
    esac
  done
}

text_menu() {
  local c
  while :; do
    status_summary
    echo "  1) start    2) stop    3) status    4) quit"
    printf 'Choose: '
    read -r c || return
    case "$c" in
      1) start_now ;;
      2) stop_now ;;
      3) ;;
      4) return ;;
      *) echo "Unknown: $c" >&2 ;;
    esac
    echo
  done
}

notify_popup() {
  [[ "${PLURIS_NOTIFY:-0}" == "1" ]] || return 0
  if have_kdialog; then
    kdialog --title "Pluris Test Console" --passivepopup "$1" 5 >/dev/null 2>&1 || true
  fi
}

usage() {
  cat <<'EOF'
Pluris Test Console — control the Pluris test server.

Non-GUI (terminal) modes — no dialogs, print status to stdout, set exit code:
  pluris-gui.sh --start    start the server (ok if already running)
  pluris-gui.sh --stop     stop the server (ok if not running)
  pluris-gui.sh --status   print current server status
  pluris-gui.sh --notify   extra flag for --start/--stop: also send a KDE notification

Plain 'start' / 'stop' / 'status' are accepted as shorthand.

GUI mode:
  pluris-gui.sh            open the KDE control panel (no display -> text menu)

Environment:
  PLURIS_HTTP_ADDR  port to bind, e.g. :8080 (default)
  PLURIS_RUN_DIR    scratch dir holding pluris.db, pidfile, log (default ~/.local/state/pluris-test)
  PLURIS_SEED=1     seed demo data before starting
  PLURIS_REBUILD=1  rebuild the console binary before starting
  PLURIS_NOTIFY=1   same as --notify

Exit codes: 0 success, 1 failure.
EOF
}

gcd="text"
if [[ -n "${DISPLAY:-}" || -n "${WAYLAND_DISPLAY:-}" ]]; then
  if have_kdialog || have_zenity; then gcd="gui"; fi
fi

mode=""
for arg in "$@"; do
  case "$arg" in
    --notify) PLURIS_NOTIFY=1 ;;
    --start) mode="start" ;;
    --stop) mode="stop" ;;
    --status) mode="status" ;;
    start) mode="start" ;;
    stop) mode="stop" ;;
    status) mode="status" ;;
    menu) mode="menu" ;;
    --help|-h) usage; exit 0 ;;
    *) ;;
  esac
done
mode="${mode:-menu}"

case "$mode" in
  start)
    if start_now; then code=0; else code=$?; fi
    echo
    status_summary
    notify_popup "Pluris server started: $URL"
    exit "$code" ;;
  stop)
    if stop_now; then code=0; else code=$?; fi
    echo
    status_summary
    notify_popup "Pluris server stopped."
    exit "$code" ;;
  status)
    status_summary
    exit 0 ;;
  menu)
    if [[ "$gcd" == "gui" ]]; then panel; else text_menu; fi ;;
esac