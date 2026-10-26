//go:build windows

package power

import (
	"fmt"
	"log/slog"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Power events delivered to the notification callback.
const (
	pbtApmSuspend         = 0x0004
	pbtApmResumeSuspend   = 0x0007
	pbtApmResumeAutomatic = 0x0012
	deviceNotifyCallback  = 2
)

// devNotifyParams mirrors DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS.
type devNotifyParams struct {
	Callback uintptr
	Context  uintptr
}

// winSource receives suspend/resume notifications and IP interface changes.
// Everything is best-effort: when a registration fails the daemon falls back
// to clock-gap detection.
type winSource struct {
	log  *slog.Logger
	out  chan Event
	mu   sync.Mutex
	reg  windows.Handle
	ip   windows.Handle
	keep []any
	once sync.Once
}

// NewSource registers for power and network notifications. It returns an error
// only when the process cannot register at all.
func NewSource(log *slog.Logger) (Source, error) {
	s := &winSource{log: log, out: make(chan Event, 8)}
	if err := s.registerPower(); err != nil {
		return nil, err
	}
	if err := s.registerNetwork(); err != nil && log != nil {
		log.Warn("power: network change notifications unavailable", "err", err)
	}
	return s, nil
}

func (s *winSource) Events() <-chan Event { return s.out }

func (s *winSource) Close() error {
	s.once.Do(func() {
		if s.reg != 0 {
			unregister.Call(uintptr(s.reg))
			s.reg = 0
		}
		if s.ip != 0 {
			cancelChange.Call(uintptr(s.ip))
			s.ip = 0
		}
	})
	return nil
}

var (
	powrprof          = windows.NewLazySystemDLL("powrprof.dll")
	registerSuspend   = powrprof.NewProc("PowerRegisterSuspendResumeNotification")
	unregister        = powrprof.NewProc("PowerUnregisterSuspendResumeNotification")
	iphlpapi          = windows.NewLazySystemDLL("iphlpapi.dll")
	notifyIPInterface = iphlpapi.NewProc("NotifyIpInterfaceChange")
	cancelChange      = iphlpapi.NewProc("CancelMibChangeNotify2")
)

func (s *winSource) registerPower() error {
	cb := syscall.NewCallback(func(_ uintptr, typ uintptr, _ uintptr) uintptr {
		switch uint32(typ) {
		case pbtApmSuspend:
			s.push(Event{Kind: Suspend, At: time.Now()})
		case pbtApmResumeSuspend, pbtApmResumeAutomatic:
			s.push(Event{Kind: Resume, At: time.Now()})
		}
		return 0
	})
	params := &devNotifyParams{Callback: cb}
	var handle windows.Handle
	r, _, err := registerSuspend.Call(deviceNotifyCallback, uintptr(unsafe.Pointer(params)), uintptr(unsafe.Pointer(&handle)))
	if r != 0 {
		return fmt.Errorf("PowerRegisterSuspendResumeNotification: %w", err)
	}
	s.mu.Lock()
	s.reg, s.keep = handle, []any{params, cb}
	s.mu.Unlock()
	return nil
}

func (s *winSource) registerNetwork() error {
	cb := syscall.NewCallback(func(_ uintptr, _ uintptr, _ uintptr) uintptr {
		s.push(Event{Kind: NetworkChanged, At: time.Now()})
		return 0
	})
	var handle windows.Handle
	// AF_UNSPEC (0) reports every interface family; the callback is advisory.
	r, _, err := notifyIPInterface.Call(0, cb, 0, 0, uintptr(unsafe.Pointer(&handle)))
	if r != 0 {
		return fmt.Errorf("NotifyIpInterfaceChange: %w", err)
	}
	s.mu.Lock()
	s.ip = handle
	s.keep = append(s.keep, cb)
	s.mu.Unlock()
	return nil
}

// push never blocks: an OS callback must return promptly, and a dropped event
// still leaves the clock-gap detector in place.
func (s *winSource) push(ev Event) {
	select {
	case s.out <- ev:
	default:
	}
}
