// Package app is the single application service shared by the HTTP API and the
// Telegram adapter. Adapters never touch SQL or the engine directly.
package app

import (
	"errors"
	"net/http"
	"time"

	"github.com/GiaHung07/502Drive/internal/drive"
	"github.com/GiaHung07/502Drive/internal/state"
)

// Error is a stable, user-presentable failure.
type Error struct {
	Status     int
	Code       string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func Fail(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

// Classify maps any internal error to an *Error; internals never leak.
func Classify(err error) *Error {
	var ae *Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ae):
		return ae
	case errors.Is(err, state.ErrNotFound):
		return Fail(http.StatusNotFound, "not_found", "Not found.")
	case errors.Is(err, state.ErrMismatch):
		return Fail(http.StatusConflict, "idempotency_mismatch", "This idempotency key was already used for a different request.")
	case errors.Is(err, state.ErrStale):
		return Fail(http.StatusConflict, "stale_revision", "This item changed. Refresh and try again.")
	case errors.Is(err, state.ErrExpired):
		return Fail(http.StatusConflict, "plan_expired", "Inspect the source again.")
	case errors.Is(err, state.ErrPlanChanged):
		return Fail(http.StatusConflict, "plan_changed", "The destination changed after the preview; inspect again.")
	case errors.Is(err, state.ErrTransition):
		return Fail(http.StatusConflict, "invalid_transition", "That action is not available in the current state.")
	case errors.Is(err, state.ErrNeedsAction):
		return Fail(http.StatusConflict, "needs_action", "The copied source needs attention before it can sync.")
	case errors.Is(err, state.ErrCapacity):
		e := Fail(http.StatusTooManyRequests, "capacity_exceeded", "Too many jobs are running. Try again later.")
		e.Retryable, e.RetryAfter = true, 30*time.Second
		return e
	case errors.Is(err, drive.ErrNotConnected), errors.Is(err, drive.ErrReconnectRequired):
		return Fail(http.StatusForbidden, "account_reconnect_required", "Connect the Google account first.")
	}
	switch drive.KindOf(err) {
	case drive.KindNotFound:
		return Fail(http.StatusNotFound, "source_not_found", "The Drive item was not found or is not shared with this account.")
	case drive.KindPermission:
		return Fail(http.StatusForbidden, "permission_denied", "This account is not allowed to access that item.")
	case drive.KindAuth:
		return Fail(http.StatusForbidden, "account_reconnect_required", "Connect the Google account again.")
	case drive.KindThrottled, drive.KindQuota:
		e := Fail(http.StatusTooManyRequests, "rate_limited", "Google Drive is limiting requests. Try again shortly.")
		e.Retryable, e.RetryAfter = true, max(drive.RetryAfterOf(err), 5*time.Second)
		return e
	case drive.KindUnavailable, drive.KindUnreachable, drive.KindCancelled:
		e := Fail(http.StatusBadGateway, "provider_unavailable", "Google Drive is not reachable right now.")
		e.Retryable = true
		return e
	}
	return Fail(http.StatusInternalServerError, "internal_error", "Internal error.")
}
