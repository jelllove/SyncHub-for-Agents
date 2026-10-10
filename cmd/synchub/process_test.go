package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/instance"
	"github.com/qinqingxu/synchub-for-agents/internal/provider"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
	"gopkg.in/yaml.v3"
)

func TestNativeCLIOutputAndExitCodes(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "synchub")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.Command("go", "build", "-o", executable, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("headless build: %v\n%s", err, output)
	}
	profile := filepath.Join(root, "profile")
	run := func(wantCode int, args ...string) envelope {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.Env = append(os.Environ(), "HOME="+root, "USERPROFILE="+root)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			var failure *exec.ExitError
			if !errors.As(err, &failure) {
				t.Fatalf("run %v: %v", args, err)
			}
			code = failure.ExitCode()
		}
		if code != wantCode {
			t.Fatalf("run %v exit=%d want=%d; stdout=%s stderr=%s", args, code, wantCode, stdout.String(), stderr.String())
		}
		var payload envelope
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("run %v invalid JSON %q: %v", args, stdout.String(), err)
		}
		if payload.SchemaVersion != 1 {
			t.Fatalf("schema = %d", payload.SchemaVersion)
		}
		return payload
	}
	payload := run(0, "--json", "--home", profile, "version")
	if !payload.OK {
		t.Fatal("version failed")
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("version touched profile: %v", err)
	}
	payload = run(2, "--json", "not-a-command")
	if payload.OK || payload.Error.Code != "invalid_input" {
		t.Fatalf("invalid input = %+v", payload)
	}
	for _, args := range [][]string{
		{"status", "--bogus", "--json"}, {"status", "--json", "--bogus"},
	} {
		payload = run(2, args...)
		if payload.OK || payload.Error.Code != "invalid_input" {
			t.Fatalf("usage error depends on JSON flag order: %+v", payload)
		}
	}
	payload = run(4, "--json", "--home", profile, "doctor")
	if payload.OK || payload.Data == nil || payload.Error.Code != "needs_attention" {
		t.Fatalf("doctor = %+v", payload)
	}
	payload = run(1, "--json", "--home", profile, "status")
	if payload.OK || payload.Error.Code != "runtime_error" {
		t.Fatalf("missing config = %+v", payload)
	}
	lock, err := instance.Acquire(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	for _, command := range []string{"status", "plan", "doctor", "sync", "daemon"} {
		args := []string{"--json", "--home", profile, command}
		want := 3
		if command == "daemon" {
			want = 2
		}
		payload = run(want, args...)
		if payload.OK {
			t.Fatalf("%s bypassed ownership", command)
		}
	}
}

func TestSyncSummaryNeverHidesAttention(t *testing.T) {
	for _, result := range []syncengine.Result{
		{Blocked: []string{"synthetic"}}, {Conflicts: 1}, {PendingInstalls: 1}, {NeedsAttention: true},
	} {
		if !summarizeSync(result).NeedsAttention {
			t.Fatalf("attention hidden: %+v", result)
		}
	}
	if summarizeSync(syncengine.Result{}).NeedsAttention {
		t.Fatal("clean result requires attention")
	}
}

func TestPlanJSONUsesSharedPolicyAndDoesNotExposeFileContents(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	home := filepath.Join(userHome, ".synchub")
	agent := filepath.Join(userHome, "synthetic-agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("illustrative private body")
	source := filepath.Join(agent, "example.txt")
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cli.ProvidersDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	p := provider.Provider{Name: "demo", Config: provider.ConfigSpec{
		Paths: map[string]string{runtime.GOOS: agent}, Include: []string{"example.txt"},
	}}
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cli.ProvidersDir(home), "demo.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default([]string{"demo"})
	cfg.RepoURL = "git@github.com:example/synthetic.git"
	cfg.FirstSync = config.FirstSyncPolicy{Strategy: config.FirstSyncStrategyUseCloud}
	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
	root := newRootCommand()
	root.SetArgs([]string{"--json", "--home", home, "plan"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var response struct {
		SchemaVersion int  `json:"schemaVersion"`
		OK            bool `json:"ok"`
		Data          struct {
			LocalOnly bool         `json:"localOnly"`
			Actions   []planAction `json:"actions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || !response.Data.LocalOnly || len(response.Data.Actions) != 1 ||
		response.Data.Actions[0].Type != "delete-local" {
		t.Fatalf("incorrect JSON action shape/policy: %s", out.String())
	}
	if bytes.Contains(out.Bytes(), content) {
		t.Fatal("file contents leaked into JSON")
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(after, content) {
		t.Fatalf("plan changed source: %v", err)
	}
}

func TestDaemonCancellationReleasesOwnershipWithoutSync(t *testing.T) {
	home := t.TempDir()
	cfg := config.Default(nil)
	cfg.RepoURL = "git@github.com:example/synthetic.git"
	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := newRootCommand()
	root.SetArgs([]string{"--home", home, "daemon"})
	root.SetOut(&bytes.Buffer{})
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(home)
	if err != nil {
		t.Fatalf("daemon leaked ownership: %v", err)
	}
	defer lock.Close()
	if _, err := os.Stat(cli.StatePath(home)); !os.IsNotExist(err) {
		t.Fatalf("cancelled daemon synchronized: %v", err)
	}
}
