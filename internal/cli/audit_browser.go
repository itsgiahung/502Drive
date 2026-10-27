package cli

import (
	"context"
	"github.com/itsgiahung/502Drive/internal/tui"
	"net/url"
)

func (a *App) tuiAuditFindings(ctx context.Context, id, kind, cursor string) (tui.FindingPage, error) {
	q := url.Values{"limit": {"20"}}
	if kind != "" {
		q.Set("kind", kind)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	r, err := a.call(ctx, "GET", "/v1/audits/"+url.PathEscape(id)+"/findings", q, nil, "")
	if err != nil {
		return tui.FindingPage{}, err
	}
	var page struct {
		Items []findingView `json:"items"`
		Next  *string       `json:"next_cursor"`
	}
	if err = r.Decode(&page); err != nil {
		return tui.FindingPage{}, err
	}
	out := tui.FindingPage{}
	if page.Next != nil {
		out.Next = *page.Next
	}
	for _, f := range page.Items {
		out.Items = append(out.Items, tui.Finding{Path: f.Path, Kind: f.Kind, SourceID: f.SourceID, DestID: f.DestID, Detail: f.Detail, Repairable: f.Repairable, Repaired: f.Repaired})
	}
	return out, nil
}
func (a *App) tuiGetJob(ctx context.Context, id string) (tui.Job, error) {
	r, err := a.call(ctx, "GET", "/v1/jobs/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return tui.Job{}, err
	}
	var j tuiJobView
	if err = r.Decode(&j); err != nil {
		return tui.Job{}, err
	}
	return j.job(), nil
}
