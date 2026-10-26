//go:build !linux && !windows

package power

import "log/slog"

// NewSource reports that this platform has no notification adapter; the clock
// gap detector still catches sleeps.
func NewSource(*slog.Logger) (Source, error) { return EmptySource{}, nil }
