package power

import "log/slog"

// WakeOptions configures the opt-in "keep the machine awake while copying".
type WakeOptions struct {
	Log *slog.Logger
	// Reason is shown by the OS (logind) in its inhibitor list.
	Reason string
}

// WakeKeeper keeps the machine from sleeping on idle. Hold and Release are
// idempotent and safe to call from any goroutine; this is an optimisation, so
// every error is swallowed (the copy itself never depends on it).
type WakeKeeper interface {
	Hold()
	Release()
}
