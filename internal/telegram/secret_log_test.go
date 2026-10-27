package telegram

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/state"
)

// syncBuffer collects log lines written from the bot's goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestBotTokenNeverReachesTheLog is finding #16: the polling URL carries the
// bot token, so an unsanitized transport error would put a credential in the
// daemon log.
func TestBotTokenNeverReachesTheLog(t *testing.T) {
	const token = "TEST_TOKEN"
	// A closed server gives a deterministic transport error, no network needed.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	dead.Close()

	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	svc := &app.Service{Store: store, Version: "test", Now: time.Now,
		Provider: func(context.Context) (drive.Provider, error) { return drivetest.New(), nil }}
	logs := &syncBuffer{}
	tg, err := New(context.Background(), Options{Service: svc, Token: token, OwnerID: 100,
		APIURL: dead.URL, PollTimeout: time.Second, Log: slog.New(slog.NewTextHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	tg.Run(ctx)

	out := logs.String()
	if !strings.Contains(out, "bot API failed") || !strings.Contains(out, "getMe") {
		t.Fatalf("the polling failure was not logged: %q", out)
	}
	if strings.Contains(out, token) {
		t.Fatalf("the bot token reached the log: %q", out)
	}
}
