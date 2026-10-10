package updater

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

type handshakeTimeout struct{}

func (handshakeTimeout) Error() string   { return "net/http: TLS handshake timeout" }
func (handshakeTimeout) Timeout() bool   { return true }
func (handshakeTimeout) Temporary() bool { return true }

func TestUpdateRequestRecoversFromTLSHandshakeTimeout(t *testing.T) {
	attempts := 0
	client := &releaseClient{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if req.Header.Get("Authorization") != "" {
			t.Fatal("retries must not send private repository credentials")
		}
		if attempts < 3 {
			return nil, handshakeTimeout{}
		}
		return response(`{"tag_name":"v0.3.5","assets":[]}`), nil
	})}}
	release, err := client.latest(context.Background())
	if err != nil || release.Tag != "v0.3.5" || attempts != 3 {
		t.Fatalf("release = %#v, attempts = %d, error = %v", release, attempts, err)
	}
}

func TestUpdateRequestRetriesAreBoundedAndCancellationIsPreserved(t *testing.T) {
	attempts := 0
	client := &releaseClient{http: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return nil, handshakeTimeout{}
	})}}
	if _, err := client.latest(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "3 attempts") ||
		!strings.Contains(err.Error(), "proxy") || attempts != 3 {
		t.Fatalf("attempts = %d, error = %v", attempts, err)
	}
	attempts = 0
	ctx, cancel := context.WithCancel(context.Background())
	client.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		cancel()
		return nil, handshakeTimeout{}
	})
	if _, err := client.latest(ctx); !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("cancellation: attempts = %d, error = %v", attempts, err)
	}
}

type trackedResponseBody struct {
	io.Reader
	closed bool
}

func (body *trackedResponseBody) Close() error {
	body.closed = true
	return nil
}

func TestUpdateRequestRetriesTransientHTTPErrorsOnly(t *testing.T) {
	for _, code := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			attempts := 0
			body := &trackedResponseBody{Reader: strings.NewReader("temporary outage")}
			client := &releaseClient{http: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				attempts++
				if attempts == 1 {
					return &http.Response{StatusCode: code, Body: body}, nil
				}
				return response(`{"tag_name":"v0.3.5","assets":[]}`), nil
			})}}
			_, err := client.latest(context.Background())
			transient := code >= 500
			if (err == nil) != transient || !body.closed || (transient && attempts != 2) || (!transient && attempts != 1) {
				t.Fatalf("attempts = %d, body closed = %v, error = %v", attempts, body.closed, err)
			}
		})
	}
}

func TestUpdaterHasExplicitBoundedTLSAndMetadataTimeouts(t *testing.T) {
	client := newReleaseClient()
	transport, ok := client.http.Transport.(*http.Transport)
	if !ok || transport.TLSHandshakeTimeout != 20*time.Second ||
		transport.ResponseHeaderTimeout != 30*time.Second || transport.Proxy == nil ||
		(transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify) {
		t.Fatalf("unsafe or missing transport limits: %#v", client.http.Transport)
	}
	client.http.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining < 89*time.Second || remaining > 90*time.Second {
			t.Errorf("metadata request budget = %v, present = %v", remaining, ok)
		}
		return response(`{"tag_name":"v0.3.5","assets":[]}`), nil
	})
	if _, err := client.latest(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateRequestRecoversFromARealStalledTLSHandshake(t *testing.T) {
	var handshakes atomic.Int32
	releaseFirstHandshake := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v0.3.5","assets":[]}`)
	}))
	server.TLS = &tls.Config{
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			if handshakes.Add(1) == 1 {
				<-releaseFirstHandshake
			}
			return nil, nil
		},
	}
	server.StartTLS()
	defer server.Close()
	defer close(releaseFirstHandshake)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 100 * time.Millisecond
	client := &releaseClient{http: &http.Client{Transport: transport}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := client.get(ctx, server.URL)
	if err != nil {
		t.Fatalf("real TLS timeout did not recover: %v", err)
	}
	defer resp.Body.Close()
	if handshakes.Load() < 2 {
		t.Fatal("the stalled TLS handshake was not retried")
	}
}

func TestUpdateRequestDoesNotRetryCertificateFailures(t *testing.T) {
	attempts := 0
	client := &releaseClient{http: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return nil, x509.UnknownAuthorityError{}
	})}}
	if _, err := client.latest(context.Background()); err == nil || attempts != 1 {
		t.Fatalf("certificate failure: attempts = %d, error = %v", attempts, err)
	}
}

func TestUpdateRequestCancelsDuringRetryDelay(t *testing.T) {
	attempts := 0
	client := &releaseClient{
		retryDelay: time.Hour,
		http: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			return nil, handshakeTimeout{}
		})},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := client.latest(ctx); !errors.Is(err, context.DeadlineExceeded) || attempts != 1 {
		t.Fatalf("retry delay cancellation: attempts = %d, error = %v", attempts, err)
	}
}
