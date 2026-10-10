package repocheck

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func packagingFile(t *testing.T, relative string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPlatformPackagingArtifacts(t *testing.T) {
	release := loadWorkflow(t, "release.yml")
	ci := loadWorkflow(t, "ci.yml")
	for _, pair := range []struct {
		job     job
		formats []string
	}{
		{release.Jobs["windows"], []string{"bin/SyncHub-for-Agents-Setup-x64.exe", "bin/SyncHub-for-Agents-Windows-x64.zip", "bin/SHA256SUMS.txt"}},
		{release.Jobs["macos"], []string{"bin/SyncHub.dmg", "bin/SyncHub-macos-universal.zip"}},
		{release.Jobs["linux"], []string{"bin/*.AppImage", "bin/*.deb", "bin/*.tar.gz", "bin/*.rpm"}},
		{ci.Jobs["package"], []string{"bin/*.exe", "bin/*.zip", "bin/*.dmg", "bin/*.AppImage", "bin/*.deb", "bin/*.tar.gz"}},
	} {
		var artifacts string
		for _, s := range pair.job.Steps {
			if strings.HasPrefix(s.Uses, "actions/upload-artifact@") {
				artifacts += s.With["path"] + "\n"
			}
		}
		for _, format := range pair.formats {
			if !strings.Contains(artifacts, format) {
				t.Errorf("artifact upload missing %q", format)
			}
		}
	}
	for relative, required := range map[string][]string{
		"build/linux/Taskfile.yml": {
			"task: create:tar", "scripts/release/linux-archive.sh",
			"GOARCH: '{{.ARCH | default ARCH}}'", "VERSION: '{{.VERSION",
		},
		"build/darwin/Taskfile.yml":   {"package:zip:", "create:zip:", "ditto -c -k --sequesterRsrc --keepParent"},
		"scripts/release/windows.ps1": {"Write-WindowsPortableBundle", "Write-ReleaseChecksums"},
	} {
		text := packagingFile(t, relative)
		for _, fragment := range required {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s missing %q", relative, fragment)
			}
		}
	}
}

func TestPlatformMacOSArchivesAreStapledBeforePublication(t *testing.T) {
	text := packagingFile(t, ".github/workflows/release.yml")
	appStaple := strings.Index(text, "xcrun stapler staple bin/SyncHub.app")
	finalZip := strings.Index(text, "wails3 task darwin:create:zip ARCH=universal")
	dmg := strings.Index(text, "wails3 task darwin:create:dmg")
	if appStaple < 0 || finalZip < appStaple || dmg < appStaple {
		t.Fatal("both macOS archives must contain the notarized, stapled app")
	}
	if !strings.Contains(text, `lipo bin/SyncHub.app/Contents/MacOS/SyncHub -verify_arch arm64 x86_64`) {
		t.Fatal("Universal releases must prove both CPU architectures are present")
	}
	if !strings.Contains(text, "bash scripts/smoke/linux-rpm-install.sh bin/SyncHub.rpm") {
		t.Fatal("RPM publication must retain the upstream Fedora installation check")
	}
}

func TestPlatformCIExercisesBothMacOSArchitectures(t *testing.T) {
	var ci struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Include []struct {
						OS   string `yaml:"os"`
						Arch string `yaml:"arch"`
					} `yaml:"include"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(packagingFile(t, ".github/workflows/ci.yml")), &ci); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range ci.Jobs["package"].Strategy.Matrix.Include {
		if strings.HasPrefix(entry.OS, "macos-") {
			found[entry.Arch] = true
		}
	}
	if !found["amd64"] || !found["arm64"] {
		t.Fatal("macOS package CI must run on native Intel and Apple Silicon hosts")
	}
}

func TestPlatformWindowsPortableBundle(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell packaging regression runs on Windows")
	}
	cmd := exec.Command("pwsh", "-NoProfile", "-File", filepath.Join("..", "..", "scripts", "release", "test-windows.ps1"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Windows packaging: %v\n%s", err, output)
	}
}

func TestPlatformLinuxArchiveContents(t *testing.T) {
	bash := "bash"
	if runtime.GOOS == "windows" {
		bash = filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe")
		if _, err := os.Stat(bash); err != nil {
			t.Skip("Linux archive fixtures on Windows require Git Bash")
		}
	}
	root := t.TempDir()
	fixtures := map[string]string{
		"bin/SyncHub": "#!/bin/sh\nexit 0\n", "LICENSE": "synthetic license",
		"build/appicon.png": "synthetic icon", "build/linux/SyncHub.desktop": "[Desktop Entry]\nExec=SyncHub\n",
		"docs/install.md": "synthetic installation guide",
	}
	for name, data := range fixtures {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "linux-archive.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, script, root, "SyncHub")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Linux archive: %v\n%s", err, output)
	}
	file, err := os.Open(filepath.Join(root, "bin", "SyncHub-linux-x64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	found := map[string]string{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(header.Name, "SyncHub/")
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		found[name] = string(content)
		if name == "SyncHub" && header.Mode&0o111 == 0 {
			t.Fatal("archive executable lost its executable permissions")
		}
	}
	for archived, source := range map[string]string{
		"SyncHub": "bin/SyncHub", "LICENSE": "LICENSE", "SyncHub.png": "build/appicon.png",
		"SyncHub.desktop": "build/linux/SyncHub.desktop", "INSTALL.md": "docs/install.md",
	} {
		if found[archived] != fixtures[source] {
			t.Errorf("archive entry %s differs from source", archived)
		}
	}
	if len(found) != 5 {
		t.Fatalf("unexpected archive entries: %v", found)
	}
	if err := os.Remove(filepath.Join(root, "LICENSE")); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(bash, script, root, "SyncHub").CombinedOutput(); err == nil {
		t.Fatalf("missing license must fail: %s", output)
	}
	if output, err := exec.Command(bash, script, root, "../escape").CombinedOutput(); err == nil {
		t.Fatalf("invalid application name must fail: %s", output)
	}
}
