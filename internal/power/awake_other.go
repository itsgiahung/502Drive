//go:build !linux && !windows

package power

type noKeeper struct{}

func (noKeeper) Hold()    {}
func (noKeeper) Release() {}

// NewWakeKeeper returns a keeper that does nothing.
func NewWakeKeeper(*WakeOptions) WakeKeeper { return noKeeper{} }
