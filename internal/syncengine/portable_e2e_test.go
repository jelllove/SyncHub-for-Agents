package syncengine

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/installplan"
	"github.com/qinqingxu/acsync/internal/portableconfig"
	"github.com/qinqingxu/acsync/internal/resource"
)

func TestPortableCompatibilityPreservesLegacyAndUnknownProviders(t *testing.T) {
	bare := newBareRemote(t)
	seed := filepath.Join(t.TempDir(), "seed")
	cloneWorkspace(t, bare, seed)
	files := map[string]string{
		"agents/claude/sessions/legacy.jsonl":                                     `{"message":"legacy"}` + "\n",
		"agents/gemini/config/settings.json":                                      `{"theme":"gemini"}`,
		"agents/_portable/config/providers/copilot/instructions/global/README.md": "portable\n",
	}
	for rel, content := range files {
		writeFile(t, filepath.Join(seed, filepath.FromSlash(rel)), content)
	}
	git(t, seed, "add", ".")
	git(t, seed, "commit", "-m", "seed legacy and portable resources")
	git(t, seed, "push", "origin", "main")

	runLegacyOwnershipPass(t, bare)

	repo := filepath.Join(t.TempDir(), "new-client")
	root := t.TempDir()
	engine := &Engine{
		Git:       cloneWorkspace(t, bare, repo),
		RepoDir:   repo,
		Home:      t.TempDir(),
		UserHome:  root,
		GOOS:      "windows",
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Resources: map[string]resource.Spec{
			"copilot/global": {
				Key: "copilot/global", Provider: "copilot", ID: "global",
				Category: resource.CategoryInstructions, Strategy: resource.StrategyTextTree,
				Layout: resource.LayoutPortable, Root: root, Targets: []string{root},
				Include: []string{"*.md"},
			},
		},
	}
	if _, err := engine.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(root, "README.md"), "portable\n")
	for rel, content := range files {
		assertFileContent(t, filepath.Join(repo, filepath.FromSlash(rel)), content)
	}
}

func runLegacyOwnershipPass(t *testing.T, bare string) {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "legacy-client")
	cloneWorkspace(t, bare, repo)
	writeFile(t, filepath.Join(repo, "agents", "claude", "config", "legacy.json"), "{}")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "legacy client update")
	git(t, repo, "push", "origin", "main")
	if _, err := os.Stat(filepath.Join(
		repo,
		"agents",
		"_portable",
		"config",
		"providers",
		"copilot",
		"instructions",
		"global",
		"README.md",
	)); err != nil {
		t.Fatalf("legacy ownership pass removed portable namespace: %v", err)
	}
}

type portableRoots struct {
	sessions     string
	instructions string
	settings     string
	skills       string
	skillAlias   string
}

func TestPortableTwoComputerMergeDeletionAndConflictSuspension(t *testing.T) {
	bare := newBareRemote(t)
	newMachine := func(name string) (*Engine, portableRoots) {
		home := t.TempDir()
		roots := portableRoots{
			sessions:     filepath.Join(home, "sessions"),
			instructions: filepath.Join(home, "instructions"),
			settings:     filepath.Join(home, "config"),
			skills:       filepath.Join(home, "claude-skills"),
			skillAlias:   filepath.Join(home, "copilot-skills"),
		}
		for _, root := range []string{
			roots.sessions, roots.instructions, roots.settings, roots.skills, roots.skillAlias,
		} {
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		repo := filepath.Join(t.TempDir(), name+"-repo")
		codecs := portableconfig.NewRegistry()
		codecs.Register("portable-settings", portableconfig.Policy{
			Portable: []string{"theme", "font"},
		})
		return &Engine{
			Git:       cloneWorkspace(t, bare, repo),
			RepoDir:   repo,
			Home:      home,
			UserHome:  home,
			GOOS:      "windows",
			StatePath: filepath.Join(home, "state.json"),
			Codecs:    codecs,
			Resources: portableMergeSpecs(roots),
			Now:       func() time.Time { return time.Unix(2000, 0) },
		}, roots
	}
	engineA, rootsA := newMachine("a")
	engineB, rootsB := newMachine("b")

	baseInstructions := "first\nlocal-base\nmiddle\nremote-base\nlast\n"
	writeFile(t, filepath.Join(rootsA.sessions, "session-a.jsonl"), `{"machine":"a"}`+"\n")
	writeFile(t, filepath.Join(rootsA.instructions, "AGENTS.md"), baseInstructions)
	writeFile(t, filepath.Join(rootsA.settings, "settings.json"), `{"theme":"base","font":12}`)
	writeFile(t, filepath.Join(rootsA.skills, "SKILL.md"), "shared skill\n")
	writeFile(t, filepath.Join(rootsA.skillAlias, "SKILL.md"), "shared skill\n")
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	if _, err := engineB.SyncOnce(); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(rootsB.sessions, "session-b.jsonl"), `{"machine":"b"}`+"\n")
	writeFile(t, filepath.Join(rootsA.instructions, "AGENTS.md"), "first\nmachine-a\nmiddle\nremote-base\nlast\n")
	writeFile(t, filepath.Join(rootsA.settings, "settings.json"), `{"theme":"machine-a","font":12}`)
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rootsB.instructions, "AGENTS.md"), "first\nlocal-base\nmiddle\nmachine-b\nlast\n")
	writeFile(t, filepath.Join(rootsB.settings, "settings.json"), `{"theme":"machine-b","font":12}`)
	resultB, err := engineB.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if resultB.Conflicts != 1 || !resultB.NeedsAttention {
		t.Fatalf("conflicted result = %#v", resultB)
	}
	wantInstructions := "first\nmachine-a\nmiddle\nmachine-b\nlast\n"
	assertFileContent(t, filepath.Join(rootsB.instructions, "AGENTS.md"), wantInstructions)
	assertFileContent(t, filepath.Join(rootsB.sessions, "session-a.jsonl"), `{"machine":"a"}`+"\n")

	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(rootsA.instructions, "AGENTS.md"), wantInstructions)
	assertFileContent(t, filepath.Join(rootsA.sessions, "session-b.jsonl"), `{"machine":"b"}`+"\n")

	if err := os.Remove(filepath.Join(rootsA.skills, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	trashPath := filepath.Join(
		engineA.RepoDir,
		".trash",
		"files",
		"agents",
		"_portable",
		"config",
		"common",
		"skills",
		"common-skills",
		"SKILL.md",
	)
	if _, err := os.Stat(trashPath); err != nil {
		t.Fatalf("deleted common Skill was not recoverable: %v", err)
	}
	if _, err := engineB.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{
		filepath.Join(rootsA.skillAlias, "SKILL.md"),
		filepath.Join(rootsB.skills, "SKILL.md"),
		filepath.Join(rootsB.skillAlias, "SKILL.md"),
	} {
		if _, err := os.Stat(filename); !os.IsNotExist(err) {
			t.Fatalf("common Skill alias was not deleted: %s, err=%v", filename, err)
		}
	}

	settingsRepoRel, err := engineB.Resources["demo/settings"].RepoPath("settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(rootsB.settings, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := engineB.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(engineB.RepoDir, filepath.FromSlash(settingsRepoRel))); err != nil {
		t.Fatalf("unresolved conflict did not suspend deletion: %v", err)
	}
	records, err := os.ReadDir(filepath.Join(engineB.Home, "conflicts"))
	if err != nil || len(records) != 1 {
		t.Fatalf("unresolved conflict was lost: %#v, %v", records, err)
	}
}

func portableMergeSpecs(roots portableRoots) map[string]resource.Spec {
	return map[string]resource.Spec{
		"demo/sessions": {
			Key: "demo/sessions", Provider: "demo", ID: "sessions",
			Category: resource.CategorySessions, Strategy: resource.StrategyFileTree,
			Layout: resource.LayoutPortable, Root: roots.sessions, Targets: []string{roots.sessions},
			Include: []string{"*.jsonl"},
		},
		"demo/instructions": {
			Key: "demo/instructions", Provider: "demo", ID: "instructions",
			Category: resource.CategoryInstructions, Strategy: resource.StrategyTextTree,
			Layout: resource.LayoutPortable, Root: roots.instructions, Targets: []string{roots.instructions},
			Include: []string{"*.md"},
		},
		"demo/settings": {
			Key: "demo/settings", Provider: "demo", ID: "settings",
			Category: resource.CategoryConfig, Strategy: resource.StrategyStructuredMerge,
			Layout: resource.LayoutPortable, Root: roots.settings, Targets: []string{roots.settings},
			Include: []string{"settings.json"}, Transformer: "portable-settings",
		},
		"common/common-skills": {
			Key: "common/common-skills", Provider: "common", ID: "common-skills",
			SharedAs: "common-skills", Category: resource.CategorySkills,
			Strategy: resource.StrategySourceTree, Layout: resource.LayoutPortable,
			Root: roots.skills, Targets: []string{roots.skills, roots.skillAlias},
			Include: []string{"**"},
		},
	}
}

type recordedCommand struct {
	executable string
	args       []string
	workingDir string
}

type portableRunner struct {
	source    bool
	installed map[string]bool
	commands  []recordedCommand
}

func (r *portableRunner) Run(
	_ context.Context,
	executable string,
	args []string,
	workingDir string,
) (installplan.RunResult, error) {
	if r.installed == nil {
		r.installed = map[string]bool{}
	}
	if executable == "claude" && equalTestArgs(args, "plugin", "list", "--json") {
		if r.source || r.installed["claude"] {
			return installplan.RunResult{Stdout: []byte(
				`[{"pluginId":"review@official","version":"1.0.0","enabled":true}]`,
			)}, nil
		}
		return installplan.RunResult{Stdout: []byte("[]")}, nil
	}
	if executable == "claude" && equalTestArgs(args, "plugin", "marketplace", "list", "--json") {
		return installplan.RunResult{Stdout: []byte("[]")}, nil
	}
	if executable == "copilot" && equalTestArgs(args, "plugin", "list") {
		if r.source || r.installed["copilot"] {
			return installplan.RunResult{Stdout: []byte("wiqd@wiqd (0.6.0)\n")}, nil
		}
		return installplan.RunResult{Stdout: []byte("Installed plugins:\n")}, nil
	}
	r.commands = append(r.commands, recordedCommand{
		executable: executable,
		args:       append([]string(nil), args...),
		workingDir: workingDir,
	})
	if executable == "claude" {
		r.installed["claude"] = true
	}
	if executable == "copilot" {
		r.installed["copilot"] = true
	}
	return installplan.RunResult{}, nil
}

func TestPortableFreshProfileRestoresSafeEnvironment(t *testing.T) {
	bare := newBareRemote(t)
	fixture, err := os.ReadFile(filepath.Join("fixtures", "portable", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	engineA, rootsA, runnerA := freshProfileMachine(t, bare, "a", true)
	writeFile(t, filepath.Join(rootsA.settings, "settings.json"), `{
		"theme":"dark",
		"token":"machine-a-secret",
		"machineId":"machine-a"
	}`)
	writeFile(t, filepath.Join(rootsA.instructions, "AGENTS.md"), string(fixture))
	writeFile(t, filepath.Join(rootsA.sessions, "initial.jsonl"), `{"message":"initial"}`+"\n")
	writeFile(t, filepath.Join(rootsA.skills, "review", "SKILL.md"), "# Review\n")
	writeFile(t, filepath.Join(rootsA.skills, "review", "package.json"), `{"name":"review-skill"}`)
	writeFile(t, filepath.Join(rootsA.skills, "review", "package-lock.json"), `{"lockfileVersion":3}`)
	writeFile(t, filepath.Join(rootsA.skills, "review", "node_modules", "dep.js"), "generated")
	writeFile(t, filepath.Join(rootsA.skills, "review", ".vscode-test", "runtime.exe"), "generated")
	writeFile(t, filepath.Join(rootsA.skills, "review", "cache.db"), "generated")
	writeFile(t, filepath.Join(rootsA.skills, "review", "native.exe"), "binary")
	large := filepath.Join(rootsA.skills, "review", "large.bin")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(resource.DefaultMaxFileSize); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	first, err := engineA.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if !first.Pushed {
		t.Fatalf("source profile did not publish resources: %#v", first)
	}
	if len(runnerA.commands) != 0 {
		t.Fatalf("source profile unexpectedly installed dependencies: %#v", runnerA.commands)
	}

	engineB, rootsB, runnerB := freshProfileMachine(t, bare, "b", false)
	writeFile(t, filepath.Join(rootsB.settings, "settings.json"), `{"token":"machine-b-secret"}`)
	pendingResult, err := engineB.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if pendingResult.PendingInstalls != 3 || !pendingResult.NeedsAttention {
		t.Fatalf("fresh restore did not require three approvals: %#v", pendingResult)
	}
	pending, err := engineB.InstallManager.Store.Pending()
	if err != nil || pending == nil || len(pending.Operations) != 3 {
		t.Fatalf("pending install plan = %#v, %v", pending, err)
	}
	for _, operation := range pending.Operations {
		if err := engineB.InstallManager.Store.Approve(operation); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, filepath.Join(rootsA.sessions, "after-approval.jsonl"), `{"message":"late"}`+"\n")
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	final, err := engineB.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if final.Restored == 0 || final.Reinstalled != 3 || final.PendingInstalls != 0 {
		t.Fatalf("final fresh restore result = %#v", final)
	}

	assertFreshProfile(t, rootsB, fixture)
	assertTrustedCommands(t, runnerB.commands, rootsB.skills)
	assertPortableRepository(t, engineB.RepoDir)
}

func freshProfileMachine(
	t *testing.T,
	bare, name string,
	source bool,
) (*Engine, portableRoots, *portableRunner) {
	t.Helper()
	home := t.TempDir()
	roots := portableRoots{
		sessions:     filepath.Join(home, "sessions"),
		instructions: filepath.Join(home, "instructions"),
		settings:     filepath.Join(home, "config"),
		skills:       filepath.Join(home, "skills"),
		skillAlias:   filepath.Join(home, "skill-alias"),
	}
	for _, root := range []string{
		roots.sessions, roots.instructions, roots.settings, roots.skills, roots.skillAlias,
		filepath.Join(home, "claude-plugins"), filepath.Join(home, "copilot-plugins"),
	} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runner := &portableRunner{source: source}
	inventory := installplan.NewBuiltinInventory(runner)
	manager := installplan.NewManager(home, inventory, runner)
	codecs := portableconfig.NewRegistry()
	codecs.Register("fresh-settings", portableconfig.Policy{
		Portable:     []string{"theme", "token", "machineId"},
		Sensitive:    []string{"token"},
		MachineLocal: []string{"machineId"},
	})
	specs := portableMergeSpecs(roots)
	settings := specs["demo/settings"]
	settings.Transformer = "fresh-settings"
	specs[settings.Key] = settings
	specs["claude/plugins"] = resource.Spec{
		Key: "claude/plugins", Provider: "claude", ID: "plugins",
		Category: resource.CategoryPlugins, Strategy: resource.StrategyInstallManifest,
		Layout: resource.LayoutPortable, Root: filepath.Join(home, "claude-plugins"),
		Targets: []string{filepath.Join(home, "claude-plugins")}, Installer: "claude-plugin",
	}
	specs["copilot/plugins"] = resource.Spec{
		Key: "copilot/plugins", Provider: "copilot", ID: "plugins",
		Category: resource.CategoryPlugins, Strategy: resource.StrategyInstallManifest,
		Layout: resource.LayoutPortable, Root: filepath.Join(home, "copilot-plugins"),
		Targets: []string{filepath.Join(home, "copilot-plugins")}, Installer: "copilot-plugin",
	}
	repo := filepath.Join(t.TempDir(), name+"-repo")
	return &Engine{
		Git:            cloneWorkspace(t, bare, repo),
		RepoDir:        repo,
		Home:           home,
		UserHome:       home,
		GOOS:           "windows",
		StatePath:      filepath.Join(home, "state.json"),
		Resources:      specs,
		Codecs:         codecs,
		Inventory:      inventory,
		InstallManager: manager,
		Now:            func() time.Time { return time.Unix(3000, 0) },
	}, roots, runner
}

func assertFreshProfile(t *testing.T, roots portableRoots, fixture []byte) {
	t.Helper()
	assertFileContent(t, filepath.Join(roots.instructions, "AGENTS.md"), string(fixture))
	assertFileContent(t, filepath.Join(roots.sessions, "initial.jsonl"), `{"message":"initial"}`+"\n")
	assertFileContent(t, filepath.Join(roots.sessions, "after-approval.jsonl"), `{"message":"late"}`+"\n")
	assertFileContent(t, filepath.Join(roots.skills, "review", "SKILL.md"), "# Review\n")
	for _, filename := range []string{
		filepath.Join(roots.skills, "review", "node_modules", "dep.js"),
		filepath.Join(roots.skills, "review", ".vscode-test", "runtime.exe"),
		filepath.Join(roots.skills, "review", "cache.db"),
		filepath.Join(roots.skills, "review", "native.exe"),
		filepath.Join(roots.skills, "review", "large.bin"),
	} {
		if _, err := os.Stat(filename); !os.IsNotExist(err) {
			t.Fatalf("generated or unsafe file was restored: %s, err=%v", filename, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(roots.settings, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["theme"] != "dark" ||
		settings["token"] != "machine-b-secret" ||
		settings["machineId"] != nil {
		t.Fatalf("restored settings = %#v", settings)
	}
}

func assertTrustedCommands(t *testing.T, commands []recordedCommand, skillsRoot string) {
	t.Helper()
	if len(commands) != 3 {
		t.Fatalf("executed commands = %#v", commands)
	}
	wants := map[string]struct{}{
		`claude` + "\x00" + strings.Join([]string{"plugin", "install", "review@official", "--scope", "user"}, "\x00"): {},
		`copilot` + "\x00" + strings.Join([]string{"plugin", "install", "wiqd@wiqd"}, "\x00"):                         {},
		`npm` + "\x00" + "ci": {},
	}
	for _, command := range commands {
		key := command.executable + "\x00" + strings.Join(command.args, "\x00")
		if _, ok := wants[key]; !ok {
			t.Fatalf("unexpected executable/argv pair: %#v", command)
		}
		delete(wants, key)
		if strings.ContainsAny(command.executable, ";&|><`") {
			t.Fatalf("command used shell syntax: %#v", command)
		}
		if command.executable == "npm" &&
			command.workingDir != filepath.Join(skillsRoot, "review") {
			t.Fatalf("npm working directory = %q", command.workingDir)
		}
	}
	if len(wants) != 0 {
		t.Fatalf("missing trusted commands: %#v", wants)
	}
}

func assertPortableRepository(t *testing.T, repo string) {
	t.Helper()
	err := filepath.WalkDir(filepath.Join(repo, "agents"), func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repo, filename)
		if err != nil {
			return err
		}
		normalized := strings.ToLower(filepath.ToSlash(rel))
		if info.Size() >= resource.DefaultMaxFileSize ||
			strings.Contains(normalized, "node_modules/") ||
			strings.Contains(normalized, ".vscode-test/") ||
			strings.Contains(normalized, "token") ||
			strings.HasSuffix(normalized, ".db") ||
			strings.HasSuffix(normalized, ".exe") {
			t.Errorf("unsafe repository file: %s (%d bytes)", normalized, info.Size())
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "machine-a-secret") ||
			strings.Contains(string(data), `"machineId"`) {
			t.Errorf("machine-local data leaked into %s", normalized)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"agents/_portable/config/providers/claude/plugins/plugins/manifest.json",
		"agents/_portable/config/providers/copilot/plugins/plugins/manifest.json",
		"agents/_portable/config/common/skills/common-skills/review/package.json",
		"agents/_portable/config/common/skills/common-skills/review/package-lock.json",
	} {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); err != nil {
			t.Errorf("portable repository missing %s: %v", rel, err)
		}
	}
}

func equalTestArgs(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
