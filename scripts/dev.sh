#!/usr/bin/env bash
# SPEC-01 dev loop (POSIX counterpart of scripts/dev.ps1): builds the daemon
# binary, starts it in the background, then runs `wails dev` (the GUI). The
# daemon is stopped when wails dev exits.
set -e

root="$(cd -- "$(dirname -- "$0")/.." && pwd)"
bin_dir="$root/build"
bin="$bin_dir/heka-dev"
log_out="$root/.heka-dev-daemon.log"
log_err="$root/.heka-dev-daemon.log.err"

mkdir -p "$bin_dir"

if ! command -v wails >/dev/null 2>&1; then
    go_bin="$(go env GOPATH)/bin"
    if [ -x "$go_bin/wails" ]; then
        export PATH="$go_bin:$PATH"
    else
        echo "wails CLI not found. Install it with: go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
        exit 1
    fi
fi

# main.go embeds all:frontend/dist, so it must exist before `go build`.
if [ ! -d "$root/frontend/dist" ]; then
    echo "Building frontend (frontend/dist missing)..."
    (cd "$root/frontend" && npm run build)
fi

echo "Building daemon binary..."
(cd "$root" && go build -o "$bin" .)

echo "Starting Heka daemon (logs: $log_out)..."
"$bin" daemon >"$log_out" 2>"$log_err" &
daemon_pid=$!
trap 'if kill -0 "$daemon_pid" 2>/dev/null; then kill "$daemon_pid" 2>/dev/null; echo "Daemon stopped."; fi' EXIT
echo "Daemon pid $daemon_pid started."

cd "$root"
wails dev
