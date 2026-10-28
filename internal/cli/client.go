package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Exit codes (documented in docs/features/cli.md).
const (
	ExitOK          = 0
	ExitInternal    = 1
	ExitUsage       = 2
	ExitAuth        = 3
	ExitNotFound    = 4
	ExitConflict    = 5
	ExitUnavailable = 6
	ExitJobFailed   = 7
	ExitCancelled   = 8
	ExitInterrupt   = 130
)

// APIError is a failure reported by the daemon or by the client itself.
type APIError struct {
	Status     int
	Code       string
	Message    string
	Retryable  bool
	RetryAfter int
	RequestID  string
	Exit       int
}

func (e *APIError) Error() string { return e.Message }

// ErrCode is the stable error code; the TUI turns it into a plain sentence.
func (e *APIError) ErrCode() string { return e.Code }

func localErr(exit int, code, msg string) *APIError {
	return &APIError{Exit: exit, Code: code, Message: msg}
}

func exitForStatus(status int) int {
	switch {
	case status == 401 || status == 403:
		return ExitAuth
	case status == 404:
		return ExitNotFound
	case status == 409 || status == 410:
		return ExitConflict
	case status == 429 || status == 502 || status == 503 || status == 504:
		return ExitUnavailable
	case status == 400 || status == 413 || status == 415 || status == 501:
		return ExitUsage
	}
	return ExitInternal
}

// Client talks to the daemon's HTTP API; it never opens the database.
type Client struct {
	Endpoint string
	Token    string
	HTTP     *http.Client
}

type Response struct {
	Status    int
	Header    http.Header
	Raw       json.RawMessage // the full envelope, for --json passthrough
	Data      json.RawMessage
	RequestID string
}

func NewKey() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "cli-" + hex.EncodeToString(b)
}

// Do sends a request. key may be empty for GETs.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, key string) (*Response, error) {
	u := strings.TrimRight(c.Endpoint, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, localErr(ExitInternal, "internal_error", err.Error())
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, localErr(ExitUsage, "invalid_request", "invalid endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, localErr(ExitInterrupt, "interrupted", "interrupted")
		}
		return nil, localErr(ExitUnavailable, "daemon_unavailable", "Cannot reach the 502drive daemon at "+redact(c.Endpoint)+". Is `502drive serve` running?")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, localErr(ExitUnavailable, "daemon_unavailable", "The daemon closed the connection.")
	}
	out := &Response{Status: resp.StatusCode, Header: resp.Header, Raw: raw}
	var env struct {
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
		Error     *struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			Retryable  bool   `json:"retryable"`
			RetryAfter *int   `json:"retry_after_seconds"`
		} `json:"error"`
	}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &env); err != nil && resp.StatusCode < 400 {
			return nil, localErr(ExitInternal, "bad_response", "The daemon returned an unreadable response.")
		}
	}
	out.Data, out.RequestID = env.Data, env.RequestID
	if resp.StatusCode >= 400 {
		e := &APIError{Status: resp.StatusCode, RequestID: env.RequestID, Exit: exitForStatus(resp.StatusCode), Code: "http_error", Message: fmt.Sprintf("The daemon returned HTTP %d.", resp.StatusCode)}
		if env.Error != nil {
			e.Code, e.Message, e.Retryable = env.Error.Code, env.Error.Message, env.Error.Retryable
			if env.Error.RetryAfter != nil {
				e.RetryAfter = *env.Error.RetryAfter
			}
		}
		if resp.StatusCode == 401 {
			e.Message = "The daemon rejected the API token. Check --token-file / DRIVE502_TOKEN_FILE."
		}
		return nil, e
	}
	return out, nil
}

func (r *Response) Decode(v any) error {
	if err := json.Unmarshal(r.Data, v); err != nil {
		return localErr(ExitInternal, "bad_response", "The daemon returned an unexpected shape.")
	}
	return nil
}

func redact(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "the configured endpoint"
	}
	u.User = nil
	u.RawQuery = ""
	return u.String()
}

// pollInterval is how often waits poll; tests shorten it.
var pollInterval = time.Second
