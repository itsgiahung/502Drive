package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

func TestSkippedClassificationCodes(t *testing.T) {
	e := newEnv(t)
	fake := e.fake
	root := fake.Folder("Root", "root")

	// 1. Unresolvable shortcut (target does not exist)
	fake.Shortcut("broken.lnk", root.ID, "non_existent_target_id")

	// 2. Direct shortcut loop (points to self)
	sSelfLoop := fake.Shortcut("self.lnk", root.ID, "temp")
	fake.Mutate(sSelfLoop.ID, func(f *drive.File) {
		f.ShortcutTargetID = sSelfLoop.ID
	})

	// 3. Two-step shortcut loop (A -> B -> A)
	sLoopA := fake.Shortcut("loopA.lnk", root.ID, "temp")
	sLoopB := fake.Shortcut("loopB.lnk", root.ID, sLoopA.ID)
	fake.Mutate(sLoopA.ID, func(f *drive.File) {
		f.ShortcutTargetID = sLoopB.ID
	})

	// 4. Shortcut followed 1 level (A -> B -> valid target C)
	validTarget := fake.File("valid.txt", root.ID, 100)
	sTargetB := fake.Shortcut("targetB.lnk", root.ID, validTarget.ID)
	fake.Shortcut("follow1.lnk", root.ID, sTargetB.ID)

	// 5. File that cannot be copied (CanCopy == false)
	noCopyFile := fake.File("protected.pdf", root.ID, 200)
	fake.Mutate(noCopyFile.ID, func(f *drive.File) {
		f.CanCopy = false
	})

	// 6. Google native export-only file (Google My Maps)
	fake.Put("MyMap", "application/vnd.google-apps.map", root.ID, -1)

	// 7. File too large for quota
	tooLargeFile := fake.File("huge.iso", root.ID, 1000)
	fake.Before = func(op string, f drive.File) error {
		if op == "copy" && f.ID == tooLargeFile.ID {
			return &drive.Error{Kind: drive.KindQuota, Reason: "too_large_for_quota"}
		}
		return nil
	}

	job := e.submit(root)
	finalJob := e.runUntil(job.ID, func(j state.Job) bool {
		return j.State == state.JobSucceeded || j.State == state.JobPartial || j.State == state.JobFailed || j.State == state.JobBlocked
	})

	if finalJob.State != state.JobSucceeded && finalJob.State != state.JobPartial {
		t.Fatalf("unexpected job state: %s, last error: %+v", finalJob.State, finalJob.LastError)
	}

	// Verify skipped breakdown by code
	byCode := finalJob.Counts.SkippedByCode
	if byCode == nil {
		t.Fatalf("expected SkippedByCode to be populated, got nil (counts: %+v)", finalJob.Counts)
	}

	if byCode[state.SkipShortcutUnresolved] < 1 {
		t.Errorf("expected shortcut_unresolved >= 1, got %d", byCode[state.SkipShortcutUnresolved])
	}
	if byCode[state.SkipShortcutLoop] < 2 {
		t.Errorf("expected shortcut_loop >= 2 (self loop + A-B cycle), got %d", byCode[state.SkipShortcutLoop])
	}
	if byCode[state.SkipCannotCopy] < 1 {
		t.Errorf("expected cannot_copy >= 1, got %d", byCode[state.SkipCannotCopy])
	}
	if byCode[state.SkipGoogleNativeExport] < 1 {
		t.Errorf("expected google_native_export_only >= 1, got %d", byCode[state.SkipGoogleNativeExport])
	}
	if byCode[state.SkipTooLargeForQuota] < 1 {
		t.Errorf("expected too_large_for_quota >= 1, got %d", byCode[state.SkipTooLargeForQuota])
	}

	// Check that follow1.lnk was followed 1 level and created at destination
	destTree := strings.Join(tree(fake, e.dest.ID, ""), "\n")
	if !strings.Contains(destTree, "follow1.lnk ->shortcut") {
		t.Errorf("expected follow1.lnk to be created at destination, tree:\n%s", destTree)
	}
}

func TestShortcutSkipPolicy(t *testing.T) {
	e := newEnv(t)
	fake := e.fake
	root := fake.Folder("Root", "root")
	validTarget := fake.File("valid.txt", root.ID, 100)
	fake.Shortcut("shortcut.lnk", root.ID, validTarget.ID)

	p := state.Plan{
		ID:                "plan_skip_shortcuts",
		Principal:         "owner",
		SourceID:          root.ID,
		SourceName:        root.Name,
		SourceKind:        state.KindFolder,
		DestinationID:     "dest",
		DestinationFolder: e.dest.ID,
		RootName:          root.Name,
		Shortcuts:         "skip",
		ExpiresAt:         time.Now().Add(time.Hour),
		Revision:          1,
	}
	if err := e.store.PutPlan(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	r, err := e.store.Admit(context.Background(), "owner", "key_skip_shortcuts", p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}

	finalJob := e.runUntil(r.Job.ID, func(j state.Job) bool {
		return j.State == state.JobSucceeded || j.State == state.JobPartial || j.State == state.JobFailed
	})

	if finalJob.Counts.SkippedByCode[state.SkipShortcutUnresolved] < 1 {
		t.Errorf("expected shortcut to be skipped with shortcut_unresolved, got counts: %+v", finalJob.Counts)
	}
}

// A shortcut whose target lookup fails because the machine was offline (or the
// account was throttled) says nothing about the target: it must be retried,
// never recorded as shortcut_unresolved.
func TestShortcutLookupOfflineIsRetriedNotSkipped(t *testing.T) {
	e := newEnv(t)
	fake := e.fake
	root := fake.Folder("Root", "root")
	target := fake.File("real.txt", root.ID, 10)
	fake.Shortcut("link.lnk", root.ID, target.ID)

	var lookups int
	fake.Before = func(op string, f drive.File) error {
		if op == "get" && f.ID == target.ID {
			lookups++
			if lookups <= 2 {
				return unreachable()
			}
		}
		return nil
	}

	job := e.submit(root)
	final := e.runUntil(job.ID, func(j state.Job) bool {
		return j.State == state.JobSucceeded || j.State == state.JobPartial || j.State == state.JobFailed || j.State == state.JobBlocked
	})
	if final.State != state.JobSucceeded {
		t.Fatalf("job = %s (%+v), counts %+v", final.State, final.LastError, final.Counts)
	}
	if n := final.Counts.SkippedByCode[state.SkipShortcutUnresolved]; n != 0 {
		t.Fatalf("offline lookup was recorded as shortcut_unresolved (%d)", n)
	}
	if lookups < 3 {
		t.Fatalf("lookup retried %d times, want the failed ones plus one success", lookups)
	}
	destTree := strings.Join(tree(fake, e.dest.ID, ""), "\n")
	if !strings.Contains(destTree, "link.lnk ->shortcut") {
		t.Fatalf("shortcut was not created after the lookup succeeded:\n%s", destTree)
	}
}

// A target the account cannot read is a definitive answer: a stable skip.
func TestShortcutForbiddenTargetIsUnresolved(t *testing.T) {
	e := newEnv(t)
	fake := e.fake
	root := fake.Folder("Root", "root")
	target := fake.File("secret.txt", root.ID, 10)
	fake.Shortcut("locked.lnk", root.ID, target.ID)
	fake.Before = func(op string, f drive.File) error {
		if op == "get" && f.ID == target.ID {
			return &drive.Error{Kind: drive.KindPermission, Status: 403}
		}
		return nil
	}

	job := e.submit(root)
	final := e.runUntil(job.ID, func(j state.Job) bool {
		return j.State == state.JobSucceeded || j.State == state.JobPartial || j.State == state.JobFailed || j.State == state.JobBlocked
	})
	if n := final.Counts.SkippedByCode[state.SkipShortcutUnresolved]; n != 1 {
		t.Fatalf("shortcut_unresolved = %d, want 1 (counts %+v)", n, final.Counts)
	}
}

// Inside a Shared Drive a file the account may not copy is skipped with
// cannot_copy while its siblings still copy; the job does not stop.
func TestSharedDriveRestrictedFileSkippedAsCannotCopy(t *testing.T) {
	e := newEnv(t)
	fake := e.fake
	sd := fake.SharedDrive("Team Drive")
	root := fake.Folder("Team Project", sd.ID)
	fake.File("open.txt", root.ID, 10)
	locked := fake.File("locked.txt", root.ID, 10)
	fake.Mutate(locked.ID, func(f *drive.File) { f.CanCopy = false })

	job := e.submit(root)
	final := e.runUntil(job.ID, func(j state.Job) bool {
		return j.State == state.JobSucceeded || j.State == state.JobPartial || j.State == state.JobFailed || j.State == state.JobBlocked
	})
	if final.State != state.JobSucceeded {
		t.Fatalf("job = %s (%+v), want succeeded: a policy skip is not a failure", final.State, final.LastError)
	}
	if n := final.Counts.SkippedByCode[state.SkipCannotCopy]; n != 1 {
		t.Fatalf("cannot_copy = %d, want 1 (counts %+v)", n, final.Counts)
	}
	kids := fake.Children(fake.Children(e.dest.ID)[0].ID)
	if len(kids) != 1 || kids[0].Name != "open.txt" {
		t.Fatalf("destination children = %+v", kids)
	}
	for _, k := range kids {
		if k.DriveID != "" {
			t.Fatalf("copy inherited a Shared Drive id: %+v", k)
		}
	}
}
