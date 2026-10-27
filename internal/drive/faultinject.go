package drive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	gdrive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// FaultAction defines the behavior when a fault rule matches.
type FaultAction int

const (
	FaultPass           FaultAction = iota // Pass through to base transport
	FaultThrottle429                       // Return 429 rateLimitExceeded with Retry-After header
	FaultDropConnection                    // Return a dial/network error simulating connection drop
	FaultLoseResponse                      // Return unexpected EOF simulating response lost after write
	FaultServerError                       // Return 500 / 503 HTTP server error
	FaultCustom                            // Execute custom handler function
)

// FaultRule specifies criteria and action for fault injection.
type FaultRule struct {
	Method       string // HTTP method match ("GET", "POST", etc., empty for any)
	PathContains string // Substring in URL to match (empty for any)
	Action       FaultAction
	StatusCode   int           // Custom HTTP status code
	RetryAfter   time.Duration // Retry-After duration for 429
	Err          error         // Custom error to return for drops
	Delay        time.Duration // Latency before action/response
	MaxHits      int           // Maximum times this rule can fire (0 = unlimited)
	SkipHits     int           // Number of matching requests to skip before firing
	Match        func(req *http.Request) bool
	Custom       func(req *http.Request) (*http.Response, error)
}

// FaultInjector wraps an http.RoundTripper and injects controlled faults
// such as 429 throttling, network disconnects, and response losses.
type FaultInjector struct {
	Base http.RoundTripper

	mu        sync.Mutex
	rules     []*FaultRule
	ruleHits  map[*FaultRule]int
	ruleSkips map[*FaultRule]int
	totalHits int
}

// NewFaultInjector constructs a FaultInjector wrapping base (or http.DefaultTransport if nil).
func NewFaultInjector(base http.RoundTripper) *FaultInjector {
	if base == nil {
		base = http.DefaultTransport
	}
	return &FaultInjector{
		Base:      base,
		rules:     make([]*FaultRule, 0),
		ruleHits:  make(map[*FaultRule]int),
		ruleSkips: make(map[*FaultRule]int),
	}
}

// AddRule appends a fault rule to the injector.
func (f *FaultInjector) AddRule(rule *FaultRule) {
	if rule == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = append(f.rules, rule)
}

// ClearRules removes all active fault rules and resets counters.
func (f *FaultInjector) ClearRules() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = f.rules[:0]
	f.ruleHits = make(map[*FaultRule]int)
	f.ruleSkips = make(map[*FaultRule]int)
}

// Hits returns the total number of faults injected.
func (f *FaultInjector) Hits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.totalHits
}

// Inject429 adds a rule injecting HTTP 429 rate limit exceeded.
func (f *FaultInjector) Inject429(pathContains string, maxHits int, retryAfter time.Duration) {
	f.AddRule(&FaultRule{
		PathContains: pathContains,
		Action:       FaultThrottle429,
		MaxHits:      maxHits,
		RetryAfter:   retryAfter,
	})
}

// InjectDrop adds a rule simulating an unreachable network dial error.
func (f *FaultInjector) InjectDrop(pathContains string, maxHits int) {
	f.AddRule(&FaultRule{
		PathContains: pathContains,
		Action:       FaultDropConnection,
		MaxHits:      maxHits,
	})
}

// InjectLoseResponse adds a rule simulating losing response after write.
func (f *FaultInjector) InjectLoseResponse(pathContains string, maxHits int) {
	f.AddRule(&FaultRule{
		PathContains: pathContains,
		Action:       FaultLoseResponse,
		MaxHits:      maxHits,
	})
}

// RoundTrip implements http.RoundTripper.
func (f *FaultInjector) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	var matched *FaultRule
	for _, r := range f.rules {
		if r.Method != "" && !strings.EqualFold(r.Method, req.Method) {
			continue
		}
		if r.PathContains != "" && !strings.Contains(req.URL.String(), r.PathContains) {
			continue
		}
		if r.Match != nil && !r.Match(req) {
			continue
		}
		if f.ruleSkips[r] < r.SkipHits {
			f.ruleSkips[r]++
			continue
		}
		if r.MaxHits > 0 && f.ruleHits[r] >= r.MaxHits {
			continue
		}
		f.ruleHits[r]++
		f.totalHits++
		matched = r
		break
	}
	f.mu.Unlock()

	if matched != nil && matched.Delay > 0 {
		select {
		case <-time.After(matched.Delay):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}

	if matched != nil {
		switch matched.Action {
		case FaultThrottle429:
			code := http.StatusTooManyRequests
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			secs := int(matched.RetryAfter.Seconds())
			if secs <= 0 {
				secs = 5
			}
			header.Set("Retry-After", strconv.Itoa(secs))
			body := []byte(fmt.Sprintf(`{
				"error": {
					"code": 429,
					"message": "Rate limit exceeded",
					"errors": [{
						"message": "Rate limit exceeded",
						"domain": "usageLimits",
						"reason": "rateLimitExceeded"
					}]
				}
			}`))
			return &http.Response{
				StatusCode: code,
				Status:     fmt.Sprintf("%d %s", code, http.StatusText(code)),
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     header,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Request:    req,
			}, nil

		case FaultDropConnection:
			if matched.Err != nil {
				return nil, matched.Err
			}
			return nil, &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: errors.New("network unreachable: simulated drop"),
			}

		case FaultLoseResponse:
			if matched.Err != nil {
				return nil, matched.Err
			}
			return nil, io.ErrUnexpectedEOF

		case FaultServerError:
			code := matched.StatusCode
			if code == 0 {
				code = http.StatusInternalServerError
			}
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			body := []byte(fmt.Sprintf(`{"error": {"code": %d, "message": "Server error"}}`, code))
			return &http.Response{
				StatusCode: code,
				Status:     fmt.Sprintf("%d %s", code, http.StatusText(code)),
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     header,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Request:    req,
			}, nil

		case FaultCustom:
			if matched.Custom != nil {
				return matched.Custom(req)
			}
		}
	}

	base := f.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// NewClientWithTransport builds a Client whose HTTP calls route through
// the provided RoundTripper, enabling fault injection for both live Drive
// integration tests and local servers.
func NewClientWithTransport(ctx context.Context, ts oauth2.TokenSource, endpoint string, transport http.RoundTripper) (*Client, error) {
	if transport == nil {
		transport = http.DefaultTransport
	}
	baseHC := &http.Client{Timeout: 60 * time.Second, Transport: transport}
	var hc *http.Client
	if ts != nil {
		hc = oauth2.NewClient(context.WithValue(ctx, oauth2.HTTPClient, baseHC), ts)
	} else {
		hc = baseHC
	}
	opts := []option.ClientOption{option.WithHTTPClient(hc)}
	if endpoint != "" {
		opts = append(opts, option.WithEndpoint(endpoint))
	}
	svc, err := gdrive.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc, hc: hc}, nil
}
