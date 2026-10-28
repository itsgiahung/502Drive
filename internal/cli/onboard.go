package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/config"
	"github.com/itsgiahung/502Drive/internal/i18n"
	"github.com/itsgiahung/502Drive/internal/present"
	"github.com/spf13/cobra"
)

const (
	ExitSetupIncomplete = 9
	// oauthPathAttempts bounds how often a bad file path may be pasted before
	// the guide gives up, so a typo never ends the whole command at once.
	oauthPathAttempts = 3
)

// openSetupPage opens a Google Cloud or consent page for the guide. It is a
// variable so tests exercise the walkthrough without launching a browser.
var openSetupPage = openBrowser

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
	var oauthClient string
	c := &cobra.Command{Use: "onboard", Short: "Guided first copy setup; unattended checklist exits 9 when incomplete", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		interactive := a.io.StdinTTY && a.io.StdoutTTY && !a.g.noInput && !a.g.json
		lang := "en"
		if interactive {
			lang = i18n.Resolve(i18n.OSLocale())
		}
		// One reader for every question: wrapping the input again would drop
		// lines already buffered while reading an earlier answer.
		rd := bufio.NewReader(a.io.In)
		if interactive {
			ready, err := a.onboardTakeOAuth(lang, rd, oauthClient)
			if err != nil {
				return err
			}
			if ready {
				if _, err := a.ensureDaemon(ctx); err != nil {
					return err
				}
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
					path := strings.TrimSpace(a.promptLine(rd, i18n.T(lang, i18n.SetupDestinationPrompt)))
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
	c.Flags().StringVar(&oauthClient, "oauth-client", "", "Google OAuth Desktop client JSON (skips the guide in a terminal)")
	return c
}

// onboardTakeOAuth resolves the OAuth client with as few questions as
// possible: an existing key asks nothing, a recent download asks one
// confirmation, otherwise one question decides between the path and the guide.
func (a *App) onboardTakeOAuth(lang string, rd *bufio.Reader, oauthClient string) (bool, error) {
	if oauthClient != "" {
		return true, a.installOAuthFile(oauthClient)
	}
	cfg, _ := config.Load(a.configPath())
	vals, _ := a.setupEnv("")
	if oauthConfigured(cfg, vals) {
		return true, nil
	}
	if p, ok := recentOAuthDownload(time.Now()); ok {
		if a.confirmDefaultYes(rd, i18n.Tf(lang, i18n.OnboardDownloadsUse, p)) {
			return true, a.installOAuthFile(p)
		}
	}
	return a.onboardAskForOAuth(lang, rd, oauthPathAttempts)
}

// oauthConfigured reports a key supplied by the config, the environment or a
// .env file. The file itself is checked when setup reads it.
func oauthConfigured(cfg config.Config, vals map[string]string) bool {
	if cfg.OAuthClientFile != "" {
		return true
	}
	if vals["oauth_client"] != "" {
		return true
	}
	return vals["google_id"] != "" && vals["google_secret"] != ""
}

// onboardAskForOAuth asks once for a path or the guide, then retries a bad
// file a bounded number of times instead of exiting on the first mistake.
func (a *App) onboardAskForOAuth(lang string, rd *bufio.Reader, tries int) (bool, error) {
	for tries > 0 {
		line := cleanPromptPath(a.promptLine(rd, i18n.T(lang, i18n.OnboardAskPath)))
		if line == "" || strings.EqualFold(line, "g") {
			path, quit, err := a.onboardGuide(lang, rd)
			if err != nil {
				return false, err
			}
			if quit {
				return false, nil
			}
			if path != "" {
				return true, a.installOAuthFile(path)
			}
			// The guide finished; ask for the file it told the user to download.
			line = cleanPromptPath(a.promptLine(rd, i18n.T(lang, i18n.OnboardPathOnly)))
			switch {
			case line == "" || strings.EqualFold(line, "q"):
				return false, nil
			case strings.EqualFold(line, "g"):
				continue
			}
		}
		err := a.installOAuthFile(line)
		if err == nil {
			return true, nil
		}
		var ae *APIError
		if errors.As(err, &ae) {
			// Writing the config failed: retrying the same input cannot help.
			return false, err
		}
		tries--
		if tries == 0 {
			return false, usage("%s", i18n.Tf(lang, i18n.OnboardPathFailed, err))
		}
		fmt.Fprintln(a.errw(), i18n.Tf(lang, i18n.OnboardPathRetry, err, tries))
	}
	return false, nil
}

// onboardGuide walks the five Google Cloud steps, opening each page and
// waiting for the user. It returns a validated path pasted at any step, or
// quit when the user leaves the guide.
func (a *App) onboardGuide(lang string, rd *bufio.Reader) (string, bool, error) {
	fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnboardGuideIntro))
	steps := []struct {
		text i18n.Key
		url  string
	}{
		{i18n.OnboardGuideProject, "https://console.cloud.google.com/projectcreate"},
		{i18n.OnboardGuideDriveAPI, "https://console.cloud.google.com/apis/library/drive.googleapis.com"},
		{i18n.OnboardGuideConsent, "https://console.cloud.google.com/auth/branding"},
		{i18n.OnboardGuideClient, "https://console.cloud.google.com/auth/clients/create"},
		{i18n.OnboardGuideDownload, ""},
	}
	bad := 0
	for i := 0; i < len(steps); i++ {
		fmt.Fprintln(a.errw(), i18n.T(lang, steps[i].text))
		if steps[i].url != "" {
			// A missing browser is not fatal: the page is named above and the
			// user can open it by hand.
			_ = openSetupPage(steps[i].url)
		}
		line := cleanPromptPath(a.promptLine(rd, i18n.T(lang, i18n.OnboardGuideKeys)))
		switch {
		case line == "" || strings.EqualFold(line, "s"):
			continue
		case strings.EqualFold(line, "q"):
			fmt.Fprintln(a.errw(), i18n.T(lang, i18n.OnboardGuideQuit))
			return "", true, nil
		}
		if err := validateOAuthFile(line); err != nil {
			bad++
			if bad >= oauthPathAttempts {
				return "", false, usage("%s", i18n.Tf(lang, i18n.OnboardPathFailed, err))
			}
			fmt.Fprintln(a.errw(), i18n.Tf(lang, i18n.OnboardPathRetry, err, oauthPathAttempts-bad))
			i-- // stay on this step and ask for a file again
			continue
		}
		return line, false, nil
	}
	return "", false, nil
}

// installOAuthFile validates the JSON and hands it to setup, which copies it
// into the private secrets folder. Validation errors are plain errors so the
// caller can offer another try; setup errors are API errors and stop here.
func (a *App) installOAuthFile(path string) error {
	path = cleanPromptPath(path)
	if err := validateOAuthFile(path); err != nil {
		return err
	}
	if err := a.runSetup(&setupInput{oauthClientFile: path, noEnv: true}, false); err != nil {
		return err
	}
	a.client = nil
	return nil
}

func validateOAuthFile(path string) error {
	path = cleanPromptPath(path)
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("the OAuth client must be a regular file: %s", path)
	}
	raw, err := readSmallFile(path)
	if err != nil {
		return fmt.Errorf("cannot read the OAuth client file: %v", err)
	}
	if err = validateOAuthClient(raw); err != nil {
		return fmt.Errorf("the OAuth client file is not a Google Desktop client JSON: %v", err)
	}
	return nil
}

// recentOAuthDownload finds the newest valid Desktop client JSON downloaded in
// the last day, so a person who just followed the guide answers one question.
func recentOAuthDownload(now time.Time) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	matches, err := filepath.Glob(filepath.Join(home, "Downloads", "client_secret*.json"))
	if err != nil {
		return "", false
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var list []candidate
	for _, p := range matches {
		st, err := os.Lstat(p)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		age := now.Sub(st.ModTime())
		if age < 0 {
			age = -age
		}
		if age > 24*time.Hour || validateOAuthFile(p) != nil {
			continue
		}
		list = append(list, candidate{p, st.ModTime()})
	}
	if len(list) == 0 {
		return "", false
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
	return list[0].path, true
}

// confirmDefaultYes reads a yes/no answer; Enter means yes.
func (a *App) confirmDefaultYes(rd *bufio.Reader, label string) bool {
	switch strings.ToLower(a.promptLine(rd, label)) {
	case "", "y", "yes":
		return true
	}
	return false
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
