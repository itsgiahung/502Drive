package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// submitPolicy admits a copy with an explicit on_conflict policy.
func (e *env) submitPolicy(src drive.File, policy string) state.Job {
	e.t.Helper()
	e.n++
	kind := state.KindFile
	if src.IsFolder() {
		kind = state.KindFolder
	}
	p := state.Plan{ID: fmt.Sprintf("plan%d", e.n), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: kind, SourceMime: src.MimeType, SourceSize: src.Size, SourceVersion: src.Fingerprint(),
		DestinationID: "dest", DestinationFolder: e.dest.ID, RootName: src.Name, OnConflict: policy, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := e.store.PutPlan(context.Background(), p); err != nil {
		e.t.Fatal(err)
	}
	r, err := e.store.Admit(context.Background(), "owner", fmt.Sprintf("key%d", e.n), p.ID, 1)
	if err != nil {
		e.t.Fatal(err)
	}
	return r.Job
}

func destNames(e *env, rootID string) []string {
	var out []string
	for _, c := range e.fake.Children(rootID) {
		out = append(out, c.Name)
	}
	return out
}

func TestConflictNamesKeepExtensions(t *testing.T) {
	cases := map[string]struct {
		n    int
		want string
	}{
		"document.pdf":    {2, "document (2).pdf"},
		"archive.tar.gz":  {2, "archive (2).tar.gz"},
		"my.report.v2.md": {3, "my.report.v2 (3).md"},
		"README":          {2, "README (2)"},
		".bashrc":         {2, ".bashrc (2)"},
	}
	for in, c := range cases {
		if got := conflictName(in, c.n); got != c.want {
			t.Errorf("conflictName(%q, %d) = %q, want %q", in, c.n, got, c.want)
		}
	}
}

// Two different files with one name in one folder: the default policy keeps
// both, the second under a numbered name, and records the decision.
func TestDuplicateNameRenamesByDefaultAndStoresDecision(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Src", "root")
	first := e.fake.File("dup.txt", src.ID, 1)
	second := e.fake.File("dup.txt", src.ID, 2)
	job := e.submitPolicy(src, state.ConflictRename)
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s %+v", j.State, j.LastError)
	}
	root := e.fake.Children(e.dest.ID)[0]
	got := strings.Join(destNames(e, root.ID), ",")
	if !strings.Contains(got, "dup.txt") || !strings.Contains(got, "dup (2).txt") || len(destNames(e, root.ID)) != 2 {
		t.Fatalf("destination names = %s", got)
	}
	var renamed int
	for _, it := range itemsOf(t, e.store, job.CollectionID) {
		if it.SourceID == first.ID || it.SourceID == second.ID {
			if it.ConflictAction == state.DecisionRename {
				renamed++
				if it.DestName != "dup (2).txt" {
					t.Fatalf("stored name = %q", it.DestName)
				}
			}
		}
	}
	if renamed != 1 {
		t.Fatalf("stored rename decisions = %d, want 1 (whichever file claimed second)", renamed)
	}
}

func TestSkipPolicyLeavesTheConflictingFileOut(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Src", "root")
	e.fake.File("dup.txt", src.ID, 1)
	e.fake.File("dup.txt", src.ID, 2)
	job := e.submitPolicy(src, state.ConflictSkip)
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s %+v", j.State, j.LastError)
	}
	root := e.fake.Children(e.dest.ID)[0]
	if n := len(destNames(e, root.ID)); n != 1 {
		t.Fatalf("copies = %d, want 1", n)
	}
	if n := j.Counts.SkippedByCode[state.SkipConflict]; n != 1 {
		t.Fatalf("conflict skips = %d (%+v)", n, j.Counts)
	}
}

// Same bytes already in the folder: a duplicate, not written again, under any policy.
func TestIdenticalContentIsNotCopiedTwice(t *testing.T) {
	for _, policy := range []string{state.ConflictRename, state.ConflictSkip, state.ConflictUpdate} {
		t.Run(policy, func(t *testing.T) {
			e := newEnv(t)
			src := e.fake.Folder("Src", "root")
			a := e.fake.File("same.bin", src.ID, 5)
			b := e.fake.File("same.bin", src.ID, 5)
			e.fake.Mutate(a.ID, func(f *drive.File) { f.MD5 = "abc" })
			e.fake.Mutate(b.ID, func(f *drive.File) { f.MD5 = "abc" })
			job := e.submitPolicy(src, policy)
			j := e.runUntil(job.ID, terminal)
			if j.State != state.JobSucceeded {
				t.Fatalf("job = %s %+v", j.State, j.LastError)
			}
			root := e.fake.Children(e.dest.ID)[0]
			if n := len(destNames(e, root.ID)); n != 1 {
				t.Fatalf("copies = %d, want 1", n)
			}
			if n := e.fake.Calls["copy"]; n != 1 { // the second file is recognised as a duplicate before any write
				t.Fatalf("copy calls = %d, want 1", n)
			}
		})
	}
}

// A same-named folder never causes its subtree to be skipped, even under skip.
func TestFolderNameClashIsRenamedUnderSkipPolicy(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Src", "root")
	d1 := e.fake.Folder("sub", src.ID)
	e.fake.File("one.txt", d1.ID, 1)
	d2 := e.fake.Folder("sub", src.ID)
	e.fake.File("two.txt", d2.ID, 2)
	job := e.submitPolicy(src, state.ConflictSkip)
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s %+v", j.State, j.LastError)
	}
	root := e.fake.Children(e.dest.ID)[0]
	names := strings.Join(destNames(e, root.ID), ",")
	if !strings.Contains(names, "sub") || !strings.Contains(names, "sub (2)") {
		t.Fatalf("folders = %s", names)
	}
	if n := len(e.fake.Children(root.ID)); n != 2 {
		t.Fatalf("children = %d", n)
	}
	for _, f := range e.fake.Children(root.ID) {
		if len(e.fake.Children(f.ID)) != 1 {
			t.Fatalf("subtree of %s lost its file", f.Name)
		}
	}
}

// update keeps both objects under the same name: nothing existing is removed.
func TestUpdatePolicyAddsSameNameWithoutRemovingOthers(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Src", "root")
	e.fake.File("dup.txt", src.ID, 1)
	e.fake.File("dup.txt", src.ID, 2)
	job := e.submitPolicy(src, state.ConflictUpdate)
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s %+v", j.State, j.LastError)
	}
	root := e.fake.Children(e.dest.ID)[0]
	if n := len(destNames(e, root.ID)); n != 2 {
		t.Fatalf("copies = %d, want 2", n)
	}
	for _, c := range e.fake.Children(root.ID) {
		if c.Name != "dup.txt" || c.Trashed {
			t.Fatalf("copy %+v", c)
		}
	}
}

// A same-named folder that 502Drive did not create is never touched: the
// wrapper takes a numbered name instead.
func TestWrapperRenamesAroundForeignFolderAndLeavesItAlone(t *testing.T) {
	e := newEnv(t)
	foreign := e.fake.Folder("Src", e.dest.ID)
	e.fake.File("keep.me", foreign.ID, 3)
	src := e.fake.Folder("Src", "root")
	e.fake.File("new.txt", src.ID, 4)
	job := e.submitPolicy(src, state.ConflictSkip)
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s %+v", j.State, j.LastError)
	}
	names := destNames(e, e.dest.ID)
	if len(names) != 2 || !contains(names, "Src") || !contains(names, "Src (2)") {
		t.Fatalf("destination names = %v, want the foreign Src and a numbered copy", names)
	}
	for _, c := range e.fake.Children(e.dest.ID) {
		if c.Name == "Src" {
			if got := e.fake.Children(c.ID); len(got) != 1 || got[0].Name != "keep.me" {
				t.Fatalf("foreign folder changed: %+v", got)
			}
		}
	}
}

// The wrapper's name check goes to Drive: when the machine is offline during
// that check the job waits and the copy still lands, without a skip.
func TestWrapperNameCheckOfflineIsRetried(t *testing.T) {
	e := newEnv(t)
	e.fake.Folder("Src", e.dest.ID)
	src := e.fake.Folder("Src", "root")
	e.fake.File("new.txt", src.ID, 4)
	var checks int
	e.fake.Before = func(op string, f drive.File) error {
		if op == "find_by_name" {
			checks++
			if checks == 1 {
				return unreachable()
			}
		}
		return nil
	}
	job := e.submitPolicy(src, state.ConflictRename)
	j := e.runUntil(job.ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job = %s %+v", j.State, j.LastError)
	}
	if checks < 2 {
		t.Fatalf("name check ran %d times, want a retry", checks)
	}
	names := destNames(e, e.dest.ID)
	if len(names) != 2 {
		t.Fatalf("names = %v", names)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
