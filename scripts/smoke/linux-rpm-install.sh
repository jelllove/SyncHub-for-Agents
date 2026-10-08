#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != Linux || $# != 2 ]]; then
  echo "Usage on Linux: linux-rpm-install.sh RPM_FILE VERSION" >&2
  exit 1
fi
package="$(realpath "$1")"
test -f "$package"
scripts="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
metadata_version="$(node "$scripts/release/linux-evidence.mjs" version "v$2" rpm)"
docker run --rm --mount "type=bind,source=$package,target=/packages/SyncHub.rpm,readonly" \
  fedora:43 bash -euo pipefail -c '
    dnf install --assumeyes /packages/SyncHub.rpm
    test "$(rpm -q --queryformat "%{VERSION}-%{RELEASE}" synchub)" = "$2"
    test "$(rpm -q --queryformat "%{ARCH}" synchub)" = x86_64
    test "$(/usr/bin/SyncHub --version)" = "$1"
    rpm --verify synchub
  ' bash "$2" "$metadata_version"
