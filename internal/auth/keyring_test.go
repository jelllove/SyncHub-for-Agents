package auth

import "testing"

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
