package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive"
)

// auditCollection copies a small tree through the CLI and returns the
// collection id.
func auditCollection(t *testing.T, r *rig) string {
	t.Helper()
	r.setupDest()
	src := r.fake.Folder("Project", "root")
	r.fake.File("a.txt", src.ID, 3)
	res := r.run("copy", src.ID, "--wait", "--quiet")
	if res.code != 0 {
		t.Fatalf("copy: %+v", res)
	}
	res = r.run("collections", "list", "--json")
	items := decodeData(t, res.out)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("collections: %s", res.out)
	}
	return items[0].(map[string]any)["id"].(string)
}

func TestAuditReportsACleanTreeAndARepairPlan(t *testing.T) {
	r := newRig(t)
	col := auditCollection(t, r)

	res := r.run("audit", col, "--json")
	if res.code != 0 {
		t.Fatalf("audit: %+v", res)
	}
	data := lastEnvelope(t, res.out)
	if data["state"] != "completed" || data["scanned_folders"].(float64) < 1 {
		t.Fatalf("audit = %v", data)
	}
	// The repair defaults to a dry run: the plan is recorded, nothing changes.
	writesBefore := r.fake.Writes
	res = r.run("audit", col, "--repair", "--json")
	if res.code != 0 {
		t.Fatalf("repair: %+v", res)
	}
	if r.fake.Writes != writesBefore {
		t.Fatalf("a dry-run repair wrote to Drive: %d -> %d", writesBefore, r.fake.Writes)
	}
}

func TestAuditShowAndKindFilter(t *testing.T) {
	r := newRig(t)
	col := auditCollection(t, r)

	// Break the destination: a.txt disappears.
	cols := r.run("collections", "list", "--json")
	_ = cols
	// find the mapped destination file by walking the fake
	var destFile string
	var walk func(id string)
	walk = func(id string) {
		for _, c := range r.fake.Children(id) {
			if c.Name == "a.txt" {
				destFile = c.ID
			}
			walk(c.ID)
		}
	}
	walk(r.fake.Children("root")[0].ID)
	r.fake.Mutate(destFile, func(f *drive.File) { f.Trashed = true })

	res := r.run("audit", col, "--json")
	if res.code != 0 {
		t.Fatalf("audit: %+v", res)
	}
	auditID := lastEnvelope(t, res.out)["id"].(string)
	if got := lastEnvelope(t, res.out)["counts"].(map[string]any)["missing_at_destination"]; got != float64(1) {
		t.Fatalf("counts = %v", lastEnvelope(t, res.out)["counts"])
	}

	res = r.run("audit", "show", auditID, "--kind", "missing_at_destination", "--json")
	if res.code != 0 || !strings.Contains(res.out, "missing_at_destination") {
		t.Fatalf("show: %+v", res)
	}
	// The composed envelope carries the audit and its findings.
	data := lastEnvelope(t, res.out)
	fs, ok := data["findings"].([]any)
	if !ok || len(fs) != 1 || fs[0].(map[string]any)["kind"] != "missing_at_destination" {
		t.Fatalf("findings in show data = %v", data["findings"])
	}
}

func TestAuditRepairAppliesAndIsRepeatable(t *testing.T) {
	r := newRig(t)
	col := auditCollection(t, r)

	var destFile string
	var walk func(id string)
	walk = func(id string) {
		for _, c := range r.fake.Children(id) {
			if c.Name == "a.txt" {
				destFile = c.ID
			}
			walk(c.ID)
		}
	}
	walk(r.fake.Children("root")[0].ID)
	r.fake.Mutate(destFile, func(f *drive.File) { f.Trashed = true })

	res := r.run("audit", col, "--repair", "--apply", "--json")
	if res.code != 0 {
		t.Fatalf("repair: %+v", res)
	}
	// The repaired finding is marked in the final report.
	if n := len(lastEnvelope(t, res.out)["findings"].([]any)); n != 1 {
		t.Fatalf("findings in report = %v", lastEnvelope(t, res.out)["findings"])
	}
	// The missing file is back (a new object under the same parent).
	live := 0
	var walk2 func(id string)
	walk2 = func(id string) {
		for _, c := range r.fake.Children(id) {
			if c.Name == "a.txt" && !c.Trashed {
				live++
			}
			walk2(c.ID)
		}
	}
	walk2(r.fake.Children("root")[0].ID)
	if live != 1 {
		t.Fatalf("live copies of a.txt: %d", live)
	}

	// A second repair changes nothing.
	writes := r.fake.Writes
	res = r.run("audit", col, "--repair", "--apply", "--json")
	if res.code != 0 {
		t.Fatalf("second repair: %+v", res)
	}
	if r.fake.Writes != writes {
		t.Fatalf("the second repair wrote to Drive: %d -> %d", writes, r.fake.Writes)
	}
}

func TestAuditRejectsUnknownCollectionAndFlagMix(t *testing.T) {
	r := newRig(t)
	if res := r.run("audit", "col_missing", "--json"); res.code != ExitNotFound {
		t.Fatalf("unknown collection exit %d: %s", res.code, res.errOut)
	}
	if res := r.run("audit", "col_x", "--repair", "--apply", "--dry-run"); res.code != ExitUsage {
		t.Fatalf("flag mix exit %d", res.code)
	}
}

// lastEnvelope parses the last JSON envelope line of a multi-envelope --json
// stream (wait prints the final job, the report follows).
func lastEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(out)))
	var last struct {
		Data map[string]any `json:"data"`
	}
	for {
		var m struct {
			Data map[string]any `json:"data"`
		}
		if err := dec.Decode(&m); err != nil {
			if last.Data == nil {
				t.Fatalf("not a JSON envelope: %q", out)
			}
			break
		}
		last = m
	}
	return last.Data
}
