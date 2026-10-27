package drive_test

import (
	"context"
	"testing"

	"github.com/itsgiahung/502Drive/internal/drive/drivetest"
)

func TestSharedDriveFakeMode(t *testing.T) {
	fake := drivetest.New()
	sd := fake.SharedDrive("Team Drive Alpha")

	if sd.DriveID == "" {
		t.Fatalf("expected SharedDrive to have non-empty DriveID, got %+v", sd)
	}

	// Create subfolder and files inside Shared Drive
	sub := fake.Folder("SubFolder", sd.ID)
	if sub.DriveID != sd.DriveID {
		t.Errorf("expected subfolder to inherit DriveID %s, got %s", sd.DriveID, sub.DriveID)
	}

	file := fake.File("data.bin", sub.ID, 123)
	if file.DriveID != sd.DriveID {
		t.Errorf("expected file to inherit DriveID %s, got %s", sd.DriveID, file.DriveID)
	}

	// Test read-only / viewer role in Shared Drive
	fake.SetSharedDriveRole(sd.DriveID, "viewer")

	// Viewer cannot add children
	getFolder, ok := fake.Get(sub.ID)
	if !ok || getFolder.CanAddChildren {
		t.Errorf("expected CanAddChildren=false for viewer in Shared Drive, got %v", getFolder.CanAddChildren)
	}

	// Creating folder in read-only Shared Drive fails
	_, err := fake.CreateFolder(context.Background(), "IllegalFolder", sub.ID, "intent1")
	if err == nil {
		t.Errorf("expected error creating folder in read-only Shared Drive, got nil")
	}

	// Copying file in read-only Shared Drive fails
	srcFile := fake.File("source.bin", "root", 10)
	_, err = fake.CopyFile(context.Background(), srcFile, sub.ID, "intent2")
	if err == nil {
		t.Errorf("expected error copying file into read-only Shared Drive, got nil")
	}

	// Restore organizer role
	fake.SetSharedDriveRole(sd.DriveID, "organizer")
	newFolder, err := fake.CreateFolder(context.Background(), "LegalFolder", sub.ID, "intent3")
	if err != nil {
		t.Fatalf("unexpected error creating folder in organizer Shared Drive: %v", err)
	}
	if newFolder.DriveID != sd.DriveID {
		t.Errorf("expected new folder to inherit DriveID %s, got %s", sd.DriveID, newFolder.DriveID)
	}

	// Test GetFile with driveId
	f, err := fake.GetFile(context.Background(), newFolder.ID, "")
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}
	if f.DriveID != sd.DriveID {
		t.Errorf("expected GetFile to return DriveID %s, got %s", sd.DriveID, f.DriveID)
	}
}

func TestSharedDriveInspectPermissions(t *testing.T) {
	fake := drivetest.New()
	sd := fake.SharedDrive("Team Drive Restricted")
	fake.SetSharedDriveRole(sd.DriveID, "viewer")

	// Destination in read-only shared drive must be refused as destination_not_writable
	f, _ := fake.Get(sd.ID)
	if f.CanAddChildren {
		t.Errorf("viewer should not have CanAddChildren=true")
	}
}
