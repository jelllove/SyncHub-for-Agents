# GitHub SSH Host Alias Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Accept GitHub SSH repository URLs that use local OpenSSH host aliases while preserving GitHub-only verification.

**Architecture:** Extend the repository URL parser to preserve a validated SSH host alias. Resolve that alias through `ssh -G` before repository access, require the effective hostname to remain `github.com`, and then let the existing strict non-interactive Git probe use the original alias URL and its local SSH configuration.

**Tech Stack:** Go, OpenSSH, Git, React/TypeScript, Go tests, repository documentation checks

---

## File Structure

- Modify `internal/repository/url.go`: validate and preserve SSH host aliases.
- Modify `internal/repository/url_test.go`: cover accepted and rejected alias URL forms.
- Modify `internal/sshprobe/probe.go`: resolve alias-specific SSH configuration
  and enforce the effective GitHub hostname.
- Modify `internal/sshprobe/probe_test.go`: verify alias resolution and rejection.
- Modify `frontend/src/onboarding/Onboarding.tsx`: show an SSH alias example.
- Modify `docs/install.md`: document the supported local SSH configuration.

### Task 1: Preserve valid SSH aliases in parsed repository URLs

**Files:**
- Modify: `internal/repository/url_test.go`
- Modify: `internal/repository/url.go`

- [ ] **Step 1: Add failing parser cases**

Extend the table in `TestParseGitHubURL` with an expected SSH host and clone URL:

```go
tests := []struct {
	raw        string
	protocol   Protocol
	sshHost    string
	owner      string
	repository string
	cloneURL   string
	ok         bool
}{
	{
		"git@github-jelllove:jelllove/agents-backup.git",
		SSH,
		"github-jelllove",
		"jelllove",
		"agents-backup",
		"git@github-jelllove:jelllove/agents-backup.git",
		true,
	},
	{
		"ssh://git@github-jelllove/jelllove/agents-backup.git",
		SSH,
		"github-jelllove",
		"jelllove",
		"agents-backup",
		"ssh://git@github-jelllove/jelllove/agents-backup.git",
		true,
	},
	{"git@-bad:acme/sync.git", "", "", "", "", "", false},
	{"git@bad*:acme/sync.git", "", "", "", "", "", false},
}
```

Keep the existing literal GitHub SSH and HTTPS cases, populate their expected
`sshHost` and `cloneURL`, and assert:

```go
if got.Protocol != test.protocol ||
	got.SSHHost != test.sshHost ||
	got.Owner != test.owner ||
	got.Repository != test.repository ||
	got.CloneURL != test.cloneURL {
	t.Fatalf("ParseGitHubURL(%q) = %#v", test.raw, got)
}
```

- [ ] **Step 2: Run the parser test and confirm failure**

```powershell
go test ./internal/repository -run TestParseGitHubURL -count=1
```

Expected: compilation fails because `GitHubURL.SSHHost` does not exist, or the
alias cases fail with `invalid GitHub repository URL`.

- [ ] **Step 3: Implement alias validation and preservation**

Add `SSHHost string` to `GitHubURL`, replace the SCP regexp with:

```go
scpPattern = regexp.MustCompile(
	`^git@([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?):([^/]+)/([^/]+?)(?:\.git)?$`,
)
sshHostPattern = regexp.MustCompile(
	`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`,
)
```

For SCP URLs, return the matched host and preserve it in `CloneURL`:

```go
return GitHubURL{
	Protocol:   SSH,
	SSHHost:    match[1],
	Owner:      match[2],
	Repository: match[3],
	CloneURL:   "git@" + match[1] + ":" + match[2] + "/" + match[3] + ".git",
}, nil
```

Move the literal `github.com` restriction into the HTTPS branch. In the SSH
branch, reject ports, validate `parsed.Hostname()` with `sshHostPattern`, retain
the existing `git` user and password checks, set `SSHHost`, and preserve the
alias in the canonical `ssh://` clone URL.

- [ ] **Step 4: Run parser and CLI tests**

```powershell
go test ./internal/repository ./internal/cli -run 'TestParseGitHubURL|TestNewGitClient' -count=1
```

Expected: PASS.

### Task 2: Resolve and constrain the alias during SSH verification

**Files:**
- Modify: `internal/sshprobe/probe_test.go`
- Modify: `internal/sshprobe/probe.go`

- [ ] **Step 1: Write failing SSH probe tests**

Update `fakeRunner` to return an effective config:

```go
if len(args) > 0 && args[0] == "-G" {
	return "hostname github.com\nuser git\nidentityfile ~/.ssh/jelllove\n", "", nil
}
```

Change `TestCheckVerifiesUnattendedRepositoryAccess` to use
`git@github-jelllove:jelllove/agents-backup.git` and assert the calls contain:

```go
for _, wanted := range []string{
	"ssh -G -l git github-jelllove",
	"git@github-jelllove:jelllove/agents-backup.git",
	"BatchMode=yes",
	"StrictHostKeyChecking=yes",
	"ls-remote",
} {
	if !strings.Contains(calls, wanted) {
		t.Fatalf("commands missing %q:\n%s", wanted, calls)
	}
}
if len(status.IdentityFiles) != 1 ||
	status.IdentityFiles[0] != "~/.ssh/jelllove" {
	t.Fatalf("identity files = %#v", status.IdentityFiles)
}
```

Add a runner that returns `hostname evil.example` and a test asserting:

```go
status, err := Check(
	context.Background(),
	runner,
	"git@github-jelllove:jelllove/agents-backup.git",
)
if err != nil {
	t.Fatal(err)
}
if status.RepositoryAccess ||
	!strings.Contains(status.Message, "must resolve to github.com") ||
	runner.gitCalled {
	t.Fatalf("status = %#v, git called = %v", status, runner.gitCalled)
}
```

- [ ] **Step 2: Run the tests and confirm alias-specific failures**

```powershell
go test ./internal/sshprobe -run TestCheck -count=1
```

Expected: FAIL because the probe still runs `ssh -G github.com` and does not
reject a non-GitHub effective hostname.

- [ ] **Step 3: Parse effective OpenSSH configuration**

Import `internal/repository`, parse `repositoryURL`, require the SSH protocol,
and invoke:

```go
configOutput, stderr, err := runner.Run(
	ctx,
	"ssh",
	"-G",
	"-l", "git",
	parsed.SSHHost,
)
```

Replace `parseIdentityFiles` with:

```go
type effectiveConfig struct {
	hostname      string
	user          string
	identityFiles []string
}

func parseConfig(output string) effectiveConfig {
	var config effectiveConfig
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "hostname":
			config.hostname = fields[1]
		case "user":
			config.user = fields[1]
		case "identityfile":
			config.identityFiles = append(config.identityFiles, fields[1])
		}
	}
	return config
}
```

Set `status.IdentityFiles` from `identityFiles`, then stop before `git
ls-remote` unless `hostname` equals `github.com` case-insensitively and `user`
equals `git` exactly. Use these messages:

```go
"SSH host alias must resolve to github.com"
"SSH repository access must use the git user"
```

- [ ] **Step 4: Run repository and SSH probe tests**

```powershell
go test ./internal/repository ./internal/sshprobe -count=1
```

Expected: PASS.

### Task 3: Document the supported URL form

**Files:**
- Modify: `frontend/src/onboarding/Onboarding.tsx`
- Modify: `docs/install.md`

- [ ] **Step 1: Add the onboarding example**

Add this line to `.url-examples`:

```tsx
<span><strong>SSH alias</strong> git@github-work:you/sync.git</span>
```

- [ ] **Step 2: Add installation guidance**

In the first-launch URL examples in `docs/install.md`, add:

```markdown
- SSH alias: `git@github-work:your-name/agent-sync.git`
```

Explain directly below the list:

```markdown
SSH aliases may select a different GitHub key through `~/.ssh/config`. The alias
must resolve to `HostName github.com`; SyncHub verifies the effective host before
contacting the repository.
```

- [ ] **Step 3: Run frontend and documentation checks**

```powershell
npm --prefix frontend run typecheck
node scripts/dev.mjs docs
```

Expected: both commands PASS.

### Task 4: Validate and relaunch

**Files:**
- Verify: all files changed by Tasks 1-3

- [ ] **Step 1: Run focused tests**

```powershell
go test ./internal/repository ./internal/sshprobe ./internal/onboarding ./internal/cli -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full repository verification**

```powershell
node scripts/dev.mjs check
node scripts/dev.mjs verify
```

Expected: both commands PASS and the source snapshot remains stable.

- [ ] **Step 3: Build and relaunch the isolated desktop app**

```powershell
go build -buildvcs=false -o .\bin\SyncHub-test.exe .
```

Stop the existing `synchub-updated` process, then run the executable with the
session's isolated `USERPROFILE`, `HOME`, `APPDATA`, and `LOCALAPPDATA`.

Expected: WebView2 starts successfully, the onboarding URL field shows the SSH
alias example, and the real user profile is not accessed.
