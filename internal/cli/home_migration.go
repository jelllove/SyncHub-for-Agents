package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	currentHomeDirName = ".synchub"
	legacyHomeDirName  = ".acsync"
)

func ensureHome(userHome string) (string, error) {
	if userHome == "" {
		return "", fmt.Errorf("resolve SyncHub home: user home is empty")
	}
	current := filepath.Join(userHome, currentHomeDirName)
	if info, err := os.Stat(current); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("resolve SyncHub home: %s is not a directory", current)
		}
		return current, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	legacy := filepath.Join(userHome, legacyHomeDirName)
	if info, err := os.Stat(legacy); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("migrate legacy home: %s is not a directory", legacy)
		}
		if err := migrateLegacyHome(legacy, current); err != nil {
			return "", err
		}
		return current, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	return current, nil
}

func migrateLegacyHome(legacyHome, currentHome string) error {
	if err := os.Rename(legacyHome, currentHome); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		tempHome := currentHome + ".migrating"
		if removeErr := os.RemoveAll(tempHome); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		if copyErr := copyDirectory(legacyHome, tempHome); copyErr != nil {
			_ = os.RemoveAll(tempHome)
			return copyErr
		}
		if renameErr := os.Rename(tempHome, currentHome); renameErr != nil {
			_ = os.RemoveAll(tempHome)
			return renameErr
		}
		return nil
	}
	return nil
}

func copyDirectory(sourceDir, targetDir string) error {
	info, err := os.Stat(sourceDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("copy directory: %s is not a directory", sourceDir)
	}
	if err := os.MkdirAll(targetDir, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(sourceDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())
		if entry.IsDir() {
			if err := copyDirectory(sourcePath, targetPath); err != nil {
				return err
			}
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(sourcePath)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, targetPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return err
	}

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, source)
	return err
}

