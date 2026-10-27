package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFilterFileReadsEveryRuleAndNumbersErrors(t *testing.T) {
	src := "# comment\n\ninclude docs/**\nexclude *.tmp\next .PDF\nmin-size 10KB\nmax-size 2MB\nmodified-after 2026-01-01T00:00:00Z\nkeep-empty-folders\n"
	s, err := parseFilterFile(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Include) != 2 || s.Include[0] != "docs/**" || s.Include[1] != "ext:PDF" || len(s.Exclude) != 1 {
		t.Fatalf("rules: include=%v exclude=%v", s.Include, s.Exclude)
	}
	if s.MinSize != 10<<10 || s.MaxSize != 2<<20 || s.ModifiedAfter == "" || !s.KeepEmptyFolders {
		t.Fatalf("bounds: %+v", s)
	}
	_, err = parseFilterFile(strings.NewReader("include a\nfrobnicate b\n"))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("unknown rule error = %v", err)
	}
}

func TestFilterFlagsCombineFileAndFlagsAndRejectBadSizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.txt")
	if err := os.WriteFile(path, []byte("exclude *.bak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := filterFlags{file: path, exclude: []string{"*.tmp"}, ext: "pdf, docx", maxSize: "1GB"}
	s, err := f.spec()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Exclude) != 2 || len(s.Include) != 2 || s.Include[0] != "ext:pdf" || s.MaxSize != 1<<30 {
		t.Fatalf("spec = %+v", s)
	}
	bad := filterFlags{minSize: "lots"}
	if _, err := bad.spec(); err == nil {
		t.Fatal("bad size accepted")
	}
	inverted := filterFlags{minSize: "5MB", maxSize: "1MB"}
	if _, err := inverted.spec(); err == nil {
		t.Fatal("min above max accepted")
	}
}
