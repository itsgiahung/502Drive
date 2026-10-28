package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/i18n"
)

// onboardTelegramWait is how long the guide waits for the owner's first
// message before asking for the id by hand. It is a variable so tests do not
// wait minutes.
var onboardTelegramWait = 3 * time.Minute

// telegramTokenAttempts bounds how often a wrong token may be pasted.
const telegramTokenAttempts = 3

type tgIdentity struct {
	ID   int64
	Name string
}

func (a *App) telegramAPI() string {
	if v := a.env("DRIVE502_TELEGRAM_API"); v != "" { // tests only
		return strings.TrimRight(v, "/")
	}
	return "https://api.telegram.org"
}

// tgCall calls one Bot API method and decodes the "result" field.
func (a *App) tgCall(ctx context.Context, token, method string, q url.Values, out any) error {
	u := a.telegramAPI() + "/bot" + token + "/" + method
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 40 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err = json.Unmarshal(body, &env); err != nil || !env.OK {
		if env.Description != "" {
			return fmt.Errorf("telegram: %s", env.Description)
		}
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

// tgBotName asks Telegram who the token belongs to.
func (a *App) tgBotName(ctx context.Context, token string) (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	if err := a.tgCall(ctx, token, "getMe", nil, &me); err != nil {
		return "", err
	}
	return me.Username, nil
}

// tgWaitForOwner waits for a private message to the bot and returns its
// sender. The update is left unacknowledged on purpose: the daemon's bot
// answers it when it starts, so the owner gets the menu straight away.
func (a *App) tgWaitForOwner(ctx context.Context, token string, wait time.Duration) (tgIdentity, bool) {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		secs := int(math.Ceil(min(time.Until(deadline).Seconds(), 25)))
		var updates []struct {
			Message struct {
				Chat struct {
					Type string `json:"type"`
				} `json:"chat"`
				From struct {
					ID        int64  `json:"id"`
					IsBot     bool   `json:"is_bot"`
					FirstName string `json:"first_name"`
					Username  string `json:"username"`
				} `json:"from"`
			} `json:"message"`
		}
		err := a.tgCall(ctx, token, "getUpdates", url.Values{"timeout": {strconv.Itoa(secs)}, "allowed_updates": {`["message"]`}}, &updates)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		for _, u := range updates {
			f := u.Message.From
			if u.Message.Chat.Type != "private" || f.IsBot || f.ID <= 0 {
				continue
			}
			name := f.FirstName
			if name == "" {
				name = f.Username
			}
			return tgIdentity{ID: f.ID, Name: Sanitize(name)}, true
		}
	}
	return tgIdentity{}, false
}

// onboardTelegram walks a person from "no bot" to a saved token and owner id.
func (a *App) onboardTelegram(ctx context.Context, lang string, rd *bufio.Reader) error {
	if cfg, err := a.loadConfig(); err == nil && cfg.TelegramTokenFile != "" && cfg.TelegramOwnerID > 0 {
		fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgAlready))
		return nil
	}
	fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgIntro))
	fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgStep1))
	_ = openSetupPage("https://t.me/BotFather")

	var token, bot string
	for try := 0; try < telegramTokenAttempts && token == ""; try++ {
		in := strings.TrimSpace(a.promptSecret(rd, i18n.T(lang, i18n.OnbTgToken)))
		if in == "" {
			fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgCancelled))
			return nil
		}
		if !telegramTokenRE.MatchString(in) {
			fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgTokenBad))
			continue
		}
		name, err := a.tgBotName(ctx, in)
		if err != nil {
			fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgTokenFail))
			continue
		}
		token, bot = in, name
	}
	if token == "" {
		fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgCancelled))
		return nil
	}
	fmt.Fprintf(a.errw(), i18n.T(lang, i18n.OnbTgBot)+"\n", Sanitize(bot))
	fmt.Fprintf(a.errw(), i18n.T(lang, i18n.OnbTgWaiting)+"\n", Sanitize(bot))
	_ = openSetupPage("https://t.me/" + url.PathEscape(bot))

	var owner int64
	if who, ok := a.tgWaitForOwner(ctx, token, onboardTelegramWait); ok {
		if a.confirmDefaultYes(rd, fmt.Sprintf(i18n.T(lang, i18n.OnbTgGot), who.Name, who.ID)) {
			owner = who.ID
		}
	}
	for try := 0; owner == 0 && try < telegramTokenAttempts; try++ {
		in := a.promptLine(rd, i18n.T(lang, i18n.OnbTgManual))
		if in == "" {
			fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgCancelled))
			return nil
		}
		id, err := strconv.ParseInt(in, 10, 64)
		if err != nil || id <= 0 {
			fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgIDBad))
			continue
		}
		owner = id
	}
	if owner == 0 {
		fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgCancelled))
		return nil
	}
	if err := a.runSetup(&setupInput{tgToken: token, owner: owner, ownerSet: true, noEnv: true}, false); err != nil {
		return err
	}
	a.client, a.cfgSet = nil, false
	fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgSaved))
	if a.daemonUp(ctx) {
		fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgRestart))
	} else {
		fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnbTgStart))
	}
	return nil
}

func (a *App) onboardTelegramFlag(cmd *cobra.Command) error {
	if !(a.io.StdinTTY && a.io.StdoutTTY) || a.g.noInput || a.g.json {
		return localErr(ExitUsage, "needs_terminal", i18n.T("en", i18n.OnbTgNeedsTTY))
	}
	lang := i18n.Resolve(i18n.OSLocale())
	return a.onboardTelegram(cmd.Context(), lang, bufio.NewReader(a.io.In))
}
