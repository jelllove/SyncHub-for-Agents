#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
appimage="${1:-"$root_dir/bin/SyncHub-x86_64.AppImage"}"
deb="${2:-$(find "$root_dir/bin" -maxdepth 1 -name '*.deb' -print -quit)}"
work_dir="$(mktemp -d)"
pid=""

cleanup() {
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid"
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$work_dir"
}
trap cleanup EXIT

test -x "$appimage"
test -n "$deb"
test -f "$deb"
dpkg-deb --info "$deb" >/dev/null
dpkg-deb --contents "$deb" > "$work_dir/deb-contents.txt"
grep -q 'usr/bin/SyncHub' "$work_dir/deb-contents.txt"

(
  cd "$work_dir"
  "$appimage" --appimage-extract >/dev/null
)
test -x "$work_dir/squashfs-root/usr/bin/SyncHub"

HOME="$work_dir/home" XDG_CONFIG_HOME="$work_dir/config" \
  xvfb-run -a "$work_dir/squashfs-root/AppRun" --hidden &
pid=$!
sleep 5
kill -0 "$pid"

echo "Linux package smoke test passed"
