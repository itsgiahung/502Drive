package engine

import (
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Unusual shapes of a source tree that real Drive folders contain. Each test
// states what must be true of the copy, not how the engine gets there.

func names(f interface{ Children(string) []drive.File }, parent string) []string {
	var out []string
	for _, c := range f.Children(parent) {
		out = append(out, c.Name)
	}
	return out
}

func TestOddNamesAreCopiedVerbatimAndNeverMerged(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Source", "root")
	odd := []string{
		"A.txt", "a.txt", // differ by case only
		"café.txt", "café.txt", // NFC and NFD spellings of one word
		" lead.txt", "trail.txt ", "dots...", ".hidden", "a/b.txt", "back\\slash.txt",
		"tab\there.txt", strings.Repeat("x", 250) + ".md", "emoji-😀.txt", "עברית.txt", "名前.txt",
	}
	for _, n := range odd {
		e.fake.File(n, src.ID, 3)
	}
	_, root := e.baseline(src)
	got := map[string]int{}
	for _, n := range names(e.fake, root) {
		got[n]++
	}
	for _, n := range odd {
		if got[n] != 1 {
			t.Errorf("name %q was copied %d times, want exactly once (copy has %v)", n, got[n], got)
		}
	}
}

func TestEmptyFilesAndEmptyFoldersAreCopied(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Source", "root")
	e.fake.File("zero.bin", src.ID, 0)
	empty := e.fake.Folder("empty dir", src.ID)
	_ = empty
	deep := src
	for i := 0; i < 30; i++ { // a deep chain of nested folders
		deep = e.fake.Folder("d", deep.ID)
	}
	e.fake.File("leaf.txt", deep.ID, 1)
	_, root := e.baseline(src)
	if len(find(e.fake, root, "zero.bin")) != 1 || len(find(e.fake, root, "empty dir")) != 1 {
		t.Fatalf("zero-byte file or empty folder missing: %v", names(e.fake, root))
	}
	cur := root
	for i := 0; i < 30; i++ {
		kids := find(e.fake, cur, "d")
		if len(kids) != 1 {
			t.Fatalf("level %d missing in a 30-level tree", i)
		}
		cur = kids[0].ID
	}
	if len(find(e.fake, cur, "leaf.txt")) != 1 {
		t.Fatal("the deepest file is missing")
	}
}

// Two byte-identical files with the same name sit in one source folder. One is
// copied and the other is skipped. When the copied one's source is later
// removed while its twin stays, the twin must still be represented in the copy:
// mirror must not trash the only copy of content that still exists at the source.
func TestMirrorKeepsTheCopyWhileAnIdenticalTwinStillExistsAtTheSource(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Source", "root")
	one := e.fake.File("IMG_1.MOV", src.ID, 4)
	two := e.fake.File("IMG_1.MOV", src.ID, 4)
	e.fake.Mutate(two.ID, func(f *drive.File) { f.MD5 = one.MD5 }) // byte-identical
	base, root := e.baseline(src)
	setProtection(t, e, base.CollectionID, true, false)
	copies := find(e.fake, root, "IMG_1.MOV")
	if len(copies) != 1 {
		t.Fatalf("an identical twin is copied once, got %d", len(copies))
	}
	// Remove whichever source file the copy maps to, keep the other.
	items := itemsOf(t, e.store, base.CollectionID)
	var mapped string
	for _, it := range items {
		if it.Name == "IMG_1.MOV" && it.State == state.ItemDone {
			mapped = it.SourceID
		}
	}
	if mapped == "" {
		t.Fatal("no mapped item")
	}
	e.fake.Mutate(mapped, func(f *drive.File) { f.Trashed = true })
	j := e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	if j.State != state.JobSucceeded && j.State != state.JobPartial {
		t.Logf("sync job = %s %+v", j.State, j.LastError)
	}
	live := 0
	for _, c := range find(e.fake, root, "IMG_1.MOV") {
		if !c.Trashed {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("one identical file still exists at the source, so the copy must keep one live file; live copies = %d", live)
	}
}

// Different files that share a name get numbered names once, and repeated
// syncs must not keep creating more of them.
func TestSameNameDifferentContentIsStableAcrossSyncs(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Source", "root")
	e.fake.File("report.pdf", src.ID, 10)
	e.fake.File("report.pdf", src.ID, 20)
	e.fake.File("report.pdf", src.ID, 30)
	base, root := e.baseline(src)
	want := names(e.fake, root)
	if len(want) != 3 {
		t.Fatalf("three different files must give three copies, got %v", want)
	}
	for i := 0; i < 3; i++ {
		e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	}
	if got := names(e.fake, root); len(got) != 3 {
		t.Fatalf("repeated syncs changed the copy: before %v, after %v", want, got)
	}
}

// A file that is trashed at the source and restored later comes back to its
// existing copy; it is not copied a second time.
func TestRestoringATrashedSourceFileDoesNotDuplicateTheCopy(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Source", "root")
	f := e.fake.File("keep.txt", src.ID, 7)
	base, root := e.baseline(src)
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Trashed = true })
	e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	e.fake.Mutate(f.ID, func(x *drive.File) { x.Trashed = false })
	e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	live := 0
	for _, c := range find(e.fake, root, "keep.txt") {
		if !c.Trashed {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("restoring the source file must leave exactly one live copy, got %d (%v)", live, names(e.fake, root))
	}
}

// Both twins disappear from the source: now the copy may be removed by mirror.
func TestMirrorTrashesTheCopyOnceBothIdenticalTwinsAreGone(t *testing.T) {
	e := newEnv(t)
	src := e.fake.Folder("Source", "root")
	one := e.fake.File("IMG_2.MOV", src.ID, 4)
	two := e.fake.File("IMG_2.MOV", src.ID, 4)
	e.fake.Mutate(two.ID, func(f *drive.File) { f.MD5 = one.MD5 })
	base, root := e.baseline(src)
	setProtection(t, e, base.CollectionID, true, false)
	e.fake.Mutate(one.ID, func(f *drive.File) { f.Trashed = true })
	e.fake.Mutate(two.ID, func(f *drive.File) { f.Trashed = true })
	e.runUntil(e.runNow(base.CollectionID).ID, terminal)
	for _, c := range find(e.fake, root, "IMG_2.MOV") {
		if !c.Trashed {
			t.Fatalf("both sources are gone, mirror should have trashed the copy: %+v", c)
		}
	}
}
