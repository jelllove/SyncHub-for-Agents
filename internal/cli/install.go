package cli

import "github.com/qinqingxu/acsync/internal/autostart"

// RunInstall enables run-at-login for execPath and returns the entry path.
func RunInstall(goos, userHome, execPath string) (string, error) {
	m, err := autostart.New(goos, userHome)
	if err != nil {
		return "", err
	}
	if err := m.Enable(execPath); err != nil {
		return "", err
	}
	return m.Path(), nil
}

// RunUninstall removes the run-at-login entry.
func RunUninstall(goos, userHome string) error {
	m, err := autostart.New(goos, userHome)
	if err != nil {
		return err
	}
	return m.Disable()
}
