// Package auth implements GitHub authentication without exposing credentials to
// the desktop frontend.
package auth

import (
	"errors"
	"strconv"

	keyring "github.com/zalando/go-keyring"
)

const keyringService = "io.github.qinqingxu.acsync/github-oauth"

var ErrNotFound = errors.New("credential not found")

type Backend interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type systemBackend struct{}

func (systemBackend) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (systemBackend) Get(service, user string) (string, error) {
	value, err := keyring.Get(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return value, err
}

func (systemBackend) Delete(service, user string) error {
	err := keyring.Delete(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

type Account struct {
	ID     int64    `json:"id"`
	Login  string   `json:"login"`
	Scopes []string `json:"scopes"`
}

type Store struct {
	backend Backend
}

func NewStore(backend Backend) *Store {
	return &Store{backend: backend}
}

func NewSystemStore() *Store {
	return NewStore(systemBackend{})
}

func accountKey(id int64) string {
	return strconv.FormatInt(id, 10)
}

func (s *Store) Save(account Account, token string) error {
	if account.ID == 0 || token == "" {
		return errors.New("account and token are required")
	}
	return s.backend.Set(keyringService, accountKey(account.ID), token)
}

func (s *Store) Token(id int64) (string, error) {
	if id == 0 {
		return "", errors.New("account ID is required")
	}
	return s.backend.Get(keyringService, accountKey(id))
}

func (s *Store) Delete(id int64) error {
	if id == 0 {
		return errors.New("account ID is required")
	}
	return s.backend.Delete(keyringService, accountKey(id))
}
