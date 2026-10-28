//go:build linux || darwin

package instance

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrBusy
	}
	if err != nil {
		return fmt.Errorf("lock profile: %w", err)
	}
	return nil
}

func unlockFile(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlock profile: %w", err)
	}
	return nil
}
