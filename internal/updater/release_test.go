package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func fixtureClient(t *testing.T, mutate func(*release), installer, sums string) *releaseClient {
	t.Helper()
	r := release{Tag: "v0.3.0"}
	for name, body := range map[string]string{InstallerName: installer, "SHA256SUMS.txt": sums} {
		r.Assets = append(r.Assets, asset{
			Name: name,
			URL:  RepositoryURL + "/releases/download/" + r.Tag + "/" + name,
			Size: int64(len(body)),
		})
	}
	if mutate != nil {
		mutate(&r)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return &releaseClient{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Error("update request must not send repository credentials")
		}
		switch req.URL.String() {
		case releaseAPI:
			return response(string(data)), nil
		case RepositoryURL + "/releases/download/v0.3.0/SHA256SUMS.txt":
			return response(sums), nil
		case RepositoryURL + "/releases/download/v0.3.0/" + InstallerName:
			return response(installer), nil
		default:
			t.Errorf("unexpected request: %s", req.URL)
			return nil, errors.New("unexpected URL")
		}
	})}}
}

func sumLine(installer string) string {
	hash := sha256.Sum256([]byte(installer))
	return hex.EncodeToString(hash[:]) + "  " + InstallerName + "\n"
}

func TestStableVersionOrdering(t *testing.T) {
	for _, input := range []string{"dev", "", "v1", "1.2.3-beta.1", "v1.2.3+dev", "1.02.3", "1.2.-1", " 1.2.3", "1.2.3/../x", "1.2.18446744073709551616"} {
		if _, err := parseVersion(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	for _, pair := range [][2]string{{"v0.3.0", "0.2.3"}, {"1.10.0", "v1.9.9"}, {"2.0.0", "1.99.99"}} {
		newer, _ := parseVersion(pair[0])
		older, _ := parseVersion(pair[1])
		if !newer.newerThan(older) || older.newerThan(newer) || newer.newerThan(newer) {
			t.Errorf("incorrect comparison of %v", pair)
		}
	}
}

func TestDownloadRequiresMatchingAssetAndChecksum(t *testing.T) {
	for _, test := range []struct {
		name      string
		mutate    func(*release)
		body      string
		sums      string
		wantError bool
	}{
		{name: "verified", body: "installer", sums: sumLine("installer")},
		{name: "tampered", body: "tampered!", sums: sumLine("installer"), wantError: true},
		{name: "missing checksum", body: "installer", sums: "different-file", wantError: true},
		{name: "duplicate checksum", body: "installer", sums: sumLine("installer") + sumLine("installer"), wantError: true},
		{name: "invalid hex", body: "installer", sums: strings.Repeat("z", 64) + "  " + InstallerName, wantError: true},
		{name: "source only", body: "installer", sums: sumLine("installer"), wantError: true,
			mutate: func(r *release) { r.Assets = nil }},
		{name: "foreign URL", body: "installer", sums: sumLine("installer"), wantError: true,
			mutate: func(r *release) { r.Assets[0].URL = "https://example.com/update.exe" }},
		{name: "HTTP URL", body: "installer", sums: sumLine("installer"), wantError: true,
			mutate: func(r *release) { r.Assets[0].URL = strings.Replace(r.Assets[0].URL, "https:", "http:", 1) }},
		{name: "duplicate asset", body: "installer", sums: sumLine("installer"), wantError: true,
			mutate: func(r *release) { r.Assets = append(r.Assets, r.Assets[0]) }},
		{name: "oversize", body: "installer", sums: sumLine("installer"), wantError: true,
			mutate: func(r *release) { r.Assets[0].Size = maxInstaller + 1 }},
		{name: "size mismatch", body: "installer", sums: sumLine("installer"), wantError: true,
			mutate: func(r *release) {
				for i := range r.Assets {
					if r.Assets[i].Name == InstallerName {
						r.Assets[i].Size++
					}
				}
			}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := fixtureClient(t, test.mutate, test.body, test.sums)
			r, err := client.latest(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			pending, err := client.download(context.Background(), r, dir)
			if (err != nil) != test.wantError {
				t.Fatalf("download = %#v, error = %v", pending, err)
			}
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if test.wantError {
				if len(files) != 0 {
					t.Fatal("failed download left executable cache")
				}
				return
			}
			if err := verifyInstaller(pending.Path, pending.Checksum); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pending.Path, []byte("tampered"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := verifyInstaller(pending.Path, pending.Checksum); err == nil {
				t.Fatal("accepted modified cached installer")
			}
		})
	}
}

func TestUnpublishedAndPrereleaseAreNeverInstalled(t *testing.T) {
	for _, mutate := range []func(*release){
		func(r *release) { r.Draft = true },
		func(r *release) { r.Prerelease = true },
		func(r *release) { r.Tag = "v0.3.0-beta" },
	} {
		client := fixtureClient(t, mutate, "installer", sumLine("installer"))
		if _, err := client.latest(context.Background()); err == nil {
			t.Fatal("accepted an unpublished or prerelease version")
		}
	}
}

func TestReleaseHTTPErrorsAndRedirectAllowlist(t *testing.T) {
	client := newReleaseClient()
	client.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
	})
	if _, err := client.latest(context.Background()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %v", err)
	}
	for _, test := range []struct {
		location string
		trusted  bool
	}{
		{"https://github.com/jelllove/SyncHub-for-Agents", true},
		{"https://release-assets.githubusercontent.com/download?token=public-download", true},
		{"http://github.com/file", false},
		{"https://github.com.evil.example/file", false},
		{"https://evil.example/github.com", false},
		{"https://user@github.com/file", false},
		{"https://github.com:8443/file", false},
	} {
		u, err := url.Parse(test.location)
		if err != nil {
			t.Fatal(err)
		}
		if got := trustedDownloadURL(u); got != test.trusted {
			t.Errorf("trusted(%s) = %v", test.location, got)
		}
		err = client.http.CheckRedirect(&http.Request{URL: u}, nil)
		if (err == nil) != test.trusted {
			t.Errorf("redirect error = %v for %s", err, u)
		}
	}
}

func TestReadLimitsAndBinaryChecksumFormat(t *testing.T) {
	if _, err := readLimited(strings.NewReader("abc"), 2); err == nil {
		t.Fatal("oversized response accepted")
	}
	line := strings.Replace(sumLine("installer"), "  ", " *", 1)
	if _, err := installerChecksum([]byte(line)); err != nil {
		t.Fatal(err)
	}
}
