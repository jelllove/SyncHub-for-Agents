package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/daemon"
	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/qinqingxu/acsync/internal/repository"
	"github.com/qinqingxu/acsync/internal/tray"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "acsync",
		Short: "Sync AI agent config and session files across machines via a private GitHub repo",
	}
	root.AddCommand(initCmd(), syncCmd(), statusCmd(), daemonCmd(), trayCmd(), installCmd(), uninstallCmd(), credentialCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func credentialCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "git-credential [get|store|erase]",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.RunCredential(args[0], os.Stdin, os.Stdout)
		},
	}
}

func initCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize acsync: bind a private repo, clone it, detect agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			setup := &repository.Setup{Client: &gitclient.Client{}}
			if err := cli.RunInit(home, repo, setup); err != nil {
				return err
			}
			fmt.Printf("Initialized acsync at %s (repo: %s)\n", home, repo)
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "private git repo URL (required)")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

func syncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Run one sync pass now",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			res, err := cli.RunSync(home, runtime.GOOS)
			if err != nil {
				return err
			}
			fmt.Printf("Sync complete: %d actions, %d blocked, pushed=%v\n",
				len(res.Actions), len(res.Blocked), res.Pushed)
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			st, err := cli.RunStatus(home, runtime.GOOS)
			if err != nil {
				return err
			}
			fmt.Printf("Repo:            %s\n", st.RepoURL)
			fmt.Printf("Enabled agents:  %v\n", st.EnabledAgents)
			if st.LastSync.IsZero() {
				fmt.Println("Last sync:       never")
			} else {
				fmt.Printf("Last sync:       %s\n", st.LastSync.Format(time.RFC3339))
			}
			fmt.Printf("Pending actions: %d\n", st.PendingActions)
			return nil
		},
	}
}

func daemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the sync daemon (periodic sync + trash cleanup) until stopped",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			return daemon.RunWithSignals(home, runtime.GOOS)
		},
	}
}

func trayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tray",
		Short: "Run acsync with a system-tray icon (daemon + UI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			return tray.Run(home, runtime.GOOS)
		},
	}
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Enable acsync to start automatically at login",
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			userHome, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			path, err := cli.RunInstall(runtime.GOOS, userHome, exe)
			if err != nil {
				return err
			}
			fmt.Printf("Autostart enabled: %s\n", path)
			return nil
		},
	}
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Disable acsync autostart",
		RunE: func(cmd *cobra.Command, args []string) error {
			userHome, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			if err := cli.RunUninstall(runtime.GOOS, userHome); err != nil {
				return err
			}
			fmt.Println("Autostart disabled")
			return nil
		},
	}
}
