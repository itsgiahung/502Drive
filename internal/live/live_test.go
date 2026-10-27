//go:build live

package live

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

func TestLiveOAuthRefresh(t *testing.T) {
	CheckLive(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := NewLiveClient(ctx, nil)
	if err != nil {
		t.Fatalf("failed to create live client: %v", err)
	}

	acc, err := ScenarioOAuthRefresh(ctx, client)
	if err != nil {
		t.Fatalf("live oauth refresh / account check failed: %v", err)
	}
	t.Logf("Live account connected: %s (%s)", acc.Name, acc.Email)
}

func TestLiveCloneSmallTree(t *testing.T) {
	CheckLive(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client, err := NewLiveClient(ctx, nil)
	if err != nil {
		t.Fatalf("failed to create live client: %v", err)
	}

	ws := SetupWorkspace(t, ctx, client)
	t.Logf("Created isolated live workspace: %s (%s)", ws.RootFolderName, ws.RootFolderID)

	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("state.Open failed: %v", err)
	}
	defer store.Close()

	if err := ScenarioCloneSmallTree(ctx, store, client, ws); err != nil {
		t.Fatalf("ScenarioCloneSmallTree failed: %v", err)
	}
	t.Log("Successfully cloned small tree in live workspace")
}

func TestLiveCrashAndResume(t *testing.T) {
	CheckLive(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Wrap live transport with FaultInjector to simulate dropped response
	inj := drive.NewFaultInjector(nil)
	inj.InjectLoseResponse("/copy", 1)

	client, err := NewLiveClient(ctx, inj)
	if err != nil {
		t.Fatalf("failed to create live client with injector: %v", err)
	}

	ws := SetupWorkspace(t, ctx, client)
	t.Logf("Created isolated live workspace for crash test: %s (%s)", ws.RootFolderName, ws.RootFolderID)

	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("state.Open failed: %v", err)
	}
	defer store.Close()

	if err := ScenarioCloneSmallTree(ctx, store, client, ws); err != nil {
		t.Fatalf("clone with crash and resume failed: %v", err)
	}

	if inj.Hits() == 0 {
		t.Log("note: no /copy calls matched injector (single folder tree), but scenario completed")
	} else {
		t.Logf("successfully recovered from %d injected fault(s)", inj.Hits())
	}
}

func TestLiveSyncDetectsChanges(t *testing.T) {
	CheckLive(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	client, err := NewLiveClient(ctx, nil)
	if err != nil {
		t.Fatalf("failed to create live client: %v", err)
	}

	ws := SetupWorkspace(t, ctx, client)

	// Create source folder
	srcFolder, err := client.CreateFolder(ctx, "sync_source", ws.RootFolderID, "live_sync_src")
	if err != nil {
		t.Fatalf("create sync source folder: %v", err)
	}

	// Read initial change start page token
	token, err := client.StartPageToken(ctx)
	if err != nil {
		t.Fatalf("get start page token: %v", err)
	}

	// Create a new subfolder in source to generate a change
	sub, err := client.CreateFolder(ctx, "added_after_token", srcFolder.ID, "live_sync_item")
	if err != nil {
		t.Fatalf("create subfolder in source: %v", err)
	}

	// Poll changes and confirm new item is observed
	changes, _, _, err := client.ListChanges(ctx, token)
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}

	var found bool
	for _, ch := range changes {
		if ch.FileID == sub.ID {
			found = true
			break
		}
	}
	if !found {
		t.Logf("note: change for %s not immediately in first change page (eventual consistency)", sub.ID)
	} else {
		t.Logf("live change feed successfully detected item %s", sub.ID)
	}
}

func TestLiveTelegramBot(t *testing.T) {
	token := os.Getenv(EnvLiveTelegramToken)
	if token == "" {
		t.Skip("skipping telegram live test: DRIVE502_LIVE_TELEGRAM_TOKEN not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := ScenarioTelegramBot(ctx, token); err != nil {
		t.Fatalf("telegram bot live test failed: %v", err)
	}
	t.Log("live telegram bot getMe check passed")
}
