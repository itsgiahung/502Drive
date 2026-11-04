package tray

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNotificationsReadBeyondMenuAndRetryIncompletePages(t *testing.T) {
	now := time.Unix(100, 0)
	var notices []Notice
	var sinceTimes []time.Time
	failedPage := true
	var commands [][]string
	host := Host{GOOS: "linux", Run: func(_ context.Context, argv []string) (string, error) {
		commands = append(commands, argv)
		return "", nil
	}}
	host.Notify = func(_ context.Context, n Notice, _ func()) error { notices = append(notices, n); return nil }
	a := newAgent(Options{
		Host: host, Now: func() time.Time { return now }, OpenJob: func(string) error { return nil },
		Status: func(context.Context) (Status, error) { return Status{Ready: true}, nil },
		Jobs:   func(context.Context) ([]Job, error) { return []Job{{ID: "menu", State: "running"}}, nil },
		JobChanges: func(_ context.Context, since time.Time, cursor string) ([]Job, string, error) {
			sinceTimes = append(sinceTimes, since)
			if cursor == "" {
				return []Job{{ID: "fast", State: "blocked", LastErrorCode: "quota_exceeded", SourceName: "Fast"}}, "next", nil
			}
			if failedPage {
				return nil, "", errors.New("temporary read failure")
			}
			return []Job{{ID: "old", State: "failed", SourceName: "Old"}}, "", nil
		},
	})
	a.p = &fakePresenter{}
	a.refresh(t.Context())
	if len(sinceTimes) != 0 || len(notices) != 0 {
		t.Fatal("first poll must only prime notification baseline")
	}
	now = now.Add(time.Second)
	a.refresh(t.Context())
	if len(notices) != 1 || notices[0].JobID != "fast" {
		t.Fatalf("job outside menu did not notify: %+v", notices)
	}
	failedPage = false
	now = now.Add(time.Second)
	a.refresh(t.Context())
	if len(commands) != 1 { // the failed job uses notify-send; blocked uses the Open action
		t.Fatalf("completion after failed page missing or duplicated: %v", commands)
	}
	if len(sinceTimes) != 4 || !sinceTimes[0].Equal(sinceTimes[2]) || len(notices) != 1 {
		t.Fatalf("failed page advanced cursor or duplicated first notice: times=%v notices=%v", sinceTimes, notices)
	}
	if len(a.snap.Jobs) != 1 || a.snap.Jobs[0].ID != "menu" {
		t.Fatal("notification history replaced the menu")
	}
}
