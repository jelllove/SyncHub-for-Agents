package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/qinqingxu/synchub-for-agents/internal/auth"
	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/desktop"
	"github.com/qinqingxu/synchub-for-agents/internal/gitclient"
	"github.com/qinqingxu/synchub-for-agents/internal/onboarding"
	"github.com/qinqingxu/synchub-for-agents/internal/repository"
	"github.com/qinqingxu/synchub-for-agents/internal/sshprobe"
)

var githubOAuthClientID string

type desktopBootstrap struct {
	newGUI        func() *guiApplication
	home          func() (string, error)
	newService    func(string, string) (*desktop.Service, error)
	newOnboarding func(string, *desktop.Service) (*onboarding.Service, error)
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--git-credential" {
		if err := cli.RunCredential(os.Args[2], os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		return
	}
	hidden := len(os.Args) == 2 && os.Args[1] == "--hidden"
	err := runDesktop(hidden, desktopBootstrap{
		newGUI:        newGUIApplication,
		home:          defaultDesktopHome,
		newService:    desktop.New,
		newOnboarding: newOnboardingService,
	})
	if err != nil {
		log.Printf("run SyncHub for Agents: %v", err)
		os.Exit(1)
	}
}

func runDesktop(hidden bool, bootstrap desktopBootstrap) error {
	gui := bootstrap.newGUI()
	home, err := bootstrap.home()
	if err != nil {
		return fmt.Errorf("resolve SyncHub for Agents home: %w", err)
	}
	core, err := bootstrap.newService(home, "")
	if err != nil {
		return fmt.Errorf("initialize SyncHub for Agents: %w", err)
	}
	onboardingService, err := bootstrap.newOnboarding(home, core)
	if err != nil {
		return fmt.Errorf("initialize onboarding: %w", err)
	}
	if err := gui.configure(core, onboardingService, hidden); err != nil {
		return fmt.Errorf("initialize desktop application: %w", err)
	}
	return gui.run()
}

func defaultDesktopHome() (string, error) {
	return cli.Home()
}

func newOnboardingService(home string, core *desktop.Service) (*onboarding.Service, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	desktopAgents, err := core.OnboardingAgents()
	if err != nil {
		return nil, err
	}
	agents := make([]onboarding.Agent, 0, len(desktopAgents))
	for _, agent := range desktopAgents {
		agents = append(agents, onboarding.Agent{
			Name:    agent.Name,
			Enabled: agent.Enabled,
			Exclude: agent.Exclude,
		})
	}
	clientID := githubOAuthClientID
	if clientID == "" {
		clientID = os.Getenv("SYNCHUB_GITHUB_CLIENT_ID")
	}
	if clientID == "" {
		clientID = os.Getenv("ACSYNC_GITHUB_CLIENT_ID")
	}
	store := auth.NewSystemStore()
	oauthClient := auth.NewOAuthClient(clientID, http.DefaultClient)
	return onboarding.New(onboarding.Dependencies{
		Home:        home,
		OAuth:       oauthClient,
		Credentials: store,
		SaveMetadata: func(metadata auth.Metadata) error {
			return auth.SaveMetadata(cli.AuthMetadataPath(home), metadata)
		},
		SSHRunner: sshprobe.SystemRunner{},
		SetupRepository: func(remote, dir, mode string) error {
			client := &gitclient.Client{Dir: dir}
			if mode == string(repository.HTTPS) {
				client.AuthMode = gitclient.AuthOAuth
				client.Executable = executable
			}
			setup := &repository.Setup{Client: client}
			return setup.Initialize(remote, dir)
		},
		SaveConfig: func(repositoryURL string, enabled map[string]bool) error {
			return core.SaveSettings(desktop.SettingsInput{
				RepositoryURL:   repositoryURL,
				IntervalMinutes: 10,
				TrashGraceDays:  30,
				Agents:          enabled,
			})
		},
		Agents: agents,
	}), nil
}
