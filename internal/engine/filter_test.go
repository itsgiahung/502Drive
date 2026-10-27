package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/itsgiahung/502Drive/internal/state"
)

// submitFiltered admits a folder copy with a selective-sync rule.
func (e *env) submitFiltered(src drive.File, spec filter.Spec) state.Job {
	e.t.Helper()
	e.n++
	p := state.Plan{ID: fmt.Sprintf("plan%d", e.n), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder, SourceMime: src.MimeType,
		SourceVersion: src.Fingerprint(), DestinationID: "dest", DestinationFolder: e.dest.ID, RootName: src.Name, Filter: spec, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := e.store.PutPlan(context.Background(), p); err != nil {
		e.t.Fatal(err)
	}
	r, err := e.store.Admit(context.Background(), "owner", fmt.Sprintf("key%d", e.n), p.ID, 1)
	if err != nil {
		e.t.Fatal(err)
	}
	return r.Job
}

// flat renders a destination folder as sorted names with "/" for folders, one
// path per line, so tests can compare whole shapes.
func flat(e *env, id string) string {
	var out []string
	for _, l := range tree(e.fake, id, "") {
		if i := strings.Index(l, "|"); i >= 0 {
			l = l[:i]
		}
		out = append(out, strings.TrimSuffix(l, " ->shortcut"))
	}
	return strings.Join(out, " ")
}

func TestFilterDropsExcludedFilesBeforeTheyAreCopied(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Proj", "root")
	e.fake.File("a.pdf", src.ID, 1)
	e.fake.File("junk.tmp", src.ID, 2)
	sub := e.fake.Folder("sub", src.ID)
	e.fake.File("b.tmp", sub.ID, 3)
	e.fake.File("c.txt", sub.ID, 4)
	copiesBefore := e.fake.Calls["copy"]

	j := e.runUntil(e.submitFiltered(src, filter.Spec{Exclude: []string{"*.tmp"}}).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job %s %+v", j.State, j.LastError)
	}
	root := e.fake.Children(e.dest.ID)[0]
	if got := flat(e, root.ID); got != "/a.pdf /sub/ /sub/c.txt" {
		t.Fatalf("destination = %q", got)
	}
	if n := e.fake.Calls["copy"] - copiesBefore; n != 2 {
		t.Fatalf("copy calls = %d, want 2: filtered files must never be written", n)
	}
	if n := j.Counts.SkippedByCode[state.SkipFiltered]; n != 2 {
		t.Fatalf("filtered = %d (%+v)", n, j.Counts)
	}
}

func TestFilteredFoldersAreNotCreatedUnlessKept(t *testing.T) {
	build := func(e *env) drive.File {
		src := e.fake.Folder("Proj", "root")
		keep := e.fake.Folder("keep", src.ID)
		e.fake.File("x.pdf", keep.ID, 1)
		junk := e.fake.Folder("junk", src.ID)
		e.fake.File("y.tmp", junk.ID, 2)
		deep := e.fake.Folder("deep", src.ID)
		chain := e.fake.Folder("a", deep.ID)
		chain = e.fake.Folder("b", chain.ID)
		e.fake.File("w.tmp", chain.ID, 3)
		e.fake.Folder("empty", src.ID)
		return src
	}
	t.Run("pruned", func(t *testing.T) {
		e := newEnv(t)
		src := build(e)
		j := e.runUntil(e.submitFiltered(src, filter.Spec{Exclude: []string{"*.tmp"}}).ID, terminal)
		if j.State != state.JobSucceeded {
			t.Fatalf("job %s %+v", j.State, j.LastError)
		}
		root := e.fake.Children(e.dest.ID)[0]
		if got := flat(e, root.ID); got != "/keep/ /keep/x.pdf" {
			t.Fatalf("destination = %q, want only the kept folder and its file", got)
		}
	})
	t.Run("keep_empty_folders", func(t *testing.T) {
		e := newEnv(t)
		src := build(e)
		j := e.runUntil(e.submitFiltered(src, filter.Spec{Exclude: []string{"*.tmp"}, KeepEmptyFolders: true}).ID, terminal)
		if j.State != state.JobSucceeded {
			t.Fatalf("job %s %+v", j.State, j.LastError)
		}
		root := e.fake.Children(e.dest.ID)[0]
		got := flat(e, root.ID)
		for _, want := range []string{"/junk/", "/deep/", "/empty/", "/keep/x.pdf"} {
			if !strings.Contains(got, want) {
				t.Fatalf("keep_empty_folders dropped %s: %q", want, got)
			}
		}
	})
}

func TestGlobExcludeOfAFolderSkipsTheWholeSubtree(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Proj", "root")
	e.fake.File("top.pdf", src.ID, 1)
	cache := e.fake.Folder("cache", src.ID)
	e.fake.File("hot.bin", cache.ID, 2)
	nested := e.fake.Folder("nest", cache.ID)
	e.fake.File("deeper.bin", nested.ID, 3)
	j := e.runUntil(e.submitFiltered(src, filter.Spec{Exclude: []string{"cache"}}).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job %s %+v", j.State, j.LastError)
	}
	root := e.fake.Children(e.dest.ID)[0]
	if got := flat(e, root.ID); got != "/top.pdf" {
		t.Fatalf("destination = %q", got)
	}
	if n := j.Counts.SkippedByCode[state.SkipFiltered]; n != 1 {
		t.Fatalf("filtered = %d; the excluded folder is one skipped item and its children are never read (%+v)", n, j.Counts)
	}
}

func TestPathRulesApplyBelowTheRootAndAreStored(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Proj", "root")
	sub := e.fake.Folder("sub", src.ID)
	deep := e.fake.Folder("deep", sub.ID)
	e.fake.File("d.txt", deep.ID, 1)
	e.fake.File("s.txt", sub.ID, 2)
	e.fake.File("top.txt", src.ID, 3)
	j := e.runUntil(e.submitFiltered(src, filter.Spec{Exclude: []string{"sub/deep/**"}}).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("job %s %+v", j.State, j.LastError)
	}
	root := e.fake.Children(e.dest.ID)[0]
	if got := flat(e, root.ID); got != "/sub/ /sub/s.txt /top.txt" {
		t.Fatalf("destination = %q", got)
	}
	// "sub/deep/**" also matches the folder itself, so the folder is excluded
	// and nothing below it is listed: d.txt has no item at all.
	rel := map[string]string{}
	for _, it := range itemsOf(t, e.store, j.CollectionID) {
		rel[it.Name] = it.RelPath
	}
	if _, listed := rel["d.txt"]; listed {
		t.Fatal("a file below an excluded folder was listed")
	}
	if rel["s.txt"] != "sub/s.txt" || rel["top.txt"] != "top.txt" || rel["deep"] != "sub/deep" {
		t.Fatalf("stored relative paths = %v", rel)
	}
}

// A filter change never deletes what was copied, stops new excluded objects,
// and copies an object that an earlier filter had kept out once it is allowed.
func TestFilterChangeKeepsCopiesAndAppliesToLaterRuns(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Proj", "root")
	e.fake.File("a.pdf", src.ID, 1)
	e.fake.File("b.tmp", src.ID, 2)
	base, root := e.baseline(src)
	ctx := context.Background()

	c := mustColl(e, base.CollectionID)
	if _, err := e.store.SetCollectionFilter(ctx, "owner", c.ID, filter.Spec{Exclude: []string{"*.tmp"}}, c.Revision); err != nil {
		t.Fatal(err)
	}
	e.fake.File("c.tmp", src.ID, 3)
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("run %s %+v", j.State, j.LastError)
	}
	if got := flat(e, *mustColl(e, base.CollectionID).RootID); !strings.Contains(got, "b.tmp") || strings.Contains(got, "c.tmp") {
		t.Fatalf("after excluding *.tmp: %q (b.tmp must stay, c.tmp must not be copied)", got)
	}

	// Clearing the filter: the object the filter kept out is copied now.
	c = mustColl(e, base.CollectionID)
	if _, err := e.store.SetCollectionFilter(ctx, "owner", c.ID, filter.Spec{}, c.Revision); err != nil {
		t.Fatal(err)
	}
	j = e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("run %s %+v", j.State, j.LastError)
	}
	if got := flat(e, *mustColl(e, base.CollectionID).RootID); !strings.Contains(got, "c.tmp") || !strings.Contains(got, "b.tmp") {
		t.Fatalf("after clearing the filter: %q", got)
	}
	_ = root
}

func TestSetCollectionFilterChecksRevision(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Proj", "root")
	e.fake.File("a", src.ID, 1)
	base, _ := e.baseline(src)
	c := mustColl(e, base.CollectionID)
	if _, err := e.store.SetCollectionFilter(context.Background(), "owner", c.ID, filter.Spec{Exclude: []string{"x"}}, c.Revision+7); err != state.ErrStale {
		t.Fatalf("stale revision accepted: %v", err)
	}
}

// Renaming a folder in the source moves the stored path of everything below
// it, so later path rules see the current names.
func TestRenamedFolderUpdatesStoredPathsBelow(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Proj", "root")
	dir := e.fake.Folder("dirA", src.ID)
	inner := e.fake.Folder("inner", dir.ID)
	e.fake.File("f.txt", inner.ID, 1)
	base, _ := e.baseline(src)
	e.fake.Mutate(dir.ID, func(x *drive.File) { x.Name = "dirB" })
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded {
		t.Fatalf("run %s %+v", j.State, j.LastError)
	}
	rel := map[string]string{}
	for _, it := range itemsOf(t, e.store, base.CollectionID) {
		rel[it.Name] = it.RelPath
	}
	if rel["dirA"] != "dirB" && rel["dirB"] != "dirB" {
		t.Fatalf("folder path = %v", rel)
	}
	if rel["f.txt"] != "dirB/inner/f.txt" || rel["inner"] != "dirB/inner" {
		t.Fatalf("paths below the renamed folder = %v", rel)
	}
}
