# AgentConfigSync Onboarding and Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a new desktop user connect a private GitHub repository through an in-app GitHub OAuth or SSH wizard, securely persist credentials, initialize empty remotes, and complete first sync without a terminal.

**Architecture:** A UI-independent onboarding service owns repository URL validation, GitHub OAuth Device Flow, OS keyring access, Git credential protocol, SSH probes, and idempotent repository initialization. The Wails frontend receives only non-secret OAuth display values and progress. All Git network commands use a host-scoped injected credential helper with terminal prompting disabled.

**Tech Stack:** Go stdlib HTTP/OAuth flow, Git CLI, OpenSSH CLI, `github.com/zalando/go-keyring` v0.2.8, Wails bindings/events, React/TypeScript.

---

## File Structure

```text
internal/auth/keyring.go                 # credential store interface/implementation
internal/auth/keyring_test.go
internal/auth/metadata.go                # active non-secret account metadata
internal/auth/metadata_test.go
internal/auth/oauth.go                   # GitHub Device Flow backend
internal/auth/oauth_test.go
internal/auth/credential.go              # Git credential protocol
internal/auth/credential_test.go
internal/repository/url.go               # strict GitHub URL parser
internal/repository/url_test.go
internal/repository/setup.go             # access probe/clone/empty repo bootstrap
internal/repository/setup_test.go
internal/sshprobe/probe.go               # key discovery and unattended access
internal/sshprobe/probe_test.go
internal/onboarding/service.go           # wizard orchestration
internal/onboarding/service_test.go
main.go                                  # packaged GUI hidden credential mode
cmd/acsync/main.go                       # optional CLI hidden credential mode
internal/gitclient/gitclient.go          # inject noninteractive credential helper
internal/gitclient/gitclient_test.go
internal/desktop/wails.go                # onboarding bindings/events
frontend/src/onboarding/*.tsx
frontend/src/onboarding/*.test.tsx
```

---

### Task 1: Validate and normalize GitHub repository URLs

**Files:**
- Create: `internal/repository/url.go`
- Create: `internal/repository/url_test.go`

- [ ] **Step 1: Write table-driven failing tests**

```go
package repository

import "testing"

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		raw      string
		protocol Protocol
		owner    string
		repo     string
		ok       bool
	}{
		{"https://github.com/acme/sync.git", HTTPS, "acme", "sync", true},
		{"git@github.com:acme/sync.git", SSH, "acme", "sync", true},
		{"ssh://git@github.com/acme/sync.git", SSH, "acme", "sync", true},
		{"https://token@github.com/acme/sync.git", "", "", "", false},
		{"https://evil.example/acme/sync.git", "", "", "", false},
		{"https://github.com/acme/sync.git?x=1", "", "", "", false},
	}
	for _, tt := range tests {
		got, err := ParseGitHubURL(tt.raw)
		if tt.ok && err != nil {
			t.Errorf("%q: %v", tt.raw, err)
			continue
		}
		if !tt.ok {
			if err == nil { t.Errorf("%q: expected error", tt.raw) }
			continue
		}
		if got.Protocol != tt.protocol || got.Owner != tt.owner || got.Repository != tt.repo {
			t.Errorf("%q => %#v", tt.raw, got)
		}
	}
}
```

- [ ] **Step 2: Run test**

```powershell
go test ./internal/repository -run TestParseGitHubURL -v
```

Expected: package/types missing.

- [ ] **Step 3: Implement strict parsing**

```go
package repository

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

type Protocol string
const (
	HTTPS Protocol = "https"
	SSH   Protocol = "ssh"
)

type GitHubURL struct {
	Protocol   Protocol
	Owner      string
	Repository string
	CloneURL   string
}

var scpPattern = regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func ParseGitHubURL(raw string) (GitHubURL, error) {
	if match := scpPattern.FindStringSubmatch(raw); match != nil {
		if !validSegments(match[1], match[2]) { return GitHubURL{}, errors.New("invalid repository path") }
		return GitHubURL{SSH, match[1], match[2], "git@github.com:" + match[1] + "/" + match[2] + ".git"}, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return GitHubURL{}, errors.New("invalid GitHub repository URL")
	}
	if parsed.Hostname() != "github.com" {
		return GitHubURL{}, errors.New("repository host must be github.com")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 || !validSegments(parts[0], parts[1]) {
		return GitHubURL{}, errors.New("repository URL must contain owner and repository")
	}
	switch parsed.Scheme {
	case "https":
		return GitHubURL{HTTPS, parts[0], parts[1], "https://github.com/" + parts[0] + "/" + parts[1] + ".git"}, nil
	case "ssh":
		if parsed.User == nil || parsed.User.Username() != "git" {
			return GitHubURL{}, errors.New("SSH GitHub URL must use git user")
		}
		return GitHubURL{SSH, parts[0], parts[1], "ssh://git@github.com/" + parts[0] + "/" + parts[1] + ".git"}, nil
	default:
		return GitHubURL{}, errors.New("supported protocols are HTTPS and SSH")
	}
}

func validSegments(owner, repo string) bool {
	return segmentPattern.MatchString(owner) && segmentPattern.MatchString(repo)
}
```

- [ ] **Step 4: Verify and commit**

```powershell
go test ./internal/repository -v
git add internal/repository
git commit -m "feat: validate GitHub repository URLs"
```

---

### Task 2: Store OAuth tokens in the OS keyring

**Files:**
- Create: `internal/auth/keyring.go`
- Create: `internal/auth/keyring_test.go`
- Create: `internal/auth/metadata.go`
- Create: `internal/auth/metadata_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add dependency**

```powershell
go get github.com/zalando/go-keyring@v0.2.8
```

- [ ] **Step 2: Write a fake-backed test**

```go
package auth

import "testing"

type memoryBackend map[string]string
func (m memoryBackend) Set(service, user, password string) error { m[service+"\x00"+user] = password; return nil }
func (m memoryBackend) Get(service, user string) (string, error) {
	value, ok := m[service+"\x00"+user]
	if !ok { return "", ErrNotFound }
	return value, nil
}
func (m memoryBackend) Delete(service, user string) error { delete(m, service+"\x00"+user); return nil }

func TestStoreNeverReturnsTokenInMetadata(t *testing.T) {
	store := NewStore(memoryBackend{})
	account := Account{ID: 42, Login: "alice", Scopes: []string{"repo"}}
	if err := store.Save(account, "gho_secret"); err != nil { t.Fatal(err) }
	got, err := store.Token(42)
	if err != nil || got != "gho_secret" { t.Fatalf("token=%q err=%v", got, err) }
	if account.Login == "gho_secret" { t.Fatal("token leaked into metadata") }
}
```

- [ ] **Step 3: Implement store abstraction**

```go
package auth

import (
	"errors"
	"strconv"

	keyring "github.com/zalando/go-keyring"
)

const keyringService = "io.github.qinqingxu.acsync/github-oauth"
var ErrNotFound = errors.New("credential not found")

type Backend interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type systemBackend struct{}
func (systemBackend) Set(s, u, p string) error { return keyring.Set(s, u, p) }
func (systemBackend) Get(s, u string) (string, error) {
	value, err := keyring.Get(s, u)
	if errors.Is(err, keyring.ErrNotFound) { return "", ErrNotFound }
	return value, err
}
func (systemBackend) Delete(s, u string) error { return keyring.Delete(s, u) }

type Account struct {
	ID     int64    `yaml:"id" json:"id"`
	Login  string   `yaml:"login" json:"login"`
	Scopes []string `yaml:"scopes" json:"scopes"`
}

type Store struct{ backend Backend }
func NewStore(backend Backend) *Store { return &Store{backend: backend} }
func NewSystemStore() *Store { return NewStore(systemBackend{}) }
func accountKey(id int64) string { return strconv.FormatInt(id, 10) }
func (s *Store) Save(account Account, token string) error {
	if account.ID == 0 || token == "" { return errors.New("account and token are required") }
	return s.backend.Set(keyringService, accountKey(account.ID), token)
}
func (s *Store) Token(id int64) (string, error) {
	return s.backend.Get(keyringService, accountKey(id))
}
func (s *Store) Delete(id int64) error {
	return s.backend.Delete(keyringService, accountKey(id))
}
```

- [ ] **Step 4: Persist only non-secret active account metadata**

Create `internal/auth/metadata.go`:

```go
package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Metadata struct {
	Active Account `json:"active"`
}

func SaveMetadata(path string, metadata Metadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadMetadata(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}
```

Add a test that saves/loads ID, login, and scopes and asserts the file does not
contain `gho_`, `token`, or a supplied secret value. The helper subprocess reads
this file to discover the active keyring account; it never scans keyring
accounts or receives account identity from the frontend.

- [ ] **Step 5: Verify**

```powershell
go test ./internal/auth -run TestStore -v
go mod tidy
git add internal/auth go.mod go.sum
git commit -m "feat: store GitHub credentials in OS keyring"
```

---

### Task 3: Implement GitHub OAuth Device Flow

**Files:**
- Create: `internal/auth/oauth.go`
- Create: `internal/auth/oauth_test.go`

- [ ] **Step 1: Add deterministic HTTP server tests**

Create tests that:

```go
func TestDeviceFlowPendingThenSucceeds(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			io.WriteString(w, `{"device_code":"backend-only","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","expires_in":60,"interval":1}`)
		case "/login/oauth/access_token":
			if polls.Add(1) == 1 {
				io.WriteString(w, `{"error":"authorization_pending"}`)
			} else {
				io.WriteString(w, `{"access_token":"gho_secret","token_type":"bearer","scope":"repo"}`)
			}
		case "/user":
			io.WriteString(w, `{"id":42,"login":"alice"}`)
		}
	}))
	defer server.Close()
	client := NewOAuthClient("client-id", server.Client())
	client.BaseURL = server.URL
	client.APIURL = server.URL
	client.Sleep = func(context.Context, time.Duration) error { return nil }
	start, err := client.Start(context.Background())
	if err != nil { t.Fatal(err) }
	if start.UserCode != "ABCD-EFGH" || start.DeviceCode != "" {
		t.Fatalf("frontend start = %#v", start)
	}
	account, token, err := client.Wait(context.Background())
	if err != nil || account.ID != 42 || token != "gho_secret" {
		t.Fatalf("account=%#v token=%q err=%v", account, token, err)
	}
}
```

- [ ] **Step 2: Implement backend-only flow state**

`OAuthClient` must store the `device_code` in private fields and return:

```go
type DeviceStart struct {
	UserCode        string        `json:"userCode"`
	VerificationURI string        `json:"verificationUri"`
	ExpiresIn       time.Duration `json:"expiresIn"`
}
```

Implement:

```go
type OAuthClient struct {
	ClientID string
	HTTP *http.Client
	BaseURL string
	APIURL string
	Sleep func(context.Context, time.Duration) error
	deviceCode string
	interval time.Duration
	deadline time.Time
}
```

Use form POSTs to:

```text
POST /login/device/code
POST /login/oauth/access_token
GET  /user
```

Require returned scope to contain `repo`. Handle:

```text
authorization_pending -> continue
slow_down             -> add 5 seconds
expired_token         -> return typed expiry error
access_denied         -> return typed denial error
all other OAuth errors -> return explicit configuration error
```

Allow only `https://github.com/login/device` in production. Never log request
or response bodies and never return `device_code` to Wails.

- [ ] **Step 3: Run auth tests**

```powershell
go test ./internal/auth -run TestDeviceFlow -race -v
```

Expected: pass.

- [ ] **Step 4: Commit**

```powershell
git add internal/auth/oauth*
git commit -m "feat: add GitHub OAuth device authorization"
```

---

### Task 4: Implement the Git credential protocol

**Files:**
- Create: `internal/auth/credential.go`
- Create: `internal/auth/credential_test.go`

- [ ] **Step 1: Write protocol tests**

Cover:

```go
func TestCredentialGetAllowsOnlyGitHubHTTPS(t *testing.T) {
	store := NewStore(memoryBackend{})
	_ = store.Save(Account{ID: 42, Login: "alice"}, "gho_secret")
	handler := CredentialHandler{Store: store, Account: Account{ID: 42, Login: "alice"}}
	var out bytes.Buffer
	err := handler.Run("get", strings.NewReader("protocol=https\nhost=github.com\n\n"), &out)
	if err != nil { t.Fatal(err) }
	if got := out.String(); got != "username=alice\npassword=gho_secret\n\n" {
		t.Fatalf("output=%q", got)
	}
	out.Reset()
	err = handler.Run("get", strings.NewReader("protocol=https\nhost=evil.example\n\n"), &out)
	if err == nil { t.Fatal("expected host rejection") }
	if strings.Contains(out.String(), "gho_") { t.Fatal("token leaked") }
}
```

Also test oversized lines, CR/NUL, missing token (`quit=1`), `store` no-op,
and matched `erase`.

- [ ] **Step 2: Implement the helper**

Use a `bufio.Scanner` with a 16 KiB maximum. Accept only:

```text
protocol=https
host=github.com
```

or `github.com:443`. For `get`, output username/password plus blank line. For
missing token, output `quit=1`. Never log input/output. `store` is a no-op.
`erase` deletes only when host/protocol match and the supplied password equals
the stored token.

- [ ] **Step 3: Verify**

```powershell
go test ./internal/auth -run TestCredential -race -v
git add internal/auth/credential*
git commit -m "feat: add scoped Git credential helper"
```

---

### Task 5: Inject credentials into every Git network operation

**Files:**
- Modify: `internal/gitclient/gitclient.go`
- Modify: `internal/gitclient/gitclient_test.go`

- [ ] **Step 1: Add command-construction tests**

Refactor command construction behind:

```go
type CommandFactory func(name string, args ...string) *exec.Cmd
```

Test that HTTPS clone/pull/push contain:

```text
-c credential.helper=
-c credential.https://github.com.helper=!\"<absolute-AgentConfigSync>\" --git-credential
-c credential.interactive=false
```

and child environment contains `GIT_TERMINAL_PROMPT=0` while removing inherited
`GIT_TRACE*` and `GIT_CURL_VERBOSE`.

- [ ] **Step 2: Add client authentication options**

```go
type AuthMode string
const (
	AuthSystem AuthMode = "system"
	AuthOAuth  AuthMode = "oauth"
)

type Client struct {
	Dir        string
	AuthMode   AuthMode
	Executable string
	Command    CommandFactory
}
```

Build arguments in one central method. For OAuth, prepend the host-scoped helper
options to clone, pull, push, and `ls-remote`. For SSH/system mode, preserve
existing Git behavior. Always set `GIT_TERMINAL_PROMPT=0` for desktop probes.

- [ ] **Step 3: Run all Git and CLI integration tests**

```powershell
go test ./internal/gitclient ./internal/cli -race
```

Expected: pass.

- [ ] **Step 4: Commit**

```powershell
git add internal/gitclient
git commit -m "feat: use secure noninteractive Git authentication"
```

---

### Task 6: Add hidden credential-helper executable mode

**Files:**
- Modify: `main.go`
- Modify: `cmd/acsync/main.go`
- Create: `internal/cli/credential.go`
- Create: `internal/cli/credential_test.go`

- [ ] **Step 1: Test helper routing**

Test `RunCredential(operation, reader, writer)` with fake account metadata and
fake keyring backend injected through parameters. Assert no output contains a
token on unsupported hosts.

- [ ] **Step 2: Add packaged-GUI helper mode before Wails starts**

The installed desktop executable is guaranteed to exist; the separate
`acsync` CLI is not. In root `main.go`, inspect arguments before creating Wails:

```go
func main() {
	if len(os.Args) == 3 && os.Args[1] == "--git-credential" {
		if err := cli.RunCredential(os.Args[2], os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		return
	}
	gui, err := newGUI()
	if err != nil {
		log.Fatal(err)
	}
	if err := gui.app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

This mode must return before Wails and its single-instance lock are initialized.
Configure Git with the absolute packaged executable:

```text
credential.https://github.com.helper=!"/absolute/path/AgentConfigSync" --git-credential
```

- [ ] **Step 3: Keep an optional hidden Cobra command for headless CLI**

```go
func credentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "git-credential [get|store|erase]",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.RunCredential(args[0], os.Stdin, os.Stdout)
		},
	}
	return cmd
}
```

Register it with the root command. It must not appear in normal help.

- [ ] **Step 4: Verify protocol through the packaged executable**

```powershell
"protocol=https`nhost=github.com`n" | .\bin\AgentConfigSync.exe --git-credential get
```

Expected: either valid username/password protocol output when configured, or
`quit=1`; no terminal prompt.

- [ ] **Step 5: Commit**

```powershell
git add main.go cmd/acsync/main.go internal/cli/credential*
git commit -m "feat: expose hidden Git credential helper"
```

---

### Task 7: Initialize empty repositories idempotently

**Files:**
- Create: `internal/repository/setup.go`
- Create: `internal/repository/setup_test.go`
- Modify: `internal/cli/init.go`
- Modify: `internal/cli/init_test.go`

- [ ] **Step 1: Add bare-remote integration tests**

Test:

1. `git init --bare` remote has no refs;
2. `Setup` clones it;
3. creates `.gitattributes`;
4. creates `main`;
5. commits and pushes;
6. a second `Setup` is a no-op;
7. initialized remote works with `RunSync`.

- [ ] **Step 2: Implement setup**

```go
type Setup struct {
	Client *gitclient.Client
	Name   string
	Email  string
}

func (s *Setup) Initialize(remote, dir string) error {
	// Validate URL before this method.
	// Clone if dir is absent/empty.
	// If HEAD exists, return.
	// Write "* -text\n" .gitattributes.
	// git checkout -B main
	// configure repository-local user.name/user.email only when missing.
	// add, commit "chore: initialize AgentConfigSync repository"
	// push -u origin main.
}
```

Add explicit `gitclient` methods for `RemoteHasHEAD`, `CheckoutBranch`,
`SetLocalConfig`, and `PushUpstream`; do not use a shell string.

- [ ] **Step 3: Route CLI init through repository setup**

Replace the direct `Clone` path in `RunInit` with the setup service while
preserving provider detection and config save. Existing non-empty initialized
repositories remain unchanged.

- [ ] **Step 4: Verify**

```powershell
go test ./internal/repository ./internal/cli -run "Test.*Init|Test.*Setup" -race -v
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/repository internal/gitclient internal/cli/init*
git commit -m "fix: initialize empty sync repositories automatically"
```

---

### Task 8: Add SSH discovery and unattended access checks

**Files:**
- Create: `internal/sshprobe/probe.go`
- Create: `internal/sshprobe/probe_test.go`

- [ ] **Step 1: Write fake-runner tests**

Define:

```go
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}
```

Test discovery of `ssh`, `ssh -G github.com` identity files, `ssh-add -L`, and
repository access through `git ls-remote`. Assert `BatchMode=yes`,
`StrictHostKeyChecking=yes`, and no prompts.

- [ ] **Step 2: Implement probes**

Return:

```go
type Status struct {
	SSHAvailable      bool     `json:"sshAvailable"`
	IdentityFiles     []string `json:"identityFiles"`
	AgentHasKeys      bool     `json:"agentHasKeys"`
	RepositoryAccess bool     `json:"repositoryAccess"`
	Message          string   `json:"message"`
}
```

Use `git ls-remote <validated-url> HEAD` as the final access test. Do not treat
key-file existence or `ssh -T` alone as repository authorization.

- [ ] **Step 3: Add explicit key generation**

Expose:

```go
func GenerateKey(ctx context.Context, runner Runner, path, comment string) (publicKey string, err error)
```

Run `ssh-keygen -q -t ed25519 -a 64 -C <comment> -f <path> -N ""` only after an
explicit UI confirmation. Return only `.pub` content. Prefer existing
agent-backed keys; explain the security trade-off of an empty passphrase.

- [ ] **Step 4: Verify and commit**

```powershell
go test ./internal/sshprobe -race -v
git add internal/sshprobe
git commit -m "feat: detect and verify GitHub SSH access"
```

---

### Task 9: Orchestrate onboarding from Wails

**Files:**
- Create: `internal/onboarding/service.go`
- Create: `internal/onboarding/service_test.go`
- Modify: `internal/desktop/wails.go`

- [ ] **Step 1: Write service-state tests**

Test transitions:

```text
welcome -> repository -> authentication -> verification -> agents -> ready
```

Ensure OAuth cancellation clears backend-only device state, failed access
does not write config, and successful setup saves config only after clone and
initialization succeed.

- [ ] **Step 2: Implement state machine**

```go
type Step string
const (
	Welcome Step = "welcome"
	Repository Step = "repository"
	Authentication Step = "authentication"
	Verification Step = "verification"
	Agents Step = "agents"
	Ready Step = "ready"
)

type State struct {
	Step Step `json:"step"`
	RepositoryURL string `json:"repositoryUrl"`
	AuthMode string `json:"authMode"`
	UserCode string `json:"userCode,omitempty"`
	VerificationURI string `json:"verificationUri,omitempty"`
	Message string `json:"message,omitempty"`
	Agents []desktop.Agent `json:"agents,omitempty"`
}
```

Methods:

```go
State() State
SetRepository(raw string) error
StartGitHubLogin(ctx context.Context) (State, error)
WaitGitHubLogin(ctx context.Context) (State, error)
VerifySSH(ctx context.Context) (State, error)
Complete(ctx context.Context, enabled map[string]bool) error
Cancel()
```

Inject OAuth, keyring, SSH runner, repository setup, and config writer
interfaces. Do not construct concrete dependencies inside tests.

- [ ] **Step 3: Make the desktop service boot without configuration**

Change `desktop.New` so a missing `config.yaml` returns an unconfigured service
instead of an error. Add:

```go
type Service struct {
	home      string
	goos      string
	mu        sync.Mutex
	daemon    *daemon.Daemon
	start     chan *daemon.Daemon
	last      daemon.CycleResult
}

func New(home, goos string) (*Service, error) {
	s := &Service{home: home, goos: goos, start: make(chan *daemon.Daemon, 1)}
	if _, err := os.Stat(cli.ConfigPath(home)); err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := s.StartConfigured(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) StartConfigured() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.daemon != nil {
		return nil
	}
	d, err := daemon.New(s.home, s.goos)
	if err != nil {
		return err
	}
	d.OnCycle = s.recordCycle
	s.daemon = d
	s.start <- d
	return nil
}

func (s *Service) Run(ctx context.Context, publish func(Snapshot)) error {
	select {
	case d := <-s.start:
		unsubscribe := d.Scheduler.Subscribe(func(scheduler.State) {
			if snapshot, err := s.Snapshot(); err == nil { publish(snapshot) }
		})
		defer unsubscribe()
		return d.Run(ctx)
	case <-ctx.Done():
		return nil
	}
}
```

For an unconfigured service, `Snapshot()` returns:

```go
Snapshot{Configured: false, State: "idle", IntervalMinutes: 10, TrashGraceDays: 30}
```

without calling `RunStatus`. `Trigger`, `Pause`, and `Resume` return an explicit
`ErrNotConfigured` rather than dereferencing a nil daemon. After onboarding
successfully writes config and initializes the repository, call
`StartConfigured`; the already-running desktop lifecycle then starts the one
daemon. Add tests for unconfigured construction and post-onboarding start.

- [ ] **Step 4: Bind to Wails**

Add exported Wails methods with the exact signatures above and emit
`onboarding:state` after each transition. Add:

```go
func (s *WailsService) NeedsOnboarding() bool
```

It returns true only when config is missing or repository access/setup is
invalid. Existing valid `~/.acsync` users skip onboarding.

- [ ] **Step 5: Verify**

```powershell
go test ./internal/onboarding ./internal/desktop -race
wails3 generate bindings -ts
```

- [ ] **Step 6: Commit**

```powershell
git add internal/onboarding internal/desktop/wails.go frontend/bindings
git commit -m "feat: add desktop onboarding orchestration"
```

---

### Task 10: Build the first-run wizard UI

**Files:**
- Create: `frontend/src/onboarding/Onboarding.tsx`
- Create: `frontend/src/onboarding/Onboarding.test.tsx`
- Create: `frontend/src/onboarding/RepositoryStep.tsx`
- Create: `frontend/src/onboarding/AuthStep.tsx`
- Create: `frontend/src/onboarding/AgentStep.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/api.ts`
- Modify: `frontend/src/styles.css`

- [ ] **Step 1: Add wizard tests**

Test:

1. repository URL validation errors appear;
2. GitHub login displays code and opens only `github.com/login/device`;
3. SSH verification status appears;
4. all detected agents default checked;
5. Complete enters dashboard;
6. no token/device code exists in rendered DOM or mocked Wails payload.

- [ ] **Step 2: Implement API wrapper**

Add typed methods matching generated bindings:

```ts
needsOnboarding()
onboardingState()
setRepository(url)
startGitHubLogin()
waitGitHubLogin()
verifySSH()
completeOnboarding(agents)
cancelOnboarding()
```

- [ ] **Step 3: Implement wizard**

`Onboarding.tsx` owns the visible step and delegates each page. GitHub auth page
shows:

```text
Open GitHub
Code: ABCD-EFGH
Waiting for authorization…
```

The backend opens only the validated verification URI. SSH page reports keys
and actual repository access separately. Agent page displays the same agent
cards as the dashboard, checked by default.

- [ ] **Step 4: Route application startup**

In `App.tsx`, call `needsOnboarding` first. Render `Onboarding` when true;
otherwise load the dashboard. After successful completion, request a fresh
desktop snapshot.

- [ ] **Step 5: Verify frontend and backend**

```powershell
npm --prefix frontend test -- --run
npm --prefix frontend run build
go test ./internal/auth ./internal/repository ./internal/sshprobe ./internal/onboarding ./internal/gitclient ./internal/cli -race
wails3 build
```

- [ ] **Step 6: Manual private-repository smoke**

On a test GitHub account:

1. OAuth authorize;
2. select an empty private repo;
3. verify the app creates and pushes `main`;
4. complete initial sync;
5. sign out locally and verify keyring entry removal;
6. repeat using SSH;
7. restart and confirm onboarding is skipped.

- [ ] **Step 7: Commit**

```powershell
git add frontend internal
git commit -m "feat: add repository and authentication onboarding"
```

---

## Done Criteria

- New users configure a private repository without a terminal.
- OAuth tokens never enter YAML, remote URLs, argv, environment, logs, or Wails
  payloads and are stored only in the OS keyring.
- Git credential helper handles only exact GitHub HTTPS requests and disables
  fallback prompting/helpers.
- SSH onboarding verifies the actual repository, not merely key existence.
- Empty remotes are initialized idempotently with `main`.
- Existing configured users skip onboarding.
- OAuth and SSH flows complete first sync through the desktop UI.
