package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/config"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

func TestOAuthCallbackWakesReconnectWithoutWaitingForCooldown(t *testing.T) {
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"fixture-access","refresh_token":"fixture-refresh","token_type":"Bearer","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{"user":{"emailAddress":"fixture@example.test","displayName":"Fixture"}}`))
	}))
	t.Cleanup(google.Close)
	dir := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(dir, "data"), TokenFile: filepath.Join(dir, "api-token"), OAuthClientFile: filepath.Join(dir, "oauth.json")}
	for path, data := range map[string]string{
		cfg.TokenFile:       "synthetic-api-token",
		cfg.OAuthClientFile: `{"installed":{"client_id":"fixture","client_secret":"fixture","redirect_uris":["http://127.0.0.1"],"auth_uri":"` + google.URL + `/auth","token_uri":"` + google.URL + `/token"}}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := state.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	err = store.SetSetting(t.Context(), heartbeatKey, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano))
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	cfg.Listen = listener.Addr().String()
	ctx, cancel := context.WithCancel(t.Context())
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- Run(ctx, cfg, Options{Listener: listener, DriveEndpoint: google.URL + "/", Maintenance: true, Started: func(string) { close(started) }})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("daemon: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
	})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not start")
	}
	call := func(method, path string, out any) {
		t.Helper()
		req, _ := http.NewRequestWithContext(t.Context(), method, "http://"+cfg.Listen+path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer synthetic-api-token")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "fixture-login")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var envelope struct{ Data json.RawMessage }
		if err = json.NewDecoder(resp.Body).Decode(&envelope); err != nil || resp.StatusCode >= 300 {
			t.Fatalf("%s %s: status=%d err=%v", method, path, resp.StatusCode, err)
		}
		if err = json.Unmarshal(envelope.Data, out); err != nil {
			t.Fatal(err)
		}
	}
	var status struct {
		Account   struct{ Status string }
		Lifecycle struct {
			State  string
			Online bool
		}
	}
	waitFor(t, "reconnect required before login", func() bool {
		call("GET", "/v1/status", &status)
		return status.Lifecycle.State == "reconnect_required"
	})
	var sess drive.AuthSession
	call("POST", "/v1/account/auth-sessions", &sess)
	u, err := url.Parse(sess.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	resp, err := http.Get(q.Get("redirect_uri") + "?code=fixture&state=" + url.QueryEscape(q.Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d", resp.StatusCode)
	}
	waitFor(t, "connected account and online lifecycle after callback", func() bool {
		call("GET", "/v1/status", &status)
		return status.Account.Status == "connected" && status.Lifecycle.State == "running" && status.Lifecycle.Online
	})
}
