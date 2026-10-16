package cli

import (
	"errors"
	"os"

	"github.com/qinqingxu/synchub-for-agents/internal/sharedlibrary"
)

func withLibrary(home string, run func(sharedlibrary.Manager) error) (err error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	lock, err := sharedlibrary.Lock(userHome)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	return run(sharedlibrary.Manager{Home: home, UserHome: userHome})
}

func LibrarySnapshot(home string) (result sharedlibrary.Snapshot, err error) {
	err = withLibrary(home, func(manager sharedlibrary.Manager) error {
		result, err = manager.Snapshot()
		return err
	})
	return result, err
}

func PreviewLibrary(home string, request sharedlibrary.Request) (result sharedlibrary.Plan, err error) {
	err = withLibrary(home, func(manager sharedlibrary.Manager) error {
		result, err = manager.Preview(request)
		return err
	})
	return result, err
}

func ApplyLibrary(home string, request sharedlibrary.Request, id string) error {
	return withLibrary(home, func(manager sharedlibrary.Manager) error {
		return manager.Apply(request, id)
	})
}
