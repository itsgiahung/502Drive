package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchReconnectsAndReportsState(t *testing.T) {
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		n := hits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: hello\ndata: {\"version\":%d}\n\n", n)
		w.(http.Flusher).Flush()
		if n == 1 {
			return // the daemon restarts: the first stream ends
		}
		fmt.Fprint(w, "event: changed\ndata: {\"version\":9}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer ts.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := (&Client{Endpoint: ts.URL, Token: "tok"}).Watch(ctx)
	want := []WatchEvent{{Connected: true, Changed: true}, {Connected: false}, {Connected: true, Changed: true}, {Connected: true, Changed: true}}
	for i, w := range want {
		select {
		case got := <-ch:
			if got != w {
				t.Fatalf("event %d = %+v, want %+v", i, got, w)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("event %d never arrived", i)
		}
	}
	cancel()
	for range ch { // the channel must close once ctx ends
	}
}

func TestWatchWithAWrongTokenNeverReportsLive(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	for ev := range (&Client{Endpoint: ts.URL, Token: "bad"}).Watch(ctx) {
		if ev.Connected {
			t.Fatal("a rejected token must not look connected")
		}
	}
}
