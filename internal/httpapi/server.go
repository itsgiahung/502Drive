// Package httpapi exposes the application service as the /v1 JSON API.
package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

const maxBody = 64 << 10

type Server struct {
	Svc *app.Service
	// TokenHash is sha256(owner bearer token).
	TokenHash [32]byte
	Log       *slog.Logger
	// WorkerReady reports whether the engine loop is running.
	WorkerReady func() bool
	// OAuthPort is the fixed callback port for logins (0 = any free port).
	OAuthPort int
	// RevokeEndpoint overrides Google's revoke URL (tests).
	RevokeEndpoint string
	// Maintenance reports that the daemon runs without workers: new work is
	// refused instead of being queued for a worker that is not there.
	Maintenance bool
	// Done is closed when the daemon begins shutting down; open streams end.
	Done <-chan struct{}
	// Stop asks the daemon to shut down (POST /v1/shutdown); nil disables it.
	Stop func()
}

// refuseMaintenance is the single gate for work admission in maintenance mode.
func (s *Server) refuseMaintenance() error {
	if !s.Maintenance {
		return nil
	}
	return app.Fail(http.StatusServiceUnavailable, "maintenance", "The daemon is in maintenance mode: new work is not accepted.")
}

func HashToken(token string) [32]byte { return sha256.Sum256([]byte(strings.TrimSpace(token))) }

type handler func(w http.ResponseWriter, r *http.Request, p app.Principal) (status int, data any, err error)

// Handler returns the routed, authenticated API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		// Liveness is deliberately minimal and unauthenticated (see the contract).
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte("{\"status\":\"alive\"}\n"))
	})
	route := func(pattern string, h handler) { mux.HandleFunc(pattern, s.wrap(true, h)) }
	route("GET /health/ready", s.ready)
	mux.HandleFunc("GET /v1/stream", s.stream)
	route("POST /v1/shutdown", s.shutdown)
	route("GET /v1/status", s.status)
	route("POST /v1/plans", s.createPlan)
	route("GET /v1/jobs", s.listJobs)
	route("POST /v1/jobs", s.createJob)
	route("GET /v1/jobs/{id}", s.getJob)
	route("GET /v1/jobs/{id}/events", s.jobEvents)
	for _, a := range []string{"pause", "resume", "cancel", "retry"} {
		route("POST /v1/jobs/{id}/"+a, s.jobAction(a))
	}
	route("GET /v1/destinations", s.listDestinations)
	route("POST /v1/destinations", s.addDestination)
	route("PUT /v1/destinations/default", s.setDefaultDestination)
	route("GET /v1/account", s.account)
	route("POST /v1/account/auth-sessions", s.startLogin)
	route("GET /v1/account/auth-sessions/{id}", s.loginStatus)
	route("POST /v1/account/revoke", s.revoke)
	route("GET /v1/preferences", s.getPreferences)
	route("PATCH /v1/preferences", s.patchPreferences)
	route("GET /v1/operators", s.listOperators)
	route("POST /v1/operators", s.addOperator)
	route("DELETE /v1/operators/{id}", s.removeOperator)
	route("GET /v1/doctor", s.doctor)
	route("GET /v1/collections", s.listCollections)
	route("GET /v1/collections/{id}", s.getCollection)
	s.syncRoutes(route)
	mux.HandleFunc("/", s.wrap(true, func(http.ResponseWriter, *http.Request, app.Principal) (int, any, error) {
		return 0, nil, app.Fail(404, "not_found", "Not found.")
	}))
	return mux
}

func requestID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

type envelope struct {
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

type errBody struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Retryable         bool   `json:"retryable"`
	RetryAfterSeconds *int   `json:"retry_after_seconds"`
}

func (s *Server) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.DiscardHandler)
}

func (s *Server) wrap(auth bool, h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestID()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Request-Id", rid)
		defer func() {
			if v := recover(); v != nil {
				s.logger().Error("panic in handler", "request", rid, "panic", fmt.Sprint(v))
				writeErr(w, rid, app.Fail(500, "internal_error", "Internal error."))
			}
		}()
		p := app.Principal{}
		if auth {
			var ok bool
			if p, ok = s.authenticate(r); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeErr(w, rid, app.Fail(401, "unauthenticated", "A valid bearer token is required."))
				return
			}
		}
		status, data, err := h(w, r, p)
		if err != nil {
			ae := app.Classify(err)
			if ae.Status >= 500 && ae.Code == "internal_error" {
				s.logger().Error("request failed", "request", rid, "path", r.URL.Path, "err", err.Error())
			}
			writeErr(w, rid, ae)
			return
		}
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(envelope{Data: data, RequestID: rid})
	}
}

func writeErr(w http.ResponseWriter, rid string, e *app.Error) {
	var ra *int
	if e.RetryAfter > 0 {
		n := int((e.RetryAfter + time.Second - 1) / time.Second)
		ra = &n
		w.Header().Set("Retry-After", strconv.Itoa(n))
	}
	w.WriteHeader(e.Status)
	json.NewEncoder(w).Encode(map[string]any{"error": errBody{e.Code, e.Message, e.Retryable, ra}, "request_id": rid})
}

func (s *Server) authenticate(r *http.Request) (app.Principal, bool) {
	h := r.Header.Get("Authorization")
	tok, found := strings.CutPrefix(h, "Bearer ")
	if !found || tok == "" {
		return app.Principal{}, false
	}
	sum := sha256.Sum256([]byte(tok))
	if subtle.ConstantTimeCompare(sum[:], s.TokenHash[:]) != 1 {
		return app.Principal{}, false
	}
	return app.Owner, true
}

// decode reads a bounded JSON body, rejecting unknown fields and trailing data.
func decode(r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mt, _, err := mime.ParseMediaType(ct); err != nil || mt != "application/json" {
			return app.Fail(http.StatusUnsupportedMediaType, "invalid_request", "Content-Type must be application/json.")
		}
	}
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody+1))
	if err != nil || len(b) > maxBody {
		return app.Fail(http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large.")
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return app.Fail(400, "invalid_request", "A JSON body is required.")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		return app.Fail(400, "invalid_request", "The JSON body is invalid or has unknown fields.")
	}
	if d.Decode(new(any)) != io.EOF {
		return app.Fail(400, "invalid_request", "Unexpected data after the JSON body.")
	}
	return nil
}

func idemKey(r *http.Request) string { return r.Header.Get("Idempotency-Key") }

func pageParams(r *http.Request) (int, error) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return 0, app.Fail(400, "invalid_request", "limit must be between 1 and 100.")
		}
		limit = n
	}
	return limit, nil
}

// cursors are opaque and bound to the principal and filters that made them.
func encodeCursor(kind, scope, filter string, n int64) *string {
	if n == 0 {
		return nil
	}
	v := base64.RawURLEncoding.EncodeToString([]byte(strings.Join([]string{kind, scope, filter, strconv.FormatInt(n, 10)}, "\x1f")))
	return &v
}

func decodeCursor(raw, kind, scope, filter string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	bad := app.Fail(400, "invalid_request", "The cursor is not valid for this request.")
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, bad
	}
	parts := strings.Split(string(b), "\x1f")
	if len(parts) != 4 || parts[0] != kind || parts[1] != scope || parts[2] != filter {
		return 0, bad
	}
	n, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || n < 1 {
		return 0, bad
	}
	return n, nil
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	v, err := s.Svc.Store.SchemaVersion(r.Context())
	ok := err == nil && s.Svc.Store.Check(r.Context()) == nil
	worker := s.WorkerReady == nil || s.WorkerReady()
	if !ok || !worker {
		return 0, nil, app.Fail(http.StatusConflict, "not_ready", "The service is not ready.")
	}
	return 200, map[string]any{"ready": true, "schema_version": v, "worker_ready": worker}, nil
}

func (s *Server) status(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	st, err := s.Svc.Status(r.Context())
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"version": st.Version, "schema_version": st.SchemaVersion, "ready": st.Ready, "account": accountDTO(st.Account),
		"bot_state": st.BotState, "running_jobs": st.Running, "queued_jobs": st.Queued, "blocked_jobs": st.Blocked}, nil
}

// ---- plans & jobs ----

func (s *Server) createPlan(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	var req app.PlanRequest
	if err := decode(r, &req); err != nil {
		return 0, nil, err
	}
	view, replayed, err := s.Svc.Inspect(r.Context(), p, idemKey(r), req)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	return 201, planDTO(view), nil
}

func markReplay(w http.ResponseWriter, replayed bool) {
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
}

type jobInput struct {
	PlanID           string `json:"plan_id"`
	ExpectedRevision *int   `json:"expected_revision"`
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	var in jobInput
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.PlanID == "" || in.ExpectedRevision == nil {
		return 0, nil, app.Fail(400, "invalid_request", "plan_id and expected_revision are required.")
	}
	res, err := s.Svc.Submit(r.Context(), p, idemKey(r), in.PlanID, *in.ExpectedRevision)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, res.Replayed)
	w.Header().Set("Location", "/v1/jobs/"+res.Job.ID)
	return 202, jobDTO(res.Job), nil
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	limit, err := pageParams(r)
	if err != nil {
		return 0, nil, err
	}
	q := r.URL.Query()
	all := q.Get("all") == "true"
	st := q.Get("state")
	if st != "" && !validJobState(st) {
		return 0, nil, app.Fail(400, "invalid_request", "Unknown job state.")
	}
	scopeKey := p.ID
	if all {
		scopeKey = "*"
	}
	cur, err := decodeCursor(q.Get("cursor"), "jobs", scopeKey, st)
	if err != nil {
		return 0, nil, err
	}
	jobs, next, err := s.Svc.Jobs(r.Context(), p, all, st, limit, cur)
	if err != nil {
		return 0, nil, err
	}
	items := make([]any, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, jobDTO(j))
	}
	return 200, map[string]any{"items": items, "next_cursor": encodeCursor("jobs", scopeKey, st, next)}, nil
}

func validJobState(s string) bool {
	switch s {
	case "queued", "running", "pausing", "paused", "cancelling", "reconciling", "blocked", "succeeded", "partial", "failed", "cancelled":
		return true
	}
	return false
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	j, err := s.Svc.Job(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return 200, jobDTO(j), nil
}

func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	limit, err := pageParams(r)
	if err != nil {
		return 0, nil, err
	}
	after := int64(0)
	if v := r.URL.Query().Get("after_sequence"); v != "" {
		if after, err = strconv.ParseInt(v, 10, 64); err != nil || after < 0 {
			return 0, nil, app.Fail(400, "invalid_request", "after_sequence must be a non-negative integer.")
		}
	}
	evs, err := s.Svc.Events(r.Context(), p, r.PathValue("id"), after, limit)
	if err != nil {
		return 0, nil, err
	}
	items := make([]any, 0, len(evs))
	next := after
	for _, e := range evs {
		items = append(items, eventDTO(e))
		next = e.Sequence
	}
	return 200, map[string]any{"items": items, "next_sequence": next}, nil
}

type actionInput struct {
	ExpectedRevision *int `json:"expected_revision"`
}

func (s *Server) jobAction(action string) handler {
	return func(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
		var in actionInput
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		if in.ExpectedRevision == nil {
			return 0, nil, app.Fail(400, "invalid_request", "expected_revision is required.")
		}
		res, err := s.Svc.JobAction(r.Context(), p, idemKey(r), r.PathValue("id"), action, *in.ExpectedRevision)
		if err != nil {
			return 0, nil, err
		}
		markReplay(w, res.Replayed)
		return 202, jobDTO(res.Job), nil
	}
}

// ---- destinations ----

func (s *Server) listDestinations(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	ds, err := s.Svc.Destinations(r.Context())
	if err != nil {
		return 0, nil, err
	}
	items := make([]any, 0, len(ds))
	for _, d := range ds {
		items = append(items, destinationDTO(d))
	}
	return 200, map[string]any{"items": items, "next_cursor": nil}, nil
}

func (s *Server) addDestination(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	var req app.DestinationRequest
	if err := decode(r, &req); err != nil {
		return 0, nil, err
	}
	d, replayed, err := s.Svc.AddDestination(r.Context(), p, idemKey(r), req)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	return 201, destinationDTO(d), nil
}

func (s *Server) setDefaultDestination(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	var in struct {
		DestinationID    string `json:"destination_id"`
		ExpectedRevision *int   `json:"expected_revision"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.DestinationID == "" || in.ExpectedRevision == nil {
		return 0, nil, app.Fail(400, "invalid_request", "destination_id and expected_revision are required.")
	}
	d, replayed, err := s.Svc.SetDefaultDestination(r.Context(), p, idemKey(r), in.DestinationID, *in.ExpectedRevision)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	return 200, destinationDTO(d), nil
}

// ---- account ----

func (s *Server) account(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	a, err := s.Svc.Account(r.Context())
	if err != nil {
		return 0, nil, err
	}
	return 200, accountDTO(a), nil
}

func (s *Server) startLogin(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	var in struct {
		NoBrowser bool `json:"no_browser"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	sess, err := s.Svc.StartLogin(r.Context(), p, idemKey(r), s.OAuthPort)
	if err != nil {
		return 0, nil, err
	}
	return 201, authSessionDTO(sess), nil
}

func (s *Server) loginStatus(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	sess, err := s.Svc.LoginStatus(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return 200, authSessionDTO(sess), nil
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	a, err := s.Svc.Revoke(r.Context(), p, idemKey(r), s.RevokeEndpoint)
	if err != nil {
		return 0, nil, err
	}
	return 202, accountDTO(a), nil
}

// ---- preferences, operators, doctor ----

func (s *Server) getPreferences(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	pr, err := s.Svc.Preferences(r.Context(), p)
	if err != nil {
		return 0, nil, err
	}
	return 200, prefsDTO(pr), nil
}

func (s *Server) patchPreferences(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	var patch app.PreferencePatch
	if err := decode(r, &patch); err != nil {
		return 0, nil, err
	}
	pr, err := s.Svc.PatchPreferences(r.Context(), p, idemKey(r), patch)
	if err != nil {
		return 0, nil, err
	}
	return 200, prefsDTO(pr), nil
}

func (s *Server) listOperators(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	ops, err := s.Svc.Operators(r.Context(), p)
	if err != nil {
		return 0, nil, err
	}
	items := make([]any, 0, len(ops))
	for _, o := range ops {
		items = append(items, operatorDTO(o))
	}
	return 200, map[string]any{"items": items, "next_cursor": nil}, nil
}

func (s *Server) addOperator(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	var in struct {
		TelegramUserID int64 `json:"telegram_user_id"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	o, replayed, err := s.Svc.AddOperator(r.Context(), p, idemKey(r), in.TelegramUserID)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	return 201, operatorDTO(o), nil
}

func (s *Server) removeOperator(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.Svc.RemoveOperator(r.Context(), p, idemKey(r), r.PathValue("id")); err != nil {
		return 0, nil, err
	}
	return http.StatusNoContent, nil, nil
}

func (s *Server) doctor(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	online := r.URL.Query().Get("online") == "true"
	checks, err := s.Svc.Doctor(r.Context(), p, online)
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"online": online, "checks": checks}, nil
}

// ---- collections ----

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	limit, err := pageParams(r)
	if err != nil {
		return 0, nil, err
	}
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("query"))
	mode := q.Get("sync_mode")
	switch mode {
	case "", "off", "scheduled", "realtime":
	default:
		return 0, nil, app.Fail(400, "invalid_request", "sync_mode must be off, scheduled or realtime.")
	}
	all := q.Get("all") == "true"
	scopeKey := p.ID
	if all {
		scopeKey = "*"
	}
	// The cursor is bound to the principal and both filters, as for /v1/jobs.
	filter := query + "\x1e" + mode
	cursor, err := decodeCursor(q.Get("cursor"), "collections", scopeKey, filter)
	if err != nil {
		return 0, nil, err
	}
	cs, next, err := s.Svc.CollectionPage(r.Context(), p, all, query, mode, limit, cursor)
	if err != nil {
		return 0, nil, err
	}
	items := make([]any, 0, len(cs))
	for _, c := range cs {
		dto, derr := s.collectionDTO(r.Context(), c)
		if derr != nil {
			return 0, nil, derr
		}
		delete(dto, "sync_mode")
		items = append(items, dto)
	}
	return 200, map[string]any{"items": items, "next_cursor": encodeCursor("collections", scopeKey, filter, next)}, nil
}

func (s *Server) getCollection(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	c, err := s.Svc.Collection(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	dto, err := s.collectionDTO(r.Context(), c)
	if err != nil {
		return 0, nil, err
	}
	delete(dto, "sync_mode")
	return 200, dto, nil
}

var _ = errors.New
var _ = context.Background
var _ state.Job
