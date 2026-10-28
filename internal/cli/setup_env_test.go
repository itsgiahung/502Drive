package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/config"
)

const (
	synthToken  = "123456:synthetic-token-abcdef"
	synthID     = "1234-synthetic.apps.googleusercontent.com"
	synthSecret = "synthetic-client-secret"
)

func setupArgs(t *testing.T, env func(string) string, args ...string) (int, string, string, string) {
	t.Helper()
	home := privateDir(t)
	cfgPath := filepath.Join(home, "config.json")
	var out, errb strings.Builder
	code := Execute(t.Context(), append([]string{"--config", cfgPath}, args...),
		IO{In: strings.NewReader(""), Out: &out, Err: &errb, Getenv: env})
	return code, out.String(), errb.String(), cfgPath
}

func loadCfg(t *testing.T, path string) config.Config {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeEnvFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil { // .env files are rarely 0600
		t.Fatal(err)
	}
	return p
}

func TestSetupTakesKeysFromADotEnvAndNeverPrintsThem(t *testing.T) {
	envFile := writeEnvFile(t, "# keys\nexport GDCLONE__TELEGRAM__BOT_TOKEN='"+synthToken+"'\n"+
		"GDCLONE__TELEGRAM__OWNER_TELEGRAM_ID=777\n"+
		"GDCLONE__GOOGLE_OAUTH__CLIENT_ID=\""+synthID+"\"\n"+
		"GDCLONE__GOOGLE_OAUTH__CLIENT_SECRET="+synthSecret+" # comment\n")
	code, out, errb, cfgPath := setupArgs(t, nil, "setup", "--env-file", envFile)
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb)
	}
	for _, secret := range []string{synthToken, synthSecret, synthID} {
		if strings.Contains(out+errb, secret) {
			t.Fatalf("a secret value was printed: %q", out+errb)
		}
	}
	cfg := loadCfg(t, cfgPath)
	if cfg.TelegramOwnerID != 777 {
		t.Fatalf("owner = %d", cfg.TelegramOwnerID)
	}
	tok, err := config.ReadPrivate(cfg.TelegramTokenFile) // fails unless the file is private
	if err != nil || strings.TrimSpace(string(tok)) != synthToken {
		t.Fatalf("telegram token file: %q, %v", tok, err)
	}
	raw, err := config.ReadPrivate(cfg.OAuthClientFile)
	if err != nil {
		t.Fatalf("oauth client file: %v", err)
	}
	if err = validateOAuthClient(raw); err != nil || !strings.Contains(string(raw), synthID) {
		t.Fatalf("the generated client JSON is not a Desktop client: %v", err)
	}
}

func TestSetupTakesKeysFromTheEnvironmentAndFlagsWin(t *testing.T) {
	env := func(k string) string {
		return map[string]string{"TELEGRAM_BOT_TOKEN": synthToken, "TELEGRAM_OWNER_ID": "10"}[k]
	}
	code, _, errb, cfgPath := setupArgs(t, env, "setup", "--telegram-owner-id", "99")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb)
	}
	cfg := loadCfg(t, cfgPath)
	if cfg.TelegramOwnerID != 99 || cfg.TelegramTokenFile == "" {
		t.Fatalf("flag must beat the environment, token must come from it: %+v", cfg)
	}
}

func TestSetupNoEnvIgnoresEverything(t *testing.T) {
	env := func(k string) string { return map[string]string{"TELEGRAM_BOT_TOKEN": synthToken}[k] }
	code, _, _, cfgPath := setupArgs(t, env, "setup", "--no-env")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if cfg := loadCfg(t, cfgPath); cfg.TelegramTokenFile != "" {
		t.Fatalf("--no-env still used the environment: %+v", cfg)
	}
}

func TestSetupRejectsBadKeysBeforeCreatingAnything(t *testing.T) {
	cases := map[string]string{
		"half google creds": "GOOGLE_OAUTH_CLIENT_ID=" + synthID + "\n",
		"bad google id":     "GOOGLE_OAUTH_CLIENT_ID=nope\nGOOGLE_OAUTH_CLIENT_SECRET=x\n",
		"bad bot token":     "TELEGRAM_BOT_TOKEN=not-a-token\n",
		"bad owner":         "TELEGRAM_OWNER_ID=abc\n",
	}
	for name, body := range cases {
		code, _, _, cfgPath := setupArgs(t, nil, "setup", "--env-file", writeEnvFile(t, body))
		if code != ExitUsage {
			t.Errorf("%s: exit %d, want usage", name, code)
		}
		if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
			t.Errorf("%s: a rejected setup created a config", name)
		}
	}
}

func TestSetupRefusesAWebOAuthClient(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "web.json")
	if err := os.WriteFile(web, []byte(`{"web":{"client_id":"a","client_secret":"b"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errb, _ := setupArgs(t, nil, "setup", "--oauth-client", web)
	if code != ExitUsage || !strings.Contains(errb, "Desktop") {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
}

func TestSetupAcceptsADesktopClientFileThatIsNotPrivate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "client_secret.json")
	body := `{"installed":{"client_id":"` + synthID + `","client_secret":"` + synthSecret + `"}}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errb, cfgPath := setupArgs(t, nil, "setup", "--oauth-client", p)
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if strings.Contains(out, synthSecret) {
		t.Fatal("the client secret was printed")
	}
	cfg := loadCfg(t, cfgPath)
	if _, err := config.ReadPrivate(cfg.OAuthClientFile); err != nil || cfg.OAuthClientFile == p {
		t.Fatalf("the client file must be copied to a private store: %v (%s)", err, cfg.OAuthClientFile)
	}
}

func TestWizardAsksOnlyForWhatIsMissing(t *testing.T) {
	home := privateDir(t)
	cfgPath := filepath.Join(home, "config.json")
	var out, errb strings.Builder
	a := &App{io: IO{In: strings.NewReader("\n" + synthToken + "\n555\n"), Out: &out, Err: &errb}}
	a.g.configPath = cfgPath
	if err := a.runSetup(&setupInput{noEnv: true}, true); err != nil {
		t.Fatal(err)
	}
	cfg := loadCfg(t, cfgPath)
	if cfg.OAuthClientFile != "" {
		t.Fatal("an empty answer must skip Google")
	}
	if cfg.TelegramOwnerID != 555 || cfg.TelegramTokenFile == "" {
		t.Fatalf("wizard answers not applied: %+v", cfg)
	}
	if strings.Contains(out.String()+errb.String(), synthToken) {
		t.Fatal("the typed token was echoed")
	}
}

func TestParseDotEnvHandlesQuotesCommentsAndExport(t *testing.T) {
	got := parseDotEnv(strings.NewReader("A=1\n# c\n\nexport B=\"two words\"\nC='x' \nD=v #note\nE=\nbad line\n"))
	want := map[string]string{"A": "1", "B": "two words", "C": "x", "D": "v", "E": ""}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["bad line"]; ok {
		t.Error("a line without = was parsed")
	}
}

// --env-file names a file on purpose: a wrong path is an error even when the
// process environment already holds usable values.
func TestSetupEnvFileThatCannotBeReadIsAnErrorEvenWithAnEnvironment(t *testing.T) {
	env := func(k string) string {
		if k == "TELEGRAM_BOT_TOKEN" {
			return synthToken
		}
		return ""
	}
	code, _, errb, cfgPath := setupArgs(t, env, "setup", "--env-file", filepath.Join(t.TempDir(), "missing.env"))
	if code != ExitUsage || !strings.Contains(errb, "cannot read the .env file") {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatal("a rejected setup created a config")
	}
}
