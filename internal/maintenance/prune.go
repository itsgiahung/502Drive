package maintenance

import (
	"context"
	"strings"

	"github.com/itsgiahung/502Drive/internal/state"
)

// Prune cleans up historical finished jobs, events, telegram updates, outbox messages,
// and write intents offline using state.Prune. It refuses to run if instance.lock
// is held by an active daemon.
func Prune(ctx context.Context, dataDir string, opt state.PruneOptions) (state.PruneReport, error) {
	unlock, err := LockDataDir(dataDir)
	if err != nil {
		return state.PruneReport{}, err
	}
	unlock()

	store, err := state.Open(dataDir)
	if err != nil {
		if strings.Contains(err.Error(), "already has a writer") {
			return state.PruneReport{}, ErrWriterRunning
		}
		return state.PruneReport{}, err
	}
	defer store.Close()

	return store.Prune(ctx, opt)
}
