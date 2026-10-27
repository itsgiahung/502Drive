package tui

import (
	"context"
	"github.com/itsgiahung/502Drive/internal/filter"
	"strings"
	"testing"
)

func TestFilterEditorValidatesBeforeSavingAndPreservesAllFields(t *testing.T) {
	var saved filter.Spec
	calls := 0
	acts := Actions{SetCollectionFilter: func(_ context.Context, _ string, _ int, s filter.Spec) error { saved = s; calls++; return nil }}
	m := New(acts, Options{Language: "en"})
	c := Collection{ID: "c", Revision: 7, Filter: filter.Spec{ModifiedAfter: "2026-01-01T00:00:00Z", KeepEmptyFolders: true}}
	m = m.openFilter(c)
	m.fe.fields[0].SetValue("[")
	m = m.validateFilter()
	if !strings.Contains(m.View(), "bad pattern") {
		t.Fatal(m.View())
	}
	m = sendKeys(t, m, "ctrl+s")
	if calls != 0 {
		t.Fatal("saved invalid filter")
	}
	m.fe.fields[0].SetValue("docs/**;*.pdf;class:doc")
	m.fe.fields[2].SetValue("10MB")
	m = m.validateFilter()
	m = sendKeys(t, m, "ctrl+s")
	if calls != 1 || len(saved.Include) != 3 || saved.MinSize != 10<<20 || !saved.KeepEmptyFolders || saved.ModifiedAfter == "" {
		t.Fatalf("saved=%+v calls=%d", saved, calls)
	}
}
