package portableconfig

import (
	"bytes"
	"strings"
	"testing"
)

func TestCodecProjectsSafeFieldsAndRestoresWithoutReplacingCredentials(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{
		Portable:     []string{"**"},
		Sensitive:    []string{"accessToken", "auth.**"},
		MachineLocal: []string{"installationId", "recentWorkspaces"},
		PathFields:   []string{"paths.skills"},
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
	_, projectedDocument, err := Parse("settings.json", projected)
	if err != nil {
		t.Fatal(err)
	}
	if got := lookup(projectedDocument, "paths", "skills"); got != `${HOME}/.agents/skills` {
		t.Fatalf("projected path = %#v", got)
	}

	remote := []byte(`{"theme":"light","paths":{"skills":"${HOME}/.agents/skills"}}`)
	restored, err := registry.Restore(
		"demo", "settings.json", "windows", `C:\Users\alice`,
		local, projected, remote,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, restoredDocument, err := Parse("settings.json", restored)
	if err != nil {
		t.Fatal(err)
	}
	if got := lookup(restoredDocument, "accessToken"); got != "local-secret" {
		t.Fatalf("credential = %#v", got)
	}
	if got := lookup(restoredDocument, "installationId"); got != "machine-a" {
		t.Fatalf("machine field = %#v", got)
	}
	if got := lookup(restoredDocument, "theme"); got != "light" {
		t.Fatalf("theme = %#v", got)
	}
	if got := lookup(restoredDocument, "paths", "skills"); got != `C:\Users\alice\.agents\skills` {
		t.Fatalf("restored path = %#v", got)
	}
}

func TestCodecProjectsAndRestoresYAMLAndTOML(t *testing.T) {
	tests := []struct {
		name  string
		rel   string
		local []byte
	}{
		{
			name:  "yaml",
			rel:   "settings.yaml",
			local: []byte("theme: dark\naccessToken: local-secret\n"),
		},
		{
			name:  "toml",
			rel:   "settings.toml",
			local: []byte("theme = 'dark'\naccessToken = 'local-secret'\n"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewRegistry()
			registry.Register("demo", Policy{
				Portable:  []string{"**"},
				Sensitive: []string{"accessToken"},
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
			_, document, err := Parse(test.rel, restored)
			if err != nil {
				t.Fatal(err)
			}
			if got := lookup(document, "theme"); got != "light" {
				t.Fatalf("theme = %#v, restored = %s", got, restored)
			}
			if got := lookup(document, "accessToken"); got != "local-secret" {
				t.Fatalf("credential = %#v, restored = %s", got, restored)
			}
		})
	}
}

func TestCodecRestoreAppliesPortableDeletion(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{
		Portable:  []string{"**"},
		Sensitive: []string{"accessToken"},
	})
	local := []byte(`{"theme":"dark","fontSize":14,"accessToken":"local"}`)
	base, err := registry.Project("demo", "settings.json", "linux", "/home/alice", local)
	if err != nil {
		t.Fatal(err)
	}

	restored, err := registry.Restore(
		"demo", "settings.json", "linux", "/home/alice",
		local, base, []byte(`{"theme":"light"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, document, err := Parse("settings.json", restored)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := document["fontSize"]; exists {
		t.Fatalf("deleted portable key was retained: %s", restored)
	}
	if document["accessToken"] != "local" || document["theme"] != "light" {
		t.Fatalf("restore = %#v", document)
	}
}

func TestCodecRestoreEmptyObjectDoesNotEraseSensitiveSibling(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{
		Portable:  []string{"**"},
		Sensitive: []string{"auth.token"},
	})
	local := []byte(`{"auth":{"token":"local-secret","preference":"on"}}`)
	base, err := registry.Project("demo", "settings.json", "linux", "/home/alice", local)
	if err != nil {
		t.Fatal(err)
	}

	restored, err := registry.Restore(
		"demo", "settings.json", "linux", "/home/alice",
		local, base, []byte(`{"auth":{}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, document, err := Parse("settings.json", restored)
	if err != nil {
		t.Fatal(err)
	}
	auth, ok := document["auth"].(map[string]any)
	if !ok || auth["token"] != "local-secret" {
		t.Fatalf("sensitive sibling was erased: %#v", document)
	}
	if _, exists := auth["preference"]; exists {
		t.Fatalf("deleted portable sibling was retained: %#v", document)
	}
}

func TestCodecRestoreCreatesMissingLocalFileWithoutCredentials(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{
		Portable:  []string{"**"},
		Sensitive: []string{"accessToken"},
	})

	restored, err := registry.Restore(
		"demo", "settings.json", "linux", "/home/alice",
		nil, nil, []byte(`{"theme":"light"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, document, err := Parse("settings.json", restored)
	if err != nil {
		t.Fatal(err)
	}
	if len(document) != 1 || document["theme"] != "light" {
		t.Fatalf("restored missing file = %#v", document)
	}
}

func TestCodecRejectsMalformedDocumentsAndUnknownTransformers(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{Portable: []string{"**"}})
	for _, test := range []struct {
		rel  string
		data []byte
	}{
		{"settings.json", []byte(`{"theme":`)},
		{"settings.yaml", []byte("theme: [\n")},
		{"settings.toml", []byte("theme = [\n")},
		{"settings.txt", []byte("theme=dark")},
		{"settings.yaml", []byte("1: value\n")},
	} {
		if _, err := registry.Project("demo", test.rel, "linux", "/home/alice", test.data); err == nil {
			t.Fatalf("Project(%q, %q) error = nil", test.rel, test.data)
		}
	}
	if _, err := registry.Project("unknown", "settings.json", "linux", "/home/alice", []byte(`{}`)); err == nil {
		t.Fatal("unknown transformer was accepted")
	}
	if _, err := registry.Restore(
		"demo", "settings.json", "linux", "/home/alice",
		[]byte(`{"broken":`), nil, []byte(`{"theme":"light"}`),
	); err == nil {
		t.Fatal("malformed local document was accepted")
	}
}

func TestGenericSafeCanonicalizesOnlySecretFreeDocuments(t *testing.T) {
	registry := NewRegistry()
	safe, err := registry.Project(
		"generic-safe", "settings.json", "linux", "/home/alice",
		[]byte(`{"theme":"dark"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(safe, []byte(`"theme": "dark"`)) {
		t.Fatalf("canonical generic projection = %s", safe)
	}
	for _, data := range [][]byte{
		[]byte(`{"apiKey":"secret"}`),
		[]byte(`{"nested":{"refresh_token":"secret"}}`),
	} {
		if _, err := registry.Project("generic-safe", "settings.json", "linux", "/home/alice", data); err == nil {
			t.Fatalf("generic-safe leaked %s", data)
		}
	}
}

func TestGenericSafeRestoreRefusesToReplaceLocalDocumentContainingSecrets(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Restore(
		"generic-safe", "settings.json", "linux", "/home/alice",
		[]byte(`{"apiKey":"local-secret","theme":"dark"}`),
		[]byte(`{"theme":"dark"}`),
		[]byte(`{"theme":"light"}`),
	); err == nil {
		t.Fatal("generic-safe restore accepted a mixed local credential document")
	}
}

func TestCodecLeavesUndeclaredStringsUntouchedAndOmitsUnmappablePathFields(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{
		Portable:   []string{"**"},
		PathFields: []string{"skillsPath"},
	})
	projected, err := registry.Project(
		"demo", "settings.json", "windows", `C:\Users\alice`,
		[]byte(`{"label":"C:\\Users\\alice\\label","skillsPath":"D:\\shared\\skills"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, document, err := Parse("settings.json", projected)
	if err != nil {
		t.Fatal(err)
	}
	if document["label"] != `C:\Users\alice\label` {
		t.Fatalf("ordinary string was rewritten: %#v", document["label"])
	}
	if _, exists := document["skillsPath"]; exists {
		t.Fatalf("unmappable path field was projected: %#v", document)
	}
}

func TestPolicyDoublestarMatchesZeroOrMoreDotSeparatedSegments(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{
		Portable:   []string{"**"},
		Sensitive:  []string{"**.*token*"},
		PathFields: []string{"**.*path"},
	})
	projected, err := registry.Project(
		"demo", "settings.json", "windows", `C:\Users\alice`,
		[]byte(`{
			"accessToken":"root-secret",
			"nested":{"refreshToken":"nested-secret"},
			"skillsPath":"C:\\Users\\alice\\.agents\\skills"
		}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, document, err := Parse("settings.json", projected)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := document["accessToken"]; exists {
		t.Fatalf("root sensitive field was projected: %#v", document)
	}
	if nested, _ := document["nested"].(map[string]any); len(nested) != 0 {
		t.Fatalf("nested sensitive field was projected: %#v", document)
	}
	if document["skillsPath"] != `${HOME}/.agents/skills` {
		t.Fatalf("root path field = %#v", document["skillsPath"])
	}
}

func TestDocumentMarshalIsDeterministic(t *testing.T) {
	for _, rel := range []string{"settings.json", "settings.yaml", "settings.toml"} {
		format, document, err := Parse(rel, []byte(documentFixture(rel)))
		if err != nil {
			t.Fatal(err)
		}
		first, err := Marshal(format, document)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Marshal(format, document)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("%s output is nondeterministic:\n%s\n%s", rel, first, second)
		}
	}
}

func documentFixture(rel string) string {
	switch {
	case strings.HasSuffix(rel, ".json"):
		return `{"z":1,"a":{"b":true}}`
	case strings.HasSuffix(rel, ".yaml"):
		return "z: 1\na:\n  b: true\n"
	default:
		return "z = 1\n[a]\nb = true\n"
	}
}

func lookup(document map[string]any, path ...string) any {
	var current any = document
	for _, segment := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[segment]
	}
	return current
}
