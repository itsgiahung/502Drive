package app

import (
	"context"
	"net/http"
	"sync"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

const (
	settingAccountLabel  = "account_label"
	settingRevokePending = "account_revoke_pending"
)

type AccountView struct {
	Status    string
	Label     *string
	LastError *state.Failure
}

func (s *Service) Account(ctx context.Context) (AccountView, error) {
	v := AccountView{Status: "disconnected"}
	pending, _, err := s.Store.Setting(ctx, settingRevokePending)
	if err != nil {
		return v, err
	}
	if label, ok, _ := s.Store.Setting(ctx, settingAccountLabel); ok && label != "" && s.Auth != nil && s.Auth.Connected() {
		v.Label = &label
	}
	switch {
	case pending == "1":
		v.Status = "revoking"
		v.LastError = &state.Failure{Code: "revoke_pending", Message: "Local access was removed, but Google could not confirm the revocation. Remove 502drive from your Google account permissions."}
	case s.Auth == nil:
	case s.Auth.NeedsReconnect():
		v.Status = "reconnect_required"
		v.LastError = &state.Failure{Code: "account_reconnect_required", Message: "Google rejected the saved access. Connect again."}
	case s.Auth.Connected():
		v.Status = "connected"
	}
	return v, nil
}

var (
	loginKeys sync.Map   // idempotency key -> session id (login URLs are not stored durably)
	loginMu   sync.Mutex // makes look-up-or-start one step, so a key never opens two sessions
)

func (s *Service) StartLogin(ctx context.Context, p Principal, key string, port int) (drive.AuthSession, error) {
	if err := requireOwner(p); err != nil {
		return drive.AuthSession{}, err
	}
	if err := state.ValidateKey(key); err != nil {
		return drive.AuthSession{}, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	if s.Auth == nil {
		return drive.AuthSession{}, Fail(http.StatusConflict, "oauth_not_configured", "No Google OAuth client is configured. See docs/oauth-setup.md.")
	}
	loginMu.Lock()
	defer loginMu.Unlock()
	if id, ok := loginKeys.Load(key); ok {
		if sess, err := s.Auth.Session(id.(string)); err == nil {
			return sess, nil
		}
	}
	sess, err := s.Auth.Start(ctx, port)
	if err != nil {
		return drive.AuthSession{}, Fail(http.StatusConflict, "callback_unavailable", err.Error())
	}
	loginKeys.Store(key, sess.ID)
	return sess, nil
}

func (s *Service) LoginStatus(ctx context.Context, p Principal, id string) (drive.AuthSession, error) {
	if err := requireOwner(p); err != nil {
		return drive.AuthSession{}, err
	}
	if s.Auth == nil {
		return drive.AuthSession{}, state.ErrNotFound
	}
	sess, err := s.Auth.Session(id)
	if err != nil {
		return drive.AuthSession{}, state.ErrNotFound
	}
	// An old session that once connected must not clear a revoke warning after the
	// credential was removed: only a credential that is present now counts.
	if sess.State == drive.SessionConnected && s.Auth.Connected() {
		_ = s.Store.SetSetting(ctx, settingRevokePending, "0")
	}
	return sess, nil
}

// Revoke removes local access and asks Google to revoke the grant.
func (s *Service) Revoke(ctx context.Context, p Principal, key, endpoint string) (AccountView, error) {
	if err := requireOwner(p); err != nil {
		return AccountView{}, err
	}
	if err := state.ValidateKey(key); err != nil {
		return AccountView{}, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	if s.Auth == nil {
		return s.Account(ctx)
	}
	remote, err := s.Auth.Revoke(ctx, endpoint)
	if err != nil {
		return AccountView{}, Fail(http.StatusInternalServerError, "revoke_failed", "Could not remove the local credential.")
	}
	flag := "0"
	if !remote {
		flag = "1"
	}
	if err = s.Store.SetSetting(ctx, settingRevokePending, flag); err != nil {
		return AccountView{}, err
	}
	_ = s.Store.SetSetting(ctx, settingAccountLabel, "")
	return s.Account(ctx)
}

// RememberAccount stores the validated account label after a login.
func (s *Service) RememberAccount(ctx context.Context, label string) error {
	return s.Store.SetSetting(ctx, settingAccountLabel, label)
}
