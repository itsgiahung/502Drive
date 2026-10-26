package drive

import (
	"context"
	"golang.org/x/oauth2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOAuthRejectsWrongStateWithoutConsumingSessionAndPersistsEncryptedToken(t *testing.T) {
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e := r.ParseForm(); e != nil {
			t.Error(e)
		}
		if r.Form.Get("code_verifier") == "" {
			t.Error("missing PKCE")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"access-fixture","refresh_token":"refresh-fixture","token_type":"Bearer","expires_in":3600,"scope":"https://www.googleapis.com/auth/drive"}`))
	}))
	defer token.Close()
	cfg := oauth2.Config{ClientID: "synthetic", Scopes: []string{Scope}, Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: token.URL}}
	dir := t.TempDir()
	a, e := NewAuth(dir, cfg, func(context.Context, string) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	session, e := a.Start(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(session.AuthorizationURL)
	redirect := u.Query().Get("redirect_uri")
	resp, e := http.Get(redirect + "?state=wrong&code=fixture")
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatal("invalid callback reported success")
	}
	resp, e = http.Get(redirect + "?state=" + url.QueryEscape(u.Query().Get("state")) + "&code=fixture")
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("valid callback rejected", resp.StatusCode)
	}
	status, e := a.Session(session.ID)
	if e != nil || status.State != "connected" {
		t.Fatalf("session %+v %v", status, e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "google-token.enc"))
	if e != nil || strings.Contains(string(b), "refresh-fixture") {
		t.Fatal("plaintext or missing token", e)
	}
	a.Close()
	a, e = NewAuth(dir, cfg, func(context.Context, string) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	tok, e := a.Token()
	if e != nil || tok.AccessToken != "access-fixture" {
		t.Fatal("restore", e)
	}
}
func TestOAuthValidationFailureNeverReportsConnected(t *testing.T) {
	cfg := oauth2.Config{ClientID: "synthetic", Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "http://127.0.0.1:1"}}
	a, e := NewAuth(t.TempDir(), cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	s, e := a.Start(context.Background(), 0)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(s.AuthorizationURL)
	c := http.Client{Timeout: time.Second}
	r, e := c.Get(u.Query().Get("redirect_uri") + "?error=access_denied&state=" + u.Query().Get("state"))
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	got, _ := a.Session(s.ID)
	if got.State != "failed" {
		t.Fatal(got)
	}
	if _, e = a.Token(); e == nil {
		t.Fatal("connected without credential")
	}
}
