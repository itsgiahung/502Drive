package app

import (
	"context"
	"net/http"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/itsgiahung/502Drive/internal/state"
)

// previewSamples bounds the example names shown per outcome.
const previewSamples = 10

// addFilterSample records one listed top-level entry as included or excluded.
func (p *Preview) addFilterSample(m *filter.Matcher, k drive.File) {
	e := filter.Entry{Name: k.Name, Rel: k.Name, IsFolder: k.IsFolder(), MimeType: k.MimeType, Size: k.Size, Modified: k.Modified}
	if m.Decide(e) {
		p.FilterIncluded++
		if len(p.IncludedSamples) < previewSamples {
			p.IncludedSamples = append(p.IncludedSamples, k.Name)
		}
		return
	}
	p.FilterExcluded++
	if len(p.ExcludedSamples) < previewSamples {
		p.ExcludedSamples = append(p.ExcludedSamples, k.Name)
	}
}

// SetCollectionFilter replaces a collection's selective-sync rule. Only later
// runs use it: nothing already at the destination is removed, and a running
// job keeps the rule it was admitted with.
func (s *Service) SetCollectionFilter(ctx context.Context, p Principal, collectionID string, spec filter.Spec, expected int) (state.Collection, error) {
	if err := spec.Validate(); err != nil {
		return state.Collection{}, Fail(http.StatusBadRequest, "invalid_filter", err.Error())
	}
	c, err := s.Store.SetCollectionFilter(ctx, scope(p, true), collectionID, spec, expected)
	if err != nil {
		return state.Collection{}, err
	}
	return c, nil
}
