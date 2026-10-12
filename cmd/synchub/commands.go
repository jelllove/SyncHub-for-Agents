package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qinqingxu/synchub-for-agents/internal/appversion"
	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
	"github.com/qinqingxu/synchub-for-agents/internal/gitclient"
	"github.com/qinqingxu/synchub-for-agents/internal/instance"
	"github.com/qinqingxu/synchub-for-agents/internal/repository"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
	"github.com/spf13/cobra"
)

type commandOptions struct {
	home string
	json bool
}

type commandError struct {
	Code     int
	Key      string
	Err      error
	reported bool
}

func (failure *commandError) Error() string { return failure.Err.Error() }
func (failure *commandError) Unwrap() error { return failure.Err }

func asCommandError(err error) *commandError {
	var failure *commandError
	if errors.As(err, &failure) {
		return failure
	}
	return &commandError{Code: 2, Key: "invalid_input", Err: err}
}

func runtimeFailure(err error) *commandError {
	if errors.Is(err, instance.ErrBusy) {
		return &commandError{Code: 3, Key: "busy", Err: err}
	}
	return &commandError{Code: 1, Key: "runtime_error", Err: err}
}

type resultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	SchemaVersion int          `json:"schemaVersion"`
	OK            bool         `json:"ok"`
	Data          any          `json:"data,omitempty"`
	Error         *resultError `json:"error,omitempty"`
}

func writeResult(cmd *cobra.Command, data any, text string, failure *commandError) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		response := envelope{SchemaVersion: 1, OK: failure == nil, Data: data}
		if failure != nil {
			response.Error = &resultError{Code: failure.Key, Message: failure.Error()}
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(response); err != nil {
			return runtimeFailure(fmt.Errorf("write JSON output: %w", err))
		}
	} else if data != nil {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), text); err != nil {
			return runtimeFailure(fmt.Errorf("write command output: %w", err))
		}
	}
	if failure != nil {
		if !jsonOutput {
			fmt.Fprintln(cmd.ErrOrStderr(), "error:", failure.Error())
		}
		failure.reported = true
		return failure
	}
	return nil
}

func reportCommandError(cmd *cobra.Command, failure *commandError) {
	if err := writeResult(cmd, nil, "", failure); err != nil && !failure.reported {
		fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
	}
}

func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return &commandError{Code: 2, Key: "invalid_input", Err: err}
	}
	return nil
}

func (options *commandOptions) owned(
	cmd *cobra.Command,
	run func(string) (any, string, error),
) error {
	home := options.home
	var err error
	if home == "" {
		home, err = cli.Home()
	} else {
		home, err = filepath.Abs(home)
	}
	if err != nil {
		return writeResult(cmd, nil, "", runtimeFailure(err))
	}
	lock, err := instance.Acquire(home)
	if err != nil {
		return writeResult(cmd, nil, "", runtimeFailure(err))
	}
	data, text, runErr := run(home)
	closeErr := lock.Close()
	if closeErr != nil {
		runErr = errors.Join(runErr, closeErr)
	}
	if runErr != nil {
		var failure *commandError
		if !errors.As(runErr, &failure) || closeErr != nil {
			failure = runtimeFailure(runErr)
		}
		return writeResult(cmd, data, text, failure)
	}
	return writeResult(cmd, data, text, nil)
}

func attention(message string) error {
	return &commandError{Code: 4, Key: "needs_attention", Err: errors.New(message)}
}

func newRootCommand() *cobra.Command {
	options := &commandOptions{}
	root := &cobra.Command{
		Use: "synchub", Short: "Sync portable AI agent resources without a GUI",
		Version: appversion.Version, SilenceUsage: true, SilenceErrors: true,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.PersistentFlags().StringVar(&options.home, "home", "", "SyncHub profile directory (default: ~/.synchub)")
	root.PersistentFlags().BoolVar(&options.json, "json", false, "Emit schemaVersion=1 JSON for finite commands")
	root.AddCommand(
		initCommand(options), statusCommand(options), planCommand(options),
		doctorCommand(options), syncCommand(options), daemonCommand(options),
		&cobra.Command{
			Use: "version", Short: "Print the CLI version", Args: noArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return writeResult(cmd, struct {
					Version string `json:"version"`
				}{appversion.Version}, appversion.Version, nil)
			},
		},
		&cobra.Command{
			Use: "git-credential [get|store|erase]", Hidden: true, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return cli.RunCredential(args[0], cmd.InOrStdin(), cmd.OutOrStdout())
			},
		},
	)
	addLegacyCommands(root, options)
	return root
}

func initCommand(options *commandOptions) *cobra.Command {
	var repo, strategy string
	var agents []string
	cmd := &cobra.Command{
		Use: "init", Short: "Bind a new profile using existing Git/SSH authentication", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strategy != config.FirstSyncStrategyMerge && strategy != config.FirstSyncStrategyUseCloud && strategy != config.FirstSyncStrategyUseLocal {
				return writeResult(cmd, nil, "", &commandError{Code: 2, Key: "invalid_input",
					Err: fmt.Errorf("--first-sync must be use-cloud, merge-cloud-local or use-local")})
			}
			if _, err := repository.ParseGitHubURL(repo); err != nil {
				return writeResult(cmd, nil, "", &commandError{Code: 2, Key: "invalid_input", Err: err})
			}
			return options.owned(cmd, func(home string) (any, string, error) {
				setup := &repository.Setup{Client: &gitclient.Client{}}
				if err := cli.RunInitWithOptions(home, repo, cli.InitOptions{FirstSync: strategy, Agents: agents}, setup); err != nil {
					return nil, "", err
				}
				data := struct {
					Home      string `json:"home"`
					FirstSync string `json:"firstSyncStrategy"`
				}{home, strategy}
				return data, "Initialized SyncHub; run plan before the first sync.", nil
			})
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Private GitHub repository URL")
	cmd.Flags().StringVar(&strategy, "first-sync", "", "use-cloud, merge-cloud-local or use-local (explicit choice required)")
	cmd.Flags().StringSliceVar(&agents, "agents", nil, "Enable only these agent names (default: all known agents)")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("first-sync")
	return cmd
}

func statusCommand(options *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use: "status", Short: "Inspect local synchronization status (no fetch)", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return options.owned(cmd, func(home string) (any, string, error) {
				status, err := cli.RunStatus(home, runtime.GOOS)
				text := fmt.Sprintf("Repo: %s\nEnabled agents: %v\nLast state update: %s\nPending actions: %d",
					status.RepoURL, status.EnabledAgents, status.LastSync.Format("2006-01-02T15:04:05Z07:00"), status.PendingActions)
				if err != nil {
					return nil, "", err
				}
				return status, text, nil
			})
		},
	}
}

type planAction struct {
	Type string `json:"type"`
	Path string `json:"repositoryPath"`
}

func planCommand(options *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use: "plan", Short: "Preview guarded actions against the local clone; does not fetch or apply", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return options.owned(cmd, func(home string) (any, string, error) {
				inspection, err := cli.Inspect(home, runtime.GOOS)
				if err != nil {
					return nil, "", err
				}
				actions := make([]planAction, 0, len(inspection.Actions))
				var text strings.Builder
				text.WriteString("Local-only preview; remote changes are not fetched.\n")
				for _, action := range inspection.Actions {
					actions = append(actions, planAction{action.Type.String(), action.RepoRel})
					fmt.Fprintf(&text, "%s %s\n", action.Type, action.RepoRel)
				}
				data := struct {
					LocalOnly      bool         `json:"localOnly"`
					Actions        []planAction `json:"actions"`
					Blocked        int          `json:"blocked"`
					Skipped        int          `json:"skipped"`
					FirstSync      string       `json:"firstSyncStrategy"`
					RequiresChoice bool         `json:"requiresFirstSyncChoice"`
				}{true, actions, len(inspection.Blocked), inspection.Skipped, inspection.FirstSync.Strategy, inspection.RequiresChoice}
				if inspection.RequiresChoice {
					return data, text.String(), attention("select the first-sync strategy in Desktop before synchronizing")
				}
				if len(inspection.Blocked) != 0 {
					return data, text.String(), attention("preview contains blocked resources")
				}
				return data, text.String(), nil
			})
		},
	}
}

func doctorCommand(options *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use: "doctor", Short: "Check local prerequisites without network access or repair", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return options.owned(cmd, func(home string) (any, string, error) {
				checks := cli.Doctor(home, runtime.GOOS)
				var text strings.Builder
				failed := false
				for _, check := range checks {
					fmt.Fprintf(&text, "%s: ok=%t %s\n", check.Name, check.OK, check.Message)
					failed = failed || !check.OK
				}
				data := struct {
					Checks []cli.DoctorCheck `json:"checks"`
				}{checks}
				if failed {
					return data, text.String(), attention("local prerequisite checks failed")
				}
				return data, text.String(), nil
			})
		},
	}
}

func syncCommand(options *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use: "sync", Short: "Run one sync; unresolved conflicts and approvals require attention", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return options.owned(cmd, func(home string) (any, string, error) {
				result, err := cli.RunSync(home, runtime.GOOS)
				if err != nil {
					if errors.Is(err, cli.ErrFirstSyncChoiceRequired) {
						return nil, "", attention(err.Error())
					}
					return nil, "", err
				}
				data := summarizeSync(result)
				text := fmt.Sprintf("Sync: %d actions, %d blocked, %d conflicts, %d pending installs, pushed=%t",
					data.Actions, data.Blocked, data.Conflicts, data.PendingInstalls, data.Pushed)
				if data.NeedsAttention {
					return data, text, attention("review blocked resources, conflicts or pending installations in Desktop")
				}
				return data, text, nil
			})
		},
	}
}

type syncSummary struct {
	Actions         int  `json:"actions"`
	Blocked         int  `json:"blocked"`
	Conflicts       int  `json:"conflicts"`
	PendingInstalls int  `json:"pendingInstalls"`
	Skipped         int  `json:"skipped"`
	Pushed          bool `json:"pushed"`
	NeedsAttention  bool `json:"needsAttention"`
}

func summarizeSync(result syncengine.Result) syncSummary {
	return syncSummary{
		Actions: len(result.Actions), Blocked: len(result.Blocked), Conflicts: result.Conflicts,
		PendingInstalls: result.PendingInstalls, Skipped: result.Skipped, Pushed: result.Pushed,
		NeedsAttention: result.NeedsAttention || len(result.Blocked) != 0 || result.Conflicts != 0 || result.PendingInstalls != 0,
	}
}

func daemonCommand(options *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use: "daemon", Short: "Run the headless scheduler until interrupted; logs stay in the profile", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.json {
				return writeResult(cmd, nil, "", &commandError{Code: 2, Key: "invalid_input", Err: errors.New("--json is supported only for finite commands")})
			}
			return options.owned(cmd, func(home string) (any, string, error) {
				cfg, err := config.Load(cli.ConfigPath(home))
				if err != nil {
					return nil, "", err
				}
				if !cfg.FirstSync.Completed && cfg.FirstSync.Strategy == config.FirstSyncStrategyChoose {
					return nil, "", attention("select the first-sync strategy in Desktop before running the daemon")
				}
				d, err := daemon.New(home, runtime.GOOS)
				if err != nil {
					return nil, "", err
				}
				return nil, "", d.Run(cmd.Context())
			})
		},
	}
}
