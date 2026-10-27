package drive

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestFaultInjectorPassThrough(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"file123","name":"hello.txt"}`))
	}))
	defer ts.Close()

	inj := NewFaultInjector(http.DefaultTransport)
	client := &http.Client{Transport: inj}

	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"id":"file123","name":"hello.txt"}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if inj.Hits() != 0 {
		t.Fatalf("expected 0 hits, got %d", inj.Hits())
	}
}

func TestFaultInjectorInject429(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("backend server should not be reached when 429 is injected")
	}))
	defer ts.Close()

	inj := NewFaultInjector(http.DefaultTransport)
	inj.Inject429("", 1, 5*time.Second)

	client := &http.Client{Transport: inj}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	if ra := resp.Header.Get("Retry-After"); ra != "5" {
		t.Fatalf("expected Retry-After 5, got %q", ra)
	}
	if inj.Hits() != 1 {
		t.Fatalf("expected 1 hit, got %d", inj.Hits())
	}

	// Second request should pass through to ts, which fails the test if reached,
	// so let's verify inj.Hits is still 1.
}

func TestFaultInjectorWithDriveClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"root","name":"My Drive"}`))
	}))
	defer ts.Close()

	inj := NewFaultInjector(http.DefaultTransport)
	inj.Inject429("/files/", 1, 3*time.Second)

	c, err := NewClientWithTransport(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"}), ts.URL, inj)
	if err != nil {
		t.Fatalf("NewClientWithTransport: %v", err)
	}

	// First call hits /files/root -> should get 429 and be wrapped as KindThrottled
	_, err = c.GetFile(context.Background(), "root", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var de *Error
	if !errors.As(err, &de) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if de.Kind != KindThrottled {
		t.Fatalf("expected KindThrottled, got %v", de.Kind)
	}
	if de.RetryAfter != 3*time.Second {
		t.Fatalf("expected RetryAfter 3s, got %v", de.RetryAfter)
	}

	// Second call should pass through and succeed
	f, err := c.GetFile(context.Background(), "root", "")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if f.ID != "root" {
		t.Fatalf("expected root, got %s", f.ID)
	}
}

func TestFaultInjectorDropConnection(t *testing.T) {
	inj := NewFaultInjector(http.DefaultTransport)
	inj.InjectDrop("", 1)

	c, err := NewClientWithTransport(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"}), "http://127.0.0.1:54321", inj)
	if err != nil {
		t.Fatalf("NewClientWithTransport: %v", err)
	}

	_, err = c.GetFile(context.Background(), "root", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var de *Error
	if !errors.As(err, &de) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if de.Kind != KindUnreachable {
		t.Fatalf("expected KindUnreachable, got %v", de.Kind)
	}
}

func TestFaultInjectorLoseResponse(t *testing.T) {
	inj := NewFaultInjector(http.DefaultTransport)
	inj.InjectLoseResponse("", 1)

	c, err := NewClientWithTransport(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"}), "http://127.0.0.1:54321", inj)
	if err != nil {
		t.Fatalf("NewClientWithTransport: %v", err)
	}

	_, err = c.GetFile(context.Background(), "root", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var de *Error
	if !errors.As(err, &de) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if de.Kind != KindUnavailable {
		t.Fatalf("expected KindUnavailable, got %v", de.Kind)
	}
}

func TestFaultInjectorMaxHitsAndSkipHits(t *testing.T) {
	var count int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	defer ts.Close()

	inj := NewFaultInjector(http.DefaultTransport)
	inj.AddRule(&FaultRule{
		Action:     FaultThrottle429,
		SkipHits:   1, // Skip request 1
		MaxHits:    2, // Inject on requests 2 and 3
		RetryAfter: 2 * time.Second,
	})

	client := &http.Client{Transport: inj}

	// Req 1: pass through
	resp, err := client.Get(ts.URL)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("req 1 failed: err=%v, code=%d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// Req 2: throttle
	resp, err = client.Get(ts.URL)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("req 2 expected 429, got err=%v, code=%d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// Req 3: throttle
	resp, err = client.Get(ts.URL)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("req 3 expected 429, got err=%v, code=%d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// Req 4: maxHits reached, pass through
	resp, err = client.Get(ts.URL)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("req 4 expected 200, got err=%v, code=%d", err, resp.StatusCode)
	}
	resp.Body.Close()

	if count != 2 {
		t.Fatalf("expected backend reached 2 times, got %d", count)
	}
}

func TestFaultInjectorConcurrent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	inj := NewFaultInjector(http.DefaultTransport)
	inj.Inject429("", 50, 1*time.Second)

	client := &http.Client{Transport: inj}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				resp, err := client.Get(ts.URL)
				if err == nil {
					resp.Body.Close()
				}
			}
		}()
	}
	wg.Wait()

	if inj.Hits() != 50 {
		t.Fatalf("expected exactly 50 hits, got %d", inj.Hits())
	}
}

func TestFaultInjectorCustomRule(t *testing.T) {
	inj := NewFaultInjector(http.DefaultTransport)
	inj.AddRule(&FaultRule{
		Match: func(req *http.Request) bool {
			return req.Header.Get("X-Custom-Test") == "match"
		},
		Action: FaultServerError,
	})

	client := &http.Client{Transport: inj}

	req, _ := http.NewRequest(http.MethodGet, "http://dummy.url", nil)
	req.Header.Set("X-Custom-Test", "match")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestFaultInjectorNetDialError(t *testing.T) {
	opErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	if kindForError(opErr) != KindUnreachable {
		t.Fatalf("expected KindUnreachable for dial error, got %v", kindForError(opErr))
	}
}

// kindForError wraps an error through drive.wrap logic for testing classification.
func kindForError(err error) Kind {
	w := wrap(err)
	if de, ok := w.(*Error); ok {
		return de.Kind
	}
	return KindInvalid
}
