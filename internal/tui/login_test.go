package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type loginFake struct {
	fakeActions
	opened []string
	states []string
	err    error
}

func (l *loginFake) acts() Actions {
	a := l.fakeActions.Actions()
	a.Login = func(context.Context) (LoginSession, error) {
		if l.err != nil {
			return LoginSession{}, l.err
		}
		return LoginSession{ID: "sess_1", URL: "https://accounts.google.com/o/oauth2/auth?client_id=x&redirect_uri=http%3A%2F%2F127.0.0.1%3A50200%2Foauth%2Fcallback"}, nil
	}
	a.LoginState = func(context.Context, string) (string, error) {
		s := l.states[0]
		if len(l.states) > 1 {
			l.states = l.states[1:]
		}
		return s, nil
	}
	a.OpenURL = func(u string) error { l.opened = append(l.opened, u); return nil }
	return a
}

func disconnectedModel(t *testing.T, l *loginFake, opt Options) Model {
	t.Helper()
	l.snap = sampleSnapshot()
	l.snap.Status.AccountStatus = "disconnected"
	l.snap.Status.Running, l.snap.Status.Queued = 0, 0
	l.snap.Jobs = nil
	opt.Width, opt.Language = 80, "en"
	m := New(l.acts(), opt)
	return run(t, m, m.fetch())
}

func TestLoginKeyOpensTheBrowserShowsTheLinkAndConnects(t *testing.T) {
	l := &loginFake{states: []string{"pending", "pending", "connected"}}
	m := disconnectedModel(t, l, Options{Color: true})
	if v := m.View(); !strings.Contains(v, "press l to sign in") {
		t.Fatalf("home gives no hint that Google is disconnected:\n%s", v)
	}
	m, cmd := update(t, m, key("l"))
	if m.screen != screenAccount {
		t.Fatalf("screen = %v, want account", m.screen)
	}
	m = run(t, m, cmd)
	if len(l.opened) != 1 || !strings.HasPrefix(l.opened[0], "https://accounts.google.com/") {
		t.Fatalf("the browser was not opened with the sign-in link: %v", l.opened)
	}
	if m.login == nil {
		t.Fatal("no sign-in in progress after the link was issued")
	}
	view := m.View()
	for _, want := range []string{"Waiting for you to approve", "client_id=x", "\x1b]8;;https://accounts.google.com"} {
		if !strings.Contains(view, want) {
			t.Fatalf("account view lacks %q:\n%s", want, view)
		}
	}
	// pending, pending, connected
	for i := 0; i < 3; i++ {
		_, c := update(t, m, loginPollMsg{id: "sess_1"})
		m = run(t, m, c)
	}
	if m.login != nil {
		t.Fatal("the sign-in should end once the daemon says connected")
	}
	if m.status != "Google is connected." {
		t.Fatalf("status = %q", m.status)
	}
}

func TestLoginFailureAndExpiryAreExplained(t *testing.T) {
	for state, want := range map[string]string{"failed": "did not complete", "expired": "expired"} {
		l := &loginFake{states: []string{state}}
		m := disconnectedModel(t, l, Options{})
		m, cmd := update(t, m, key("l"))
		m = run(t, m, cmd)
		_, c := update(t, m, loginPollMsg{id: "sess_1"})
		m = run(t, m, c)
		if m.login != nil || !strings.Contains(m.status, want) {
			t.Fatalf("%s: login=%v status=%q", state, m.login, m.status)
		}
	}
}

func TestLoginErrorFromTheDaemonIsShownAndNothingStarts(t *testing.T) {
	l := &loginFake{err: errors.New("the daemon has no Google OAuth client configured; run 502drive setup")}
	m := disconnectedModel(t, l, Options{})
	m, cmd := update(t, m, key("l"))
	m = run(t, m, cmd)
	if m.login != nil || !strings.Contains(m.status, "502drive setup") {
		t.Fatalf("login=%v status=%q", m.login, m.status)
	}
}

func TestEscHidesTheLinkAndLateAnswersAreIgnored(t *testing.T) {
	l := &loginFake{states: []string{"connected"}}
	m := disconnectedModel(t, l, Options{})
	m, cmd := update(t, m, key("l"))
	m = run(t, m, cmd)
	m, _ = update(t, m, key("esc"))
	if m.login != nil || m.screen != screenAccount {
		t.Fatalf("esc should only hide the link: login=%v screen=%v", m.login, m.screen)
	}
	m, _ = update(t, m, loginStateMsg{id: "sess_1", state: "connected"})
	if m.status == "Google is connected." {
		t.Fatal("an answer for a hidden sign-in changed the status")
	}
}

func TestLongLinksWrapWithinTheTerminalWidth(t *testing.T) {
	for _, line := range wrapRunes(strings.Repeat("a", 250), 78) {
		if len([]rune(line)) > 78 {
			t.Fatalf("chunk of %d runes", len([]rune(line)))
		}
	}
	if got := strings.Join(wrapRunes(strings.Repeat("ab", 90), 40), ""); got != strings.Repeat("ab", 90) {
		t.Fatal("wrapping lost characters, so the link could not be copied back together")
	}
}
