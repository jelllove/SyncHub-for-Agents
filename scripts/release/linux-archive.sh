#!/usr/bin/env bash
set -euo pipefail

root_dir="${1:?Repository root is required}"
app_name="${2:?Application name is required}"
if [[ ! "$app_name" =~ ^[A-Za-z0-9_-]+$ ]]; then
  echo "Invalid application name: $app_name" >&2
  exit 1
fi
root_dir="$(cd "$root_dir" && pwd)"
test -x "$root_dir/bin/$app_name"
for source in "bin/$app_name" LICENSE build/appicon.png "build/linux/$app_name.desktop" docs/install.md; do
  if [[ ! -s "$root_dir/$source" ]]; then
    echo "Missing or empty archive input: $source" >&2
    exit 1
  fi
done

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir "$work_dir/$app_name"
cp -p "$root_dir/bin/$app_name" "$work_dir/$app_name/$app_name"
cp "$root_dir/LICENSE" "$work_dir/$app_name/LICENSE"
cp "$root_dir/build/appicon.png" "$work_dir/$app_name/$app_name.png"
cp "$root_dir/build/linux/$app_name.desktop" "$work_dir/$app_name/$app_name.desktop"
cp "$root_dir/docs/install.md" "$work_dir/$app_name/INSTALL.md"
tar -czf "$work_dir/archive.tar.gz" -C "$work_dir" "$app_name"
mv "$work_dir/archive.tar.gz" "$root_dir/bin/$app_name-linux-x64.tar.gz"
