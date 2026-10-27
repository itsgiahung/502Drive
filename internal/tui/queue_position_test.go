package tui

import (
	"strings"
	"testing"
)

func TestQueueUsesServerAheadAndSuppressesWhilePaused(t *testing.T) {
	m := New(Actions{}, Options{Language: "en"})
	ahead := 8
	j := Job{State: "queued", QueueAhead: &ahead}
	if got := m.queueLine(j); !strings.Contains(got, "8 ahead") {
		t.Fatal(got)
	}
	m.snap.Status.QueuePaused = true
	if got := m.queueLine(j); strings.Contains(got, "ahead") || !strings.Contains(got, "paused") {
		t.Fatal(got)
	}
	m.snap.Status.QueuePaused = false
	j.QueueAhead = nil
	if got := m.queueLine(j); strings.Contains(got, "ahead") {
		t.Fatal(got)
	}
}
