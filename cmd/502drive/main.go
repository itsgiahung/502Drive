package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/itsgiahung/502Drive/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	w := 80
	if tw, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 0 {
		w = tw
	}
	os.Exit(cli.Execute(ctx, os.Args[1:], cli.IO{
		In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv,
		StdinTTY: term.IsTerminal(int(os.Stdin.Fd())), StdoutTTY: term.IsTerminal(int(os.Stdout.Fd())), StderrTTY: term.IsTerminal(int(os.Stderr.Fd())),
		Width: w,
	}))
}
