package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// errRejected means the Bot API refused the token. Only a configuration change
// fixes it, so the adapter reports "error" instead of retrying blindly.
var errRejected = errors.New("telegram: bot token rejected")

// pollClient is the small polling seam: getUpdates is called here so the raw
// update can be stored before the offset advances. Everything else goes
// through the SDK.
type pollClient struct {
	base  string
	token string
	http  *http.Client
}

type apiEnvelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// apiError is a Bot API failure with the server's retry hint.
type apiError struct {
	method      string
	code        int
	description string
	retryAfter  time.Duration
}

func (e *apiError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.method, e.code, e.description)
}

func (c *pollClient) call(ctx context.Context, method string, params any, dest any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	endpoint := c.base + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return redactToken(err, c.token)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return redactToken(err, c.token)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return err
	}
	var env apiEnvelope
	if err = json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("telegram %s: malformed response", method)
	}
	if !env.OK {
		if env.ErrorCode == http.StatusUnauthorized || env.ErrorCode == http.StatusNotFound {
			return fmt.Errorf("%w: %s", errRejected, env.Description)
		}
		return &apiError{method: method, code: env.ErrorCode, description: env.Description,
			retryAfter: time.Duration(env.Parameters.RetryAfter) * time.Second}
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(env.Result, dest)
}

func (c *pollClient) getUpdates(ctx context.Context, offset int64, limit int, timeout time.Duration) ([]json.RawMessage, error) {
	params := map[string]any{
		"offset":          offset,
		"limit":           limit,
		"timeout":         int(timeout.Seconds()),
		"allowed_updates": []string{"message", "callback_query"},
	}
	var out []json.RawMessage
	err := c.call(ctx, "getUpdates", params, &out)
	return out, err
}

func (c *pollClient) getMe(ctx context.Context) error {
	return c.call(ctx, "getMe", map[string]any{}, nil)
}

// retryAfterOf reports the server hint and whether the error is retryable.
func retryAfterOf(err error) (time.Duration, bool) {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.retryAfter, ae.code == http.StatusTooManyRequests
	}
	return 0, false
}

// redactToken keeps the bot token out of transport errors: net/http wraps the
// request URL in a *url.Error, and that URL is ".../bot<token>/<method>". The
// SDK redacts its own errors the same way; the polling seam needs its own copy
// because it bypasses the SDK. Mutating in place keeps errors.Is working for
// context cancellation and timeouts.
func redactToken(err error, token string) error {
	if token == "" {
		return err
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		ue.URL = strings.ReplaceAll(ue.URL, token, "***")
	}
	return err
}
