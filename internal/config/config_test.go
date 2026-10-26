package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupIsPrivateAndNeverOverwritesIdentity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private", "config.json")
	c, e := Setup(p)
	if e != nil {
		t.Fatal(e)
	}
	first, e := os.ReadFile(c.TokenFile)
	if e != nil {
		t.Fatal(e)
	}
	if len(strings.TrimSpace(string(first))) < 40 {
		t.Fatal("weak token")
	}
	if _, e = Setup(p); e == nil {
		t.Fatal("setup overwrote existing identity")
	}
	got, e := Load(p)
	if e != nil || got.DataDir != c.DataDir {
		t.Fatal(got, e)
	}
	for _, f := range []string{p, c.TokenFile} {
		i, e := os.Stat(f)
		if e != nil || i.Mode().Perm() != 0600 {
			t.Fatal("permissions", f, e)
		}
	}
}
func TestRejectPublicListenerAndUnknownConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if e := os.WriteFile(p, []byte(`{"listen":"0.0.0.0:5020","data_dir":"x","token_file":"y"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(p); e == nil {
		t.Fatal("public bind allowed")
	}
	if e := os.WriteFile(p, []byte(`{"listen":"127.0.0.1:5020","typo":true}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(p); e == nil {
		t.Fatal("unknown config accepted")
	}
}
