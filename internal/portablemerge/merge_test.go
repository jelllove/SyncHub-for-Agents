package portablemerge

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

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

func TestStructuredMergeRecursesAndPreservesOneSidedDeletions(t *testing.T) {
	result, err := Structured(
		[]byte(`{"editor":{"theme":"dark","font":12},"remove":"old"}`),
		[]byte(`{"editor":{"theme":"light","font":12}}`),
		[]byte(`{"editor":{"theme":"dark","font":14},"remove":"old"}`),
	)
	if err != nil || result.Conflict {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	want := "{\n  \"editor\": {\n    \"font\": 14,\n    \"theme\": \"light\"\n  }\n}\n"
	if string(result.Data) != want {
		t.Fatalf("merged = %s", result.Data)
	}
}

func TestStructuredMergeReportsSameKeyAndTypeConflicts(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		local  string
		remote string
	}{
		{"same-key", `{"theme":"dark"}`, `{"theme":"light"}`, `{"theme":"blue"}`},
		{"map-to-scalars", `{"value":{"nested":true}}`, `{"value":"local"}`, `{"value":"remote"}`},
		{"delete-versus-modify", `{"value":"base"}`, `{}`, `{"value":"remote"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Structured([]byte(test.base), []byte(test.local), []byte(test.remote))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Conflict || len(result.Data) != 0 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestStructuredMergeRejectsMalformedOrNonObjectDocuments(t *testing.T) {
	for _, input := range [][]byte{
		[]byte(`{"broken":`),
		[]byte(`["array"]`),
		[]byte(`{"valid":true} trailing`),
	} {
		if _, err := Structured(input, []byte(`{}`), []byte(`{}`)); err == nil {
			t.Fatalf("Structured(%s) error = nil", input)
		}
	}
}

func TestBinaryMergeIsConservative(t *testing.T) {
	tests := []struct {
		name     string
		base     []byte
		local    []byte
		remote   []byte
		want     []byte
		conflict bool
	}{
		{"local-only", []byte{0}, []byte{1}, []byte{0}, []byte{1}, false},
		{"remote-only", []byte{0}, []byte{0}, []byte{2}, []byte{2}, false},
		{"same-change", []byte{0}, []byte{1}, []byte{1}, []byte{1}, false},
		{"both-change", []byte{0}, []byte{1}, []byte{2}, nil, true},
		{"delete-modify", []byte{0}, nil, []byte{2}, nil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Binary(test.base, test.local, test.remote)
			if result.Conflict != test.conflict || !bytes.Equal(result.Data, test.want) {
				t.Fatalf("Binary() = %#v", result)
			}
		})
	}
}

func TestGitTextMergerCombinesNonOverlappingEditsAndCleansTempFiles(t *testing.T) {
	parent := t.TempDir()
	merger := GitTextMerger{TempParent: parent}
	base := []byte("first\nlocal-base\nmiddle\nremote-base\nlast\n")
	local := []byte("first\nlocal-change\nmiddle\nremote-base\nlast\n")
	remote := []byte("first\nlocal-base\nmiddle\nremote-change\nlast\n")

	result, err := merger.Merge(base, local, remote)
	if err != nil || result.Conflict {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if !bytes.Contains(result.Data, []byte("local-change")) ||
		!bytes.Contains(result.Data, []byte("remote-change")) {
		t.Fatalf("merged text = %s", result.Data)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary merge files remain: %#v", entries)
	}
}

func TestGitTextMergerReportsOverlapWithoutReturningMarkers(t *testing.T) {
	merger := GitTextMerger{TempParent: t.TempDir()}
	result, err := merger.Merge(
		[]byte("before\nbase\nafter\n"),
		[]byte("before\nlocal\nafter\n"),
		[]byte("before\nremote\nafter\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Conflict || len(result.Data) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestGitTextMergerReportsInfrastructureFailure(t *testing.T) {
	merger := GitTextMerger{
		GitPath:    filepath.Join(t.TempDir(), "missing-git"),
		TempParent: t.TempDir(),
	}
	if _, err := merger.Merge([]byte("base"), []byte("local"), []byte("remote")); err == nil {
		t.Fatal("missing git executable returned nil error")
	}
}
