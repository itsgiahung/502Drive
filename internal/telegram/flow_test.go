package telegram

import (
	"context"
	"strings"
	"testing"
)

func (h *harness) say(text string) {
	h.t.Helper()
	h.api.push(h.messageUpdate(h.nextUpdateID(), h.ownerID, h.ownerID, "private", text))
	h.pull()
}

func (h *harness) tap(card fakeMessage, label string) {
	h.t.Helper()
	h.api.push(h.click(card, label))
	h.pull()
}

func TestFirstLinkNeedsNoSetupAndKeepsTheSourceName(t *testing.T) {
	h := newBareHarness(t, 100)
	h.say("https://drive.google.com/drive/folders/" + h.src.ID)
	card := h.lastCard()
	for _, want := range []string{"Project Alpha", "My Drive", "🏷"} {
		if !strings.Contains(card.Text, want) {
			t.Fatalf("preview lacks %q:\n%s", want, card.Text)
		}
	}
	if strings.Contains(card.Text, "destination") {
		t.Fatalf("no 'choose a destination first' error may appear:\n%s", card.Text)
	}
	if len(card.Styles) == 0 || card.Styles[0] != "success" {
		t.Fatalf("Copy now is the green button: %v", card.Styles)
	}
	for _, want := range []string{vietnamese.copyNow, vietnamese.renameBtn, vietnamese.pickBtn} {
		found := false
		for _, b := range card.ButtonTexts {
			found = found || b == want
		}
		if !found {
			t.Fatalf("preview lacks the %q button: %v", want, card.ButtonTexts)
		}
	}
	h.tap(card, vietnamese.copyNow)
	jobs := h.jobs()
	if len(jobs) != 1 || jobs[0].RootName != "Project Alpha" {
		t.Fatalf("the copy should keep the source's name: %+v", jobs)
	}
}

func TestRenameAsksThenUsesTheTypedName(t *testing.T) {
	h := newHarness(t, 100)
	h.say("https://drive.google.com/drive/folders/" + h.src.ID)
	h.tap(h.lastCard(), vietnamese.renameBtn)
	if c := h.lastCard(); !strings.Contains(c.Text, "/cancel") {
		t.Fatalf("expected the rename prompt:\n%s", c.Text)
	}
	h.say("Bản sao tháng 10")
	card := h.lastCard()
	if !strings.Contains(card.Text, "Bản sao tháng 10") {
		t.Fatalf("the preview should show the typed name:\n%s", card.Text)
	}
	h.tap(card, vietnamese.copyNow)
	if jobs := h.jobs(); len(jobs) != 1 || jobs[0].RootName != "Bản sao tháng 10" {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestPickingAFolderChangesWhereTheCopyIsSaved(t *testing.T) {
	h := newBareHarness(t, 100)
	target := h.drive.Folder("Backups", "root")
	h.drive.Folder("2026", target.ID)
	h.say("https://drive.google.com/drive/folders/" + h.src.ID)
	h.tap(h.lastCard(), vietnamese.pickBtn)
	card := h.lastCard()
	if !strings.Contains(card.Text, "My Drive") || !strings.Contains(strings.Join(card.ButtonTexts, "|"), "Backups") {
		t.Fatalf("the chooser should list the folders of My Drive:\n%s %v", card.Text, card.ButtonTexts)
	}
	h.tap(card, "Backups")
	card = h.lastCard()
	if !strings.Contains(card.Text, "My Drive › Backups") || !strings.Contains(strings.Join(card.ButtonTexts, "|"), "2026") {
		t.Fatalf("inside Backups:\n%s %v", card.Text, card.ButtonTexts)
	}
	h.tap(card, vietnamese.saveHere)
	card = h.lastCard()
	if !strings.Contains(card.Text, "Backups") {
		t.Fatalf("the preview should now save into Backups:\n%s", card.Text)
	}
	h.tap(card, vietnamese.copyNow)
	jobs := h.jobs()
	if len(jobs) != 1 {
		t.Fatalf("jobs = %+v", jobs)
	}
	dests, err := h.svc.Destinations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var backups string
	for _, d := range dests {
		if d.FolderID == target.ID {
			backups = d.ID
		}
	}
	if backups == "" || jobs[0].DestinationID != backups {
		t.Fatalf("the job must target Backups: job dest %q, Backups dest %q", jobs[0].DestinationID, backups)
	}
}

func TestNewFolderFromTheChooserIsCreatedAndOpened(t *testing.T) {
	h := newBareHarness(t, 100)
	h.say("https://drive.google.com/drive/folders/" + h.src.ID)
	h.tap(h.lastCard(), vietnamese.pickBtn)
	h.tap(h.lastCard(), vietnamese.newFolderBtn)
	h.say("Lưu trữ mới")
	card := h.lastCard()
	if !strings.Contains(card.Text, "Lưu trữ mới") {
		t.Fatalf("the new folder should be opened in the chooser:\n%s", card.Text)
	}
	found := false
	for _, f := range h.drive.Children("root") {
		found = found || f.Name == "Lưu trữ mới"
	}
	if !found {
		t.Fatal("the folder was not created in Drive")
	}
}

func TestALinkWhileWaitingForANameIsTreatedAsALink(t *testing.T) {
	h := newHarness(t, 100)
	h.say("https://drive.google.com/drive/folders/" + h.src.ID)
	h.tap(h.lastCard(), vietnamese.renameBtn)
	other := h.drive.Folder("Other", "root")
	h.say("https://drive.google.com/drive/folders/" + other.ID)
	if c := h.lastCard(); !strings.Contains(c.Text, "Other") {
		t.Fatalf("a pasted link must start a new preview instead of becoming a name:\n%s", c.Text)
	}
}

func TestErrorsAreShownInTheUsersLanguage(t *testing.T) {
	h := newHarness(t, 100)
	h.say("https://drive.google.com/drive/folders/doesnotexist123456")
	if c := h.lastSent(); !strings.Contains(c.Text, vietnamese.errMsg["source_not_found"]) && !strings.Contains(c.Text, vietnamese.errMsg["not_found"]) {
		t.Fatalf("expected a Vietnamese 'not found', got:\n%s", c.Text)
	}
}
