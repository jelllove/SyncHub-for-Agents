package main

import (
	"encoding/json"
	"fmt"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/sharedlibrary"
	"github.com/spf13/cobra"
)

func libraryCommand(options *commandOptions) *cobra.Command {
	root := &cobra.Command{Use: "library", Short: "Preview and manage shared skills and instructions", Args: noArgs}
	root.AddCommand(&cobra.Command{
		Use: "status", Short: "List canonical resources, import candidates and target drift", Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return options.owned(cmd, func(home string) (any, string, error) {
				snapshot, err := cli.LibrarySnapshot(home)
				text, marshalErr := json.MarshalIndent(snapshot, "", "  ")
				if err != nil {
					return nil, "", err
				}
				return snapshot, string(text), marshalErr
			})
		},
	})
	for _, action := range []string{"import", "apply", "restore", "detach"} {
		request := sharedlibrary.Request{Action: action}
		var approval string
		cmd := &cobra.Command{
			Use: action, Short: "Preview " + action + "; execute only with --approve <preview-id>", Args: noArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return options.owned(cmd, func(home string) (any, string, error) {
					if approval != "" {
						if err := cli.ApplyLibrary(home, request, approval); err != nil {
							return nil, "", err
						}
						return map[string]string{"action": request.Action, "status": "applied"}, "Library operation applied.", nil
					}
					plan, err := cli.PreviewLibrary(home, request)
					if err != nil {
						return nil, "", err
					}
					text, err := json.MarshalIndent(plan, "", "  ")
					if err != nil {
						return nil, "", err
					}
					return plan, fmt.Sprintf("%s\nReview changes, then repeat with --approve %s", text, plan.ID), nil
				})
			},
		}
		cmd.Flags().StringVar(&request.Kind, "kind", "", "skills or instructions")
		cmd.Flags().StringVar(&request.Name, "name", "", "Skill directory name, or global for instructions")
		cmd.Flags().StringVar(&request.Harness, "harness", "", "claude or common; copilot is import-only")
		cmd.Flags().StringVar(&approval, "approve", "", "Exact preview ID authorizing these changes")
		for _, flag := range []string{"kind", "name", "harness"} {
			_ = cmd.MarkFlagRequired(flag)
		}
		root.AddCommand(cmd)
	}
	return root
}
