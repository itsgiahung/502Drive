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

// Keep-empty-folders is a switch, sizes and dates take the forms people type,
// and mistakes are explained in words, never as a Go parse error.
func TestFilterEditorUsesASwitchAndPlainInputs(t *testing.T) {
	var saved filter.Spec
	acts := Actions{SetCollectionFilter: func(_ context.Context, _ string, _ int, s filter.Spec) error { saved = s; return nil }}
	m := New(acts, Options{Language: "en"})
	m = m.openFilter(Collection{ID: "c", Revision: 1, Filter: filter.Spec{MinSize: 10 << 20, ModifiedAfter: "2026-01-31T00:00:00Z"}})
	if m.fe.fields[2].Value() != "10MB" || m.fe.fields[3].Value() != "" || m.fe.fields[4].Value() != "2026-01-31" {
		t.Fatalf("stored bounds shown as %q %q %q", m.fe.fields[2].Value(), m.fe.fields[3].Value(), m.fe.fields[4].Value())
	}
	m = sendKeys(t, m, "tab", "tab", "tab", "tab", "tab") // to the switch
	if m.fe.row != keepRow {
		t.Fatalf("row = %d, want the switch", m.fe.row)
	}
	m = sendKeys(t, m, "f", "a", "l")
	if m.fe.keep || m.fe.err != "" {
		t.Fatal("letters must not change or break the switch")
	}
	m = sendKeys(t, m, " ")
	if !m.fe.keep || !strings.Contains(m.View(), "[✓] Yes") {
		t.Fatalf("space did not turn the switch on:\n%s", m.View())
	}
	m.fe.fields[2].SetValue("ten megs")
	m = m.validateFilter()
	if view := m.View(); strings.Contains(view, "strconv") || !strings.Contains(view, "e.g. 500KB") {
		t.Fatalf("size error not explained in words:\n%s", view)
	}
	m.fe.fields[2].SetValue("")
	m.fe.fields[4].SetValue("31/01/2026")
	m = m.validateFilter()
	if !strings.Contains(m.View(), "2026-01-31") {
		t.Fatalf("date error not explained:\n%s", m.View())
	}
	m.fe.fields[4].SetValue("2026-02-01")
	m = m.validateFilter()
	m = sendKeys(t, m, "ctrl+s")
	if !saved.KeepEmptyFolders || saved.ModifiedAfter != "2026-02-01T00:00:00Z" || saved.MinSize != 0 {
		t.Fatalf("saved %+v", saved)
	}
}
