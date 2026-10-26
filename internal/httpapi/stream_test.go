package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func openStream(t *testing.T, h *harness, token string) (*http.Response, *bufio.Reader, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", h.ts.URL+"/v1/stream", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return res, bufio.NewReader(res.Body), cancel
}

// readEvent returns the next "event:" name, skipping comments and data lines.
func readEvent(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				got <- "EOF"
				return
			}
			if name, ok := strings.CutPrefix(strings.TrimSpace(line), "event: "); ok {
				got <- name
				return
			}
		}
	}()
	select {
	case e := <-got:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("no stream event in 5s")
		return ""
	}
}

func TestStreamRequiresTokenAndSendsHello(t *testing.T) {
	h := newHarness(t, false)
	res, _, cancel := openStream(t, h, "")
	cancel()
	if res.StatusCode != 401 {
		t.Fatalf("unauthenticated stream = %d, want 401", res.StatusCode)
	}
	res, br, cancel := openStream(t, h, testToken)
	defer cancel()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if got := readEvent(t, br); got != "hello" {
		t.Fatalf("first event = %q, want hello", got)
	}
}

func TestStreamWakesOnWriteAndCoalescesBursts(t *testing.T) {
	h := newHarness(t, false)
	_, br, cancel := openStream(t, h, testToken)
	defer cancel()
	readEvent(t, br) // hello
	for i := 0; i < 50; i++ {
		if err := h.store.SetSetting(context.Background(), "burst", "x"); err != nil {
			t.Fatal(err)
		}
	}
	if got := readEvent(t, br); got != "changed" {
		t.Fatalf("event after writes = %q, want changed", got)
	}
	// A burst of 50 writes inside the coalescing window is one event: the next
	// one must wait for another write (or the heartbeat, which is a comment).
	next := make(chan string, 1)
	go func() { next <- readEvent(t, br) }()
	select {
	case e := <-next:
		t.Fatalf("unexpected second event %q without a new write", e)
	case <-time.After(400 * time.Millisecond):
	}
}

func TestStreamEndsWhenTheDaemonStops(t *testing.T) {
	h := newHarness(t, false)
	done := make(chan struct{})
	h.srv.Done = done
	_, br, cancel := openStream(t, h, testToken)
	defer cancel()
	readEvent(t, br)
	close(done)
	if got := readEvent(t, br); got != "EOF" {
		t.Fatalf("after shutdown got %q, want the stream to end", got)
	}
}

func TestShutdownEndpointNeedsAuthAndStopsOnce(t *testing.T) {
	h := newHarness(t, false)
	if r := h.do("POST", "/v1/shutdown", nil, "", false); r.status != 401 {
		t.Fatalf("unauthenticated shutdown = %d", r.status)
	}
	if r := h.do("POST", "/v1/shutdown", nil, "", true); r.status != 501 {
		t.Fatalf("shutdown without a Stop hook = %d, want 501", r.status)
	}
	stopped := make(chan struct{}, 2)
	h.srv.Stop = func() { stopped <- struct{}{} }
	if r := h.do("POST", "/v1/shutdown", nil, "", true); r.status != 202 {
		t.Fatalf("shutdown = %d, want 202", r.status)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop was not called")
	}
}
