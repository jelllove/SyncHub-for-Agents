package onboarding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/auth"
)

type fakeOAuth struct {
	cancelled bool
}

func (oauth *fakeOAuth) Start(context.Context) (auth.DeviceStart, error) {
	return auth.DeviceStart{
		UserCode:        "ABCD-EFGH",
		VerificationURI: "https://github.com/login/device",
		ExpiresIn:       time.Minute,
	}, nil
}

func (oauth *fakeOAuth) Wait(context.Context) (auth.Account, string, error) {
	return auth.Account{ID: 42, Login: "alice", Scopes: []string{"repo"}}, "gho_secret", nil
}

func (oauth *fakeOAuth) Cancel() {
	oauth.cancelled = true
}

type fakeCredentials struct {
	saved bool
}

func (credentials *fakeCredentials) Save(auth.Account, string) error {
	credentials.saved = true
	return nil
}

func (credentials *fakeCredentials) Delete(int64) error {
	credentials.saved = false
	return nil
}

func TestOAuthOnboardingInitializesBeforeSavingConfiguration(t *testing.T) {
	oauth := &fakeOAuth{}
	credentials := &fakeCredentials{}
	setupCalls := 0
	configCalls := 0
	service := New(Dependencies{
		Home:        t.TempDir(),
		OAuth:       oauth,
		Credentials: credentials,
		SaveMetadata: func(auth.Metadata) error {
			return nil
		},
		SetupRepository: func(string, string, string) error {
			setupCalls++
			return nil
		},
		SaveConfig: func(string, map[string]bool) error {
			if setupCalls == 0 {
				t.Fatal("configuration saved before repository setup")
			}
			configCalls++
			return nil
		},
		Agents: []Agent{{Name: "claude", Enabled: true}},
	})

	if got := service.State().Step; got != Welcome {
		t.Fatalf("initial step = %q", got)
	}
	if err := service.SetRepository("https://github.com/acme/sync.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartGitHubLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := service.WaitGitHubLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Step != Agents || !credentials.saved {
		t.Fatalf("state = %#v, credentials saved = %v", state, credentials.saved)
	}
	if err := service.Complete(context.Background(), map[string]bool{"claude": true}); err != nil {
		t.Fatal(err)
	}
	if configCalls != 1 || service.State().Step != Ready {
		t.Fatalf("config calls = %d, state = %#v", configCalls, service.State())
	}
}

func TestFailedRepositorySetupDoesNotSaveConfiguration(t *testing.T) {
	configCalls := 0
	service := New(Dependencies{
		Home:        t.TempDir(),
		OAuth:       &fakeOAuth{},
		Credentials: &fakeCredentials{},
		SaveMetadata: func(auth.Metadata) error {
			return nil
		},
		SetupRepository: func(string, string, string) error {
			return errors.New("access denied")
		},
		SaveConfig: func(string, map[string]bool) error {
			configCalls++
			return nil
		},
	})
	if err := service.SetRepository("https://github.com/acme/sync.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartGitHubLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.WaitGitHubLogin(context.Background()); err == nil {
		t.Fatal("repository failure unexpectedly succeeded")
	}
	if configCalls != 0 {
		t.Fatalf("configuration saved %d times", configCalls)
	}
}

func TestCancelClearsOAuthFlow(t *testing.T) {
	oauth := &fakeOAuth{}
	service := New(Dependencies{OAuth: oauth})
	service.Cancel()
	if !oauth.cancelled {
		t.Fatal("OAuth flow was not cancelled")
	}
}

type blockingOAuth struct {
	started chan struct{}
}

func (oauth *blockingOAuth) Start(context.Context) (auth.DeviceStart, error) {
	return auth.DeviceStart{
		UserCode:        "ABCD",
		VerificationURI: "https://github.com/login/device",
		ExpiresIn:       time.Minute,
	}, nil
}

func (oauth *blockingOAuth) Wait(ctx context.Context) (auth.Account, string, error) {
	close(oauth.started)
	<-ctx.Done()
	return auth.Account{}, "", ctx.Err()
}

func (oauth *blockingOAuth) Cancel() {}

func TestCancelStopsActiveOAuthPolling(t *testing.T) {
	oauth := &blockingOAuth{started: make(chan struct{})}
	service := New(Dependencies{
		OAuth:       oauth,
		Credentials: &fakeCredentials{},
		SaveMetadata: func(auth.Metadata) error {
			return nil
		},
	})
	if err := service.SetRepository("https://github.com/acme/sync.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartGitHubLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := service.WaitGitHubLogin(context.Background())
		done <- err
	}()
	<-oauth.started
	service.Cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitGitHubLogin() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OAuth polling did not stop after cancellation")
	}
}
