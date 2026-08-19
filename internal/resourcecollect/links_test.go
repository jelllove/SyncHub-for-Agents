package resourcecollect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

func TestResolveRootUsesCanonicalLinkTargetWithoutExposingItAsOriginal(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "skills")
	createRootLink(t, target, link)

	got, err := ResolveRoot(link)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginalRoot != link {
		t.Fatalf("OriginalRoot = %q, want %q", got.OriginalRoot, link)
	}
	if got.CanonicalRoot != filepath.Clean(canonical) {
		t.Fatalf("CanonicalRoot = %q, want %q", got.CanonicalRoot, canonical)
	}
	if got.Identity == "" || got.Identity == got.CanonicalRoot {
		t.Fatalf("Identity must be opaque and non-empty: %q", got.Identity)
	}
	if _, err := os.Stat(got.CanonicalRoot); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorRequiresApprovalForNestedLinksEscapingRoot(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	writeCollectorFile(t, filepath.Join(external, "outside.md"), []byte("external"))
	link := filepath.Join(root, "linked")
	createRootLink(t, external, link)
	spec := sourceSpec(root)

	unapproved, err := New(Options{StageParent: t.TempDir()}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	defer unapproved.Close()
	if len(unapproved.Artifacts) != 0 {
		t.Fatalf("unapproved artifacts = %#v", unapproved.Artifacts)
	}
	if !hasIssue(unapproved.Skipped, "linked", "unapproved-link") {
		t.Fatalf("unapproved link issue = %#v", unapproved.Skipped)
	}

	canonical, err := filepath.EvalSymlinks(external)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := New(Options{
		StageParent:   t.TempDir(),
		ApprovedLinks: approvedLinks{filepath.Clean(canonical): true},
	}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	defer approved.Close()
	repoRel := "agents/_portable/config/common/skills/common-skills/linked/outside.md"
	if _, ok := approved.Artifacts[repoRel]; !ok {
		t.Fatalf("approved linked artifact missing: %#v", approved.Artifacts)
	}
}

func TestCollectorPrunesGeneratedDirectoryLinksBeforeResolvingThem(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	writeCollectorFile(t, filepath.Join(external, "package", "index.js"), []byte("dependency"))
	createRootLink(t, external, filepath.Join(root, "node_modules"))

	result, err := New(Options{StageParent: t.TempDir()}).Collect([]resource.Spec{sourceSpec(root)})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	if len(result.Artifacts) != 0 {
		t.Fatalf("generated linked artifacts = %#v", result.Artifacts)
	}
	if !hasIssue(result.Skipped, "node_modules", "generated-content") {
		t.Fatalf("generated link was not pruned: %#v", result.Skipped)
	}
	if hasIssue(result.Skipped, "node_modules", "unapproved-link") {
		t.Fatalf("generated link should be filtered before approval: %#v", result.Skipped)
	}
}

type approvedLinks map[string]bool

func (a approvedLinks) IsApproved(target string) bool {
	return a[filepath.Clean(target)]
}
