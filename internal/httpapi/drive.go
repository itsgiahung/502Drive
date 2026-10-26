package httpapi

import (
	"net/http"
	"time"

	"github.com/itsgiahung/502Drive/internal/app"
	"github.com/itsgiahung/502Drive/internal/drive"
)

func fileKind(f drive.File) string {
	switch {
	case f.IsFolder():
		return "folder"
	case f.IsShortcut():
		return "shortcut"
	}
	return kindDTO("file", f.MimeType)
}

func optTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return ts(t)
}

// driveFileDTO is the display metadata of one Drive item. Nothing here is a
// credential; owner details are the account owner's own Drive.
func driveFileDTO(f drive.File) map[string]any {
	var size any
	if f.Size != nil {
		size = *f.Size
	}
	var owner any
	if f.OwnerName != "" || f.OwnerEmail != "" {
		owner = map[string]string{"name": f.OwnerName, "email": f.OwnerEmail}
	}
	return map[string]any{
		"id": f.ID, "name": f.Name, "kind": fileKind(f), "mime_type": f.MimeType, "size_bytes": size,
		"md5": f.MD5, "created_at": optTime(f.Created), "modified_at": optTime(f.Modified), "owner": owner,
		"web_url": f.WebLink, "can_copy": f.CanCopy, "can_add_children": f.CanAddChildren,
		"resource_key": f.ResourceKey, "shortcut_target_id": f.ShortcutTargetID,
	}
}

func (s *Server) browseFolder(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	q := r.URL.Query()
	v, err := s.Svc.Browse(r.Context(), p, r.PathValue("id"), q.Get("resource_key"), q.Get("page_token"))
	if err != nil {
		return 0, nil, err
	}
	path := make([]any, 0, len(v.Path))
	for _, f := range v.Path {
		path = append(path, map[string]string{"id": f.ID, "name": f.Name})
	}
	items := make([]any, 0, len(v.Children))
	for _, f := range v.Children {
		items = append(items, driveFileDTO(f))
	}
	var next any
	if v.NextPageToken != "" {
		next = v.NextPageToken
	}
	return 200, map[string]any{"folder": driveFileDTO(v.Folder), "path": path, "items": items, "next_page_token": next}, nil
}

func (s *Server) driveFile(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	f, err := s.Svc.FileInfo(r.Context(), p, r.PathValue("id"), r.URL.Query().Get("resource_key"))
	if err != nil {
		return 0, nil, err
	}
	return 200, driveFileDTO(f), nil
}

type mkdirInput struct {
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
}

func (s *Server) mkdir(w http.ResponseWriter, r *http.Request, p app.Principal) (int, any, error) {
	if err := s.refuseMaintenance(); err != nil {
		return 0, nil, err
	}
	var in mkdirInput
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	f, replayed, err := s.Svc.Mkdir(r.Context(), p, idemKey(r), in.ParentID, in.Name)
	if err != nil {
		return 0, nil, err
	}
	markReplay(w, replayed)
	return 201, driveFileDTO(f), nil
}
