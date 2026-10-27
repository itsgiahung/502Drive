package app_test

import (
	"context"
	"testing"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
	"github.com/itsgiahung/502Drive/internal/state"
)

func TestSharedDriveAppPermissions(t *testing.T) {
	fake := drivetest.New()
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := &app.Service{
		Store:    store,
		Provider: func(context.Context) (drive.Provider, error) { return fake, nil },
	}

	sdSrc := fake.SharedDrive("Source Shared Drive")
	fileSrc := fake.File("restricted.bin", sdSrc.ID, 100)

	sdDst := fake.SharedDrive("Dest Shared Drive")

	// Save destination profile
	dstProfile, _, err := store.AddDestination(context.Background(), "owner", "dst_key", nil, state.Destination{
		ID:       "dst_sd",
		FolderID: sdDst.ID,
		DriveID:  sdDst.DriveID,
		Name:     "Dest SD",
		Revision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. When destination shared drive is viewer only (missing write permission)
	fake.SetSharedDriveRole(sdDst.DriveID, "viewer")
	_, _, err = svc.Inspect(context.Background(), app.Owner, "key1", app.PlanRequest{
		Source:        sdSrc.ID,
		DestinationID: dstProfile.ID,
	})
	if err == nil || app.Classify(err).Code != "destination_not_writable" {
		t.Fatalf("expected destination_not_writable, got %v", err)
	}

	// 2. Restore destination permission, but restrict source file copy permission (viewer only)
	fake.SetSharedDriveRole(sdDst.DriveID, "organizer")
	fake.SetSharedDriveRole(sdSrc.DriveID, "viewer")
	_, _, err = svc.Inspect(context.Background(), app.Owner, "key2", app.PlanRequest{
		Source:        fileSrc.ID,
		DestinationID: dstProfile.ID,
	})
	if err == nil || app.Classify(err).Code != "copy_not_permitted" {
		t.Fatalf("expected copy_not_permitted for viewer on source file, got %v", err)
	}

	// 3. Organizer in both -> Inspect succeeds
	fake.SetSharedDriveRole(sdSrc.DriveID, "organizer")
	view, _, err := svc.Inspect(context.Background(), app.Owner, "key3", app.PlanRequest{
		Source:        sdSrc.ID,
		DestinationID: dstProfile.ID,
	})
	if err != nil {
		t.Fatalf("unexpected Inspect error: %v", err)
	}
	if view.SourceID != sdSrc.ID || view.DestinationID != dstProfile.ID {
		t.Fatalf("unexpected plan view: %+v", view)
	}
}
