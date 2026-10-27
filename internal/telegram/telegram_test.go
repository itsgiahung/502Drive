package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/state"
)

const testBotToken = "424242:test-token"

// ---- fake Bot API ----

type fakeMessage struct {
	ChatID      int64
	MessageID   int
	Text        string
	Buttons     []string
	ButtonTexts []string
	Styles      []string
	// Persistent reply keyboard, when the message installs one.
	Reply       []string
	Persistent  bool
	Placeholder string
}

// fakeDocument is one file the bot uploaded with sendDocument.
type fakeDocument struct {
	ChatID   int64
	Filename string
	Content  []byte
	Caption  string
}

type queued struct {
	id   int64
	body []byte
}

type fakeAPI struct {
	t     *testing.T
	token string
	srv   *httptest.Server

	mu        sync.Mutex
	updates   []queued
	sent      []fakeMessage
	edits     []fakeMessage
	log       []fakeMessage
	answers   []string
	docs      []fakeDocument
	nextID    int
	polls     int
	getMeCode int
	sendErr   func(call int) (code int, description string, retryAfter int)
	editErr   func(call int) (code int, description string, retryAfter int)
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, token: testBotToken}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) push(body []byte) {
	var head struct {
		ID int64 `json:"update_id"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		f.t.Fatalf("bad update fixture: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, queued{id: head.ID, body: body})
}

func (f *fakeAPI) messages() (sent, edits []fakeMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeMessage(nil), f.sent...), append([]fakeMessage(nil), f.edits...)
}

func (f *fakeAPI) cards() []fakeMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeMessage(nil), f.log...)
}

func (f *fakeAPI) documents() []fakeDocument {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeDocument(nil), f.docs...)
}

func (f *fakeAPI) answerTexts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.answers...)
}

func (f *fakeAPI) setEditErr(fn func(int) (int, string, int)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.editErr = fn
}

func (f *fakeAPI) setSendErr(fn func(int) (int, string, int)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendErr = fn
}

func (f *fakeAPI) write(w http.ResponseWriter, payload any) {
	b, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func (f *fakeAPI) fail(w http.ResponseWriter, code int, description string, retryAfter int) {
	f.write(w, map[string]any{"ok": false, "error_code": code, "description": description,
		"parameters": map[string]any{"retry_after": retryAfter}})
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	method := strings.TrimPrefix(r.URL.Path, "/bot"+f.token+"/")
	switch method {
	case "getMe":
		f.mu.Lock()
		code := f.getMeCode
		f.mu.Unlock()
		if code != 0 {
			f.fail(w, code, "Unauthorized", 0)
			return
		}
		f.write(w, map[string]any{"ok": true, "result": map[string]any{"id": 999, "is_bot": true, "first_name": "502"}})
	case "getUpdates":
		var p struct {
			Offset int64 `json:"offset"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &p)
		f.mu.Lock()
		f.polls++
		var parts []string
		for _, u := range f.updates {
			if u.id >= p.Offset {
				parts = append(parts, string(u.body))
			}
		}
		f.mu.Unlock()
		if len(parts) == 0 {
			// Mimic long polling so the loop does not spin.
			time.Sleep(10 * time.Millisecond)
		}
		f.write(w, map[string]any{"ok": true, "result": json.RawMessage("[" + strings.Join(parts, ",") + "]")})
	case "sendMessage", "editMessageText":
		if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
			f.t.Errorf("cannot parse %s body: %v", method, err)
		}
		chatID, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
		messageID, _ := strconv.Atoi(r.FormValue("message_id"))
		m := fakeMessage{ChatID: chatID, MessageID: messageID, Text: r.FormValue("text")}
		if markup := r.FormValue("reply_markup"); markup != "" {
			var kb struct {
				InlineKeyboard [][]struct {
					Text         string `json:"text"`
					CallbackData string `json:"callback_data"`
					Style        string `json:"style"`
				} `json:"inline_keyboard"`
				Keyboard [][]struct {
					Text string `json:"text"`
				} `json:"keyboard"`
				IsPersistent bool   `json:"is_persistent"`
				Placeholder  string `json:"input_field_placeholder"`
			}
			if err := json.Unmarshal([]byte(markup), &kb); err != nil {
				f.t.Errorf("bad reply_markup %q: %v", markup, err)
			}
			for _, row := range kb.InlineKeyboard {
				for _, b := range row {
					m.Buttons = append(m.Buttons, b.CallbackData)
					m.ButtonTexts = append(m.ButtonTexts, b.Text)
					m.Styles = append(m.Styles, b.Style)
				}
			}
			for _, row := range kb.Keyboard {
				for _, b := range row {
					m.Reply = append(m.Reply, b.Text)
				}
			}
			m.Persistent, m.Placeholder = kb.IsPersistent, kb.Placeholder
		}
		f.mu.Lock()
		f.nextID++
		if method == "sendMessage" {
			m.MessageID = f.nextID
			f.sent = append(f.sent, m)
			f.log = append(f.log, m)
			if f.sendErr != nil {
				if code, desc, ra := f.sendErr(len(f.sent)); code != 0 {
					f.mu.Unlock()
					f.fail(w, code, desc, ra)
					return
				}
			}
			f.mu.Unlock()
			f.write(w, map[string]any{"ok": true, "result": map[string]any{
				"message_id": m.MessageID, "date": 1700000000,
				"chat": map[string]any{"id": chatID, "type": "private"}, "text": m.Text}})
			return
		}
		f.edits = append(f.edits, m)
		f.log = append(f.log, m)
		if f.editErr != nil {
			if code, desc, ra := f.editErr(len(f.edits)); code != 0 {
				f.mu.Unlock()
				f.fail(w, code, desc, ra)
				return
			}
		}
		f.mu.Unlock()
		f.write(w, map[string]any{"ok": true, "result": map[string]any{
			"message_id": m.MessageID, "date": 1700000000,
			"chat": map[string]any{"id": chatID, "type": "private"}, "text": m.Text}})
	case "sendDocument":
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			f.t.Errorf("cannot parse %s body: %v", method, err)
		}
		chatID, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
		doc := fakeDocument{ChatID: chatID, Caption: r.FormValue("caption")}
		if r.MultipartForm != nil {
			for _, headers := range r.MultipartForm.File {
				for _, fh := range headers {
					doc.Filename = fh.Filename
					file, err := fh.Open()
					if err != nil {
						f.t.Errorf("cannot open the uploaded file: %v", err)
						continue
					}
					doc.Content, _ = io.ReadAll(file)
					file.Close()
				}
			}
		}
		f.mu.Lock()
		f.nextID++
		f.docs = append(f.docs, doc)
		id := f.nextID
		f.mu.Unlock()
		f.write(w, map[string]any{"ok": true, "result": map[string]any{
			"message_id": id, "date": 1700000000,
			"chat": map[string]any{"id": chatID, "type": "private"}}})
	case "answerCallbackQuery":
		if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
			f.t.Errorf("cannot parse %s body: %v", method, err)
		}
		f.mu.Lock()
		f.answers = append(f.answers, r.FormValue("text"))
		f.mu.Unlock()
		f.write(w, map[string]any{"ok": true, "result": true})
	default:
		f.t.Errorf("unexpected Bot API method %q", method)
		f.fail(w, 404, "Not Found", 0)
	}
}

// ---- harness ----

type harness struct {
	t       *testing.T
	store   *state.Store
	drive   *drivetest.Fake
	svc     *app.Service
	api     *fakeAPI
	tg      *Telegram
	src     drive.File
	dest    state.Destination
	ownerID int64
	updates int64

	mu  sync.Mutex
	now time.Time
}

func newHarness(t *testing.T, ownerID int64) *harness { return buildHarness(t, ownerID, true) }

// newBareHarness has no destination at all: the first copy must still work.
func newBareHarness(t *testing.T, ownerID int64) *harness { return buildHarness(t, ownerID, false) }

func buildHarness(t *testing.T, ownerID int64, withDest bool) *harness {
	t.Helper()
	h := &harness{t: t, ownerID: ownerID, now: time.Now().UTC()}
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	h.store = store
	h.drive = drivetest.New()
	h.src = h.drive.Folder("Project Alpha", "root")
	for i := range 3 {
		h.drive.File(fmt.Sprintf("doc-%d.txt", i), h.src.ID, 12)
	}
	h.svc = &app.Service{Store: store, Version: "test", Now: h.clock,
		Provider: func(context.Context) (drive.Provider, error) { return h.drive, nil }}
	h.api = newFakeAPI(t)
	if withDest {
		destFolder := h.drive.Folder("Archive", "root")
		d, _, err := h.svc.AddDestination(context.Background(), app.Owner, "dst-1", app.DestinationRequest{Source: destFolder.ID, Name: "Archive"})
		if err != nil {
			t.Fatal(err)
		}
		if d, _, err = h.svc.SetDefaultDestination(context.Background(), app.Owner, "dst-default", d.ID, d.Revision); err != nil {
			t.Fatal(err)
		}
		h.dest = d
	}
	h.tg = h.newTelegram()
	return h
}

func (h *harness) newTelegram() *Telegram {
	h.t.Helper()
	tg, err := New(context.Background(), Options{Service: h.svc, Token: testBotToken, OwnerID: h.ownerID,
		APIURL: h.api.srv.URL, EditInterval: 10 * time.Millisecond, Now: h.clock})
	if err != nil {
		h.t.Fatal(err)
	}
	return tg
}

func (h *harness) clock() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.now
}

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	h.now = h.now.Add(d)
	h.mu.Unlock()
}

// pull runs one polling cycle the way Run does: fetch, store, handle.
func (h *harness) pull() {
	h.t.Helper()
	ctx := context.Background()
	offset, err := h.store.TelegramOffset(ctx, h.tg.instance)
	if err != nil {
		h.t.Fatal(err)
	}
	updates, err := h.tg.poll.getUpdates(ctx, offset, maxBatch, 0)
	if err != nil {
		h.t.Fatalf("getUpdates: %v", err)
	}
	if err := h.tg.ingest(ctx, updates); err != nil {
		h.t.Fatalf("ingest: %v", err)
	}
	h.tg.processPending(ctx)
}

func (h *harness) flush() {
	h.t.Helper()
	h.tg.flush(context.Background())
}

func (h *harness) jobs() []state.Job {
	h.t.Helper()
	jobs, _, err := h.svc.Jobs(context.Background(), app.Owner, true, "", "", 50, 0)
	if err != nil {
		h.t.Fatal(err)
	}
	return jobs
}

func (h *harness) lastCard() fakeMessage {
	h.t.Helper()
	cards := h.api.cards()
	if len(cards) == 0 {
		h.t.Fatal("no card was rendered")
	}
	return cards[len(cards)-1]
}

// buttonData returns the callback data of the first button whose label
// contains want.
func (h *harness) buttonData(card fakeMessage, want string) string {
	h.t.Helper()
	for i, data := range card.Buttons {
		if strings.Contains(card.ButtonTexts[i], want) {
			return data
		}
	}
	h.t.Fatalf("no button containing %q in %v", want, card.ButtonTexts)
	return ""
}

// nextUpdateID keeps fixture update ids monotonic: Telegram offsets only move
// forward, so a smaller id would never be delivered again.
func (h *harness) nextUpdateID() int64 {
	h.updates++
	return 10000 + h.updates
}

// click builds the callback update for the first button whose label contains
// want.
func (h *harness) click(card fakeMessage, want string) []byte {
	h.t.Helper()
	data := h.buttonData(card, want)
	return h.callbackUpdate(h.nextUpdateID(), h.ownerID, h.ownerID, card.MessageID, data)
}

// readyCollection creates a copied folder whose baseline finished.
func (h *harness) readyCollection(name string) string {
	h.t.Helper()
	ctx := context.Background()
	src := h.drive.Folder(name, "root")
	plan := state.Plan{ID: state.ID("plan"), Principal: "owner", SourceID: src.ID, SourceName: src.Name, SourceKind: state.KindFolder,
		DestinationID: "dst", DestinationFolder: h.dest.FolderID, RootName: src.Name, ExpiresAt: time.Now().Add(time.Hour), Revision: 1}
	if err := h.store.PutPlan(ctx, plan); err != nil {
		h.t.Fatal(err)
	}
	res, err := h.store.Admit(ctx, "owner", state.ID("admit"), plan.ID, 1)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err = h.store.UpdateJob(ctx, res.Job.ID, func(j *state.Job) (string, bool) {
		j.State = state.JobSucceeded
		return "finished", true
	}); err != nil {
		h.t.Fatal(err)
	}
	if err = h.store.SetCollectionState(ctx, res.Job.CollectionID, "ready"); err != nil {
		h.t.Fatal(err)
	}
	return res.Job.CollectionID
}

func (h *harness) lastSent() fakeMessage {
	h.t.Helper()
	sent, _ := h.api.messages()
	if len(sent) == 0 {
		h.t.Fatal("no message was sent")
	}
	return sent[len(sent)-1]
}

func (h *harness) messageUpdate(updateID, userID, chatID int64, chatType, text string) []byte {
	b, _ := json.Marshal(map[string]any{
		"update_id": updateID,
		"message": map[string]any{
			"message_id": updateID,
			"date":       1700000000,
			"chat":       map[string]any{"id": chatID, "type": chatType},
			"from":       map[string]any{"id": userID, "is_bot": false, "first_name": "U"},
			"text":       text,
		},
	})
	return b
}

func (h *harness) callbackUpdate(updateID, userID, chatID int64, messageID int, data string) []byte {
	b, _ := json.Marshal(map[string]any{
		"update_id": updateID,
		"callback_query": map[string]any{
			"id":   fmt.Sprintf("cbq%d", updateID),
			"from": map[string]any{"id": userID, "is_bot": false, "first_name": "U"},
			"data": data,
			"message": map[string]any{
				"message_id": messageID,
				"date":       1700000000,
				"chat":       map[string]any{"id": chatID, "type": "private"},
				"text":       "previous",
			},
		},
	})
	return b
}

func (h *harness) enableAutoCopy() {
	h.t.Helper()
	on, id, rev := true, h.dest.ID, h.dest.Revision
	if _, err := h.svc.PatchPreferences(context.Background(), app.Owner, "auto-on",
		app.PreferencePatch{AutoCopy: &on, DestinationID: &id, DestinationRevision: &rev}); err != nil {
		h.t.Fatal(err)
	}
}

// ---- tests ----

func TestGroupChatIsIgnored(t *testing.T) {
	h := newHarness(t, 100)
	h.api.push(h.messageUpdate(1, 100, 100, "group", h.src.ID))
	h.pull()
	if jobs := h.jobs(); len(jobs) != 0 {
		t.Fatalf("group paste created %d jobs", len(jobs))
	}
	if sent, _ := h.api.messages(); len(sent) != 0 {
		t.Fatalf("group paste got %d replies", len(sent))
	}
}

func TestUnknownUserIsIgnored(t *testing.T) {
	h := newHarness(t, 100)
	h.api.push(h.messageUpdate(1, 777, 777, "private", h.src.ID))
	h.pull()
	if jobs := h.jobs(); len(jobs) != 0 {
		t.Fatalf("unknown user created %d jobs", len(jobs))
	}
	if sent, _ := h.api.messages(); len(sent) != 0 {
		t.Fatalf("unknown user got %d replies", len(sent))
	}
}

func TestOperatorGetsMenu(t *testing.T) {
	h := newHarness(t, 100)
	if _, _, err := h.svc.AddOperator(context.Background(), app.Owner, "op-1", 555); err != nil {
		t.Fatal(err)
	}
	h.api.push(h.messageUpdate(1, 555, 555, "private", "/start"))
	h.pull()
	sent := h.lastSent()
	if !strings.Contains(sent.Text, "Dán link Drive") {
		t.Fatalf("home menu is missing its vi hint: %q", sent.Text)
	}
	if !strings.Contains(strings.Join(sent.ButtonTexts, "|"), "Công việc") {
		t.Fatalf("home menu is missing its vi buttons: %v", sent.ButtonTexts)
	}
}

func TestPlainTextIsNotFetched(t *testing.T) {
	h := newHarness(t, 100)
	h.api.push(h.messageUpdate(1, 100, 100, "private", "hello there"))
	h.pull()
	if jobs := h.jobs(); len(jobs) != 0 {
		t.Fatalf("plain text admitted %d jobs", len(jobs))
	}
	if sent := h.lastSent(); !strings.Contains(sent.Text, vietnamese.invalidLink) {
		t.Fatalf("plain text was not rejected: %q", sent.Text)
	}
}

func TestMenuLanguageComesFromPreferences(t *testing.T) {
	h := newHarness(t, 100)
	en := "en"
	if _, err := h.svc.PatchPreferences(context.Background(), app.Owner, "lang-en", app.PreferencePatch{Language: &en}); err != nil {
		t.Fatal(err)
	}
	h.api.push(h.messageUpdate(1, 100, 100, "private", "/start"))
	h.pull()
	sent := h.lastSent()
	if !strings.Contains(sent.Text, english.welcomeTail) {
		t.Fatalf("language preference ignored: %q", sent.Text)
	}
	if !strings.Contains(strings.Join(sent.ButtonTexts, "|"), english.kbSync) {
		t.Fatalf("en menu labels missing: %v", sent.ButtonTexts)
	}
}

func TestCrashBeforeInboxCommitKeepsTheUpdate(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	h.api.push(h.messageUpdate(1, 100, 100, "private", h.src.ID))

	// Crash after fetching but before the batch is committed.
	ctx := context.Background()
	first, err := h.tg.poll.getUpdates(ctx, 0, maxBatch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("fake API returned %d updates", len(first))
	}
	h.tg = h.newTelegram()
	h.pull()
	if jobs := h.jobs(); len(jobs) != 1 {
		t.Fatalf("after restart %d jobs were admitted, want 1", len(jobs))
	}
}

func TestCrashAfterInboxCommitDoesNotDuplicate(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	h.api.push(h.messageUpdate(1, 100, 100, "private", h.src.ID))

	ctx := context.Background()
	updates, err := h.tg.poll.getUpdates(ctx, 0, maxBatch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.tg.ingest(ctx, updates); err != nil {
		t.Fatal(err)
	}
	if jobs := h.jobs(); len(jobs) != 0 {
		t.Fatalf("ingest alone admitted %d jobs", len(jobs))
	}

	h.tg = h.newTelegram()
	h.tg.processPending(ctx)
	if jobs := h.jobs(); len(jobs) != 1 {
		t.Fatalf("recovery admitted %d jobs, want 1", len(jobs))
	}
	h.pull()
	if jobs := h.jobs(); len(jobs) != 1 {
		t.Fatalf("a second cycle admitted another job: %d", len(jobs))
	}
}

func TestDoubleCallbackCreatesOneJob(t *testing.T) {
	h := newHarness(t, 100)
	h.api.push(h.messageUpdate(1, 100, 100, "private", h.src.ID))
	h.pull()
	preview := h.lastSent()
	if len(preview.Buttons) == 0 || !strings.HasPrefix(preview.Buttons[0], callbackPrefix) {
		t.Fatalf("preview has no copy callback: %+v", preview.Buttons)
	}
	h.api.push(h.callbackUpdate(2, 100, 100, preview.MessageID, preview.Buttons[0]))
	h.api.push(h.callbackUpdate(3, 100, 100, preview.MessageID, preview.Buttons[0]))
	h.pull()
	if jobs := h.jobs(); len(jobs) != 1 {
		t.Fatalf("double click admitted %d jobs, want 1", len(jobs))
	}
	answers := h.api.answerTexts()
	if len(answers) != 2 {
		t.Fatalf("got %d callback acks, want 2", len(answers))
	}
	if answers[1] != vietnamese.expired {
		t.Fatalf("second click answered %q, want the expiry toast", answers[1])
	}
}

func TestExpiredCallbackDoesNothing(t *testing.T) {
	h := newHarness(t, 100)
	h.api.push(h.messageUpdate(1, 100, 100, "private", h.src.ID))
	h.pull()
	preview := h.lastSent()

	h.advance(callbackTTL + time.Minute)
	h.api.push(h.callbackUpdate(2, 100, 100, preview.MessageID, preview.Buttons[0]))
	h.pull()
	if jobs := h.jobs(); len(jobs) != 0 {
		t.Fatalf("expired callback admitted %d jobs", len(jobs))
	}
	if got := h.api.answerTexts(); len(got) != 1 || got[0] != vietnamese.expired {
		t.Fatalf("expired callback answered %v", got)
	}
}

func TestCallbackFromAnotherPrincipalDoesNothing(t *testing.T) {
	h := newHarness(t, 100)
	if _, _, err := h.svc.AddOperator(context.Background(), app.Owner, "op-1", 555); err != nil {
		t.Fatal(err)
	}
	h.api.push(h.messageUpdate(1, 100, 100, "private", h.src.ID))
	h.pull()
	preview := h.lastSent()

	h.api.push(h.callbackUpdate(2, 555, 555, preview.MessageID, preview.Buttons[0]))
	h.pull()
	if jobs := h.jobs(); len(jobs) != 0 {
		t.Fatalf("foreign callback admitted %d jobs", len(jobs))
	}
	if got := h.api.answerTexts(); len(got) != 1 || got[0] != vietnamese.expired {
		t.Fatalf("foreign callback answered %v", got)
	}
}

func TestPasteAutoCopyCreatesOneJob(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	update := h.messageUpdate(7, 100, 100, "private", h.src.ID)
	h.api.push(update)
	h.api.push(update) // Telegram redelivers the same update after a lost ack
	h.pull()
	if jobs := h.jobs(); len(jobs) != 1 {
		t.Fatalf("auto-copy admitted %d jobs, want 1", len(jobs))
	}
	if sent, _ := h.api.messages(); len(sent) != 1 {
		t.Fatalf("auto-copy sent %d messages, want one job card", len(sent))
	}
	h.pull()
	if jobs := h.jobs(); len(jobs) != 1 {
		t.Fatalf("a second polling cycle admitted another job: %d", len(jobs))
	}
}

func TestAutoCopyFallsBackToPreviewWhenRevisionMoved(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	// Changing the default moves the destination revision.
	if _, _, err := h.svc.SetDefaultDestination(context.Background(), app.Owner, "dst-default-2", h.dest.ID, h.dest.Revision); err != nil {
		t.Fatal(err)
	}
	h.api.push(h.messageUpdate(7, 100, 100, "private", h.src.ID))
	h.pull()
	if jobs := h.jobs(); len(jobs) != 0 {
		t.Fatalf("stale auto-copy binding admitted %d jobs", len(jobs))
	}
	if !strings.Contains(h.lastSent().Text, vietnamese.confirmDest) {
		t.Fatalf("preview does not ask to confirm the destination: %q", h.lastSent().Text)
	}
}

func TestProgressRateLimitDoesNotSpam(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	h.api.setEditErr(func(call int) (int, string, int) {
		if call == 1 {
			return http.StatusTooManyRequests, "Too Many Requests: retry after 1", 1
		}
		return 0, "", 0
	})
	h.api.push(h.messageUpdate(7, 100, 100, "private", h.src.ID))
	h.pull()
	jobs := h.jobs()
	if len(jobs) != 1 {
		t.Fatalf("want one job, got %d", len(jobs))
	}
	if _, err := h.svc.JobAction(context.Background(), app.Owner, "pause-1", jobs[0].ID, "pause", jobs[0].Revision); err != nil {
		t.Fatal(err)
	}
	h.advance(20 * time.Millisecond)
	h.flush()
	h.flush() // still inside retry_after: must not call the API again
	if _, edits := h.api.messages(); len(edits) != 1 {
		t.Fatalf("429 caused %d edit attempts, want 1", len(edits))
	}
	h.advance(2 * time.Second)
	h.flush()
	if _, edits := h.api.messages(); len(edits) != 2 {
		t.Fatalf("after retry_after the panel was not retried: %d edits", len(edits))
	}
	jobs = h.jobs()
	if jobs[0].State != state.JobPaused {
		t.Fatalf("a 429 changed the job state to %q", jobs[0].State)
	}
}

func TestNotModifiedIsANoOp(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	h.api.push(h.messageUpdate(7, 100, 100, "private", h.src.ID))
	h.pull()
	jobs := h.jobs()
	if _, err := h.svc.JobAction(context.Background(), app.Owner, "pause-1", jobs[0].ID, "pause", jobs[0].Revision); err != nil {
		t.Fatal(err)
	}
	h.api.setEditErr(func(int) (int, string, int) {
		return http.StatusBadRequest, "Bad Request: message is not modified", 0
	})
	h.advance(20 * time.Millisecond)
	h.flush()
	h.advance(time.Second)
	h.flush()
	if sent, edits := h.api.messages(); len(sent) != 1 || len(edits) != 1 {
		t.Fatalf("not-modified should not resend: %d sent, %d edits", len(sent), len(edits))
	}
}

func TestDeletedPanelIsReplacedOnce(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	h.api.push(h.messageUpdate(7, 100, 100, "private", h.src.ID))
	h.pull()
	jobs := h.jobs()
	if _, err := h.svc.JobAction(context.Background(), app.Owner, "pause-1", jobs[0].ID, "pause", jobs[0].Revision); err != nil {
		t.Fatal(err)
	}
	h.api.setEditErr(func(int) (int, string, int) {
		return http.StatusBadRequest, "Bad Request: message to edit not found", 0
	})
	h.advance(20 * time.Millisecond)
	h.flush()
	sent, edits := h.api.messages()
	if len(edits) != 1 || len(sent) != 2 {
		t.Fatalf("deleted panel: %d edits, %d sent (want 1 edit then one replacement)", len(edits), len(sent))
	}
	if !strings.Contains(sent[1].Text, "Project Alpha") {
		t.Fatalf("replacement panel lost the job name: %q", sent[1].Text)
	}
}

func TestForbiddenBotDoesNotChangeTheJob(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	h.api.push(h.messageUpdate(7, 100, 100, "private", h.src.ID))
	h.pull()
	jobs := h.jobs()
	if _, err := h.svc.JobAction(context.Background(), app.Owner, "pause-1", jobs[0].ID, "pause", jobs[0].Revision); err != nil {
		t.Fatal(err)
	}
	h.api.setEditErr(func(int) (int, string, int) {
		return http.StatusForbidden, "Forbidden: bot was blocked by the user", 0
	})
	h.advance(20 * time.Millisecond)
	h.flush()
	if jobs = h.jobs(); jobs[0].State != state.JobPaused {
		t.Fatalf("a blocked bot changed the job state to %q", jobs[0].State)
	}
	due, err := h.store.DueOutbox(context.Background(), h.clock().Add(24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("undeliverable notification is still pending: %+v", due)
	}
}

func TestDriveNamesAreEscaped(t *testing.T) {
	h := newHarness(t, 100)
	evil := h.drive.Folder(`<b>bold</b> & "quoted"`, "root")
	h.api.push(h.messageUpdate(1, 100, 100, "private", evil.ID))
	h.pull()
	sent := h.lastSent()
	if strings.Contains(sent.Text, "<b>bold</b>") {
		t.Fatalf("raw HTML from a Drive name reached the message: %q", sent.Text)
	}
	if !strings.Contains(sent.Text, "&lt;b&gt;bold&lt;/b&gt;") || !strings.Contains(sent.Text, "&amp;") {
		t.Fatalf("Drive name was not escaped: %q", sent.Text)
	}
}

func TestFinalNotificationIsDeliveredOnce(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	job := admit(t, h.store, "owner", "Nightly sync")
	finish(t, h.store, job.ID, state.JobSucceeded, "")

	if _, err := h.store.EnqueueFinal(ctx, "owner", job.ID, 100, state.JobSucceeded, "final:"+job.ID, h.clock()); err != nil {
		t.Fatal(err)
	}
	h.flush()
	sent, edits := h.api.messages()
	if len(sent) != 1 || len(edits) != 0 {
		t.Fatalf("sent=%d edits=%d", len(sent), len(edits))
	}
	if !strings.Contains(sent[0].Text, vietnamese.noticeCompleted) || !strings.Contains(sent[0].Text, "Nightly sync") {
		t.Fatalf("notification text = %q", sent[0].Text)
	}
	// A delivered row is not sent again.
	h.flush()
	if sent, _ = h.api.messages(); len(sent) != 1 {
		t.Fatalf("notification repeated: %d messages", len(sent))
	}
	// The job itself is untouched by delivery.
	if current, err := h.svc.Job(ctx, app.Owner, job.ID); err != nil || current.State != state.JobSucceeded {
		t.Fatalf("job changed: %+v %v", current, err)
	}

	// A subscription error has no job and names its code.
	if _, err := h.store.EnqueueFinal(ctx, "owner", "", 100, "auth", "", h.clock()); err != nil {
		t.Fatal(err)
	}
	h.flush()
	sent, _ = h.api.messages()
	if len(sent) != 2 || !strings.Contains(sent[1].Text, "auth") {
		t.Fatalf("subscription error message = %+v", sent)
	}
}

func TestFinalNotificationSurvivesABlockedBot(t *testing.T) {
	h := newHarness(t, 100)
	ctx := context.Background()
	job := admit(t, h.store, "owner", "Blocked bot")
	finish(t, h.store, job.ID, state.JobSucceeded, "")
	if _, err := h.store.EnqueueFinal(ctx, "owner", job.ID, 100, state.JobSucceeded, "final:"+job.ID, h.clock()); err != nil {
		t.Fatal(err)
	}
	h.api.setSendErr(func(call int) (int, string, int) {
		return http.StatusForbidden, "Forbidden: bot was blocked by the user", 0
	})
	h.flush()
	if current, err := h.svc.Job(ctx, app.Owner, job.ID); err != nil || current.State != state.JobSucceeded {
		t.Fatalf("a blocked bot changed the job: %+v %v", current, err)
	}
	due, err := h.store.DueOutbox(ctx, h.clock().Add(24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("an undeliverable notification is still pending: %+v", due)
	}
}

func TestSyncFromTheLibraryCard(t *testing.T) {
	h := newHarness(t, 100)
	id := h.readyCollection("Library Sync")
	ctx := context.Background()

	h.api.push(h.messageUpdate(1, 100, 100, "private", "/start"))
	h.pull()
	home := h.lastCard()

	// Copied sources page lists the collection with its real sync state.
	lib := h.callbackUpdate(2, 100, 100, home.MessageID, "nav:library")
	h.api.push(lib)
	h.pull()
	library := h.lastCard()
	if !strings.Contains(library.Text, "Library Sync") || !strings.Contains(library.Text, vietnamese.syncOff) {
		t.Fatalf("library card = %q", library.Text)
	}

	h.api.push(h.click(library, "Library Sync"))
	h.pull()
	card := h.lastCard()
	if !strings.Contains(card.Text, vietnamese.syncLabel+": "+vietnamese.syncOff) {
		t.Fatalf("collection card = %q", card.Text)
	}

	// Preset picker, then the daily preset.
	h.api.push(h.click(card, vietnamese.enableSync))
	h.pull()
	presets := h.lastCard()
	if !strings.Contains(presets.Text, vietnamese.pickPreset) {
		t.Fatalf("preset card = %q", presets.Text)
	}
	dailyData := h.buttonData(presets, vietnamese.daily2200)
	h.api.push(h.click(presets, vietnamese.daily2200))
	h.pull()
	preview := h.lastCard()
	if !strings.Contains(preview.Text, vietnamese.nextRuns) || strings.Count(preview.Text, "\n· ") != 3 {
		t.Fatalf("preview card = %q", preview.Text)
	}
	if !strings.Contains(preview.Text, syncTimezone) {
		t.Fatalf("preview does not name the zone: %q", preview.Text)
	}
	h.api.push(h.click(preview, vietnamese.save))
	h.pull()
	card = h.lastCard()
	if !strings.Contains(card.Text, vietnamese.syncStateLabel("active")) || !strings.Contains(card.Text, vietnamese.nextRun) {
		t.Fatalf("after saving a preset = %q", card.Text)
	}
	view, configured, err := h.svc.SyncStatus(ctx, id)
	if err != nil || !configured {
		t.Fatalf("subscription: %v %v", configured, err)
	}
	sub := view.Subscription
	if sub.Mode != state.ModeScheduled || sub.State != state.SyncActive || sub.Schedule == nil || sub.Schedule.LocalTime != "22:00" {
		t.Fatalf("stored subscription = %+v", sub)
	}
	savedRevision := sub.Revision

	// The preset button is one-shot: clicking it again changes nothing.
	h.api.push(h.callbackUpdate(h.nextUpdateID(), 100, 100, card.MessageID, dailyData))
	h.pull()
	if got := h.api.answerTexts(); len(got) == 0 || got[len(got)-1] != vietnamese.expired {
		t.Fatalf("second preset click answered %v", got)
	}
	if view, _, _ = h.svc.SyncStatus(ctx, id); view.Subscription.Revision != savedRevision {
		t.Fatalf("a consumed button changed the subscription: %+v", view.Subscription)
	}

	// Run now admits a delta run and shows it on the card.
	h.api.push(h.click(card, vietnamese.runNow))
	h.pull()
	card = h.lastCard()
	if !strings.Contains(card.Text, vietnamese.activeJob) {
		t.Fatalf("run now card = %q", card.Text)
	}

	// Pause is reflected on the card.
	h.api.push(h.click(card, vietnamese.pauseSync))
	h.pull()
	card = h.lastCard()
	if view, _, _ = h.svc.SyncStatus(ctx, id); view.Subscription.State != state.SyncPaused {
		t.Fatalf("state = %q", view.Subscription.State)
	}
	if !strings.Contains(card.Text, vietnamese.syncStateLabel("paused")) {
		t.Fatalf("paused card = %q", card.Text)
	}

	// Stop keeps the copied files and offers resume only.
	h.api.push(h.click(card, vietnamese.stopSync))
	h.pull()
	card = h.lastCard()
	if view, _, _ = h.svc.SyncStatus(ctx, id); view.Subscription.State != state.SyncStopped {
		t.Fatalf("state = %q", view.Subscription.State)
	}
	if !strings.Contains(strings.Join(card.ButtonTexts, "|"), vietnamese.resumeSync) {
		t.Fatalf("stopped card buttons = %v", card.ButtonTexts)
	}
}

func TestSyncPreviewUsesTheTimezonePreference(t *testing.T) {
	h := newHarness(t, 100)
	id := h.readyCollection("Timezone chat")
	tz := "America/New_York"
	if _, err := h.svc.PatchPreferences(context.Background(), app.Owner, "tz-1", app.PreferencePatch{Timezone: &tz}); err != nil {
		t.Fatal(err)
	}
	h.api.push(h.messageUpdate(1, 100, 100, "private", "/start"))
	h.pull()
	h.api.push(h.callbackUpdate(2, 100, 100, h.lastCard().MessageID, "nav:library"))
	h.pull()
	h.api.push(h.click(h.lastCard(), "Timezone chat"))
	h.pull()
	h.api.push(h.click(h.lastCard(), vietnamese.enableSync))
	h.pull()
	h.api.push(h.click(h.lastCard(), vietnamese.daily2200))
	h.pull()
	preview := h.lastCard()
	if !strings.Contains(preview.Text, tz) {
		t.Fatalf("preview does not use the preference: %q", preview.Text)
	}
	if strings.Count(preview.Text, "EDT")+strings.Count(preview.Text, "EST") < 3 {
		t.Fatalf("preview does not show three local runs: %q", preview.Text)
	}
	_ = id
}

func TestRealtimePresetFromChat(t *testing.T) {
	h := newHarness(t, 100)
	id := h.readyCollection("Realtime chat")
	h.api.push(h.messageUpdate(1, 100, 100, "private", "/start"))
	h.pull()
	lib := h.callbackUpdate(2, 100, 100, h.lastCard().MessageID, "nav:library")
	h.api.push(lib)
	h.pull()
	h.api.push(h.click(h.lastCard(), "Realtime chat"))
	h.pull()
	h.api.push(h.click(h.lastCard(), vietnamese.enableSync))
	h.pull()
	h.api.push(h.click(h.lastCard(), vietnamese.realtimePreset))
	h.pull()
	card := h.lastCard()
	if !strings.Contains(card.Text, "realtime") {
		t.Fatalf("realtime card = %q", card.Text)
	}
	view, configured, err := h.svc.SyncStatus(context.Background(), id)
	if err != nil || !configured || view.Subscription.Mode != state.ModeRealtime {
		t.Fatalf("subscription = %+v %v %v", view.Subscription, configured, err)
	}
	if view.Subscription.Cursor == "" {
		t.Fatal("the realtime cursor was not captured")
	}
}

func TestPollingStateAndRejectedToken(t *testing.T) {
	h := newHarness(t, 100)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.tg.Run(ctx)
	}()
	waitFor(t, func() bool { return h.tg.State() == StatePolling })
	cancel()
	<-done
	if h.tg.State() != StatePolling {
		t.Fatalf("shutdown changed the state to %q", h.tg.State())
	}

	bad := h.newTelegram()
	h.api.mu.Lock()
	h.api.getMeCode = http.StatusUnauthorized
	h.api.mu.Unlock()
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		bad.Run(ctx2)
	}()
	waitFor(t, func() bool { return bad.State() == StateError })
	cancel2()
	<-done2
}

func TestStateIsDisabledWithoutToken(t *testing.T) {
	h := newHarness(t, 100)
	if _, err := New(context.Background(), Options{Service: h.svc, Token: "  "}); err == nil {
		t.Fatal("an empty token must be rejected")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not reached in time")
}

func TestJobPanelShowsABarAndAnAnimationFrameWhileRunning(t *testing.T) {
	tx := english
	total := 200
	j := state.Job{ID: "job_1", RootName: "Photos", State: "running", Counts: state.Counts{Completed: 50, Total: &total}}
	a := jobTextFrame(tx, j, 0)
	b := jobTextFrame(tx, j, 1)
	if a == b {
		t.Fatal("the next frame must look different, or the panel would not animate")
	}
	for _, want := range []string{"📂", "📁", "▰▰▰▱", "25%"} {
		if !strings.Contains(a, want) {
			t.Fatalf("running panel lacks %q:\n%s", want, a)
		}
	}
	j.State = "succeeded"
	done := jobTextFrame(tx, j, 3)
	if strings.Contains(done, "➤") || !strings.Contains(done, "✅") {
		t.Fatalf("a finished job must stop animating and show a tick:\n%s", done)
	}
	if jobTextFrame(tx, j, 3) != jobTextFrame(tx, j, 4) {
		t.Fatal("a finished panel must not change between frames (no pointless edits)")
	}
	j.Counts.Total = nil
	if strings.Contains(jobTextFrame(tx, j, 0), "%") {
		t.Fatal("an unknown total must not show a percentage")
	}
	if progressBar(500, 100) != "▰▰▰▰▰▰▰▰▰▰▰▰▰▰ 100%" {
		t.Fatalf("an overshooting count must clamp: %q", progressBar(500, 100))
	}
}

func TestStartInstallsAPersistentKeyboardThenAColourfulWelcome(t *testing.T) {
	h := newHarness(t, 100)
	h.api.push(h.messageUpdate(1, 100, 100, "private", "/start"))
	h.pull()
	sent, _ := h.api.messages()
	if len(sent) < 2 {
		t.Fatalf("expected the keyboard message and the welcome card, got %d messages", len(sent))
	}
	kb, card := sent[len(sent)-2], sent[len(sent)-1]
	if !kb.Persistent || len(kb.Reply) != 6 || kb.Placeholder == "" {
		t.Fatalf("the quick menu must be a persistent 6-key keyboard with a placeholder: %+v", kb)
	}
	for _, want := range []string{vietnamese.kbCopy, vietnamese.kbJobs, vietnamese.kbSync, vietnamese.kbDest, vietnamese.kbAccount, vietnamese.kbSettings} {
		found := false
		for _, r := range kb.Reply {
			found = found || r == want
		}
		if !found {
			t.Fatalf("keyboard lacks %q: %v", want, kb.Reply)
		}
	}
	if !strings.Contains(card.Text, "<blockquote>") || !strings.Contains(card.Text, "502Drive") {
		t.Fatalf("the welcome card should use a quote block:\n%s", card.Text)
	}
	if len(card.Styles) == 0 || card.Styles[0] != "primary" {
		t.Fatalf("the Copy button should be the highlighted one: styles=%v", card.Styles)
	}
}

func TestKeyboardTapsOpenTheSamePagesAsTheButtons(t *testing.T) {
	h := newHarness(t, 100)
	for i, tap := range []struct{ label, want string }{
		{vietnamese.kbJobs, vietnamese.jobsEmpty},
		{english.kbJobs, vietnamese.jobsEmpty},
		{vietnamese.kbCopy, vietnamese.copyHint},
	} {
		h.api.push(h.messageUpdate(int64(i+1), 100, 100, "private", tap.label))
		h.pull()
		if sent := h.lastSent(); !strings.Contains(sent.Text, tap.want) {
			t.Fatalf("tap %q opened %q, want %q", tap.label, sent.Text, tap.want)
		}
	}
}

func TestKeyboardTapFromAStrangerIsIgnored(t *testing.T) {
	h := newHarness(t, 100)
	before, _ := h.api.messages()
	h.api.push(h.messageUpdate(1, 999, 999, "private", vietnamese.kbJobs))
	h.pull()
	if after, _ := h.api.messages(); len(after) != len(before) {
		t.Fatal("an unauthorised user got an answer")
	}
}
