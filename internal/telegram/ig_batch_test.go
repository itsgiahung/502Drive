package telegram

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

// Batch IG, slice 1: several links in one message (S6) and the queue.

func (h *harness) folderLink(f drive.File) string {
	return "https://drive.google.com/drive/folders/" + f.ID
}

func (h *harness) addOperator(telegramID int64) app.Principal {
	h.t.Helper()
	op, _, err := h.svc.AddOperator(context.Background(), app.Owner, fmt.Sprintf("op-%d", telegramID), telegramID)
	if err != nil {
		h.t.Fatal(err)
	}
	return app.Principal{ID: op.ID, Role: app.RoleOperator}
}

func (h *harness) sayAs(userID int64, text string) {
	h.t.Helper()
	h.api.push(h.messageUpdate(h.nextUpdateID(), userID, userID, "private", text))
	h.pull()
}

func (h *harness) tapAs(userID int64, card fakeMessage, label string) {
	h.t.Helper()
	data := h.buttonData(card, label)
	h.api.push(h.callbackUpdate(h.nextUpdateID(), userID, userID, card.MessageID, data))
	h.pull()
}

// forge stores a callback row that no button ever offered and returns its data.
func (h *harness) forge(principal string, chat int64, action, object string, revision int, payload string) string {
	h.t.Helper()
	id := fmt.Sprintf("forged%d", h.nextUpdateID())
	err := h.store.PutCallback(context.Background(), state.Callback{ID: id, Principal: principal, ChatID: chat,
		Action: action, Object: object, Revision: revision, Payload: payload,
		ExpiresAt: h.clock().Add(callbackTTL), CreatedAt: h.clock()})
	if err != nil {
		h.t.Fatal(err)
	}
	return callbackPrefix + id
}

func (h *harness) pushCallback(userID int64, messageID int, data string) {
	h.t.Helper()
	h.api.push(h.callbackUpdate(h.nextUpdateID(), userID, userID, messageID, data))
	h.pull()
}

func batchIDs(jobs []state.Job) map[string]bool {
	out := map[string]bool{}
	for _, j := range jobs {
		out[j.BatchID] = true
	}
	return out
}

func TestSeveralLinksInOneMessageAreOneBatchWithOneConfirmation(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	a := h.drive.Folder("Alpha two", "root")
	b := h.drive.Folder("Beta three", "root")
	h.say(h.folderLink(a) + "\n" + h.folderLink(b) + " " + h.srcLink())

	card := h.lastCard()
	if len(h.jobs()) != 0 {
		t.Fatalf("a batch preview must not create jobs: %+v", h.jobs())
	}
	for _, want := range []string{fmt.Sprintf(ig.batchTitle, 3), "Alpha two", "Beta three", "Project Alpha"} {
		if !strings.Contains(card.Text, want) {
			t.Fatalf("the batch preview lacks %q:\n%s", want, card.Text)
		}
	}
	h.tap(card, fmt.Sprintf(ig.batchGo, 3))

	jobs := h.jobs() // newest first
	if len(jobs) != 3 {
		t.Fatalf("want one job per link, got %d", len(jobs))
	}
	ids := batchIDs(jobs)
	if len(ids) != 1 || ids[""] {
		t.Fatalf("all the jobs must share one batch id: %v", ids)
	}
	// The queue keeps the order the links were sent in.
	if jobs[2].RootName != "Alpha two" || jobs[1].RootName != "Beta three" || jobs[0].RootName != "Project Alpha" {
		t.Fatalf("batch order = %s, %s, %s", jobs[2].RootName, jobs[1].RootName, jobs[0].RootName)
	}
	summary := h.lastCard()
	if !strings.Contains(summary.Text, fmt.Sprintf(ig.batchDone, 3, 3, 0, 0)) || !strings.Contains(summary.Text, "Alpha two") ||
		!strings.Contains(summary.Text, vietnamese.stateLabel(state.JobQueued)) {
		t.Fatalf("the summary must count the batch and show each job:\n%s", summary.Text)
	}

	// Refresh reads the server: a finished job shows as done.
	h.setJobState(jobs[2].ID, state.JobSucceeded)
	h.tap(summary, ig.batchRefresh)
	if !strings.Contains(h.lastCard().Text, vietnamese.stateLabel(state.JobSucceeded)) {
		t.Fatalf("a refresh must show the new state:\n%s", h.lastCard().Text)
	}

	// The confirmation is one-shot.
	h.api.push(h.click(card, fmt.Sprintf(ig.batchGo, 3)))
	h.pull()
	if len(h.jobs()) != 3 || h.lastAnswer() != vietnamese.expired {
		t.Fatalf("a second tap created more work (%d jobs, answer %q)", len(h.jobs()), h.lastAnswer())
	}
}

func TestBatchOfPastedLinksStartsAtOnceWhenPasteToCopyIsBound(t *testing.T) {
	h := newHarness(t, 100)
	h.enableAutoCopy()
	a := h.drive.Folder("Auto one", "root")
	b := h.drive.Folder("Auto two", "root")
	update := h.messageUpdate(7, 100, 100, "private", h.folderLink(a)+" "+h.folderLink(b))
	h.api.push(update)
	h.api.push(update) // redelivered after a lost ack
	h.pull()
	if jobs := h.jobs(); len(jobs) != 2 || len(batchIDs(jobs)) != 1 {
		t.Fatalf("auto-copy must admit the batch once: %d jobs, batches %v", len(jobs), batchIDs(jobs))
	}
	// A crash between the admission and the inbox record replays the update.
	ctx := context.Background()
	if err := h.tg.handle(ctx, update); err != nil {
		t.Fatal(err)
	}
	if err := h.tg.handle(ctx, update); err != nil {
		t.Fatal(err)
	}
	if jobs := h.jobs(); len(jobs) != 2 {
		t.Fatalf("a replayed update admitted more jobs: %d", len(jobs))
	}
}

func TestBatchReportsBadAndAlreadyCopiedLinksWithoutStoppingTheRest(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	g := vietnamese.g()
	// One source is copied already.
	h.say(h.srcLink())
	h.tap(h.lastCard(), vietnamese.copyNow)
	h.finishCopy()
	fresh := h.drive.Folder("Fresh one", "root")

	h.say(h.srcLink() + " https://drive.google.com/drive/folders/1NoSuchFolderAtAll_abcdefghij " + h.folderLink(fresh))
	card := h.lastCard()
	if !strings.Contains(card.Text, vietnamese.errMsg["source_not_found"]) {
		t.Fatalf("the unreadable link must be explained in the preview:\n%s", card.Text)
	}
	h.tap(card, fmt.Sprintf(ig.batchGo, 2))

	summary := h.lastCard().Text
	if !strings.Contains(summary, "đã nhận 1") || !strings.Contains(summary, "bỏ qua 1") || !strings.Contains(summary, "lỗi 1") {
		t.Fatalf("counts should be accepted 1, skipped 1, failed 1:\n%s", summary)
	}
	if !strings.Contains(summary, g.errMsg["already_copied"]) || !strings.Contains(summary, "Fresh one") {
		t.Fatalf("each link needs its own line:\n%s", summary)
	}
	if got := len(h.jobs()); got != 2 { // the earlier copy plus the fresh link
		t.Fatalf("only the fresh link may become a job: %d jobs", got)
	}
}

func TestBatchTakesTheFirstTwentyLinksAndSaysSo(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	var links []string
	for i := range maxBatchLinks + 2 {
		links = append(links, h.folderLink(h.drive.Folder(fmt.Sprintf("Bulk %02d", i), "root")))
	}
	h.say(strings.Join(links, " "))
	card := h.lastCard()
	if !strings.Contains(card.Text, fmt.Sprintf(ig.batchTooMany, maxBatchLinks, 2)) {
		t.Fatalf("the cap must be announced:\n%s", card.Text)
	}
	h.tap(card, fmt.Sprintf(ig.batchGo, maxBatchLinks))
	if got := len(h.jobs()); got != maxBatchLinks {
		t.Fatalf("want %d jobs, got %d", maxBatchLinks, got)
	}
}

func TestBatchOfOnlyBadLinksCreatesNothing(t *testing.T) {
	h := newHarness(t, 100)
	h.say("https://drive.google.com/drive/folders/1MissingOneAAAAAAAAAAAAAAA https://drive.google.com/drive/folders/1MissingTwoBBBBBBBBBBBBBBB")
	card := h.lastCard()
	if !strings.Contains(card.Text, vietnamese.ig().batchNone) || len(h.jobs()) != 0 {
		t.Fatalf("nothing usable must create nothing:\n%s", card.Text)
	}
	for _, b := range card.ButtonTexts {
		if strings.Contains(b, "📥") {
			t.Fatalf("there must be no confirm button: %v", card.ButtonTexts)
		}
	}
}

func TestBatchRespectsAllowlistPrivateChatAndPrincipal(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	op := h.addOperator(555)
	a := h.drive.Folder("Op one", "root")
	b := h.drive.Folder("Op two", "root")
	text := h.folderLink(a) + " " + h.folderLink(b)

	// A stranger and a group chat get nothing, not even an error.
	before, _ := h.api.messages()
	h.api.push(h.messageUpdate(h.nextUpdateID(), 777, 777, "private", text))
	h.api.push(h.messageUpdate(h.nextUpdateID(), 100, -100123, "group", text))
	h.pull()
	if after, _ := h.api.messages(); len(after) != len(before) || len(h.jobs()) != 0 {
		t.Fatalf("a stranger or a group created work")
	}

	// The operator's batch belongs to the operator.
	h.sayAs(555, text)
	card := h.lastCard()
	// The owner cannot press the operator's button.
	h.pushCallback(100, card.MessageID, h.buttonData(card, fmt.Sprintf(ig.batchGo, 2)))
	if len(h.jobs()) != 0 || h.lastAnswer() != vietnamese.expired {
		t.Fatalf("another person's button worked (%d jobs, %q)", len(h.jobs()), h.lastAnswer())
	}
	h.tapAs(555, card, fmt.Sprintf(ig.batchGo, 2))
	jobs, _, err := h.svc.Jobs(context.Background(), op, false, "", "", 50, 0)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("operator jobs = %d, %v", len(jobs), err)
	}
	for _, j := range jobs {
		if j.Principal != op.ID {
			t.Fatalf("job %s belongs to %s", j.ID, j.Principal)
		}
	}
	if owned, _, _ := h.svc.Jobs(context.Background(), app.Owner, false, "", "", 50, 0); len(owned) != 0 {
		t.Fatalf("the owner's own list must stay empty: %+v", owned)
	}
}

func TestCopyCommandAlsoTakesSeveralLinks(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	a := h.drive.Folder("Cmd one", "root")
	b := h.drive.Folder("Cmd two", "root")
	h.say("/copy " + h.folderLink(a) + " " + h.folderLink(b))
	h.tap(h.lastCard(), fmt.Sprintf(ig.batchGo, 2))
	if len(h.jobs()) != 2 {
		t.Fatalf("/copy with two links made %d jobs", len(h.jobs()))
	}
}

// ---- the queue ----

func TestOwnerPausesAndResumesTheQueueFromTheJobsPage(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	ctx := context.Background()
	h.queuedJob("owner", "Waiting")

	h.say(vietnamese.kbJobs)
	card := h.lastCard()
	if hasButton(card, ig.queueResumeBtn) || strings.Contains(card.Text, ig.queuePaused) {
		t.Fatalf("a running queue offers no resume:\n%s %v", card.Text, card.ButtonTexts)
	}
	h.tap(card, ig.queuePauseBtn)
	if paused, _ := h.store.QueuePaused(ctx); !paused {
		t.Fatal("the pause button did not pause the queue")
	}
	card = h.lastCard()
	if !strings.Contains(card.Text, ig.queuePaused) || !hasButton(card, ig.queueResumeBtn) || hasButton(card, ig.queuePauseBtn) {
		t.Fatalf("a paused queue says so and offers resume:\n%s %v", card.Text, card.ButtonTexts)
	}
	h.say("/start")
	if !strings.Contains(h.lastCard().Text, ig.queuePaused) {
		t.Fatalf("the home card must show the paused queue:\n%s", h.lastCard().Text)
	}

	h.say(vietnamese.kbJobs)
	h.tap(h.lastCard(), ig.queueResumeBtn)
	if paused, _ := h.store.QueuePaused(ctx); paused {
		t.Fatal("the resume button did not resume the queue")
	}
}

func TestOperatorSeesThePausedQueueButCannotChangeIt(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	ctx := context.Background()
	op := h.addOperator(555)
	h.queuedJob(op.ID, "Mine")
	if _, err := h.svc.QueuePause(ctx, app.Owner, true); err != nil {
		t.Fatal(err)
	}
	h.sayAs(555, vietnamese.kbJobs)
	card := h.lastCard()
	if !strings.Contains(card.Text, ig.queuePaused) || hasButton(card, ig.queuePauseBtn) || hasButton(card, ig.queueResumeBtn) {
		t.Fatalf("an operator is told, but gets no queue button:\n%s %v", card.Text, card.ButtonTexts)
	}
	// A forged resume button is refused by the service, not hidden by the UI.
	h.pushCallback(555, card.MessageID, h.forge(op.ID, 555, "queue_resume", "", 0, ""))
	if paused, _ := h.store.QueuePaused(ctx); !paused {
		t.Fatal("an operator resumed the queue")
	}
	if !strings.Contains(h.lastCard().Text, vietnamese.errMsg["forbidden"]) {
		t.Fatalf("the refusal should be explained:\n%s", h.lastCard().Text)
	}
}

func queuedPositions(t *testing.T, h *harness) []string {
	t.Helper()
	order, err := h.tg.queueOrder(context.Background(), app.Owner)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, j := range order {
		names = append(names, j.RootName)
	}
	return names
}

func TestWaitingJobCanMoveToTheFrontAndBackAndGetPriority(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	h.queuedJob("owner", "First")
	h.queuedJob("owner", "Second")
	c := h.queuedJob("owner", "Third")

	card := h.openJobCard(c.ID)
	if !strings.Contains(card.Text, fmt.Sprintf(ig.queuePos, 3, 3)) {
		t.Fatalf("the third job is third in line:\n%s", card.Text)
	}
	if !hasButton(card, ig.moveTop) || hasButton(card, ig.moveBottom) {
		t.Fatalf("the last job can only move up: %v", card.ButtonTexts)
	}
	h.tap(card, ig.moveTop)
	if got := strings.Join(queuedPositions(t, h), ","); got != "Third,First,Second" {
		t.Fatalf("order after 'to the front' = %s", got)
	}
	card = h.lastCard()
	if !strings.Contains(card.Text, fmt.Sprintf(ig.queuePos, 1, 3)) || hasButton(card, ig.moveTop) || !hasButton(card, ig.moveBottom) {
		t.Fatalf("the card must follow the move:\n%s %v", card.Text, card.ButtonTexts)
	}
	h.tap(card, ig.moveBottom)
	if got := strings.Join(queuedPositions(t, h), ","); got != "First,Second,Third" {
		t.Fatalf("order after 'to the back' = %s", got)
	}
	h.tap(h.lastCard(), ig.movePrio)
	if got := strings.Join(queuedPositions(t, h), ","); got != "Third,First,Second" {
		t.Fatalf("a prioritised job must lead: %s", got)
	}
	if j, _ := h.svc.Job(context.Background(), app.Owner, c.ID); j.Priority != prioHigh {
		t.Fatalf("priority = %d", j.Priority)
	}
	h.tap(h.lastCard(), ig.moveNormal)
	if j, _ := h.svc.Job(context.Background(), app.Owner, c.ID); j.Priority != 0 {
		t.Fatalf("priority after 'normal' = %d", j.Priority)
	}
}

// openJobCard renders the card of a job the way the Jobs page opens it.
func (h *harness) openJobCard(jobID string) fakeMessage {
	h.t.Helper()
	j, err := h.svc.Job(context.Background(), app.Owner, jobID)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err = h.tg.jobCard(context.Background(), app.Owner, h.ownerID, 0, j, false); err != nil {
		h.t.Fatal(err)
	}
	return h.lastCard()
}

func TestStaleMoveButtonShowsTheTruthAndMovesNothing(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	h.queuedJob("owner", "First")
	h.queuedJob("owner", "Second")
	c := h.queuedJob("owner", "Third")
	card := h.openJobCard(c.ID)

	// Another device moves the job first: the card's revision is now old.
	if _, err := h.svc.MoveJob(context.Background(), app.Owner, c.ID, c.Revision, state.Move{Priority: new(int)}); err != nil {
		t.Fatal(err)
	}
	before := strings.Join(queuedPositions(t, h), ",")
	h.tap(card, ig.moveTop)
	if got := strings.Join(queuedPositions(t, h), ","); got != before {
		t.Fatalf("a stale button moved the job: %s -> %s", before, got)
	}
	if !strings.Contains(h.lastCard().Text, "Third") {
		t.Fatalf("the card should show the current job:\n%s", h.lastCard().Text)
	}
}

func TestOperatorCannotMoveSomeoneElsesJob(t *testing.T) {
	h := newHarness(t, 100)
	op := h.addOperator(555)
	mine := h.queuedJob(op.ID, "Op job")
	other := h.queuedJob("owner", "Owner job")
	h.sayAs(555, vietnamese.kbJobs)
	card := h.lastCard()
	h.pushCallback(555, card.MessageID, h.forge(op.ID, 555, "job_move", other.ID, other.Revision, "top"))
	order, _ := h.tg.queueOrder(context.Background(), app.Owner)
	if len(order) != 2 || order[0].ID != mine.ID {
		t.Fatalf("an operator reordered the owner's job: %+v", order)
	}
	if j, _ := h.svc.Job(context.Background(), app.Owner, other.ID); j.Revision != other.Revision {
		t.Fatalf("the owner's job was touched (revision %d -> %d)", other.Revision, j.Revision)
	}
}

func TestPausedQueueKeepsBatchJobsWaiting(t *testing.T) {
	h := newHarness(t, 100)
	ig := vietnamese.ig()
	ctx := context.Background()
	if _, err := h.svc.QueuePause(ctx, app.Owner, true); err != nil {
		t.Fatal(err)
	}
	a := h.drive.Folder("P one", "root")
	b := h.drive.Folder("P two", "root")
	h.say(h.folderLink(a) + " " + h.folderLink(b))
	h.tap(h.lastCard(), fmt.Sprintf(ig.batchGo, 2))
	summary := h.lastCard()
	if !strings.Contains(summary.Text, ig.queuePaused) || !hasButton(summary, ig.queueResumeBtn) {
		t.Fatalf("the batch summary must show the paused queue:\n%s %v", summary.Text, summary.ButtonTexts)
	}
	for _, j := range h.jobs() {
		if j.State != state.JobQueued {
			t.Fatalf("job %s left the queue while it is paused: %s", j.ID, j.State)
		}
	}
	h.tap(summary, ig.queueResumeBtn)
	if paused, _ := h.store.QueuePaused(ctx); paused {
		t.Fatal("resume from the batch card did not resume")
	}
	if !strings.Contains(h.lastCard().Text, "P one") {
		t.Fatalf("resume should return to the batch summary:\n%s", h.lastCard().Text)
	}
}

func TestBatchPayloadSurvivesJSONAndRejectsGarbage(t *testing.T) {
	bp := batchPayload{ID: batchIDFor("tg:x:1"), Items: []batchItem{{Plan: "plan_1", Rev: 2, Name: "n"}}}
	got, ok := decodeBatch(bp.json())
	if !ok || got.ID != bp.ID || len(got.Items) != 1 || got.Items[0].Plan != "plan_1" {
		t.Fatalf("round trip = %+v %v", got, ok)
	}
	if _, ok = decodeBatch("{"); ok {
		t.Fatal("garbage decoded")
	}
	if id := batchIDFor("tg:i:1"); len(id) < 8 || len(id) > 64 || strings.ContainsAny(id, ": ") {
		t.Fatalf("batch id %q is not a valid app batch id", id)
	}
}
