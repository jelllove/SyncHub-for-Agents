# GitHub SSH Host Alias Support

## Goal

Accept GitHub SSH repository URLs that use an OpenSSH host alias, such as
`git@github-jelllove:jelllove/agents-backup.git`, so users can select different
keys and other SSH options from their local `~/.ssh/config`.

## Scope

- Accept SCP-style and `ssh://` GitHub URLs whose SSH host is a valid local
  alias rather than the literal `github.com`.
- Preserve the alias in the stored and cloned repository URL.
- Resolve the alias with `ssh -G` during SSH verification.
- Require the effective SSH hostname to be `github.com` and the effective user
  to be `git`.
- Use alias-specific identity files and other SSH configuration when checking
  repository access.
- Document the supported alias form in onboarding and installation guidance.

HTTPS URLs remain restricted to `github.com`. Supporting self-hosted Git
services is outside this change.

## URL Parsing

`ParseGitHubURL` will accept:

- `git@github.com:owner/repository.git`
- `git@github-alias:owner/repository.git`
- `ssh://git@github.com/owner/repository.git`
- `ssh://git@github-alias/owner/repository.git`

The SSH host must contain only letters, digits, dots, underscores, and hyphens,
must start and end with a letter or digit, and must not contain wildcards,
whitespace, path separators, a port, or embedded credentials. Owner and
repository validation is unchanged.

The parsed model will retain the SSH host. Canonical clone URLs will preserve
that host instead of replacing it with `github.com`.

## SSH Verification

Before repository access is attempted, `sshprobe.Check` will extract the SSH
host from the validated repository URL and run:

```text
ssh -G <ssh-host>
```

The probe will parse the effective `hostname`, `user`, and all `identityfile`
entries. Verification stops with an actionable status message if the effective
hostname is not `github.com` or the effective user is not `git`.

If the alias resolves correctly, the existing non-interactive `git ls-remote`
check runs against the alias URL with `BatchMode=yes`,
`StrictHostKeyChecking=yes`, and the existing connection timeout. OpenSSH then
applies the alias's `HostName`, `IdentityFile`, `IdentitiesOnly`, proxy, and
related configuration.

## Error Handling and Security

- A syntactically invalid alias is rejected when the repository URL is entered.
- An unknown or broken alias reports the existing SSH configuration error.
- An alias resolving outside GitHub is rejected before Git contacts the
  repository.
- An alias that changes the SSH user away from `git` is rejected.
- Strict host-key verification remains enabled.
- HTTPS OAuth behavior is unchanged.

## Testing

- Repository parser tests cover accepted SCP and `ssh://` aliases, alias
  preservation, and malformed aliases.
- SSH probe tests confirm `ssh -G` receives the alias, effective GitHub host and
  user are enforced, alias identity files are reported, and `git ls-remote`
  receives the original alias URL.
- Existing literal `github.com` SSH and HTTPS cases remain passing.
- Onboarding help text and installation documentation include an alias example.

## Acceptance Criteria

1. `git@github-jelllove:jelllove/agents-backup.git` is accepted by onboarding.
2. SSH verification runs `ssh -G github-jelllove`.
3. An alias resolving to `hostname github.com` and `user git` proceeds to
   `git ls-remote` using the original alias URL.
4. The alias URL is stored unchanged apart from the existing canonical `.git`
   suffix.
5. Aliases resolving to a non-GitHub hostname or non-`git` user are rejected
   with a clear message.
6. Literal GitHub SSH URLs and GitHub HTTPS URLs continue to work.
