package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

func TestQuotaResumeTimes(t *testing.T) {
	if loc, err := time.LoadLocation("America/Los_Angeles"); err == nil {
		// 2026-10-27 12:00 UTC is 05:00 PDT; the next Pacific midnight is
		// 2026-10-28 00:00 PDT = 07:00 UTC.
		now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
		if got, want := nextDailyQuotaReset(now), time.Date(2026, 10, 28, 7, 0, 0, 0, time.UTC); !got.Equal(want) {
			t.Fatalf("daylight reset = %v, want %v", got, want)
		}
		// Across the DST fall-back (2026-11-01): midnight PST is UTC-8.
		now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
		if got, want := nextDailyQuotaReset(now), time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC); !got.Equal(want) {
			t.Fatalf("standard reset = %v, want %v", got, want)
		}
		_ = loc
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	daily := &drive.Error{Kind: drive.KindQuota, Status: 403, Reason: "dailyLimitExceeded"}
	storage := &drive.Error{Kind: drive.KindQuota, Status: 403, Reason: "storageQuotaExceeded"}
	other := &drive.Error{Kind: drive.KindQuota, Status: 403, Reason: "quotaExceeded"}
	if got := quotaResume(storage, now); !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("storage resume = %v", got)
	}
	if got := quotaResume(other, now); !got.Equal(now.Add(time.Hour)) {
		t.Fatalf("unknown quota resume = %v", got)
	}
	if got := quotaResume(daily, now); !got.After(now) {
		t.Fatalf("daily resume = %v", got)
	}
}

func TestDailyQuotaBlocksTheJobAndResumesAfterTheReset(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Quota", "root")
	e.fake.File("a.txt", src.ID, 10)
	e.fake.File("b.txt", src.ID, 20)
	var blocked atomic.Bool
	blocked.Store(true)
	e.fake.Before = func(op string, _ drive.File) error {
		if op == "copy" && blocked.Load() {
			return &drive.Error{Kind: drive.KindQuota, Status: 403, Reason: "dailyLimitExceeded"}
		}
		return nil
	}
	var mu sync.Mutex
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	eng := e.engine()
	eng.Now = clock
	job := e.submit(src)
	j := e.runEngine(eng, job.ID)
	if j.State != state.JobBlocked || j.LastError == nil || j.LastError.Code != "quota_exceeded" {
		t.Fatalf("state=%s err=%+v", j.State, j.LastError)
	}
	wantResume := time.Date(2026, 10, 28, 7, 0, 0, 0, time.UTC)
	if j.ResumeAfter == nil || !j.ResumeAfter.Equal(wantResume) {
		t.Fatalf("resume_after = %v, want %v", j.ResumeAfter, wantResume)
	}
	coll := mustColl(e, job.CollectionID)
	if coll.RootID == nil || len(e.fake.Children(*coll.RootID)) != 0 {
		t.Fatalf("a quota error copied files into the destination")
	}
	items, err := e.store.Items(context.Background(), job.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	pending, hit := 0, 0
	for _, it := range items {
		if it.Kind != state.KindFile {
			continue
		}
		// A worker that never reached the provider before the job blocked
		// leaves its item pending without an error code.
		if it.ErrorCode == "quota_exceeded" {
			hit++
		}
		if it.State != state.ItemPending || it.Attempts != 0 || (it.ErrorCode != "" && it.ErrorCode != "quota_exceeded") {
			t.Fatalf("item after quota = state %s attempts %d code %q", it.State, it.Attempts, it.ErrorCode)
		}
		pending++
	}
	if pending != 2 || hit == 0 {
		t.Fatalf("pending files = %d, quota hits = %d", pending, hit)
	}

	// The reset passes and the provider accepts writes again: the engine
	// resumes the same job (no new job id) and finishes it.
	blocked.Store(false)
	mu.Lock()
	now = wantResume
	mu.Unlock()
	j = e.runEngineUntil(eng, job.ID, func(j state.Job) bool {
		return j.State == state.JobSucceeded || j.State == state.JobPartial || j.State == state.JobFailed
	})
	if j.State != state.JobSucceeded {
		t.Fatalf("after resume: state=%s err=%+v", j.State, j.LastError)
	}
	if j.ID != job.ID {
		t.Fatalf("resume created a new job: %s -> %s", job.ID, j.ID)
	}
	if j.ResumeAfter != nil {
		t.Fatalf("resume_after survived the resume: %v", j.ResumeAfter)
	}
}

func TestStorageQuotaBlocksForADayWithoutBurningAttempts(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Full", "root")
	e.fake.File("a.txt", src.ID, 5)
	e.fake.Before = func(op string, _ drive.File) error {
		if op == "copy" {
			return &drive.Error{Kind: drive.KindQuota, Status: 403, Reason: "storageQuotaExceeded"}
		}
		return nil
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	eng := e.engine()
	eng.Now = func() time.Time { return now }
	j := e.runEngine(eng, e.submit(src).ID)
	if j.State != state.JobBlocked || j.ResumeAfter == nil || !j.ResumeAfter.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("state=%s resume=%v", j.State, j.ResumeAfter)
	}
	if got := e.fake.CallCount("copy"); got != 1 {
		t.Fatalf("storage quota retried blindly: %d copy calls", got)
	}
	items, _ := e.store.Items(context.Background(), j.CollectionID)
	for _, it := range items {
		if it.Kind == state.KindFile && it.Attempts != 0 {
			t.Fatalf("quota burned an attempt: %+v", it)
		}
	}
}

// The ledger records the bytes of completed copies once, by hour.
func TestCopiedBytesAreLedgeredOnce(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Sizes", "root")
	e.fake.File("a.bin", src.ID, 10)
	e.fake.File("b.bin", src.ID, 20)
	e.fake.File("c.bin", src.ID, 5)
	base, _ := e.baseline(src)

	now := time.Now().UTC()
	got, err := e.store.CopiedBytes24h(context.Background(), "owner", now)
	if err != nil || got != 35 {
		t.Fatalf("ledger = %d err=%v", got, err)
	}
	// An unchanged delta run re-completes nothing: the ledger must not grow.
	j := e.runEngine(e.engine(), e.runNow(base.CollectionID).ID)
	if j.State != state.JobSucceeded {
		t.Fatalf("delta: %s %+v", j.State, j.LastError)
	}
	if got, _ = e.store.CopiedBytes24h(context.Background(), "owner", now); got != 35 {
		t.Fatalf("ledger after a no-op run = %d", got)
	}
	if got, _ = e.store.CopiedBytes24h(context.Background(), "owner", now.Add(25*time.Hour)); got != 0 {
		t.Fatalf("ledger outside 24h = %d", got)
	}
}
