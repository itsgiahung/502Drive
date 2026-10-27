package app_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/itsgiahung/502Drive/internal/state"
)

// The plan preview reports how the filter splits the top level, with at most
// ten examples per outcome, and a bad rule is refused before anything is stored.
func TestPlanPreviewCountsAndSamplesTheFilter(t *testing.T) {
	fake := drivetest.New()
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := &app.Service{Store: store, Provider: func(context.Context) (drive.Provider, error) { return fake, nil }}
	dst, _, err := store.AddDestination(context.Background(), "owner", "dst_key", nil, state.Destination{ID: "dst", FolderID: "root", Name: "My Drive", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	src := fake.Folder("Lib", "root")
	for i := range 12 {
		fake.File(fmt.Sprintf("doc%02d.tmp", i), src.ID, 1)
	}
	fake.File("keep.pdf", src.ID, 1)
	fake.Folder("sub", src.ID)

	view, _, err := svc.Inspect(context.Background(), app.Owner, "pk1", app.PlanRequest{
		Source: src.ID, DestinationID: dst.ID, Filter: filter.Spec{Exclude: []string{"*.tmp"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := view.Preview
	if p.FilterExcluded != 12 || p.FilterIncluded != 2 {
		t.Fatalf("included=%d excluded=%d, want 2 and 12", p.FilterIncluded, p.FilterExcluded)
	}
	if len(p.ExcludedSamples) != 10 || len(p.IncludedSamples) != 2 {
		t.Fatalf("samples: excluded=%d included=%d", len(p.ExcludedSamples), len(p.IncludedSamples))
	}

	_, _, err = svc.Inspect(context.Background(), app.Owner, "pk2", app.PlanRequest{
		Source: src.ID, DestinationID: dst.ID, Filter: filter.Spec{Include: []string{"class:movies"}},
	})
	if err == nil || app.Classify(err).Status != http.StatusBadRequest || app.Classify(err).Code != "invalid_filter" {
		t.Fatalf("bad rule: %v", err)
	}
}
