package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	RepositoryURL = "https://github.com/jelllove/SyncHub-for-Agents"
	releaseAPI    = "https://api.github.com/repos/jelllove/SyncHub-for-Agents/releases/latest"
	InstallerName = "SyncHub-for-Agents-Setup-x64.exe"
	maxInstaller  = 256 << 20
)

type version [3]uint64

func parseVersion(text string) (version, error) {
	var result version
	parts := strings.Split(strings.TrimPrefix(text, "v"), ".")
	if len(parts) != 3 {
		return result, fmt.Errorf("not a stable release version: %q", text)
	}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return result, fmt.Errorf("invalid version: %q", text)
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return result, fmt.Errorf("not a stable release version: %q", text)
			}
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return result, fmt.Errorf("invalid version: %q", text)
		}
		result[i] = n
	}
	return result, nil
}

func (v version) newerThan(other version) bool {
	for i := range v {
		if v[i] != other[i] {
			return v[i] > other[i]
		}
	}
	return false
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

type releaseClient struct {
	http *http.Client
}

func newReleaseClient() *releaseClient {
	return &releaseClient{http: &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many update download redirects")
			}
			if !trustedDownloadURL(req.URL) {
				return errors.New("update redirected outside GitHub HTTPS hosts")
			}
			return nil
		},
	}}
}

func trustedDownloadURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	default:
		return false
	}
}

func (c *releaseClient) get(ctx context.Context, location string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SyncHub-for-Agents-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request update: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub update request returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (c *releaseClient) latest(ctx context.Context) (*release, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := c.get(ctx, releaseAPI)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := readLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, err
	}
	var result release
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode GitHub release: %w", err)
	}
	if result.Draft || result.Prerelease {
		return nil, errors.New("GitHub latest release is not a published stable release")
	}
	if _, err := parseVersion(result.Tag); err != nil {
		return nil, err
	}
	return &result, nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("update response exceeds size limit")
	}
	return data, nil
}

func (r *release) findAsset(name string) (asset, error) {
	var found *asset
	for _, candidate := range r.Assets {
		if candidate.Name != name {
			continue
		}
		if found != nil {
			return asset{}, fmt.Errorf("release contains duplicate asset %s", name)
		}
		found = &candidate
	}
	if found == nil {
		return asset{}, fmt.Errorf("release %s has no %s; use the release page for manual installation", r.Tag, name)
	}
	expected := RepositoryURL + "/releases/download/" + url.PathEscape(r.Tag) + "/" + name
	if found.URL != expected {
		return asset{}, fmt.Errorf("unexpected download URL for %s", name)
	}
	if found.Size <= 0 || found.Size > maxInstaller {
		return asset{}, fmt.Errorf("invalid release asset size for %s", name)
	}
	return *found, nil
}

func installerChecksum(data []byte) (string, error) {
	var checksum string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != InstallerName {
			continue
		}
		raw, err := hex.DecodeString(fields[0])
		if err != nil || len(raw) != sha256.Size || checksum != "" {
			return "", errors.New("invalid or duplicate installer checksum")
		}
		checksum = strings.ToLower(fields[0])
	}
	if checksum == "" {
		return "", errors.New("SHA256SUMS.txt does not contain the Windows installer")
	}
	return checksum, nil
}

type pendingUpdate struct {
	Path     string
	Checksum string
	Version  string
}

func (c *releaseClient) download(ctx context.Context, r *release, directory string) (*pendingUpdate, error) {
	installer, err := r.findAsset(InstallerName)
	if err != nil {
		return nil, err
	}
	sums, err := r.findAsset("SHA256SUMS.txt")
	if err != nil {
		return nil, err
	}
	resp, err := c.get(ctx, sums.URL)
	if err != nil {
		return nil, err
	}
	data, readErr := readLimited(resp.Body, 64<<10)
	resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	checksum, err := installerChecksum(data)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create update directory: %w", err)
	}
	resp, err = c.get(ctx, installer.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	file, err := os.CreateTemp(directory, "installer-*.exe")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		file.Close()
		if !keep {
			os.Remove(file.Name())
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, installer.Size+1))
	if err != nil {
		return nil, fmt.Errorf("download installer: %w", err)
	}
	if size != installer.Size {
		return nil, errors.New("downloaded installer size differs from release metadata")
	}
	if hex.EncodeToString(hash.Sum(nil)) != checksum {
		return nil, errors.New("installer SHA-256 verification failed; refusing to install")
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	keep = true
	return &pendingUpdate{Path: file.Name(), Checksum: checksum, Version: r.Tag}, nil
}

func verifyInstaller(path, checksum string) error {
	raw, err := hex.DecodeString(checksum)
	if err != nil || len(raw) != sha256.Size {
		return errors.New("invalid expected installer checksum")
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, maxInstaller+1))
	if err != nil {
		return err
	}
	if size == 0 || size > maxInstaller || hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(checksum) {
		return errors.New("cached installer SHA-256 verification failed")
	}
	return nil
}
