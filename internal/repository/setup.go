package repository

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qinqingxu/synchub-for-agents/internal/gitclient"
)

type Setup struct {
	Client *gitclient.Client
	Name   string
	Email  string
}

func (s *Setup) Initialize(remote, dir string) error {
	if s.Client == nil {
		return errors.New("Git client is required")
	}
	if remote == "" || dir == "" {
		return errors.New("repository remote and directory are required")
	}
	empty, err := emptyOrMissing(dir)
	if err != nil {
		return err
	}
	if empty {
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := s.Client.Clone(remote, dir); err != nil {
			return fmt.Errorf("clone repository: %w", err)
		}
	}
	s.Client.Dir = dir
	origin, exists, err := s.Client.RemoteURL("origin")
	if err != nil {
		return err
	}
	if !exists || !sameRemote(origin, remote) {
		return errors.New("existing repository origin does not match selected remote")
	}

	remoteRefs, err := s.Client.LsRemote(remote)
	if err != nil {
		return err
	}
	hasHead, err := s.Client.HasHEAD()
	if err != nil {
		return err
	}
	if hasHead {
		if err := ensureAttributes(dir); err != nil {
			return err
		}
		if remoteRefs != "" {
			return nil
		}
		branch, err := s.Client.CurrentBranch()
		if err != nil {
			return err
		}
		if branch != "main" {
			return errors.New("incomplete repository bootstrap is not on main branch")
		}
		return s.Client.PushUpstream("origin", "main")
	}
	if err := ensureAttributes(dir); err != nil {
		return err
	}
	if err := s.Client.CheckoutBranch("main"); err != nil {
		return err
	}
	if err := s.ensureIdentity(); err != nil {
		return err
	}
	if err := s.Client.AddAll(); err != nil {
		return err
	}
	if err := s.Client.Commit("chore: initialize SyncHub repository"); err != nil {
		return err
	}
	if err := s.Client.PushUpstream("origin", "main"); err != nil {
		return err
	}
	return nil
}

func sameRemote(left, right string) bool {
	if filepath.IsAbs(left) && filepath.IsAbs(right) {
		leftPath, leftErr := filepath.Abs(left)
		rightPath, rightErr := filepath.Abs(right)
		return leftErr == nil && rightErr == nil && filepath.Clean(leftPath) == filepath.Clean(rightPath)
	}
	return left == right
}

func (s *Setup) ensureIdentity() error {
	name := s.Name
	if name == "" {
		name = "SyncHub"
	}
	email := s.Email
	if email == "" {
		email = "synchub@users.noreply.github.com"
	}
	if _, exists, err := s.Client.LocalConfig("user.name"); err != nil {
		return err
	} else if !exists {
		if err := s.Client.SetLocalConfig("user.name", name); err != nil {
			return err
		}
	}
	if _, exists, err := s.Client.LocalConfig("user.email"); err != nil {
		return err
	} else if !exists {
		if err := s.Client.SetLocalConfig("user.email", email); err != nil {
			return err
		}
	}
	return nil
}

func emptyOrMissing(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func ensureAttributes(dir string) error {
	path := filepath.Join(dir, ".gitattributes")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte("* -text\n"), 0o644)
}
