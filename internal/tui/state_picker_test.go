package tui

import (
	"context"
	"testing"
)

func TestStatePickerCombinesAndCancels(t *testing.T) {
	m := New(Actions{Snapshot: func(_ctx context.Context) (Snapshot, error) { return Snapshot{}, nil }}, Options{Language: "en"})
	m.screen = screenJobs
	m = m.openStatePicker()
	next, _ := m.statePickerKey(" ")
	m = next.(Model)
	next, _ = m.statePickerKey("down")
	m = next.(Model)
	next, _ = m.statePickerKey(" ")
	m = next.(Model)
	next, _ = m.statePickerKey("enter")
	m = next.(Model)
	if m.jq.State != "queued,running" {
		t.Fatal(m.jq)
	}
	m = m.openStatePicker()
	next, _ = m.statePickerKey(" ")
	m = next.(Model)
	next, _ = m.statePickerKey("esc")
	m = next.(Model)
	if m.jq.State != "queued,running" {
		t.Fatal(m.jq)
	}
}
