# NPM installer project guidance

## Project documents

This package's specification is [docs/spec.md](docs/spec.md); its architecture is
[docs/arch.md](docs/arch.md). Keep both aligned with actual modules and behavior.
The repository's general guidance also applies.

## Implementation requirements

Keep archive/version selection pinned and explicit. Reject unsafe paths, links,
oversized content and checksum failures. Never disable TLS verification, invoke
an OS installer, launch during postinstall, edit user startup/update settings, or
run application synchronization as a test. Use synthetic package prefixes,
temporary local archives and injected transport/process boundaries.

Only publish the documented files whitelist to the public NPM registry.
Never package credentials, `.npmrc`, native download caches, logs or user data.
Do not create GitHub tags/releases or change the user's registry configuration
as part of NPM publication. Require real official-registry authentication.

## Completion requirements

Run `npm test`, inspect `npm pack --dry-run --ignore-scripts --json`, and run
`npm pack` before publishing. The ordinary test command includes the deterministic
documentation check; do not bypass it or weaken it to accept unfinished sections.
Test documentation failures only in isolated fixtures.
Verify the packed artifact in a synthetic local NPM installation using only the
native version query. After publishing, inspect the public version/integrity and
verify a fresh install when possible. If authentication is blocked, report that
publication is incomplete and retain the verified local tarball.
