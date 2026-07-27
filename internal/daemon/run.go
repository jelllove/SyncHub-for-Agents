package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// RunWithSignals builds a daemon for home and runs it until SIGINT/SIGTERM.
func RunWithSignals(home, goos string) error {
	d, err := New(home, goos)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return d.Run(ctx)
}
