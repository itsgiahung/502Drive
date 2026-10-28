package drive

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type tokenServer struct {
	*httptest.Server
	hits   atomic.Int32
	body   atomic.Value // string
	status atomic.Int32
	delay  time.Duration
}

func newTokenServer(t *testing.T, body string) *tokenServer {
	ts := &tokenServer{}
	ts.body.Store(body)
	ts.status.Store(200)
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.hits.Add(1)
		time.Sleep(ts.delay)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(ts.status.Load()))
		w.Write([]byte(ts.body.Load().(string)))
	}))
	t.Cleanup(ts.Close)
	return ts
}

const goodToken = `{"access_token":"access-1","refresh_token":"refresh-1","token_type":"Bearer","expires_in":3600,"scope":"https://www.googleapis.com/auth/drive"}`

func cfgFor(ts *tokenServer) oauth2.Config {
	return oauth2.Config{ClientID: "synthetic", Scopes: []string{Scope}, Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: ts.URL, AuthStyle: oauth2.AuthStyleInParams}}
}

func startSession(t *testing.T, a *Auth) (AuthSession, string, string) {
	t.Helper()
	s, err := a.Start(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(s.AuthorizationURL)
	return s, u.Query().Get("redirect_uri"), u.Query().Get("state")
}

func get(t *testing.T, u string) int {
	t.Helper()
	c := http.Client{Timeout: 5 * time.Second}
	r, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	return r.StatusCode
}

func TestEarlyWrongPathMethodAndDuplicateCallbacksDoNotBreakTheSession(t *testing.T) {
	ts := newTokenServer(t, goodToken)
	a, err := NewAuth(t.TempDir(), cfgFor(ts), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	s, redirect, state := startSession(t, a)
	base := strings.TrimSuffix(redirect, callbackPath)
	if c := get(t, base+"/other?state="+state+"&code=x"); c != 404 {
		t.Errorf("wrong path = %d", c)
	}
	req, _ := http.NewRequest("POST", redirect+"?state="+state+"&code=x", nil)
	if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode != 405 {
		t.Errorf("POST callback: %v", err)
	}
	if c := get(t, redirect+"?code=x"); c != 400 { // no state at all
		t.Errorf("missing state = %d", c)
	}
	if got, _ := a.Session(s.ID); got.State != SessionPending {
		t.Fatalf("noise consumed the session: %+v", got)
	}
	if ts.hits.Load() != 0 {
		t.Fatal("exchange attempted for an invalid request")
	}
	if c := get(t, redirect+"?code=x&state="+state); c != 200 {
		t.Fatalf("valid callback = %d", c)
	}
	// the listener may already be closed; either way a replay must not succeed
	c := http.Client{Timeout: time.Second}
	if r, err := c.Get(redirect + "?code=x&state=" + state); err == nil {
		r.Body.Close()
		if r.StatusCode == 200 {
			t.Error("duplicate callback accepted")
		}
	}
	if ts.hits.Load() != 1 {
		t.Fatalf("token exchanged %d times", ts.hits.Load())
	}
}

func TestSessionExpiryAndPortConflict(t *testing.T) {
	ts := newTokenServer(t, goodToken)
	a, _ := NewAuth(t.TempDir(), cfgFor(ts), nil)
	defer a.Close()
	now := time.Now()
	a.now = func() time.Time { return now }
	s, redirect, state := startSession(t, a)
	now = now.Add(11 * time.Minute)
	if c := get(t, redirect+"?code=x&state="+state); c != 409 {
		t.Errorf("expired callback = %d", c)
	}
	if got, _ := a.Session(s.ID); got.State != SessionExpired {
		t.Errorf("state %s", got.State)
	}
	if ts.hits.Load() != 0 {
		t.Error("exchange after expiry")
	}
	// fixed port already taken: fail up front, before any URL is handed out
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	if _, err := a.Start(context.Background(), l.Addr().(*net.TCPAddr).Port); err == nil {
		t.Error("port conflict not reported")
	}
}

func TestSlowCallbackConnectionIsCutOff(t *testing.T) {
	old := readHeaderTimeout
	readHeaderTimeout = 150 * time.Millisecond
	defer func() { readHeaderTimeout = old }()
	ts := newTokenServer(t, goodToken)
	a, _ := NewAuth(t.TempDir(), cfgFor(ts), nil)
	defer a.Close()
	s, redirect, state := startSession(t, a)
	u, _ := url.Parse(redirect)
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("GET /oauth/callback?code=x HTTP/1.1\r\nHost: x\r\n")) // never finishes
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	conn.Read(buf) // returns once the server drops or answers 408
	if got, _ := a.Session(s.ID); got.State != SessionPending {
		t.Fatalf("slow client changed the session: %+v", got)
	}
	if c := get(t, redirect+"?code=x&state="+state); c != 200 {
		t.Fatalf("valid callback after slow client = %d", c)
	}
}

func TestLoginFailuresNeverLeaveACredential(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"no refresh token", `{"access_token":"a","token_type":"Bearer","expires_in":3600,"scope":"https://www.googleapis.com/auth/drive"}`},
		{"drive scope missing", `{"access_token":"a","refresh_token":"r","token_type":"Bearer","expires_in":3600,"scope":"openid email"}`},
	} {
		ts := newTokenServer(t, tc.body)
		dir := t.TempDir()
		a, _ := NewAuth(dir, cfgFor(ts), nil)
		s, redirect, state := startSession(t, a)
		if c := get(t, redirect+"?code=x&state="+state); c == 200 {
			t.Errorf("%s: reported success", tc.name)
		}
		if got, _ := a.Session(s.ID); got.State != SessionFailed || got.Error == "" {
			t.Errorf("%s: %+v", tc.name, got)
		}
		if _, err := os.Stat(filepath.Join(dir, tokenFile)); err == nil || a.Connected() {
			t.Errorf("%s: credential saved", tc.name)
		}
		a.Close()
	}
}

// A login that returns no refresh token must not borrow the old one: it could
// belong to another Google account. The previous credential stays as it was.
func TestReloginWithoutARefreshTokenNeverInheritsTheOldOne(t *testing.T) {
	ts := newTokenServer(t, goodToken)
	dir := t.TempDir()
	a, _ := NewAuth(dir, cfgFor(ts), nil)
	defer a.Close()
	_, redirect, state := startSession(t, a)
	get(t, redirect+"?code=x&state="+state)
	ts.body.Store(`{"access_token":"access-b","token_type":"Bearer","expires_in":3600,"scope":"https://www.googleapis.com/auth/drive"}`)
	_, redirect, state = startSession(t, a)
	if c := get(t, redirect+"?code=y&state="+state); c == 200 {
		t.Fatal("a login without a refresh token reported success")
	}
	tok, _ := a.Token()
	if tok.AccessToken != "access-1" || tok.RefreshToken != "refresh-1" {
		t.Fatalf("the previous credential must stay untouched: %+v", tok)
	}
}

func TestCallbackRejectsAnUnexpectedHostHeader(t *testing.T) {
	ts := newTokenServer(t, goodToken)
	a, _ := NewAuth(t.TempDir(), cfgFor(ts), nil)
	defer a.Close()
	s, redirect, state := startSession(t, a)
	req, _ := http.NewRequest(http.MethodGet, redirect+"?code=x&state="+state, nil)
	req.Host = "other-host.example.invalid"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("a callback with a foreign Host header was accepted")
	}
	if got, _ := a.Session(s.ID); got.State != SessionPending {
		t.Fatalf("the refused request must leave the session pending: %+v", got)
	}
}

func TestSaveFailureKeepsThePreviousCredential(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	ts := newTokenServer(t, goodToken)
	dir := t.TempDir()
	a, _ := NewAuth(dir, cfgFor(ts), nil)
	defer a.Close()
	_, redirect, state := startSession(t, a)
	get(t, redirect+"?code=x&state="+state)
	ts.body.Store(strings.Replace(goodToken, "access-1", "access-NEW", 1))
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	s, redirect, state := startSession(t, a)
	if c := get(t, redirect+"?code=y&state="+state); c == 200 {
		t.Fatal("success reported although the credential could not be saved")
	}
	if got, _ := a.Session(s.ID); got.State != SessionFailed {
		t.Fatalf("%+v", got)
	}
	if tok, err := a.Token(); err != nil || tok.AccessToken != "access-1" {
		t.Fatalf("valid credential replaced: %+v %v", tok, err)
	}
}

func TestRefreshIsSerialisedPersistedAndInvalidGrantStopsRetrying(t *testing.T) {
	ts := newTokenServer(t, `{"access_token":"fresh","token_type":"Bearer","expires_in":3600}`)
	ts.delay = 50 * time.Millisecond
	dir := t.TempDir()
	a, _ := NewAuth(dir, cfgFor(ts), nil)
	a.token = &oauth2.Token{AccessToken: "old", RefreshToken: "refresh-1", Expiry: time.Now().Add(-time.Hour), TokenType: "Bearer"}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := a.TokenSource(context.Background()).Token()
			if err != nil || tok.AccessToken != "fresh" {
				t.Errorf("%v %v", tok, err)
			}
		}()
	}
	wg.Wait()
	if ts.hits.Load() != 1 {
		t.Fatalf("refresh storm: %d token calls", ts.hits.Load())
	}
	a.Close()
	b, _ := NewAuth(dir, cfgFor(ts), nil)
	defer b.Close()
	if tok, err := b.Token(); err != nil || tok.RefreshToken != "refresh-1" {
		t.Fatalf("refreshed token not persisted with the refresh token: %+v %v", tok, err)
	}
	// Google revokes the grant
	ts.status.Store(400)
	ts.body.Store(`{"error":"invalid_grant"}`)
	b.mu.Lock()
	b.token.Expiry = time.Now().Add(-time.Hour)
	b.mu.Unlock()
	before := ts.hits.Load()
	for range 3 {
		_, err := b.TokenSource(context.Background()).Token()
		if err != ErrReconnectRequired {
			t.Fatalf("err = %v", err)
		}
	}
	if ts.hits.Load() != before+1 {
		t.Fatalf("kept hammering Google after invalid_grant: %d calls", ts.hits.Load()-before)
	}
	if !b.NeedsReconnect() {
		t.Fatal("reconnect flag not set")
	}
	if _, err := b.Token(); err != ErrReconnectRequired {
		t.Fatalf("Token() = %v", err)
	}
}

func TestTokenFileIsEncryptedTamperedFileIsRejectedAndRevokeForgets(t *testing.T) {
	ts := newTokenServer(t, goodToken)
	dir := t.TempDir()
	a, _ := NewAuth(dir, cfgFor(ts), nil)
	_, redirect, state := startSession(t, a)
	get(t, redirect+"?code=x&state="+state)
	raw, _ := os.ReadFile(filepath.Join(dir, tokenFile))
	if strings.Contains(string(raw), "refresh-1") || strings.Contains(string(raw), "access-1") {
		t.Fatal("token stored in plaintext")
	}
	for _, f := range []string{tokenFile, keyFile} {
		if i, _ := os.Stat(filepath.Join(dir, f)); i.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v", f, i.Mode().Perm())
		}
	}
	// revoke: local credential is removed even when Google is unreachable
	var revoked atomic.Value
	rv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		revoked.Store(r.PostForm.Get("token"))
	}))
	ok, err := a.Revoke(context.Background(), rv.URL)
	rv.Close()
	if err != nil || !ok || revoked.Load() != "refresh-1" {
		t.Fatalf("revoke %v %v %v", ok, err, revoked.Load())
	}
	if a.Connected() {
		t.Fatal("still connected after revoke")
	}
	a.Close()
	// tampering
	b, _ := NewAuth(dir, cfgFor(ts), nil)
	_, redirect, state = startSession(t, b)
	get(t, redirect+"?code=x&state="+state)
	b.Close()
	raw, _ = os.ReadFile(filepath.Join(dir, tokenFile))
	raw[len(raw)/2] ^= 1
	os.WriteFile(filepath.Join(dir, tokenFile), raw, 0o600)
	if _, err := NewAuth(dir, cfgFor(ts), nil); err == nil {
		t.Fatal("tampered credential accepted")
	}
	// loose key permissions are refused
	os.Chmod(filepath.Join(dir, keyFile), 0o644)
	if _, err := NewAuth(dir, cfgFor(ts), nil); err == nil {
		t.Fatal("group-readable master key accepted")
	}
	_ = fmt.Sprint
}
