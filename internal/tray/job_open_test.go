package tray

import (
	"context"
	"strings"
	"testing"
)

func TestOpenJobPassesOnlyValidatedIDAsArguments(t *testing.T) {
	h := &fakeHost{goos: "linux", have: map[string]bool{"xterm": true}}
	if err := OpenTUIJob(h.Host(), "/synthetic/502drive", "job_safe"); err != nil {
		t.Fatal(err)
	}
	if len(h.started) != 1 || strings.Join(h.started[0], "|") != "xterm|-e|/synthetic/502drive|menu|--job|job_safe" {
		t.Fatal(h.started)
	}
	if err := OpenTUIJob(h.Host(), "/synthetic/502drive", "job_x;bad"); err == nil {
		t.Fatal("unsafe job ID reached terminal")
	}
}
func TestTrayJobSlotUsesSnapshotID(t *testing.T) {
	var opened string
	a := newAgent(Options{OpenJob: func(id string) error { opened = id; return nil }})
	a.snap.Jobs = []Job{{ID: "job_safe", SourceName: "$(unsafe)"}}
	a.action(context.Background(), "job:0")
	if opened != "job_safe" {
		t.Fatal(opened)
	}
	opened = ""
	a.action(context.Background(), "job:99")
	if opened != "" {
		t.Fatal(opened)
	}
}
