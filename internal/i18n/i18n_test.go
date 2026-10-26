package i18n

import "testing"

func TestBothLanguagesDefineEveryKey(t *testing.T) {
	for key, value := range VI {
		if value == "" {
			t.Errorf("vi:%s is empty", key)
		}
		if _, ok := EN[key]; !ok {
			t.Errorf("en is missing %s", key)
		}
	}
	for key, value := range EN {
		if value == "" {
			t.Errorf("en:%s is empty", key)
		}
		if _, ok := VI[key]; !ok {
			t.Errorf("vi is missing %s", key)
		}
	}
	if len(VI) != len(EN) {
		t.Fatalf("table sizes differ: vi=%d en=%d", len(VI), len(EN))
	}
	if len(Keys()) != len(EN) {
		t.Fatalf("Keys() = %d, table = %d", len(Keys()), len(EN))
	}
}

func TestFormatsStayUsable(t *testing.T) {
	for key, value := range VI {
		want := countVerbs(value)
		got := countVerbs(EN[key])
		if want != got {
			t.Errorf("%s has %d verbs in vi but %d in en", key, want, got)
		}
	}
}

// countVerbs counts printf verbs so the two tables stay interchangeable.
func countVerbs(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+1 < len(s) && s[i+1] != '%' {
			n++
		}
	}
	return n
}

func TestLookupFallbacks(t *testing.T) {
	if T("vi", Jobs) != VI[Jobs] {
		t.Fatalf("vi lookup = %q", T("vi", Jobs))
	}
	if T("en", Jobs) != EN[Jobs] {
		t.Fatalf("en lookup = %q", T("en", Jobs))
	}
	if T("fr", Jobs) != EN[Jobs] {
		t.Fatalf("unknown language did not fall back to English: %q", T("fr", Jobs))
	}
	if T("vi", Key("missing")) != "missing" {
		t.Fatalf("missing key = %q", T("vi", Key("missing")))
	}
	if Tf("vi", StatusLine, "x", 1, 2, 3) == "" {
		t.Fatal("formatting an empty result")
	}
	if len(HelpLines("vi")) != len(HelpLines("en")) {
		t.Fatal("help lines differ between languages")
	}
	if HelpLines("vi")[0] == HelpLines("en")[0] {
		t.Fatal("help lines are not translated")
	}
}
