package state

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDefaultTimezoneFromTZ(t *testing.T) {
	t.Setenv("TZ", "America/New_York")
	if got := DefaultTimezone(); got != "America/New_York" {
		t.Fatalf("TZ zone = %q", got)
	}
	t.Setenv("TZ", "Mars/Olympus")
	if got := DefaultTimezone(); got != "Asia/Ho_Chi_Minh" {
		t.Fatalf("invalid TZ fell back to %q", got)
	}
	t.Setenv("TZ", "")
	if got := DefaultTimezone(); got != "Asia/Ho_Chi_Minh" {
		t.Fatalf("unset TZ = %q", got)
	}
}

func TestAdmissionSurvivesRestartAndConcurrentReplay(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := Plan{ID: "plan_test", Principal: "owner", SourceID: "source_test", SourceName: "Alpha", DestinationID: "dest_test", RootName: "Alpha", ExpiresAt: time.Now().Add(time.Minute), Revision: 1}
	if err = s.PutPlan(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.Admit(context.Background(), "owner", "same-key", p.ID, 1)
			if e != nil {
				t.Error(e)
				return
			}
			ids <- r.Job.ID
		}()
	}
	wg.Wait()
	close(ids)
	var id string
	for got := range ids {
		if id != "" && got != id {
			t.Fatal("duplicate admission", got, id)
		}
		id = got
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Admit(context.Background(), "owner", "same-key", p.ID, 1)
	if err != nil || !r.Replayed || r.Job.ID != id {
		t.Fatalf("replay after restart: %+v %v", r, err)
	}
	_, err = s.Admit(context.Background(), "owner", "same-key", "other_plan", 1)
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("payload mismatch: %v", err)
	}
	r2, err := s.Admit(context.Background(), "owner", "another-key", p.ID, 1)
	if err != nil || r2.Job.ID != id {
		t.Fatalf("consumed plan created another job: %+v %v", r2, err)
	}
	events, err := s.Events(context.Background(), "owner", id, 0, 100)
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %v: %v", events, err)
	}
}

func TestScopeRevisionAndPausedStateSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p := Plan{ID: "p", Principal: "alice", ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err = s.PutPlan(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Admit(ctx, "bob", "k", "p", 1); !errors.Is(err, ErrNotFound) {
		t.Fatal("scope", err)
	}
	r, err := s.Admit(ctx, "alice", "k", "p", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Action(ctx, "bob", "x", r.Job.ID, "cancel", 1); !errors.Is(err, ErrNotFound) {
		t.Fatal("action scope", err)
	}
	if _, err = s.Action(ctx, "alice", "x", r.Job.ID, "pause", 99); !errors.Is(err, ErrStale) {
		t.Fatal("stale", err)
	}
	paused, err := s.Action(ctx, "alice", "x", r.Job.ID, "pause", 1)
	if err != nil || paused.Job.State != "paused" {
		t.Fatalf("pause %+v %v", paused, err)
	}
	replay, err := s.Action(ctx, "alice", "x", r.Job.ID, "pause", 1)
	if err != nil || !replay.Replayed {
		t.Fatal("replay must precede revision check", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	j, err := s.GetJob(ctx, "alice", r.Job.ID)
	if err != nil || j.State != "paused" || j.DesiredState != "pause" {
		t.Fatalf("lost pause %+v %v", j, err)
	}
}

func TestExclusiveLockAndExpiredPlan(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if other, e := Open(dir); e == nil {
		other.Close()
		t.Fatal("second writer admitted")
	}
	p := Plan{ID: "expired", Principal: "owner", Revision: 1, ExpiresAt: time.Now().Add(-time.Minute)}
	if err = s.PutPlan(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Admit(context.Background(), "owner", "k", p.ID, 1); !errors.Is(err, ErrExpired) {
		t.Fatal("expiry", err)
	}
	jobs, _, err := s.ListJobs(context.Background(), "owner", "", 100, 0)
	if err != nil || len(jobs) != 0 {
		t.Fatal("rejected admission left a job", jobs, err)
	}
}
