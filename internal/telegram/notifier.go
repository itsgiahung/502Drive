package telegram

import (
	"context"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Notification kinds map a finished job to the preference that gates it.
const (
	noticeCompleted = "completed"
	noticeFailed    = "failed"
	noticeAction    = "action_required"
)

// Notifier turns finished jobs and subscription failures into durable
// Telegram notifications for the principal that owns them. It is used for work
// the user did not start in a chat; chat jobs already have a progress panel.
type Notifier struct {
	svc     *app.Service
	store   *state.Store
	ownerID int64
	now     func() time.Time
}

// NotifierOptions wires the notifier to the service and the owner's chat id.
type NotifierOptions struct {
	Service *app.Service
	OwnerID int64
	Now     func() time.Time
}

func NewNotifier(o NotifierOptions) *Notifier {
	n := &Notifier{svc: o.Service, store: o.Service.Store, ownerID: o.OwnerID, now: o.Now}
	if n.now == nil {
		n.now = time.Now
	}
	return n
}

// JobFinished enqueues at most one notification for a terminal job, if the
// owner asked for that class of event.
func (n *Notifier) JobFinished(ctx context.Context, jobID string) error {
	job, err := n.svc.Job(ctx, app.Owner, jobID)
	if err != nil {
		return err
	}
	kind, ok := notificationKind(job.State)
	if !ok {
		return nil
	}
	prefs, err := n.store.Preferences(ctx, job.Principal)
	if err != nil {
		return err
	}
	if !notifyEnabled(prefs, kind) {
		return nil
	}
	if kind == noticeCompleted && job.Trigger != "initial" {
		// A sync run that touched nothing changed nothing; stay quiet, but
		// forget an earlier failure so the next one is announced again.
		touched, terr := n.store.JobTouchedItems(ctx, jobID)
		if terr != nil {
			return terr
		}
		if touched == 0 {
			return n.store.ClearNotice(ctx, job.Principal, job.CollectionID)
		}
	}
	panel, err := n.store.HasProgressPanel(ctx, job.Principal, jobID)
	if err != nil {
		return err
	}
	if panel {
		return nil
	}
	if kind == noticeCompleted {
		if err = n.store.ClearNotice(ctx, job.Principal, job.CollectionID); err != nil {
			return err
		}
	} else {
		fresh, cerr := n.store.ClaimNotice(ctx, job.Principal, job.CollectionID, noticeCode(job))
		if cerr != nil {
			return cerr
		}
		if !fresh {
			return nil
		}
	}
	chat, ok, err := n.chat(ctx, job.Principal)
	if err != nil || !ok {
		return err
	}
	_, err = n.store.EnqueueFinal(ctx, job.Principal, jobID, chat, noticeCode(job), "final:"+jobID, n.now())
	return err
}

// SubscriptionHealthy clears the announced error of a subscription, so a later
// failure is announced again.
func (n *Notifier) SubscriptionHealthy(ctx context.Context, collectionID string) error {
	principal, err := n.store.CollectionOwner(ctx, collectionID)
	if err != nil {
		return err
	}
	return n.store.ClearNotice(ctx, principal, collectionID)
}

// SubscriptionError announces one distinct error of a subscription. A repeating
// failure stays silent until the error changes or a run succeeds.
func (n *Notifier) SubscriptionError(ctx context.Context, collectionID, code string) error {
	principal, err := n.store.CollectionOwner(ctx, collectionID)
	if err != nil {
		return err
	}
	prefs, err := n.store.Preferences(ctx, principal)
	if err != nil {
		return err
	}
	if !prefs.NotifyActionRequired && !prefs.NotifyFailed {
		return nil
	}
	fresh, err := n.store.ClaimNotice(ctx, principal, collectionID, code)
	if err != nil || !fresh {
		return err
	}
	chat, ok, err := n.chat(ctx, principal)
	if err != nil || !ok {
		return err
	}
	_, err = n.store.EnqueueFinal(ctx, principal, "", chat, code, "", n.now())
	return err
}

// chat resolves the principal's chat: the configured owner id, or an enabled
// Telegram operator.
func (n *Notifier) chat(ctx context.Context, principal string) (int64, bool, error) {
	if principal == app.Owner.ID {
		return n.ownerID, n.ownerID > 0, nil
	}
	return n.store.TelegramChat(ctx, principal)
}

// notificationKind maps a job state to the preference class, or false when the
// state is not worth a message (queued, cancelled, ...).
func notificationKind(jobState string) (string, bool) {
	switch jobState {
	case state.JobSucceeded:
		return noticeCompleted, true
	case state.JobPartial, state.JobFailed:
		return noticeFailed, true
	case state.JobBlocked:
		return noticeAction, true
	}
	return "", false
}

func notifyEnabled(prefs state.Preferences, kind string) bool {
	switch kind {
	case noticeCompleted:
		return prefs.NotifyCompleted
	case noticeFailed:
		return prefs.NotifyFailed
	case noticeAction:
		return prefs.NotifyActionRequired
	}
	return false
}

// noticeCode is the error identity used to announce a failure once.
func noticeCode(job state.Job) string {
	code := job.State
	if job.LastError != nil && job.LastError.Code != "" {
		code += ":" + job.LastError.Code
	}
	return code
}
