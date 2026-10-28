package tray

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestIconStatesAreDistinctAndSized(t *testing.T) {
	seen := map[string]State{}
	for _, st := range []State{StateIdle, StateBusy, StateNeeds, StateError, StateOffline} {
		data, err := PNG(st, 0, 64)
		if err != nil {
			t.Fatalf("%s: %v", st, err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", st, err)
		}
		if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 64 {
			t.Fatalf("%s: bounds %v", st, b)
		}
		if other, dup := seen[string(data)]; dup {
			t.Fatalf("%s and %s render identically", st, other)
		}
		seen[string(data)] = st
	}
	for _, bad := range []int{8, 15, 600} {
		if _, err := PNG(StateIdle, 0, bad); err == nil {
			t.Fatalf("size %d was accepted", bad)
		}
	}
}

func TestBusyFramesDiffer(t *testing.T) {
	frames := map[string]int{}
	for f := range busyFrames {
		data, err := PNG(StateBusy, f, 64)
		if err != nil {
			t.Fatal(err)
		}
		frames[string(data)] = f
	}
	if len(frames) != busyFrames {
		t.Fatalf("busy produced %d distinct frames, want %d", len(frames), busyFrames)
	}
}

func TestFrameAtIsCappedAtFourFPS(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	if FrameAt(base) != FrameAt(base.Add(249*time.Millisecond)) {
		t.Fatal("the frame changed inside one 250ms window")
	}
	if FrameAt(base) == FrameAt(base.Add(250*time.Millisecond)) {
		t.Fatal("the frame did not change after 250ms")
	}
	distinct := map[int]bool{}
	for i := range 4 {
		distinct[FrameAt(base.Add(time.Duration(i)*250*time.Millisecond))] = true
	}
	if len(distinct) != busyFrames {
		t.Fatalf("frames in one second = %d, want %d", len(distinct), busyFrames)
	}
}

func TestICOIsAValidContainer(t *testing.T) {
	sizes := []int{16, 32, 48, 64, 256}
	data, err := ICO(StateIdle, sizes)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(data[0:]) != 0 || binary.LittleEndian.Uint16(data[2:]) != 1 {
		t.Fatal("bad ICO header")
	}
	count := int(binary.LittleEndian.Uint16(data[4:]))
	if count != len(sizes) {
		t.Fatalf("count = %d", count)
	}
	for i, size := range sizes {
		entry := data[6+16*i:]
		dim := int(entry[0])
		if size >= 256 {
			if dim != 0 {
				t.Fatalf("256 must be encoded as 0, got %d", dim)
			}
		} else if dim != size {
			t.Fatalf("entry %d width = %d, want %d", i, dim, size)
		}
		if entry[1] != entry[0] {
			t.Fatalf("entry %d is not square", i)
		}
		offset := int(binary.LittleEndian.Uint32(entry[12:]))
		length := int(binary.LittleEndian.Uint32(entry[8:]))
		if offset < 6+16*len(sizes) || offset+length > len(data) {
			t.Fatalf("entry %d points outside the file", i)
		}
		payload := data[offset : offset+length]
		if !bytes.HasPrefix(payload, []byte("\x89PNG\r\n\x1a\n")) {
			t.Fatalf("entry %d is not PNG", i)
		}
	}
}

func TestWriteIconsCreatesUsableAssets(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "icons")
	if err := WriteIcons(dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, IconPNG))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatalf("desktop icon is %v", img.Bounds())
	}
	ico, err := os.ReadFile(filepath.Join(dir, IconICO))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(ico, []byte{0, 0, 1, 0}) {
		t.Fatal("not an ICO file")
	}
	if err := WriteIcons(""); err == nil {
		t.Fatal("an empty directory was accepted")
	}
}

func snapOf(running, queued, blocked int, account string, jobs ...Job) Snapshot {
	return Snapshot{Status: Status{Version: "test", Ready: true, Running: running, Queued: queued,
		Blocked: blocked, AccountStatus: account}, Jobs: jobs}
}

// itemByID finds a menu row (top level or inside a submenu) by its fixed id.
func menuRow(t *testing.T, v View, id string) Item {
	t.Helper()
	for _, it := range v.Items {
		if it.ID == id {
			return it
		}
		for _, c := range it.Children {
			if c.ID == id {
				return c
			}
		}
	}
	t.Fatalf("no menu item %q in %+v", id, v.Items)
	return Item{}
}

func TestClassifyPrecedence(t *testing.T) {
	paused := snapOf(1, 0, 0, "connected")
	paused.Status.QueuePaused = true
	blockedPaused := snapOf(0, 0, 1, "connected")
	blockedPaused.Status.QueuePaused = true
	resuming := snapOf(0, 0, 0, "connected")
	resuming.Status.Lifecycle = "resuming"
	sleeping := snapOf(0, 0, 0, "connected")
	sleeping.Status.Lifecycle = "sleeping"
	accountLost := snapOf(0, 0, 0, "connected")
	accountLost.Status.Lifecycle = "reconnect_required"
	for _, tc := range []struct {
		name   string
		snap   Snapshot
		online bool
		want   State
	}{
		{"offline wins", snapOf(1, 1, 1, "connected"), false, StateOffline},
		{"sleeping machine is offline", sleeping, true, StateOffline},
		{"not ready", Snapshot{Status: Status{Ready: false}}, true, StateError},
		{"blocked", snapOf(0, 0, 2, "connected"), true, StateNeeds},
		{"account", snapOf(0, 0, 0, "reconnect_required"), true, StateNeeds},
		{"account lost after resume", accountLost, true, StateNeeds},
		{"resuming reconnects", resuming, true, StateReconnecting},
		{"paused", paused, true, StatePaused},
		{"blocked beats paused", blockedPaused, true, StateNeeds},
		{"busy", snapOf(1, 0, 0, "connected"), true, StateBusy},
		{"queued is busy", snapOf(0, 3, 0, "connected"), true, StateBusy},
		{"idle", snapOf(0, 0, 0, "connected"), true, StateIdle},
	} {
		if got := Classify(tc.snap, tc.online); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestBuildViewUsesTheLanguage(t *testing.T) {
	vi := BuildViewLang(snapOf(1, 0, 0, "connected"), true, "", "vi")
	for id, want := range map[string]string{
		ActionOpenTUI: "Mở 502Drive", ActionQueueToggle: "Tạm dừng tất cả",
		ActionStopDaemon: "Dừng 502Drive…", ActionQuit: "Thoát khay", ActionOpenAll: "Mở tất cả",
	} {
		if got := menuRow(t, vi, id).Title; got != want {
			t.Errorf("vi %s = %q, want %q", id, got, want)
		}
	}
	if !strings.Contains(menuRow(t, vi, ActionAutostart).Title, "Khởi động cùng hệ thống") {
		t.Fatalf("vi autostart = %q", menuRow(t, vi, ActionAutostart).Title)
	}
	en := BuildViewLang(snapOf(1, 0, 0, "connected"), true, "", "en")
	for id, want := range map[string]string{
		ActionOpenTUI: "Open 502Drive", ActionQueueToggle: "Pause all",
		ActionStopDaemon: "Stop 502Drive…", ActionQuit: "Quit tray",
	} {
		if got := menuRow(t, en, id).Title; got != want {
			t.Errorf("en %s = %q, want %q", id, got, want)
		}
	}
	if !strings.Contains(menuRow(t, en, ActionAutostart).Title, "Start on login") {
		t.Fatalf("en autostart = %q", menuRow(t, en, ActionAutostart).Title)
	}
	// Start is for a down daemon (hidden while online); stopping is the confirmed item, only online.
	if !menuRow(t, en, ActionDaemon).Hidden && menuRow(t, en, ActionDaemon).Enabled {
		t.Fatal("start is offered while the daemon is online")
	}
	if !menuRow(t, en, ActionStopDaemon).Enabled {
		t.Fatal("stop is disabled while online")
	}
	// The default BuildView stays English for the CLI contract.
	if got := menuRow(t, BuildView(snapOf(0, 0, 0, "connected"), true, ""), ActionOpenTUI).Title; got != "Open 502Drive" {
		t.Fatalf("default view = %q", got)
	}
	withJob := BuildViewLang(snapOf(1, 0, 0, "connected", Job{SourceName: "Alpha", State: "running", Completed: 2}), true, "", "vi")
	if !strings.Contains(menuRow(t, withJob, "job:0").Title, "ĐANG CHẠY") {
		t.Fatalf("vi job line = %q", menuRow(t, withJob, "job:0").Title)
	}
}

func TestBuildViewKeepsAStableStructure(t *testing.T) {
	empty := BuildView(Snapshot{}, false, "")
	busy := BuildView(snapOf(1, 2, 0, "connected", Job{SourceName: "Alpha", State: "running", Completed: 3}),
		true, "copied job_1")
	if len(empty.Items) != len(busy.Items) {
		t.Fatalf("menu length changed: %d vs %d", len(empty.Items), len(busy.Items))
	}
	for i := range empty.Items {
		if empty.Items[i].ID != busy.Items[i].ID || empty.Items[i].Separator != busy.Items[i].Separator {
			t.Fatalf("item %d changed identity: %+v vs %+v", i, empty.Items[i], busy.Items[i])
		}
	}
	ids := []string{}
	for _, it := range busy.Items {
		if it.Separator {
			continue
		}
		ids = append(ids, it.ID)
		for _, c := range it.Children {
			if c.Separator {
				continue
			}
			ids = append(ids, c.ID)
		}
	}
	// Grouped: status, clone/open, jobs, queue pause, update, settings, quit and stop.
	want := []string{"status", "status:work", "status:attention", ActionDaemon, ActionPaste, ActionOpenTUI, "jobs", "job:0", "job:1", "job:2",
		ActionOpenAll, ActionOpenResult, ActionQueueToggle, ActionUpdate, "settings", ActionAutostart, ActionUpdateCheck, ActionQuit, ActionStopDaemon}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if busy.State != StateBusy {
		t.Fatalf("busy view state = %q", busy.State)
	}
	if menuRow(t, busy, "job:0").Title == "" || !menuRow(t, busy, "job:1").Hidden {
		t.Fatalf("job slots = %+v", menuRow(t, busy, "job:0"))
	}
	if !strings.Contains(busy.Title, "copied job_1") {
		t.Fatalf("note not shown in the title: %q", busy.Title)
	}
	offline := BuildView(Snapshot{}, false, "")
	if menuRow(t, offline, ActionPaste).Enabled {
		t.Fatal("paste is enabled while offline")
	}
	if menuRow(t, offline, ActionDaemon).Title != "Start 502Drive" {
		t.Fatalf("daemon label = %q", menuRow(t, offline, ActionDaemon).Title)
	}
	if !menuRow(t, offline, ActionDaemon).Enabled || menuRow(t, offline, ActionStopDaemon).Enabled {
		t.Fatalf("start/stop enabled flags offline")
	}
	if menuRow(t, offline, ActionQueueToggle).Enabled {
		t.Fatal("queue pause is enabled while the daemon is offline")
	}
}

func TestPasteAndQueueStateAreGreyedOutWithAReason(t *testing.T) {
	notConnected := BuildView(snapOf(0, 0, 0, "reconnect_required"), true, "")
	paste := menuRow(t, notConnected, ActionPaste)
	if paste.Enabled || !strings.Contains(paste.Title, "Google") {
		t.Fatalf("paste without a Google account = %+v", paste)
	}
	offline := BuildView(Snapshot{}, false, "")
	if q := menuRow(t, offline, ActionQueueToggle); q.Enabled || !strings.Contains(q.Title, "not running") {
		t.Fatalf("queue toggle offline = %+v", q)
	}
	paused := snapOf(0, 1, 0, "connected")
	paused.Status.QueuePaused = true
	toggle := menuRow(t, BuildView(paused, true, ""), ActionQueueToggle)
	if toggle.Title != "Resume all" || !toggle.Enabled {
		t.Fatalf("paused toggle = %+v", toggle)
	}
}

func TestBuildViewEnablesTheResultOnlyWhenThereIsOne(t *testing.T) {
	done := Job{SourceName: "Alpha", State: "succeeded", ResultURL: "https://drive.google.com/drive/folders/abc"}
	view := BuildView(snapOf(0, 0, 0, "connected", done), true, "")
	if !menuRow(t, view, ActionOpenResult).Enabled {
		t.Fatal("result item disabled although a copy completed")
	}
	none := BuildView(snapOf(0, 0, 0, "connected"), true, "")
	if menuRow(t, none, ActionOpenResult).Enabled {
		t.Fatal("result item enabled without a finished copy")
	}
}

func TestBuildViewStripsControlCharacters(t *testing.T) {
	name := "evil\x1b[31m\nname\u202ereversed"
	view := BuildView(snapOf(0, 0, 0, "connected", Job{SourceName: name, State: "running"}), true, "")
	line := menuRow(t, view, "job:0").Title
	if strings.ContainsAny(line, "\x1b\n\r\u202e") {
		t.Fatalf("control characters reached the menu: %q", line)
	}
	if !strings.Contains(line, "evil") || !strings.Contains(line, "name") {
		t.Fatalf("the name was mangled: %q", line)
	}
}

// The tooltip carries the detail the menu does not: the account, the counts,
// and what to do when something needs the user or the daemon is down.
func TestTooltipCarriesDetailAndHints(t *testing.T) {
	blocked := BuildViewLang(snapOf(0, 0, 2, "connected"), true, "", "en")
	if !strings.Contains(blocked.Tooltip, "Google: connected") || !strings.Contains(blocked.Tooltip, "Something needs you") {
		t.Fatalf("blocked tooltip = %q", blocked.Tooltip)
	}
	offline := BuildViewLang(Snapshot{}, false, "", "en")
	if !strings.Contains(offline.Tooltip, "Start 502Drive") {
		t.Fatalf("offline tooltip = %q", offline.Tooltip)
	}
}

// Vietnamese menu labels, including the reasons, must stay short enough for a
// menu row (48 characters) so they are not cut off by the desktop.
func TestVietnameseMenuLabelsFitTheRow(t *testing.T) {
	offline := BuildViewLang(Snapshot{}, false, "", "vi")
	notConnected := BuildViewLang(snapOf(0, 0, 0, "reconnect_required"), true, "", "vi")
	for _, v := range []View{offline, notConnected} {
		for _, it := range v.Items {
			for _, row := range append([]Item{it}, it.Children...) {
				if n := utf8.RuneCountInString(row.Title); n > 48 && !strings.HasPrefix(row.ID, "status") && !strings.HasPrefix(row.ID, jobSlotID) {
					t.Errorf("row %q is %d characters: %q", row.ID, n, row.Title)
				}
			}
		}
	}
}

// Only a changed view reaches the desktop: an idle poll pushes nothing.
func TestAgentPushesOnlyChangedViews(t *testing.T) {
	paused := false
	o := Options{
		Host: Host{GOOS: "linux", Getenv: func(string) string { return "" }},
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, AccountStatus: "connected", QueuePaused: paused}, nil
		},
		Jobs:       func(context.Context) ([]Job, error) { return nil, nil },
		Copy:       func(context.Context, string) (string, error) { return "", nil },
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0) },
		QueuePause: func(context.Context, bool) error { return nil },
	}
	a := newAgent(o)
	p := &fakePresenter{}
	a.p = p
	a.refresh(context.Background())
	a.refresh(context.Background())
	a.refresh(context.Background())
	if len(p.views) != 1 {
		t.Fatalf("idle polls pushed %d views, want 1", len(p.views))
	}
	paused = true
	a.refresh(context.Background())
	if len(p.views) != 2 || p.last().State != StatePaused {
		t.Fatalf("pause did not reach the desktop: %d views, last %q", len(p.views), p.last().State)
	}
}

// The queue toggle calls the daemon with the opposite of the state it shows.
func TestQueueToggleCallsTheDaemonWithTheOppositeState(t *testing.T) {
	var calls []bool
	paused := false
	o := Options{
		Host: Host{GOOS: "linux", Getenv: func(string) string { return "" }},
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, AccountStatus: "connected", QueuePaused: paused}, nil
		},
		Jobs: func(context.Context) ([]Job, error) { return nil, nil },
		Copy: func(context.Context, string) (string, error) { return "", nil },
		QueuePause: func(_ context.Context, p bool) error {
			calls = append(calls, p)
			paused = p
			return nil
		},
	}
	a := newAgent(o)
	a.p = &fakePresenter{}
	a.refresh(context.Background())
	a.action(context.Background(), ActionQueueToggle)
	a.action(context.Background(), ActionQueueToggle)
	if len(calls) != 2 || calls[0] != true || calls[1] != false {
		t.Fatalf("toggle calls = %v, want [true false]", calls)
	}
}

// The update row and tooltip appear only when the opt-in check found a newer
// release, and the Settings row shows whether the check is on.
func TestBuildViewShowsAnAvailableUpdate(t *testing.T) {
	snap := snapOf(0, 0, 0, "connected")
	snap.Update, snap.CheckUpdates = "v9.0.0", true
	v := BuildView(snap, true, "")
	if row := menuRow(t, v, ActionUpdate); row.Hidden || !strings.Contains(row.Title, "v9.0.0") {
		t.Fatalf("update row = %+v", row)
	}
	if !strings.Contains(v.Tooltip, "New release available: v9.0.0") {
		t.Fatalf("tooltip = %q", v.Tooltip)
	}
	if row := menuRow(t, v, ActionUpdateCheck); !strings.Contains(row.Title, "✓") {
		t.Fatalf("update check row = %+v", row)
	}

	none := BuildView(snapOf(0, 0, 0, "connected"), true, "")
	if row := menuRow(t, none, ActionUpdate); !row.Hidden {
		t.Fatalf("update row without a release = %+v", row)
	}
	if row := menuRow(t, none, ActionUpdateCheck); strings.Contains(row.Title, "✓") {
		t.Fatalf("disabled check is marked on: %+v", row)
	}
}

// The update row needs a second click; only that second click opens the fixed
// install command.
func TestUpdateNeedsConfirmationAndRunsTheFixedCommand(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	installs := 0
	a := newAgent(Options{
		Now: func() time.Time { return now },
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, AccountStatus: "connected"}, nil
		},
		Jobs: func(context.Context) ([]Job, error) { return nil, nil },
		Copy: func(context.Context, string) (string, error) { return "job_1", nil },
		InstallUpdate: func(context.Context) error {
			installs++
			return nil
		},
	})
	p := &fakePresenter{}
	a.p = p
	a.mu.Lock()
	a.update = "v9.0.0"
	a.mu.Unlock()
	a.refresh(t.Context())

	a.action(t.Context(), ActionUpdate)
	if installs != 0 {
		t.Fatal("the first click installed a release")
	}
	row := itemByID(p.last(), ActionUpdate)
	if row == nil || row.Hidden || !strings.Contains(row.Title, "Confirm") || !strings.Contains(row.Title, "v9.0.0") {
		t.Fatalf("confirmation row = %+v", row)
	}

	now = now.Add(time.Second)
	a.action(t.Context(), ActionUpdate)
	if installs != 1 {
		t.Fatalf("installs = %d after the confirmation click", installs)
	}
	if row := itemByID(p.last(), ActionUpdate); row == nil || !strings.Contains(row.Title, "Update to") {
		t.Fatalf("row after install = %+v", row)
	}
}

func TestUpdateConfirmationExpires(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	installs := 0
	a := newAgent(Options{
		Now: func() time.Time { return now },
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, AccountStatus: "connected"}, nil
		},
		Jobs:          func(context.Context) ([]Job, error) { return nil, nil },
		Copy:          func(context.Context, string) (string, error) { return "", nil },
		InstallUpdate: func(context.Context) error { installs++; return nil },
	})
	a.p = &fakePresenter{}
	a.mu.Lock()
	a.update = "v9.0.0"
	a.mu.Unlock()
	a.action(t.Context(), ActionUpdate) // armed
	now = now.Add(11 * time.Second)     // past the window
	a.action(t.Context(), ActionUpdate)
	if installs != 0 {
		t.Fatal("a stale confirmation installed a release")
	}
	now = now.Add(time.Second)
	a.action(t.Context(), ActionUpdate)
	if installs != 1 {
		t.Fatalf("installs = %d after re-arming", installs)
	}
}

func TestUpdateCheckTogglePersistsThePreference(t *testing.T) {
	var calls []bool
	a := newAgent(Options{
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, AccountStatus: "connected"}, nil
		},
		Jobs:            func(context.Context) ([]Job, error) { return nil, nil },
		Copy:            func(context.Context, string) (string, error) { return "", nil },
		SetCheckUpdates: func(on bool) error { calls = append(calls, on); return nil },
	})
	p := &fakePresenter{}
	a.p = p
	a.refresh(t.Context())
	a.action(t.Context(), ActionUpdateCheck)
	a.action(t.Context(), ActionUpdateCheck)
	if len(calls) != 2 || calls[0] != true || calls[1] != false {
		t.Fatalf("setter calls = %v, want [true false]", calls)
	}
	if row := menuRow(t, p.last(), ActionUpdateCheck); strings.Contains(row.Title, "✓") {
		t.Fatalf("check still marked on: %+v", row)
	}
}

// The daily check is opt-in: it never runs while the preference is off and an
// error keeps the last known tag.
func TestCheckForUpdateHonoursOptIn(t *testing.T) {
	calls, fail := 0, false
	a := newAgent(Options{UpdateCheck: func(context.Context) (string, error) {
		calls++
		if fail {
			return "", errors.New("offline")
		}
		return "v9.0.0", nil
	}})
	a.checkForUpdate(t.Context())
	if calls != 0 {
		t.Fatal("the check ran while it was disabled")
	}
	a.mu.Lock()
	a.checkUpdates = true
	a.mu.Unlock()
	a.checkForUpdate(t.Context())
	if calls != 1 {
		t.Fatalf("checks = %d, want 1", calls)
	}
	a.mu.Lock()
	tag := a.update
	a.mu.Unlock()
	if tag != "v9.0.0" {
		t.Fatalf("tag = %q", tag)
	}
	fail = true
	a.checkForUpdate(t.Context())
	a.mu.Lock()
	tag = a.update
	a.mu.Unlock()
	if tag != "v9.0.0" {
		t.Fatalf("a failed check cleared the tag: %q", tag)
	}
}

// The install command is fixed; only the running binary's path is inserted.
func TestUpdateCommandUsesFixedArguments(t *testing.T) {
	h := Host{GOOS: "linux", Getenv: func(string) string { return "" },
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil }}
	argv, err := UpdateCommand(h, "/opt/502drive/502drive")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(argv[len(argv)-5:], " ")
	want := "/opt/502drive/502drive update --install --restart --yes"
	if got != want {
		t.Fatalf("tail = %q, want %q (argv %v)", got, want, argv)
	}
}

// Every state has an icon that stays distinct from idle at small sizes.
func TestEveryStateIconIsReadableAt16px(t *testing.T) {
	idle, err := drawIcon(StateIdle, 0, 16)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range AllStates {
		img, err := drawIcon(st, 0, 16)
		if err != nil {
			t.Fatalf("%s: %v", st, err)
		}
		if st == StateIdle {
			continue
		}
		if string(img.Pix) == string(idle.Pix) {
			t.Errorf("%s icon is identical to idle at 16px", st)
		}
	}
}

func TestPollDelay(t *testing.T) {
	if got := PollDelay(true, StateBusy, 0); got != 3*time.Second {
		t.Fatalf("busy delay = %v", got)
	}
	if got := PollDelay(true, StateIdle, 0); got != 10*time.Second {
		t.Fatalf("idle delay = %v", got)
	}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second, 30 * time.Second}
	for i, w := range want {
		if got := PollDelay(false, StateOffline, i); got != w {
			t.Fatalf("backoff %d = %v, want %v", i, got, w)
		}
	}
}

// fakeHost records argv and answers LookPath from a fixed set.
type fakeHost struct {
	goos    string
	env     map[string]string
	have    map[string]bool
	run     func(argv []string) (string, error)
	ran     [][]string
	started [][]string
}

func (h *fakeHost) Host() Host {
	return Host{
		GOOS:   h.goos,
		Getenv: func(k string) string { return h.env[k] },
		LookPath: func(name string) (string, error) {
			if h.have[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, argv []string) (string, error) {
			h.ran = append(h.ran, argv)
			if h.run != nil {
				return h.run(argv)
			}
			return "", nil
		},
		Start: func(argv []string) error {
			h.started = append(h.started, argv)
			return nil
		},
	}
}

func TestTerminalCommandTriesTheDocumentedOrder(t *testing.T) {
	exe := "/usr/bin/502drive"
	cases := []struct {
		name string
		have map[string]bool
		env  map[string]string
		want []string
	}{
		{"gnome", map[string]bool{"gnome-terminal": true}, nil, []string{"gnome-terminal", "--", exe, "menu"}},
		{"ghostty before a gnome-terminal wrapper", map[string]bool{"ghostty": true, "gnome-terminal": true}, nil,
			[]string{"ghostty", "-e", exe, "menu"}},
		{"x-terminal-emulator is the last resort", map[string]bool{"x-terminal-emulator": true, "gnome-terminal": true}, nil,
			[]string{"gnome-terminal", "--", exe, "menu"}},
		{"only x-terminal-emulator", map[string]bool{"x-terminal-emulator": true}, nil,
			[]string{"x-terminal-emulator", "-e", exe, "menu"}},
		{"konsole", map[string]bool{"konsole": true}, nil, []string{"konsole", "-e", exe, "menu"}},
		{"kitty", map[string]bool{"kitty": true}, nil, []string{"kitty", exe, "menu"}},
		{"env terminal wins", map[string]bool{"kitty": true, "alacritty": true}, map[string]string{"TERMINAL": "alacritty"},
			[]string{"alacritty", "-e", exe, "menu"}},
		{"unknown env terminal", map[string]bool{"mystery": true}, map[string]string{"TERMINAL": "mystery"},
			[]string{"mystery", "-e", exe, "menu"}},
		{"env terminal missing falls back", map[string]bool{"kitty": true}, map[string]string{"TERMINAL": "broken"},
			[]string{"kitty", exe, "menu"}},
	}
	for _, tc := range cases {
		h := &fakeHost{goos: "linux", have: tc.have, env: tc.env}
		got, err := TerminalCommand(h.Host(), exe)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Fatalf("%s: argv %q, want %q", tc.name, got, tc.want)
		}
	}
	h := &fakeHost{goos: "linux", have: map[string]bool{}}
	if _, err := TerminalCommand(h.Host(), exe); err == nil {
		t.Fatal("no terminal available was not reported")
	}
}

func TestTerminalCommandOnWindows(t *testing.T) {
	exe := `C:\Program Files\502Drive\502drive.exe`
	h := &fakeHost{goos: "windows", have: map[string]bool{"wt.exe": true, "cmd": true}}
	got, err := TerminalCommand(h.Host(), exe)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\x00") != strings.Join([]string{"wt.exe", exe, "menu"}, "\x00") {
		t.Fatalf("argv = %q", got)
	}
	h = &fakeHost{goos: "windows", have: map[string]bool{"cmd": true}}
	got, err = TerminalCommand(h.Host(), exe)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\x00") != strings.Join([]string{"cmd", "/c", "start", "", exe, "menu"}, "\x00") {
		t.Fatalf("cmd fallback argv = %q", got)
	}
}

func TestReadDriveLinkValidatesTheClipboard(t *testing.T) {
	const url = "https://drive.google.com/drive/folders/abcdefghijklmnop"
	h := &fakeHost{goos: "linux", have: map[string]bool{"xclip": true},
		run: func(argv []string) (string, error) { return "\n  " + url + "  \n", nil }}
	link, err := ReadDriveLink(context.Background(), h.Host())
	if err != nil {
		t.Fatal(err)
	}
	if link != "https://drive.google.com/open?id=abcdefghijklmnop" {
		t.Fatalf("link = %q", link)
	}

	multi := &fakeHost{goos: "linux", have: map[string]bool{"xclip": true},
		run: func([]string) (string, error) { return url + "\n" + url + "\n", nil }}
	if _, err = ReadDriveLink(context.Background(), multi.Host()); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("two links accepted: %v", err)
	}
	text := &fakeHost{goos: "linux", have: map[string]bool{"xclip": true},
		run: func([]string) (string, error) { return "just some text", nil }}
	if _, err = ReadDriveLink(context.Background(), text.Host()); err == nil {
		t.Fatal("arbitrary clipboard text was accepted")
	}
	none := &fakeHost{goos: "linux", have: map[string]bool{}}
	if _, err = ReadDriveLink(context.Background(), none.Host()); err == nil || !strings.Contains(err.Error(), "clipboard tool") {
		t.Fatalf("no clipboard tool: %v", err)
	}
}

func TestOpenURLRejectsNonDriveTargets(t *testing.T) {
	h := &fakeHost{goos: "linux", have: map[string]bool{"xdg-open": true}}
	for _, url := range []string{"http://drive.google.com/x", "https://evil.example/drive.google.com/", "file:///etc/passwd"} {
		if err := OpenURL(context.Background(), h.Host(), url); err == nil {
			t.Fatalf("%s was accepted", url)
		}
	}
	if len(h.ran) != 0 {
		t.Fatalf("a rejected URL reached exec: %v", h.ran)
	}
	ok := "https://drive.google.com/drive/folders/abcdefghijklmnop"
	if err := OpenURL(context.Background(), h.Host(), ok); err != nil {
		t.Fatal(err)
	}
	if len(h.ran) != 1 || h.ran[0][0] != "xdg-open" {
		t.Fatalf("open argv = %v", h.ran)
	}
}

func TestControlDaemonUsesFixedCommands(t *testing.T) {
	linux := &fakeHost{goos: "linux", have: map[string]bool{"systemctl": true}}
	if _, err := ControlDaemon(context.Background(), linux.Host(), DaemonStart, "/usr/bin/502drive"); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{"systemctl", "--user", "start", "502drive.service"}, "\x00")
	if got := strings.Join(linux.ran[0], "\x00"); got != want {
		t.Fatalf("start argv = %q", got)
	}
	if _, err := ControlDaemon(context.Background(), linux.Host(), DaemonStop, "/usr/bin/502drive"); err != nil {
		t.Fatal(err)
	}
	want = strings.Join([]string{"systemctl", "--user", "stop", "502drive.service"}, "\x00")
	if got := strings.Join(linux.ran[1], "\x00"); got != want {
		t.Fatalf("stop argv = %q", got)
	}

	// Windows never kills by name: stopping goes through POST /v1/shutdown.
	windows := &fakeHost{goos: "windows", have: map[string]bool{"taskkill": true}}
	if _, err := ControlDaemon(context.Background(), windows.Host(), DaemonStart, `C:\502Drive\502drive.exe`); err != nil {
		t.Fatal(err)
	}
	if len(windows.started) != 1 || windows.started[0][1] != "serve" {
		t.Fatalf("windows start argv = %v", windows.started)
	}
	if _, err := ControlDaemon(context.Background(), windows.Host(), DaemonStop, ""); err == nil {
		t.Fatal("windows daemon stop must refuse to run taskkill")
	}
	if len(windows.ran) != 0 {
		t.Fatalf("windows stop ran %v", windows.ran)
	}
	if _, err := ControlDaemon(context.Background(), (&fakeHost{goos: "darwin"}).Host(), DaemonStart, ""); err == nil {
		t.Fatal("macOS daemon control should be refused")
	}
}

// fakePresenter records what the agent renders.
type fakePresenter struct {
	mu      sync.Mutex
	views   []View
	icons   []State
	frames  []int
	quits   int
	showErr error
}

func (p *fakePresenter) show(v View) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.views = append(p.views, v)
	return p.showErr
}

func (p *fakePresenter) setIcon(st State, frame int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.icons = append(p.icons, st)
	p.frames = append(p.frames, frame)
	return nil
}

func (p *fakePresenter) quit() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.quits++
}

func (p *fakePresenter) last() View {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.views) == 0 {
		return View{}
	}
	return p.views[len(p.views)-1]
}

func TestAgentRefreshAndActions(t *testing.T) {
	h := &fakeHost{goos: "linux", have: map[string]bool{"xclip": true, "systemctl": true},
		run: func(argv []string) (string, error) {
			if argv[0] == "xclip" {
				return "https://drive.google.com/drive/folders/abcdefghijklmnop", nil
			}
			return "unit started\n", nil
		}}
	var copied []string
	statusErr := error(nil)
	o := Options{
		Motion: true,
		Exe:    "/usr/bin/502drive",
		Host:   h.Host(),
		Status: func(context.Context) (Status, error) {
			if statusErr != nil {
				return Status{}, statusErr
			}
			return Status{Ready: true, Running: 1, AccountStatus: "connected", Version: "test"}, nil
		},
		Jobs: func(context.Context) ([]Job, error) {
			return []Job{{SourceName: "Alpha", State: "running", Completed: 2}}, nil
		},
		Copy: func(_ context.Context, link string) (string, error) {
			copied = append(copied, link)
			return "job_1", nil
		},
		Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	a := newAgent(o)
	p := &fakePresenter{}
	a.p = p

	view, online := a.refresh(context.Background())
	if !online || view.State != StateBusy {
		t.Fatalf("view = %+v online=%v", view, online)
	}

	a.action(context.Background(), ActionPaste)
	if len(copied) != 1 || !strings.Contains(copied[0], "open?id=") {
		t.Fatalf("copied = %v", copied)
	}
	if !strings.Contains(p.last().Title, "copied job_1") {
		t.Fatalf("note not shown: %q", p.last().Title)
	}

	// The pasted job is not in the snapshot, so the result item stays disabled.
	a.action(context.Background(), ActionOpenResult)
	if !strings.Contains(p.last().Title, "no completed copy") {
		t.Fatalf("result note = %q", p.last().Title)
	}

	// Offline keeps the last snapshot but reports the offline view.
	statusErr = errors.New("connection refused")
	view, online = a.refresh(context.Background())
	if online || view.State != StateOffline {
		t.Fatalf("offline view = %+v", view)
	}
	if menuRow(t, view, "job:0").Hidden {
		t.Fatal("the last jobs were dropped while offline")
	}

	// Start daemon is the only daemon action from the tray toggle; stopping
	// lives in the confirmed stop item, and quitting leaves the daemon alone.
	a.action(context.Background(), ActionDaemon)
	if len(h.ran) == 0 || h.ran[len(h.ran)-1][0] != "systemctl" {
		t.Fatalf("daemon not started through systemd: %v", h.ran)
	}
	a.action(context.Background(), ActionQuit)
	if p.quits != 1 {
		t.Fatalf("quit count = %d", p.quits)
	}
}

// stopAgent builds an agent whose clock and stop counter the test owns.
func stopAgent(t *testing.T, running, queued int, now *time.Time, stops *int, mutate func(*Options)) (*agent, *fakePresenter) {
	t.Helper()
	o := Options{
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, Running: running, Queued: queued, AccountStatus: "connected"}, nil
		},
		Jobs: func(context.Context) ([]Job, error) { return nil, nil },
		Copy: func(context.Context, string) (string, error) { return "", nil },
		Now:  func() time.Time { return *now },
		StopDaemon: func(context.Context) (string, error) {
			*stops++
			return "daemon stopped", nil
		},
	}
	if mutate != nil {
		mutate(&o)
	}
	a := newAgent(o)
	p := &fakePresenter{}
	a.p = p
	a.refresh(context.Background())
	return a, p
}

func itemByID(v View, id string) *Item {
	for i := range v.Items {
		if v.Items[i].ID == id {
			return &v.Items[i]
		}
	}
	return nil
}

func TestQuitTrayKeepsTheDaemonRunning(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	stops := 0
	a, p := stopAgent(t, 1, 1, &now, &stops, nil)
	a.action(context.Background(), ActionQuit)
	if p.quits != 1 || stops != 0 {
		t.Fatalf("quits = %d, daemon stops = %d", p.quits, stops)
	}
}

func TestQuitStopsTheDaemonOnlyWhenOptIn(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	stops := 0
	a, p := stopAgent(t, 0, 0, &now, &stops, func(o *Options) { o.QuitStopsDaemon = true })
	a.action(context.Background(), ActionQuit)
	if p.quits != 1 || stops != 1 {
		t.Fatalf("quits = %d, daemon stops = %d", p.quits, stops)
	}
}

func TestStopDaemonNeedsASecondClickWhileJobsRun(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	stops := 0
	a, p := stopAgent(t, 1, 2, &now, &stops, nil)
	// The first click only arms the confirmation; it never stops anything.
	a.action(context.Background(), ActionStopDaemon)
	if stops != 0 {
		t.Fatal("the first click stopped the daemon")
	}
	item := itemByID(p.last(), ActionStopDaemon)
	if item == nil || !strings.Contains(item.Title, "3") || !strings.Contains(item.Title, "confirm") {
		t.Fatalf("confirm title = %+v", item)
	}
	// The second click inside the window stops and clears the arm.
	now = now.Add(time.Second)
	a.action(context.Background(), ActionStopDaemon)
	if stops != 1 {
		t.Fatalf("stops = %d after the confirm click", stops)
	}
	a.refresh(context.Background())
	if got := itemByID(p.last(), ActionStopDaemon); got == nil || got.Title != "Stop 502Drive…" {
		t.Fatalf("title after stop = %+v", got)
	}
}

func TestStopDaemonConfirmationExpires(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	stops := 0
	a, _ := stopAgent(t, 0, 1, &now, &stops, nil)
	a.action(context.Background(), ActionStopDaemon) // armed
	now = now.Add(11 * time.Second)                  // past the window
	a.action(context.Background(), ActionStopDaemon)
	if stops != 0 {
		t.Fatal("a stale confirmation stopped the daemon")
	}
	now = now.Add(time.Second)
	a.action(context.Background(), ActionStopDaemon)
	if stops != 1 {
		t.Fatalf("stops = %d after re-arming", stops)
	}
}

func TestStopDaemonActsAtOnceWhenIdle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	stops := 0
	a, _ := stopAgent(t, 0, 0, &now, &stops, nil)
	a.action(context.Background(), ActionStopDaemon)
	if stops != 1 {
		t.Fatalf("stops = %d with no jobs", stops)
	}
}

func TestStopDaemonDoesNothingWhileOffline(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	stops := 0
	a, _ := stopAgent(t, 1, 0, &now, &stops, func(o *Options) {
		o.Status = func(context.Context) (Status, error) { return Status{}, errors.New("down") }
	})
	a.action(context.Background(), ActionStopDaemon)
	if stops != 0 {
		t.Fatal("an offline tray stopped the daemon")
	}
}

func TestAgentIconLoopAnimatesOnlyWhenBusyAndAllowed(t *testing.T) {
	o := Options{
		Motion: true,
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, Running: 1, AccountStatus: "connected"}, nil
		},
		Jobs: func(context.Context) ([]Job, error) { return nil, nil },
		Copy: func(context.Context, string) (string, error) { return "", nil },
	}
	a := newAgent(o)
	p := &fakePresenter{}
	a.p = p
	if _, online := a.refresh(context.Background()); !online {
		t.Fatal("refresh failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	a.iconLoop(ctx)
	p.mu.Lock()
	icons := append([]State(nil), p.icons...)
	p.mu.Unlock()
	if len(icons) < 2 {
		t.Fatalf("busy animation produced %d icon updates", len(icons))
	}

	// No motion: the loop never touches the icon.
	o.Motion = false
	a2 := newAgent(o)
	p2 := &fakePresenter{}
	a2.p = p2
	if _, online := a2.refresh(context.Background()); !online {
		t.Fatal("refresh failed")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	a2.iconLoop(ctx2)
	p2.mu.Lock()
	quiet := len(p2.icons)
	p2.mu.Unlock()
	if quiet != 0 {
		t.Fatalf("--no-motion still animated %d times", quiet)
	}
}

func TestIconIsTheProjectLogoWithStateOverlays(t *testing.T) {
	idle, err := PNG(StateIdle, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodeIcon(idle)
	if err != nil {
		t.Fatal(err)
	}
	// the centre of the logo is opaque artwork and the squircle corners are transparent
	if _, _, _, a := img.At(32, 32).RGBA(); a == 0 {
		t.Fatal("logo centre is transparent")
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Fatal("logo corner should be transparent")
	}
	// offline halves the opacity of everything
	off, _ := PNG(StateOffline, 0, 64)
	oi, _ := decodeIcon(off)
	_, _, _, a1 := img.At(32, 32).RGBA()
	_, _, _, a2 := oi.At(32, 32).RGBA()
	if a2 == 0 || a2 >= a1 {
		t.Fatalf("offline alpha %d vs idle %d", a2, a1)
	}
	// every ICO size renders without error, including odd ones
	for _, size := range []int{16, 24, 32, 48, 64, 128, 256} {
		if _, err := PNG(StateNeeds, 0, size); err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
	}
}

func TestJobTransitionsNotifyOnlyOnEndStatesSeenBefore(t *testing.T) {
	prev := []Job{{ID: "a", SourceName: "Photos", State: "running"}, {ID: "b", SourceName: "Docs", State: "queued"},
		{ID: "c", SourceName: "Old", State: "succeeded"}}
	cur := []Job{{ID: "a", SourceName: "Photos", State: "succeeded"}, {ID: "b", SourceName: "Docs", State: "running"},
		{ID: "c", SourceName: "Old", State: "succeeded"}, {ID: "d", SourceName: "New", State: "succeeded"}}
	got := jobTransitions(prev, cur, "en")
	if len(got) != 1 || got[0].Body != "Photos" || got[0].Title != "Copy finished" {
		t.Fatalf("transitions = %+v, want only Photos finishing", got)
	}
	failed := jobTransitions([]Job{{ID: "a", State: "running"}}, []Job{{ID: "a", State: "failed", SourceName: "X"}}, "vi")
	if len(failed) != 1 || failed[0].Title != "Copy thất bại" {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestNotifyArgvNeverGivesNamesToTheShellOrMarkup(t *testing.T) {
	argv := notifyArgv(Notice{Title: "t", Body: "--evil <b>x</b> & $(rm -rf ~)"})
	if argv[0] != "notify-send" || argv[4] == "" {
		t.Fatalf("argv = %q", argv)
	}
	sep := -1
	for i, a := range argv {
		if a == "--" {
			sep = i
		}
	}
	if sep < 0 || sep > 3 {
		t.Fatalf("text must come after --: %q", argv)
	}
	if got := argv[len(argv)-1]; strings.Contains(got, "<b>") || !strings.Contains(got, "&lt;b&gt;") {
		t.Fatalf("markup not escaped: %q", got)
	}
}

func TestTitleShowsRealCopyCountsOnlyWhenKnown(t *testing.T) {
	total := 1284
	snap := Snapshot{Status: Status{Ready: true, Running: 1, AccountStatus: "connected"},
		Jobs: []Job{{ID: "a", State: "running", Completed: 312, Total: &total}}}
	if got := BuildViewLang(snap, true, "", "en").Title; !strings.Contains(got, "312 / 1284") {
		t.Fatalf("title = %q", got)
	}
	snap.Jobs[0].Total = nil
	if got := BuildViewLang(snap, true, "", "en").Title; strings.Contains(got, "/") {
		t.Fatalf("unknown total must not show a count: %q", got)
	}
}

func TestTrayAutostartToggle(t *testing.T) {
	state := false
	p := &fakePresenter{}
	a := newAgent(Options{
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, AccountStatus: "connected"}, nil
		},
		Jobs: func(context.Context) ([]Job, error) {
			return nil, nil
		},
		Copy: func(context.Context, string) (string, error) {
			return "job_1", nil
		},
		AutostartToggle: func() (bool, error) {
			state = !state
			return state, nil
		},
	})
	a.p = p

	ctx := t.Context()
	a.refresh(ctx)

	// Toggle on
	a.action(ctx, ActionAutostart)
	if !state {
		t.Fatal("autostart should be toggled on")
	}

	// Toggle off
	a.action(ctx, ActionAutostart)
	if state {
		t.Fatal("autostart should be toggled off")
	}
}

func TestTrayDynamicLanguageChangeWithoutRestart(t *testing.T) {
	currentLang := "en"
	p := &fakePresenter{}
	a := newAgent(Options{
		Status: func(context.Context) (Status, error) {
			return Status{Ready: true, AccountStatus: "connected"}, nil
		},
		Jobs: func(context.Context) ([]Job, error) {
			return nil, nil
		},
		Copy: func(context.Context, string) (string, error) {
			return "job_1", nil
		},
		Language: func(context.Context) (string, error) {
			return currentLang, nil
		},
	})
	a.p = p

	ctx := t.Context()
	view, _ := a.refresh(ctx)
	if got := menuRow(t, view, ActionOpenTUI).Title; got != "Open 502Drive" {
		t.Fatalf("expected English 'Open 502Drive', got %q", got)
	}

	// Dynamically change language preference to Vietnamese
	currentLang = "vi"
	view, _ = a.refresh(ctx)
	if got := menuRow(t, view, ActionOpenTUI).Title; got != "Mở 502Drive" {
		t.Fatalf("expected Vietnamese 'Mở 502Drive' after dynamic change, got %q", got)
	}
}

// Quiet hours suppress desktop notifications; outside them the notice is sent.
func TestNotifySkippedDuringQuietHours(t *testing.T) {
	notices := []Notice{{Title: "done", Body: "x"}}
	for _, tc := range []struct {
		name  string
		quiet func(context.Context) (bool, error)
		want  int
	}{
		{"nil quiet sends", nil, 1},
		{"not quiet sends", func(context.Context) (bool, error) { return false, nil }, 1},
		{"quiet suppresses", func(context.Context) (bool, error) { return true, nil }, 0},
		{"lookup error suppresses", func(context.Context) (bool, error) { return true, errors.New("down") }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &fakeHost{goos: "linux", have: map[string]bool{"notify-send": true}}
			a := &agent{o: Options{Host: h.Host(), Quiet: tc.quiet}}
			a.notify(context.Background(), notices)
			if len(h.ran) != tc.want {
				t.Fatalf("ran %d notify commands, want %d", len(h.ran), tc.want)
			}
		})
	}
}
