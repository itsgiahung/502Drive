package cli

import (
	"context"
	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/itsgiahung/502Drive/internal/tui"
	"net/url"
)

func (a *App) tuiSetCollectionFilter(ctx context.Context, id string, rev int, spec filter.Spec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	_, err := a.call(ctx, "PUT", "/v1/collections/"+url.PathEscape(id)+"/filter", nil, map[string]any{"filter": spec, "expected_revision": rev}, NewKey())
	return err
}
func (a *App) tuiPreviewCollectionFilter(ctx context.Context, c tui.Collection, spec filter.Spec) (*tui.PlanFilter, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	p, _, err := a.makePlan(ctx, c.SourceID, c.DestinationID, c.RootName, NewKey(), planOpts{Filter: spec})
	if err != nil {
		return nil, err
	}
	return tuiPlan(p).Filter, nil
}
