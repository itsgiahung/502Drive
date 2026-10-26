package state

import (
	"context"
	"database/sql"
	"sync"
)

// changes is the daemon's cheap "something was written" signal. Clients that
// stream (the HTTP /v1/stream endpoint) wake on it and re-read; they never
// receive row data from it, so a missed wake-up only costs a later refresh.
type changes struct {
	mu sync.Mutex
	n  uint64
	ch chan struct{}
}

func newChanges() *changes { return &changes{ch: make(chan struct{})} }

func (c *changes) bump() {
	c.mu.Lock()
	c.n++
	close(c.ch)
	c.ch = make(chan struct{})
	c.mu.Unlock()
}

func (c *changes) current() (uint64, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n, c.ch
}

// dbConn counts successful writes made through database/sql so every write
// path (single statements and transactions) feeds the same signal.
type dbConn struct {
	*sql.DB
	c *changes
}

func (d *dbConn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	res, err := d.DB.ExecContext(ctx, query, args...)
	if err == nil {
		d.c.bump()
	}
	return res, err
}

// Version returns the current write counter and a channel that is closed at
// the next write. Take the channel, then re-read data, then wait on it: that
// order cannot miss a change.
func (s *Store) Version() (uint64, <-chan struct{}) { return s.db.c.current() }
