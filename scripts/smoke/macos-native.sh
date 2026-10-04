#!/bin/bash
set -euo pipefail

if [[ "$(uname -s)" != Darwin || $# != 3 ]]; then
  echo "Usage on macOS: macos-native.sh PACKAGE_DIRECTORY EVIDENCE_DIRECTORY RELEASE_TAG" >&2
  exit 1
fi
tools="$(cd "$(dirname "$0")/../release" && pwd)"
project="$(cd "$(dirname "$0")/macos-ui" && pwd)"
package="$(cd "$1" && pwd)"
evidence="$2"
tag="$3"
version="$(node "$tools/macos-evidence.mjs" version "$tag")"
[[ ! -e "$evidence" ]]
mkdir -p "$evidence/screenshots" "$evidence/profile" "$evidence/Applications" "$evidence/mount"
evidence="$(cd "$evidence" && pwd)"
dmg="$package/SyncHub-macOS-universal-adhoc.dmg"
cd "$package"
shasum -a 256 -c SHA256SUMS-macOS.txt
hdiutil verify "$dmg"
mounted=false
cleanup() {
  if [[ "$mounted" == true ]]; then
    hdiutil detach "$evidence/mount"
  fi
}
trap cleanup EXIT
hdiutil attach "$dmg" -readonly -nobrowse -mountpoint "$evidence/mount"
mounted=true
ditto "$evidence/mount/SyncHub.app" "$evidence/Applications/SyncHub.app"
app="$evidence/Applications/SyncHub.app"
codesign --verify --deep --strict --verbose=2 "$app"
codesign --display --verbose=4 "$app" 2> "$evidence/signature.txt"
grep -q 'Signature=adhoc' "$evidence/signature.txt"
[[ "$("$app/Contents/MacOS/SyncHub" --version)" == "$version" ]]
[[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$app/Contents/Info.plist")" == "$version" ]]
lipo -verify_arch arm64 x86_64 "$app/Contents/MacOS/SyncHub"
hdiutil detach "$evidence/mount"
mounted=false
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$app"
architecture="$(uname -m)"
digest="$(shasum -a 256 "$dmg" | awk '{print $1}')"
jq -n --arg version "$version" --arg architecture "$architecture" --arg digest "$digest" \
  '{version:$version,architecture:$architecture,originalDMGDigest:$digest,installedFromDMG:true,
    adHocSignatureVerified:true,executableVersionVerified:true,isolatedHome:true}' > "$evidence/installation.json"
set +e
xcodebuild test -project "$project/NativeSmoke.xcodeproj" -scheme NativeSmoke \
  -destination 'platform=macOS' -derivedDataPath "$evidence/DerivedData" \
  -resultBundlePath "$evidence/native.xcresult" \
  CODE_SIGN_IDENTITY=- CODE_SIGNING_ALLOWED=YES \
  SMOKE_APP_PATH="$app" SMOKE_EVIDENCE_DIR="$evidence" \
  > "$evidence/xcodebuild.log" 2>&1
status=$?
set -e
if [[ -d "$evidence/native.xcresult" ]]; then
  xcrun xcresulttool get test-results summary --path "$evidence/native.xcresult" \
    --format json > "$evidence/test-summary.json"
fi
cat "$evidence/xcodebuild.log"
if [[ "$status" != 0 ]]; then
  echo "Native installed-app UI tests failed; preserving xcresult, screenshots, and logs." >&2
  exit "$status"
fi
node "$tools/macos-evidence.mjs" verify "$evidence" "$tag" "$architecture"
