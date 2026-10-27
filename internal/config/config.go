package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Config struct {
	Listen            string `json:"listen"`
	DataDir           string `json:"data_dir"`
	TokenFile         string `json:"token_file"`
	OAuthClientFile   string `json:"oauth_client_file,omitempty"`
	TelegramTokenFile string `json:"telegram_token_file,omitempty"`
	TelegramOwnerID   int64  `json:"telegram_owner_id,omitempty"`
	OAuthCallbackPort int    `json:"oauth_callback_port,omitempty"`
	// Container allows listen to bind the container's own interfaces
	// ("0.0.0.0:PORT") instead of loopback. Only the container image sets it,
	// and the host must publish loopback-only ports. Setup never enables it.
	Container bool `json:"container,omitempty"`
	// CheckUpdates lets the tray ask GitHub once a day whether a newer release
	// exists. Off by default: nothing contacts the network unasked.
	CheckUpdates bool `json:"check_updates,omitempty"`
	// KeepAwakeWhileCopying holds a block inhibitor while jobs run. Off by
	// default on purpose: keeping a laptop awake costs battery and heat.
	KeepAwakeWhileCopying bool `json:"keep_awake_while_copying,omitempty"`
	// TrayQuitStopsDaemon restores the old behaviour where quitting the tray
	// also stops the daemon. Off by default: quitting the tray keeps
	// 502Drive running; stop it with `502drive stop` or the tray's stop item.
	TrayQuitStopsDaemon bool `json:"tray_quit_stops_daemon,omitempty"`
	// FullReconcileEvery is how often a realtime subscription re-reads the
	// whole source tree (Go duration, e.g. "168h"). Empty keeps the 7-day
	// default; "0" or "off" disables the periodic reconcile.
	FullReconcileEvery string `json:"full_reconcile_every,omitempty"`
	// Workers is the starting account-wide concurrency (default 4).
	Workers int `json:"workers,omitempty"`
	// MaxWorkers is the AIMD ceiling (default 8, never below Workers).
	MaxWorkers int `json:"max_workers,omitempty"`
	// ReadRequestsPerSecond paces list/get/changes calls through one account
	// token bucket (default 8; 0 keeps the default).
	ReadRequestsPerSecond float64 `json:"read_requests_per_second,omitempty"`
	// ThrottleSuccesses is how many consecutive successful calls grow the
	// concurrency by one (default 8).
	ThrottleSuccesses int `json:"throttle_successes,omitempty"`
	// MaxActiveJobs bounds how many jobs run at once (default 2). Each job
	// already fans out across the account-wide throttle; this is the queue
	// depth above it.
	MaxActiveJobs int `json:"max_active_jobs,omitempty"`
}

// ActiveJobs returns the number of jobs the engine may run at once.
func (c Config) ActiveJobs() int {
	if c.MaxActiveJobs <= 0 {
		return 2
	}
	return c.MaxActiveJobs
}

// Concurrency returns the effective AIMD start/ceiling and success window.
func (c Config) Concurrency() (start, maxWorkers, successes int) {
	start, maxWorkers, successes = c.Workers, c.MaxWorkers, c.ThrottleSuccesses
	if start == 0 {
		start = 4
	}
	if maxWorkers == 0 {
		maxWorkers = 8
	}
	if maxWorkers < start {
		maxWorkers = start
	}
	if successes == 0 {
		successes = 8
	}
	return
}

// ReadRate returns the read budget in requests per second (default 8).
func (c Config) ReadRate() float64 {
	if c.ReadRequestsPerSecond == 0 {
		return 8
	}
	return c.ReadRequestsPerSecond
}

// DefaultFullReconcileEvery is the periodic full reconcile interval used when
// the config does not name one.
const DefaultFullReconcileEvery = 7 * 24 * time.Hour

// FullReconcileEveryDuration parses the configured interval. Zero disables the
// periodic reconcile; an empty value keeps the default.
func (c Config) FullReconcileEveryDuration() (time.Duration, error) {
	v := strings.TrimSpace(c.FullReconcileEvery)
	switch v {
	case "":
		return DefaultFullReconcileEvery, nil
	case "0", "off":
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, errors.New("full_reconcile_every must be a duration such as 168h, or 0 to disable it")
	}
	return d, nil
}

// AppDirName is the per-user folder that holds config.json, the API token and
// the secrets. LegacyAppDirName is the folder used before v0.3.0: an existing
// setup there keeps working as it is. Its config.json names absolute paths
// inside that folder, so it is never moved behind the user's back.
const (
	AppDirName       = "502drive"
	LegacyAppDirName = "502drive-next"
)

// DefaultPath is <user config dir>/502drive/config.json, or the pre-0.3
// <user config dir>/502drive-next/config.json when only that one exists.
func DefaultPath() string {
	p, e := os.UserConfigDir()
	if e != nil {
		return "config.json"
	}
	return defaultPathIn(p)
}

func defaultPathIn(base string) string {
	current := filepath.Join(base, AppDirName, "config.json")
	if _, err := os.Stat(current); err == nil {
		return current
	}
	legacy := filepath.Join(base, LegacyAppDirName, "config.json")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}

// IsLegacyDir reports whether dir is a pre-0.3 config folder.
func IsLegacyDir(dir string) bool {
	return filepath.Base(filepath.Clean(dir)) == LegacyAppDirName
}
func ValidateListen(v string) error {
	return validateListen(v, false)
}

// validateListen accepts loopback always, and the unspecified addresses only
// when container explicitly opts in: there the host publishes 127.0.0.1 only.
func validateListen(v string, container bool) error {
	host, port, e := net.SplitHostPort(v)
	if e != nil || port == "" {
		return errors.New("listen must be a loopback IP and port")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("listen must be an IP address")
	}
	if ip.IsLoopback() {
		return nil
	}
	if container && ip.IsUnspecified() {
		return nil
	}
	return errors.New("only loopback listeners are supported")
}
func Load(path string) (Config, error) {
	var c Config
	f, e := os.Open(path)
	if e != nil {
		return c, errors.New("config unavailable; run setup with an explicit private directory")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, errors.New("invalid config")
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return c, errors.New("trailing config data")
	}
	if e = validateListen(c.Listen, c.Container); e != nil {
		return c, e
	}
	if c.DataDir == "" || c.TokenFile == "" {
		return c, errors.New("data_dir and token_file are required")
	}
	base, e := filepath.Abs(filepath.Dir(path))
	if e != nil {
		return c, e
	}
	for _, v := range []*string{&c.DataDir, &c.TokenFile, &c.OAuthClientFile, &c.TelegramTokenFile} {
		if *v != "" && !filepath.IsAbs(*v) {
			*v = filepath.Join(base, *v)
		}
	}
	return c, nil
}
func Setup(path string) (Config, error) {
	var c Config
	p, e := filepath.Abs(path)
	if e != nil {
		return c, e
	}
	dir := filepath.Dir(p)
	if _, e = os.Lstat(p); !os.IsNotExist(e) {
		return c, errors.New("config already exists or cannot be inspected")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return c, e
	}
	i, e := os.Lstat(dir)
	if e != nil || !i.IsDir() {
		return c, errors.New("config parent must be a directory")
	}
	if runtime.GOOS != "windows" && i.Mode().Perm()&0077 != 0 {
		return c, errors.New("config directory must be private (0700)")
	}
	c = Config{Listen: "127.0.0.1:5020", DataDir: filepath.Join(dir, "data-v2"), TokenFile: filepath.Join(dir, "api-token")}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		return c, e
	}
	if e = WriteExclusive(c.TokenFile, []byte(base64.RawURLEncoding.EncodeToString(raw)+"\n")); e != nil {
		return c, e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return c, e
	}
	if e = WriteExclusive(p, append(b, '\n')); e != nil {
		_ = os.Remove(c.TokenFile)
		return c, e
	}
	return c, nil
}
func WriteExclusive(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	return errors.Join(e, closeErr)
}
func ReadPrivate(path string) ([]byte, error) {
	i, e := os.Lstat(path)
	if e != nil || !i.Mode().IsRegular() {
		return nil, errors.New("private file unavailable")
	}
	if runtime.GOOS != "windows" && i.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("private file permissions must exclude group and other")
	}
	if i.Size() > 65536 {
		return nil, errors.New("private file too large")
	}
	return os.ReadFile(path)
}

// Save rewrites an existing config atomically with private permissions.
func Save(path string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(append(b, '\n'))
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

// Validate checks values that Load does not.
func (c Config) Validate() error {
	if c.OAuthCallbackPort < 0 || c.OAuthCallbackPort > 65535 {
		return errors.New("oauth_callback_port is out of range")
	}
	if _, err := c.FullReconcileEveryDuration(); err != nil {
		return err
	}
	start, maxWorkers, successes := c.Concurrency()
	if start < 1 || start > 64 || maxWorkers < 1 || maxWorkers > 64 {
		return errors.New("workers and max_workers must be between 1 and 64")
	}
	if successes < 1 || successes > 1000 {
		return errors.New("throttle_successes must be between 1 and 1000")
	}
	if r := c.ReadRequestsPerSecond; r < 0 || r > 1000 {
		return errors.New("read_requests_per_second must be between 0 and 1000")
	}
	if c.MaxActiveJobs < 0 || c.MaxActiveJobs > 32 {
		return errors.New("max_active_jobs must be between 1 and 32")
	}
	return validateListen(c.Listen, c.Container)
}
