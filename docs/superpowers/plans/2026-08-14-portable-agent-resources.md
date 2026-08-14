# Portable Agent Resources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Synchronize and restore portable agent configuration, instructions, Skills, Plugin declarations, and sessions while keeping credentials, machine state, caches, dependencies, and platform binaries local.

**Architecture:** Add versioned typed resource declarations beside the legacy provider schema, resolve them into platform-specific resource specs, and synchronize new artifacts through the backward-compatible `agents/_portable/config` pseudo-provider. Use immutable staged artifacts, safe structured projections, content-backed three-way merge, trusted install plans, and category-aware Wails APIs while retaining existing session and deletion behavior.

**Tech Stack:** Go 1.26.5, Wails v3 beta 8, React 19, TypeScript 7, YAML v3, doublestar globs, Git CLI, private Git remotes.

---

**Design spec:** `docs/superpowers/specs/2026-08-14-portable-agent-resources-design.md`

**Execution note:** Perform implementation in a dedicated worktree created from
`master` after commit `deadcf4`. Do not use the real `~/.acsync/repo` or live
agent directories in tests; use `t.TempDir()` and local bare Git repositories.

## Planned File Structure

### Resource model and configuration

- Create `internal/resource/model.go`: category, strategy, layout, declaration,
  resolved spec, issue, and stable-key types.
- Create `internal/resource/namespace.go`: legacy and `_portable` repository
  path encoding/decoding.
- Create `internal/resource/filter.go`: generated-content and size policy.
- Modify `internal/provider/provider.go`: strict v1/v2 parsing and v1
  normalization into typed declarations.
- Modify `internal/config/config.go`: schema v2 category settings and custom
  resources.
- Modify `internal/cli/runtime.go`: resolve provider declarations and custom
  resources into concrete specs.

### Portable bytes and synchronization

- Create `internal/resourcecollect/collector.go`: immutable collection into a
  per-run stage.
- Create `internal/resourcecollect/links.go`: root-link approval, canonical
  identity, shared-resource aliases, and copy fallback metadata.
- Create `internal/portableconfig/codec.go`: structured document projection and
  restore interface.
- Create `internal/portableconfig/document.go`: JSON, YAML, and TOML parsing.
- Create `internal/portableconfig/policy.go`: built-in safe-field policies and
  home-path normalization.
- Create `internal/portablemerge/merge.go`: structured merge and Git
  `merge-file` text merge.
- Create `internal/conflict/store.go`: safe base/local/remote conflict bundles.
- Create `internal/state/base.go`: content-addressed base bytes for true
  three-way merge.
- Create `internal/syncengine/resources.go`: typed-resource reconciliation and
  remote ownership filtering.
- Create `internal/syncengine/resource_apply.go`: strategy-aware push, restore,
  deletion, and alias materialization.
- Modify `internal/syncengine/engine.go`, `collect.go`, `apply.go`,
  `reconcile.go`, `cleanup.go`, and `models.go`: integrate typed resources
  without regressing legacy sessions.

### Plugin and dependency restoration

- Create `internal/installplan/model.go`: declarations, operations, approvals,
  and pending plan.
- Create `internal/installplan/discovery.go`: Skill dependency discovery.
- Create `internal/installplan/claude.go`: Claude JSON Plugin inventory and
  trusted CLI operations.
- Create `internal/installplan/copilot.go`: Copilot Plugin inventory parser and
  trusted CLI operations.
- Create `internal/installplan/store.go`: local approvals and pending plans.
- Create `internal/installplan/executor.go`: explicit argv execution and local
  recovery.

### Desktop and documentation

- Modify `internal/desktop/models.go`, `service.go`, and `wails.go`: resource
  settings, previews, installation approval, conflicts, and result counts.
- Modify `internal/settings/settings.go`: keep the legacy local settings page
  consistent with category settings.
- Regenerate `frontend/bindings/**`.
- Split resource UI into `frontend/src/resources/*.tsx` and keep orchestration
  in `frontend/src/App.tsx`.
- Create `frontend/src/resources/CustomResourceEditor.tsx`: validate custom
  source and per-platform restore target mappings.
- Modify `frontend/src/style.css`.
- Create `docs/portable-resources.md` and update `docs/install.md`.

## Spec Coverage Matrix

| Design requirement | Implementation tasks |
| --- | --- |
| Typed Sessions, Config, Instructions, Skills, and Plugins | Tasks 1, 2, and 7 |
| Common resources, symbolic-link identity, and copy fallback | Tasks 2, 3, 6, and 11 |
| Backward-compatible `_portable` pseudo-provider | Tasks 2, 6, and 11 |
| Generated-content filtering and 50 MiB limit | Tasks 3 and 11 |
| Field-level safe configuration and local credential preservation | Tasks 4, 7, and 11 |
| Cross-platform home path conversion | Tasks 2, 4, and 11 |
| True three-way merge and preserved conflicts | Tasks 5, 6, 9, 10, and 11 |
| Deletion propagation, local recovery, and remote trash | Tasks 6, 8, and 11 |
| Plugin declarations and Skill dependency reconstruction | Tasks 7, 8, 9, and 11 |
| First-use installation confirmation and later approved updates | Tasks 8, 9, and 10 |
| Category settings, previews, custom directories, and split results | Tasks 1, 9, and 10 |
| Config/repository migration and fresh-profile acceptance | Tasks 1, 6, 11, and 12 |

## Task 1: Add Typed Provider Resources and Config Schema v2

**Files:**
- Create: `internal/resource/model.go`
- Create: `internal/resource/model_test.go`
- Modify: `internal/provider/provider.go`
- Modify: `internal/provider/provider_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing resource validation and v1 normalization tests**

Add table-driven tests that prove valid categories and strategies are accepted,
`_portable` is rejected as a provider name, unknown adapters fail, and a v1
provider normalizes into separate legacy config and session declarations.

```go
func TestProviderDeclarationsNormalizeV1(t *testing.T) {
	p, err := Parse([]byte(`
name: demo
config:
  paths: {windows: "%USERPROFILE%\\.demo"}
  include: ["settings.json"]
  sessions: ["sessions/**/*.jsonl"]
  exclude: ["**/*token*"]
`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Declarations()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("declarations = %#v", got)
	}
	if got[0].Category != resource.CategoryConfig ||
		got[0].Layout != resource.LayoutLegacy ||
		got[1].Category != resource.CategorySessions {
		t.Fatalf("normalized declarations = %#v", got)
	}
}

func TestParseRejectsReservedPortableProvider(t *testing.T) {
	_, err := Parse([]byte("schema_version: 2\nname: _portable\nresources: []\n"))
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("error = %v", err)
	}
}
```

- [ ] **Step 2: Write failing config v1-to-v2 tests**

```go
func TestLoadV1DefaultsSafeCategoriesWithoutReenablingAgent(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, []byte(`
version: 1
agents:
  claude: false
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents["claude"] {
		t.Fatal("disabled provider was re-enabled")
	}
	if !cfg.CategoryEnabled("claude", resource.CategorySkills) {
		t.Fatal("missing v1 category should default enabled")
	}
}

func TestExplicitCategoryDisableWins(t *testing.T) {
	cfg := Config{
		Agents: map[string]bool{"claude": true},
		Categories: map[string]map[string]bool{
			"claude": {string(resource.CategoryPlugins): false},
		},
	}
	if cfg.CategoryEnabled("claude", resource.CategoryPlugins) {
		t.Fatal("explicit category disable was ignored")
	}
}
```

- [ ] **Step 3: Run targeted tests and verify they fail**

Run:

```powershell
go test ./internal/resource ./internal/provider ./internal/config
```

Expected: FAIL because `resource.Category`, `Provider.Declarations`,
`Config.Categories`, and `CategoryEnabled` do not exist.

- [ ] **Step 4: Implement the typed resource contract**

Create `internal/resource/model.go` with these exact public contracts:

```go
package resource

import (
	"fmt"
	"strings"
)

type Category string

const (
	CategorySessions     Category = "sessions"
	CategoryConfig       Category = "config"
	CategoryInstructions Category = "instructions"
	CategorySkills       Category = "skills"
	CategoryPlugins      Category = "plugins"
)

type Strategy string

const (
	StrategyFileTree        Strategy = "file-tree"
	StrategyTextTree        Strategy = "text-tree"
	StrategyStructuredMerge Strategy = "structured-merge"
	StrategySourceTree      Strategy = "source-tree"
	StrategyInstallManifest Strategy = "install-manifest"
)

type Layout string

const (
	LayoutLegacy   Layout = "legacy"
	LayoutPortable Layout = "portable"
)

type Declaration struct {
	ID          string            `yaml:"id"`
	Category    Category          `yaml:"category"`
	Paths       map[string]string `yaml:"paths"`
	Include     []string          `yaml:"include,omitempty"`
	Exclude     []string          `yaml:"exclude,omitempty"`
	Strategy    Strategy          `yaml:"strategy"`
	Layout      Layout            `yaml:"layout,omitempty"`
	Transformer string            `yaml:"transformer,omitempty"`
	Installer   string            `yaml:"installer,omitempty"`
	SharedAs    string            `yaml:"shared_as,omitempty"`
}

type Issue struct {
	ResourceKey string `json:"resourceKey"`
	Path        string `json:"path"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Bytes       int64  `json:"bytes,omitempty"`
}

var knownTransformers = map[string]struct{}{
	"claude-settings": {}, "copilot-settings": {}, "gemini-settings": {},
	"vscode-settings": {}, "vscode-mcp": {}, "cursor-settings": {},
	"common-skill-lock": {}, "generic-safe": {},
}

var knownInstallers = map[string]struct{}{
	"claude-plugin": {}, "copilot-plugin": {}, "skill-dependencies": {},
}

func (d Declaration) Normalized() Declaration {
	if d.Layout == "" {
		d.Layout = LayoutPortable
	}
	return d
}

func (d Declaration) Validate(provider string) error {
	if provider == "_portable" {
		return fmt.Errorf("provider name %q is reserved", provider)
	}
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("provider %s resource: missing id", provider)
	}
	switch d.Category {
	case CategorySessions, CategoryConfig, CategoryInstructions, CategorySkills, CategoryPlugins:
	default:
		return fmt.Errorf("provider %s resource %s: unsupported category %q", provider, d.ID, d.Category)
	}
	switch d.Strategy {
	case StrategyFileTree, StrategyTextTree, StrategyStructuredMerge, StrategySourceTree, StrategyInstallManifest:
	default:
		return fmt.Errorf("provider %s resource %s: unsupported strategy %q", provider, d.ID, d.Strategy)
	}
	if d.Transformer != "" {
		if _, ok := knownTransformers[d.Transformer]; !ok {
			return fmt.Errorf("provider %s resource %s: unknown transformer %q", provider, d.ID, d.Transformer)
		}
	}
	if d.Installer != "" {
		if _, ok := knownInstallers[d.Installer]; !ok {
			return fmt.Errorf("provider %s resource %s: unknown installer %q", provider, d.ID, d.Installer)
		}
	}
	return nil
}
```

The resource package owns the stable built-in adapter IDs. Custom provider
files may reference those IDs but cannot introduce executable adapters.

- [ ] **Step 5: Implement strict provider parsing and v1 normalization**

Add `SchemaVersion` and `Resources` to `provider.Provider`, decode with
`yaml.Decoder.KnownFields(true)`, reject mixed v1/v2 definitions, and implement
`Declarations()` so v1 `include` becomes `legacy-config` and v1 `sessions`
becomes `legacy-sessions`. Both inherit paths and excludes; config uses
`StrategyFileTree`, sessions uses `StrategyFileTree`, and both use
`LayoutLegacy`.

```go
type Provider struct {
	SchemaVersion int                    `yaml:"schema_version,omitempty"`
	Name          string                 `yaml:"name"`
	Config        ConfigSpec             `yaml:"config,omitempty"`
	Resources     []resource.Declaration `yaml:"resources,omitempty"`
	Secrets       SecretSpec             `yaml:"secrets"`
}

func (p Provider) Declarations() ([]resource.Declaration, error) {
	if len(p.Resources) > 0 {
		out := make([]resource.Declaration, len(p.Resources))
		for index, declaration := range p.Resources {
			declaration = declaration.Normalized()
			if err := declaration.Validate(p.Name); err != nil {
				return nil, err
			}
			out[index] = declaration
		}
		return out, nil
	}
	var out []resource.Declaration
	if len(p.Config.Include) > 0 {
		out = append(out, resource.Declaration{
			ID: "legacy-config", Category: resource.CategoryConfig,
			Paths: p.Config.Paths, Include: p.Config.Include, Exclude: p.Config.Exclude,
			Strategy: resource.StrategyFileTree, Layout: resource.LayoutLegacy,
		})
	}
	if len(p.Config.Sessions) > 0 {
		out = append(out, resource.Declaration{
			ID: "legacy-sessions", Category: resource.CategorySessions,
			Paths: p.Config.Paths, Include: p.Config.Sessions, Exclude: p.Config.Exclude,
			Strategy: resource.StrategyFileTree, Layout: resource.LayoutLegacy,
		})
	}
	for index := range out {
		out[index] = out[index].Normalized()
		if err := out[index].Validate(p.Name); err != nil {
			return nil, err
		}
	}
	return out, nil
}
```

- [ ] **Step 6: Implement config schema v2**

Keep `Agents map[string]bool` as the master switch and add category and custom
resource settings without duplicating implicit defaults.

```go
const currentVersion = 2

type CustomResource struct {
	ID       string            `yaml:"id"`
	Category resource.Category `yaml:"category"`
	Paths    map[string]string `yaml:"paths"`
	Targets  map[string]string `yaml:"targets"`
	Include  []string          `yaml:"include,omitempty"`
	Exclude  []string          `yaml:"exclude,omitempty"`
	Strategy resource.Strategy `yaml:"strategy"`
}

type Config struct {
	Version             int                        `yaml:"version,omitempty"`
	RepoURL             string                     `yaml:"repo_url"`
	SyncIntervalMinutes int                        `yaml:"sync_interval_minutes"`
	TrashGraceDays      int                        `yaml:"trash_grace_days"`
	Agents              map[string]bool            `yaml:"agents"`
	Categories          map[string]map[string]bool `yaml:"categories,omitempty"`
	CustomResources     []CustomResource           `yaml:"custom_resources,omitempty"`
}

func (c Config) CategoryEnabled(provider string, category resource.Category) bool {
	if !c.Agents[provider] {
		return false
	}
	values, exists := c.Categories[provider]
	if !exists {
		return true
	}
	enabled, exists := values[string(category)]
	return !exists || enabled
}
```

Validate custom IDs, categories, strategies, and paths before `Save`. Custom
resources must reject `StrategyInstallManifest`.

- [ ] **Step 7: Run targeted tests**

Run:

```powershell
go test ./internal/resource ./internal/provider ./internal/config
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/resource internal/provider internal/config
git commit -m "feat: define portable agent resources"
```

## Task 2: Resolve Resources and Encode the Portable Namespace

**Files:**
- Create: `internal/resource/namespace.go`
- Create: `internal/resource/namespace_test.go`
- Create: `internal/resource/spec.go`
- Modify: `internal/cli/runtime.go`
- Modify: `internal/cli/runtime_test.go`

- [ ] **Step 1: Write failing namespace tests**

```go
func TestPortableRepoPathUsesUnknownPseudoProvider(t *testing.T) {
	spec := Spec{
		Key: "claude/settings", Provider: "claude", ID: "settings",
		Category: CategoryConfig, Layout: LayoutPortable,
	}
	got, err := spec.RepoPath("settings.json")
	if err != nil {
		t.Fatal(err)
	}
	want := "agents/_portable/config/providers/claude/config/settings/settings.json"
	if got != want {
		t.Fatalf("RepoPath = %q, want %q", got, want)
	}
}

func TestCommonRepoPathUsesSharedIdentity(t *testing.T) {
	spec := Spec{
		Key: "common/common-skills", Provider: "common", ID: "skills",
		Category: CategorySkills, Layout: LayoutPortable, SharedAs: "common-skills",
	}
	got, err := spec.RepoPath("brainstorming/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	want := "agents/_portable/config/common/skills/common-skills/brainstorming/SKILL.md"
	if got != want {
		t.Fatalf("RepoPath = %q", got)
	}
}
```

- [ ] **Step 2: Write failing resolution tests**

Cover category disablement, per-platform paths, custom resources, shared
resource coalescing, and v1 provider normalization.

```go
func TestBuildResourceSpecsCoalescesSharedSkills(t *testing.T) {
	providers := []provider.Provider{
		{Name: "claude", Resources: []resource.Declaration{{
			ID: "skills", Category: resource.CategorySkills,
			Paths: map[string]string{"windows": `%USERPROFILE%\.claude\skills`},
			Strategy: resource.StrategySourceTree, SharedAs: "common-skills",
		}}},
		{Name: "common", Resources: []resource.Declaration{{
			ID: "skills", Category: resource.CategorySkills,
			Paths: map[string]string{"windows": `%USERPROFILE%\.agents\skills`},
			Strategy: resource.StrategySourceTree, SharedAs: "common-skills",
		}}},
	}
	cfg := config.Config{Agents: map[string]bool{"claude": true, "common": true}}
	specs, err := BuildResourceSpecs(cfg, providers, "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	spec := specs["common/common-skills"]
	if len(spec.Targets) != 2 {
		t.Fatalf("targets = %#v", spec.Targets)
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```powershell
go test ./internal/resource ./internal/cli
```

Expected: FAIL because resolved specs and namespace functions do not exist.

- [ ] **Step 4: Implement resolved specs and safe path encoding**

```go
type Spec struct {
	Key          string
	Provider     string
	ID           string
	Category     Category
	Strategy     Strategy
	Layout       Layout
	Root         string
	Targets      []string
	Include      []string
	Exclude      []string
	Transformer  string
	Installer    string
	SharedAs     string
	KeyPatterns  []string
}
```

`RepoPath` must reject empty, absolute, backslash-containing, and traversal
paths. Legacy resources encode as
`agents/<provider>/<config|sessions>/<relative>`. Portable provider resources
encode as shown in the tests. Shared resources encode below
`agents/_portable/config/common`.

Add `ParseRepoPath` returning a `RepoRef` with `Provider`, `Category`,
`ResourceID`, `Relative`, `Portable`, and `Common`. Reject malformed paths
instead of cleaning them.

- [ ] **Step 5: Implement `BuildResourceSpecs`**

Resolve declarations for enabled provider/category pairs. Use
`pathresolver.ResolveFor`, preserve all alias targets, and key non-shared
resources by `<provider>/<id>`. Key shared resources by
`common/<shared_as>`. Reject conflicting strategies or transformers for the
same shared key.

Custom resources use provider ID `custom`, portable layout, and no installer.
Resolve the current platform's `Paths` entry as the collection root and its
`Targets` entry as the restore target. Missing current-platform target mappings
remain visible in preview as `target-mapping-required` and are not restored.
Assign transformer `generic-safe` to custom `structured-merge` resources; it
synchronizes the whole canonical document only when generic scanning proves the
entire document safe.
Do not remove the old `BuildSpecs` function until Task 6 switches the engine.

- [ ] **Step 6: Run tests**

```powershell
go test ./internal/resource ./internal/cli ./internal/pathresolver
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/resource internal/cli/runtime.go internal/cli/runtime_test.go
git commit -m "feat: resolve portable resource paths"
```

## Task 3: Collect Immutable Source Artifacts and Shared Links

**Files:**
- Create: `internal/resource/filter.go`
- Create: `internal/resource/filter_test.go`
- Create: `internal/resourcecollect/collector.go`
- Create: `internal/resourcecollect/collector_test.go`
- Create: `internal/resourcecollect/links.go`
- Create: `internal/resourcecollect/links_test.go`

- [ ] **Step 1: Write failing filter tests**

Verify `node_modules`, `.vscode-test`, `.venv`, `__pycache__`, active databases,
logs, locks, VCS internals, and declared generated output are excluded. Verify a
49 MiB source asset is allowed and a 50 MiB file is blocked with reason
`file-too-large`.

```go
func TestDefaultPolicyBlocksGeneratedAndOversizedFiles(t *testing.T) {
	policy := DefaultFilterPolicy()
	cases := map[string]bool{
		"skill/SKILL.md": true,
		"skill/node_modules/pkg/index.js": false,
		"skill/scripts/.vscode-test/code.exe": false,
		"skill/.venv/pyvenv.cfg": false,
		"skill/state.db-wal": false,
	}
	for path, want := range cases {
		if got := policy.Allows(path, 1024); got != want {
			t.Errorf("Allows(%q) = %v, want %v", path, got, want)
		}
	}
	if policy.Allows("skill/archive.bin", 50<<20) {
		t.Fatal("50 MiB artifact must be blocked")
	}
}
```

- [ ] **Step 2: Write failing immutable-stage and link tests**

The test must mutate the original file after collection and prove the staged
artifact retains the scanned bytes. Add a link test that creates two root links
to one target and asserts one artifact plus two targets. On Windows, skip only
when `os.Symlink` returns a privilege error.

```go
func TestCollectorStagesExactScannedBytes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(source, []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := resource.Spec{
		Key: "common/common-skills", Provider: "common", ID: "skills",
		Category: resource.CategorySkills, Strategy: resource.StrategySourceTree,
		Layout: resource.LayoutPortable, Root: root, Targets: []string{root},
		Include: []string{"**"},
	}
	result, err := New(Options{
		StageParent: t.TempDir(),
		GOOS: "linux",
		UserHome: "/home/alice",
	}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"accessToken":"changed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	artifact := result.Artifacts["agents/_portable/config/common/skills/common-skills/SKILL.md"]
	got, err := os.ReadFile(artifact.StagePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "safe" {
		t.Fatalf("staged bytes = %q", got)
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

```powershell
go test ./internal/resource ./internal/resourcecollect
```

Expected: FAIL because filtering and immutable collection are absent.

- [ ] **Step 4: Implement filtering**

Create `FilterPolicy` with `MaxFileSize`, generated globs, and executable
extensions. `Allows` must normalize to forward slashes and fail closed on
invalid paths. Do not blanket-exclude `dist` or `build`.

```go
const DefaultMaxFileSize int64 = 50 << 20

var generatedGlobs = []string{
	"**/.git/**", "**/.svn/**", "**/node_modules/**",
	"**/.venv/**", "**/venv/**", "**/__pycache__/**",
	"**/.vscode-test/**", "**/coverage/**", "**/.cache/**",
	"**/*.db", "**/*.db-*", "**/*.sqlite", "**/*.sqlite-*",
	"**/*.log", "**/*.lock", "**/*.tmp",
}
```

Executable extensions are blocked for source-tree resources:
`.exe`, `.dll`, `.so`, `.dylib`, and `.node`. File-tree sessions retain their
existing explicit allowlist behavior.

- [ ] **Step 5: Implement link resolution and alias identity**

`ResolveRoot` returns `CanonicalRoot`, `OriginalRoot`, and a stable identity.
Use `filepath.EvalSymlinks` for a root link. Reject nested links that resolve
outside the canonical root unless their exact target appears in a local
approved-link store. Never store absolute targets in repository artifacts.

Coalesce specs sharing `SharedAs` before walking. Preserve all original targets
for restore.

- [ ] **Step 6: Implement immutable collection**

Define:

```go
type Projector interface {
	Project(transformer, rel, goos, home string, data []byte) ([]byte, error)
}

type InventoryProvider interface {
	Inventory(spec resource.Spec) (map[string][]byte, error)
}

type Options struct {
	StageParent string
	GOOS        string
	UserHome    string
	Projector   Projector
	Inventory   InventoryProvider
	Filter      resource.FilterPolicy
}

type Collector struct {
	options Options
}

func New(options Options) *Collector {
	if options.Filter.MaxFileSize == 0 {
		options.Filter = resource.DefaultFilterPolicy()
	}
	return &Collector{options: options}
}

type Artifact struct {
	RepoRel     string
	ResourceKey string
	Relative    string
	StagePath   string
	Targets     []string
	Hash        string
	Size        int64
	ModTime     int64
}

type Result struct {
	Snapshot  state.Snapshot
	Artifacts map[string]Artifact
	Blocked   []resource.Issue
	Skipped   []resource.Issue
	StageRoot string
}
```

Create a unique stage beneath the caller-provided stage parent, copy or project
selected bytes into it, scan the staged bytes, hash those same bytes, and return
only stage paths as upload sources. `StrategyInstallManifest` calls the injected
`InventoryProvider` and stages its returned logical filenames and bytes instead
of walking installed payload directories. When no inventory provider exists,
record an `installer-unavailable` skipped issue. A selected-file read,
projection, or scan failure records an issue for that resource and continues
with other resources; stage-directory creation and cleanup failures return an
error. `Result.Close()` removes that exact stage directory.

- [ ] **Step 7: Run tests**

```powershell
go test ./internal/resource ./internal/resourcecollect ./internal/secret
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/resource internal/resourcecollect
git commit -m "feat: stage portable resources safely"
```

## Task 4: Add Safe Structured Configuration Projection

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/portableconfig/codec.go`
- Create: `internal/portableconfig/document.go`
- Create: `internal/portableconfig/policy.go`
- Create: `internal/portableconfig/codec_test.go`
- Modify: `internal/pathresolver/pathresolver.go`
- Modify: `internal/pathresolver/pathresolver_test.go`

- [ ] **Step 1: Add TOML support using the ecosystem parser**

Run:

```powershell
go get github.com/pelletier/go-toml/v2@v2.4.3
```

Expected: `go.mod` and `go.sum` add `github.com/pelletier/go-toml/v2`.

- [ ] **Step 2: Write failing projection and restore tests**

```go
func TestCodecProjectsSafeFieldsAndRestoresWithoutReplacingCredentials(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{
		Portable:    []string{"**"},
		Sensitive:   []string{"accessToken", "auth.**"},
		MachineLocal: []string{"installationId", "recentWorkspaces"},
		PathFields:  []string{"paths.skills"},
	})
	local := []byte(`{
	  "theme":"dark",
	  "accessToken":"local-secret",
	  "installationId":"machine-a",
	  "paths":{"skills":"C:\\Users\\alice\\.agents\\skills"}
	}`)
	projected, err := registry.Project("demo", "settings.json", "windows", `C:\Users\alice`, local)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(projected, []byte("local-secret")) ||
		bytes.Contains(projected, []byte("machine-a")) {
		t.Fatalf("unsafe projection: %s", projected)
	}
	remote := []byte(`{"theme":"light","paths":{"skills":"${HOME}/.agents/skills"}}`)
	restored, err := registry.Restore(
		"demo", "settings.json", "windows", `C:\Users\alice`,
		local, projected, remote,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(restored, []byte(`"accessToken": "local-secret"`)) ||
		!bytes.Contains(restored, []byte(`"theme": "light"`)) {
		t.Fatalf("restored = %s", restored)
	}
}
```

Add this table test for YAML and TOML, then separate malformed-input, portable
key deletion, and missing-local-file tests:

```go
func TestCodecProjectsAndRestoresYAMLAndTOML(t *testing.T) {
	tests := []struct {
		name  string
		rel   string
		local []byte
		want  string
	}{
		{
			name: "yaml", rel: "settings.yaml",
			local: []byte("theme: dark\naccessToken: local-secret\n"),
			want: "theme: light",
		},
		{
			name: "toml", rel: "settings.toml",
			local: []byte("theme = 'dark'\naccessToken = 'local-secret'\n"),
			want: `theme = 'light'`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewRegistry()
			registry.Register("demo", Policy{
				Portable: []string{"**"}, Sensitive: []string{"accessToken"},
			})
			projected, err := registry.Project("demo", test.rel, "linux", "/home/alice", test.local)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(projected, []byte("local-secret")) {
				t.Fatalf("projection leaked secret: %s", projected)
			}
			remote := bytes.ReplaceAll(projected, []byte("dark"), []byte("light"))
			restored, err := registry.Restore(
				"demo", test.rel, "linux", "/home/alice",
				test.local, projected, remote,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(restored, []byte(test.want)) ||
				!bytes.Contains(restored, []byte("local-secret")) {
				t.Fatalf("restored = %s", restored)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

```powershell
go test ./internal/portableconfig ./internal/pathresolver
```

Expected: FAIL because the codec and home-token helpers do not exist.

- [ ] **Step 4: Implement portable path tokenization**

Add:

```go
const HomeToken = "${HOME}"

func TokenizeHome(value, goos, home string) (string, bool)
func ExpandHomeToken(value, goos, home string) (string, error)
```

Only replace a clean path equal to or below `home`. Reject traversal and do not
rewrite arbitrary string fields.

- [ ] **Step 5: Implement document parsing**

Use extension-based JSON, YAML, and TOML parsers. Normalize mappings into
`map[string]any`; reject non-string mapping keys. Marshal deterministically with
two-space JSON indentation, YAML indentation of two, and the TOML encoder.

```go
type Format string

const (
	JSON Format = "json"
	YAML Format = "yaml"
	TOML Format = "toml"
)

func Parse(rel string, data []byte) (Format, map[string]any, error)
func Marshal(format Format, value map[string]any) ([]byte, error)
```

- [ ] **Step 6: Implement policy projection and restore**

Use doublestar matching over dot-separated key paths. Projection copies only
portable paths, removes sensitive and machine-local paths, and tokenizes only
declared path fields. Restore removes portable keys deleted between the base
and remote projection, overlays remote portable keys, and leaves sensitive and
machine-local local keys untouched.

After projection, run the existing generic secret scanner. If scanning or
parsing fails, return an error; do not emit a partial projection.

- [ ] **Step 7: Connect the codec to `resourcecollect.Projector`**

Add an adapter method matching:

```go
func (r *Registry) Project(transformer, rel, goos, home string, data []byte) ([]byte, error)
```

An empty transformer returns an immutable copy only for non-structured
strategies. A structured resource without a registered transformer is an error.
`generic-safe` parses the document, runs the generic secret scanner, and returns
the whole canonical document only when it contains no sensitive fields.

- [ ] **Step 8: Run tests**

```powershell
go test ./internal/portableconfig ./internal/pathresolver ./internal/resourcecollect ./internal/secret
```

Expected: PASS.

- [ ] **Step 10: Commit**

```powershell
git add go.mod go.sum internal/portableconfig internal/pathresolver internal/resourcecollect
git commit -m "feat: project portable configuration safely"
```

## Task 5: Persist Base Bytes and Preserve True Conflicts

**Files:**
- Create: `internal/state/base.go`
- Create: `internal/state/base_test.go`
- Create: `internal/portablemerge/merge.go`
- Create: `internal/portablemerge/merge_test.go`
- Create: `internal/conflict/store.go`
- Create: `internal/conflict/store_test.go`
- Modify: `internal/syncengine/reconcile.go`
- Modify: `internal/syncengine/reconcile_test.go`

- [ ] **Step 1: Write failing base-store tests**

```go
func TestBaseStoreCapturesAndLoadsByRepoPath(t *testing.T) {
	store := NewBaseStore(t.TempDir())
	if err := store.Put("agents/_portable/config/providers/demo/config/settings/settings.json", []byte("base")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get("agents/_portable/config/providers/demo/config/settings/settings.json")
	if err != nil || !ok || string(got) != "base" {
		t.Fatalf("Get = %q, %v, %v", got, ok, err)
	}
}
```

Store blobs under `base/objects/<sha256>` and a path-to-hash index under
`base/index.json`. Write the index atomically.

- [ ] **Step 2: Replace the LWW test with merge/conflict tests**

```go
func TestReconcileBothChangedRequestsMergeInsteadOfLWW(t *testing.T) {
	base := state.Snapshot{"p": meta("base", 5)}
	local := state.Snapshot{"p": meta("local", 20)}
	remote := state.Snapshot{"p": meta("remote", 10)}
	action := find(Reconcile(base, local, remote), "p")
	if action == nil || action.Type != MergeBoth {
		t.Fatalf("action = %#v", action)
	}
}
```

Keep existing create, pull, delete, blocked, and same-content tests unchanged.

- [ ] **Step 3: Write failing structured and text merge tests**

Test independent JSON keys, same-key conflicts, non-overlapping text edits,
overlapping text edits, binary conflicts, and delete-versus-modify conflicts.

```go
func TestStructuredMergeCombinesIndependentKeys(t *testing.T) {
	result, err := Structured(
		[]byte(`{"theme":"dark","font":12}`),
		[]byte(`{"theme":"light","font":12}`),
		[]byte(`{"theme":"dark","font":14}`),
	)
	if err != nil || result.Conflict {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if string(result.Data) != "{\n  \"font\": 14,\n  \"theme\": \"light\"\n}\n" {
		t.Fatalf("merged = %s", result.Data)
	}
}
```

- [ ] **Step 4: Run tests and verify failure**

```powershell
go test ./internal/state ./internal/portablemerge ./internal/conflict ./internal/syncengine
```

Expected: FAIL because base storage, merge results, conflict storage, and
`MergeBoth` do not exist.

- [ ] **Step 5: Implement the base store**

Use SHA-256 content addressing, canonical forward-slash repo paths, atomic
index writes, and explicit `Get`, `Put`, `Delete`, and `CaptureRepo` methods.
`CaptureRepo` reads only paths in the successful synchronized snapshot.

- [ ] **Step 6: Implement conservative merges**

Define:

```go
type Result struct {
	Data     []byte
	Conflict bool
}

func Structured(base, local, remote []byte) (Result, error)

type TextMerger interface {
	Merge(base, local, remote []byte) (Result, error)
}
```

Structured merge recursively applies one-side changes and reports conflict when
both sides changed the same scalar or changed a map into incompatible types.

Implement `GitTextMerger` by writing three temp files and invoking:

```text
git merge-file --stdout <local> <base> <remote>
```

Exit 0 returns merged bytes. Exit 1 returns `Conflict: true` without treating it
as an infrastructure error. Any other exit is an error with stderr included.
Never write conflict-marker output into an active agent file.

- [ ] **Step 7: Implement conflict bundles**

```go
type Record struct {
	ID          string    `json:"id"`
	ResourceKey string    `json:"resourceKey"`
	RepoRel     string    `json:"repoRel"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Scanner func(record Record, variant string, data []byte) error

func NewStore(localRoot, repoDir string, scanner Scanner) *Store
func (s *Store) Create(record Record, base, local, remote []byte) error
func (s *Store) List() ([]Record, error)
func (s *Store) Resolve(id string, merged []byte) error
```

Store local bundles under `<acsync-home>/conflicts/<id>/` and repository-safe
bundles under
`agents/_portable/config/conflicts/<id>/{record.json,base,local,remote}`.
Scan every variant before writing the repository bundle. Reject path traversal.

- [ ] **Step 8: Add `MergeBoth` to reconciliation**

Add `MergeBoth` to `ActionType`. Both-modified and delete-versus-modify cases
must produce `MergeBoth`; same content remains no-op. Do not use mtime for typed
resource conflicts.

- [ ] **Step 9: Run tests**

```powershell
go test ./internal/state ./internal/portablemerge ./internal/conflict ./internal/syncengine
```

Expected: PASS.

- [ ] **Step 10: Commit**

```powershell
git add internal/state internal/portablemerge internal/conflict internal/syncengine/reconcile*
git commit -m "feat: preserve concurrent resource edits"
```

## Task 6: Integrate Typed Resources into the Sync Engine

**Files:**
- Create: `internal/syncengine/resources.go`
- Create: `internal/syncengine/resources_test.go`
- Create: `internal/syncengine/resource_apply.go`
- Create: `internal/syncengine/resource_apply_test.go`
- Modify: `internal/syncengine/engine.go`
- Modify: `internal/syncengine/engine_test.go`
- Modify: `internal/syncengine/collect.go`
- Modify: `internal/syncengine/apply.go`
- Modify: `internal/syncengine/cleanup.go`
- Modify: `internal/syncengine/cleanup_test.go`
- Modify: `internal/syncengine/models.go`
- Modify: `internal/cli/runtime.go`
- Modify: `internal/cli/sync.go`
- Modify: `internal/cli/sync_test.go`
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/daemon_test.go`

- [ ] **Step 1: Write failing remote-ownership compatibility tests**

```go
func TestUnknownProvidersRemainUntouchedButPortablePathsAreOwned(t *testing.T) {
	remote := state.Snapshot{
		"agents/disabled/config/settings.json": {},
		"agents/_portable/config/providers/claude/instructions/global/CLAUDE.md": {},
	}
	specs := map[string]resource.Spec{
		"claude/global": {
			Key: "claude/global", Provider: "claude", ID: "global",
			Category: resource.CategoryInstructions, Layout: resource.LayoutPortable,
		},
	}
	owned, untouched := SplitRemoteSnapshot(remote, specs)
	if len(owned) != 1 || len(untouched) != 1 {
		t.Fatalf("owned=%#v untouched=%#v", owned, untouched)
	}
}
```

The disabled provider path must never become a delete action. The `_portable`
path must be scanned and reconciled by the matching typed resource.

- [ ] **Step 2: Write failing strategy-aware apply tests**

Cover raw text restore, structured restore retaining a local token, staged
upload, local deletion to the recovery area, common alias link creation, and
managed-copy fallback.

```go
func TestResourceApplierRestoresStructuredProjection(t *testing.T) {
	local := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, local, `{"theme":"dark","accessToken":"local"}`)
	remote := []byte(`{"theme":"light"}`)
	codecs := portableconfig.NewRegistry()
	codecs.Register("demo", portableconfig.Policy{
		Portable: []string{"**"},
		Sensitive: []string{"accessToken"},
	})
	applier := ResourceApplier{
		Codecs: codecs,
		Base:   state.NewBaseStore(t.TempDir()),
	}
	err := applier.Restore(resource.Spec{
		Key: "demo/settings", Strategy: resource.StrategyStructuredMerge,
		Transformer: "demo", Targets: []string{filepath.Dir(local)},
	}, "settings.json", remote)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(local)
	if !bytes.Contains(got, []byte(`"accessToken": "local"`)) {
		t.Fatalf("credential was replaced: %s", got)
	}
}
```

- [ ] **Step 3: Write an engine integration test for legacy plus portable data**

Machine A creates a legacy session and a portable instruction. Machine B pulls
both. B changes the instruction while A changes a non-overlapping line; both
sync and receive the merged file. Assert the portable path is below
`agents/_portable/config`.

- [ ] **Step 4: Run tests and verify failure**

```powershell
go test ./internal/syncengine ./internal/cli
```

Expected: FAIL because the engine still accepts only `AgentSpec`.

- [ ] **Step 5: Implement resource ownership and scanning**

`SplitRemoteSnapshot` must:

- match legacy paths only to enabled legacy specs;
- match `_portable` provider/common paths by stable resource key;
- treat conflict and install metadata as internal owned paths;
- leave unknown or disabled providers untouched;
- reject malformed `_portable` paths as blocked.

Remote bytes pass the owning resource scanner and, for structured config, parse
through the registered codec before reconciliation.

- [ ] **Step 6: Implement strategy-aware apply**

Use staged artifacts for push. For pull:

- file/text/source trees copy atomically to each target;
- structured config calls codec restore with local original, base projection,
  and remote projection;
- shared resources materialize once and link aliases;
- link permission failure creates managed copies and records them in
  `<acsync-home>/aliases.json`;
- deletes move active Skill/Plugin source to
  `<acsync-home>/local-trash/<timestamp>/<resource-key>/` before removal.

All target paths must remain below the resolved target root after `Join` and
`Clean`.

- [ ] **Step 7: Update the engine orchestration**

Replace `Engine.Specs map[string]AgentSpec` with:

```go
type Engine struct {
	Git          *gitclient.Client
	RepoDir      string
	StatePath    string
	Home         string
	GOOS         string
	UserHome     string
	Resources    map[string]resource.Spec
	Codecs       *portableconfig.Registry
	Base         *state.BaseStore
	Conflicts    *conflict.Store
	PushRetries  int
	Now          func() time.Time
	OnProgress   func(Progress)
}
```

Collect into `<repo>/.git/acsync-stage`, defer exact stage cleanup, split owned
remote state, reconcile, resolve `MergeBoth`, apply non-conflicting actions,
commit/push, then capture successful base bytes. Unresolved conflicts do not
advance the canonical file or its base.

Extend `Result` and `Progress` with `Restored`, `Reinstalled`, `Skipped`, and
`Conflicts` counts while retaining existing fields. File read, projection,
scan, merge, restore, and install errors are attached to their stable resource
as `resource.Issue`; unrelated resources continue. Stage creation, state-store,
Git, and repository-wide failures still return an error immediately.

- [ ] **Step 8: Preserve secure deletion and trash behavior**

Update cleanup validation to accept canonical `_portable` paths and conflict
bundles only when they pass parser and scanner checks. Blocked files are still
deleted permanently rather than copied to trash. Deletion is suppressed for a
resource with an unresolved conflict.

- [ ] **Step 9: Propagate partial failure through the daemon**

Extend `daemon.CycleResult` with `Skipped`, `Conflicts`, `PendingInstalls`, and
`NeedsAttention`. A successful sync with resource issues still runs cleanup and
publishes `StageComplete`, but its label is `Synchronization needs attention`
and `NeedsAttention` is true. Cleanup or infrastructure failure still suppresses
the complete event.

Add a daemon test where `sync` returns one restored resource plus one blocked
resource issue. Assert cleanup runs, the complete event is emitted, and
`CycleResult.NeedsAttention` is true.

- [ ] **Step 10: Switch CLI runtime to resource specs**

`runSyncWithUserHome` calls `BuildResourceSpecs`, creates the codec registry,
base store, and conflict store from `home`, and passes them to `Engine`.
Remove `BuildSpecs` and `AgentSpec` only after all references and tests migrate.

- [ ] **Step 11: Run targeted tests**

```powershell
go test ./internal/resource/... ./internal/portableconfig ./internal/portablemerge ./internal/conflict ./internal/state ./internal/syncengine ./internal/cli ./internal/daemon
```

Expected: PASS.

- [ ] **Step 12: Commit**

```powershell
git add internal/syncengine internal/cli internal/resource internal/resourcecollect internal/state internal/daemon
git commit -m "feat: sync typed portable resources"
```

## Task 7: Define Built-in Agent and Common Resources

**Files:**
- Create: `internal/provider/builtin/common.yaml`
- Modify: `internal/provider/builtin/claude.yaml`
- Modify: `internal/provider/builtin/copilot.yaml`
- Modify: `internal/provider/builtin/gemini.yaml`
- Modify: `internal/provider/builtin/vscode-copilot.yaml`
- Modify: `internal/provider/builtin/cursor.yaml`
- Create: `internal/portableconfig/policies.go`
- Modify: `internal/provider/provider_test.go`
- Modify: `internal/portableconfig/codec_test.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing built-in coverage tests**

Assert every built-in is schema v2, resource IDs are unique, all five
categories exist where supported, common Skills use `shared_as:
common-skills`, structured resources reference registered transformers, and no
built-in selects credential, database, cache, or runtime paths.

```go
func TestBuiltinsDeclarePortableResources(t *testing.T) {
	providers, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Provider{}
	for _, p := range providers {
		byName[p.Name] = p
	}
	for _, name := range []string{"claude", "copilot", "gemini", "vscode-copilot", "cursor", "common"} {
		if byName[name].SchemaVersion != 2 {
			t.Errorf("%s schema = %d", name, byName[name].SchemaVersion)
		}
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

```powershell
go test ./internal/provider ./internal/portableconfig ./internal/config
```

Expected: FAIL because built-ins remain v1 and policies are not registered.

- [ ] **Step 3: Convert Claude and Copilot manifests**

Use these resource IDs and selections:

```yaml
schema_version: 2
name: claude
resources:
  - id: legacy-config
    category: config
    paths: {windows: "%USERPROFILE%\\.claude", darwin: "~/.claude", linux: "~/.claude"}
    include: ["settings.json", "CLAUDE.md"]
    exclude: ["**/.credentials.json", "**/*token*", "**/shell-snapshots/**"]
    strategy: file-tree
    layout: legacy
  - id: sessions
    category: sessions
    paths: {windows: "%USERPROFILE%\\.claude", darwin: "~/.claude", linux: "~/.claude"}
    include: ["history.jsonl", "sessions/**/*.json", "projects/**/sessions-index.json", "projects/**/*.jsonl"]
    exclude: ["**/.credentials.json", "**/*token*", "**/shell-snapshots/**"]
    strategy: file-tree
    layout: legacy
  - id: settings
    category: config
    paths: {windows: "%USERPROFILE%\\.claude", darwin: "~/.claude", linux: "~/.claude"}
    include: ["settings.json", "settings.local.json", "config.json", "parallel-agents.json"]
    strategy: structured-merge
    transformer: claude-settings
  - id: instructions
    category: instructions
    paths: {windows: "%USERPROFILE%\\.claude", darwin: "~/.claude", linux: "~/.claude"}
    include: ["CLAUDE.md", "commands/**", "prompts/**", "README.md"]
    strategy: text-tree
  - id: skills
    category: skills
    paths: {windows: "%USERPROFILE%\\.claude\\skills", darwin: "~/.claude/skills", linux: "~/.claude/skills"}
    include: ["**"]
    strategy: source-tree
    shared_as: common-skills
  - id: plugins
    category: plugins
    paths: {windows: "%USERPROFILE%\\.claude\\plugins", darwin: "~/.claude/plugins", linux: "~/.claude/plugins"}
    strategy: install-manifest
    installer: claude-plugin
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
```

Use this Copilot manifest:

```yaml
schema_version: 2
name: copilot
resources:
  - id: legacy-config
    category: config
    paths: {windows: "%USERPROFILE%\\.copilot", darwin: "~/.copilot", linux: "~/.copilot"}
    include: ["config.json"]
    exclude: &copilot_excludes ["pkg/**", "logs/**", "media-cache/**", "servers/**", "**/*token*", "**/hosts.json", "**/*.lock", "**/*.db", "**/*.db-*"]
    strategy: file-tree
    layout: legacy
  - id: sessions
    category: sessions
    paths: {windows: "%USERPROFILE%\\.copilot", darwin: "~/.copilot", linux: "~/.copilot"}
    include: ["session-state/*/events.jsonl", "session-state/*/workspace.yaml", "session-state/*/vscode.metadata.json", "session-state/*/vscode.requests.metadata.json", "session-state/*/plan.md", "session-state/*/checkpoints/**/*.md"]
    exclude: *copilot_excludes
    strategy: file-tree
    layout: legacy
  - id: settings
    category: config
    paths: {windows: "%USERPROFILE%\\.copilot", darwin: "~/.copilot", linux: "~/.copilot"}
    include: ["config.json", "settings.json", "permissions-config.json"]
    exclude: *copilot_excludes
    strategy: structured-merge
    transformer: copilot-settings
  - id: instructions
    category: instructions
    paths: {windows: "%USERPROFILE%\\.copilot", darwin: "~/.copilot", linux: "~/.copilot"}
    include: ["README.md", "prompts/**", "instructions/**"]
    exclude: *copilot_excludes
    strategy: text-tree
  - id: skills
    category: skills
    paths: {windows: "%USERPROFILE%\\.copilot\\skills", darwin: "~/.copilot/skills", linux: "~/.copilot/skills"}
    include: ["**"]
    strategy: source-tree
    shared_as: common-skills
  - id: plugins
    category: plugins
    paths: {windows: "%USERPROFILE%\\.copilot\\installed-plugins", darwin: "~/.copilot/installed-plugins", linux: "~/.copilot/installed-plugins"}
    strategy: install-manifest
    installer: copilot-plugin
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
```

- [ ] **Step 4: Convert Gemini, VS Code Copilot, Cursor, and Common manifests**

Use these complete manifests:

```yaml
# internal/provider/builtin/gemini.yaml
schema_version: 2
name: gemini
resources:
  - id: sessions
    category: sessions
    paths: {windows: "%USERPROFILE%\\.gemini", darwin: "~/.gemini", linux: "~/.gemini"}
    include: ["sessions/**/*.json", "tmp/**/chats/**/*.jsonl", "history/**/*.json"]
    exclude: &gemini_excludes ["**/*token*", "**/oauth_creds.json", "**/google_accounts.json", "**/installation_id", "**/state.json", "**/trustedFolders.json"]
    strategy: file-tree
    layout: legacy
  - id: settings
    category: config
    paths: {windows: "%USERPROFILE%\\.gemini", darwin: "~/.gemini", linux: "~/.gemini"}
    include: ["settings.json"]
    exclude: *gemini_excludes
    strategy: structured-merge
    transformer: gemini-settings
  - id: instructions
    category: instructions
    paths: {windows: "%USERPROFILE%\\.gemini", darwin: "~/.gemini", linux: "~/.gemini"}
    include: ["GEMINI.md", "commands/**", "prompts/**", "README.md"]
    exclude: *gemini_excludes
    strategy: text-tree
  - id: skills
    category: skills
    paths: {windows: "%USERPROFILE%\\.gemini\\skills", darwin: "~/.gemini/skills", linux: "~/.gemini/skills"}
    include: ["**"]
    strategy: source-tree
    shared_as: common-skills
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
---
# internal/provider/builtin/vscode-copilot.yaml
schema_version: 2
name: vscode-copilot
resources:
  - id: sessions
    category: sessions
    paths:
      windows: "%USERPROFILE%\\AppData\\Roaming\\Code\\User\\workspaceStorage"
      darwin: "~/Library/Application Support/Code/User/workspaceStorage"
      linux: "~/.config/Code/User/workspaceStorage"
    include: ["*/workspace.json", "*/chatSessions/*.json", "*/chatSessions/*.jsonl"]
    exclude: &vscode_excludes ["**/*token*", "**/chatEditingSessions/**", "**/state.vscdb*", "**/*.db", "**/*.sqlite"]
    strategy: file-tree
    layout: legacy
  - id: settings
    category: config
    paths:
      windows: "%USERPROFILE%\\AppData\\Roaming\\Code\\User"
      darwin: "~/Library/Application Support/Code/User"
      linux: "~/.config/Code/User"
    include: ["settings.json", "profiles/*/settings.json"]
    exclude: *vscode_excludes
    strategy: structured-merge
    transformer: vscode-settings
  - id: mcp
    category: config
    paths:
      windows: "%USERPROFILE%\\AppData\\Roaming\\Code\\User"
      darwin: "~/Library/Application Support/Code/User"
      linux: "~/.config/Code/User"
    include: ["mcp.json", "profiles/*/mcp.json"]
    exclude: *vscode_excludes
    strategy: structured-merge
    transformer: vscode-mcp
  - id: user-files
    category: config
    paths:
      windows: "%USERPROFILE%\\AppData\\Roaming\\Code\\User"
      darwin: "~/Library/Application Support/Code/User"
      linux: "~/.config/Code/User"
    include: ["keybindings.json", "snippets/**", "profiles/*/keybindings.json", "profiles/*/snippets/**"]
    exclude: *vscode_excludes
    strategy: file-tree
  - id: instructions
    category: instructions
    paths:
      windows: "%USERPROFILE%\\AppData\\Roaming\\Code\\User"
      darwin: "~/Library/Application Support/Code/User"
      linux: "~/.config/Code/User"
    include: ["prompts/**", "profiles/*/prompts/**"]
    exclude: *vscode_excludes
    strategy: text-tree
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
---
# internal/provider/builtin/cursor.yaml
schema_version: 2
name: cursor
resources:
  - id: sessions
    category: sessions
    paths: {windows: "%USERPROFILE%\\.cursor", darwin: "~/.cursor", linux: "~/.cursor"}
    include: ["chats/**/*.json"]
    exclude: &cursor_excludes ["**/*token*", "**/*.db", "**/*.db-*", "**/Cache/**"]
    strategy: file-tree
    layout: legacy
  - id: settings
    category: config
    paths: {windows: "%USERPROFILE%\\.cursor", darwin: "~/.cursor", linux: "~/.cursor"}
    include: ["settings.json"]
    exclude: *cursor_excludes
    strategy: structured-merge
    transformer: cursor-settings
  - id: instructions
    category: instructions
    paths: {windows: "%USERPROFILE%\\.cursor", darwin: "~/.cursor", linux: "~/.cursor"}
    include: ["rules/**", "prompts/**", "README.md"]
    exclude: *cursor_excludes
    strategy: text-tree
  - id: skills
    category: skills
    paths: {windows: "%USERPROFILE%\\.cursor\\skills", darwin: "~/.cursor/skills", linux: "~/.cursor/skills"}
    include: ["**"]
    strategy: source-tree
    shared_as: common-skills
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
---
# internal/provider/builtin/common.yaml
schema_version: 2
name: common
resources:
  - id: skills
    category: skills
    paths: {windows: "%USERPROFILE%\\.agents\\skills", darwin: "~/.agents/skills", linux: "~/.agents/skills"}
    include: ["**"]
    exclude: &common_excludes ["**/*.bak.*", "**/node_modules/**", "**/.vscode-test/**", "**/.venv/**", "**/__pycache__/**"]
    strategy: source-tree
    shared_as: common-skills
  - id: hooks
    category: instructions
    paths: {windows: "%USERPROFILE%\\.agents", darwin: "~/.agents", linux: "~/.agents"}
    include: ["AGENTS.md", "README.md", "hooks/**"]
    exclude: *common_excludes
    strategy: text-tree
  - id: skill-lock
    category: config
    paths: {windows: "%USERPROFILE%\\.agents", darwin: "~/.agents", linux: "~/.agents"}
    include: [".skill-lock.json", ".copilot-for-azure-skills-manifest.json"]
    exclude: *common_excludes
    strategy: structured-merge
    transformer: common-skill-lock
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
```

- [ ] **Step 5: Register built-in safe-field policies**

Create a registry constructor:

```go
func BuiltinRegistry() *Registry {
	r := NewRegistry()
	r.Register("claude-settings", Policy{
		Portable: []string{"**"},
		Sensitive: []string{
			"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*",
		},
		MachineLocal: []string{"installationId", "recentWorkspaces", "cache.**"},
		PathFields: []string{"**.*Path", "**.*Directory"},
	})
	r.Register("copilot-settings", Policy{
		Portable: []string{"**"},
		Sensitive: []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*", "hosts.**"},
		MachineLocal: []string{"installationId", "recentRepositories", "cache.**", "session.**"},
		PathFields: []string{"**.*Path", "**.*Directory"},
	})
	r.Register("gemini-settings", Policy{
		Portable: []string{"theme", "model", "tools.**", "mcpServers.**", "context.**"},
		Sensitive: []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*", "accounts.**"},
		MachineLocal: []string{"installationId", "trustedFolders", "recentProjects", "state.**"},
		PathFields: []string{"context.fileName"},
	})
	r.Register("vscode-settings", Policy{
		Portable: []string{"**"},
		Sensitive: []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*"},
		MachineLocal: []string{"window.restoreWindows", "workbench.localHistory.**", "recentlyOpened.**"},
		PathFields: []string{"**.*Path", "**.*Directory"},
	})
	r.Register("vscode-mcp", Policy{
		Portable: []string{"servers.**.type", "servers.**.command", "servers.**.args", "servers.**.url"},
		Sensitive: []string{"servers.**.env.**", "servers.**.headers.**", "**.*token*", "**.*secret*", "**.*password*"},
		PathFields: []string{"servers.**.command", "servers.**.args"},
	})
	r.Register("cursor-settings", Policy{
		Portable: []string{"**"},
		Sensitive: []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*"},
		MachineLocal: []string{"installationId", "recentWorkspaces", "cache.**"},
		PathFields: []string{"**.*Path", "**.*Directory"},
	})
	r.Register("common-skill-lock", Policy{
		Portable: []string{"skills.**.name", "skills.**.source", "skills.**.version", "skills.**.revision"},
		Sensitive: []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*"},
	})
	return r
}
```

Each policy must have a positive safe-field fixture and a
credential/machine-field negative fixture in `codec_test.go`.

- [ ] **Step 6: Verify real-directory discovery without reading file contents**

Run a preview against the current user profile that prints only resource IDs,
counts, byte totals, and exclusions. It must show the Claude and Common Skills
roots as one shared resource and must not select Copilot `pkg`, VS Code
`globalStorage`, VS Code `chatEditingSessions`, or active databases.

Run:

```powershell
go test ./internal/provider ./internal/portableconfig ./internal/cli -run 'Builtin|Resource|Preview' -v
```

Expected: PASS with no file contents in output.

- [ ] **Step 7: Commit**

```powershell
git add internal/provider internal/portableconfig internal/config
git commit -m "feat: cover portable agent configuration"
```

## Task 8: Build Trusted Plugin and Skill Dependency Plans

**Files:**
- Create: `internal/installplan/model.go`
- Create: `internal/installplan/model_test.go`
- Create: `internal/installplan/discovery.go`
- Create: `internal/installplan/discovery_test.go`
- Create: `internal/installplan/claude.go`
- Create: `internal/installplan/claude_test.go`
- Create: `internal/installplan/copilot.go`
- Create: `internal/installplan/copilot_test.go`
- Create: `internal/installplan/store.go`
- Create: `internal/installplan/store_test.go`
- Create: `internal/installplan/executor.go`
- Create: `internal/installplan/executor_test.go`
- Modify: `internal/syncengine/engine.go`
- Modify: `internal/syncengine/engine_test.go`

- [ ] **Step 1: Write failing declaration and approval tests**

```go
func TestApprovalIsBoundToAdapterAndSource(t *testing.T) {
	store := NewStore(t.TempDir())
	op := Operation{
		ID: "copilot:wiqd@wiqd", Adapter: "copilot-plugin",
		Source: "wiqd@wiqd", Executable: "copilot",
		Args: []string{"plugin", "install", "wiqd@wiqd"},
	}
	if err := store.Approve(op); err != nil {
		t.Fatal(err)
	}
	if !store.IsApproved(op) {
		t.Fatal("approved operation was not recognized")
	}
	op.Source = "wiqd@other-marketplace"
	if store.IsApproved(op) {
		t.Fatal("source change must require new approval")
	}
}
```

Assert serialized declarations never contain executable or argv fields.

- [ ] **Step 2: Write failing discovery parser tests**

Use fixtures matching observed CLI output:

```go
var claudeListFixture = []byte(`[
  {
    "pluginId":"code-review@claude-plugins-official",
    "name":"code-review",
    "marketplaceName":"claude-plugins-official",
    "version":"1.2.0",
    "enabled":true
  }
]`)

var copilotListFixture = []byte(`
Installed plugins:
  • wiqd@wiqd (v0.6.0)
`)
```

Assert Claude marketplace JSON maps
`claude-plugins-official -> github:anthropics/claude-plugins-official` and
Copilot maps `wiqd@wiqd` to a declaration whose source remains
`wiqd@wiqd`.

- [ ] **Step 3: Write failing Skill dependency tests**

Use isolated Skill trees for:

- `package-lock.json` -> `npm ci`;
- `pnpm-lock.yaml` -> `pnpm install --frozen-lockfile`;
- `yarn.lock` -> `yarn install --immutable`;
- `uv.lock` -> `uv sync --frozen`;
- `requirements.txt` -> `python -m pip install -r requirements.txt`;
- `go.mod` -> `go mod download`.

Assert `node_modules` and `.vscode-test` never become declarations.

- [ ] **Step 4: Run tests and verify failure**

```powershell
go test ./internal/installplan
```

Expected: FAIL because the package does not exist.

- [ ] **Step 5: Implement trusted models and stores**

```go
type Declaration struct {
	ID       string            `json:"id"`
	Adapter  string            `json:"adapter"`
	Source   string            `json:"source"`
	Version  string            `json:"version,omitempty"`
	Enabled  bool              `json:"enabled"`
	Settings map[string]string `json:"settings,omitempty"`
}

type Operation struct {
	ID         string   `json:"id"`
	Adapter    string   `json:"adapter"`
	Source     string   `json:"source"`
	Kind       string   `json:"kind"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	WorkingDir string   `json:"workingDir,omitempty"`
}

type Plan struct {
	ID         string      `json:"id"`
	Operations []Operation `json:"operations"`
	Approved   bool        `json:"approved"`
}
```

Repository manifests contain `Declaration` only. `Operation` and approvals stay
under `<acsync-home>/install/`. Generate the plan ID from canonical
declarations and operation identities.

The local store exposes:

```go
func (s *Store) SavePending(plan Plan) error
func (s *Store) Pending() (*Plan, error)
func (s *Store) Approve(operation Operation) error
func (s *Store) IsApproved(operation Operation) bool
func (s *Store) ClearPending(id string) error
```

- [ ] **Step 6: Implement built-in adapters**

Use a narrow `Runner` interface with explicit executable, argv, working
directory, stdout, stderr, and exit error.

Claude discovery runs:

```text
claude plugin list --json
claude plugin marketplace list --json
```

Claude operations are generated as:

```text
claude plugin install <plugin@marketplace> --scope user
claude plugin update <plugin@marketplace> --scope user
claude plugin uninstall <plugin@marketplace> --scope user --keep-data
```

Copilot discovery runs `copilot plugin list`. Copilot operations are:

```text
copilot plugin install <source>
copilot plugin update <name@marketplace>
copilot plugin uninstall <name@marketplace>
```

Reject shell metacharacters and execute argv directly with `exec.CommandContext`;
never concatenate a command string.

Expose the adapters to collection through:

```go
type InventoryRegistry struct {
	adapters map[string]Adapter
}

func (r *InventoryRegistry) Inventory(spec resource.Spec) (map[string][]byte, error) {
	adapter, ok := r.adapters[spec.Installer]
	if !ok {
		return nil, fmt.Errorf("installer adapter %q is unavailable", spec.Installer)
	}
	declarations, err := adapter.Discover(context.Background(), spec)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(declarations, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string][]byte{"manifest.json": append(data, '\n')}, nil
}
```

`Adapter.Discover` returns declarations only. It never returns executable or
argv data.

- [ ] **Step 7: Implement dependency planning and execution**

Dependency operations are generated only from recognized lock/manifests in
approved Skill source. Before uninstall, copy the managed Plugin payload to
`<acsync-home>/local-trash/<timestamp>/plugins/<adapter>/<id>`. Run without
elevation. Missing executables leave operations pending with an actionable
error.

- [ ] **Step 8: Integrate pending plans into sync results**

Inject `InventoryRegistry` into `resourcecollect.Collector`. After source-tree
collection, run dependency discovery over staged Skill artifacts and stage the
canonical declaration list at
`agents/_portable/config/install/skill-dependencies.json`. After applying remote
declarations, compare desired declarations to local inventory. Save a pending
plan. Execute only operations whose adapter/source approval exists; first-use
operations remain pending. Add `PendingInstalls` and `Reinstalled` to
`syncengine.Result`.

- [ ] **Step 9: Run tests**

```powershell
go test ./internal/installplan ./internal/syncengine
```

Expected: PASS. Tests use fake runners and make no network calls.

- [ ] **Step 10: Commit**

```powershell
git add internal/installplan internal/syncengine
git commit -m "feat: restore plugins from trusted plans"
```

## Task 9: Expose Resource Preview, Settings, Conflicts, and Install Approval

**Files:**
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`
- Modify: `internal/desktop/service_test.go`
- Modify: `internal/desktop/wails.go`
- Modify: `internal/desktop/wails_test.go`
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`
- Modify: `internal/onboarding/service.go`
- Modify: `internal/onboarding/service_test.go`
- Modify: `main.go`
- Modify: `main_test.go`

- [ ] **Step 1: Write failing desktop model tests**

Assert `Snapshot` exposes resource categories, preview totals, install plan,
conflicts, and split result counts. Assert `SaveSettings` persists category
switches and custom resources.

```go
func configuredResourceService(t *testing.T) *Service {
	t.Helper()
	home := configuredHome(t)
	plans := installplan.NewStore(filepath.Join(home, "install"))
	if err := plans.SavePending(installplan.Plan{
		ID: "plan-1",
		Operations: []installplan.Operation{{
			ID: "copilot:wiqd@wiqd", Adapter: "copilot-plugin",
			Source: "wiqd@wiqd", Executable: "copilot",
			Args: []string{"plugin", "install", "wiqd@wiqd"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	conflicts := conflict.NewStore(filepath.Join(home, "conflicts"), filepath.Join(home, "repo"), nil)
	if err := conflicts.Create(conflict.Record{
		ID: "conflict-1", ResourceKey: "claude/settings",
		RepoRel: "agents/_portable/config/providers/claude/config/settings/settings.json",
	}, []byte(`{"theme":"dark"}`), []byte(`{"theme":"light"}`), []byte(`{"theme":"system"}`)); err != nil {
		t.Fatal(err)
	}
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func findAgent(agents []Agent, name string) Agent {
	for _, agent := range agents {
		if agent.Name == name {
			return agent
		}
	}
	return Agent{}
}

func TestSnapshotIncludesResourceCategoriesAndPendingWork(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()
	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	claude := findAgent(got.Agents, "claude")
	if len(claude.Resources) == 0 {
		t.Fatal("Claude resources missing")
	}
	if got.PendingInstallPlan == nil || len(got.Conflicts) != 1 {
		t.Fatalf("pending state = %#v", got)
	}
}
```

- [ ] **Step 2: Write failing Wails operation tests**

Cover:

```go
func (s *WailsService) ResourcePreview() (ResourcePreview, error)
func (s *WailsService) PreviewCustomResource(input CustomResourceInput) (ResourcePreview, error)
func (s *WailsService) ApproveInstallPlan(id string) error
func (s *WailsService) ResolveConflict(input ConflictResolution) error
```

Wrong plan IDs and conflict IDs return explicit errors and make no changes.

- [ ] **Step 3: Run tests and verify failure**

```powershell
go test ./internal/desktop ./internal/settings ./internal/onboarding .
```

Expected: FAIL because resource-facing desktop models and methods do not exist.

- [ ] **Step 4: Add desktop-safe models**

```go
type ResourceCategory struct {
	ID            string `json:"id"`
	Category      string `json:"category"`
	Enabled       bool   `json:"enabled"`
	Supported     bool   `json:"supported"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	FileCount     int    `json:"fileCount"`
	Bytes         int64  `json:"bytes"`
	ExcludedFiles int    `json:"excludedFiles"`
	ExcludedBytes int64  `json:"excludedBytes"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
}

type Agent struct {
	Name      string             `json:"name"`
	Enabled   bool               `json:"enabled"`
	Exclude   []string           `json:"exclude"`
	Resources []ResourceCategory `json:"resources"`
}

type ConflictResolution struct {
	ID      string `json:"id"`
	Choice  string `json:"choice"`
	Content string `json:"content,omitempty"`
}

type ResourcePreview struct {
	Resources     []ResourceCategory `json:"resources"`
	Files         int                `json:"files"`
	Bytes         int64              `json:"bytes"`
	ExcludedFiles int                `json:"excludedFiles"`
	ExcludedBytes int64              `json:"excludedBytes"`
	Issues        []ResourceIssue    `json:"issues"`
}

type ResourceIssue struct {
	ResourceKey string `json:"resourceKey"`
	Path        string `json:"path"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Bytes       int64  `json:"bytes"`
}

type InstallOperation struct {
	ID         string   `json:"id"`
	Adapter    string   `json:"adapter"`
	Source     string   `json:"source"`
	Kind       string   `json:"kind"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	WorkingDir string   `json:"workingDir"`
}

type InstallPlan struct {
	ID         string             `json:"id"`
	Operations []InstallOperation `json:"operations"`
}

type ConflictSummary struct {
	ID          string    `json:"id"`
	ResourceKey string    `json:"resourceKey"`
	Path        string    `json:"path"`
	CreatedAt   time.Time `json:"createdAt"`
}

type SettingsInput struct {
	RepositoryURL   string                         `json:"repositoryUrl"`
	IntervalMinutes int                            `json:"intervalMinutes"`
	TrashGraceDays  int                            `json:"trashGraceDays"`
	Agents          map[string]bool                `json:"agents"`
	Categories      map[string]map[string]bool     `json:"categories"`
	CustomResources []CustomResourceInput           `json:"customResources"`
}

type CustomResourceInput struct {
	ID       string            `json:"id"`
	Category string            `json:"category"`
	Paths    map[string]string `json:"paths"`
	Targets  map[string]string `json:"targets"`
	Include  []string          `json:"include"`
	Exclude  []string          `json:"exclude"`
	Strategy string            `json:"strategy"`
}
```

Add preview, issue, install-plan, conflict, custom-resource, and progress result
models using only safe metadata. Extend `Snapshot` with `Preview
ResourcePreview`, `PendingInstallPlan *InstallPlan`, and `Conflicts
[]ConflictSummary`. Add `Platform string` so the custom resource editor writes
the correct path mapping. Do not expose file content, credentials, or raw logs
through `Snapshot`.

- [ ] **Step 5: Implement service operations**

`ResourcePreview` performs discovery and safety classification without writing
the repository. `PreviewCustomResource` validates and previews only the supplied
candidate without saving it. `SaveSettings` validates category/custom settings
and preserves existing installation approvals. `ApproveInstallPlan` verifies
the current pending plan ID before saving approvals and scheduling a sync.
`ResolveConflict` validates choice `local`, `remote`, or `merged`; merged
content passes the resource scanner before resolution. `Snapshot` reports
state `error` when the latest cycle has `NeedsAttention` even if the scheduler
has returned to idle, and clears that state only after a later clean cycle.

- [ ] **Step 6: Update onboarding and legacy settings**

Onboarding still selects provider master switches. Category defaults remain
enabled and become editable after onboarding. The legacy settings page renders
category checkboxes and custom resource summaries so it cannot silently erase
desktop-created settings.

- [ ] **Step 7: Run tests**

```powershell
go test ./internal/desktop ./internal/settings ./internal/onboarding .
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/desktop internal/settings internal/onboarding main.go main_test.go
git commit -m "feat: expose portable resource controls"
```

## Task 10: Build the Desktop Resource Management UI

**Files:**
- Create: `frontend/src/resources/ResourceSettings.tsx`
- Create: `frontend/src/resources/RestorePreview.tsx`
- Create: `frontend/src/resources/InstallPlanPanel.tsx`
- Create: `frontend/src/resources/ConflictPanel.tsx`
- Create: `frontend/src/resources/ResultSummary.tsx`
- Create: `frontend/src/resources/CustomResourceEditor.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/style.css`
- Regenerate: `frontend/bindings/github.com/qinqingxu/acsync/internal/desktop/models.ts`
- Regenerate: `frontend/bindings/github.com/qinqingxu/acsync/internal/desktop/wailsservice.ts`

- [ ] **Step 1: Regenerate Wails bindings**

Run:

```powershell
wails3 generate bindings -clean=true -ts -i
```

Expected: generated models include resource categories, previews, install
plans, conflicts, and the four new Wails methods.

- [ ] **Step 2: Implement the category settings component**

`ResourceSettings` renders one expandable agent row and category toggles. It
shows source, target, counts, bytes, exclusions, unsupported reasons, and Common
Resources. Use bound model types directly.

```tsx
import type { Agent } from '../../bindings/github.com/qinqingxu/acsync/internal/desktop/models'

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / (1024 * 1024)).toFixed(1)} MiB`
}

function updateCategory(
  current: Record<string, Record<string, boolean>>,
  agent: string,
  category: string,
  enabled: boolean,
) {
  return {
    ...current,
    [agent]: {
      ...(current[agent] ?? {}),
      [category]: enabled,
    },
  }
}

export function ResourceSettings({
  agents,
  categories,
  onChange,
}: {
  agents: Agent[]
  categories: Record<string, Record<string, boolean>>
  onChange: (next: Record<string, Record<string, boolean>>) => void
}) {
  return (
    <div className="resource-settings">
      {agents.map((agent) => (
        <details key={agent.name}>
          <summary><strong>{agent.name}</strong></summary>
          {(agent.resources ?? []).map((resource) => (
            <label className="resource-row" key={resource.id}>
              <span>
                <strong>{resource.category}</strong>
                <small>{resource.source} → {resource.target}</small>
                <small>{resource.fileCount} files · {formatBytes(resource.bytes)}</small>
                {resource.reason && <small className="warning">{resource.reason}</small>}
              </span>
              <input
                type="checkbox"
                disabled={!resource.supported}
                checked={categories[agent.name]?.[resource.category] ?? resource.enabled}
                onChange={(event) => onChange(updateCategory(
                  categories, agent.name, resource.category, event.target.checked,
                ))}
              />
            </label>
          ))}
        </details>
      ))}
    </div>
  )
}
```

- [ ] **Step 3: Implement restore and install panels**

`RestorePreview` groups safe resources, path mappings, blocked items, and total
bytes. `InstallPlanPanel` displays provider, source, version, target, exact
executable/argv, and requires explicit approval. Call
`ApproveInstallPlan(plan.id)` and refresh snapshot only after success.

- [ ] **Step 4: Implement the custom resource editor**

The editor requires a stable ID, one of the five resource categories, a current
platform source, a current platform restore target, include globs, exclude
globs, and strategy `file-tree`, `text-tree`, `structured-merge`, or
`source-tree`. It does not offer `install-manifest`. Custom Sessions require
`file-tree`; custom Plugin source requires `source-tree`.

```tsx
import type {
  CustomResourceInput,
} from '../../bindings/github.com/qinqingxu/acsync/internal/desktop/models'

export interface CustomResourceDraft {
  id: string
  category: 'sessions' | 'config' | 'instructions' | 'skills' | 'plugins'
  source: string
  target: string
  include: string
  exclude: string
  strategy: 'file-tree' | 'text-tree' | 'structured-merge' | 'source-tree'
}

export function toCustomResource(
  draft: CustomResourceDraft,
  goos: string,
): CustomResourceInput {
  return {
    id: draft.id.trim(),
    category: draft.category,
    paths: { [goos]: draft.source.trim() },
    targets: { [goos]: draft.target.trim() },
    include: draft.include.split('\n').map((value) => value.trim()).filter(Boolean),
    exclude: draft.exclude.split('\n').map((value) => value.trim()).filter(Boolean),
    strategy: draft.strategy,
  }
}
```

Before adding, call `PreviewCustomResource` with the candidate and show its safe,
excluded, blocked, and oversized counts. Disable Save until ID, source, target,
and include are valid.

- [ ] **Step 5: Implement conflict resolution**

List safe metadata only. Offer Use local, Use remote, and Edit merged. Editing
starts empty unless the backend supplies an already safe merge candidate; do
not put base/local/remote raw content into global application state.

- [ ] **Step 6: Integrate split result counts and settings**

Extend `SettingsPanel` state with categories and custom resources. Pass the
complete settings input to `SaveSettings`. Replace the single completion string
with Restored, Reinstalled, Skipped, Blocked, and Conflicts counts. Any conflict
or failed/pending install leaves the hero state as `Needs attention`.

- [ ] **Step 7: Add accessible styling**

Add focus states, disabled explanations, responsive details rows, warning and
conflict colors, scroll containment for long plans, and `aria-live` on install
and conflict results. Preserve the current tray-sized window layout.

- [ ] **Step 8: Build the frontend**

Run:

```powershell
Set-Location frontend
npm run build
Set-Location ..
```

Expected: TypeScript and Vite production build succeed.

- [ ] **Step 9: Run desktop tests after generated bindings**

```powershell
go test ./internal/desktop ./internal/onboarding
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add frontend internal/desktop
git commit -m "feat: manage portable resources in desktop app"
```

## Task 11: Prove Migration, Two-Computer Merge, and Fresh Restore

**Files:**
- Create: `internal/syncengine/portable_e2e_test.go`
- Create: `internal/syncengine/fixtures/portable/README.md`
- Modify: `internal/syncengine/engine_test.go`
- Modify: `internal/secret/secret_test.go`
- Create: `docs/portable-resources.md`
- Modify: `docs/install.md`

- [ ] **Step 1: Write the v1 repository compatibility test**

Create a local bare remote with legacy sessions/config plus an
`agents/_portable/config` resource. Run a helper that mirrors the old client's
known-provider ownership behavior, then run the new engine. Assert neither
client deletes the portable pseudo-provider and the new client does not touch a
disabled provider.

- [ ] **Step 2: Write the two-computer merge and deletion test**

Machine A and B use separate homes, repos, state stores, and agent roots. Assert:

1. different session files form a union;
2. non-overlapping instruction edits merge;
3. same-key config edits create one unresolved conflict;
4. unrelated files continue syncing;
5. deleting a common Skill propagates to both aliases and enters trash;
6. unresolved conflict deletion is suspended.

- [ ] **Step 3: Write the fresh-profile restore test**

Seed Machine A with:

- safe settings plus a credential and machine ID;
- global instructions;
- a common Skill with `package.json` and `package-lock.json`;
- generated `node_modules` and `.vscode-test` content;
- Claude and Copilot Plugin declarations;
- sessions.

Sync to a local bare Git remote. Connect empty Machine B, inspect its restore and
installation preview, approve fake-runner operations, and restore. Assert:

- safe settings, instructions, Skill source, declarations, and sessions match;
- Machine A's credential and machine ID are absent from Machine B;
- Machine B's pre-existing credential remains if supplied;
- generated dependencies and runtimes are absent from Git;
- no database, cache, token, executable, or file at least 50 MiB exists in Git;
- the fake runner received exact argv and no shell command string;
- final result reports restored and reinstalled counts.

- [ ] **Step 4: Run end-to-end tests**

```powershell
go test ./internal/syncengine ./internal/installplan ./internal/secret -run 'Portable|FreshProfile|TwoComputer|Compatibility' -v
```

Expected: PASS.

- [ ] **Step 5: Document user-visible behavior**

Create `docs/portable-resources.md` with:

- what each category synchronizes;
- explicit non-synchronized content;
- first-computer and new-computer flows;
- Plugin/dependency confirmation;
- conflict resolution;
- custom directory restrictions;
- deletion and 30-day recovery;
- troubleshooting for blocked files, unavailable symlinks, and failed install
  commands.

Update `docs/install.md` so a new installation instructs the user to review the
restore preview and installation plan after repository connection.

- [ ] **Step 6: Run documentation-adjacent tests**

```powershell
go test ./internal/desktop ./internal/onboarding ./internal/settings
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/syncengine internal/installplan internal/secret docs
git commit -m "test: verify portable environment restore"
```

## Task 12: Full Validation, Package, Install, and Persist the Result

**Files:**
- Verify only; update implementation files only if a validation failure is
  directly caused by this feature.

- [ ] **Step 1: Format modified Go files**

Run:

```powershell
$files = git diff --name-only master...HEAD -- '*.go'
if ($files) { gofmt -w $files }
```

Expected: command exits 0.

- [ ] **Step 2: Run the complete backend suite**

Run:

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run vet**

Run:

```powershell
go vet ./...
```

Expected: PASS.

- [ ] **Step 4: Regenerate bindings and build production frontend**

Run sequentially to avoid the known `frontend/dist` race:

```powershell
wails3 generate bindings -clean=true -ts -i
Set-Location frontend
npm run build
Set-Location ..
go test ./...
```

Expected: binding generation, frontend build, and the post-build Go tests all
pass.

- [ ] **Step 5: Build the Windows installer**

Run:

```powershell
wails3 package GOOS=windows ARCH=amd64 INSTALL_SCOPE=user
```

Expected:
`bin\AgentConfigSync-amd64-installer.exe` exists and the package command exits
0.

- [ ] **Step 6: Run isolated installer smoke validation**

```powershell
.\scripts\smoke\windows.ps1 -Installer .\bin\AgentConfigSync-amd64-installer.exe
```

Expected: `Windows installer smoke test passed`.

- [ ] **Step 7: Install the validated desktop build and restart it**

Resolve and stop only the currently installed AgentConfigSync PID, install
silently for the current user, and start the exact installed executable:

```powershell
$installed = Join-Path $env:LOCALAPPDATA 'Programs\AgentConfigSync\AgentConfigSync.exe'
$running = @(Get-Process | Where-Object {
  try { $_.Path -eq $installed } catch { $false }
})
foreach ($process in $running) {
  Stop-Process -Id $process.Id
  Wait-Process -Id $process.Id -ErrorAction SilentlyContinue
}
$installer = Resolve-Path '.\bin\AgentConfigSync-amd64-installer.exe'
$install = Start-Process -FilePath $installer -ArgumentList '/S' -Wait -PassThru
if ($install.ExitCode -ne 0) { throw "Installer failed with $($install.ExitCode)" }
$app = Start-Process -FilePath $installed -ArgumentList '--hidden' -PassThru
Start-Sleep -Seconds 5
$app.Refresh()
if ($app.HasExited) { throw 'Installed AgentConfigSync exited during startup' }
```

Expected: one installed process remains running and the user's configured
repository and credentials are preserved.

- [ ] **Step 8: Verify Git cleanliness and commit any generated binding delta**

```powershell
git status --short
```

If binding generation changed tracked files after Task 10, commit only those
generated files:

```powershell
git add frontend/bindings
git commit -m "build: refresh desktop bindings"
```

Expected: `git status --short` is empty.
