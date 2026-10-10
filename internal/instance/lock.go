// Package instance coordinates cooperating SyncHub processes that share a profile.
package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrBusy = errors.New("SyncHub profile is busy; quit the running Desktop or daemon before retrying")

type Lock struct {
	mu   sync.Mutex
	file *os.File
}

func Acquire(home string) (*Lock, error) {
	if strings.TrimSpace(home) == "" {
		return nil, fmt.Errorf("acquire profile ownership: empty home")
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("prepare profile lock directory: %w", err)
	}
	path := filepath.Join(home, ".instance.lock")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("profile lock is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect profile lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open profile lock: %w", err)
	}
	fail := func(cause error) (*Lock, error) {
		return nil, errors.Join(cause, file.Close())
	}
	actual, err := file.Stat()
	if err != nil {
		return fail(fmt.Errorf("inspect opened profile lock: %w", err))
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fail(fmt.Errorf("recheck profile lock: %w", err))
	}
	if !actual.Mode().IsRegular() || !info.Mode().IsRegular() || !os.SameFile(info, actual) {
		return fail(fmt.Errorf("profile lock changed while opening"))
	}
	if err := lockFile(file); err != nil {
		return fail(err)
	}
	return &Lock{file: file}, nil
}

func (lock *Lock) Close() error {
	lock.mu.Lock()
	defer lock.mu.Unlock()
	if lock.file == nil {
		return nil
	}
	err := errors.Join(unlockFile(lock.file), lock.file.Close())
	lock.file = nil
	return err
}
