package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/engine"
	"github.com/itsgiahung/502Drive/internal/state"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	EnvDrive502Live      = "DRIVE502_LIVE"
	EnvLiveClientSecret  = "DRIVE502_LIVE_CLIENT_SECRET"
	EnvLiveAccessToken   = "DRIVE502_LIVE_ACCESS_TOKEN"
	EnvLiveTokenData     = "DRIVE502_LIVE_TOKEN_DATA"
	EnvLiveTelegramToken = "DRIVE502_LIVE_TELEGRAM_TOKEN"
)

// IsLiveEnabled reports whether live integration testing is enabled.
func IsLiveEnabled() bool {
	return os.Getenv(EnvDrive502Live) == "1"
}

// LiveFolderName returns a unique directory name for test isolation.
func LiveFolderName() string {
	return fmt.Sprintf("502drive-live-test-%d", time.Now().Unix())
}

// CheckLive verifies that live tests are explicitly enabled and credentials are present.
// If not, it skips the test with an explanatory message.
func CheckLive(t testing.TB) {
	t.Helper()
	if !IsLiveEnabled() {
		t.Skip("skipping live test: DRIVE502_LIVE=1 not set")
	}
	if os.Getenv(EnvLiveAccessToken) == "" &&
		os.Getenv(EnvLiveClientSecret) == "" &&
		os.Getenv(EnvLiveTokenData) == "" {
		t.Skip("skipping live test: no credentials found (set DRIVE502_LIVE_ACCESS_TOKEN or DRIVE502_LIVE_CLIENT_SECRET / DRIVE502_LIVE_TOKEN_DATA)")
	}
}

// LiveWorkspace manages an isolated root directory on Google Drive for testing.
// All created files and folders are contained within RootFolderID.
// On cleanup, RootFolderID is moved to trash.
type LiveWorkspace struct {
	RootFolderID   string
	RootFolderName string
	Provider       drive.Provider

	mu      sync.Mutex
	cleaned bool
}

// CreateWorkspace creates a new isolated root folder on Drive.
func CreateWorkspace(ctx context.Context, p drive.Provider) (*LiveWorkspace, error) {
	name := LiveFolderName()
	f, err := p.CreateFolder(ctx, name, "root", "live_harness_workspace")
	if err != nil {
		return nil, fmt.Errorf("create live workspace folder: %w", err)
	}
	return &LiveWorkspace{
		RootFolderID:   f.ID,
		RootFolderName: name,
		Provider:       p,
	}, nil
}

// Cleanup moves the workspace folder to Google Drive trash.
func (w *LiveWorkspace) Cleanup(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cleaned || w.RootFolderID == "" {
		return nil
	}
	w.cleaned = true
	return w.Provider.TrashFile(ctx, w.RootFolderID)
}

// SetupWorkspace creates a workspace and registers its cleanup with the test.
func SetupWorkspace(t testing.TB, ctx context.Context, p drive.Provider) *LiveWorkspace {
	t.Helper()
	ws, err := CreateWorkspace(ctx, p)
	if err != nil {
		t.Fatalf("failed to setup live workspace: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := ws.Cleanup(cleanCtx); err != nil {
			t.Logf("warning: failed to clean up live workspace %s (%s): %v", ws.RootFolderName, ws.RootFolderID, err)
		}
	})
	return ws
}

// NewLiveClient constructs a Google Drive Client using live test credentials and
// optional custom transport (e.g. for fault injection).
func NewLiveClient(ctx context.Context, transport http.RoundTripper) (*drive.Client, error) {
	if token := os.Getenv(EnvLiveAccessToken); token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: token,
			TokenType:   "Bearer",
		})
		return drive.NewClientWithTransport(ctx, ts, "", transport)
	}

	secretData := os.Getenv(EnvLiveClientSecret)
	tokenData := os.Getenv(EnvLiveTokenData)
	if secretData == "" {
		return nil, errors.New("missing live credentials")
	}

	var rawSecret []byte
	if strings.HasPrefix(secretData, "{") {
		rawSecret = []byte(secretData)
	} else {
		var err error
		rawSecret, err = os.ReadFile(secretData)
		if err != nil {
			return nil, fmt.Errorf("read client secret: %w", err)
		}
	}

	cfg, err := google.ConfigFromJSON(rawSecret, drive.Scope)
	if err != nil {
		return nil, fmt.Errorf("parse client secret: %w", err)
	}

	var tok oauth2.Token
	if strings.HasPrefix(tokenData, "{") {
		if err := json.Unmarshal([]byte(tokenData), &tok); err != nil {
			return nil, fmt.Errorf("unmarshal token JSON: %w", err)
		}
	} else if tokenData != "" {
		rawTok, err := os.ReadFile(tokenData)
		if err != nil {
			return nil, fmt.Errorf("read token file: %w", err)
		}
		if err := json.Unmarshal(rawTok, &tok); err != nil {
			return nil, fmt.Errorf("unmarshal token file JSON: %w", err)
		}
	} else {
		return nil, errors.New("missing live token data")
	}

	ts := cfg.TokenSource(ctx, &tok)
	return drive.NewClientWithTransport(ctx, ts, "", transport)
}

// ScenarioOAuthRefresh verifies that the live credentials can access Google Drive
// and retrieve account details.
func ScenarioOAuthRefresh(ctx context.Context, p drive.Provider) (*drive.Account, error) {
	acc, err := p.Account(ctx)
	if err != nil {
		return nil, fmt.Errorf("account check failed: %w", err)
	}
	if acc.Email == "" {
		return nil, errors.New("live account email is empty")
	}
	return &acc, nil
}

// ScenarioCloneSmallTree creates a small directory tree in ws, runs a copy job
// to a destination folder inside ws, and verifies the destination hierarchy.
func ScenarioCloneSmallTree(ctx context.Context, s *state.Store, p drive.Provider, ws *LiveWorkspace) error {
	// 1. Create source and dest folders inside workspace
	srcFolder, err := p.CreateFolder(ctx, "source", ws.RootFolderID, "live_test_src")
	if err != nil {
		return fmt.Errorf("create source: %w", err)
	}
	destFolder, err := p.CreateFolder(ctx, "dest", ws.RootFolderID, "live_test_dest")
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}

	// 2. Create a subfolder inside source
	subFolder, err := p.CreateFolder(ctx, "subfolder", srcFolder.ID, "live_test_sub")
	if err != nil {
		return fmt.Errorf("create subfolder: %w", err)
	}

	// 3. Plan and Admit a copy job
	planID := fmt.Sprintf("plan_live_%d", time.Now().UnixNano())
	plan := state.Plan{
		ID:                planID,
		Principal:         "owner",
		SourceID:          srcFolder.ID,
		SourceName:        srcFolder.Name,
		SourceKind:        state.KindFolder,
		SourceMime:        drive.MimeFolder,
		SourceVersion:     srcFolder.Fingerprint(),
		DestinationID:     "dest",
		DestinationFolder: destFolder.ID,
		RootName:          "source_copy",
		ExpiresAt:         time.Now().Add(time.Hour),
		Revision:          1,
	}
	if err := s.PutPlan(ctx, plan); err != nil {
		return fmt.Errorf("put plan: %w", err)
	}
	admitResult, err := s.Admit(ctx, "owner", fmt.Sprintf("key_live_%d", time.Now().UnixNano()), planID, 1)
	if err != nil {
		return fmt.Errorf("admit job: %w", err)
	}

	// 4. Run engine to completion
	eng := &engine.Engine{
		Store: s,
		Provider: func(context.Context) (drive.Provider, error) {
			return p, nil
		},
		Workers:     2,
		BackoffBase: 10 * time.Millisecond,
		BackoffMax:  100 * time.Millisecond,
		Poll:        20 * time.Millisecond,
	}
	engCtx, cancelEng := context.WithTimeout(ctx, 60*time.Second)
	defer cancelEng()

	done := make(chan error, 1)
	go func() {
		for {
			select {
			case <-engCtx.Done():
				done <- engCtx.Err()
				return
			default:
				job, err := s.GetJob(engCtx, "owner", admitResult.Job.ID)
				if err != nil {
					done <- err
					return
				}
				if job.State == state.JobSucceeded {
					done <- nil
					return
				}
				if job.State == state.JobFailed || job.State == state.JobBlocked {
					done <- fmt.Errorf("job entered state %s", job.State)
					return
				}
				eng.Wake()
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	runCtx, stopRun := context.WithCancel(engCtx)
	go func() {
		_ = eng.Run(runCtx)
	}()
	defer stopRun()

	if err := <-done; err != nil {
		return fmt.Errorf("engine execution failed: %w", err)
	}

	// 5. Verify copied destination contents
	children, _, err := p.ListChildren(ctx, destFolder.ID, "", "")
	if err != nil {
		return fmt.Errorf("list destination: %w", err)
	}
	if len(children) == 0 {
		return errors.New("destination folder is empty after copy")
	}

	// Find the copied root folder
	var copiedRoot *drive.File
	for i := range children {
		if children[i].Name == "source_copy" {
			copiedRoot = &children[i]
			break
		}
	}
	if copiedRoot == nil {
		return errors.New("copied root folder 'source_copy' not found in destination")
	}

	// Verify subfolder exists inside copied root
	subChildren, _, err := p.ListChildren(ctx, copiedRoot.ID, "", "")
	if err != nil {
		return fmt.Errorf("list copied root: %w", err)
	}
	var foundSub bool
	for _, sc := range subChildren {
		if sc.Name == subFolder.Name {
			foundSub = true
			break
		}
	}
	if !foundSub {
		return errors.New("subfolder was not copied under destination root")
	}

	return nil
}

// ScenarioTelegramBot verifies Telegram bot connectivity using the test token.
func ScenarioTelegramBot(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("no telegram bot token")
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram getMe request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram getMe returned status %d", resp.StatusCode)
	}
	var payload struct {
		OK          bool   `json:"ok"`
		Result      any    `json:"result"`
		Description string `json:"description"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("unmarshal telegram response: %w", err)
	}
	if !payload.OK {
		return fmt.Errorf("telegram error: %s", payload.Description)
	}
	return nil
}
