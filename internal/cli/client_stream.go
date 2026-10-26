package cli

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"time"
)

// WatchEvent is one signal from the daemon's realtime stream.
type WatchEvent struct {
	Connected bool
	Changed   bool
}

const (
	watchMinBackoff = 500 * time.Millisecond
	watchMaxBackoff = 5 * time.Second
	// watchIdle is longer than the server's 15 s heartbeat; silence this long
	// means the connection is dead even if the OS has not noticed.
	watchIdle = 45 * time.Second
)

// Watch opens /v1/stream and keeps it open: it reconnects with backoff and
// reports connection state, so a UI can show "live" or "reconnecting" and fall
// back to polling. The channel closes when ctx ends.
func (c *Client) Watch(ctx context.Context) <-chan WatchEvent {
	out := make(chan WatchEvent, 8)
	go func() {
		defer close(out)
		backoff := watchMinBackoff
		for ctx.Err() == nil {
			live := c.watchOnce(ctx, out)
			send(out, WatchEvent{Connected: false})
			if live {
				backoff = watchMinBackoff
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, watchMaxBackoff)
		}
	}()
	return out
}

// send never blocks: when the reader is behind, the oldest signal is dropped,
// which is safe because every signal only means "re-read".
func send(out chan WatchEvent, ev WatchEvent) {
	for {
		select {
		case out <- ev:
			return
		default:
			select {
			case <-out:
			default:
			}
		}
	}
}

// watchOnce holds one connection and reports whether it ever became live.
func (c *Client) watchOnce(ctx context.Context, out chan WatchEvent) (live bool) {
	conn, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(conn, "GET", strings.TrimRight(c.Endpoint, "/")+"/v1/stream", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")
	// The shared client has a request timeout that would cut a long stream.
	hc := &http.Client{}
	if c.HTTP != nil {
		hc.Transport = c.HTTP.Transport
	}
	res, err := hc.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false
	}
	watchdog := time.AfterFunc(watchIdle, cancel)
	defer watchdog.Stop()
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		watchdog.Reset(watchIdle)
		name, ok := strings.CutPrefix(sc.Text(), "event: ")
		if !ok {
			continue
		}
		switch name {
		case "hello", "changed":
			live = true
			send(out, WatchEvent{Connected: true, Changed: true})
		}
	}
	return live
}
