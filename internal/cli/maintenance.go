package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/maintenance"
)

// maintenanceCmds are offline data-directory commands: they never contact the
// daemon and refuse to run while a daemon holds the instance lock.
func (a *App) maintenanceCmds() []*cobra.Command {
	return []*cobra.Command{a.backupCmd(), a.restoreCmd(), a.cleanupCmd()}
}

func (a *App) backupCmd() *cobra.Command {
	return &cobra.Command{Use: "backup PATH", Short: "Cold snapshot of the stopped data directory", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			m, err := maintenance.Backup(cmd.Context(), maintenance.Options{DataDir: cfg.DataDir, ConfigPath: a.configPath(), Version: Version}, args[0])
			if err != nil {
				return maintenanceErr(err)
			}
			if a.g.json {
				return a.printJSON(m)
			}
			fmt.Fprintf(a.out(), "%s %s\n", a.style.good("backup:"), args[0])
			printKV(a.out(), a.style, []kv{{"schema", fmt.Sprint(m.SchemaVersion)}, {"version", m.Version},
				{"files", fmt.Sprint(len(m.Files))}, {"token", yesNo(m.TokenIncluded)}})
			return nil
		}}
}

func (a *App) restoreCmd() *cobra.Command {
	var to string
	c := &cobra.Command{Use: "restore PATH", Short: "Restore a backup into a new data directory", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			target := to
			if target == "" {
				target = cfg.DataDir
			}
			report, err := maintenance.Restore(cmd.Context(), maintenance.RestoreOptions{Backup: args[0], Target: target})
			if err != nil {
				return maintenanceErr(err)
			}
			if a.g.json {
				return a.printJSON(report)
			}
			fmt.Fprintf(a.out(), "%s %s\n", a.style.good("restored:"), report.Target)
			printKV(a.out(), a.style, []kv{{"schema", fmt.Sprint(report.SchemaVersion)}, {"version", report.Manifest.Version},
				{"token", yesNo(report.Manifest.TokenIncluded)}, {"config", yesNo(report.ConfigWritten)}})
			if report.ConfigWritten {
				fmt.Fprintf(a.errw(), "%s point --config at %s\n", a.style.dim("hint:"), report.Target+"/config.json")
			}
			return nil
		}}
	c.Flags().StringVar(&to, "to", "", "new or empty data directory (default: the configured data_dir)")
	return c
}

func (a *App) cleanupCmd() *cobra.Command {
	var apply, dry bool
	var olderThan time.Duration
	c := &cobra.Command{Use: "cleanup", Short: "List or delete abandoned temp files (dry-run by default)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if apply && dry {
				return usage("choose either --dry-run or --apply")
			}
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			report, err := maintenance.Cleanup(cmd.Context(), maintenance.CleanupOptions{DataDir: cfg.DataDir, OlderThan: olderThan, Apply: apply})
			if err != nil {
				return maintenanceErr(err)
			}
			if a.g.json {
				return a.printJSON(report)
			}
			mode := a.style.warn("dry-run")
			if apply {
				mode = a.style.good("apply")
			}
			fmt.Fprintf(a.out(), "%s %s\n", mode, report.DataDir)
			paths := report.Candidates
			if apply {
				paths = report.Removed
			}
			if len(paths) == 0 {
				fmt.Fprintf(a.out(), "No temp files older than %s.\n", report.OlderThan)
			}
			for _, p := range paths {
				fmt.Fprintf(a.out(), "  %s\n", Sanitize(p))
			}
			printKV(a.out(), a.style, []kv{{"files", fmt.Sprint(len(paths))}, {"bytes", fmt.Sprint(report.Bytes)},
				{"skipped", fmt.Sprint(len(report.Skipped))}})
			if !apply && len(paths) > 0 {
				fmt.Fprintf(a.errw(), "%s re-run with --apply to delete these files\n", a.style.dim("hint:"))
			}
			return nil
		}}
	c.Flags().BoolVar(&apply, "apply", false, "delete the listed files")
	c.Flags().BoolVar(&dry, "dry-run", false, "only list candidates (default)")
	c.Flags().DurationVar(&olderThan, "older-than", maintenance.DefaultTempAge, "only touch files older than this")
	return c
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func (a *App) printJSON(v any) error {
	b, err := json.Marshal(map[string]any{"data": v, "request_id": ""})
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out(), string(b))
	return nil
}

// maintenanceErr maps the maintenance sentinels to the documented exit codes.
func maintenanceErr(err error) error {
	switch {
	case errors.Is(err, maintenance.ErrWriterRunning):
		return localErr(ExitUnavailable, "writer_running", err.Error())
	case errors.Is(err, maintenance.ErrTargetExists):
		return localErr(ExitConflict, "target_exists", err.Error())
	case errors.Is(err, maintenance.ErrManifest), errors.Is(err, maintenance.ErrUnsafePath):
		return localErr(ExitUsage, "invalid_backup", err.Error())
	case errors.Is(err, maintenance.ErrIntegrity):
		return localErr(ExitUsage, "backup_integrity", err.Error())
	case errors.Is(err, maintenance.ErrSchema):
		return localErr(ExitUsage, "unsupported_schema", err.Error())
	case errors.Is(err, maintenance.ErrSecret):
		return localErr(ExitUsage, "backup_secret", err.Error())
	}
	return localErr(ExitInternal, "maintenance_failed", strings.TrimSpace(err.Error()))
}
