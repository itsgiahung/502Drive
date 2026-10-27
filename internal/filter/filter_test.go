package filter

import (
	"testing"
	"time"
)

func sz(n int64) *int64 { return &n }

func TestGlobByNameAndPath(t *testing.T) {
	cases := []struct {
		rule string
		e    Entry
		keep bool
	}{
		{"*.tmp", Entry{Name: "x.tmp", Rel: "a/b/x.tmp"}, false}, // a name pattern matches at any depth
		{"*.tmp", Entry{Name: "x.txt", Rel: "a/x.txt"}, true},
		{"docs/**", Entry{Name: "b.txt", Rel: "docs/a/b.txt"}, false},    // ** spans segments
		{"docs/**", Entry{Name: "b.txt", Rel: "other/docs/b.txt"}, true}, // anchored to the root
		{"**/cache/*", Entry{Name: "x", Rel: "p/q/cache/x"}, false},
		{"**/cache/*", Entry{Name: "x", Rel: "p/q/cache/y/x"}, true}, // one segment only
		{"báo cáo*.pdf", Entry{Name: "báo cáo 2026.pdf", Rel: "báo cáo 2026.pdf"}, false},
	}
	for _, c := range cases {
		m, err := Spec{Exclude: []string{c.rule}}.Compile()
		if err != nil {
			t.Fatalf("compile %q: %v", c.rule, err)
		}
		if got := m.Decide(c.e); got != c.keep {
			t.Errorf("exclude %q on %+v: keep=%v, want %v", c.rule, c.e, got, c.keep)
		}
	}
}

func TestExcludeWinsOverInclude(t *testing.T) {
	m, err := Spec{Include: []string{"*.pdf"}, Exclude: []string{"secret*"}}.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if m.Decide(Entry{Name: "secret.pdf", Rel: "secret.pdf", Size: sz(1)}) {
		t.Fatal("exclude must win over include")
	}
	if !m.Decide(Entry{Name: "open.pdf", Rel: "open.pdf", Size: sz(1)}) {
		t.Fatal("included file dropped")
	}
	if m.Decide(Entry{Name: "open.txt", Rel: "open.txt", Size: sz(1)}) {
		t.Fatal("file matching no include rule was kept")
	}
}

func TestExtensionAndClassRules(t *testing.T) {
	m, err := Spec{Include: []string{"ext:.PDF", "class:image"}}.Compile()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		mime string
		keep bool
	}{
		"a.pdf":  {"application/pdf", true},
		"b.PNG":  {"image/png", true},
		"c.mp4":  {"video/mp4", false},
		"d.docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", false},
	}
	for name, c := range cases {
		if got := m.Decide(Entry{Name: name, Rel: name, MimeType: c.mime, Size: sz(3)}); got != c.keep {
			t.Errorf("%s: keep=%v want %v", name, got, c.keep)
		}
	}
	ex, err := Spec{Exclude: []string{"class:video", "class:native"}}.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if ex.Decide(Entry{Name: "clip", Rel: "clip", MimeType: "video/mp4", Size: sz(1)}) {
		t.Fatal("video class not excluded")
	}
	if ex.Decide(Entry{Name: "Notes", Rel: "Notes", MimeType: "application/vnd.google-apps.document"}) {
		t.Fatal("native class not excluded")
	}
}

func TestFoldersOnlyPrunedByGlobExcludes(t *testing.T) {
	m, err := Spec{Include: []string{"*.pdf"}, Exclude: []string{"ext:.pdf", "class:video", "tmp"}}.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if !m.Decide(Entry{Name: "reports", Rel: "reports", IsFolder: true}) {
		t.Fatal("include rule or ext/class must not prune a folder")
	}
	if m.Decide(Entry{Name: "tmp", Rel: "tmp", IsFolder: true}) {
		t.Fatal("glob exclude must prune the folder subtree")
	}
}

func TestSizeAndModifiedBounds(t *testing.T) {
	m, err := Spec{MinSize: 10, MaxSize: 100, ModifiedAfter: "2026-01-01T00:00:00Z"}.Compile()
	if err != nil {
		t.Fatal(err)
	}
	old := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		e    Entry
		keep bool
	}{
		{"small", Entry{Name: "s", Size: sz(9), Modified: recent}, false},
		{"edge-min", Entry{Name: "s", Size: sz(10), Modified: recent}, true},
		{"big", Entry{Name: "b", Size: sz(101), Modified: recent}, false},
		{"old", Entry{Name: "o", Size: sz(50), Modified: old}, false},
		{"unknown-time", Entry{Name: "u", Size: sz(50)}, true},
		{"native-no-size", Entry{Name: "doc", MimeType: "application/vnd.google-apps.document"}, true},
	}
	for _, c := range cases {
		if got := m.Decide(c.e); got != c.keep {
			t.Errorf("%s: keep=%v want %v", c.name, got, c.keep)
		}
	}
}

func TestInvalidSpecsAreRejected(t *testing.T) {
	bad := []Spec{
		{Include: []string{""}},
		{Exclude: []string{"["}},
		{Include: []string{"class:movies"}},
		{Include: []string{"ext:"}},
		{MinSize: -1},
		{MinSize: 10, MaxSize: 5},
		{ModifiedAfter: "yesterday"},
	}
	for i, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("case %d accepted: %+v", i, s)
		}
	}
	valid := Spec{Include: []string{"**/*.md"}, Exclude: []string{"class:archive"}, MinSize: 1, MaxSize: 2, ModifiedAfter: "2026-01-02T03:04:05Z"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
}

func TestActiveIgnoresKeepEmptyFolders(t *testing.T) {
	if (Spec{}).Active() || (Spec{KeepEmptyFolders: true}).Active() {
		t.Fatal("a spec without rules must not be active")
	}
	if !(Spec{Exclude: []string{"x"}}).Active() {
		t.Fatal("an exclude rule must be active")
	}
}

func TestMimeClassAndParseSize(t *testing.T) {
	if MimeClass("application/zip", false) != "archive" || MimeClass("text/csv", false) != "doc" || MimeClass("", true) != "other" {
		t.Fatal("mime classes")
	}
	for in, want := range map[string]int64{"1024": 1024, "10KB": 10 << 10, "2mb": 2 << 20, "1 GB": 1 << 30} {
		got, err := ParseSize(in)
		if err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "x", "-1MB", "1.5GB"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) accepted", in)
		}
	}
}
