#!/bin/bash
set -euo pipefail

if [[ "$(uname -s)" != Darwin || $# != 3 ]]; then
  echo "Usage on macOS: macos-adhoc.sh SOURCE_DIRECTORY OUTPUT_DIRECTORY RELEASE_TAG" >&2
  exit 1
fi
tools="$(cd "$(dirname "$0")" && pwd)"
source="$(cd "$1" && pwd)"
output="$2"
tag="$3"
version="$(node "$tools/macos-evidence.mjs" version "$tag")"
if [[ -e "$output" ]]; then
  echo "Refusing to reuse packaging output: $output" >&2
  exit 1
fi
mkdir -p "$output"
output="$(cd "$output" && pwd)"
cd "$source"
source_commit="$(git rev-parse HEAD)"
tag_commit="$(git rev-parse "refs/tags/$tag^{commit}")"
[[ "$source_commit" == "$tag_commit" ]]
[[ -z "$(git status --porcelain)" ]]
before="$(shasum -a 256 go.mod go.sum frontend/package.json frontend/package-lock.json)"
npm ci --prefix frontend
npm --prefix frontend run build
app="$output/SyncHub.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp build/darwin/Info.plist "$app/Contents/Info.plist"
cp build/darwin/icons.icns "$app/Contents/Resources/icons.icns"
for key in CFBundleVersion CFBundleShortVersionString; do
  /usr/libexec/PlistBuddy -c "Set :$key $version" "$app/Contents/Info.plist"
done
for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=1 \
    CGO_CFLAGS="-mmacosx-version-min=12.0" \
    CGO_LDFLAGS="-mmacosx-version-min=12.0" \
    MACOSX_DEPLOYMENT_TARGET=12.0 \
    go build -mod=readonly -tags production -trimpath -buildvcs=false \
      -ldflags "-w -s -X github.com/qinqingxu/synchub-for-agents/internal/appversion.Version=$version" \
      -o "$output/SyncHub-$arch" .
done
lipo -create "$output/SyncHub-arm64" "$output/SyncHub-amd64" -output "$app/Contents/MacOS/SyncHub"
lipo -verify_arch arm64 x86_64 "$app/Contents/MacOS/SyncHub"
codesign --force --deep --sign - "$app"
codesign --verify --deep --strict --verbose=2 "$app"
codesign --display --verbose=4 "$app" 2> "$output/signature.txt"
grep -q 'Signature=adhoc' "$output/signature.txt"
[[ "$("$app/Contents/MacOS/SyncHub" --version)" == "$version" ]]
[[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$app/Contents/Info.plist")" == "$version" ]]
mkdir "$output/dmg-root"
ditto "$app" "$output/dmg-root/SyncHub.app"
ln -s /Applications "$output/dmg-root/Applications"
dmg="SyncHub-macOS-universal-adhoc.dmg"
hdiutil create -volname "SyncHub $version (ad-hoc)" -srcfolder "$output/dmg-root" \
  -format UDZO "$output/$dmg"
hdiutil verify "$output/$dmg"
[[ "$before" == "$(shasum -a 256 go.mod go.sum frontend/package.json frontend/package-lock.json)" ]]
cd "$output"
shasum -a 256 "$dmg" > SHA256SUMS-macOS.txt
digest="$(shasum -a 256 "$dmg" | awk '{print $1}')"
jq -n --arg tag "$tag" --arg commit "$source_commit" --arg digest "$digest" \
  '{tag:$tag, sourceCommit:$commit, dmgSHA256:$digest, signature:"ad-hoc", notarized:false, architectures:["arm64","x86_64"]}' \
  > build-receipt.json
echo "Built $output/$dmg. AD-HOC signed, NOT notarized; Gatekeeper may block first launch."
