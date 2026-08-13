// Package onboarding orchestrates first-run repository and authentication setup.
package onboarding

import (
	"context"
	"errors"
	"path/filepath"
	"sync"

	"github.com/qinqingxu/acsync/internal/auth"
	"github.com/qinqingxu/acsync/internal/repository"
	"github.com/qinqingxu/acsync/internal/sshprobe"
)

type Step string

const (
	Welcome        Step = "welcome"
	Repository     Step = "repository"
	Authentication Step = "authentication"
	Verification   Step = "verification"
	Agents         Step = "agents"
	Ready          Step = "ready"
)

type Agent struct {
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Exclude []string `json:"exclude"`
}

type State struct {
	Step            Step    `json:"step"`
	RepositoryURL   string  `json:"repositoryUrl"`
	AuthMode        string  `json:"authMode"`
	UserCode        string  `json:"userCode,omitempty"`
	VerificationURI string  `json:"verificationUri,omitempty"`
	Message         string  `json:"message,omitempty"`
	Agents          []Agent `json:"agents,omitempty"`
}

type OAuth interface {
	Start(context.Context) (auth.DeviceStart, error)
	Wait(context.Context) (auth.Account, string, error)
	Cancel()
}

type CredentialStore interface {
	Save(auth.Account, string) error
	Delete(int64) error
}

type Dependencies struct {
	Home            string
	OAuth           OAuth
	Credentials     CredentialStore
	SaveMetadata    func(auth.Metadata) error
	SSHRunner       sshprobe.Runner
	SetupRepository func(remote, dir, authMode string) error
	SaveConfig      func(repositoryURL string, enabled map[string]bool) error
	Agents          []Agent
}

type Service struct {
	mu           sync.RWMutex
	dependencies Dependencies
	state        State
	loginCancel  context.CancelFunc
	loginCtx     context.Context
	account      auth.Account
}

func New(dependencies Dependencies) *Service {
	agents := append([]Agent(nil), dependencies.Agents...)
	return &Service{
		dependencies: dependencies,
		state: State{
			Step:   Welcome,
			Agents: agents,
		},
	}
}

func (s *Service) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

func (s *Service) SetRepository(raw string) error {
	parsed, err := repository.ParseGitHubURL(raw)
	if err != nil {
		return err
	}
	s.Cancel()
	s.mu.Lock()
	s.state.RepositoryURL = parsed.CloneURL
	s.state.AuthMode = string(parsed.Protocol)
	s.state.Step = Authentication
	s.state.Message = ""
	s.mu.Unlock()
	return nil
}

func (s *Service) StartGitHubLogin(ctx context.Context) (State, error) {
	s.mu.RLock()
	mode := s.state.AuthMode
	s.mu.RUnlock()
	if mode != string(repository.HTTPS) {
		return s.State(), errors.New("GitHub login is available only for HTTPS repositories")
	}
	if s.dependencies.OAuth == nil {
		return s.State(), errors.New("GitHub OAuth is not configured")
	}
	start, err := s.dependencies.OAuth.Start(ctx)
	if err != nil {
		return s.State(), err
	}
	loginCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.loginCancel != nil {
		s.loginCancel()
	}
	s.loginCancel = cancel
	s.loginCtx = loginCtx
	s.state.Step = Authentication
	s.state.UserCode = start.UserCode
	s.state.VerificationURI = start.VerificationURI
	s.state.Message = "Waiting for GitHub authorization"
	state := cloneState(s.state)
	s.mu.Unlock()
	return state, nil
}

func (s *Service) WaitGitHubLogin(ctx context.Context) (State, error) {
	if s.dependencies.OAuth == nil || s.dependencies.Credentials == nil ||
		s.dependencies.SaveMetadata == nil {
		return s.State(), errors.New("GitHub OAuth dependencies are not configured")
	}
	s.mu.RLock()
	flowCtx := s.loginCtx
	s.mu.RUnlock()
	if flowCtx == nil {
		return s.State(), errors.New("device authorization has not been started")
	}
	waitCtx, cancel := context.WithCancel(flowCtx)
	stop := context.AfterFunc(ctx, cancel)
	defer func() {
		stop()
		cancel()
	}()
	account, token, err := s.dependencies.OAuth.Wait(waitCtx)
	if err != nil {
		return s.State(), err
	}
	if err := s.dependencies.Credentials.Save(account, token); err != nil {
		return s.State(), err
	}
	if err := s.dependencies.SaveMetadata(auth.Metadata{Active: account}); err != nil {
		_ = s.dependencies.Credentials.Delete(account.ID)
		return s.State(), err
	}
	s.mu.Lock()
	s.account = account
	s.loginCancel = nil
	s.loginCtx = nil
	s.state.Step = Verification
	s.state.UserCode = ""
	s.state.VerificationURI = ""
	s.state.Message = "Verifying repository access"
	s.mu.Unlock()
	if err := s.initializeRepository(ctx); err != nil {
		return s.State(), err
	}
	s.mu.Lock()
	s.state.Step = Agents
	s.state.Message = "Repository access verified"
	state := cloneState(s.state)
	s.mu.Unlock()
	return state, nil
}

func (s *Service) VerifySSH(ctx context.Context) (State, error) {
	s.mu.RLock()
	mode := s.state.AuthMode
	remote := s.state.RepositoryURL
	s.mu.RUnlock()
	if mode != string(repository.SSH) {
		return s.State(), errors.New("SSH verification requires an SSH repository URL")
	}
	if s.dependencies.SSHRunner == nil {
		return s.State(), errors.New("SSH verification is not configured")
	}
	s.mu.Lock()
	s.state.Step = Verification
	s.state.Message = "Checking SSH keys and repository access"
	s.mu.Unlock()
	status, err := sshprobe.Check(ctx, s.dependencies.SSHRunner, remote)
	if err != nil {
		return s.State(), err
	}
	if !status.RepositoryAccess {
		s.mu.Lock()
		s.state.Message = status.Message
		state := cloneState(s.state)
		s.mu.Unlock()
		return state, errors.New(status.Message)
	}
	if err := s.initializeRepository(ctx); err != nil {
		return s.State(), err
	}
	s.mu.Lock()
	s.state.Step = Agents
	s.state.Message = status.Message
	state := cloneState(s.state)
	s.mu.Unlock()
	return state, nil
}

func (s *Service) Complete(ctx context.Context, enabled map[string]bool) error {
	s.mu.RLock()
	step := s.state.Step
	remote := s.state.RepositoryURL
	s.mu.RUnlock()
	if step != Agents {
		return errors.New("repository verification must complete before selecting agents")
	}
	if err := s.initializeRepository(ctx); err != nil {
		return err
	}
	if s.dependencies.SaveConfig == nil {
		return errors.New("configuration writer is not configured")
	}
	if err := s.dependencies.SaveConfig(remote, enabled); err != nil {
		return err
	}
	s.mu.Lock()
	for index := range s.state.Agents {
		s.state.Agents[index].Enabled = enabled[s.state.Agents[index].Name]
	}
	s.state.Step = Ready
	s.state.Message = "AgentConfigSync is ready"
	s.mu.Unlock()
	return nil
}

func (s *Service) Cancel() {
	s.mu.Lock()
	cancel := s.loginCancel
	s.loginCancel = nil
	s.loginCtx = nil
	s.state.UserCode = ""
	s.state.VerificationURI = ""
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if s.dependencies.OAuth != nil {
		s.dependencies.OAuth.Cancel()
	}
}

func (s *Service) initializeRepository(_ context.Context) error {
	if s.dependencies.SetupRepository == nil {
		return errors.New("repository setup is not configured")
	}
	s.mu.RLock()
	remote := s.state.RepositoryURL
	mode := s.state.AuthMode
	s.mu.RUnlock()
	return s.dependencies.SetupRepository(remote, filepath.Join(s.dependencies.Home, "repo"), mode)
}

func cloneState(state State) State {
	state.Agents = append([]Agent(nil), state.Agents...)
	return state
}
