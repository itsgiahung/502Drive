//go:build linux

package power

import (
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// delayMax mirrors logind's InhibitDelayMaxSec default: the supervisor never
// gets more than a few seconds to checkpoint before the machine sleeps, and we
// release the lock at the same deadline even when the daemon is slow.
const delayMax = 5 * time.Second

const (
	logindBus     = "org.freedesktop.login1"
	logindPath    = "/org/freedesktop/login1"
	logindManager = "org.freedesktop.login1.Manager"
	prepareSignal = "org.freedesktop.login1.Manager.PrepareForSleep"
)

// NewSource subscribes to logind's PrepareForSleep on the system bus. When the
// bus or logind is missing it returns an error; the daemon then relies on the
// clock-gap detector alone.
func NewSource(log *slog.Logger) (Source, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}
	var owner bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, logindBus).Store(&owner); err != nil || !owner {
		conn.Close()
		return nil, errors.New("logind is not available on the system bus")
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(dbus.ObjectPath(logindPath)),
		dbus.WithMatchInterface(logindManager),
		dbus.WithMatchMember("PrepareForSleep"),
	); err != nil {
		conn.Close()
		return nil, err
	}
	s := &logindSource{conn: conn, log: log, out: make(chan Event, 8), done: make(chan struct{})}
	sigs := make(chan *dbus.Signal, 8)
	conn.Signal(sigs)
	go s.loop(sigs)
	return s, nil
}

type logindSource struct {
	conn *dbus.Conn
	log  *slog.Logger
	out  chan Event
	done chan struct{}
	once sync.Once
}

func (s *logindSource) Events() <-chan Event { return s.out }

func (s *logindSource) Close() error {
	s.once.Do(func() {
		close(s.done)
		s.conn.Close()
	})
	return nil
}

func (s *logindSource) loop(sigs <-chan *dbus.Signal) {
	for {
		select {
		case <-s.done:
			return
		case sig, ok := <-sigs:
			if !ok {
				return
			}
			if sig.Name != prepareSignal || len(sig.Body) != 1 {
				continue
			}
			going, _ := sig.Body[0].(bool)
			if going {
				ack := s.inhibitDelay()
				select {
				case s.out <- Event{Kind: Suspend, At: time.Now(), Ack: ack}:
				default:
					ack() // nobody is listening; never keep the OS waiting
				}
				continue
			}
			select {
			case s.out <- Event{Kind: Resume, At: time.Now()}:
			default:
			}
		}
	}
}

// inhibitDelay asks logind for a delay lock so the daemon can checkpoint. The
// lock is released by the returned function, or automatically at delayMax.
func (s *logindSource) inhibitDelay() func() {
	obj := s.conn.Object(logindBus, dbus.ObjectPath(logindPath))
	var fd dbus.UnixFD
	err := obj.Call(logindManager+".Inhibit", 0, "sleep", "502Drive", "Checkpoint before sleep", "delay").Store(&fd)
	if err != nil {
		if s.log != nil {
			s.log.Warn("power: cannot take the logind delay lock", "err", err)
		}
		return func() {}
	}
	f := os.NewFile(uintptr(fd), "logind-delay")
	var once sync.Once
	ack := func() { once.Do(func() { _ = f.Close() }) }
	time.AfterFunc(delayMax, ack)
	return ack
}
