package telegram

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// The queue (S6) in chat: the owner can pause and resume every job at once
// (the same POST /v1/queue/pause the CLI uses), and any caller can move one of
// their own waiting jobs to the front or the back or give it a priority.
// Moves are bound to the revision on the button, so a card that went stale
// after another device acted is refused and the current truth is shown.

const (
	queueScanMax = 500
	// prioHigh is the priority the "Prioritise" button sets (queue range -5..5).
	prioHigh = 1
)

func (t *Telegram) queuePaused(ctx context.Context) bool {
	st, err := t.svc.Status(ctx)
	return err == nil && st.QueuePaused
}

// queueControls is the paused notice and, for the owner only, the
// pause/resume button. Operators cannot pause everybody's jobs, so they get
// the notice and no button.
func (t *Telegram) queueControls(ctx context.Context, p app.Principal, tx texts, payload string) (string, []button) {
	ig := tx.ig()
	paused := t.queuePaused(ctx)
	line := ""
	if paused {
		line = ig.queuePaused
	}
	if p.Role != app.RoleOwner {
		return line, nil
	}
	if paused {
		return line, []button{styled(act(ig.queueResumeBtn, "queue_resume", "", 0, payload), "success")}
	}
	return line, []button{act(ig.queuePauseBtn, "queue_pause", "", 0, payload)}
}

// actQueue pauses or resumes the whole queue, then shows where it came from.
func (t *Telegram) actQueue(ctx context.Context, p app.Principal, st state.Callback) error {
	if _, err := t.svc.QueuePause(ctx, p, st.Action == "queue_pause"); err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	if bp, ok := decodeBatch(st.Payload); ok {
		return t.batchCard(ctx, p, st.ChatID, st.MessageID, bp)
	}
	return t.page(ctx, p, st.ChatID, st.MessageID, "jobs")
}

// queueOrder returns the waiting jobs the caller can see in the order the
// queue serves them: priority first, then the queue order, then admission.
// (The engine also shares workers fairly between batches, so a batch can be
// interleaved with other jobs; the order here is the one moves act on.)
func (t *Telegram) queueOrder(ctx context.Context, p app.Principal) ([]state.Job, error) {
	jobs, _, err := t.jobsWhere(ctx, p, p.Role == app.RoleOwner, state.JobQueued, "", queueScanMax)
	if err != nil {
		return nil, err
	}
	slices.Reverse(jobs) // oldest admission first, so ties keep their order
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].Priority != jobs[j].Priority {
			return jobs[i].Priority > jobs[j].Priority
		}
		return jobs[i].QueueOrder < jobs[j].QueueOrder
	})
	return jobs, nil
}

// queueInfo is the "place in the queue" line and the move buttons of a
// waiting job; nothing for any other state.
func (t *Telegram) queueInfo(ctx context.Context, p app.Principal, tx texts, j state.Job) (string, [][]button) {
	if j.State != state.JobQueued {
		return "", nil
	}
	order, err := t.queueOrder(ctx, p)
	if err != nil {
		return "", nil
	}
	pos := slices.IndexFunc(order, func(q state.Job) bool { return q.ID == j.ID })
	if pos < 0 {
		return "", nil
	}
	ig := tx.ig()
	var moves []button
	if pos > 0 {
		moves = append(moves, act(ig.moveTop, "job_move", j.ID, j.Revision, "top"))
	}
	if pos < len(order)-1 {
		moves = append(moves, act(ig.moveBottom, "job_move", j.ID, j.Revision, "bottom"))
	}
	var rows [][]button
	if len(moves) > 0 {
		rows = append(rows, moves)
	}
	if j.Priority < prioHigh {
		rows = append(rows, []button{act(ig.movePrio, "job_move", j.ID, j.Revision, "prio")})
	} else {
		rows = append(rows, []button{act(ig.moveNormal, "job_move", j.ID, j.Revision, "normal")})
	}
	return fmt.Sprintf(ig.queuePos, pos+1, len(order)), rows
}

// actJobMove applies one move at the revision on the button. The target is
// read again at click time, so a queue that changed meanwhile is not guessed.
func (t *Telegram) actJobMove(ctx context.Context, p app.Principal, st state.Callback) error {
	var m state.Move
	switch st.Payload {
	case "top", "bottom":
		order, err := t.queueOrder(ctx, p)
		if err != nil {
			return t.showError(ctx, p, st.ChatID, st.MessageID, err)
		}
		if len(order) > 0 {
			if st.Payload == "top" && order[0].ID != st.Object {
				m.Before = order[0].ID
			}
			if st.Payload == "bottom" && order[len(order)-1].ID != st.Object {
				m.After = order[len(order)-1].ID
			}
		}
	case "prio":
		v := prioHigh
		m.Priority = &v
	case "normal":
		v := 0
		m.Priority = &v
	}
	if m.Before != "" || m.After != "" || m.Priority != nil {
		if _, err := t.svc.MoveJob(ctx, p, st.Object, st.Revision, m); err != nil {
			// A stale card is expected after another device acted; show the truth.
			if j, jerr := t.svc.Job(ctx, p, st.Object); jerr == nil {
				_, cardErr := t.jobCard(ctx, p, st.ChatID, st.MessageID, j, false)
				return cardErr
			}
			return t.showError(ctx, p, st.ChatID, st.MessageID, err)
		}
	}
	j, err := t.svc.Job(ctx, p, st.Object)
	if err != nil {
		return t.showError(ctx, p, st.ChatID, st.MessageID, err)
	}
	_, err = t.jobCard(ctx, p, st.ChatID, st.MessageID, j, false)
	return err
}
