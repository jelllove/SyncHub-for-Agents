# NPM installer architecture

## System boundaries

The NPM package is an installation and process-launch adapter for existing
desktop release binaries. It does not implement synchronization or the Wails UI.
The public NPM registry supplies JavaScript; official GitHub release URLs supply
the pinned native archive. TLS and the shipped archive hashes are trust boundaries.

## Module responsibilities

- `release.json` pins desktop version, archive names, sizes and SHA-256 values.
- `lib/platform.mjs` validates the manifest and selects supported OS/CPU pairs.
- `lib/download.mjs` performs bounded, credential-free HTTPS downloads.
- `lib/extract.mjs` extracts verified ZIP/tar archives into an isolated staging tree.
- `lib/install.mjs` owns staging, locking, integrity receipts and installation.
- `lib/cli.mjs` validates arguments and preserves native child-process results.
- `bin/synchub-for-agents.mjs` validates CLI usage and launches the native process.
- `scripts/install.mjs` runs the download-only NPM lifecycle entry point.
- `scripts/check-docs.mjs` validates persistent delivery documentation.
- `test/` covers installation, extraction, launch behavior and documentation gates.

## Data and state flow

Platform selection resolves the pinned archive. A package-local staging directory
receives the download, then verification and extraction. Only regular files and
directories under the selected payload root are accepted. The executable is
identified inside the extracted tree and hashed into an installation receipt.
A completed staging payload is renamed to `native`; no partial tree is launched.
Later commands verify the receipt and executable before starting the application.
All package-owned state is separate from the desktop's user settings and sync data.

## Technology decisions

Use Node 24.13 or later with ESM and `node:test`, matching the repository's
toolchain. Pin `yauzl` 3.4.0 for ZIP reading and `tar` 7.5.22 for tar extraction;
use additional path/type/size checks rather than trusting extraction defaults.
Use built-in cryptography, filesystem, streaming and child-process APIs.
Archive hashes are shipped in the NPM tarball, not fetched as mutable installation
policy. macOS uses the existing Universal ZIP; Windows uses its portable ZIP;
Linux uses its native tar.gz and keeps GTK4/WebKitGTK as system prerequisites.

## Error handling

Return contextual exceptions to lifecycle/CLI entry points and exit nonzero.
Network retries are limited to transient failures, retain HTTPS validation and
never fall back to mirrors or source compilation. Reject missing or malformed
receipts and corrupt executables. Cleanup only specifically created package-local
staging paths, never agent homes, sync repositories or broad filesystem roots.
Keep authentication and publication errors separate from local package validation.

## Execution

In a source checkout, run `npm ci --ignore-scripts`, then `npm test`.
Run `npm pack` to validate and create the distributable archive. Install its
tarball into a synthetic prefix for smoke testing. Official publication uses
`npm publish --registry=https://registry.npmjs.org/ --access public` with the
user's authenticated account; the machine's internal default registry is not used.
No login token is copied into package files or test fixtures.

## Validation

`npm test` runs Node unit/integration tests plus the deterministic documentation
check. Documentation failures use isolated temporary trees rather than modifying
the project's documents. Archive fixtures include rejected paths and links;
transport/process injection keeps normal tests offline and free of user data.
Repository setup restores locked installer dependencies with lifecycle scripts
disabled, and repository verification includes the package test command.
Real archive smoke testing only extracts under ignored fixtures and runs
`--version`, never the desktop GUI or synchronization.
