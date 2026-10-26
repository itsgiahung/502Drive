package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/itsgiahung/502Drive/internal/config"
)

// setupInput is what `setup` (and the first-run wizard) gathers. Secrets are
// held as bytes and only ever written to private files, never printed.
type setupInput struct {
	oauthClientFile   string
	telegramTokenFile string
	envFile           string
	noEnv             bool
	owner             int64
	ownerSet          bool
	port              int
	portSet           bool
	container         bool

	oauthJSON []byte
	tgToken   string
	sources   []string // human-readable origins, never values
}

const maxSecretInput = 64 << 10

// containerListen is the only listener a container config may bind; the host
// publishes it on loopback only.
const containerListen = "0.0.0.0:5020"

var telegramTokenRE = regexp.MustCompile(`^[0-9]{3,}:[A-Za-z0-9_-]{10,}$`)

// envNames lists, per setting, the variable names setup understands. The last
// group is what the earlier gdclone .env used, so an existing file just works.
var envNames = map[string][]string{
	"telegram_token":  {"TELEGRAM_BOT_TOKEN", "DRIVE502_TELEGRAM_TOKEN", "GDCLONE__TELEGRAM__BOT_TOKEN"},
	"telegram_owner":  {"TELEGRAM_OWNER_ID", "DRIVE502_TELEGRAM_OWNER_ID", "GDCLONE__TELEGRAM__OWNER_TELEGRAM_ID"},
	"google_id":       {"GOOGLE_OAUTH_CLIENT_ID", "DRIVE502_GOOGLE_CLIENT_ID", "GDCLONE__GOOGLE_OAUTH__CLIENT_ID"},
	"google_secret":   {"GOOGLE_OAUTH_CLIENT_SECRET", "DRIVE502_GOOGLE_CLIENT_SECRET", "GDCLONE__GOOGLE_OAUTH__CLIENT_SECRET"},
	"oauth_client":    {"DRIVE502_OAUTH_CLIENT_FILE"},
	"telegram_tokenf": {"DRIVE502_TELEGRAM_TOKEN_FILE"},
}

func (a *App) setupCmd() *cobra.Command {
	var in setupInput
	c := &cobra.Command{Use: "setup", Short: "Create the private config and API token (and take Google/Telegram keys)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			in.ownerSet = cmd.Flags().Changed("telegram-owner-id")
			in.portSet = cmd.Flags().Changed("oauth-callback-port")
			explicit := in.oauthClientFile != "" || in.telegramTokenFile != "" || in.ownerSet || in.portSet || in.envFile != "" || in.container
			return a.runSetup(&in, a.interactive() && !explicit)
		}}
	f := c.Flags()
	f.StringVar(&in.oauthClientFile, "oauth-client", "", "Google OAuth Desktop client JSON (copied into the private secrets folder)")
	f.StringVar(&in.telegramTokenFile, "telegram-token-file", "", "file holding the Telegram bot token (copied into the private secrets folder)")
	f.Int64Var(&in.owner, "telegram-owner-id", 0, "Telegram numeric user id of the owner")
	f.IntVar(&in.port, "oauth-callback-port", 0, "fixed loopback port for the OAuth callback (0 = any free port)")
	f.StringVar(&in.envFile, "env-file", "", "read keys from this .env file (default: ./.env and <config dir>/.env)")
	f.BoolVar(&in.noEnv, "no-env", false, "ignore environment variables and .env files")
	f.BoolVar(&in.container, "container", false, "bind the container's interfaces (listen 0.0.0.0:5020); publish loopback ports only")
	return c
}

// runSetup validates everything first, so a bad value never leaves a
// half-configured setup behind, then writes the config and the secrets.
func (a *App) runSetup(in *setupInput, wizard bool) error {
	if in.ownerSet && in.owner <= 0 {
		return usage("--telegram-owner-id must be a positive numeric Telegram user id")
	}
	if in.portSet && (in.port < 0 || in.port > 65535) {
		return usage("--oauth-callback-port must be between 0 and 65535 (0 uses a free port)")
	}
	if err := a.collectSetup(in); err != nil {
		return err
	}
	if wizard {
		if err := a.wizard(in); err != nil {
			return err
		}
	}

	path := a.configPath()
	cfg, err := config.Load(path)
	created := false
	if err != nil {
		if _, statErr := os.Lstat(path); statErr == nil {
			return localErr(ExitUsage, "config_invalid", err.Error())
		}
		if cfg, err = config.Setup(path); err != nil {
			return localErr(ExitUsage, "setup_failed", err.Error())
		}
		created = true
	}
	changed, err := a.installSetup(&cfg, in)
	if err != nil {
		return err
	}
	if in.container && (!cfg.Container || cfg.Listen != containerListen) {
		cfg.Container, cfg.Listen = true, containerListen
		changed = true
	}
	if changed {
		if err = config.Save(path, cfg); err != nil {
			return localErr(ExitInternal, "setup_failed", err.Error())
		}
	}
	a.cfgSet = false
	a.printSetup(path, cfg, created, in)
	return nil
}

// collectSetup reads the files named by flags, then fills what is still empty
// from the environment and .env files. Nothing is written here.
func (a *App) collectSetup(in *setupInput) error {
	if in.oauthClientFile != "" {
		raw, err := readSmallFile(in.oauthClientFile)
		if err != nil {
			return usage("cannot read the OAuth client file: %v", err)
		}
		if err = validateOAuthClient(raw); err != nil {
			return usage("the OAuth client file is not a Google Desktop client JSON: %v", err)
		}
		in.oauthJSON = raw
		in.sources = append(in.sources, "Google client: --oauth-client")
	}
	if in.telegramTokenFile != "" {
		raw, err := readSmallFile(in.telegramTokenFile)
		if err != nil {
			return usage("cannot read the Telegram token file: %v", err)
		}
		tok := strings.TrimSpace(string(raw))
		if !telegramTokenRE.MatchString(tok) {
			return usage("the Telegram token file does not contain a bot token (expected 123456:ABC…)")
		}
		in.tgToken = tok
		in.sources = append(in.sources, "Telegram token: --telegram-token-file")
	}
	if in.noEnv {
		return nil
	}
	vals, origin := a.setupEnv(in.envFile)
	if in.envFile != "" && origin == "" {
		return usage("cannot read the .env file %s", in.envFile)
	}
	if in.oauthJSON == nil {
		if err := a.oauthFromEnv(in, vals, origin); err != nil {
			return err
		}
	}
	if in.tgToken == "" {
		if tok := vals["telegram_token"]; tok != "" {
			if !telegramTokenRE.MatchString(tok) {
				return usage("the Telegram bot token in %s is not valid (expected 123456:ABC…)", origin)
			}
			in.tgToken = tok
			in.sources = append(in.sources, "Telegram token: "+origin)
		}
	}
	if !in.ownerSet {
		if v := vals["telegram_owner"]; v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				return usage("the Telegram owner id in %s must be a positive number", origin)
			}
			in.owner, in.ownerSet = id, true
			in.sources = append(in.sources, "Telegram owner id: "+origin)
		}
	}
	return nil
}

func (a *App) oauthFromEnv(in *setupInput, vals map[string]string, origin string) error {
	if path := vals["oauth_client"]; path != "" {
		raw, err := readSmallFile(path)
		if err != nil || validateOAuthClient(raw) != nil {
			return usage("the OAuth client file named in %s is missing or not a Desktop client JSON", origin)
		}
		in.oauthJSON = raw
		in.sources = append(in.sources, "Google client: "+origin)
		return nil
	}
	id, secret := vals["google_id"], vals["google_secret"]
	if id == "" && secret == "" {
		return nil
	}
	if id == "" || secret == "" {
		return usage("%s has only one of the Google client id and secret; set both", origin)
	}
	raw, err := buildOAuthClient(id, secret)
	if err != nil {
		return usage("%v", err)
	}
	in.oauthJSON = raw
	in.sources = append(in.sources, "Google client id and secret: "+origin)
	return nil
}

// setupEnv merges the process environment with the first .env source found
// and returns the values per setting and a label for where the .env came from.
func (a *App) setupEnv(envFile string) (map[string]string, string) {
	file := map[string]string{}
	origin := ""
	candidates := []string{envFile}
	if envFile == "" {
		if wd, err := os.Getwd(); err == nil {
			candidates = []string{filepath.Join(wd, ".env"), filepath.Join(a.stateDir(), ".env")}
		}
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		raw, err := readSmallFile(p)
		if err != nil {
			continue
		}
		file, origin = parseDotEnv(bytes.NewReader(raw)), p
		break
	}
	out := map[string]string{}
	for key, names := range envNames {
		for _, n := range names {
			if v := strings.TrimSpace(a.env(n)); v != "" {
				out[key] = v
				break
			}
		}
		if out[key] != "" {
			continue
		}
		for _, n := range names {
			if v := strings.TrimSpace(file[n]); v != "" {
				out[key] = v
				break
			}
		}
	}
	if origin == "" && len(out) > 0 {
		origin = "the environment"
	} else if origin != "" {
		origin = ".env (" + origin + ")"
	}
	return out, origin
}

// parseDotEnv reads KEY=VALUE lines: comments, an optional "export ", and
// single or double quotes are handled; nothing is expanded or executed.
func parseDotEnv(r io.Reader) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxSecretInput)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}

func readSmallFile(path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if st.Size() > maxSecretInput {
		return nil, errors.New("file is too large")
	}
	return os.ReadFile(abs)
}

// validateOAuthClient accepts the JSON Google offers for a Desktop client.
func validateOAuthClient(raw []byte) error {
	var doc map[string]struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return errors.New("invalid JSON")
	}
	c, ok := doc["installed"]
	if !ok {
		if _, web := doc["web"]; web {
			return errors.New("this is a Web client; create a Desktop app client instead")
		}
		return errors.New(`no "installed" section`)
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return errors.New("client_id or client_secret is empty")
	}
	return nil
}

// buildOAuthClient makes the Desktop client JSON from an id and a secret.
func buildOAuthClient(id, secret string) ([]byte, error) {
	if !strings.HasSuffix(id, ".apps.googleusercontent.com") {
		return nil, errors.New("the Google client id should end with .apps.googleusercontent.com")
	}
	doc := map[string]any{"installed": map[string]any{
		"client_id": id, "client_secret": secret,
		"auth_uri": "https://accounts.google.com/o/oauth2/auth", "token_uri": "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"redirect_uris":               []string{"http://localhost"},
	}}
	return json.Marshal(doc)
}

// installSetup writes the secrets into <config dir>/secrets with private
// permissions and points the config at them.
func (a *App) installSetup(cfg *config.Config, in *setupInput) (bool, error) {
	changed := false
	if in.oauthJSON != nil || in.tgToken != "" {
		dir := filepath.Join(a.stateDir(), "secrets")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return false, localErr(ExitInternal, "setup_failed", err.Error())
		}
		if in.oauthJSON != nil {
			p := filepath.Join(dir, "oauth-client.json")
			if err := writePrivate(p, in.oauthJSON); err != nil {
				return false, localErr(ExitInternal, "setup_failed", err.Error())
			}
			cfg.OAuthClientFile, changed = p, true
		}
		if in.tgToken != "" {
			p := filepath.Join(dir, "telegram-token")
			if err := writePrivate(p, []byte(in.tgToken+"\n")); err != nil {
				return false, localErr(ExitInternal, "setup_failed", err.Error())
			}
			cfg.TelegramTokenFile, changed = p, true
		}
	}
	if in.ownerSet {
		cfg.TelegramOwnerID, changed = in.owner, true
	}
	if in.portSet {
		cfg.OAuthCallbackPort, changed = in.port, true
	}
	return changed, nil
}

// writePrivate replaces a file atomically with 0600 permissions.
func writePrivate(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".secret-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
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
	return os.Rename(tmp.Name(), path)
}

func (a *App) printSetup(path string, cfg config.Config, created bool, in *setupInput) {
	if a.g.quiet && !a.g.json {
		return
	}
	if a.g.json {
		b, _ := json.Marshal(map[string]any{"data": map[string]any{"config": path, "created": created, "data_dir": cfg.DataDir,
			"token_file": cfg.TokenFile, "oauth_client_configured": cfg.OAuthClientFile != "",
			"telegram_configured": cfg.TelegramTokenFile != "", "telegram_owner_id": cfg.TelegramOwnerID,
			"oauth_callback_port": cfg.OAuthCallbackPort, "listen": cfg.Listen, "container": cfg.Container}, "request_id": ""})
		fmt.Fprintln(a.out(), string(b))
		return
	}
	verb := "Using existing"
	if created {
		verb = "Created"
	}
	fmt.Fprintf(a.out(), "%s config  %s\n", verb, path)
	fmt.Fprintf(a.out(), "API token       %s (private; never printed)\n", cfg.TokenFile)
	fmt.Fprintf(a.out(), "Data directory  %s\n", cfg.DataDir)
	for _, s := range in.sources {
		fmt.Fprintf(a.out(), "Read            %s (value never shown)\n", s)
	}
	if cfg.OAuthClientFile != "" {
		fmt.Fprintln(a.out(), "Google          OAuth client stored privately; next: 502drive account login (or `l` in the TUI)")
	}
	if cfg.TelegramTokenFile != "" {
		fmt.Fprintf(a.out(), "Telegram        token stored privately, owner id %d\n", cfg.TelegramOwnerID)
	}
	if cfg.OAuthCallbackPort > 0 {
		fmt.Fprintf(a.out(), "OAuth callback  fixed loopback port %d\n", cfg.OAuthCallbackPort)
	}
	if cfg.Container {
		fmt.Fprintf(a.out(), "Container       listens on %s; publish only 127.0.0.1 ports from the host\n", cfg.Listen)
	}
	if cfg.OAuthClientFile == "" {
		fmt.Fprintln(a.out(), "\nGoogle is not configured yet: run `502drive setup` again in a terminal, put")
		fmt.Fprintln(a.out(), "GOOGLE_OAUTH_CLIENT_ID / GOOGLE_OAUTH_CLIENT_SECRET in a .env file, or pass")
		fmt.Fprintln(a.out(), "--oauth-client client_secret.json (see docs/oauth-setup.md).")
	}
	fmt.Fprintln(a.out(), "\nStart everything with:  502drive   (or just: 502d)")
}

// ---- interactive wizard ----

type wizText struct{ intro, google, tgToken, tgOwner, skip, bad string }

func wizardText() wizText {
	lang := strings.ToLower(os.Getenv("LANG") + os.Getenv("LC_ALL") + os.Getenv("LANGUAGE"))
	if strings.Contains(lang, "vi") {
		return wizText{
			intro:   "Thiết lập nhanh (Enter để bỏ qua bất kỳ mục nào, chạy lại `502drive setup` sau cũng được).",
			google:  "Đường dẫn file OAuth client JSON của Google (kéo-thả file vào đây), hoặc Enter để bỏ qua",
			tgToken: "Token bot Telegram (ẩn khi gõ), hoặc Enter để bỏ qua",
			tgOwner: "Telegram user id của bạn (số, lấy từ @userinfobot)",
			skip:    "bỏ qua",
			bad:     "Giá trị không hợp lệ, bỏ qua mục này.",
		}
	}
	return wizText{
		intro:   "Quick setup (press Enter to skip anything; you can run `502drive setup` again later).",
		google:  "Path to the Google OAuth client JSON (drag the file here), or Enter to skip",
		tgToken: "Telegram bot token (hidden while typing), or Enter to skip",
		tgOwner: "Your Telegram numeric user id (get it from @userinfobot)",
		skip:    "skipped",
		bad:     "That value is not valid; skipping it.",
	}
}

// wizard asks only for what is still missing after flags, env and .env.
func (a *App) wizard(in *setupInput) error {
	if in.oauthJSON != nil && (in.tgToken != "" && in.ownerSet) {
		return nil
	}
	t := wizardText()
	fmt.Fprintln(a.errw(), t.intro)
	rd := bufio.NewReader(a.io.In)
	if in.oauthJSON == nil {
		if v := a.promptLine(rd, t.google); v != "" {
			raw, err := readSmallFile(strings.Trim(v, `"' `))
			if err != nil || validateOAuthClient(raw) != nil {
				fmt.Fprintln(a.errw(), t.bad)
			} else {
				in.oauthJSON = raw
				in.sources = append(in.sources, "Google client: the file you gave")
			}
		}
	}
	if in.tgToken == "" {
		if v := a.promptSecret(rd, t.tgToken); v != "" {
			if !telegramTokenRE.MatchString(v) {
				fmt.Fprintln(a.errw(), t.bad)
			} else {
				in.tgToken = v
				in.sources = append(in.sources, "Telegram token: typed")
			}
		}
	}
	if in.tgToken != "" && !in.ownerSet {
		if v := a.promptLine(rd, t.tgOwner); v != "" {
			if id, err := strconv.ParseInt(v, 10, 64); err != nil || id <= 0 {
				fmt.Fprintln(a.errw(), t.bad)
			} else {
				in.owner, in.ownerSet = id, true
			}
		}
	}
	return nil
}

func (a *App) promptLine(rd *bufio.Reader, label string) string {
	fmt.Fprintf(a.errw(), "%s: ", label)
	line, _ := rd.ReadString('\n')
	return strings.TrimSpace(line)
}

// promptSecret hides what is typed when the input is a real terminal.
func (a *App) promptSecret(rd *bufio.Reader, label string) string {
	if f, ok := a.io.In.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprintf(a.errw(), "%s: ", label)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(a.errw())
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	return a.promptLine(rd, label)
}
