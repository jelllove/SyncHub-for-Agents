package repocheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func devcontainerFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func devcontainerInstructions(t *testing.T) []string {
	t.Helper()
	var result []string
	var current string
	for _, line := range strings.Split(devcontainerFile(t, ".devcontainer", "Dockerfile"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		current += strings.TrimSuffix(line, "\\") + " "
		if strings.HasSuffix(line, "\\") {
			continue
		}
		result = append(result, strings.Join(strings.Fields(current), " "))
		current = ""
	}
	if current != "" {
		t.Fatal("Dockerfile has an unterminated continuation")
	}
	return result
}

func TestDevcontainerToolchainsMatchRepositoryPins(t *testing.T) {
	goPin := regexp.MustCompile(`(?m)^go (\d+\.\d+\.\d+)$`).FindStringSubmatch(devcontainerFile(t, "go.mod"))
	if len(goPin) != 2 {
		t.Fatal("go.mod must declare an exact Go version")
	}
	nodePin := strings.TrimSpace(devcontainerFile(t, ".node-version"))
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(nodePin) {
		t.Fatal(".node-version must declare an exact Node version")
	}
	instructions := devcontainerInstructions(t)
	var images []string
	var lastUser string
	for _, instruction := range instructions {
		if strings.HasPrefix(instruction, "FROM ") {
			images = append(images, instruction)
		}
		if strings.HasPrefix(instruction, "USER ") {
			lastUser = instruction
		}
	}
	if lastUser != "USER vscode" {
		t.Fatalf("the final image must default to the non-root developer, got %q", lastUser)
	}
	pins := []struct{ image, stage string }{
		{"docker.io/library/golang:" + goPin[1] + "-bookworm", " AS go-toolchain"},
		{"docker.io/library/node:" + nodePin + "-bookworm-slim", " AS node-toolchain"},
		{"mcr.microsoft.com/devcontainers/base:ubuntu-24.04", ""},
	}
	if len(images) != len(pins) {
		t.Fatalf("expected only the three pinned toolchain/base stages, got %v", images)
	}
	doc := devcontainerFile(t, "docs", "devcontainer.md")
	for i, pin := range pins {
		pattern := "^FROM " + regexp.QuoteMeta(pin.image) + `@sha256:[a-f0-9]{64}` + regexp.QuoteMeta(pin.stage) + "$"
		if !regexp.MustCompile(pattern).MatchString(images[i]) {
			t.Fatalf("stage must pin %s by immutable digest: %s", pin.image, images[i])
		}
		if !strings.Contains(doc, strings.Fields(images[i])[1]) {
			t.Errorf("document the exact selected image identity: %s", images[i])
		}
	}
	for _, required := range []string{
		"COPY --from=go-toolchain /usr/local/go/ /usr/local/go/",
		"COPY --from=node-toolchain /usr/local/bin/node /usr/local/bin/node",
		"COPY --from=node-toolchain /usr/local/lib/node_modules/npm/ /usr/local/lib/node_modules/npm/",
		"USER vscode",
		"WORKDIR /workspaces/synchub-for-agents",
	} {
		if !slices.Contains(instructions, required) {
			t.Errorf("missing toolchain/user instruction: %s", required)
		}
	}
	script := strings.Join(instructions, "\n")
	for _, required := range []string{
		"GOTOOLCHAIN=local",
		`test "$(go env GOVERSION)" = "go` + goPin[1] + `"`,
		`test "$(node -p process.versions.node)" = "` + nodePin + `"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("build must check the installed pins without automatic toolchain switching: %s", required)
		}
	}
}

func TestDevcontainerInstallsNativePrerequisitesFromSignedSnapshot(t *testing.T) {
	script := strings.Join(devcontainerInstructions(t), "\n")
	install := regexp.MustCompile(`apt-get install --yes --no-install-recommends (.+?) &&`).FindStringSubmatch(script)
	if len(install) != 2 {
		t.Fatal("expected an explicit, fail-closed native dependency install")
	}
	packages := strings.Fields(install[1])
	for _, name := range []string{"gcc", "pkg-config", "libgtk-4-dev", "libwebkitgtk-6.0-dev", "libfuse2", "xvfb", "xauth"} {
		if !slices.Contains(packages, name) {
			t.Errorf("missing Linux prerequisite %s", name)
		}
	}
	for _, required := range []string{
		"COPY ubuntu.sources /etc/apt/sources.list.d/ubuntu.sources",
		"apt-get update -o APT::Update::Error-Mode=any",
		"pkg-config --modversion gtk4 webkitgtk-6.0",
		"ldconfig -p | grep -F libfuse.so.2",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("missing native build validation: %s", required)
		}
	}
	sources := devcontainerFile(t, ".devcontainer", "ubuntu.sources")
	snapshot := regexp.MustCompile(`(?m)^URIs: https://snapshot\.ubuntu\.com/ubuntu/(\d{8}T\d{6}Z)/$`).FindStringSubmatch(sources)
	if len(snapshot) != 2 {
		t.Fatal("Ubuntu dependencies must use a dated HTTPS snapshot, not a moving mirror")
	}
	if _, err := time.Parse("20060102T150405Z", snapshot[1]); err != nil {
		t.Fatalf("invalid snapshot timestamp: %v", err)
	}
	for _, required := range []string{
		"Types: deb", "Suites: noble noble-updates noble-security",
		"Components: main universe restricted multiverse",
		"Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg",
	} {
		if !slices.Contains(strings.Split(sources, "\n"), required) {
			t.Errorf("missing signed Ubuntu source setting: %s", required)
		}
	}
	for _, unsafe := range []string{"Trusted:", "Check-Valid-Until:", "Allow-Insecure:", "--allow-unauthenticated"} {
		if strings.Contains(sources+"\n"+script, unsafe) {
			t.Errorf("do not bypass package verification with %s", unsafe)
		}
	}
}

func TestDevcontainerUsesScopedNonRootSetup(t *testing.T) {
	var config struct {
		Name  string `json:"name"`
		Build struct {
			Dockerfile string `json:"dockerfile"`
			Context    string `json:"context"`
		} `json:"build"`
		WorkspaceMount      string   `json:"workspaceMount"`
		WorkspaceFolder     string   `json:"workspaceFolder"`
		ContainerUser       string   `json:"containerUser"`
		RemoteUser          string   `json:"remoteUser"`
		UpdateRemoteUserUID bool     `json:"updateRemoteUserUID"`
		Privileged          bool     `json:"privileged"`
		Init                bool     `json:"init"`
		RunArgs             []string `json:"runArgs"`
		PostCreateCommand   []string `json:"postCreateCommand"`
		WaitFor             string   `json:"waitFor"`
	}
	decoder := json.NewDecoder(strings.NewReader(devcontainerFile(t, ".devcontainer", "devcontainer.json")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		t.Fatalf("unexpected devcontainer configuration (review new hooks, mounts or features): %v", err)
	}
	if config.Build.Context != "." || config.Build.Dockerfile != "Dockerfile" {
		t.Fatal("build context must remain .devcontainer, never the repository or its parent")
	}
	if config.ContainerUser != "vscode" || config.RemoteUser != "vscode" || config.Privileged || !config.Init {
		t.Fatal("container processes and lifecycle commands must run as the non-root developer with an init")
	}
	if config.WorkspaceFolder != "/workspaces/synchub-for-agents" ||
		config.WorkspaceMount != "source=${localWorkspaceFolder},target=/workspaces/synchub-for-agents,type=bind" {
		t.Fatal("interactive development must mount only the selected repository, not host homes or credentials")
	}
	if !reflect.DeepEqual(config.RunArgs, []string{"--cap-drop=ALL", "--security-opt=no-new-privileges"}) {
		t.Fatal("do not enable capabilities, host networking, devices or extra host mounts")
	}
	if !reflect.DeepEqual(config.PostCreateCommand, []string{"node", "scripts/dev.mjs", "setup"}) ||
		config.WaitFor != "postCreateCommand" {
		t.Fatal("wait for the existing setup command; do not install global tools or change shared Git configuration")
	}
}

func TestDevcontainerBuildContextContainsOnlyBuildInputs(t *testing.T) {
	ignore := strings.Fields(devcontainerFile(t, ".devcontainer", ".dockerignore"))
	if !reflect.DeepEqual(ignore, []string{"**", "!Dockerfile", "!.dockerignore", "!ubuntu.sources"}) {
		t.Fatal("allow only explicit devcontainer build inputs, even inside the scoped context")
	}
	for _, instruction := range devcontainerInstructions(t) {
		if strings.HasPrefix(instruction, "ADD ") {
			t.Fatal("do not add unchecked downloads or additional source trees to the image")
		}
		if strings.HasPrefix(instruction, "COPY ") && !strings.HasPrefix(instruction, "COPY --from=") &&
			instruction != "COPY ubuntu.sources /etc/apt/sources.list.d/ubuntu.sources" {
			t.Fatalf("unexpected local build input: %s", instruction)
		}
	}
}
