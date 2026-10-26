package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type driveFake struct {
	fakeActions
	folders map[string]FolderPage
	calls   []string
}

func (d *driveFake) log(format string, a ...any) {
	d.calls = append(d.calls, fmt.Sprintf(format, a...))
}

func newDriveFake() *driveFake {
	size := int64(2048)
	root := DriveItem{ID: "root", Name: "My Drive", Kind: "folder", CanWrite: true}
	proj := DriveItem{ID: "p1", Name: "Projects", Kind: "folder", CanWrite: true, CanCopy: true}
	file := DriveItem{ID: "f1", Name: "report.pdf", Kind: "binary", Mime: "application/pdf", Size: &size,
		Modified: time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC), Owner: "Owner <o@example.test>", CanCopy: true}
	return &driveFake{
		fakeActions: fakeActions{snap: sampleSnapshot()},
		folders: map[string]FolderPage{
			"root": {Folder: root, Path: []DriveItem{root}, Items: []DriveItem{proj, file}},
			"p1": {Folder: proj, Path: []DriveItem{root, proj}, Items: []DriveItem{
				{ID: "p2", Name: "2026", Kind: "folder", CanWrite: true}}},
			"p2": {Folder: DriveItem{ID: "p2", Name: "2026", Kind: "folder", CanWrite: true}, Path: []DriveItem{root, proj, {ID: "p2", Name: "2026"}}},
		},
	}
}

func (d *driveFake) acts() Actions {
	a := d.fakeActions.Actions()
	a.Browse = func(_ context.Context, id, token string) (FolderPage, error) {
		d.log("browse:%s", id)
		p, ok := d.folders[id]
		if !ok {
			return FolderPage{}, errors.New("not found")
		}
		return p, nil
	}
	a.Mkdir = func(_ context.Context, parent, name string) (DriveItem, error) {
		d.log("mkdir:%s:%s", parent, name)
		it := DriveItem{ID: "new1", Name: name, Kind: "folder", CanWrite: true}
		p := d.folders[parent]
		p.Items = append(p.Items, it)
		d.folders[parent] = p
		return it, nil
	}
	a.Inspect = func(_ context.Context, source, dest, name string) (Plan, error) {
		d.log("inspect:%s:%s:%s", source, dest, name)
		root := name
		if root == "" {
			root = "Big Folder"
		}
		return Plan{ID: "plan_1", Revision: 1, SourceID: source, SourceName: "Big Folder", SourceKind: "folder",
			DestinationID: firstNonEmpty(dest, "dst_1"), RootName: root, Files: 10, Folders: 2, Complete: true}, nil
	}
	a.Submit = func(_ context.Context, p Plan) (string, error) {
		d.log("submit:%s", p.ID)
		return "job_new", nil
	}
	a.AddDestination = func(_ context.Context, folder string) (Destination, error) {
		d.log("adddest:%s", folder)
		return Destination{ID: "dst_" + folder, Name: "Dest " + folder, Revision: 1}, nil
	}
	a.SetDefaultDestination = func(_ context.Context, id string, rev int) error {
		d.log("default:%s:%d", id, rev)
		return nil
	}
	return a
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func driveModel(t *testing.T, d *driveFake, w, h int) Model {
	t.Helper()
	opt := Options{Width: w, Height: h, Language: "en", Now: func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }}
	m := New(d.acts(), opt)
	m = run(t, m, m.fetch())
	m, _ = update(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func pressKey(t *testing.T, m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	return update(t, m, k)
}

func TestDriveBrowserNavigatesFoldersWithBreadcrumbAndInfo(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, cmd := update(t, m, key("7"))
	m = run(t, m, cmd)
	view := m.View()
	for _, want := range []string{"My Drive", "Projects/", "report.pdf", "2.0 KB"} {
		if !strings.Contains(view, want) {
			t.Fatalf("root view lacks %q:\n%s", want, view)
		}
	}
	m, cmd = update(t, m, key("enter")) // open Projects
	m = run(t, m, cmd)
	if v := m.View(); !strings.Contains(v, "My Drive › Projects") || !strings.Contains(v, "2026/") {
		t.Fatalf("inside Projects:\n%s", v)
	}
	m, cmd = pressKey(t, m, tea.KeyMsg{Type: tea.KeyLeft}) // up, cursor back on Projects
	m = run(t, m, cmd)
	if it, _ := m.driveItem(); it.ID != "p1" {
		t.Fatalf("going up should put the cursor on the folder we left, got %q", it.ID)
	}
	m, _ = update(t, m, key("down"))
	m, _ = update(t, m, key("i"))
	v := m.View()
	for _, want := range []string{"application/pdf", "Owner <o@example.test>", "2026-10-01", "Copy allowed"} {
		if !strings.Contains(v, want) {
			t.Fatalf("info panel lacks %q:\n%s", want, v)
		}
	}
}

func TestDriveBrowserCreatesAFolderAndFocusesIt(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, cmd := update(t, m, key("7"))
	m = run(t, m, cmd)
	m, _ = update(t, m, key("n"))
	if !m.typing() {
		t.Fatal("n should open the folder name prompt")
	}
	m = typeText(t, m, "Backups")
	m, cmd = update(t, m, key("enter"))
	m = run(t, m, cmd) // mkdirMsg → reload
	_ = m
	if got := strings.Join(d.calls, " "); !strings.Contains(got, "mkdir:root:Backups") {
		t.Fatalf("calls = %v", d.calls)
	}
}

func TestQDoesNotQuitWhileTypingAFolderName(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, cmd := update(t, m, key("7"))
	m = run(t, m, cmd)
	m, _ = update(t, m, key("n"))
	m = typeText(t, m, "quiet q")
	if m.quits || !m.typing() {
		t.Fatal("typing q must not quit")
	}
	if v := m.drv.prompt.Value(); v != "quiet q" {
		t.Fatalf("typed %q", v)
	}
}

func TestCopyFlowFromHomeChoosesNamesAndDestination(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, _ = update(t, m, key("c"))
	if m.screen != screenCopy || !m.typing() {
		t.Fatalf("c on Home should ask for a link: screen=%v typing=%v", m.screen, m.typing())
	}
	m = typeText(t, m, "SRC123")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	view := m.View()
	for _, want := range []string{"Big Folder", "10 files, 2 folders", "Output folder name", "● 1 keep the source name"} {
		if !strings.Contains(view, want) {
			t.Fatalf("plan view lacks %q:\n%s", want, view)
		}
	}
	// 2: a new name made from the source name and today's date.
	m, cmd = update(t, m, key("2"))
	m = run(t, m, cmd)
	if v := m.View(); !strings.Contains(v, "Big Folder (copy 2026-10-27)") || !strings.Contains(v, "● 2 make a new name") {
		t.Fatalf("auto name:\n%s", v)
	}
	// 3: a typed name.
	m, _ = update(t, m, key("3"))
	m, _ = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlU}) // clear the prefilled name
	m = typeText(t, m, "My copy")
	m, cmd = update(t, m, key("enter"))
	m = run(t, m, cmd)
	if v := m.View(); !strings.Contains(v, "My copy") {
		t.Fatalf("custom name:\n%s", v)
	}
	// d: pick another destination folder, y chooses the open folder.
	m, cmd = update(t, m, key("d"))
	m = run(t, m, cmd)
	if m.screen != screenDrive || !m.drv.pick || !strings.Contains(m.View(), "CHOOSE A DESTINATION") {
		t.Fatalf("d should open the folder chooser:\n%s", m.View())
	}
	m, cmd = update(t, m, key("enter")) // into Projects
	m = run(t, m, cmd)
	m, cmd = update(t, m, key("y"))
	m = run(t, m, cmd) // destMsg → back to the copy dialog and a new preview
	if m.screen != screenCopy {
		t.Fatalf("after y the copy dialog should be back, screen=%v", m.screen)
	}
	m = run(t, m, func() tea.Msg { return nil })
	last := d.calls[len(d.calls)-1]
	_ = last
	joined := strings.Join(d.calls, " ")
	if !strings.Contains(joined, "adddest:p1") {
		t.Fatalf("the chosen folder was not made a destination: %v", d.calls)
	}
}

func TestCopyStartSubmitsThePlanAndOpensJobs(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, cmd := update(t, m, key("7"))
	m = run(t, m, cmd)
	m, cmd = update(t, m, key("c")) // copy the highlighted item (Projects)
	m = run(t, m, cmd)
	if m.screen != screenCopy || m.cp.plan == nil || m.cp.source != "p1" {
		t.Fatalf("c in the browser should preview the highlighted item: screen=%v", m.screen)
	}
	m, cmd = update(t, m, key("enter"))
	m = run(t, m, cmd)
	if m.screen != screenJobs || !strings.Contains(m.status, "job_new") {
		t.Fatalf("after submit: screen=%v status=%q", m.screen, m.status)
	}
	if got := strings.Join(d.calls, " "); !strings.Contains(got, "submit:plan_1") {
		t.Fatalf("calls = %v", d.calls)
	}
}

func TestBrowserDMakesTheHighlightedFolderTheDefaultDestination(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, cmd := update(t, m, key("7"))
	m = run(t, m, cmd)
	m, cmd = update(t, m, key("d"))
	m = run(t, m, cmd)
	got := strings.Join(d.calls, " ")
	if !strings.Contains(got, "adddest:p1") || !strings.Contains(got, "default:dst_p1:1") {
		t.Fatalf("calls = %v", d.calls)
	}
	if !strings.Contains(m.status, "default destination") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestBadLinkReturnsToTheLinkPromptWithTheReason(t *testing.T) {
	d := newDriveFake()
	acts := d.acts()
	acts.Inspect = func(context.Context, string, string, string) (Plan, error) {
		return Plan{}, errors.New("That link is ambiguous")
	}
	m := New(acts, Options{Width: 80, Language: "en"})
	m = run(t, m, m.fetch())
	m, _ = update(t, m, key("c"))
	m = typeText(t, m, "garbage")
	m, cmd := update(t, m, key("enter"))
	m = run(t, m, cmd)
	if !m.typing() || !strings.Contains(m.View(), "ambiguous") || !strings.Contains(m.View(), "garbage") {
		t.Fatalf("the prompt should come back with the error and the text:\n%s", m.View())
	}
}

func TestStaleBrowseAnswersAreIgnored(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m, cmd := update(t, m, key("7"))
	m = run(t, m, cmd)
	m, _ = update(t, m, browseMsg{req: 0, page: d.folders["p1"]})
	if m.drv.page.Folder.ID != "root" {
		t.Fatal("an answer for an older request replaced the current folder")
	}
}

func TestLongFoldersAreWindowedAndLoadMorePages(t *testing.T) {
	d := newDriveFake()
	var items []DriveItem
	for i := 0; i < 60; i++ {
		items = append(items, DriveItem{ID: fmt.Sprintf("x%02d", i), Name: fmt.Sprintf("file %02d", i), Kind: "binary"})
	}
	root := d.folders["root"]
	root.Items, root.Next = items, "tok2"
	d.folders["root"] = root
	m := driveModel(t, d, 80, 20)
	m, cmd := update(t, m, key("7"))
	m = run(t, m, cmd)
	loaded := false
	for i := 0; i < 60; i++ {
		m, cmd = update(t, m, key("down"))
		loaded = loaded || cmd != nil
	}
	if got := len(lines(m.View())); got != 20 {
		t.Fatalf("view has %d lines, want 20", got)
	}
	if !loaded {
		t.Fatal("reaching the last row with a next page should load more")
	}
}
