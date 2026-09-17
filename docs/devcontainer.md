# Linux development container

The [devcontainer configuration](../.devcontainer/devcontainer.json) provides
Ubuntu 24.04, the exact Go and Node versions declared by
[go.mod](../go.mod) and [.node-version](../.node-version), npm, Git, and Linux
native build dependencies. It adds a repeatable development/check environment;
it does not change application behavior or establish a readiness score.

## Open for interactive development

1. Install Docker Desktop with Linux containers (or Docker Engine on Linux) and
   the VS Code Dev Containers extension. `docker info --format '{{.OSType}}'`
   must report `linux`. Allow several GiB for image layers, modules, and builds.
2. Open the repository root, not its parent, in VS Code. Use **Dev Containers:
   Reopen in Container**, then wait for the setup lifecycle command to finish.
3. The existing `node scripts/dev.mjs setup` downloads Go modules, runs locked
   `npm ci`, and builds embedded frontend assets. Failures remain failures; fix
   connectivity or the reported prerequisite and rerun that same command.

Registry access is required for setup. If npm reports a TLS handshake error,
check access to that same public registry URL from the host as well as the
container. Do not disable TLS/certificate verification or alter lockfiles to
turn a network failure into a passing setup.

The interactive workspace is the **selected repository bind mount**, writable
so edits and setup outputs persist. It is not an isolated test checkout. Do not
run automated validation against that mount when the host tree must remain
untouched; use the read-only-copy procedure below instead. Do not share
`frontend/node_modules` between Windows and Linux; `setup` installs the current
platform's locked dependencies. A dedicated checkout or WSL checkout avoids
cross-platform dependency churn. Windows-linked worktrees whose `.git` file
points outside the mount are not portable into Linux; use a standalone checkout
rather than mounting additional host directories to repair that link.

Container processes, terminals, and setup run as `vscode`, not root. On Linux,
Dev Containers can align that user's UID/GID with the workspace owner. All
capabilities are dropped and privilege escalation is disabled; change native
dependencies in the Dockerfile and rebuild instead of using `sudo` in a running
container. There is no Docker socket, host networking, privileged mode, host home,
credential, display, or device mount in this configuration.

Review client-level Dev Containers settings as well: clients can independently
copy Git configuration or forward credentials, SSH agents, and display sockets.
Disable such optional sharing for isolated use; this configuration does not need
it. Setup never changes shared Git configuration or installs global tools,
Wails, or agent skills.

## Reproducible inputs

The [Dockerfile](../.devcontainer/Dockerfile) copies toolchains from official
versioned images into the Ubuntu devcontainer base. These are immutable
multi-platform **index** identities, resolved on 2026-09-17:

| Input | Pinned image |
| --- | --- |
| Go 1.26.6 | `docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36` |
| Node 24.17.0, including npm/npx | `docker.io/library/node:24.17.0-bookworm-slim@sha256:862263c612aa437e3037674b85419622a9d93bff80aa1eee5398dfe686375532` |
| Ubuntu 24.04 developer base | `mcr.microsoft.com/devcontainers/base:ubuntu-24.04@sha256:d94c97dd9cacf183d0a6fd12a8e87b526e9e928307674ae9c94139139c0c6eae` |

The image manifests advertise Linux amd64 and arm64. A successful build on one
architecture does not validate the other. Go's automatic toolchain download is
disabled with `GOTOOLCHAIN=local`, so a changed Go requirement fails visibly
instead of silently selecting a different compiler. npm comes from the pinned
Node image; project dependencies retain their existing lockfile/checksum checks.

[Ubuntu package sources](../.devcontainer/ubuntu.sources) use the dated
`20260917T000000Z` snapshot for `noble`, `noble-updates`, and `noble-security`,
with Ubuntu archive signing keys. Neither signature nor freshness verification
is disabled, and any package-index download error fails the build rather than
silently using incomplete indexes. The explicit packages are `gcc`, `pkg-config`, `libgtk-4-dev`,
`libwebkitgtk-6.0-dev`, `libfuse2`, `xvfb`, and `xauth`. The last package is needed
by `xvfb-run` when recommendations are not installed. Ubuntu 24.04 resolves the
`libfuse2` request to `libfuse2t64`; the build checks the actual `libfuse.so.2`
library, not an empty dpkg record for the old package name.

These pins stabilize the base, toolchain, and native package inputs; they do not
promise bit-for-bit application artifacts or permanent network availability.
Frozen images/snapshots do not acquire future security fixes automatically.
Refresh them deliberately, verify the new identities with
`docker buildx imagetools inspect <versioned-image>`, update the Dockerfile's
version assertions and this table, and rerun configuration tests and a real build.
Coordinate Go/Node pin changes with the owners of the root version files.

Build context is **only** the [.devcontainer directory](../.devcontainer).
Its [local ignore file](../.devcontainer/.dockerignore) additionally allows only
the Dockerfile, package source definition, and ignore file. Source code, Git
metadata, installed skills, credentials, and dependency directories are not
uploaded during the image build. No root ignore-file change is needed.
Allow a sufficiently long build timeout: a cold download from the signed
snapshot can exceed ten minutes on a slow connection.

## Build and check the environment without mounting source

From PowerShell, use an image tag and container names reserved for your work:

```powershell
docker build --progress=plain --tag synchub-devcontainer:local C:\XQQ\SyncHub-for-Agents\.devcontainer
docker run --rm --name synchub-devcontainer-tools --network none `
  --cap-drop=ALL --security-opt=no-new-privileges --user vscode `
  synchub-devcontainer:local bash -c 'set -euo pipefail; id; go version; node --version; npm --version; gcc --version; pkg-config --modversion gtk4 webkitgtk-6.0; ldconfig -p | grep -F libfuse.so.2; xvfb-run -a printenv DISPLAY'
```

The image build itself asserts the exact Go/Node versions as the non-root user,
resolves native pkg-config modules, and checks npm/npx and headless prerequisites.
The runtime probe uses no mounts and no network. Do not use the repository root
as an alternative build context.

The focused configuration regressions use Go's standard library and join the
existing repository checks, with no new test dependency:

```text
go test ./tools/repocheck -run '^TestDevcontainer' -count=1
```

They check version-file agreement, digest pinning, native prerequisites and signed
snapshots, build-context isolation, the non-root configuration, and the exact
shared setup command. They do not substitute for actually building the image.

## Validate application checks in an isolated copy

For validation, mount **only the repository, read-only**, at `/source`. Copy
Git-listed tracked and non-ignored pending files into the container's temporary
workspace. This includes current changes without creating a commit; it excludes
Git metadata, ignored dependencies/artifacts, and installed `.agents` skills.
Use a stable, trusted checkout without concurrent edits or tracked deletions for
the example below. The archive copy fails on missing inputs rather than silently
claiming a complete snapshot. Source symlinks are not dereferenced by `tar`.

```powershell
$checks = @'
set -euo pipefail
workspace=$(mktemp -d /tmp/synchub-devcontainer.XXXXXXXX)
mkdir "$workspace/repo"
git -c safe.directory=/source -C /source ls-files --deduplicate -z --cached --others --exclude-standard -- . ":(exclude).agents" > "$workspace/files"
tar -C /source --null --verbatim-files-from -T "$workspace/files" -cf "$workspace/source.tar"
tar -C "$workspace/repo" --no-same-owner -xf "$workspace/source.tar"
cd "$workspace/repo"
git init --quiet
node scripts/dev.mjs setup
go test ./tools/repocheck -run "^TestDevcontainer" -count=1
node scripts/dev.mjs check
xvfb-run -a go test ./... -count=1
npm --prefix frontend run test:lint
npm --prefix frontend test
'@
docker run --rm --name synchub-devcontainer-check --user vscode `
  --cap-drop=ALL --security-opt=no-new-privileges `
  --pids-limit=512 --memory=6g --cpus=2 `
  --mount 'type=bind,source=C:\XQQ\SyncHub-for-Agents,target=/source,readonly' `
  --workdir /tmp synchub-devcontainer:local bash -c $checks.Replace("`r", "")
```

Only the command-local Git `safe.directory` override applies to the read-only
source. `git init` creates metadata **inside the temporary copy**, with no commit,
host Git configuration change, or copied credential configuration. Setup, caches,
test fixtures, and build outputs remain inside the container and disappear with
`--rm`. The copy does not preserve commit provenance, so these individual checks
are not a `dev.mjs verify` source/revision attestation or a hosted CI result.
Dependency downloads need ordinary outbound network access, not host networking.

Each failed command stops the example and returns a nonzero exit status. Preserve
the terminal output and report failures honestly. Do not run installer smoke
scripts, sync/cleanup commands, or packaging/install tasks against the host user,
and never change the source mount to writable just to make a check pass.

## Headless and platform limits

- Xvfb provides a disposable X display for tests, not a full desktop session.
  GUI startup, tray integration, a desktop keyring/session bus, interactive
  rendering, and real user synchronization still need appropriate native
  platform testing with synthetic data.
- `libfuse2` supplies a userspace library, not access to `/dev/fuse`. This image
  does not grant FUSE devices or capabilities and does not validate AppImage
  mounting, installers, or release packaging.
- A Linux build does not test Windows WebView2/NSIS, macOS frameworks/signing,
  mobile targets, or another CPU architecture.
- Wails CLI installation is optional and remains an explicit developer action;
  use the version and instructions in the
  [development reference](development.md). It is not installed by post-create.
- No server is started or port published automatically. For a frontend-only
  preview, run the existing frontend dev command and use editor port forwarding;
  that alone does not exercise desktop bindings.

All example probe containers use `--rm`. If a run is interrupted, inspect and
remove only its exact named container. The image and BuildKit cache remain
reusable; remove your chosen tag with `docker image rm synchub-devcontainer:local`
only when no longer needed. Never use system/container/image prune or broad
process cleanup. See [contribution guidance](../CONTRIBUTING.md) for the normal
validation and handoff contract.
