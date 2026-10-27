package tray

import (
	"context"
	"testing"
	"time"
)

func TestNeedsActionNoticesCoalesceAndHonorPreferences(t *testing.T) {
	now := time.Unix(100, 0)
	var got []Notice
	var opened []string
	host := Host{GOOS: "linux", Notify: func(_ context.Context, n Notice, open func()) error { got = append(got, n); open(); return nil }}
	a := newAgent(Options{Host: host, Now: func() time.Time { return now }, OpenJob: func(id string) error { opened = append(opened, id); return nil }})
	notices := jobTransitions([]Job{{ID: "job_a", State: "running"}, {ID: "job_b", State: "running"}}, []Job{{ID: "job_a", State: "blocked", LastErrorCode: "quota_exceeded"}, {ID: "job_b", State: "blocked", LastErrorCode: "quota_exceeded"}}, "en")
	a.notify(context.Background(), notices)
	if len(got) != 1 || len(opened) != 1 || opened[0] != "job_a" {
		t.Fatalf("got=%v opened=%v", got, opened)
	}
	now = now.Add(59 * time.Second)
	a.notify(context.Background(), notices)
	if len(got) != 1 {
		t.Fatal(got)
	}
	now = now.Add(time.Second)
	a.notify(context.Background(), notices)
	if len(got) != 2 {
		t.Fatal(got)
	}
	a.o.NotifyEnabled = func(context.Context, string) (bool, error) { return false, nil }
	now = now.Add(time.Minute)
	a.notify(context.Background(), notices)
	if len(got) != 2 {
		t.Fatal("disabled notifications delivered")
	}
}
