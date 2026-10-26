package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheckExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   int
	}{
		{"ready", 200, `{"data":{"ready":true,"schema_version":5,"worker_ready":true},"request_id":"req_1"}`, ExitOK},
		{"not ready", 409, `{"error":{"code":"not_ready","message":"The service is not ready."},"request_id":"req_2"}`, ExitInternal},
		{"unauthorized", 401, `{"error":{"code":"unauthenticated","message":"no"},"request_id":"req_3"}`, ExitInternal},
		{"server error", 500, `{"error":{"code":"internal_error","message":"boom"},"request_id":"req_4"}`, ExitInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/health/ready" {
					t.Errorf("path = %q", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}), "healthcheck")
			if code != tc.want {
				t.Fatalf("exit %d, want %d (stderr %q)", code, tc.want, errOut)
			}
			switch tc.want {
			case ExitOK:
				if strings.TrimSpace(out) != "ready" {
					t.Fatalf("stdout = %q", out)
				}
			default:
				if strings.TrimSpace(errOut) == "" {
					t.Fatal("no reason on stderr")
				}
			}
		})
	}
}

func TestHealthcheckIsOneOrZeroForDocker(t *testing.T) {
	// Docker only distinguishes 0 from non-zero; every failure must be 1.
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := ts.URL
	ts.Close() // nothing listens any more
	var out, errOut strings.Builder
	code := Execute(t.Context(), []string{"--config", privateDir(t) + "/config.json", "--endpoint", url, "healthcheck"},
		IO{In: strings.NewReader(""), Out: &out, Err: &errOut, Getenv: func(k string) string {
			if k == "DRIVE502_TOKEN" {
				return "tok"
			}
			return ""
		}})
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "not ready") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestHealthcheckJSONIsTheEnvelope(t *testing.T) {
	code, out, errOut := driveCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"ready": true, "schema_version": 5}, "request_id": "req_1"})
	}), "healthcheck", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var env struct {
		Data struct {
			Ready bool `json:"ready"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not an envelope: %v (%q)", err, out)
	}
	if !env.Data.Ready || env.RequestID != "req_1" {
		t.Fatalf("envelope = %+v", env)
	}
}
