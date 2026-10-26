package tray

import (
	"errors"
	"path/filepath"
	"testing"
)

// The tray lock is the single-instance guarantee: a second tray must see it,
// and a crashed tray must never leave it behind.
func TestLockInstanceAllowsExactlyOneTray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tray.lock")
	release, err := LockInstance(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = LockInstance(path); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second lock err = %v", err)
	}
	if !InstanceRunning(path) {
		t.Fatal("InstanceRunning did not see the held lock")
	}
	release()
	if InstanceRunning(path) {
		t.Fatal("the lock survived its release")
	}
	release, err = LockInstance(path)
	if err != nil {
		t.Fatalf("restart after release: %v", err)
	}
	release()
}
