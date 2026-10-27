package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdUnitTextIsUserScopedAndHardened(t *testing.T) {
	home, errHome := os.UserHomeDir()
	if errHome != nil {
		t.Skip("no home directory")
	}
	text := SystemdUnitText("/usr/bin/502drive", home+"/.local/share/502drive", home+"/.config/502drive-next")
	for _, want := range []string{
		"ExecStart=/usr/bin/502drive serve",
		"Restart=on-failure",
		"StartLimitBurst=5",
		"ReadWritePaths=%h/.config/502drive-next %h/.local/share/502drive",
		"[Install]\nWantedBy=default.target",
		"UMask=0077",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("unit missing %q:\n%s", want, text)
		}
	}
	// The package unit must stay opt-in; the user unit is the one with Install.
	pkg, err := os.ReadFile(filepath.Join("..", "..", "packaging", "502drive.service"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(pkg), "\n[Install]\n") {
		t.Fatal("packaging unit must stay opt-in")
	}
}

func TestSystemdInstallIsIdempotentAndStatusReportsEnabled(t *testing.T) {
	config := t.TempDir()
	enabled := false
	m := &Manager{GOOS: "linux", ConfigDir: config, ExePath: "/opt/502Drive/502drive",
		ExecCmd: func(name string, args ...string) ([]byte, error) {
			if name != "systemctl" || len(args) < 2 || args[0] != "--user" {
				return nil, errors.New("unexpected command")
			}
			switch args[1] {
			case "daemon-reload":
				return nil, nil
			case "enable":
				enabled = true
				return nil, nil
			case "is-enabled":
				if enabled {
					return []byte("enabled\n"), nil
				}
				return []byte("disabled\n"), errors.New("exit status 1")
			}
			return nil, errors.New("unknown systemctl verb")
		}}

	if installed, on, _ := m.SystemdStatus(); installed || on {
		t.Fatal("status before install")
	}
	path, err := m.InstallSystemd("/home/u/data", "/home/u/config", true)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(config, "systemd", "user", SystemdUnitName)
	if path != want {
		t.Fatalf("unit path = %s, want %s", path, want)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("unit mode = %04o", fi.Mode().Perm())
	}
	installed, on, detail := m.SystemdStatus()
	if !installed || !on || detail != "enabled" {
		t.Fatalf("status after install = %v %v %q", installed, on, detail)
	}
	if _, err = m.InstallSystemd("/home/u/data", "/home/u/config", true); err != nil {
		t.Fatalf("second install: %v", err)
	}
	// A disabled unit is installed but not enabled: doctor treats it as a problem.
	enabled = false
	if installed, on, detail = m.SystemdStatus(); !installed || on || detail != "disabled" {
		t.Fatalf("disabled status = %v %v %q", installed, on, detail)
	}
	if err = m.DisableSystemd(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unit file still exists after disable")
	}
	if err = m.DisableSystemd(); err != nil {
		t.Fatalf("second disable: %v", err)
	}
}
