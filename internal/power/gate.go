package power

import (
	"context"
	"sync"
)

// Gate is the online switch shared by the lifecycle and the engine. It is safe
// for concurrent use and wakes every waiter on a transition.
type Gate struct {
	mu     sync.Mutex
	online bool
	ch     chan struct{}
}

// NewGate returns a gate in the given state.
func NewGate(online bool) *Gate {
	return &Gate{online: online, ch: make(chan struct{})}
}

// Online reports whether remote work is allowed now.
func (g *Gate) Online() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.online
}

// Set changes the state and wakes waiters. It reports whether it changed.
func (g *Gate) Set(online bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.online == online {
		return false
	}
	g.online = online
	close(g.ch)
	g.ch = make(chan struct{})
	return true
}

// Wait blocks until the gate is online or ctx ends. It reports whether the gate
// is online. A waiter that observed offline is registered before the state is
// re-checked, so no transition can be missed.
func (g *Gate) Wait(ctx context.Context) bool {
	for {
		g.mu.Lock()
		if g.online {
			g.mu.Unlock()
			return true
		}
		ch := g.ch
		g.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return false
		}
	}
}
