//go:build linux

package power

import (
	"log/slog"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

// logindKeeper takes a block inhibitor while jobs run. It is opt-in: blocking
// sleep affects the battery and heat, so it is never enabled by default.
type logindKeeper struct {
	log    *slog.Logger
	reason string

	mu   sync.Mutex
	conn *dbus.Conn
	file *os.File
}

// NewWakeKeeper returns a logind-backed keeper; failures are silent no-ops.
func NewWakeKeeper(o *WakeOptions) WakeKeeper {
	k := &logindKeeper{}
	if o != nil {
		k.log, k.reason = o.Log, o.Reason
	}
	if k.reason == "" {
		k.reason = "502Drive is copying"
	}
	return k
}

func (k *logindKeeper) Hold() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.file != nil {
		return
	}
	if k.conn == nil {
		conn, err := dbus.SystemBus()
		if err != nil {
			k.warn("cannot reach the system bus", err)
			return
		}
		k.conn = conn
	}
	obj := k.conn.Object(logindBus, dbus.ObjectPath(logindPath))
	var fd dbus.UnixFD
	if err := obj.Call(logindManager+".Inhibit", 0, "sleep:idle", "502Drive", k.reason, "block").Store(&fd); err != nil {
		k.warn("cannot take the logind block lock", err)
		return
	}
	k.file = os.NewFile(uintptr(fd), "logind-block")
}

func (k *logindKeeper) Release() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.file != nil {
		_ = k.file.Close()
		k.file = nil
	}
}

func (k *logindKeeper) warn(msg string, err error) {
	if k.log != nil {
		k.log.Warn("power: "+msg, "err", err)
	}
}
