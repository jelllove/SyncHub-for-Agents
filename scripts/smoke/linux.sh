#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
appimage="${1:-"$root_dir/bin/SyncHub-x86_64.AppImage"}"
deb="${2:-$(find "$root_dir/bin" -maxdepth 1 -name '*.deb' -print -quit)}"
rpm_package="${3:-"$root_dir/bin/SyncHub.rpm"}"
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
mkdir "$work_dir/deb"
dpkg-deb --extract "$deb" "$work_dir/deb"
test -f "$rpm_package"
test "$(rpm -qp --queryformat '%{NAME}' "$rpm_package")" = synchub
rpm -qpl "$rpm_package" > "$work_dir/rpm-contents.txt"
grep -q '^/usr/bin/SyncHub$' "$work_dir/rpm-contents.txt"
mkdir "$work_dir/rpm"
bsdtar --extract --file "$rpm_package" --directory "$work_dir/rpm" --no-same-owner
test -x "$work_dir/rpm/usr/bin/SyncHub"
rpm_version="$("$work_dir/rpm/usr/bin/SyncHub" --version)"
deb_version="$("$work_dir/deb/usr/bin/SyncHub" --version)"
test -n "$rpm_version"
test "$rpm_version" = "$deb_version"

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
