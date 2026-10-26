// Package drivetest is an in-memory drive.Provider for tests. It can apply a
// write but lose the response, throttle, and page results, which are the
// failure windows the copy engine must survive.
package drivetest

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/GiaHung07/502Drive/internal/drive"
)

type Fake struct {
	mu       sync.Mutex
	files    map[string]drive.File
	next     int
	PageSize int

	// Before runs ahead of every call; returning an error fails the call
	// without any effect. op is e.g. "copy", "folder", "list", "get".
	Before func(op string, f drive.File) error
	// LoseResponse makes a write take effect but return an unavailable error.
	LoseResponse func(op string, f drive.File) bool

	Calls  map[string]int
	Writes int

	log           []drive.Change // append-only change feed; token = index
	InvalidTokens bool           // makes every feed cursor expire
}

func New() *Fake {
	f := &Fake{files: map[string]drive.File{}, PageSize: 100, Calls: map[string]int{}}
	f.files["root"] = drive.File{ID: "root", Name: "My Drive", MimeType: drive.MimeFolder, CanAddChildren: true, CanCopy: true}
	return f
}

func (f *Fake) id() string { f.next++; return fmt.Sprintf("fake%06d", f.next) }

// Put adds a file under parent and returns it. size<0 means no size (native/folder).
func (f *Fake) Put(name, mime, parent string, size int64) drive.File {
	f.mu.Lock()
	defer f.mu.Unlock()
	x := drive.File{ID: f.id(), Name: name, MimeType: mime, Parents: []string{parent}, Version: "1", CanCopy: true, CanAddChildren: mime == drive.MimeFolder}
	if size >= 0 {
		s := size
		x.Size = &s
		x.MD5 = fmt.Sprintf("md5-%s", x.ID)
	}
	f.files[x.ID] = x
	f.record(x)
	return x
}

func (f *Fake) record(x drive.File) {
	c := x
	f.log = append(f.log, drive.Change{FileID: x.ID, Removed: x.Trashed, File: &c})
}

func (f *Fake) Folder(name, parent string) drive.File {
	return f.Put(name, drive.MimeFolder, parent, -1)
}
func (f *Fake) File(name, parent string, size int64) drive.File {
	return f.Put(name, "application/octet-stream", parent, size)
}

func (f *Fake) Mutate(id string, fn func(*drive.File)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	x := f.files[id]
	fn(&x)
	f.files[id] = x
	f.record(x)
}

func (f *Fake) Get(id string) (drive.File, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	x, ok := f.files[id]
	return x, ok
}

// Children returns the non-trashed children of a folder sorted by name then ID.
func (f *Fake) Children(parent string) []drive.File {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.childrenLocked(parent)
}

func (f *Fake) childrenLocked(parent string) []drive.File {
	var out []drive.File
	for _, x := range f.files {
		if x.Trashed {
			continue
		}
		for _, p := range x.Parents {
			if p == parent {
				out = append(out, x)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Count returns the number of non-trashed files under (and including) root.
func (f *Fake) Count(root string) int {
	n := 1
	for _, c := range f.Children(root) {
		if c.IsFolder() {
			n += f.Count(c.ID)
		} else {
			n++
		}
	}
	return n
}

func (f *Fake) call(op string, x drive.File) error {
	f.Calls[op]++
	if f.Before != nil {
		return f.Before(op, x)
	}
	return nil
}

func (f *Fake) Account(ctx context.Context) (drive.Account, error) {
	return drive.Account{Email: "user@example.test", Name: "Test User"}, nil
}

func (f *Fake) GetFile(ctx context.Context, id, key string) (drive.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("get", drive.File{ID: id}); err != nil {
		return drive.File{}, err
	}
	x, ok := f.files[id]
	if !ok || x.Trashed {
		return drive.File{}, &drive.Error{Kind: drive.KindNotFound, Status: 404}
	}
	return x, nil
}

func (f *Fake) ListChildren(ctx context.Context, folder, key, token string) ([]drive.File, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("list", drive.File{ID: folder}); err != nil {
		return nil, "", err
	}
	if _, ok := f.files[folder]; !ok {
		return nil, "", &drive.Error{Kind: drive.KindNotFound, Status: 404}
	}
	all := f.childrenLocked(folder)
	start := 0
	if token != "" {
		n, err := strconv.Atoi(token)
		if err != nil {
			return nil, "", &drive.Error{Kind: drive.KindInvalid, Status: 400}
		}
		start = n
	}
	end := start + f.PageSize
	next := ""
	if end < len(all) {
		next = strconv.Itoa(end)
	} else {
		end = len(all)
	}
	if start > end {
		start = end
	}
	return all[start:end], next, nil
}

func (f *Fake) write(op string, x drive.File, mk func() drive.File) (drive.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call(op, x); err != nil {
		return drive.File{}, err
	}
	parent := ""
	if len(x.Parents) > 0 {
		parent = x.Parents[0]
	}
	if p, ok := f.files[parent]; !ok || !p.IsFolder() {
		return drive.File{}, &drive.Error{Kind: drive.KindNotFound, Status: 404}
	}
	out := mk()
	f.files[out.ID] = out
	f.record(out)
	f.Writes++
	if f.LoseResponse != nil && f.LoseResponse(op, x) {
		return drive.File{}, &drive.Error{Kind: drive.KindUnavailable, Err: fmt.Errorf("response lost")}
	}
	return out, nil
}

func props(intent string) map[string]string {
	if intent == "" {
		return nil
	}
	return map[string]string{drive.IntentProperty: intent}
}

func (f *Fake) CreateFolder(ctx context.Context, name, parent, intent string) (drive.File, error) {
	return f.write("folder", drive.File{Name: name, Parents: []string{parent}}, func() drive.File {
		return drive.File{ID: f.id(), Name: name, MimeType: drive.MimeFolder, Parents: []string{parent}, Version: "1", CanAddChildren: true, CanCopy: true, AppProperties: props(intent)}
	})
}

func (f *Fake) CopyFile(ctx context.Context, src drive.File, parent, intent string) (drive.File, error) {
	req := src
	req.Parents = []string{parent}
	return f.write("copy", req, func() drive.File {
		x := f.files[src.ID]
		x.ID, x.Parents, x.AppProperties = f.id(), []string{parent}, props(intent)
		return x
	})
}

func (f *Fake) CreateShortcut(ctx context.Context, name, parent, target, key, intent string) (drive.File, error) {
	return f.write("shortcut", drive.File{Name: name, Parents: []string{parent}}, func() drive.File {
		return drive.File{ID: f.id(), Name: name, MimeType: drive.MimeShortcut, Parents: []string{parent}, Version: "1", ShortcutTargetID: target, AppProperties: props(intent)}
	})
}

func (f *Fake) FindByIntent(ctx context.Context, parent, intent string) ([]drive.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("find", drive.File{}); err != nil {
		return nil, err
	}
	var out []drive.File
	for _, c := range f.childrenLocked(parent) {
		if c.AppProperties[drive.IntentProperty] == intent {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *Fake) RenameFile(ctx context.Context, id, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("rename", drive.File{ID: id}); err != nil {
		return err
	}
	x, ok := f.files[id]
	if !ok {
		return &drive.Error{Kind: drive.KindNotFound, Status: 404}
	}
	x.Name = name
	f.files[id] = x
	f.record(x)
	return nil
}

func (f *Fake) MoveFile(ctx context.Context, id, from, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("move", drive.File{ID: id}); err != nil {
		return err
	}
	x, ok := f.files[id]
	if !ok {
		return &drive.Error{Kind: drive.KindNotFound, Status: 404}
	}
	x.Parents = []string{to}
	f.files[id] = x
	f.record(x)
	return nil
}

func (f *Fake) TrashFile(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("trash", drive.File{ID: id}); err != nil {
		return err
	}
	x, ok := f.files[id]
	if !ok {
		return &drive.Error{Kind: drive.KindNotFound, Status: 404}
	}
	x.Trashed = true
	f.files[id] = x
	f.record(x)
	return nil
}

func (f *Fake) StartPageToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("starttoken", drive.File{}); err != nil {
		return "", err
	}
	return strconv.Itoa(len(f.log)), nil
}

func (f *Fake) ListChanges(ctx context.Context, token string) ([]drive.Change, string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("changes", drive.File{}); err != nil {
		return nil, "", "", err
	}
	n, err := strconv.Atoi(token)
	if err != nil || f.InvalidTokens || n > len(f.log) {
		return nil, "", "", &drive.Error{Kind: drive.KindInvalid, Status: 400, Reason: "invalid_page_token"}
	}
	end := min(n+f.PageSize, len(f.log))
	out := append([]drive.Change(nil), f.log[n:end]...)
	if end < len(f.log) {
		return out, strconv.Itoa(end), "", nil
	}
	return out, "", strconv.Itoa(end), nil
}
