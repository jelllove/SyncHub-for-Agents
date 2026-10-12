package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/provider"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
	"gopkg.in/yaml.v3"
)

func TestPlanAppliesFirstSyncPolicyWithoutChangingResources(t *testing.T) {
	userHome := t.TempDir()
	home := filepath.Join(userHome, ".synchub")
	root := filepath.Join(userHome, "synthetic-agent")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "example.txt")
	if err := os.WriteFile(source, []byte("local only"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ProvidersDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	p := provider.Provider{Name: "demo", Config: provider.ConfigSpec{
		Paths: map[string]string{runtime.GOOS: root}, Include: []string{"example.txt"},
	}}
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ProvidersDir(home), "demo.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default([]string{"demo"})
	cfg.RepoURL = "git@github.com:example/synthetic.git"
	cfg.FirstSync = config.FirstSyncPolicy{Strategy: config.FirstSyncStrategyUseCloud}
	if err := config.Save(ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := inspectWithUserHome(home, runtime.GOOS, userHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != syncengine.DeleteLocal {
		t.Fatalf("use-cloud plan = %+v, want delete-local", plan.Actions)
	}
	after, err := os.ReadFile(ConfigPath(home))
	if err != nil || string(before) != string(after) {
		t.Fatalf("plan changed config: %v", err)
	}
	content, err := os.ReadFile(source)
	if err != nil || string(content) != "local only" {
		t.Fatalf("plan applied action: %q, %v", content, err)
	}
	if _, err := os.Stat(StatePath(home)); !os.IsNotExist(err) {
		t.Fatalf("plan wrote synchronization state: %v", err)
	}
}

type countingInitializer struct{ calls int }

func (initializer *countingInitializer) Initialize(_, dir string) error {
	initializer.calls++
	return os.MkdirAll(dir, 0o700)
}

func TestInitRequiresExplicitPolicyAndPreservesExistingProfile(t *testing.T) {
	home := t.TempDir()
	initializer := &countingInitializer{}
	if err := RunInitWithOptions(home, "git@github.com:example/synthetic.git", InitOptions{}, initializer); err == nil {
		t.Fatal("empty policy accepted")
	}
	if initializer.calls != 0 {
		t.Fatal("invalid input reached repository setup")
	}
	opts := InitOptions{FirstSync: config.FirstSyncStrategyMerge}
	if err := RunInitWithOptions(home, "git@github.com:example/synthetic.git", opts, initializer); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FirstSync.Completed || cfg.FirstSync.Strategy != config.FirstSyncStrategyMerge {
		t.Fatalf("first-sync policy = %+v", cfg.FirstSync)
	}
	before, err := os.ReadFile(ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if err := RunInitWithOptions(home, "git@github.com:example/different.git", opts, initializer); err == nil {
		t.Fatal("existing configuration overwritten")
	}
	after, err := os.ReadFile(ConfigPath(home))
	if err != nil || string(before) != string(after) || initializer.calls != 1 {
		t.Fatalf("existing profile changed, initializer calls=%d: %v", initializer.calls, err)
	}
}

func TestInitRejectsUnknownAgentsBeforeCloning(t *testing.T) {
	initializer := &countingInitializer{}
	err := RunInitWithOptions(t.TempDir(), "git@github.com:example/synthetic.git",
		InitOptions{FirstSync: config.FirstSyncStrategyMerge, Agents: []string{"not-an-agent"}}, initializer)
	if err == nil || initializer.calls != 0 {
		t.Fatalf("unknown agent reached repository setup: %v, calls=%d", err, initializer.calls)
	}
}
