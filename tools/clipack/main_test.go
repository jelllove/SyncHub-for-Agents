package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveContainsOnlyExecutableAndDocumentation(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			root := t.TempDir()
			name := "synchub"
			if goos == "windows" {
				name += ".exe"
			}
			files := map[string]string{name: "synthetic executable", "LICENSE": "synthetic license", "CLI.md": "synthetic guide"}
			for path, content := range files {
				if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			archive := filepath.Join(t.TempDir(), "package")
			if err := writeArchive(archive, root, name, goos == "windows"); err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]string)
			if goos == "windows" {
				reader, err := zip.OpenReader(archive)
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				for _, file := range reader.File {
					entry, err := file.Open()
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(entry)
					entry.Close()
					if err != nil {
						t.Fatal(err)
					}
					seen[file.Name] = string(data)
				}
			} else {
				file, err := os.Open(archive)
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
				for {
					header, err := reader.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					if header.Name == "synchub-cli/"+name && header.Mode != 0o755 {
						t.Fatalf("executable mode = %o", header.Mode)
					}
					data, err := io.ReadAll(reader)
					if err != nil {
						t.Fatal(err)
					}
					seen[header.Name] = string(data)
				}
			}
			if len(seen) != len(files) {
				t.Fatalf("archive entries = %v", seen)
			}
			for path, content := range files {
				if seen["synchub-cli/"+path] != content {
					t.Fatalf("archive missing exact bytes of %s", path)
				}
			}
		})
	}
}

func TestPackagingRejectsUnsupportedTargetsAndUnsafeVersions(t *testing.T) {
	for _, opts := range []options{
		{GOOS: "windows", Arch: "arm64", Version: "0.1.0"},
		{GOOS: "linux", Arch: "amd64", Version: "../invalid"},
		{GOOS: "darwin", Arch: "arm64", Version: "0.1.0 -X unsafe=value"},
	} {
		opts.Out = filepath.Join(t.TempDir(), "not-created")
		if err := packageCLI(opts); err == nil {
			t.Fatalf("invalid packaging options accepted: %+v", opts)
		}
		if _, err := os.Stat(opts.Out); !os.IsNotExist(err) {
			t.Fatalf("invalid options created output: %v", err)
		}
	}
}
