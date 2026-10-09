---
name: synchub-release-authoring
description: Author and verify a complete SyncHub release across Windows, macOS and Linux. Use before preparing release tags, publishing or supplementing GitHub releases, changing packaging, or mirroring release assets; never declare a Windows-only release complete.
---

# SyncHub cross-platform release authoring

## When to use

Read this skill before any release-authoring or release-packaging work, including
AI-assisted changes that affect the release matrix. Ordinary code changes do not
authorize a release. Create tags, publish releases or mirror assets only when the
maintainer explicitly requests those operations.

Read the [validation skill](../synchub-validation/SKILL.md),
[development guide](../../../docs/development.md#native-desktop-packages),
[installation matrix](../../../docs/install.md#package-matrix-and-validation-scope)
and the current workflows before choosing a publication route. Workflow files,
not historical release notes, determine the commands and inputs available today.

## Required release matrix

Every completed desktop release must attach packages for all three OS families
to the **same release tag**, built from the **same immutable source commit**:

| Platform | Required CPU coverage | Required assets |
| --- | --- | --- |
| Windows | x64 / amd64 | `SyncHub-for-Agents-Setup-x64.exe` and `SyncHub-for-Agents-Windows-x64.zip` |
| macOS | Apple Silicon ARM64 and Intel x64 | Universal `.dmg` and `.zip`, each containing both CPU slices |
| Linux / Ubuntu | x64 / amd64 | `SyncHub.deb`, `SyncHub-x86_64.AppImage` and `SyncHub-linux-x64.tar.gz` |
| Linux / RPM | x64 / amd64 | `SyncHub.rpm` when produced by the current packaging workflow |

There are seven mandatory package files; the current workflows also produce RPM,
making eight. Preserve RPM coverage unless the maintainer explicitly changes that
scope; never silently drop it because its validation failed. Windows ARM64 and
Linux ARM64 are not required by the current matrix.

For macOS, use either the Developer ID/notarized pair (`SyncHub.dmg` and
`SyncHub-macos-universal.zip`) or the explicitly approved ad-hoc pair
(`SyncHub-macOS-universal-adhoc.dmg` and `SyncHub-macOS-universal-adhoc.zip`).
An ad-hoc build is not notarized. If signing credentials are unavailable, report
the blocker and obtain approval for the labeled ad-hoc route; do not silently
substitute it or claim Gatekeeper acceptance.

Also attach a final `SHA256SUMS.txt` covering **every package actually attached**.
Retain `SHA256SUMS-macOS.txt` and `SHA256SUMS-Linux.txt` when supplementary
workflows produce them. Checksums detect corruption, not publisher identity.
A workflow artifact, source archive or NPM wrapper does not replace an OS package
attached to the GitHub release.

## 1. Establish authorization, source and identity

- Record the destination repository, branch, version/tag and exact commit.
  Verify that the intended changes are committed and included in that commit.
  A dirty worktree is not part of a release built from its tag.
- Inspect Git status and preserve unrelated work. Stage only authorized changes;
  do not bundle other pending desktop features into a skill/documentation commit.
- Before GitHub operations, inspect `git remote get-url origin` (or the existing
  GitHub remote if origin is absent). Switch `gh` authentication and verify it
  with `gh api --hostname <host> user --jq .login`: use `jelllove` for
  `github.com/jelllove`, `qinqingxu` for `github.com/microsoft` or
  `github.com/microosft`, and `qinqiangxu` for `msft.ghe.com`.
  For unmapped owners, ask which account to use rather than guessing.
- Inspect current release variables, environment approvals and signing-secret
  availability without printing secret values. Do not change global Git/npm
  configuration, fetch credentials from unrelated profiles, or bypass approvals.
- Before uploading and again before final handoff, resolve the live tag to its
  commit, including annotated tags. Reject missing/moved tags or evidence from a
  different commit. Never force-move a published tag to fix a package.

## 2. Validate and choose the existing workflow route

On a fresh checkout run `node scripts/dev.mjs setup`. For application,
packaging, workflow or validation-script changes run
`node scripts/dev.mjs verify` and retain the actual report. Documentation-only
changes require `node scripts/dev.mjs docs`; neither command proves native
installation on another OS.

Use these existing routes instead of inventing parallel packaging logic:

| Workflow | Trigger and role | Completion limitation |
| --- | --- | --- |
| [Release](../../../.github/workflows/release.yml) | Authorized `v*` tag push builds Windows. `RELEASE_MACOS=true` enables signed/notarized Universal macOS; `RELEASE_LINUX=true` enables Linux packaging. | Publish accepts skipped macOS/Linux jobs. A green run can still be Windows-only. |
| [macOS installer validation](../../../.github/workflows/macos-installer.yml) | Dispatch with explicit `release_tag`; builds the tagged app and tests the same DMG/ZIP on ARM64 and Intel. `publish=true` uploads only after both native jobs pass. | Supplementary publishing requires an existing published release and explicit approval for ad-hoc assets. |
| [Linux installer validation](../../../.github/workflows/linux-installer.yml) | Dispatch with explicit `release_tag`; verifies Ubuntu startup, archives and Fedora RPM installation. | Produces artifacts only; it does not attach them to the release automatically. |

Do not rely on the installer workflows' historical default tag. Specify the
intended tag every time. Their tooling ref and tagged application source are
separate; use a ref containing the required workflow/tooling and verify the
receipt's application source commit.

For example, after release authorization, with `$Tag` set to the intended tag
and `$ToolingRef` set to the reviewed tooling branch, PowerShell dispatches are:

```powershell
gh workflow run macos-installer.yml --ref $ToolingRef -f release_tag=$Tag -f publish=false
gh workflow run linux-installer.yml --ref $ToolingRef -f release_tag=$Tag
```

Record and wait for those exact run IDs, not another run for the same workflow.
The macOS `publish=false` run is inspection only. To publish through its existing
guarded publisher, explicitly dispatch with `publish=true` and wait for that
run's build, both native jobs and publish job. Do not upload a previous run's
untested bytes manually. For Linux, download `linux-verified-package` from the
successful run and follow the source/digest guards below before uploading.

The tag workflow can publish Windows before supplementary jobs finish. Treat
that release as **incomplete**, disclose the missing assets, and do not announce
cross-platform completion until all gates pass. This skill is an agent procedure,
not a claim that CI currently enforces atomic all-platform publication.

## 3. Require platform evidence for this version

- **Windows:** require the native Windows release build and its packaging/updater
  tests. Verify installer/ZIP versions, x64 architecture and the ZIP's executable
  provenance. Disclose unsigned builds; do not imply SmartScreen approval.
- **macOS:** verify Universal ARM64 and Intel slices and the signing status.
  Require native installed-app validation on both macOS 15 ARM64 and Intel hosts.
  Reuse the existing isolated DMG installation and XCTest onboarding tests, and
  verify that ZIP app contents match the tested DMG. Signing/notarization logs
  alone are not installed-app UI evidence. Native results for ad-hoc packages
  do not validate different signed/notarized bytes; that route needs installed-app
  evidence for its own final packages before completion.
- **Linux:** require Ubuntu 24.04 x64 native startup checks for the extracted
  packages, correct version/architecture and tar.gz executable/content equality
  with the verified DEB. If publishing RPM, also require the Fedora 43 container
  dependency-resolution, installation and executable-version job.
- Use isolated synthetic homes, Git configuration and profiles, as the existing
  smoke scripts do. Never test synchronization, startup registration or installers
  against real user homes, credentials or sync repositories.

Report exact scope: Ubuntu checks are extracted-package native startup under
Xvfb/private D-Bus with a scoped test AppArmor allowance, not clean-OS apt
installation or unrestricted desktop certification. Fedora checks are container
RPM installation, not Fedora graphical UI. macOS checks do not establish
Gatekeeper acceptance or support on every macOS version. Do not claim Ubuntu
22.04, other Debian/Ubuntu versions, RHEL, AlmaLinux or Rocky Linux as verified
without new native evidence for this release.

A skipped, cancelled, failed or missing required job is a blocker, not a pass.
Preserve failing run links and artifacts; never substitute an earlier release's
results, synthetic packaging tests or browser screenshots for native evidence.

## 4. Attach and independently verify final bytes

1. Inspect the destination release's existing assets before any upload. Refuse
   duplicate names or unapproved replacement. Preserve other platforms and notes;
   never use blanket clobber/delete operations to resolve conflicts.
2. Download packages from the exact successful run. Check its source receipt
   against the live release tag, verify every size/digest and required filename,
   and reject partial or unsafe manifests. Reuse
   [macOS evidence guards](../../../scripts/release/macos-evidence.mjs) and
   [Linux evidence guards](../../../scripts/release/linux-evidence.mjs).
3. Upload only verified package bytes and their platform manifests. After adding
   supplementary packages, generate a combined `SHA256SUMS.txt` from the final
   package set, not the old Windows-only manifest. Do not hash checksum files
   into themselves. If the combined manifest already exists, inspect and replace
   only that manifest as part of the authorized release operation.
4. Independently list and download the assets from the public release into an
   isolated directory. Confirm the required OS/CPU/format matrix, nonempty files,
   version/source consistency and all final SHA-256 values. Local artifacts or
   upload exit status alone are not completion evidence.
5. Inspect final release notes. Include a platform/CPU/package table, signing and
   notarization status, exact validated distributions/test scope, prerequisites,
   checksum instructions and hosted run links. Distinguish actual tested results
   from intended support. Follow [installation guidance](../../../docs/install.md).
6. If mirroring is explicitly requested, verify access and tag/commit alignment
   separately for each repository. Copy the already verified bytes, manifests
   and notes rather than rebuilding; download both destinations and compare
   hashes. Do not silently choose a similarly spelled GitHub account/repository.

## Completion gate and handoff

Before saying "released", confirm all of the following:

- [ ] Authorized tag resolves to the intended committed source at every destination.
- [ ] Windows x64 EXE and ZIP are attached and verified.
- [ ] macOS Universal DMG and ZIP cover ARM64 and Intel and have truthful signing labels.
- [ ] Linux x64 DEB, AppImage and tar.gz are attached and verified; RPM is included
      and validated when produced by the selected workflow.
- [ ] Required native runs for this exact version passed; links and scope are recorded.
- [ ] Downloaded final package bytes match the complete `SHA256SUMS.txt`.
- [ ] Release notes and any authorized mirror reflect those exact assets/results.

**Correct:** report the release URL, source commit, complete asset table,
successful native run links and downloaded-asset hash verification, with signing
and distribution limitations.

**Incorrect:** call a green Windows-only tag workflow a completed cross-platform
release, attach CI artifacts without verifying them, use packages from different
commits under one tag, or call the NPM launcher a substitute for DMG/DEB packages.

If any required package or evidence is missing, report the release as
**blocked/incomplete**, name the missing item and failing command/run, and keep
working only within the authorized scope. Never waive an OS silently.
