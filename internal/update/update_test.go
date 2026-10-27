package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestPickAssetIgnoresMalformedLinesAndOtherPlatforms(t *testing.T) {
	good := strings.Repeat("a", 64)
	sums := []byte("not-a-hash  502drive_1.0.0_linux_amd64.tar.gz\n" +
		good + "  502drive_1.0.0_darwin_arm64.tar.gz\n" +
		good + "  ../evil_linux_amd64.tar.gz\n" +
		good + "  502drive_1.0.0_linux_amd64.tar.gz\n" +
		good + "  502drive_1.0.0_windows_amd64.zip\n")
	a, err := PickAsset(sums, "linux", "amd64")
	if err != nil || a.Name != "502drive_1.0.0_linux_amd64.tar.gz" || a.SHA256 != good {
		t.Fatalf("linux asset = %+v %v", a, err)
	}
	if a, err = PickAsset(sums, "windows", "amd64"); err != nil || !strings.HasSuffix(a.Name, ".zip") {
		t.Fatalf("windows asset = %+v %v", a, err)
	}
	if _, err = PickAsset(sums, "linux", "riscv64"); err == nil {
		t.Fatal("a missing platform must be an error")
	}
	if err = Verify([]byte("x"), good); err == nil {
		t.Fatal("a wrong digest verified")
	}
}

func TestExtractReadsOnlyTheNamedTopLevelFile(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"../502drive": "evil", "README.md": "doc", "502drive": "bin"} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	files, err := Extract(buf.Bytes(), false, "502drive")
	if err != nil || string(files["502drive"]) != "bin" || len(files) != 1 {
		t.Fatalf("files = %v %v", files, err)
	}
	if _, err = Extract(buf.Bytes(), false, "502drive-tray"); err == nil {
		t.Fatal("a missing file must be an error")
	}
}
