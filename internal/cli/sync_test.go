package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copiedCollection copies a folder and returns the collection id.
func (r *rig) copiedCollection(name string) string {
	r.t.Helper()
	src := r.fake.Folder(name, "root")
	if res := r.run("copy", src.ID, "--wait", "--quiet"); res.code != 0 {
		r.t.Fatalf("copy: %+v", res)
	}
	list := decodeData(r.t, r.run("collections", "list", "--json").out)
	items := list["items"].([]any)
	if len(items) == 0 {
		r.t.Fatal("no collection after copy")
	}
	for _, it := range items {
		c := it.(map[string]any)
		if c["source_name"] == name {
			return c["id"].(string)
		}
	}
	r.t.Fatalf("collection %q not listed", name)
	return ""
}

func (r *rig) syncJSON(args ...string) (map[string]any, result) {
	r.t.Helper()
	res := r.run(append(args, "--json")...)
	if res.code != 0 {
		return nil, res
	}
	return decodeData(r.t, res.out), res
}

func TestSyncEnableStatusAndActions(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	id := r.copiedCollection("Library")

	status := r.run("sync", "status", id)
	if status.code != ExitOK || !strings.Contains(status.out, "off") {
		t.Fatalf("status before enable: %+v", status)
	}
	if !strings.Contains(status.errOut, "sync enable") {
		t.Fatalf("no hint for an unsynced collection: %+v", status)
	}

	sub, res := r.syncJSON("sync", "enable", id, "--daily", "22:00", "--timezone", "UTC")
	if sub == nil {
		t.Fatalf("enable: %+v", res)
	}
	if sub["mode"] != "scheduled" || sub["state"] != "active" || sub["next_run_at"] == nil {
		t.Fatalf("subscription = %+v", sub)
	}
	if sub["deletion_policy"] != "preserve_destination" || sub["content_policy"] != "versioned_copy" {
		t.Fatalf("policies = %+v", sub)
	}
	schedule, _ := sub["schedule"].(map[string]any)
	if schedule["kind"] != "daily" || schedule["local_time"] != "22:00" || schedule["timezone"] != "UTC" {
		t.Fatalf("schedule = %+v", schedule)
	}

	shown, res := r.syncJSON("sync", "status", id)
	if shown == nil {
		t.Fatalf("status: %+v", res)
	}
	human := r.run("sync", "status", id)
	if human.code != ExitOK || !strings.Contains(human.out, "daily at 22:00") {
		t.Fatalf("human status: %+v", human)
	}

	paused, res := r.syncJSON("sync", "pause", id)
	if paused == nil || paused["state"] != "paused" {
		t.Fatalf("pause: %+v %+v", paused, res)
	}
	resumed, res := r.syncJSON("sync", "resume", id)
	if resumed == nil || resumed["state"] != "active" || resumed["next_run_at"] == nil {
		t.Fatalf("resume: %+v %+v", resumed, res)
	}
	stopped, res := r.syncJSON("sync", "stop", id)
	if stopped == nil {
		t.Fatalf("stop: %+v", res)
	}
	if s := stopped["state"]; s != "stopped" && s != "stopping" {
		t.Fatalf("stopped state = %v", s)
	}

	// A second collection has no subscription: sync actions refuse clearly.
	other := r.copiedCollection("Unsynced")
	if res := r.run("sync", "pause", other); res.code != ExitConflict || !strings.Contains(res.errOut, "sync enable") {
		t.Fatalf("pause without a subscription: %+v", res)
	}
}

func TestSyncEnablePresets(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	id := r.copiedCollection("Presets")

	weekly, res := r.syncJSON("sync", "enable", id, "--weekly", "mon,wed@07:30", "--timezone", "Asia/Ho_Chi_Minh", "--policy", "replace_copy")
	if weekly == nil {
		t.Fatalf("weekly: %+v", res)
	}
	schedule := weekly["schedule"].(map[string]any)
	days := schedule["weekdays"].([]any)
	if schedule["kind"] != "weekly" || schedule["local_time"] != "07:30" || len(days) != 2 || days[0].(float64) != 1 || days[1].(float64) != 3 {
		t.Fatalf("weekly schedule = %+v", schedule)
	}
	if weekly["content_policy"] != "replace_copy" {
		t.Fatalf("policy = %v", weekly["content_policy"])
	}

	interval, res := r.syncJSON("sync", "enable", id, "--every", "6h")
	if interval == nil {
		t.Fatalf("interval: %+v", res)
	}
	schedule = interval["schedule"].(map[string]any)
	if schedule["kind"] != "interval" || schedule["interval_minutes"].(float64) != 360 || schedule["anchor_at"] == nil {
		t.Fatalf("interval schedule = %+v", schedule)
	}

	rt, res := r.syncJSON("sync", "enable", id, "--mode", "realtime")
	if rt == nil {
		t.Fatalf("realtime: %+v", res)
	}
	if rt["mode"] != "realtime" || rt["schedule"] != nil {
		t.Fatalf("realtime = %+v", rt)
	}
	if s := rt["state"]; s != "bootstrapping" && s != "active" && s != "needs_reconcile" {
		t.Fatalf("realtime state = %v", s)
	}
}

func TestSyncEnableDryRunPreviewsThreeRuns(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	id := r.copiedCollection("Dry run")
	data, res := r.syncJSON("sync", "enable", id, "--daily", "22:00", "--timezone", "UTC", "--dry-run")
	if data == nil {
		t.Fatalf("dry run: %+v", res)
	}
	runs, _ := data["next_runs"].([]any)
	if len(runs) != 3 {
		t.Fatalf("next_runs = %v", data["next_runs"])
	}
	sched, _ := data["schedule"].(map[string]any)
	if sched["kind"] != "daily" || sched["timezone"] != "UTC" {
		t.Fatalf("schedule = %+v", sched)
	}
	// A dry run must not create a subscription.
	after := decodeData(t, r.run("collections", "get", id, "--json").out)
	if after["sync"] != nil {
		t.Fatalf("dry run saved a subscription: %+v", after["sync"])
	}
	human := r.run("sync", "enable", id, "--daily", "22:00", "--timezone", "UTC", "--dry-run")
	if human.code != ExitOK || strings.Count(human.out, "UTC") < 3 {
		t.Fatalf("human preview: %+v", human)
	}
}

func TestSyncEnableUsesTheTimezonePreference(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	id := r.copiedCollection("Prefs tz")
	token, err := os.ReadFile(filepath.Join(r.home, "api-token"))
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{Endpoint: r.addr, Token: strings.TrimSpace(string(token))}
	ctx := context.Background()
	if _, err := client.Do(ctx, "PATCH", "/v1/preferences", nil, map[string]any{"timezone": "America/New_York"}, NewKey()); err != nil {
		t.Fatal(err)
	}
	data, res := r.syncJSON("sync", "enable", id, "--daily", "22:00")
	if data == nil {
		t.Fatalf("enable: %+v", res)
	}
	sched, _ := data["schedule"].(map[string]any)
	if sched["timezone"] != "America/New_York" {
		t.Fatalf("schedule did not use the preference: %+v", sched)
	}
	// An explicit flag still wins.
	data, res = r.syncJSON("sync", "enable", id, "--daily", "22:00", "--timezone", "UTC")
	if data == nil {
		t.Fatalf("enable with flag: %+v", res)
	}
	sched, _ = data["schedule"].(map[string]any)
	if sched["timezone"] != "UTC" {
		t.Fatalf("explicit zone ignored: %+v", sched)
	}
}

func TestSyncEnableRejectsBadPresets(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	id := r.copiedCollection("Bad presets")
	for _, c := range []struct {
		name string
		args []string
	}{
		{"no preset", []string{"sync", "enable", id}},
		{"two presets", []string{"sync", "enable", id, "--daily", "10:00", "--every", "30m"}},
		{"weekly without time", []string{"sync", "enable", id, "--weekly", "mon,wed"}},
		{"weekly duplicate day", []string{"sync", "enable", id, "--weekly", "mon,mon@10:00"}},
		{"weekly unknown day", []string{"sync", "enable", id, "--weekly", "funday@10:00"}},
		{"sub-minute interval", []string{"sync", "enable", id, "--every", "7s"}},
		{"interval too short", []string{"sync", "enable", id, "--every", "1m"}},
		{"bad time", []string{"sync", "enable", id, "--daily", "25:00"}},
		{"bad zone", []string{"sync", "enable", id, "--daily", "10:00", "--timezone", "Mars/Olympus"}},
		{"realtime with preset", []string{"sync", "enable", id, "--mode", "realtime", "--daily", "10:00"}},
		{"bad mode", []string{"sync", "enable", id, "--mode", "cron", "--daily", "10:00"}},
		{"bad policy", []string{"sync", "enable", id, "--daily", "10:00", "--policy", "mirror"}},
	} {
		if res := r.run(c.args...); res.code != ExitUsage {
			t.Errorf("%s: exit %d (%s)", c.name, res.code, res.errOut)
		}
	}
}

func TestSyncRunNowCoalesces(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	id := r.copiedCollection("Run now")
	first, res := r.syncJSON("sync", "run", id)
	if first == nil {
		t.Fatalf("run: %+v", res)
	}
	if first["collection_id"] != id || first["id"] == nil {
		t.Fatalf("job = %+v", first)
	}
	second, res := r.syncJSON("sync", "run", id)
	if second == nil {
		t.Fatalf("second run: %+v", res)
	}
	if second["id"] != first["id"] {
		t.Fatalf("a second parallel run was admitted: %v vs %v", second["id"], first["id"])
	}
}

func TestMenuUsesTheLanguagePreference(t *testing.T) {
	t.Setenv("LC_ALL", "vi_VN.UTF-8")
	r := newRig(t)
	r.setupDest()
	vi := r.menu("0\n")
	if vi.code != 0 || !strings.Contains(vi.out, "Bạn muốn làm gì?") || !strings.Contains(vi.out, "Quay lại / Thoát") {
		t.Fatalf("vi menu: %+v", vi)
	}
	// Switch the preference to English and reopen the menu.
	token, err := os.ReadFile(filepath.Join(r.home, "api-token"))
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{Endpoint: r.addr, Token: strings.TrimSpace(string(token))}
	if _, err = client.Do(context.Background(), "PATCH", "/v1/preferences", nil, map[string]any{"language": "en"}, NewKey()); err != nil {
		t.Fatal(err)
	}
	en := r.menu("0\n")
	if en.code != 0 || !strings.Contains(en.out, "What would you like to do?") {
		t.Fatalf("en menu: %+v", en)
	}
}

func TestMenuSyncPreset(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	r.copiedCollection("Menu sync")
	res := r.menu("3\n1\n2\n0\n0\n")
	if res.code != 0 {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(res.out, "Sync scheduled: active") {
		t.Fatalf("menu did not enable the daily preset:\n%s", res.out)
	}
	if !strings.Contains(res.out, "daily at 22:00") {
		t.Fatalf("menu summary missing the preset:\n%s", res.out)
	}
}
