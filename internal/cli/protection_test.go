package cli

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProtectionFlagsPreserveUnchangedOptions(t *testing.T) {
	var body map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/collections/col_1/protection" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Method == "PUT" {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
		}
		reply(w, 200, map[string]any{"collection_id": "col_1", "revision": 4, "options": map[string]any{"mirror": false, "snapshots": true, "keep_days": 45, "keep_count": 12}})
	})
	code, out, errOut := driveCLI(t, h, "sync", "options", "col_1", "--mirror", "--json")
	if code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, out, errOut)
	}
	opts := body["options"].(map[string]any)
	if opts["mirror"] != true || opts["snapshots"] != true || opts["keep_days"] != float64(45) {
		t.Fatalf("body=%v", body)
	}
}

func TestSnapshotRestoreDefaultsToDryRun(t *testing.T) {
	var dry bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			reply(w, 200, map[string]any{"id": "snap_1", "revision": 3})
			return
		}
		var body struct {
			Dry      bool `json:"dry_run"`
			Expected int  `json:"expected_revision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		dry = body.Dry
		if body.Expected != 3 {
			t.Errorf("revision=%d", body.Expected)
		}
		reply(w, 202, jobRow("job_1", "queued", 1))
	})
	code, out, errOut := driveCLI(t, h, "snapshots", "restore", "snap_1", "--wait=false")
	if code != 0 || !dry {
		t.Fatalf("dry=%v code=%d out=%s err=%s", dry, code, out, errOut)
	}
}
