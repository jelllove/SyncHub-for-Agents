#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
app_path="${1:-"$root_dir/bin/SyncHub.app"}"
binary="$app_path/Contents/MacOS/SyncHub"
smoke_home="$(mktemp -d)"
pid=""

cleanup() {
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid"
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$smoke_home"
}
trap cleanup EXIT

test -x "$binary"
plutil -lint "$app_path/Contents/Info.plist"
codesign --verify --deep --strict "$app_path"

HOME="$smoke_home" "$binary" --hidden &
pid=$!
sleep 5
kill -0 "$pid"

echo "macOS desktop smoke test passed"
