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
func TestContainerOptInAllowsBindingOwnInterfaces(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	write := func(body string) error {
		if e := os.WriteFile(p, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
		_, e := Load(p)
		return e
	}
	ok := `{"listen":"0.0.0.0:5020","container":true,"data_dir":"` + filepath.Dir(p) + `/data","token_file":"` + filepath.Dir(p) + `/token"}`
	if e := write(ok); e != nil {
		t.Fatalf("container listen rejected: %v", e)
	}
	if e := write(`{"listen":"0.0.0.0:5020","data_dir":"x","token_file":"y"}`); e == nil {
		t.Fatal("public bind accepted without the container opt-in")
	}
	if e := write(`{"listen":"192.0.2.10:5020","container":true,"data_dir":"x","token_file":"y"}`); e == nil {
		t.Fatal("a specific non-loopback address must stay rejected")
	}
	if e := write(`{"listen":"[::]:5020","container":true,"data_dir":"x","token_file":"y"}`); e != nil {
		t.Fatalf("ipv6 unspecified rejected: %v", e)
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

func TestTrayQuitStopsDaemonOptionRoundTrips(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	body := `{"listen":"127.0.0.1:5020","data_dir":"` + filepath.Dir(p) + `/data","token_file":"` + filepath.Dir(p) + `/token","tray_quit_stops_daemon":true}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.TrayQuitStopsDaemon {
		t.Fatal("tray_quit_stops_daemon was dropped")
	}
	if err = Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(p)
	if err != nil || !cfg.TrayQuitStopsDaemon {
		t.Fatalf("reload = %v / %v", err, cfg.TrayQuitStopsDaemon)
	}
	// The default stays off: quitting the tray never stops the daemon.
	def := filepath.Join(t.TempDir(), "config.json")
	body = `{"listen":"127.0.0.1:5020","data_dir":"` + filepath.Dir(def) + `/data","token_file":"` + filepath.Dir(def) + `/token"}`
	if err = os.WriteFile(def, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err = Load(def); err != nil || cfg.TrayQuitStopsDaemon {
		t.Fatalf("default = %v / %v", err, cfg.TrayQuitStopsDaemon)
	}
}

func TestActiveJobsDefaultsToTwoAndValidatesRange(t *testing.T) {
	var c Config
	if got := c.ActiveJobs(); got != 2 {
		t.Fatalf("default active jobs = %d", got)
	}
	c.MaxActiveJobs = 5
	if got := c.ActiveJobs(); got != 5 {
		t.Fatalf("configured active jobs = %d", got)
	}
	c.Listen = "127.0.0.1:5020"
	c.MaxActiveJobs = 33
	if err := c.Validate(); err == nil {
		t.Fatal("max_active_jobs above the limit accepted")
	}
	c.MaxActiveJobs = -1
	if err := c.Validate(); err == nil {
		t.Fatal("negative max_active_jobs accepted")
	}
}

// A fresh install uses "502drive"; an existing pre-0.3 setup in "502drive-next"
// keeps being found (its config.json holds absolute paths inside that folder),
// and once "502drive" has a config it wins.
func TestDefaultPathKeepsALegacySetup(t *testing.T) {
	base := t.TempDir()
	fresh := filepath.Join(base, AppDirName, "config.json")
	if got := defaultPathIn(base); got != fresh {
		t.Fatalf("fresh install = %s, want %s", got, fresh)
	}
	legacy := filepath.Join(base, LegacyAppDirName, "config.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := defaultPathIn(base); got != legacy {
		t.Fatalf("legacy setup = %s, want %s", got, legacy)
	}
	if !IsLegacyDir(filepath.Dir(legacy)) || IsLegacyDir(filepath.Dir(fresh)) {
		t.Fatal("IsLegacyDir misclassifies")
	}
	if err := os.MkdirAll(filepath.Dir(fresh), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fresh, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := defaultPathIn(base); got != fresh {
		t.Fatalf("both present = %s, want the new one", got)
	}
}
