// Command 502drive-tray is the GUI-subsystem entry point for the Windows tray.
// Windows releases build it with -H=windowsgui so no console window appears;
// on other platforms the command simply runs the same tray path and reports
// that the tray is unsupported.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/GiaHung07/502Drive/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(cli.Execute(ctx, []string{"tray"}, cli.IO{
		In:        os.Stdin,
		Out:       os.Stdout,
		Err:       os.Stderr,
		Getenv:    os.Getenv,
		StdinTTY:  false,
		StdoutTTY: false,
		Width:     80,
	}))
}
