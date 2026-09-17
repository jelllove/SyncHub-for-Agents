# Security policy

## Report privately

Report suspected vulnerabilities through
[GitHub private vulnerability reporting](https://github.com/jelllove/SyncHub-for-Agents/security/advisories/new).
If private reporting is unavailable, contact a repository maintainer privately
through a contact method they publish. Do not open a public issue containing
exploit details, credentials, private repository contents, or session data.

Include the affected version/commit, operating system, impact, reproduction steps
using synthetic data, and any proposed mitigation. Redact logs and remove secrets
before attaching files. If a credential was exposed, revoke/rotate it immediately;
removing it from the latest file does not remove it from Git history.

## Scope and handling

Security-sensitive areas include credential filtering, portable configuration
projection, path/symlink containment, Git authentication, conflict/deletion safety,
installer approval/execution, and update downloads.

Maintainers should triage reports privately, coordinate a fix and regression
tests, and agree on disclosure timing with the reporter. No response-time SLA or
backport guarantee is promised here. Use the latest stable release; report the
exact affected version even if it is older so impact can be assessed.

Dependency updates and the CI **Security checks** assist review; they are not a
guarantee that all vulnerabilities or secrets are detected. Do not upload private
sync data or source to external scanning services as part of routine maintenance.
Application trash retention is not secure erasure: synchronized Git history may
retain earlier content.
