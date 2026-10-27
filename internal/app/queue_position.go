package app

import (
	"context"
	"github.com/itsgiahung/502Drive/internal/state"
)

func (s *Service) jobQueuePositions(ctx context.Context, jobs []state.Job) error {
	queued := false
	for _, j := range jobs {
		queued = queued || j.State == state.JobQueued
	}
	if !queued {
		return nil
	}
	positions, err := s.Store.QueuePositions(ctx)
	if err != nil {
		return err
	}
	paused, err := s.Store.QueuePaused(ctx)
	if err != nil {
		return err
	}
	for i := range jobs {
		j := &jobs[i]
		if pos, ok := positions[j.ID]; ok && j.State == state.JobQueued {
			ahead := pos - 1
			j.QueuePosition = &pos
			j.QueueAhead = &ahead
			j.QueueIsPaused = paused
		}
	}
	return nil
}
