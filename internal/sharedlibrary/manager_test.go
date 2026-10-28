package sharedlibrary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) Manager {
	t.Helper()
	return Manager{Home: filepath.Join(t.TempDir(), "profile"), UserHome: t.TempDir()}
}

func put(t *testing.T, name, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func execute(t *testing.T, m Manager, r Request) Plan {
	t.Helper()
	p, err := m.Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Apply(r, p.ID); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportPreviewApprovalAndStalePlan(t *testing.T) {
	m := fixture(t)
	source := filepath.Join(m.UserHome, ".claude", "skills", "review", "SKILL.md")
	put(t, source, "---\nname: review\ndescription: Review code\n---\nReview carefully.\n")
	r := Request{Action: "import", Kind: "skills", Name: "review", Harness: "claude"}
	p, err := m.Preview(r)
	if err != nil || len(p.Changes) != 1 || p.ID == "" {
		t.Fatalf("preview: %+v %v", p, err)
	}
	dest := filepath.Join(m.UserHome, ".agents", "library", "skills", "review", "SKILL.md")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("preview modified canonical source")
	}
	put(t, source, "---\nname: review\ndescription: Updated\n---\nChanged.\n")
	if err := m.Apply(r, p.ID); err == nil {
		t.Fatal("stale import was accepted")
	}
	execute(t, m, r)
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
}

func TestApplyDetectsDriftAndRestorePreservesOriginal(t *testing.T) {
	m := fixture(t)
	source := filepath.Join(m.UserHome, ".agents", "library", "skills", "review", "SKILL.md")
	target := filepath.Join(m.UserHome, ".claude", "skills", "review", "SKILL.md")
	put(t, source, "---\nname: review\ndescription: Shared\n---\nShared.\n")
	put(t, target, "---\nname: review\ndescription: Private\n---\nPrivate.\n")
	r := Request{Action: "apply", Kind: "skills", Name: "review", Harness: "claude"}
	execute(t, m, r)
	put(t, target, "User edit\n")
	if _, err := m.Preview(r); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("expected drift protection, got %v", err)
	}
	restore := r
	restore.Action = "restore"
	if _, err := m.Preview(restore); err == nil {
		t.Fatal("restore would overwrite drift")
	}
	detach := r
	detach.Action = "detach"
	execute(t, m, detach)
	data, _ := os.ReadFile(target)
	if string(data) != "User edit\n" {
		t.Fatal("detach changed private data")
	}
	execute(t, m, r)
	execute(t, m, restore)
	data, _ = os.ReadFile(target)
	if string(data) != "User edit\n" {
		t.Fatal("restore did not recover original")
	}
}

func TestRejectTraversalSecretsAndSymlinks(t *testing.T) {
	m := fixture(t)
	for _, name := range []string{"../escape", "a/b", "CON", "a:b", "a.", ""} {
		if _, err := m.Preview(Request{Action: "apply", Kind: "skills", Name: name, Harness: "claude"}); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
	put(t, filepath.Join(m.UserHome, ".agents", "library", "skills", "review", "SKILL.md"), "password = \"do-not-copy-this-value\"")
	if _, err := m.Preview(Request{Action: "apply", Kind: "skills", Name: "review", Harness: "claude"}); err == nil {
		t.Fatal("secret was accepted")
	}
}

func TestInstructionsMappingAndCommonSkills(t *testing.T) {
	m := fixture(t)
	put(t, filepath.Join(m.UserHome, ".agents", "library", "instructions", "AGENTS.md"), "Review tests before changing code.\n")
	execute(t, m, Request{Action: "apply", Kind: "instructions", Name: "global", Harness: "claude"})
	if _, err := os.Stat(filepath.Join(m.UserHome, ".claude", "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Preview(Request{Action: "apply", Kind: "instructions", Name: "global", Harness: "common"}); err == nil {
		t.Fatal("unsupported instruction adapter accepted")
	}
	put(t, filepath.Join(m.UserHome, ".agents", "library", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review\n---\nCheck tests.\n")
	execute(t, m, Request{Action: "apply", Kind: "skills", Name: "review", Harness: "common"})
	if _, err := os.Stat(filepath.Join(m.UserHome, ".agents", "skills", "review", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(m.UserHome, ".copilot", "skills", "review")); !os.IsNotExist(err) {
		t.Fatal("duplicated native discovery")
	}
}

func TestRefreshUpdatesOnlyApprovedTargetsAndProtectsDrift(t *testing.T) {
	m := fixture(t)
	source := filepath.Join(Root(m.UserHome), "skills", "review", "SKILL.md")
	target := filepath.Join(m.UserHome, ".claude", "skills", "review", "SKILL.md")
	put(t, source, "Version one\n")
	execute(t, m, Request{Action: "apply", Kind: "skills", Name: "review", Harness: "claude"})
	put(t, source, "Version two\n")
	if err := m.Refresh(true, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "Version two\n" {
		t.Fatal("approved output was not updated")
	}
	if _, err := os.Stat(filepath.Join(m.UserHome, ".agents", "skills", "review")); !os.IsNotExist(err) {
		t.Fatal("unapproved output created")
	}
	put(t, target, "Private edit\n")
	put(t, source, "Version three\n")
	if err := m.Refresh(true, true); err == nil {
		t.Fatal("drift was overwritten")
	}
	data, _ = os.ReadFile(target)
	if string(data) != "Private edit\n" {
		t.Fatal("drift lost")
	}
}

func TestRecoveryOfInterruptedApplyAndStaleTarget(t *testing.T) {
	m := fixture(t)
	source := filepath.Join(Root(m.UserHome), "skills", "review")
	target := filepath.Join(m.UserHome, ".claude", "skills", "review")
	put(t, filepath.Join(source, "SKILL.md"), "Shared\n")
	put(t, filepath.Join(target, "SKILL.md"), "Original\n")
	r := Request{Action: "apply", Kind: "skills", Name: "review", Harness: "claude"}
	p, err := m.Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(target, "extra.md"), "Unrelated addition\n")
	if err := m.Apply(r, p.ID); err == nil {
		t.Fatal("accepted changed target")
	}
	execute(t, m, r)
	state, err := m.load()
	if err != nil {
		t.Fatal(err)
	}
	rec := state.Records[key(r)]
	rec.Pending = true
	rec.Previous = rec.Original
	state.Records[key(r)] = rec
	if err := m.save(state); err != nil {
		t.Fatal(err)
	}
	r.Action = "restore"
	execute(t, m, r)
	data, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(data) != "Original\n" {
		t.Fatal("recovery lost original")
	}
	if _, err := os.Stat(filepath.Join(target, "extra.md")); err != nil {
		t.Fatal("recovery lost original extra file")
	}
}

func TestRejectGeneratedContentAndLinks(t *testing.T) {
	m := fixture(t)
	source := filepath.Join(Root(m.UserHome), "skills", "review")
	put(t, filepath.Join(source, "SKILL.md"), "Review\n")
	r := Request{Action: "apply", Kind: "skills", Name: "review", Harness: "claude"}
	put(t, filepath.Join(source, ".env"), "DATABASE_URL=private-value\n")
	if _, err := m.Preview(r); err == nil {
		t.Fatal("accepted credential file")
	}
	if err := os.Remove(filepath.Join(source, ".env")); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(source, "node_modules", "pkg", "index.js"), "Generated\n")
	if _, err := m.Preview(r); err == nil {
		t.Fatal("accepted generated files")
	}
}

func TestImportKeepsRecoveryBackup(t *testing.T) {
	m := fixture(t)
	source := filepath.Join(m.UserHome, ".claude", "skills", "review", "SKILL.md")
	canonical := filepath.Join(Root(m.UserHome), "skills", "review", "SKILL.md")
	put(t, source, "New source\n")
	put(t, canonical, "Previous canonical\n")
	p := execute(t, m, Request{Action: "import", Kind: "skills", Name: "review", Harness: "claude"})
	backup := filepath.Join(m.UserHome, ".synchub", "library-import-backups", p.ID+".json")
	if _, err := os.Stat(backup); err != nil {
		t.Fatal("missing canonical import backup:", err)
	}
}

func TestNativeOutputRejectsDuplicateCopilotDiscovery(t *testing.T) {
	m := fixture(t)
	put(t, filepath.Join(Root(m.UserHome), "skills", "review", "SKILL.md"), "Review\n")
	put(t, filepath.Join(m.UserHome, ".copilot", "skills", "review", "SKILL.md"), "Private review\n")
	if _, err := m.Preview(Request{Action: "apply", Kind: "skills", Name: "review", Harness: "common"}); err == nil {
		t.Fatal("created duplicate native skill discovery")
	}
}

func TestLinksAreRejectedWithoutChangingTarget(t *testing.T) {
	m := fixture(t)
	source := filepath.Join(Root(m.UserHome), "skills", "review")
	put(t, filepath.Join(source, "SKILL.md"), "Review\n")
	outside := filepath.Join(t.TempDir(), "external.md")
	put(t, outside, "Do not modify\n")
	if err := os.Symlink(outside, filepath.Join(source, "linked.md")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := m.Preview(Request{Action: "apply", Kind: "skills", Name: "review", Harness: "claude"}); err == nil {
		t.Fatal("followed source link")
	}
}
