package telegram

import (
	"context"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/itsgiahung/502Drive/internal/state"
	"time"
)

func (t *Telegram) progressView(ctx context.Context, principal string, j state.Job) jobView {
	v := t.jobViewFor(ctx, principal)
	t.progressMu.Lock()
	defer t.progressMu.Unlock()
	if t.progress == nil {
		t.progress = map[string][]present.Sample{}
	}
	for id, samples := range t.progress {
		if len(samples) > 0 && t.now().Sub(samples[len(samples)-1].At) > time.Minute {
			delete(t.progress, id)
		}
	}
	if j.State != state.JobRunning {
		delete(t.progress, j.ID)
		return v
	}
	samples := present.AppendSample(t.progress[j.ID], present.Sample{At: t.now(), Completed: j.Counts.Completed})
	t.progress[j.ID] = samples
	v.samples = samples
	return v
}
