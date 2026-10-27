package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/config"
	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/spf13/cobra"
)

const ExitSetupIncomplete = 9

func (a *App) setupChecklist(ctx context.Context) (app.Setup, error) {
	r, err := a.call(ctx, "GET", "/v1/setup", nil, nil, "")
	if err == nil {
		var out app.Setup
		err = r.Decode(&out)
		return out, err
	}
	// An older daemon already exposes the same checks through Doctor.
	if e, ok := err.(*APIError); !ok || e.Code != "not_found" {
		return app.Setup{}, err
	}
	r, err = a.call(ctx, "GET", "/v1/doctor", nil, nil, "")
	if err != nil {
		return app.Setup{}, err
	}
	var body struct {
		Checks []app.Check `json:"checks"`
	}
	if err = r.Decode(&body); err != nil {
		return app.Setup{}, err
	}
	out := app.Setup{Ready: true, Checks: []app.Check{}}
	for _, c := range body.Checks {
		switch c.Name {
		case "database", "oauth_client", "account", "destination":
			out.Checks = append(out.Checks, c)
			out.Ready = out.Ready && c.Status == "passed"
		}
	}
	return out, nil
}

func (a *App) onboardCmd() *cobra.Command {
	return &cobra.Command{Use: "onboard", Short: "Guided first copy setup; unattended checklist exits 9 when incomplete", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		interactive := a.io.StdinTTY && a.io.StdoutTTY && !a.g.noInput && !a.g.json
		lang := "en"
		if interactive {
			lang = i18n.Resolve(i18n.OSLocale())
		}
		if interactive {
			cfg, err := config.Load(a.configPath())
			if err != nil || cfg.OAuthClientFile == "" {
				fmt.Fprintln(a.errw(), present.SetupStep("oauth_client", lang))
				_ = openBrowser("https://console.cloud.google.com/apis/credentials")
				rd := bufio.NewReader(a.io.In)
				path := a.promptLine(rd, i18n.T(lang, i18n.SetupOAuthPrompt))
				st, err := os.Lstat(path)
				if err != nil || !st.Mode().IsRegular() || (runtime.GOOS != "windows" && st.Mode().Perm() != 0600) {
					return usage("OAuth client must be a regular file with permissions 0600")
				}
				if err = a.runSetup(&setupInput{oauthClientFile: path, noEnv: true}, false); err != nil {
					return err
				}
				a.client = nil
			}
			if _, err := a.ensureDaemon(ctx); err != nil {
				return err
			}
		}
		out, err := a.setupChecklist(ctx)
		if err != nil {
			// Without a daemon/config, readiness is unknown; never infer credentials.
			out = app.Setup{Checks: []app.Check{}}
			for _, name := range []string{"daemon", "oauth_client", "account", "destination"} {
				out.Checks = append(out.Checks, app.Check{Name: name, Status: "unknown", Message: present.SetupStep(name, "en")})
			}
		}
		if interactive && err == nil {
			for _, c := range out.Checks {
				if c.Status == "passed" {
					continue
				}
				fmt.Fprintln(a.errw(), present.SetupStep(c.Name, lang))
				switch c.Name {
				case "oauth_client":
					fmt.Fprintln(a.errw(), i18n.T(lang, i18n.SetupRestart))
				case "account":
					login, _, _ := a.accountCmd().Find([]string{"login"})
					login.SetContext(ctx)
					if e := login.RunE(login, nil); e != nil {
						return e
					}
				case "destination":
					path := a.promptLine(bufio.NewReader(a.io.In), i18n.T(lang, i18n.SetupDestinationPrompt))
					d, e := a.tuiAddDestination(ctx, path)
					if e != nil {
						return e
					}
					if e = a.tuiSetDefaultDestination(ctx, d.ID, d.Revision); e != nil {
						return e
					}
				}
			}
			if fresh, e := a.setupChecklist(ctx); e == nil {
				out = fresh
			}
		}
		// Checklist JSON is always machine-readable, including without --json.
		b, _ := json.Marshal(map[string]any{"data": out, "request_id": ""})
		fmt.Fprintln(a.out(), string(b))
		if !out.Ready {
			return localErr(ExitSetupIncomplete, "setup_incomplete", i18n.T(lang, i18n.SetupIncomplete))
		}
		return nil
	}}
}
