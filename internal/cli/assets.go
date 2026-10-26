package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/tray"
)

// assetsCmd writes the files the packages need: icons and shell completions.
// It is hidden because users install assets with the package, not by hand.
func (a *App) assetsCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{Use: "assets", Hidden: true, Args: cobra.NoArgs,
		Short: "Write build assets (icons, completions) and exit",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dir == "" {
				return usage("--dir is required")
			}
			if err := a.writeBuildAssets(cmd.Context(), dir); err != nil {
				return err
			}
			fmt.Fprintf(a.out(), "Wrote assets in %s\n", dir)
			return nil
		}}
	c.Flags().StringVar(&dir, "dir", "", "output directory")
	return c
}

func (a *App) writeBuildAssets(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return localErr(ExitInternal, "assets_failed", "Cannot create the asset directory: "+err.Error())
	}
	if err := tray.WriteIcons(dir); err != nil {
		return localErr(ExitInternal, "assets_failed", "Cannot write icons: "+err.Error())
	}
	return a.writeCompletions(ctx, dir)
}

// writeCompletions runs the real `completion` subcommand into files, so the
// packaged completions always match the shipped command tree.
func (a *App) writeCompletions(ctx context.Context, dir string) error {
	for _, shell := range []struct{ name, file string }{
		{"bash", "502drive.bash"},
		{"zsh", "_502drive"},
		{"fish", "502drive.fish"},
		{"powershell", "502drive.ps1"},
	} {
		path := filepath.Join(dir, shell.file)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return localErr(ExitInternal, "assets_failed", "Cannot create "+path+": "+err.Error())
		}
		root := a.rootCommand()
		root.SetArgs([]string{"completion", shell.name})
		root.SetOut(f)
		root.SetErr(io.Discard)
		err = root.ExecuteContext(ctx)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return localErr(ExitInternal, "assets_failed", "Cannot generate the "+shell.name+" completion: "+err.Error())
		}
	}
	return nil
}
