package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

const loginPollEvery = time.Second

// loginState is a Google sign-in in progress: the link is shown and the daemon
// is polled until the user approves, the link expires or the attempt fails.
type loginState struct {
	id, url string
}

// startLogin asks the daemon for a sign-in link, tries to open the browser and
// moves to the Account screen where the link and the progress are shown.
func (m Model) startLogin() (tea.Model, tea.Cmd) {
	if m.acts.Login == nil {
		return m, nil
	}
	m.screen, m.cursor, m.top = screenAccount, 0, 0
	m.status = ""
	acts, ctx := m.acts, m.ctx
	return m, func() tea.Msg {
		sess, err := acts.Login(ctx)
		if err == nil && acts.OpenURL != nil {
			_ = acts.OpenURL(sess.URL)
		}
		return loginStartedMsg{sess: sess, err: err}
	}
}

func (m Model) onLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loginStartedMsg:
		if msg.err != nil {
			m.status = Sanitize(msg.err.Error())
			return m, nil
		}
		m.login = &loginState{id: msg.sess.ID, url: msg.sess.URL}
		return m, loginTick(msg.sess.ID)
	case loginPollMsg:
		if m.login == nil || m.login.id != msg.id || m.acts.LoginState == nil {
			return m, nil
		}
		acts, ctx, id := m.acts, m.ctx, msg.id
		return m, func() tea.Msg {
			state, err := acts.LoginState(ctx, id)
			return loginStateMsg{id: id, state: state, err: err}
		}
	case loginStateMsg:
		if m.login == nil || m.login.id != msg.id {
			return m, nil
		}
		switch {
		case msg.err != nil:
			m.login = nil
			m.status = Sanitize(msg.err.Error())
		case msg.state == "connected":
			m.login = nil
			m.status = m.t(i18n.LoginDone)
			return m, m.refetch()
		case msg.state == "failed":
			m.login = nil
			m.status = m.t(i18n.LoginFailed)
		case msg.state == "expired":
			m.login = nil
			m.status = m.t(i18n.LoginExpired)
		default:
			return m, loginTick(msg.id)
		}
	}
	return m, nil
}

func loginTick(id string) tea.Cmd {
	return tea.Tick(loginPollEvery, func(time.Time) tea.Msg { return loginPollMsg{id: id} })
}

// loginLines shows the link twice: as a clickable hyperlink (terminals that
// support it) and as plain wrapped text, so it can still be copied.
func (m Model) loginLines() []string {
	if m.login == nil {
		return nil
	}
	lines := []string{"", m.accent(m.t(i18n.LoginWaiting))}
	if m.opt.Color {
		lines = append(lines, ansi.SetHyperlink(m.login.url)+m.t(i18n.LoginOpen)+ansi.ResetHyperlink())
	}
	width := max(20, m.width-2)
	for _, chunk := range wrapRunes(m.login.url, width) {
		lines = append(lines, m.dim(chunk))
	}
	return append(lines, "", m.dim(m.t(i18n.LoginHide)))
}

func wrapRunes(s string, width int) []string {
	r := []rune(strings.TrimSpace(s))
	var out []string
	for len(r) > width {
		out = append(out, string(r[:width]))
		r = r[width:]
	}
	return append(out, string(r))
}
