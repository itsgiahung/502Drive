package drive

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// Scope is the broad Drive scope needed to copy files the user can read.
const Scope = "https://www.googleapis.com/auth/drive"

const (
	SessionPending   = "pending"
	SessionVerifying = "verifying"
	SessionConnected = "connected"
	SessionFailed    = "failed"
	SessionExpired   = "expired"

	callbackPath = "/oauth/callback"
	sessionTTL   = 10 * time.Minute
	tokenFile    = "google-token.enc"
	keyFile      = "master.key"
	formatV1     = "502drive-token-v1"
)

// readHeaderTimeout bounds how long a callback connection may dribble headers.
var readHeaderTimeout = 5 * time.Second

var ErrNotConnected = errors.New("google account is not connected")
var ErrReconnectRequired = errors.New("google account needs to be reconnected")

// AuthSession is the public view of a login attempt. It never carries the
// verifier, authorization code or token.
type AuthSession struct {
	ID               string    `json:"id"`
	State            string    `json:"state"`
	AuthorizationURL string    `json:"authorization_url,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
	Error            string    `json:"error,omitempty"`
}

type session struct {
	AuthSession
	oauthState string
	verifier   string
	listener   net.Listener
	server     *http.Server
	timer      *time.Timer
	done       bool
}

// Auth owns the OAuth client config, login sessions and the encrypted token.
type Auth struct {
	dir      string
	cfg      oauth2.Config
	validate func(ctx context.Context, accessToken string) error
	now      func() time.Time

	mu        sync.Mutex
	sessions  map[string]*session
	token     *oauth2.Token
	reconnect bool
	closed    bool
	gcm       cipher.AEAD
}

// NewAuth loads (or creates) the private key and any saved token in dir.
// validate is called with the fresh access token before a login is reported
// as connected.
func NewAuth(dir string, cfg oauth2.Config, validate func(context.Context, string) error) (*Auth, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := loadOrCreateKey(filepath.Join(dir, keyFile))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	a := &Auth{dir: dir, cfg: cfg, validate: validate, now: time.Now, sessions: map[string]*session{}, gcm: gcm}
	tok, err := a.readToken()
	if err != nil {
		return nil, err
	}
	a.token = tok
	return a, nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		if i, e := os.Lstat(path); e != nil || !i.Mode().IsRegular() || i.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("master key must be a private regular file")
		}
		k, e := base64.RawStdEncoding.DecodeString(string(b))
		if e != nil || len(k) != 32 {
			return nil, errors.New("master key is malformed")
		}
		return k, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	k := make([]byte, 32)
	if _, err = rand.Read(k); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	_, err = f.WriteString(base64.RawStdEncoding.EncodeToString(k))
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return nil, err
	}
	return k, nil
}

type tokenEnvelope struct {
	Format string `json:"format"`
	Nonce  []byte `json:"nonce"`
	Data   []byte `json:"data"`
}

func (a *Auth) readToken() (*oauth2.Token, error) {
	b, err := os.ReadFile(filepath.Join(a.dir, tokenFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var env tokenEnvelope
	if json.Unmarshal(b, &env) != nil || env.Format != formatV1 || len(env.Nonce) != a.gcm.NonceSize() {
		return nil, errors.New("saved token has an unsupported format")
	}
	plain, err := a.gcm.Open(nil, env.Nonce, env.Data, []byte(formatV1))
	if err != nil {
		return nil, errors.New("saved token cannot be decrypted with this key")
	}
	var t oauth2.Token
	if err = json.Unmarshal(plain, &t); err != nil {
		return nil, errors.New("saved token is corrupt")
	}
	return &t, nil
}

// writeToken replaces the saved token atomically; a failure leaves the old
// credential untouched.
func (a *Auth) writeToken(t *oauth2.Token) error {
	plain, err := json.Marshal(t)
	if err != nil {
		return err
	}
	nonce := make([]byte, a.gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	env, _ := json.Marshal(tokenEnvelope{Format: formatV1, Nonce: nonce, Data: a.gcm.Seal(nil, nonce, plain, []byte(formatV1))})
	tmp, err := os.CreateTemp(a.dir, "token-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(env)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(a.dir, tokenFile))
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Start binds the loopback listener first, then builds the authorization URL
// so the callback can never arrive before something is listening. port 0 asks
// the OS for a free port; the listener is kept for the whole session.
func (a *Auth) Start(ctx context.Context, port int) (AuthSession, error) {
	if a.cfg.ClientID == "" {
		return AuthSession{}, errors.New("oauth client is not configured")
	}
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return AuthSession{}, fmt.Errorf("callback port unavailable: %w", err)
	}
	id, e1 := randomString(18)
	state, e2 := randomString(24)
	verifier := oauth2.GenerateVerifier()
	if err = errors.Join(e1, e2); err != nil {
		l.Close()
		return AuthSession{}, err
	}
	cfg := a.cfg
	cfg.RedirectURL = "http://" + l.Addr().String() + callbackPath
	s := &session{oauthState: state, verifier: verifier, listener: l}
	s.ID = id
	s.State = SessionPending
	s.ExpiresAt = a.now().Add(sessionTTL)
	s.AuthorizationURL = cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", "consent"))

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) { a.callback(s, cfg, w, r) })
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		l.Close()
		return AuthSession{}, errors.New("auth is closed")
	}
	a.sessions[id] = s
	s.timer = time.AfterFunc(sessionTTL, func() { a.expire(s) })
	a.mu.Unlock()
	go s.server.Serve(l)
	return s.AuthSession, nil
}

func (a *Auth) expire(s *session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !s.done {
		s.done, s.State = true, SessionExpired
		a.shutdownLocked(s)
	}
}

func (a *Auth) shutdownLocked(s *session) {
	if s.timer != nil {
		s.timer.Stop()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}()
}

func reply(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	fmt.Fprintln(w, msg)
}

func (a *Auth) callback(s *session, cfg oauth2.Config, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		reply(w, http.StatusMethodNotAllowed, "Method not allowed.")
		return
	}
	q := r.URL.Query()
	// A request that does not carry our state is noise; it must not touch
	// the session.
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(s.oauthState)) != 1 {
		reply(w, http.StatusBadRequest, "Invalid login request. Return to 502drive and start again if needed.")
		return
	}
	a.mu.Lock()
	if s.done || !a.now().Before(s.ExpiresAt) {
		if !s.done {
			s.done, s.State = true, SessionExpired
			a.shutdownLocked(s)
		}
		a.mu.Unlock()
		reply(w, http.StatusConflict, "This login link is no longer valid. Start again from 502drive.")
		return
	}
	s.done = true // single use: later callbacks are rejected
	if q.Get("error") != "" || q.Get("code") == "" {
		s.State, s.Error = SessionFailed, "authorization was denied or incomplete"
		a.shutdownLocked(s)
		a.mu.Unlock()
		reply(w, http.StatusBadRequest, "Authorization was not granted. Return to 502drive to try again.")
		return
	}
	s.State = SessionVerifying
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	err := a.finish(ctx, s, cfg, q.Get("code"))
	a.mu.Lock()
	if err != nil {
		s.State, s.Error = SessionFailed, err.Error()
	} else {
		s.State = SessionConnected
	}
	a.shutdownLocked(s)
	a.mu.Unlock()
	if err != nil {
		reply(w, http.StatusBadGateway, "Could not finish connecting. Return to 502drive for details.")
		return
	}
	reply(w, http.StatusOK, "Connected. You can close this tab and return to 502drive.")
}

func (a *Auth) finish(ctx context.Context, s *session, cfg oauth2.Config, code string) error {
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(s.verifier))
	if err != nil {
		return errors.New("token exchange failed")
	}
	if granted, ok := tok.Extra("scope").(string); ok && granted != "" && !strings.Contains(" "+granted+" ", " "+Scope+" ") {
		return errors.New("the Drive permission was not granted; sign in again and allow access to Google Drive")
	}
	a.mu.Lock()
	old := a.token
	a.mu.Unlock()
	if tok.RefreshToken == "" {
		if old == nil || old.RefreshToken == "" {
			return errors.New("google did not return a refresh token; revoke 502drive access in your Google account and sign in again")
		}
		tok.RefreshToken = old.RefreshToken
	}
	if a.validate != nil {
		if err = a.validate(ctx, tok.AccessToken); err != nil {
			return fmt.Errorf("account validation failed: %w", err)
		}
	}
	if err = a.writeToken(tok); err != nil {
		return errors.New("could not save credential")
	}
	a.mu.Lock()
	a.token, a.reconnect = tok, false
	a.mu.Unlock()
	return nil
}

// Session returns the current status of a login attempt.
func (a *Auth) Session(id string) (AuthSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[id]
	if !ok {
		return AuthSession{}, errors.New("unknown auth session")
	}
	if !s.done && !a.now().Before(s.ExpiresAt) {
		s.done, s.State = true, SessionExpired
		a.shutdownLocked(s)
	}
	out := s.AuthSession
	if s.State != SessionPending { // URL is only useful while waiting
		out.AuthorizationURL = ""
	}
	return out, nil
}

// Token returns the cached credential without refreshing it.
func (a *Auth) Token() (*oauth2.Token, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token == nil {
		return nil, ErrNotConnected
	}
	if a.reconnect {
		return nil, ErrReconnectRequired
	}
	t := *a.token
	return &t, nil
}

// Connected reports whether a usable credential exists.
func (a *Auth) Connected() bool { _, err := a.Token(); return err == nil }

// NeedsReconnect reports whether the last refresh proved the grant invalid.
func (a *Auth) NeedsReconnect() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.reconnect
}

// TokenSource returns a refreshing source. Refreshes are serialized, new
// tokens are persisted, and invalid_grant marks the account as needing
// reconnect instead of retrying forever.
func (a *Auth) TokenSource(ctx context.Context) oauth2.TokenSource {
	return &persistingSource{a: a, ctx: ctx}
}

type persistingSource struct {
	a   *Auth
	ctx context.Context
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	a := p.a
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token == nil {
		return nil, ErrNotConnected
	}
	if a.reconnect {
		return nil, ErrReconnectRequired
	}
	if a.token.Valid() {
		t := *a.token
		return &t, nil
	}
	ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()
	fresh, err := a.cfg.TokenSource(ctx, a.token).Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.ErrorCode == "invalid_grant" {
			a.reconnect = true
			return nil, ErrReconnectRequired
		}
		return nil, errors.New("token refresh failed")
	}
	if fresh.RefreshToken == "" {
		fresh.RefreshToken = a.token.RefreshToken
	}
	if err = a.writeToken(fresh); err != nil {
		return nil, errors.New("could not save refreshed credential")
	}
	a.token = fresh
	t := *fresh
	return &t, nil
}

// Forget removes the local credential. It does not revoke the grant at Google.
func (a *Auth) Forget() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.Remove(filepath.Join(a.dir, tokenFile)); err != nil && !os.IsNotExist(err) {
		return err
	}
	a.token, a.reconnect = nil, false
	return nil
}

// Close stops all pending login listeners. It is safe to call repeatedly.
func (a *Auth) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	for _, s := range a.sessions {
		if !s.done {
			s.done, s.State = true, SessionExpired
		}
		if s.server != nil {
			s.server.Close()
		}
		if s.timer != nil {
			s.timer.Stop()
		}
	}
}

// DefaultRevokeEndpoint is Google's token revocation endpoint.
const DefaultRevokeEndpoint = "https://oauth2.googleapis.com/revoke"

// Revoke disables the credential locally first, then asks Google to revoke
// the grant. remoteOK=false means the local credential is gone but the grant
// may still exist at Google, which the caller must report truthfully.
func (a *Auth) Revoke(ctx context.Context, endpoint string) (remoteOK bool, err error) {
	a.mu.Lock()
	tok := a.token
	a.mu.Unlock()
	if tok == nil {
		return true, nil
	}
	if err = a.Forget(); err != nil {
		return false, err
	}
	if endpoint == "" {
		endpoint = DefaultRevokeEndpoint
	}
	secret := tok.RefreshToken
	if secret == "" {
		secret = tok.AccessToken
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(url.Values{"token": {secret}}.Encode()))
	if err != nil {
		return false, nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}
