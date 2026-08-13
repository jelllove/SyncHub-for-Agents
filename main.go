package main

import (
	"log"
	"net/http"
	"os"

	"github.com/qinqingxu/acsync/internal/auth"
	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/desktop"
	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/qinqingxu/acsync/internal/onboarding"
	"github.com/qinqingxu/acsync/internal/repository"
	"github.com/qinqingxu/acsync/internal/sshprobe"
)

var githubOAuthClientID string

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--git-credential" {
		if err := cli.RunCredential(os.Args[2], os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		return
	}
	hidden := len(os.Args) == 2 && os.Args[1] == "--hidden"
	home, err := defaultDesktopHome()
	if err != nil {
		log.Printf("resolve AgentConfigSync home: %v", err)
		os.Exit(1)
	}
	core, err := desktop.New(home, "")
	if err != nil {
		log.Printf("initialize AgentConfigSync: %v", err)
		os.Exit(1)
	}
	onboardingService, err := newOnboardingService(home, core)
	if err != nil {
		log.Printf("initialize onboarding: %v", err)
		os.Exit(1)
	}
	gui, err := newGUIApplication(core, onboardingService, hidden)
	if err != nil {
		log.Printf("initialize desktop application: %v", err)
		os.Exit(1)
	}
	if err := gui.run(); err != nil {
		log.Printf("run AgentConfigSync: %v", err)
		os.Exit(1)
	}
}

func defaultDesktopHome() (string, error) {
	return cli.Home()
}

func newOnboardingService(home string, core *desktop.Service) (*onboarding.Service, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	snapshot, err := core.Snapshot()
	if err != nil {
		return nil, err
	}
	agents := make([]onboarding.Agent, 0, len(snapshot.Agents))
	for _, agent := range snapshot.Agents {
		agents = append(agents, onboarding.Agent{
			Name:    agent.Name,
			Enabled: agent.Enabled,
			Exclude: agent.Exclude,
		})
	}
	clientID := githubOAuthClientID
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
