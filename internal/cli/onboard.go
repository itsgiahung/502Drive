package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
				path := cleanPromptPath(a.promptLine(rd, i18n.T(lang, i18n.SetupOAuthPrompt)))
				// The file is validated and copied into the private secrets
				// folder by setup, so a freshly downloaded 0644 file is fine.
				st, err := os.Lstat(path)
				if err != nil || !st.Mode().IsRegular() {
					return usage("the OAuth client must be a regular file: %s", path)
				}
				raw, err := readSmallFile(path)
				if err != nil {
					return usage("cannot read the OAuth client file: %v", err)
				}
				if err = validateOAuthClient(raw); err != nil {
					return usage("the OAuth client file is not a Google Desktop client JSON: %v", err)
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
					path := strings.TrimSpace(a.promptLine(bufio.NewReader(a.io.In), i18n.T(lang, i18n.SetupDestinationPrompt)))
					if path == "" { // Enter: copies go to My Drive
						root, e := a.myDriveID(ctx)
						if e != nil {
							return e
						}
						path = root
					}
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
		if interactive {
			// A person gets a readable checklist; scripts and --json get JSON.
			for _, c := range out.Checks {
				mark := "✓"
				if c.Status != "passed" {
					mark = "✗"
				}
				fmt.Fprintf(a.out(), "%s %s\n", mark, present.SetupStep(c.Name, lang))
			}
			if out.Ready {
				fmt.Fprintln(a.out(), i18n.T(lang, i18n.SetupReady))
			}
		} else {
			b, _ := json.Marshal(map[string]any{"data": out, "request_id": ""})
			fmt.Fprintln(a.out(), string(b))
		}
		if !out.Ready {
			return localErr(ExitSetupIncomplete, "setup_incomplete", i18n.T(lang, i18n.SetupIncomplete))
		}
		return nil
	}}
}

// cleanPromptPath turns a typed or drag-and-dropped path into a plain one:
// surrounding whitespace and quotes are dropped and a leading ~ is expanded.
func cleanPromptPath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && (p[0] == '\'' || p[0] == '"') && p[len(p)-1] == p[0] {
		p = p[1 : len(p)-1]
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	return p
}

// myDriveID is the real ID of the user's My Drive root ("root" is only an
// alias that the destination check does not take).
func (a *App) myDriveID(ctx context.Context) (string, error) {
	r, err := a.call(ctx, "GET", "/v1/drive/folders/root", nil, nil, "")
	if err != nil {
		return "", err
	}
	var page struct {
		Folder struct {
			ID string `json:"id"`
		} `json:"folder"`
	}
	if err = r.Decode(&page); err != nil {
		return "", err
	}
	if page.Folder.ID == "" {
		return "", localErr(ExitInternal, "my_drive_unknown", "Could not find My Drive; paste a folder link instead.")
	}
	return page.Folder.ID, nil
}
