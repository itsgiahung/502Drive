package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutostartLinuxEnableDisableIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config")
	exePath := "/usr/local/bin/502drive"

	m := &Manager{
		GOOS:      "linux",
		ConfigDir: configDir,
		ExePath:   exePath,
	}

	// 1. Initial status is off
	on, where := m.Status()
	if on {
		t.Fatalf("expected autostart to be off initially, got on (%s)", where)
	}
	expectedFile := filepath.Join(configDir, "autostart", "502drive.desktop")
	if where != expectedFile {
		t.Fatalf("expected location %q, got %q", expectedFile, where)
	}

	// 2. Enable autostart
	if err := m.Enable(); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}

	on, _ = m.Status()
	if !on {
		t.Fatalf("expected autostart to be enabled, got off")
	}

	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("failed to read desktop file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `Exec="`+exePath+`" up --quiet`) {
		t.Fatalf("desktop file does not contain expected Exec line:\n%s", content)
	}
	if !strings.Contains(content, "Type=Application") || !strings.Contains(content, "X-GNOME-Autostart-enabled=true") {
		t.Fatalf("desktop file missing standard keys:\n%s", content)
	}

	// 3. Second Enable() is idempotent
	if err := m.Enable(); err != nil {
		t.Fatalf("second Enable() should succeed, got: %v", err)
	}
	on, _ = m.Status()
	if !on {
		t.Fatalf("expected autostart to remain enabled")
	}

	// 4. Disable autostart
	if err := m.Disable(); err != nil {
		t.Fatalf("Disable() failed: %v", err)
	}
	on, _ = m.Status()
	if on {
		t.Fatalf("expected autostart to be disabled")
	}
	if _, err := os.Stat(expectedFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("desktop file still exists after Disable()")
	}

	// 5. Second Disable() is idempotent
	if err := m.Disable(); err != nil {
		t.Fatalf("second Disable() should succeed, got: %v", err)
	}
}

func TestAutostartWindowsMockRegistry(t *testing.T) {
	reg := make(map[string]string)
	exePath := `C:\Program Files\502Drive\502drive.exe`

	m := &Manager{
		GOOS:    "windows",
		ExePath: exePath,
		ExecCmd: func(name string, args ...string) ([]byte, error) {
			if name != "reg" {
				return nil, errors.New("unexpected command " + name)
			}
			if len(args) < 2 {
				return nil, errors.New("insufficient args")
			}
			subcmd := args[0]
			key := args[1]
			valName := "502Drive"
			switch subcmd {
			case "query":
				val, ok := reg[key+"\\"+valName]
				if !ok {
					return nil, errors.New("ERROR: The system was unable to find the specified registry key or value.")
				}
				return []byte(val), nil
			case "add":
				// args: reg add KEY /v VALUE /t REG_SZ /d DATA /f
				var data string
				for i := 2; i < len(args); i++ {
					if args[i] == "/d" && i+1 < len(args) {
						data = args[i+1]
						break
					}
				}
				reg[key+"\\"+valName] = data
				return []byte("The operation completed successfully."), nil
			case "delete":
				delete(reg, key+"\\"+valName)
				return []byte("The operation completed successfully."), nil
			default:
				return nil, errors.New("unknown subcmd " + subcmd)
			}
		},
	}

	// 1. Initial status
	on, where := m.Status()
	if on {
		t.Fatalf("expected autostart off initially")
	}
	if !strings.Contains(where, "Run") {
		t.Fatalf("expected registry key in where, got %s", where)
	}

	// 2. Enable
	if err := m.Enable(); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}
	on, _ = m.Status()
	if !on {
		t.Fatalf("expected autostart enabled")
	}
	regVal := reg[WinRunKey+`\502Drive`]
	if regVal != `"`+exePath+`" up --quiet` {
		t.Fatalf("unexpected registry value data: %q", regVal)
	}

	// 3. Disable
	if err := m.Disable(); err != nil {
		t.Fatalf("Disable() failed: %v", err)
	}
	on, _ = m.Status()
	if on {
		t.Fatalf("expected autostart disabled")
	}
}

func TestAutostartDarwinUnsupported(t *testing.T) {
	m := &Manager{
		GOOS: "darwin",
	}
	on, where := m.Status()
	if on || !strings.Contains(where, "unsupported") {
		t.Fatalf("darwin Status unexpected: %v, %s", on, where)
	}
	if err := m.Enable(); err == nil || !strings.Contains(err.Error(), "not supported on macOS") {
		t.Fatalf("darwin Enable expected error, got %v", err)
	}
	if err := m.Disable(); err == nil || !strings.Contains(err.Error(), "not supported on macOS") {
		t.Fatalf("darwin Disable expected error, got %v", err)
	}
}

func TestAutostartToggle(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		GOOS:      "linux",
		ConfigDir: tempDir,
		ExePath:   "/usr/bin/502drive",
	}

	// Initially off -> toggle enables
	nowOn, err := m.Toggle()
	if err != nil || !nowOn {
		t.Fatalf("first toggle should enable: %v, %v", nowOn, err)
	}
	if on, _ := m.Status(); !on {
		t.Fatalf("status should be on")
	}

	// Toggle disables
	nowOn, err = m.Toggle()
	if err != nil || nowOn {
		t.Fatalf("second toggle should disable: %v, %v", nowOn, err)
	}
	if on, _ := m.Status(); on {
		t.Fatalf("status should be off")
	}
}

func TestInstallerIssMatchesAutostartMechanism(t *testing.T) {
	// Look for packaging/windows/502drive.iss relative to repo root
	paths := []string{
		"../../packaging/windows/502drive.iss",
		"packaging/windows/502drive.iss",
	}
	var data []byte
	var err error
	for _, p := range paths {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("could not read 502drive.iss: %v", err)
	}

	content := string(data)
	// Check that the Run key sets 502drive.exe up --quiet
	want := `ValueData: """{app}\{#MyAppExeName}"" up --quiet"`
	if !strings.Contains(content, want) {
		t.Fatalf("packaging/windows/502drive.iss does not match unified autostart mechanism.\nWanted snippet:\n%s\nIn file:\n%s", want, content)
	}
}
