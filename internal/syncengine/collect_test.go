package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/acsync/internal/secret"
	"github.com/qinqingxu/acsync/internal/state"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectMapsAndFilters(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "settings.json"), `{"theme":"dark"}`)
	writeFile(t, filepath.Join(root, "projects", "foo", "abc.jsonl"), `{"m":1}`)
	writeFile(t, filepath.Join(root, ".credentials.json"), `{"token":"x"}`)
	writeFile(t, filepath.Join(root, "secret.json"), `{"apiKey":"sk-1"}`)

	sc := secret.NewScanner(
		[]string{"**/.credentials.json"},
		[]string{"apiKey", "token"},
	)
	spec := AgentSpec{
		Name:     "claude",
		Root:     root,
		Include:  []string{"settings.json", "secret.json"},
		Sessions: []string{"projects/**/*.jsonl"},
		Scanner:  sc,
	}

	c, err := Collect([]AgentSpec{spec})
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}

	if _, ok := c.Snapshot["agents/claude/config/settings.json"]; !ok {
		t.Error("expected settings.json in config")
	}
	if _, ok := c.Snapshot["agents/claude/sessions/projects/foo/abc.jsonl"]; !ok {
		t.Error("expected session file mapped under sessions/")
	}
	if _, ok := c.Snapshot["agents/claude/config/.credentials.json"]; ok {
		t.Error(".credentials.json should be excluded by glob")
	}
	if _, ok := c.Snapshot["agents/claude/config/secret.json"]; ok {
		t.Error("secret.json should be blocked by content scan")
	}
	if src := c.Sources["agents/claude/config/settings.json"]; src == "" {
		t.Error("expected a source path for settings.json")
	}
}

func TestScanRemoteBlockedFindsRemoteOnlySecret(t *testing.T) {
	repoDir := t.TempDir()
	repoRel := "agents/demo/sessions/session.json"
	writeFile(t, filepath.Join(repoDir, filepath.FromSlash(repoRel)), `{"accessToken":"secret"}`)
	specs := map[string]AgentSpec{"demo": {
		Name:    "demo",
		Scanner: secret.NewScanner(nil, []string{"token"}),
	}}

	blocked, err := ScanRemoteBlocked(repoDir, specs, state.Snapshot{
		repoRel: {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || blocked[0] != repoRel {
		t.Fatalf("blocked = %#v, want %q", blocked, repoRel)
	}
}

func TestScanRemoteBlockedFindsDisallowedRemotePath(t *testing.T) {
	repoDir := t.TempDir()
	repoRel := "agents/vscode-copilot/sessions/workspace/chatEditingSessions/id/state.json"
	writeFile(t, filepath.Join(repoDir, filepath.FromSlash(repoRel)), `{"safe":true}`)
	specs := map[string]AgentSpec{"vscode-copilot": {
		Name:     "vscode-copilot",
		Sessions: []string{"*/chatSessions/*.jsonl"},
	}}

	blocked, err := ScanRemoteBlocked(repoDir, specs, state.Snapshot{repoRel: {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || blocked[0] != repoRel {
		t.Fatalf("blocked = %#v, want disallowed remote path", blocked)
	}
}

func TestFilterSnapshotForSpecsIgnoresDisabledProvider(t *testing.T) {
	snapshot := state.Snapshot{
		"agents/enabled/config/settings.json":  {},
		"agents/disabled/config/settings.json": {},
	}
	filtered := FilterSnapshotForSpecs(snapshot, map[string]AgentSpec{
		"enabled": {Name: "enabled"},
	})
	if len(filtered) != 1 {
		t.Fatalf("filtered = %#v", filtered)
	}
	if _, ok := filtered["agents/enabled/config/settings.json"]; !ok {
		t.Fatal("enabled provider path missing")
	}
}
