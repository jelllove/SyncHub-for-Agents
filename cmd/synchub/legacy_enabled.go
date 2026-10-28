//go:build legacytray

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	legacytray "github.com/qinqingxu/synchub-for-agents/internal/tray/legacy"
	"github.com/spf13/cobra"
)

func addLegacyCommands(root *cobra.Command, options *commandOptions) {
	root.AddCommand(legacyTrayCommand(options, legacytray.Run))
	for _, name := range []string{"install", "uninstall"} {
		root.AddCommand(&cobra.Command{
			Use: name, Short: "Configure default-profile legacy tray autostart", Args: noArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				if options.home != "" {
					return writeResult(cmd, nil, "", &commandError{Code: 2, Key: "invalid_input",
						Err: errors.New("legacy autostart does not support --home; use Desktop start-at-login settings")})
				}
				return options.owned(cmd, func(_ string) (any, string, error) {
					userHome, err := os.UserHomeDir()
					if err != nil {
						return nil, "", err
					}
					if cmd.Name() == "uninstall" {
						err = cli.RunUninstall(runtime.GOOS, userHome)
					} else {
						executable, executableErr := os.Executable()
						if executableErr != nil {
							return nil, "", executableErr
						}
						_, err = cli.RunInstall(runtime.GOOS, userHome, executable)
					}
					if err != nil {
						return nil, "", err
					}
					result := struct {
						Enabled bool `json:"enabled"`
					}{cmd.Name() == "install"}
					return result, fmt.Sprintf("Legacy autostart enabled=%t", result.Enabled), nil
				})
			},
		})
	}
}

func legacyTrayCommand(options *commandOptions, run func(string, string) error) *cobra.Command {
	return &cobra.Command{
		Use: "tray", Short: "Run the legacy native tray", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.json {
				return writeResult(cmd, nil, "", &commandError{Code: 2, Key: "invalid_input",
					Err: errors.New("--json is supported only for finite commands")})
			}
			return options.owned(cmd, func(home string) (any, string, error) {
				return nil, "", run(home, runtime.GOOS)
			})
		},
	}
}
