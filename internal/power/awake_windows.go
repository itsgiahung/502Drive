//go:build windows

package power

import (
	"runtime"
	"sync"

	"golang.org/x/sys/windows"
)

// SetThreadExecutionState flags.
const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

// winKeeper keeps the current *thread's* execution state, so one goroutine is
// pinned to an OS thread and owns the calls. Hold/Release are serialized and
// wait for the thread to apply the change, so the state is never left set by a
// racing release.
type winKeeper struct {
	req  chan bool
	ack  chan struct{}
	once sync.Once
}

// NewWakeKeeper returns a keeper backed by SetThreadExecutionState.
func NewWakeKeeper(*WakeOptions) WakeKeeper {
	k := &winKeeper{req: make(chan bool), ack: make(chan struct{})}
	go k.loop()
	return k
}

func (k *winKeeper) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")
	for hold := range k.req {
		flags := uintptr(esContinuous)
		if hold {
			flags |= esSystemRequired
		}
		proc.Call(flags)
		k.ack <- struct{}{}
	}
}

func (k *winKeeper) apply(hold bool) {
	select {
	case k.req <- hold:
		<-k.ack
	case <-k.closed():
	}
}

// closed is a channel closed when the keeper is discarded; the keeper lives as
// long as the daemon, so it only stops when the process exits.
func (k *winKeeper) closed() <-chan struct{} {
	return nil
}

func (k *winKeeper) Hold()    { k.apply(true) }
func (k *winKeeper) Release() { k.apply(false) }
