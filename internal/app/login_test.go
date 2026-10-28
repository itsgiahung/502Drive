package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"golang.org/x/oauth2"

	"github.com/itsgiahung/502Drive/internal/drive"
)

func loginService(t *testing.T) (*Service, string) {
	t.Helper()
	svc, _ := newTestService(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/revoke" {
			w.WriteHeader(http.StatusInternalServerError) // Google could not revoke
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer","expires_in":3600,"scope":"https://www.googleapis.com/auth/drive"}`))
	}))
	t.Cleanup(ts.Close)
	auth, err := drive.NewAuth(t.TempDir(), oauth2.Config{ClientID: "id", ClientSecret: "s",
		Endpoint: oauth2.Endpoint{AuthURL: "http://127.0.0.1:1/auth", TokenURL: ts.URL + "/token"}, Scopes: []string{drive.Scope}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.Close() })
	svc.Auth = auth
	return svc, ts.URL
}

// The same Idempotency-Key, sent by many callers at once, opens one session.
func TestConcurrentLoginsWithOneKeyShareOneSession(t *testing.T) {
	svc, _ := loginService(t)
	const callers = 16
	ids := make([]string, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s, err := svc.StartLogin(t.Context(), Owner, "same-key-0001", 0)
			if err != nil {
				t.Errorf("start: %v", err)
				return
			}
			ids[i] = s.ID
		}()
	}
	close(start)
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("one key must give one session, got %v", ids)
		}
	}
}

// A revoke Google did not confirm keeps its warning even if an old login
// session, which once connected, is polled afterwards.
func TestOldLoginStatusCannotClearAFailedRevokeWarning(t *testing.T) {
	svc, base := loginService(t)
	sess, err := svc.StartLogin(t.Context(), Owner, "login-key-0001", 0)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(sess.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	resp, err := http.Get(q.Get("redirect_uri") + "?code=x&state=" + q.Get("state"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got, err := svc.LoginStatus(t.Context(), Owner, sess.ID); err != nil || got.State != drive.SessionConnected {
		t.Fatalf("login did not connect: %+v %v", got, err)
	}
	if _, err = svc.Revoke(t.Context(), Owner, "revoke-key-0001", base+"/revoke"); err != nil {
		t.Fatal(err)
	}
	pending := func() string {
		v, _, _ := svc.Store.Setting(t.Context(), settingRevokePending)
		return v
	}
	if pending() != "1" {
		t.Fatalf("a revoke Google rejected must stay pending, got %q", pending())
	}
	if _, err = svc.LoginStatus(t.Context(), Owner, sess.ID); err != nil { // the old session is polled again
		t.Logf("old session: %v", err)
	}
	if pending() != "1" {
		t.Fatalf("polling an old session cleared the revoke warning")
	}
}
