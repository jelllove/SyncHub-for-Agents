package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qinqingxu/acsync/internal/auth"
	"github.com/qinqingxu/acsync/internal/gitclient"
)

type credentialBackend map[string]string

func (b credentialBackend) Set(service, user, password string) error {
	b[service+"\x00"+user] = password
	return nil
}

func (b credentialBackend) Get(service, user string) (string, error) {
	value, ok := b[service+"\x00"+user]
	if !ok {
		return "", auth.ErrNotFound
	}
	return value, nil
}

func (b credentialBackend) Delete(service, user string) error {
	key := service + "\x00" + user
	if _, ok := b[key]; !ok {
		return auth.ErrNotFound
	}
	delete(b, key)
	return nil
}

func TestRunCredentialLoadsMetadataAndKeepsUnsupportedHostsSecret(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	account := auth.Account{ID: 42, Login: "alice", Scopes: []string{"repo"}}
	if err := auth.SaveMetadata(AuthMetadataPath(home), auth.Metadata{Active: account}); err != nil {
		t.Fatal(err)
	}

	store := auth.NewStore(credentialBackend{})
	if err := store.Save(account, "gho_secret"); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := RunCredentialWith(home, "get", strings.NewReader("protocol=https\nhost=github.com\n\n"), &output, store)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "password=gho_secret") {
		t.Fatalf("output = %q", output.String())
	}

	output.Reset()
	err = RunCredentialWith(home, "get", strings.NewReader("protocol=https\nhost=evil.example\n\n"), &output, store)
	if err == nil {
		t.Fatal("unsupported host unexpectedly accepted")
	}
	if strings.Contains(output.String(), "gho_secret") {
		t.Fatal("token leaked for unsupported host")
	}
	if errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("unexpected metadata error: %v", err)
	}
}

func TestNewGitClientUsesOAuthHelperWhenMetadataExists(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	account := auth.Account{ID: 42, Login: "alice", Scopes: []string{"repo"}}
	if err := auth.SaveMetadata(AuthMetadataPath(home), auth.Metadata{Active: account}); err != nil {
		t.Fatal(err)
	}
	client, err := NewGitClient(home, "https://github.com/acme/sync.git", filepath.Join(home, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	if client.AuthMode != gitclient.AuthOAuth || !filepath.IsAbs(client.Executable) {
		t.Fatalf("client = %#v", client)
	}
}
