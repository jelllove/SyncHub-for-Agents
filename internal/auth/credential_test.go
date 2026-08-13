package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestCredentialGetAllowsOnlyGitHubHTTPS(t *testing.T) {
	store := NewStore(memoryBackend{})
	account := Account{ID: 42, Login: "alice"}
	if err := store.Save(account, "gho_secret"); err != nil {
		t.Fatal(err)
	}
	handler := CredentialHandler{Store: store, Account: account}

	var output bytes.Buffer
	err := handler.Run("get", strings.NewReader("protocol=https\nhost=github.com\n\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "username=alice\npassword=gho_secret\n\n" {
		t.Fatalf("output = %q", got)
	}

	output.Reset()
	err = handler.Run("get", strings.NewReader("protocol=https\nhost=evil.example\n\n"), &output)
	if err == nil {
		t.Fatal("unsupported host unexpectedly accepted")
	}
	if strings.Contains(output.String(), "gho_") {
		t.Fatal("token leaked for unsupported host")
	}
}

func TestCredentialGetSignalsMissingCredential(t *testing.T) {
	handler := CredentialHandler{
		Store:   NewStore(memoryBackend{}),
		Account: Account{ID: 42, Login: "alice"},
	}
	var output bytes.Buffer
	err := handler.Run("get", strings.NewReader("protocol=https\nhost=github.com:443\n\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "quit=1\n\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestCredentialEraseRequiresMatchingToken(t *testing.T) {
	store := NewStore(memoryBackend{})
	account := Account{ID: 42, Login: "alice"}
	if err := store.Save(account, "gho_secret"); err != nil {
		t.Fatal(err)
	}
	handler := CredentialHandler{Store: store, Account: account}

	err := handler.Run("erase", strings.NewReader("protocol=https\nhost=github.com\npassword=wrong\n\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Token(42); err != nil {
		t.Fatalf("wrong token erased credential: %v", err)
	}
	err = handler.Run("erase", strings.NewReader("protocol=https\nhost=github.com\npassword=gho_secret\n\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Token(42); err != ErrNotFound {
		t.Fatalf("credential still exists: %v", err)
	}
}
