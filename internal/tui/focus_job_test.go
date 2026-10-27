package tui

import "testing"

func TestFocusedJobSurvivesListReordering(t *testing.T) {
	m := New(Actions{}, Options{JobID: "job_target", NoMotion: true})
	first := Snapshot{Jobs: []Job{{ID: "job_other"}, {ID: "job_target", SourceName: "Target", State: "blocked"}}}
	next, _ := m.onSnapshot(snapshotMsg{snap: first})
	m = next.(Model)
	if j, ok := m.currentJob(); !ok || j.ID != "job_target" {
		t.Fatal(j, ok)
	}
	first.Jobs = append([]Job{{ID: "job_new"}}, first.Jobs...)
	next, _ = m.onSnapshot(snapshotMsg{snap: first})
	m = next.(Model)
	if j, ok := m.currentJob(); !ok || j.ID != "job_target" {
		t.Fatal(j, ok)
	}
}
