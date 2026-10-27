package cli

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/itsgiahung/502Drive/internal/present"
)

// deepView is the deep scan block of a plan (S7); the daemon owns the scan.
type deepView struct {
	State        string   `json:"state"`
	Complete     bool     `json:"complete"`
	Folders      int      `json:"folders"`
	Files        int      `json:"files"`
	Bytes        int64    `json:"bytes"`
	Native       int      `json:"native_files"`
	Shortcuts    int      `json:"shortcuts"`
	CannotCopy   int      `json:"cannot_copy"`
	Unreadable   int      `json:"unreadable_folders"`
	Scanned      int      `json:"scanned"`
	EstimateSecs *int64   `json:"estimate_seconds"`
	Basis        string   `json:"estimate_basis"`
	Warnings     []string `json:"warnings"`
}

// waitDeepPlan polls the plan until its deep scan leaves "running". It only
// reads; interrupting it stops watching and leaves the scan running.
func (a *App) waitDeepPlan(ctx context.Context, id string) (planView, *Response, error) {
	for {
		r, err := a.call(ctx, "GET", "/v1/plans/"+url.PathEscape(id), nil, nil, "")
		if err != nil {
			return planView{}, nil, err
		}
		var plan planView
		if err = r.Decode(&plan); err != nil {
			return planView{}, nil, err
		}
		if plan.Preview.Deep == nil || plan.Preview.Deep.State != "running" {
			return plan, r, nil
		}
		select {
		case <-ctx.Done():
			return plan, r, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// printDeep renders the deep block: totals first, then what the copy will skip
// or cannot do, then the estimate and the warnings.
func printDeep(a *App, d *deepView) {
	fmt.Fprintf(a.out(), "deep scan: %s\n", d.State)
	if !d.Complete {
		fmt.Fprintln(a.out(), "  partial counts; not a total")
	}
	fmt.Fprintf(a.out(), "  files %d, folders %d, bytes %s\n", d.Files, d.Folders, present.HumanBytes(d.Bytes))
	fmt.Fprintf(a.out(), "  native (Google) %d, shortcuts %d, cannot copy %d, unreadable folders %d\n", d.Native, d.Shortcuts, d.CannotCopy, d.Unreadable)
	if d.EstimateSecs != nil {
		fmt.Fprintf(a.out(), "  estimated time %s (from recent copies)\n", (time.Duration(*d.EstimateSecs) * time.Second).String())
	} else {
		fmt.Fprintln(a.out(), "  estimated time unknown (no measured copies yet)")
	}
	for _, w := range d.Warnings {
		fmt.Fprintf(a.out(), "  warning: %s\n", w)
	}
}
