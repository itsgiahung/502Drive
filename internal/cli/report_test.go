package cli

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Time
		ok   bool
	}{
		{"30d", now.Add(-30 * 24 * time.Hour), true},
		{"12h", now.Add(-12 * time.Hour), true},
		{"2w", now.Add(-14 * 24 * time.Hour), true},
		{"2026-01-02", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), true},
		{"2026-01-02T03:04:05Z", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), true},
		{"", time.Time{}, true},
		{"yesterday", time.Time{}, false},
		{"0d", time.Time{}, false},
		{"30days", time.Time{}, false},
	}
	for _, c := range cases {
		got, err := parseSince(c.in, now)
		if c.ok != (err == nil) {
			t.Fatalf("parseSince(%q) err = %v", c.in, err)
		}
		if c.ok && !got.Equal(c.want) {
			t.Fatalf("parseSince(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCSVFormulaGuardAndQuoting(t *testing.T) {
	var buf bytes.Buffer
	rows := [][]string{
		{"=SUM(A1)", "+1", "-1", "@cmd", "\tTab", "\rCR", "plain"},
		{"a,b", `say "hi"`, "line\nbreak", "", "'already", "=1+1", "ok"},
	}
	if err := writeCSV(&buf, []string{"a", "b", "c", "d", "e", "f", "g"}, rows); err != nil {
		t.Fatal(err)
	}
	read, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v\n%s", err, buf.String())
	}
	if len(read) != 3 {
		t.Fatalf("rows = %d\n%s", len(read), buf.String())
	}
	for i, want := range []string{"'=SUM(A1)", "'+1", "'-1", "'@cmd", "'\tTab", "'\rCR", "plain"} {
		if read[1][i] != want {
			t.Fatalf("guarded cell %d = %q, want %q", i, read[1][i], want)
		}
	}
	if read[2][0] != "a,b" || read[2][1] != `say "hi"` || read[2][2] != "line\nbreak" {
		t.Fatalf("quoting changed data: %#v", read[2])
	}
}

func TestJobsReportListsK1CodesAndFormats(t *testing.T) {
	r := newRig(t)
	src := r.fake.Folder("Reported", "root")
	f := r.fake.File("secret.txt", src.ID, 7)
	r.fake.Mutate(f.ID, func(x *drive.File) { x.CanCopy = false })
	if res := r.run("copy", src.ID, "--wait", "--quiet"); res.code != 0 {
		t.Fatalf("copy: %+v", res)
	}

	res := r.run("jobs", "report", "@last")
	if res.code != 0 || !strings.Contains(res.out, "cannot_copy") || !strings.Contains(res.out, "secret.txt") {
		t.Fatalf("table report = %+v", res)
	}
	res = r.run("jobs", "report", "@last", "--format", "csv")
	if res.code != 0 || !strings.HasPrefix(res.out, "state,code,reason,path,attempts,source_id,destination_id\n") {
		t.Fatalf("csv report = %+v", res)
	}
	read, err := csv.NewReader(strings.NewReader(res.out)).ReadAll()
	if err != nil || len(read) != 2 || read[1][1] != "cannot_copy" {
		t.Fatalf("csv rows = %#v, %v", read, err)
	}
	res = r.run("jobs", "report", "@last", "--format", "json")
	data := decodeData(t, res.out)
	items := data["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["code"] != "cannot_copy" {
		t.Fatalf("json report = %s", res.out)
	}
	if data["truncated"] != false {
		t.Fatalf("truncated = %v", data["truncated"])
	}
	// --json and --format csv must not pretend to be both.
	if res = r.run("jobs", "report", "@last", "--json", "--format", "csv"); res.code != ExitUsage {
		t.Fatalf("conflicting formats = %+v", res)
	}
}

func TestHistoryExportFormatsAndWindow(t *testing.T) {
	r := newRig(t)
	src := r.fake.Folder("Historic", "root")
	r.fake.File("one.txt", src.ID, 3)
	res := r.run("copy", src.ID, "--wait", "--quiet")
	if res.code != 0 {
		t.Fatalf("copy: %+v", res)
	}
	jobID := strings.TrimSpace(res.out)

	res = r.run("history", "export", "--format", "csv")
	if res.code != 0 || !strings.Contains(res.out, "job_id,state,kind") || !strings.Contains(res.out, jobID) {
		t.Fatalf("csv history = %+v", res)
	}
	res = r.run("history", "export", "--format", "json")
	data := decodeData(t, res.out)
	if len(data["items"].([]any)) == 0 {
		t.Fatalf("json history = %s", res.out)
	}
	res = r.run("history", "export", "--since", "2999-01-01T00:00:00Z", "--format", "csv")
	if res.code != 0 || strings.Contains(res.out, jobID) {
		t.Fatalf("future window = %+v", res)
	}
	if res = r.run("history", "export", "--since", "nope"); res.code != ExitUsage {
		t.Fatalf("bad since = %+v", res)
	}
}

func TestReportTruncatesWithNotice(t *testing.T) {
	r := newRig(t)
	src := r.fake.Folder("Many problems", "root")
	for _, name := range []string{"a.txt", "b.txt"} {
		f := r.fake.File(name, src.ID, 3)
		r.fake.Mutate(f.ID, func(x *drive.File) { x.CanCopy = false })
	}
	if res := r.run("copy", src.ID, "--wait", "--quiet"); res.code != 0 {
		t.Fatalf("copy: %+v", res)
	}
	res := r.run("jobs", "report", "@last", "--max-rows", "1", "--format", "csv")
	if res.code != 0 {
		t.Fatalf("report: %+v", res)
	}
	read, err := csv.NewReader(strings.NewReader(res.out)).ReadAll()
	if err != nil || len(read) != 2 {
		t.Fatalf("truncated csv rows = %d, %v", len(read), err)
	}
	if !strings.Contains(res.errOut, "stopped at 1") {
		t.Fatalf("truncation note missing: %q", res.errOut)
	}
}
