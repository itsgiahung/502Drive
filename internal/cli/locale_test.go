package cli

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
)

func (r *rig) client() *Client {
	r.t.Helper()
	token, err := os.ReadFile(filepath.Join(r.home, "api-token"))
	if err != nil {
		r.t.Fatal(err)
	}
	return &Client{Endpoint: r.addr, Token: strings.TrimSpace(string(token))}
}

func TestHumanDatesUsePreferenceLocaleAndTimezone(t *testing.T) {
	r := newRig(t)
	folder := r.fake.Folder("Dated", "root")
	f := r.fake.File("report.txt", folder.ID, 2048)
	at := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	r.fake.Mutate(f.ID, func(x *drive.File) { x.Modified, x.Created = at, at })

	client := r.client()
	ctx := context.Background()
	if _, err := client.Do(ctx, "PATCH", "/v1/preferences", nil, map[string]any{"language": "vi", "timezone": "Asia/Ho_Chi_Minh"}, NewKey()); err != nil {
		t.Fatal(err)
	}
	res := r.run("ls", folder.ID)
	// 20:00Z is 03:00 next day in Ho Chi Minh City; Vietnamese dates are dd/mm/yyyy.
	if res.code != 0 || !strings.Contains(res.out, "02/01/2026") || !strings.Contains(res.out, "2.0 KB") {
		t.Fatalf("vi ls = %+v", res)
	}
	// JSON keeps the raw UTC timestamp and is not localized.
	js := r.run("info", f.ID, "--json")
	if js.code != 0 || !strings.Contains(js.out, "2026-01-01T20:00:00Z") {
		t.Fatalf("json info = %+v", js)
	}
	// English + UTC renders the same instant as an ISO date.
	if _, err := client.Do(ctx, "PATCH", "/v1/preferences", nil, map[string]any{"language": "en", "timezone": "UTC"}, NewKey()); err != nil {
		t.Fatal(err)
	}
	res = r.run("ls", folder.ID)
	if res.code != 0 || !strings.Contains(res.out, "2026-01-01") {
		t.Fatalf("en ls = %+v", res)
	}
}

var idPattern = regexp.MustCompile(`job_[0-9a-f]{32}|[0-9]{4}-[0-9]{2}-[0-9]{2}[ T][0-9:.]+Z?`)

// TestReportHumanSnapshot pins the human report table. JSON is covered by the
// existing envelope tests and must not change with localization.
func TestReportHumanSnapshot(t *testing.T) {
	var out strings.Builder
	a := &App{io: IO{Out: &out, Width: 100}, style: ResolveStyle(StyleInput{Width: 100, Plain: true})}
	a.printReport(jobView{ID: "job_fixed", State: "partial"}, []problemItem{
		{State: "failed", Code: "unreadable", Path: "Docs/a.pdf", Attempts: 3},
		{State: "skipped", Code: "filtered", Path: "Media/b.mp4", Attempts: 0},
	}, false)
	got := idPattern.ReplaceAllString(strings.TrimSpace(out.String()), "<id>")
	want, err := os.ReadFile(filepath.Join("testdata", "human", "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.TrimSpace(string(want)) {
		t.Fatalf("report snapshot changed:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
