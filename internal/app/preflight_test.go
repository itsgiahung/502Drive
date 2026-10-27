package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/state"
)

func deepService(t *testing.T) (*app.Service, *drivetest.Fake, state.Destination) {
	t.Helper()
	fake := drivetest.New()
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	svc := &app.Service{Store: store, Provider: func(context.Context) (drive.Provider, error) { return fake, nil }}
	dst, _, err := store.AddDestination(context.Background(), "owner", "dst_key", nil, state.Destination{ID: "dst", FolderID: "root", Name: "My Drive", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	return svc, fake, dst
}

// waitDeep polls the plan until its deep scan leaves "running" (a condition,
// not a fixed sleep) and returns it.
func waitDeep(t *testing.T, svc *app.Service, planID string) state.DeepPreflight {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		p, err := svc.Plan(context.Background(), app.Owner, planID)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if p.Deep != nil && p.Deep.State != "running" {
			return *p.Deep
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("deep scan did not finish")
	return state.DeepPreflight{}
}

// TestDeepPreflightCountsWithoutWriting scans a tree with every kind of entry
// the copy engine can meet and checks the counts, the warnings, and that the
// scan performed no write against Drive.
func TestDeepPreflightCountsWithoutWriting(t *testing.T) {
	svc, fake, dst := deepService(t)
	ctx := context.Background()
	src := fake.Folder("Deep", "root")
	a := fake.File("a.txt", src.ID, 100)
	sub := fake.Folder("sub", src.ID)
	fake.File("b.txt", sub.ID, 50)
	fake.Put("Doc", "application/vnd.google-apps.document", src.ID, -1)
	fake.Shortcut("link", src.ID, a.ID)
	nocopy := fake.File("nocopy.bin", src.ID, 25)
	fake.Mutate(nocopy.ID, func(f *drive.File) { f.CanCopy = false })
	locked := fake.Folder("locked", src.ID)
	fake.Before = func(op string, f drive.File) error {
		if op == "list" && f.ID == locked.ID {
			return &drive.Error{Kind: drive.KindPermission, Status: 403, Reason: "forbidden"}
		}
		return nil
	}

	view, _, err := svc.Inspect(ctx, app.Owner, "deep-count", app.PlanRequest{Source: src.ID, DestinationID: dst.ID, Deep: true})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if view.Deep == nil || view.Deep.State != "running" || view.Deep.Complete {
		t.Fatalf("deep must start running, got %+v", view.Deep)
	}
	d := waitDeep(t, svc, view.ID)
	if d.State != "complete" || !d.Complete {
		t.Fatalf("scan = %+v", d)
	}
	if d.Folders != 2 || d.Files != 4 || d.Native != 1 || d.Shortcuts != 1 || d.CannotCopy != 1 || d.Unreadable != 1 {
		t.Fatalf("counts = %+v", d)
	}
	// Bytes covers what the copy would move: native docs have no size and the
	// no-permission file is not copyable, so only a.txt and b.txt count.
	if d.Bytes != 150 {
		t.Fatalf("bytes = %d, want 150", d.Bytes)
	}
	if d.EstimateSecs != nil || d.EstimateBasis != "unknown" {
		t.Fatalf("no measurement yet, estimate must be unknown: %+v", d)
	}
	if !hasWarning(d.Warnings, "estimate_unknown") {
		t.Fatalf("warnings = %v", d.Warnings)
	}
	if fake.Writes != 0 || fake.CallCount("copy") != 0 || fake.CallCount("folder") != 0 {
		t.Fatalf("deep scan wrote to Drive: writes=%d", fake.Writes)
	}
	_ = a
	_ = sub
}

// TestDeepPreflightCancelKeepsPartialCountsOutOfComplete cancels a scan in the
// middle: it stops, is not reported complete, and nothing is written.
func TestDeepPreflightCancelStopsScan(t *testing.T) {
	svc, fake, dst := deepService(t)
	ctx := context.Background()
	src := fake.Folder("Wide", "root")
	for i := range 60 {
		fake.Folder("f"+string(rune('a'+i%26))+string(rune('a'+i/26)), src.ID)
	}
	fake.Before = func(op string, f drive.File) error {
		if op == "list" {
			time.Sleep(5 * time.Millisecond)
		}
		return nil
	}
	view, _, err := svc.Inspect(ctx, app.Owner, "deep-cancel", app.PlanRequest{Source: src.ID, DestinationID: dst.ID, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	// Wait until the scan has made progress, then cancel it.
	for range 2000 {
		p, _ := svc.Plan(ctx, app.Owner, view.ID)
		if p.Deep != nil && p.Deep.Scanned > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err = svc.CancelPreflight(ctx, app.Owner, view.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	d := waitDeep(t, svc, view.ID)
	if d.State != "cancelled" || d.Complete {
		t.Fatalf("cancelled scan = %+v", d)
	}
	if fake.Writes != 0 {
		t.Fatalf("cancelled scan wrote %d times", fake.Writes)
	}
}

// TestDeepPreflightExpiresWithThePlan stops a scan whose plan has expired and
// says so rather than reporting a partial tree as complete.
func TestDeepPreflightExpiresWithThePlan(t *testing.T) {
	svc, fake, dst := deepService(t)
	svc.DeepPlanTTL = time.Nanosecond
	src := fake.Folder("Short", "root")
	fake.Folder("x", src.ID)
	view, _, err := svc.Inspect(context.Background(), app.Owner, "deep-expire", app.PlanRequest{Source: src.ID, DestinationID: dst.ID, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if d := waitDeep(t, svc, view.ID); d.State != "expired" || d.Complete {
		t.Fatalf("expired scan = %+v", d)
	}
}

// TestDeepPreflightAfterRestartIsInterrupted: a scan that was running when the
// daemon stopped has no goroutine left, so it is reported as interrupted, and
// cancelling it is a no-op that keeps the state.
func TestDeepPreflightAfterRestartIsInterrupted(t *testing.T) {
	svc, fake, dst := deepService(t)
	ctx := context.Background()
	src := fake.Folder("Gone", "root")
	plan := state.Plan{ID: "plan_gone", Principal: "owner", SourceID: src.ID, SourceName: "Gone", SourceKind: state.KindFolder,
		DestinationID: dst.ID, RootName: "Gone", ExpiresAt: time.Now().Add(time.Hour), Revision: 1,
		Deep: &state.DeepPreflight{State: "running"}}
	if _, _, err := svc.Store.PutPlanIdempotent(ctx, "owner", "k-gone", "req", plan); err != nil {
		t.Fatal(err)
	}
	p, err := svc.Plan(ctx, app.Owner, "plan_gone")
	if err != nil {
		t.Fatal(err)
	}
	if p.Deep == nil || p.Deep.State != "interrupted" {
		t.Fatalf("restart must report interrupted, got %+v", p.Deep)
	}
	if _, err = svc.CancelPreflight(ctx, app.Owner, "plan_gone"); err != nil {
		t.Fatal(err)
	}
	if p, _ = svc.Plan(ctx, app.Owner, "plan_gone"); p.Deep.State != "interrupted" {
		t.Fatalf("cancel of a stopped scan changed state to %s", p.Deep.State)
	}
}

// TestDeepPreflightWarnsAboutStorageAndDailyLimit: the scan does not block on
// quota; it only warns when the copy would not fit the free space or would
// exceed the remaining daily server-side copy allowance.
func TestDeepPreflightWarnsAboutStorageAndDailyLimit(t *testing.T) {
	svc, fake, dst := deepService(t)
	src := fake.Folder("Quota", "root")
	fake.File("big.bin", src.ID, 150)
	fake.QuotaLimit, fake.QuotaUsage = 100, 90 // 10 bytes free, the copy needs 150
	svc.DailyCopyLimit = 120                   // the copy would exceed the day's allowance

	view, _, err := svc.Inspect(context.Background(), app.Owner, "deep-quota", app.PlanRequest{Source: src.ID, DestinationID: dst.ID, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	d := waitDeep(t, svc, view.ID)
	if d.State != "complete" {
		t.Fatalf("quota must warn, not block: %+v", d)
	}
	if !hasWarning(d.Warnings, "destination_storage_short") || !hasWarning(d.Warnings, "may_hit_daily_limit") {
		t.Fatalf("warnings = %v", d.Warnings)
	}
	if d.StorageFree == nil || *d.StorageFree != 10 {
		t.Fatalf("storage free = %v", d.StorageFree)
	}
}

func hasWarning(ws []string, want string) bool {
	for _, w := range ws {
		if w == want {
			return true
		}
	}
	return false
}
