package live

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/state"
)

func TestLiveFolderNameFormat(t *testing.T) {
	name := LiveFolderName()
	if !strings.HasPrefix(name, "502drive-live-test-") {
		t.Fatalf("expected prefix 502drive-live-test-, got %s", name)
	}
}

func TestCheckLiveSkipsWhenDisabled(t *testing.T) {
	// Ensure DRIVE502_LIVE is not "1"
	old := os.Getenv(EnvDrive502Live)
	os.Unsetenv(EnvDrive502Live)
	defer os.Setenv(EnvDrive502Live, old)

	if IsLiveEnabled() {
		t.Fatal("expected IsLiveEnabled to be false when env is unset")
	}
}

func TestLiveWorkspaceIsolationAndCleanup(t *testing.T) {
	fake := drivetest.New()
	ctx := context.Background()

	ws, err := CreateWorkspace(ctx, fake)
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}
	if !strings.HasPrefix(ws.RootFolderName, "502drive-live-test-") {
		t.Fatalf("unexpected folder name: %s", ws.RootFolderName)
	}

	rootFile, err := fake.GetFile(ctx, ws.RootFolderID, "")
	if err != nil {
		t.Fatalf("failed to get created workspace folder: %v", err)
	}
	if rootFile.Trashed {
		t.Fatal("workspace folder should not be trashed initially")
	}

	// Cleanup should trash the folder
	if err := ws.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// After trash, GetFile should either return NotFound/404 (as Fake does) or Trashed=true
	trashedFile, err := fake.GetFile(ctx, ws.RootFolderID, "")
	if err == nil && !trashedFile.Trashed {
		t.Fatal("workspace folder should be marked trashed after cleanup")
	}

	// Second cleanup should be a safe no-op
	if err := ws.Cleanup(ctx); err != nil {
		t.Fatalf("second Cleanup call failed: %v", err)
	}
}

func TestScenarioCloneSmallTreeWithFake(t *testing.T) {
	fake := drivetest.New()
	ctx := context.Background()

	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("state.Open failed: %v", err)
	}
	defer store.Close()

	ws, err := CreateWorkspace(ctx, fake)
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}
	defer func() { _ = ws.Cleanup(ctx) }()

	if err := ScenarioCloneSmallTree(ctx, store, fake, ws); err != nil {
		t.Fatalf("ScenarioCloneSmallTree failed: %v", err)
	}
}
