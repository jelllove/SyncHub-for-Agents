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
lipo "$binary" -verify_arch "$(uname -m)"
hdiutil verify "$root_dir/bin/SyncHub.dmg"
arch="$(uname -m)"
if [[ "$arch" == "x86_64" ]]; then arch=amd64; fi
archive="${2:-"$root_dir/bin/SyncHub-macos-$arch.zip"}"
test -s "$archive"
ditto -x -k "$archive" "$smoke_home/archive"
codesign --verify --deep --strict "$smoke_home/archive/SyncHub.app"
cmp "$binary" "$smoke_home/archive/SyncHub.app/Contents/MacOS/SyncHub"

HOME="$smoke_home" "$binary" --hidden &
pid=$!
sleep 5
kill -0 "$pid"

echo "macOS desktop smoke test passed"
