package cli

import (
	"strings"
	"testing"
)

func TestHistoryClearPreviewsAsksAndKeepsTheCopy(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("History src", "root")
	r.fake.File("a.txt", src.ID, 10)
	if res := r.run("copy", src.ID, "--wait", "--quiet"); res.code != 0 {
		t.Fatalf("copy = %+v", res)
	}

	if res := r.run("history", "clear"); res.code == 0 || !strings.Contains(res.errOut, "exactly one") {
		t.Fatalf("a scope is required: %+v", res)
	}
	if res := r.run("history", "clear", "--older-than", "0d"); res.code == 0 {
		t.Fatalf("0d must be refused: %+v", res)
	}
	res := r.run("history", "clear", "--all-finished", "--dry-run")
	if res.code != 0 || !strings.Contains(res.out, "Would clear 1 finished") {
		t.Fatalf("preview = %+v", res)
	}
	if res = r.run("jobs", "list", "--json"); !strings.Contains(res.out, "History src") {
		t.Fatalf("a preview must not hide anything: %+v", res)
	}
	// No terminal and no --yes: refuse instead of clearing.
	if res = r.run("history", "clear", "--all-finished"); res.code == 0 || !strings.Contains(res.errOut, "--yes") {
		t.Fatalf("scripts need --yes: %+v", res)
	}
	res = r.run("history", "clear", "--all-finished", "--yes")
	if res.code != 0 || !strings.Contains(res.out, "Cleared 1 finished") || !strings.Contains(res.out, "not touched") {
		t.Fatalf("clear = %+v", res)
	}
	if res = r.run("jobs", "list", "--json"); strings.Contains(res.out, "History src") {
		t.Fatalf("the job should be gone from the list: %s", res.out)
	}
	if res = r.run("collections", "list", "--json"); !strings.Contains(res.out, "History src") {
		t.Fatalf("the copy must stay tracked: %s", res.out)
	}
	if res = r.run("history", "clear", "--all-finished", "--yes"); !strings.Contains(res.out, "Nothing to clear") {
		t.Fatalf("second clear = %+v", res)
	}
}
