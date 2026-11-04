package tui

import (
	"context"
	"fmt"
	"testing"
)

func TestLibraryScrollLoadsMoreWithoutMovingSelection(t *testing.T) {
	var sources []Collection
	for i := range 21 {
		sources = append(sources, Collection{ID: fmt.Sprintf("col_%02d", i), SourceName: fmt.Sprintf("Source %d", i), State: "ready"})
	}
	pages := 1
	m := New(Actions{
		Snapshot: func(context.Context) (Snapshot, error) {
			snap := Snapshot{Collections: sources[:min(20*pages, len(sources))]}
			if pages == 1 {
				snap.CollectionsNext = "next"
			}
			return snap, nil
		},
		SetCollectionPages: func(n int) { pages = n },
	}, Options{Width: 80, Height: 24, NoMotion: true, Language: "en"})
	m = settle(t, m, m.fetch())
	m = sendKeys(t, m, "3", "down", "end")
	if pages != 2 || len(m.snap.Collections) != 21 || m.cursor != 19 || m.snap.Collections[m.cursor].ID != "col_19" {
		t.Fatalf("loading retargeted row: pages=%d rows=%d cursor=%d", pages, len(m.snap.Collections), m.cursor)
	}
	m = sendKeys(t, m, "end")
	if m.cursor != 20 || m.snap.Collections[m.cursor].ID != "col_20" || pages != 2 {
		t.Fatal("last source is inaccessible or another page was fetched")
	}
}
