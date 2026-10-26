package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewerComparesNumericallyAndIgnoresDevBuilds(t *testing.T) {
	cases := []struct {
		cur, tag string
		want     bool
	}{
		{"1.2.3", "v1.2.4", true},
		{"1.2.3", "v1.10.0", true}, // numeric, not lexical
		{"1.2.3", "v1.2.3", false},
		{"1.3.0", "v1.2.9", false},
		{"v2.0.0-rc1", "v2.0.1", true},
		{"dev", "v9.9.9", false},
		{"1.2.3", "nightly", false},
	}
	for _, c := range cases {
		if got := Newer(c.cur, c.tag); got != c.want {
			t.Errorf("Newer(%q,%q) = %v, want %v", c.cur, c.tag, got, c.want)
		}
	}
}

func TestLatestReadsTheReleaseAndMapsNotFound(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+Repo+"/releases/latest" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"tag_name":"v1.4.0","html_url":"https://example.test/r"}`))
	}))
	defer ok.Close()
	rel, err := Latest(context.Background(), nil, ok.URL)
	if err != nil || rel.Tag != "v1.4.0" || rel.URL == "" {
		t.Fatalf("release = %+v, %v", rel, err)
	}
	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	if _, err = Latest(context.Background(), nil, missing.URL); err != ErrNoRelease {
		t.Fatalf("404 = %v, want ErrNoRelease", err)
	}
}
