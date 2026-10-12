//go:build !legacytray

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func addLegacyCommands(root *cobra.Command, _ *commandOptions) {
	for _, name := range []string{"tray", "install", "uninstall"} {
		root.AddCommand(&cobra.Command{
			Use: name, Hidden: true, Args: noArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return writeResult(cmd, nil, "", &commandError{Code: 2, Key: "invalid_input",
					Err: fmt.Errorf("%s belongs to the optional legacytray build; use Desktop for tray and start-at-login settings", cmd.Name())})
			},
		})
	}
}
