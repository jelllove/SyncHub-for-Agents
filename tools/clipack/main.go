package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type options struct {
	GOOS    string
	Arch    string
	Version string
	Out     string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.GOOS, "os", runtime.GOOS, "Target OS: windows, darwin or linux")
	flag.StringVar(&opts.Arch, "arch", runtime.GOARCH, "Target Go architecture")
	flag.StringVar(&opts.Version, "version", "0.0.0", "Release version (optional v prefix)")
	flag.StringVar(&opts.Out, "out", filepath.Join(".artifacts", "cli-package"), "Output directory")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	if err := packageCLI(opts); err != nil {
		fmt.Fprintln(os.Stderr, "package CLI:", err)
		os.Exit(1)
	}
}

func packageCLI(opts options) (retErr error) {
	opts.Version = strings.TrimPrefix(opts.Version, "v")
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`).MatchString(opts.Version) {
		return fmt.Errorf("version must be a semantic version without whitespace or path separators")
	}
	supported := opts.Arch == "amd64" && (opts.GOOS == "windows" || opts.GOOS == "linux" || opts.GOOS == "darwin") ||
		opts.GOOS == "darwin" && opts.Arch == "arm64"
	if !supported {
		return fmt.Errorf("unsupported CLI target %s/%s", opts.GOOS, opts.Arch)
	}
	platform, arch := opts.GOOS, opts.Arch
	if platform == "darwin" {
		platform = "macos"
	}
	if arch == "amd64" {
		arch = "x64"
	}
	suffix := ".tar.gz"
	binaryName := "synchub"
	if opts.GOOS == "windows" {
		suffix = ".zip"
		binaryName += ".exe"
	}
	asset := "SyncHub-CLI-" + platform + "-" + arch + suffix
	manifest := "SHA256SUMS-cli-" + platform + "-" + arch + ".txt"
	receipt := "CLI-BUILD-" + platform + "-" + arch + ".json"
	for _, name := range []string{asset, manifest, receipt} {
		if _, err := os.Lstat(filepath.Join(opts.Out, name)); err == nil {
			return fmt.Errorf("refuse to overwrite existing output %s", name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(opts.Out, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(opts.Out, ".cli-build-")
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, os.RemoveAll(stage)) }()
	binary := filepath.Join(stage, binaryName)
	build := exec.Command("go", "build", "-trimpath",
		"-ldflags", "-X github.com/qinqingxu/synchub-for-agents/internal/appversion.Version="+opts.Version,
		"-o", binary, "./cmd/synchub")
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+opts.GOOS, "GOARCH="+opts.Arch)
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build %s/%s: %w\n%s", opts.GOOS, opts.Arch, err, output)
	}
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return fmt.Errorf("read executable build metadata: %w", err)
	}
	settings := make(map[string]string)
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != opts.GOOS || settings["GOARCH"] != opts.Arch || settings["CGO_ENABLED"] != "0" {
		return fmt.Errorf("executable target or CGO metadata does not match requested headless build")
	}
	if opts.GOOS == runtime.GOOS && opts.Arch == runtime.GOARCH {
		smoke := exec.Command(binary, "version", "--json")
		smoke.Env = append(os.Environ(), "HOME="+stage, "USERPROFILE="+stage)
		output, err := smoke.CombinedOutput()
		var response struct {
			SchemaVersion int  `json:"schemaVersion"`
			OK            bool `json:"ok"`
			Data          struct {
				Version string `json:"version"`
			} `json:"data"`
		}
		parseErr := json.Unmarshal(output, &response)
		if err != nil || parseErr != nil || response.SchemaVersion != 1 || !response.OK || response.Data.Version != opts.Version {
			return fmt.Errorf("native CLI version smoke failed: %v, output=%q", err, output)
		}
	}
	for source, destination := range map[string]string{"LICENSE": "LICENSE", filepath.Join("docs", "cli.md"): "CLI.md"} {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stage, destination), data, 0o644); err != nil {
			return err
		}
	}
	stagedArchive := filepath.Join(stage, asset)
	if err := writeArchive(stagedArchive, stage, binaryName, opts.GOOS == "windows"); err != nil {
		return err
	}
	data, err := os.ReadFile(stagedArchive)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	if err := publishBytes(filepath.Join(opts.Out, asset), data); err != nil {
		return err
	}
	if err := publishBytes(filepath.Join(opts.Out, manifest), []byte(hash+"  "+asset+"\n")); err != nil {
		return err
	}
	receiptData, err := json.MarshalIndent(map[string]any{
		"schemaVersion": 1, "version": opts.Version, "goos": opts.GOOS, "goarch": opts.Arch,
		"goVersion": info.GoVersion, "cgoEnabled": false, "asset": asset, "sha256": hash,
		"sourceCommit": settings["vcs.revision"], "sourceDirty": settings["vcs.modified"] == "true",
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := publishBytes(filepath.Join(opts.Out, receipt), append(receiptData, '\n')); err != nil {
		return err
	}
	fmt.Printf("Packaged %s (SHA-256 %s)\n", asset, hash)
	return nil
}

func publishBytes(path string, data []byte) (retErr error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	_, err = file.Write(data)
	return err
}

func writeArchive(destination, root, binaryName string, zipped bool) (retErr error) {
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	names := []string{binaryName, "LICENSE", "CLI.md"}
	if zipped {
		writer := zip.NewWriter(file)
		defer func() { retErr = errors.Join(retErr, writer.Close()) }()
		for _, name := range names {
			data, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				return err
			}
			header := &zip.FileHeader{Name: "synchub-cli/" + name, Method: zip.Deflate}
			header.SetMode(0o644)
			if name == binaryName {
				header.SetMode(0o755)
			}
			entry, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			if _, err := entry.Write(data); err != nil {
				return err
			}
		}
		return nil
	}
	gz := gzip.NewWriter(file)
	defer func() { retErr = errors.Join(retErr, gz.Close()) }()
	writer := tar.NewWriter(gz)
	defer func() { retErr = errors.Join(retErr, writer.Close()) }()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		mode := int64(0o644)
		if name == binaryName {
			mode = 0o755
		}
		if err := writer.WriteHeader(&tar.Header{
			Name: "synchub-cli/" + name, Mode: mode, Size: int64(len(data)), ModTime: time.Unix(0, 0),
		}); err != nil {
			return err
		}
		if _, err := io.Copy(writer, bytes.NewReader(data)); err != nil {
			return err
		}
	}
	return nil
}
