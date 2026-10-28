package state

import (
	"errors"
	"testing"
	"time"
)

// A callback is one-shot and tied to the message it was sent in: the same data
// arriving from another message is refused and does not burn the button.
func TestCallbackMustComeFromItsOwnMessage(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	now := time.Now()
	put := func(id string, msg int) {
		if err := s.PutCallback(ctx, Callback{ID: id, Principal: "owner", ChatID: 5, MessageID: msg, Action: "lang",
			ExpiresAt: now.Add(time.Hour), CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	put("bound", 111)
	if _, err := s.ClaimCallback(ctx, "bound", "owner", 5, 222, true, now); !errors.Is(err, ErrCallbackStale) {
		t.Fatalf("another message must be refused, got %v", err)
	}
	if _, err := s.ClaimCallback(ctx, "bound", "owner", 5, 111, true, now); err != nil {
		t.Fatalf("the original message must still work after a refused attempt: %v", err)
	}
	if _, err := s.ClaimCallback(ctx, "bound", "owner", 5, 111, true, now); !errors.Is(err, ErrCallbackStale) {
		t.Fatalf("a second click is refused: %v", err)
	}
	put("unbound", 0) // not yet bound to a message: accepted from any message of that chat
	if _, err := s.ClaimCallback(ctx, "unbound", "owner", 5, 333, true, now); err != nil {
		t.Fatalf("an unbound callback: %v", err)
	}
}
