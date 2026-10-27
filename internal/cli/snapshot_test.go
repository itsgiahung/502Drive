package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestJSONOutputSnapshotAndExitCodesGuardsAgainstRegression asserts that
// JSON output envelope structures and exit codes remain strictly invariant.
func TestJSONOutputSnapshotAndExitCodesGuardsAgainstRegression(t *testing.T) {
	run := func(args ...string) (int, string, string) {
		var out, errb strings.Builder
		code := Execute(t.Context(), args, IO{In: strings.NewReader(""), Out: &out, Err: &errb})
		return code, out.String(), errb.String()
	}

	// 1. Version command --json
	code, stdout, stderr := run("version", "--json")
	if code != ExitOK {
		t.Fatalf("version --json code = %d, stderr = %s", code, stderr)
	}
	var vEnv struct {
		Data struct {
			Version string `json:"version"`
			Go      string `json:"go"`
			OS      string `json:"os"`
			Arch    string `json:"arch"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(stdout), &vEnv); err != nil {
		t.Fatalf("version JSON envelope invalid: %v\nOutput: %s", err, stdout)
	}
	if vEnv.Data.Version == "" || vEnv.Data.OS == "" {
		t.Fatalf("version data incomplete: %+v", vEnv)
	}

	// 2. Autostart status --json
	code, stdout, stderr = run("autostart", "status", "--json")
	if code != ExitOK {
		t.Fatalf("autostart status --json code = %d, stderr = %s", code, stderr)
	}
	var autoEnv struct {
		Data struct {
			Enabled  bool   `json:"enabled"`
			Location string `json:"location"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(stdout), &autoEnv); err != nil {
		t.Fatalf("autostart status JSON invalid: %v\nOutput: %s", err, stdout)
	}

	// 3. Command error (e.g. jobs get nope) with --json
	r := newRig(t)
	res := r.run("jobs", "get", "nope", "--json")
	if res.out != "" || res.code != ExitNotFound {
		t.Fatalf("error leaked to stdout or wrong code: %+v", res)
	}
	var errEnv struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(res.errOut), &errEnv); err != nil {
		t.Fatalf("error JSON envelope invalid: %v\nStderr: %s", err, res.errOut)
	}
	if errEnv.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want not_found", errEnv.Error.Code)
	}
}
