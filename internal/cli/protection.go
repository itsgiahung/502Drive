package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

type protectionView struct {
	CollectionID string `json:"collection_id"`
	Revision     int    `json:"revision"`
	Options      struct {
		Mirror    bool `json:"mirror"`
		Snapshots bool `json:"snapshots"`
		KeepDays  int  `json:"keep_days"`
		KeepCount int  `json:"keep_count"`
	} `json:"options"`
}

func (a *App) protectionCmd() *cobra.Command {
	var mirror, snapshots, dry bool
	var days, count int
	c := &cobra.Command{Use: "options COLLECTION", Short: "Show or change opt-in mirror and snapshot options", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if days < 1 || days > 3650 || count < 1 || count > 1000 {
			return usage("--keep-days must be 1–3650 and --keep-count 1–1000")
		}
		path := "/v1/collections/" + url.PathEscape(args[0]) + "/protection"
		r, err := a.call(cmd.Context(), "GET", path, nil, nil, "")
		if err != nil {
			return err
		}
		var v protectionView
		if err = r.Decode(&v); err != nil {
			return err
		}
		changed := false
		for _, flag := range []string{"mirror", "snapshots", "keep-days", "keep-count"} {
			changed = changed || cmd.Flags().Changed(flag)
		}
		if cmd.Flags().Changed("mirror") {
			v.Options.Mirror = mirror
		}
		if cmd.Flags().Changed("snapshots") {
			v.Options.Snapshots = snapshots
		}
		if cmd.Flags().Changed("keep-days") {
			v.Options.KeepDays = days
		}
		if cmd.Flags().Changed("keep-count") {
			v.Options.KeepCount = count
		}
		if changed && !dry {
			r, err = a.call(cmd.Context(), "PUT", path, nil, map[string]any{"expected_revision": v.Revision, "options": v.Options}, NewKey())
			if err != nil {
				return err
			}
			if err = r.Decode(&v); err != nil {
				return err
			}
		}
		if dry && a.g.json {
			b, _ := json.Marshal(map[string]any{"data": v, "request_id": ""})
			fmt.Fprintln(a.out(), string(b))
			return nil
		}
		a.emit(r, v.CollectionID, func() {
			fmt.Fprintf(a.out(), "mirror: %t\nsnapshots: %t\nkeep: %d days, %d unpinned snapshots\n", v.Options.Mirror, v.Options.Snapshots, v.Options.KeepDays, v.Options.KeepCount)
		})
		if dry {
			fmt.Fprintln(a.errw(), "dry run: options were not saved")
		}
		return nil
	}}
	c.Flags().BoolVar(&mirror, "mirror", false, "mirror source removals (owned copies only; never permanent delete)")
	c.Flags().BoolVar(&snapshots, "snapshots", false, "capture a restore point after each successful run")
	c.Flags().IntVar(&days, "keep-days", 30, "snapshot retention in days; pinned snapshots are kept")
	c.Flags().IntVar(&count, "keep-count", 10, "maximum unpinned snapshots per collection")
	c.Flags().BoolVar(&dry, "dry-run", false, "preview options without saving")
	return c
}

func (a *App) snapshotsCmd() *cobra.Command {
	root := &cobra.Command{Use: "snapshots", Short: "Capture, inspect and restore collection restore points"}
	var listCursor, planCursor string
	list := &cobra.Command{Use: "list COLLECTION", Short: "List snapshots, newest first", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/collections/"+url.PathEscape(args[0])+"/snapshots", url.Values{"limit": {"100"}, "cursor": {listCursor}}, nil, "")
		if err != nil {
			return err
		}
		var page struct {
			Next  *string `json:"next_cursor"`
			Items []struct {
				ID, State string
				Pinned    bool
				CreatedAt string `json:"created_at"`
			} `json:"items"`
		}
		if err = r.Decode(&page); err != nil {
			return err
		}
		a.emit(r, "", func() {
			for _, v := range page.Items {
				fmt.Fprintf(a.out(), "%s  %s  pinned=%t  %s\n", v.ID, v.State, v.Pinned, v.CreatedAt)
			}
		})
		if page.Next != nil && !a.g.json {
			fmt.Fprintf(a.errw(), "more snapshots: repeat with --cursor %s\n", *page.Next)
		}
		return nil
	}}
	list.Flags().StringVar(&listCursor, "cursor", "", "opaque cursor returned by the previous page")
	show := &cobra.Command{Use: "show SNAPSHOT", Short: "Show a restore point", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/snapshots/"+url.PathEscape(args[0]), nil, nil, "")
		if err != nil {
			return err
		}
		var v map[string]any
		if err = r.Decode(&v); err != nil {
			return err
		}
		a.emit(r, args[0], func() {
			fmt.Fprintf(a.out(), "%s  %v\ncollection: %v\nitems: %v\npinned: %v\nexpires: %v\n", Sanitize(args[0]), v["state"], v["collection_id"], v["items"], v["pinned"], v["expires_at"])
		})
		return nil
	}}
	for _, pin := range []bool{true, false} {
		pin := pin
		name := "unpin"
		if pin {
			name = "pin"
		}
		root.AddCommand(&cobra.Command{Use: name + " SNAPSHOT", Short: name + " a restore point", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			path := "/v1/snapshots/" + url.PathEscape(args[0])
			r, err := a.call(cmd.Context(), "GET", path, nil, nil, "")
			if err != nil {
				return err
			}
			var v struct {
				Revision int `json:"revision"`
			}
			if err = r.Decode(&v); err != nil {
				return err
			}
			method := "DELETE"
			if pin {
				method = "POST"
			}
			r, err = a.call(cmd.Context(), method, path+"/pin", nil, map[string]any{"expected_revision": v.Revision}, NewKey())
			if err != nil {
				return err
			}
			a.emit(r, args[0], func() { fmt.Fprintf(a.out(), "%s: pinned=%t\n", Sanitize(args[0]), pin) })
			return nil
		}})
	}
	plan := &cobra.Command{Use: "plan JOB", Short: "Show the durable restore action plan", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, err := a.call(cmd.Context(), "GET", "/v1/jobs/"+url.PathEscape(args[0])+"/restore-plan", url.Values{"limit": {"100"}, "cursor": {planCursor}}, nil, "")
		if err != nil {
			return err
		}
		var v struct {
			Next  *string `json:"next_cursor"`
			Items []struct {
				Action, State string
				Target        struct{ Name, Path string }
			} `json:"items"`
		}
		if err = r.Decode(&v); err != nil {
			return err
		}
		a.emit(r, args[0], func() {
			for _, it := range v.Items {
				fmt.Fprintf(a.out(), "%s  %s  %s\n", it.Action, it.State, Sanitize(it.Target.Path+" "+it.Target.Name))
			}
		})
		if v.Next != nil && !a.g.json {
			fmt.Fprintf(a.errw(), "more actions: repeat with --cursor %s\n", *v.Next)
		}
		return nil
	}}
	plan.Flags().StringVar(&planCursor, "cursor", "", "opaque cursor returned by the previous page")
	root.AddCommand(list, show, plan, a.snapshotJobCmd(false), a.snapshotJobCmd(true))
	return root
}

func (a *App) snapshotJobCmd(restore bool) *cobra.Command {
	var apply, dry, wait bool
	var timeout time.Duration
	use, short := "create COLLECTION", "Capture the current collection mapping and retained copies"
	if restore {
		use, short = "restore SNAPSHOT", "Preview a restore; --apply restores and pauses sync"
	}
	c := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if apply && dry {
			return usage("choose --apply or --dry-run")
		}
		path := "/v1/collections/" + url.PathEscape(args[0])
		suffix := "/snapshots"
		if restore {
			path = "/v1/snapshots/" + url.PathEscape(args[0])
			suffix = "/restore"
		}
		r, err := a.call(cmd.Context(), "GET", path, nil, nil, "")
		if err != nil {
			return err
		}
		var v struct {
			Revision int `json:"revision"`
		}
		if err = r.Decode(&v); err != nil {
			return err
		}
		body := map[string]any{"expected_revision": v.Revision}
		if restore {
			body["dry_run"] = !apply
		}
		r, err = a.call(cmd.Context(), "POST", path+suffix, nil, body, NewKey())
		if err != nil {
			return err
		}
		var j jobView
		if err = r.Decode(&j); err != nil {
			return err
		}
		if wait {
			return a.waitJob(cmd.Context(), j.ID, timeout)
		}
		a.emit(r, j.ID, func() { fmt.Fprintln(a.out(), j.ID) })
		return nil
	}}
	if restore {
		c.Flags().BoolVar(&apply, "apply", false, "apply the restore (pauses sync until you resume it)")
		c.Flags().BoolVar(&dry, "dry-run", false, "preview only (the default)")
	}
	c.Flags().BoolVar(&wait, "wait", true, "wait until the job finishes")
	c.Flags().DurationVar(&timeout, "wait-timeout", 10*time.Minute, "stop waiting after this duration; job keeps running")
	return c
}
