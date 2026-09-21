# Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org).

SyncHub for Agents publishes Windows installers from the public repository at
`qinqingxu/SyncHub-for-Agents`. Signed artifacts must be built from source code
and release scripts in that repository. The project must not sign unrelated
third-party binaries as SyncHub artifacts.

## Signed artifacts

The project intends to sign these Windows release artifacts:

- `SyncHub.exe`
- `SyncHub-for-Agents-Setup-x64.exe`

Release packages can include unsigned upstream system libraries when required by
Windows or the application framework, but SyncHub's own executables and installer
should be signed.

## Team roles

- Committers and reviewers: maintainers with write access to
  `qinqingxu/SyncHub-for-Agents`.
- Approvers: repository administrators who approve release signing requests.
- Changes from non-committers must be reviewed through pull requests before
  signed release artifacts are produced.

## Release approval

Every signing request must correspond to a GitHub release tag and a successful
release workflow run. Signing approval is a human decision and does not bypass
required CI, code-owner review, or release checks.

## Privacy policy

This program will not transfer any information to other networked systems unless specifically requested by the user or the person installing or operating it.
SyncHub uses a private Git repository selected by the user to synchronize agent
configuration and session resources. Credentials, tokens, machine identifiers,
logs, caches, and local-only state are excluded from synchronization by design.
