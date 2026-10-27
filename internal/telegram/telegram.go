// Package telegram is the Telegram adapter: it long-polls one bot, stores
// every update durably, resolves the caller to an app.Principal and drives the
// same application service the HTTP API uses. It never touches SQL directly.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Bot states reported through Service.BotState.
const (
	StateDisabled = "disabled"
	StateStarting = "starting"
	StatePolling  = "polling"
	StateError    = "error"
)

const (
	callbackTTL    = 15 * time.Minute
	callbackPrefix = "cb_"
	navPrefix      = "nav:"
	maxBatch       = 100
)

type Options struct {
	Service *app.Service
	Token   string
	// OwnerID is the configured telegram_owner_id. Zero means only operators.
	OwnerID int64
	// APIURL defaults to https://api.telegram.org.
	APIURL string
	Log    *slog.Logger
	// PollTimeout is the getUpdates long-poll window.
	PollTimeout time.Duration
	// EditInterval is the minimum spacing between two edits of one progress
	// panel.
	EditInterval time.Duration
	HTTPClient   *http.Client
	Now          func() time.Time
}

// Telegram is one long-polling bot. Create it with New and run it with Run.
type Telegram struct {
	svc      *app.Service
	store    *state.Store
	ownerID  int64
	bot      *bot.Bot
	poll     *pollClient
	log      *slog.Logger
	instance string

	pollTimeout time.Duration
	editEvery   time.Duration
	now         func() time.Time
	// reportWindow is how far back the daily report looks (24h when zero).
	reportWindow time.Duration

	state atomic.Value // string
	proc  sync.Mutex
	// names remembers each chat's first name for the welcome card (memory only).
	names sync.Map // int64 -> string
	// await holds the free-text answer a chat owes (rename / new folder name).
	await sync.Map // int64 -> awaitState
}

func New(ctx context.Context, o Options) (*Telegram, error) {
	if o.Service == nil || o.Service.Store == nil {
		return nil, errors.New("telegram: application service required")
	}
	token := strings.TrimSpace(o.Token)
	if token == "" {
		return nil, errors.New("telegram: bot token required")
	}
	if strings.ContainsAny(token, " \t\r\n/") {
		return nil, errors.New("telegram: bot token is malformed")
	}
	base := strings.TrimRight(o.APIURL, "/")
	if base == "" {
		base = "https://api.telegram.org"
	}
	timeout := o.PollTimeout
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	client := o.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout + 20*time.Second}
	}
	b, err := bot.New(token, bot.WithServerURL(base), bot.WithSkipGetMe(), bot.WithHTTPClient(timeout, client))
	if err != nil {
		return nil, err
	}
	instance, err := o.Service.Store.TelegramInstance(ctx)
	if err != nil {
		return nil, err
	}
	t := &Telegram{svc: o.Service, store: o.Service.Store, ownerID: o.OwnerID, bot: b,
		poll: &pollClient{base: base, token: token, http: client}, instance: instance,
		pollTimeout: timeout, editEvery: o.EditInterval, now: o.Now, log: o.Log}
	if t.log == nil {
		t.log = slog.New(slog.DiscardHandler)
	}
	if t.editEvery <= 0 {
		t.editEvery = 3 * time.Second
	}
	if t.now == nil {
		t.now = time.Now
	}
	t.state.Store(StateStarting)
	return t, nil
}

// State reports disabled|starting|polling|error for /v1/status.
func (t *Telegram) State() string {
	if v, ok := t.state.Load().(string); ok {
		return v
	}
	return StateStarting
}

func (t *Telegram) setState(s string) { t.state.Store(s) }

// Run polls until ctx is cancelled. Transient Bot API failures keep the daemon
// running; the adapter only reports them as "error".
func (t *Telegram) Run(ctx context.Context) {
	t.setState(StateStarting)
	go t.notifyLoop(ctx)
	go t.reportLoop(ctx)
	go t.retryLoop(ctx)
	t.processPending(ctx)

	var wait time.Duration
	var verified bool
	for ctx.Err() == nil {
		if !verified {
			if err := t.poll.getMe(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				t.fail("getMe", err)
				wait = nextWait(wait, err)
				t.sleep(ctx, wait)
				continue
			}
			verified = true
			// The token works: report polling now, not after the first 30 s long poll.
			t.setState(StatePolling)
		}
		offset, err := t.store.TelegramOffset(ctx, t.instance)
		if err != nil {
			t.log.Error("telegram: cannot read offset", "err", err)
			t.setState(StateError)
			t.sleep(ctx, time.Second)
			continue
		}
		updates, err := t.poll.getUpdates(ctx, offset, maxBatch, t.pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, errRejected) {
				verified = false
			}
			t.fail("getUpdates", err)
			wait = nextWait(wait, err)
			t.sleep(ctx, wait)
			continue
		}
		wait = 0
		t.setState(StatePolling)
		if len(updates) == 0 {
			continue
		}
		if err = t.ingest(ctx, updates); err != nil {
			// The offset did not advance: the same batch comes back.
			t.log.Error("telegram: cannot store updates", "err", err)
			t.sleep(ctx, time.Second)
			continue
		}
		t.processPending(ctx)
	}
}

// ingest stores a batch and advances the offset in one transaction. It is the
// only place that moves the polling cursor.
func (t *Telegram) ingest(ctx context.Context, updates []json.RawMessage) error {
	items := make([]state.Inbox, 0, len(updates))
	var next int64
	for _, raw := range updates {
		var head struct {
			ID int64 `json:"update_id"`
		}
		if json.Unmarshal(raw, &head) != nil || head.ID <= 0 {
			continue
		}
		items = append(items, state.Inbox{UpdateID: head.ID, Body: append([]byte(nil), raw...)})
		if head.ID+1 > next {
			next = head.ID + 1
		}
	}
	if len(items) == 0 {
		return nil
	}
	_, err := t.store.IngestUpdates(ctx, t.instance, items, next)
	return err
}

// processPending handles stored updates. Updates that failed stay pending for
// a bounded retry; replaying them is safe because the admission key comes from
// the update id.
func (t *Telegram) processPending(ctx context.Context) {
	t.proc.Lock()
	defer t.proc.Unlock()
	for round := 0; round < 20 && ctx.Err() == nil; round++ {
		items, err := t.store.PendingUpdates(ctx, t.instance, 8)
		if err != nil {
			t.log.Error("telegram: cannot read inbox", "err", err)
			return
		}
		if len(items) == 0 {
			return
		}
		for _, it := range items {
			code := ""
			if err := t.handle(ctx, it.Body); err != nil {
				code = errorCode(err)
				t.log.Warn("telegram: update failed", "update_id", it.UpdateID, "code", code)
			}
			if err := t.store.FinishUpdate(ctx, t.instance, it.UpdateID, code); err != nil {
				t.log.Error("telegram: cannot record update", "err", err)
				return
			}
		}
	}
}

func (t *Telegram) retryLoop(ctx context.Context) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.processPending(ctx)
		}
	}
}

func (t *Telegram) handle(ctx context.Context, raw []byte) error {
	var u models.Update
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil // undecodable updates are dropped, not retried forever
	}
	switch {
	case u.CallbackQuery != nil:
		return t.onCallback(ctx, u.CallbackQuery)
	case u.Message != nil:
		return t.onMessage(ctx, u.ID, u.Message)
	}
	return nil
}

// principal maps a verified Telegram user id to a principal. Unknown users are
// ignored silently.
func (t *Telegram) principal(ctx context.Context, telegramID int64) (app.Principal, bool, error) {
	if telegramID <= 0 {
		return app.Principal{}, false, nil
	}
	if t.ownerID > 0 && telegramID == t.ownerID {
		return app.Owner, true, nil
	}
	id, ok, err := t.store.OperatorByTelegram(ctx, telegramID)
	if err != nil || !ok {
		return app.Principal{}, false, err
	}
	return app.Principal{ID: id, Role: app.RoleOperator}, true, nil
}

func (t *Telegram) sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		d = time.Second
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (t *Telegram) fail(method string, err error) {
	code := "unavailable"
	if errors.Is(err, errRejected) {
		code = "unauthorized"
	}
	t.setState(StateError)
	t.log.Warn("telegram: bot API failed", "method", method, "code", code, "err", err.Error())
}

func nextWait(prev time.Duration, err error) time.Duration {
	if d, ok := retryAfterOf(err); ok && d > 0 {
		return d
	}
	if errors.Is(err, errRejected) {
		return 30 * time.Second
	}
	if prev <= 0 {
		return time.Second
	}
	if prev *= 2; prev > 30*time.Second {
		return 30 * time.Second
	}
	return prev
}

func errorCode(err error) string {
	var ae *app.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	if errors.Is(err, state.ErrCallbackStale) {
		return "callback_stale"
	}
	return "internal"
}
