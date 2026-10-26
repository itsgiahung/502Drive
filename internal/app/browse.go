package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

func validKey(key string) error {
	if err := state.ValidateKey(key); err != nil {
		return Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	return nil
}

const (
	// browseMaxDepth bounds the breadcrumb walk up the parent chain.
	browseMaxDepth = 12
	// RootFolder is Drive's alias for "My Drive".
	RootFolder = "root"
)

// FolderView is one folder of the Drive browser: where it is, what is in it.
type FolderView struct {
	Folder drive.File
	// Path runs from the highest ancestor the account can see down to Folder.
	Path          []drive.File
	Children      []drive.File
	NextPageToken string
}

// Browse lists one page of a folder (folders first, then by name). The
// breadcrumb is only built for the first page. Owner only: it exposes the
// owner's Drive.
func (s *Service) Browse(ctx context.Context, p Principal, folderID, resourceKey, pageToken string) (FolderView, error) {
	var v FolderView
	if err := requireOwner(p); err != nil {
		return v, err
	}
	if folderID == "" {
		folderID = RootFolder
	}
	prov, err := s.provider(ctx)
	if err != nil {
		return v, err
	}
	folder, err := prov.GetFile(ctx, folderID, resourceKey)
	if err != nil {
		return v, err
	}
	if !folder.IsFolder() || folder.Trashed {
		return v, Fail(http.StatusBadRequest, "not_a_folder", "That item is not a folder.")
	}
	children, next, err := prov.ListChildren(ctx, folder.ID, resourceKey, pageToken)
	if err != nil {
		return v, err
	}
	v = FolderView{Folder: folder, Children: children, NextPageToken: next}
	if pageToken == "" {
		v.Path = breadcrumb(ctx, prov, folder)
	}
	return v, nil
}

// breadcrumb walks the first parent upward. A parent the account cannot read
// (a shared folder inside someone else's tree) simply ends the walk.
func breadcrumb(ctx context.Context, prov drive.Provider, folder drive.File) []drive.File {
	path := []drive.File{folder}
	cur := folder
	for depth := 0; depth < browseMaxDepth && len(cur.Parents) > 0; depth++ {
		parent, err := prov.GetFile(ctx, cur.Parents[0], "")
		if err != nil || parent.Trashed {
			break
		}
		path = append(path, parent)
		cur = parent
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// FileInfo returns the metadata of one file or folder (owner only).
func (s *Service) FileInfo(ctx context.Context, p Principal, id, resourceKey string) (drive.File, error) {
	if err := requireOwner(p); err != nil {
		return drive.File{}, err
	}
	prov, err := s.provider(ctx)
	if err != nil {
		return drive.File{}, err
	}
	f, err := prov.GetFile(ctx, id, resourceKey)
	if err != nil {
		return drive.File{}, err
	}
	if f.Trashed {
		return drive.File{}, Fail(http.StatusNotFound, "not_found", "That item is in the trash.")
	}
	return f, nil
}

// Mkdir creates a folder under parentID. The write carries an intent derived
// from the idempotency key, the parent and the name, so a lost response or a
// retry adopts the folder that was already made instead of making a second.
func (s *Service) Mkdir(ctx context.Context, p Principal, key, parentID, name string) (drive.File, bool, error) {
	if err := requireOwner(p); err != nil {
		return drive.File{}, false, err
	}
	if err := validKey(key); err != nil {
		return drive.File{}, false, err
	}
	if !validName(name) {
		return drive.File{}, false, Fail(http.StatusBadRequest, "invalid_request", "name must be 1-255 characters without control characters or surrounding spaces.")
	}
	if parentID == "" {
		parentID = RootFolder
	}
	prov, err := s.provider(ctx)
	if err != nil {
		return drive.File{}, false, err
	}
	parent, err := prov.GetFile(ctx, parentID, "")
	if err != nil {
		return drive.File{}, false, err
	}
	if !parent.IsFolder() || parent.Trashed {
		return drive.File{}, false, Fail(http.StatusBadRequest, "not_a_folder", "The parent is not a folder.")
	}
	if !parent.CanAddChildren {
		return drive.File{}, false, Fail(http.StatusConflict, "destination_not_writable", "This account cannot add files to that folder.")
	}
	sum := sha256.Sum256([]byte(key + "\x00" + parent.ID + "\x00" + name))
	intent := "mkdir-" + hex.EncodeToString(sum[:12])
	if found, ferr := prov.FindByIntent(ctx, parent.ID, intent); ferr == nil && len(found) == 1 {
		return found[0], true, nil
	}
	f, err := prov.CreateFolder(ctx, name, parent.ID, intent)
	if err == nil {
		return f, false, nil
	}
	// Unknown outcome (the request may have reached Drive): look for our own
	// tagged folder before reporting a failure.
	var de *drive.Error
	if errors.As(err, &de) && de.Kind == drive.KindUnavailable {
		if found, ferr := prov.FindByIntent(ctx, parent.ID, intent); ferr == nil && len(found) == 1 {
			return found[0], true, nil
		}
	}
	return drive.File{}, false, err
}
