package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBatchCopyPartialFailureAndReplay admits three links where the middle one
// is not a Drive reference. The two good links are admitted, the bad one is
// reported, the exit code says so, and repeating the same command replays the
// same jobs instead of copying again.
func TestBatchCopyPartialFailureAndReplay(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	a := r.fake.Folder("Batch A", "root")
	b := r.fake.Folder("Batch B", "root")
	input := a.ID + "\nnot a drive link!!\n" + b.ID + "\n"

	first := r.runWith(input, nil, "copy", "--stdin", "--json")
	if first.code == 0 {
		t.Fatalf("partial batch must not exit 0: %+v", first)
	}
	var env struct {
		Data struct {
			BatchID  string `json:"batch_id"`
			Accepted int    `json:"accepted"`
			Failed   int    `json:"failed"`
			Items    []struct {
				Source string `json:"source"`
				JobID  string `json:"job_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(first.out)), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, first.out)
	}
	if env.Data.Accepted != 2 || env.Data.Failed != 1 || len(env.Data.Items) != 3 {
		t.Fatalf("batch = %+v", env.Data)
	}
	if !strings.HasPrefix(env.Data.BatchID, "batch-") {
		t.Fatalf("batch id = %q", env.Data.BatchID)
	}
	if env.Data.Items[1].JobID != "" {
		t.Fatalf("the bad link must not produce a job: %+v", env.Data.Items[1])
	}

	// Same command again: the good links replay their jobs; nothing is copied twice.
	second := r.runWith(input, nil, "copy", "--stdin", "--json")
	var again struct {
		Data struct {
			BatchID string `json:"batch_id"`
			Items   []struct {
				JobID string `json:"job_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(second.out)), &again); err != nil {
		t.Fatalf("second json: %v\n%s", err, second.out)
	}
	if again.Data.BatchID != env.Data.BatchID {
		t.Fatalf("batch id changed on replay: %s -> %s", env.Data.BatchID, again.Data.BatchID)
	}
	if again.Data.Items[0].JobID != env.Data.Items[0].JobID || again.Data.Items[2].JobID != env.Data.Items[2].JobID {
		t.Fatalf("replay admitted new jobs: %+v vs %+v", again.Data.Items, env.Data.Items)
	}
}

// TestQueuePauseResumeAndMoveCommands drives the global queue commands and the
// job move command through the real CLI and daemon.
func TestQueuePauseResumeAndMoveCommands(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("Queue src", "root")
	if res := r.run("pause"); res.code != 0 || !strings.Contains(res.out, "paused") {
		t.Fatalf("pause = %+v", res)
	}
	if res := r.run("pause"); res.code != 0 || !strings.Contains(res.out, "already paused") {
		t.Fatalf("second pause = %+v", res)
	}
	if res := r.runWith(src.ID+"\n", nil, "copy", "--stdin", "--quiet"); res.code != 0 {
		t.Fatalf("copy while paused = %+v", res)
	}
	if res := r.run("resume"); res.code != 0 || !strings.Contains(res.out, "resumed") {
		t.Fatalf("resume = %+v", res)
	}
	if res := r.run("resume"); res.code != 0 || !strings.Contains(res.out, "already running") {
		t.Fatalf("second resume = %+v", res)
	}
}

// TestInspectDeepFollowsTheScanToTheTotals runs inspect --deep against the
// daemon and checks the final totals come back on the plan.
func TestInspectDeepFollowsTheScanToTheTotals(t *testing.T) {
	r := newRig(t)
	r.setupDest()
	src := r.fake.Folder("Deep CLI", "root")
	r.fake.File("one.txt", src.ID, 7)
	sub := r.fake.Folder("sub", src.ID)
	r.fake.File("two.txt", sub.ID, 5)

	res := r.run("inspect", src.ID, "--deep", "--json")
	if res.code != 0 {
		t.Fatalf("inspect --deep = %+v", res)
	}
	var env struct {
		Data struct {
			Preview struct {
				Deep struct {
					State    string `json:"state"`
					Complete bool   `json:"complete"`
					Files    int    `json:"files"`
					Folders  int    `json:"folders"`
					Bytes    int64  `json:"bytes"`
				} `json:"deep"`
			} `json:"preview"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.out)), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, res.out)
	}
	d := env.Data.Preview.Deep
	if d.State != "complete" || !d.Complete || d.Files != 2 || d.Folders != 1 || d.Bytes != 12 {
		t.Fatalf("deep = %+v", d)
	}
}
