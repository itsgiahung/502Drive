package tray

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// ErrAlreadyRunning means another tray holds the instance lock.
var ErrAlreadyRunning = errors.New("the 502drive tray is already running")

// LockInstance takes the single-instance lock for the tray. The lock dies with
// the process, so a crashed tray never blocks the next one.
func LockInstance(path string) (release func(), err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	l := flock.New(path, flock.SetPermissions(0o600))
	ok, err := l.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAlreadyRunning
	}
	return func() { _ = l.Unlock() }, nil
}

// InstanceRunning reports whether a tray currently holds the lock.
func InstanceRunning(path string) bool {
	release, err := LockInstance(path)
	if err != nil {
		return errors.Is(err, ErrAlreadyRunning)
	}
	release()
	return false
}
