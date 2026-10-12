package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--git-credential" {
		if err := cli.RunCredential(os.Args[2], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "credential helper:", err)
			os.Exit(1)
		}
		return
	}
	root := newRootCommand()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := root.ExecuteContext(ctx); err != nil {
		failure := asCommandError(err)
		if !failure.reported {
			if flagErr := root.PersistentFlags().Set("json", strconv.FormatBool(requestedJSON(os.Args[1:]))); flagErr != nil {
				fmt.Fprintln(os.Stderr, "output mode:", flagErr)
				os.Exit(1)
			}
			reportCommandError(root, failure)
		}
		os.Exit(failure.Code)
	}
}

func requestedJSON(args []string) bool {
	enabled := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		switch arg {
		case "--home", "--repo", "--first-sync", "--agents":
			index++
		case "--json":
			enabled = true
		default:
			if value, ok := strings.CutPrefix(arg, "--json="); ok {
				if parsed, err := strconv.ParseBool(value); err == nil {
					enabled = parsed
				}
			}
		}
	}
	return enabled
}
