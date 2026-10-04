package auth

import (
	"errors"
	"testing"
)

type failingDeleteBackend struct {
	memoryBackend
	failService string
}

func (b failingDeleteBackend) Delete(service, user string) error {
	if service == b.failService {
		return errors.New("synthetic deletion failure")
	}
	return b.memoryBackend.Delete(service, user)
}

func TestForgetRestoresCredentialsWhenEitherDeletionFails(t *testing.T) {
	for _, failed := range []string{keyringService, legacyKeyringService} {
		t.Run(failed, func(t *testing.T) {
			backend := failingDeleteBackend{memoryBackend: memoryBackend{}, failService: failed}
			for _, service := range []string{keyringService, legacyKeyringService} {
				if err := backend.Set(service, accountKey(42), service+"-token"); err != nil {
					t.Fatal(err)
				}
			}
			if err := NewStore(backend).Forget(42); err == nil {
				t.Fatal("credential deletion failure was hidden")
			}
			for _, service := range []string{keyringService, legacyKeyringService} {
				value, err := backend.Get(service, accountKey(42))
				if err != nil || value != service+"-token" {
					t.Fatalf("%s credential was not restored: %v", service, err)
				}
			}
		})
	}
}

func TestForgetRemovesCurrentAndLegacyCredentialOnlyForSelectedAccount(t *testing.T) {
	backend := memoryBackend{}
	for _, service := range []string{keyringService, legacyKeyringService} {
		if err := backend.Set(service, accountKey(42), "synthetic"); err != nil {
			t.Fatal(err)
		}
		if err := backend.Set(service, accountKey(43), "preserved"); err != nil {
			t.Fatal(err)
		}
	}
	store := NewStore(backend)
	if err := store.Forget(42); err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{keyringService, legacyKeyringService} {
		if _, err := backend.Get(service, accountKey(42)); err != ErrNotFound {
			t.Fatalf("%s still has removed account", service)
		}
		if value, err := backend.Get(service, accountKey(43)); err != nil || value != "preserved" {
			t.Fatal("another account was removed")
		}
	}
}

type memoryBackend map[string]string

func (m memoryBackend) Set(service, user, password string) error {
	m[service+"\x00"+user] = password
	return nil
}

func (m memoryBackend) Get(service, user string) (string, error) {
	value, ok := m[service+"\x00"+user]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (m memoryBackend) Delete(service, user string) error {
	delete(m, service+"\x00"+user)
	return nil
}

func TestStoreKeepsTokenSeparateFromAccountMetadata(t *testing.T) {
	store := NewStore(memoryBackend{})
	account := Account{ID: 42, Login: "alice", Scopes: []string{"repo"}}
	if err := store.Save(account, "gho_secret"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Token(42)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gho_secret" {
		t.Fatalf("Token() = %q", got)
	}
	if account.Login == got {
		t.Fatal("token leaked into account metadata")
	}
}

func TestStoreTokenFallsBackToLegacyService(t *testing.T) {
	backend := memoryBackend{}
	account := Account{ID: 42}
	if err := backend.Set(legacyKeyringService, accountKey(account.ID), "legacy-token"); err != nil {
		t.Fatal(err)
	}

	store := NewStore(backend)
	token, err := store.Token(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if token != "legacy-token" {
		t.Fatalf("Token() = %q, want legacy-token", token)
	}
}
