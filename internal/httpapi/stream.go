package httpapi

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
)

const (
	maxStreams = 16
	// streamCoalesce batches bursts of writes (a copy updates items many times
	// per second) into one "changed" event.
	streamCoalesce  = 150 * time.Millisecond
	streamHeartbeat = 15 * time.Second
)

var activeStreams atomic.Int32

// stream is the realtime channel: Server-Sent Events carrying only a change
// counter. A client re-reads the resources it shows when "changed" arrives, so
// there is nothing to replay after a reconnect; a new connection always starts
// with an immediate "hello".
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	rid := requestID()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Request-Id", rid)
	if _, ok := s.authenticate(r); !ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeErr(w, rid, app.Fail(401, "unauthenticated", "A valid bearer token is required."))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		writeErr(w, rid, app.Fail(500, "internal_error", "Streaming is not supported here."))
		return
	}
	if activeStreams.Add(1) > maxStreams {
		activeStreams.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		writeErr(w, rid, &app.Error{Status: 429, Code: "too_many_streams", Message: "Too many open streams.", Retryable: true, RetryAfter: 2 * time.Second})
		return
	}
	defer activeStreams.Add(-1)
	// The server-wide write timeout would cut a long-lived stream.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	version, changed := s.Svc.Store.Version()
	fmt.Fprintf(w, "retry: 2000\nevent: hello\ndata: {\"version\":%d}\n\n", version)
	flusher.Flush()

	beat := time.NewTicker(streamHeartbeat)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.Done:
			return
		case <-beat.C:
			fmt.Fprint(w, ": ping\n\n")
		case <-changed:
			select {
			case <-time.After(streamCoalesce):
			case <-r.Context().Done():
				return
			case <-s.Done:
				return
			}
			version, changed = s.Svc.Store.Version()
			fmt.Fprintf(w, "event: changed\ndata: {\"version\":%d}\n\n", version)
		}
		flusher.Flush()
	}
}

// shutdown asks the daemon to stop. It answers first and stops a moment later
// so the caller sees the 202.
func (s *Server) shutdown(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if s.Stop == nil {
		return 0, nil, app.Fail(http.StatusNotImplemented, "not_supported", "This daemon cannot be stopped over HTTP.")
	}
	stop := s.Stop
	time.AfterFunc(200*time.Millisecond, stop)
	return http.StatusAccepted, map[string]string{"status": "stopping"}, nil
}
