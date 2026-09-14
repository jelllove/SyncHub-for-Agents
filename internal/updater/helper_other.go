//go:build !windows

package updater

import (
	"errors"
	"os"
)

func LaunchInstaller(pending pendingUpdate, executable string, restart bool, resultPath string) error {
	return errors.New("automatic installation is supported only on Windows amd64")
}

func runPlatformHelper(options helperOptions) error {
	return errors.New("automatic installation is supported only on Windows amd64")
}

func replaceHelperFile(source, destination string) error {
	return os.Rename(source, destination)
}

func helperStatusPending(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
