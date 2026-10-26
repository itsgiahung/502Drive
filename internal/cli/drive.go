package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/itsgiahung/502Drive/internal/drive"
)

// todayFunc lets tests pin the date used by --auto-name.
var todayFunc = time.Now

// drivePath is one breadcrumb entry from /v1/drive/folders.
type drivePath struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// driveFolderPage is one page of GET /v1/drive/folders/{id}.
type driveFolderPage struct {
	Folder driveFileView   `json:"folder"`
	Path   []drivePath     `json:"path"`
	Items  []driveFileView `json:"items"`
	Next   *string         `json:"next_page_token"`
}

// driveRef resolves a Folder argument: empty or "root" means My Drive;
// anything else must be one Drive link or ID.
func driveRef(input string) (id, key string, err error) {
	s := strings.TrimSpace(input)
	if s == "" || strings.EqualFold(s, "root") {
		return "root", "", nil
	}
	ref, perr := drive.Parse(s)
	if perr != nil {
		return "", "", usage("expected one Drive link, Drive ID or \"root\"")
	}
	return ref.ID, ref.Key, nil
}

// ls lists one folder. Folders come first, then files, each sorted by name.
func (a *App) lsCmd() *cobra.Command {
	var all bool
	c := &cobra.Command{Use: "ls [FOLDER]", Short: "List a Drive folder (empty or root = My Drive)",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := "root"
			if len(args) == 1 {
				input = args[0]
			}
			id, key, err := driveRef(input)
			if err != nil {
				return err
			}
			page, r, err := a.browseFolder(cmd.Context(), id, key, "")
			if err != nil {
				return err
			}
			if !all {
				a.emit(r, "", func() { a.printFolder(page) })
				return nil
			}
			merged := page
			for pages := 0; merged.Next != nil && *merged.Next != ""; pages++ {
				if pages >= 200 {
					return localErr(ExitUnavailable, "pagination_limit", "The folder has more than 200 pages; use the TUI to browse it.")
				}
				next, nextResp, err := a.browseFolder(cmd.Context(), id, key, *merged.Next)
				if err != nil {
					return err
				}
				merged.Items = append(merged.Items, next.Items...)
				merged.Next = next.Next
				r = nextResp
			}
			a.emitMergedFolder(r, merged)
			return nil
		}}
	c.Flags().BoolVar(&all, "all", false, "read every page (follows next_page_token)")
	return c
}

// browseFolder fetches one page. The breadcrumb is only returned with the
// first page, so later pages leave Path empty.
func (a *App) browseFolder(ctx context.Context, id, key, pageToken string) (driveFolderPage, *Response, error) {
	var out driveFolderPage
	q := url.Values{}
	if key != "" {
		q.Set("resource_key", key)
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	r, err := a.call(ctx, "GET", "/v1/drive/folders/"+url.PathEscape(id), q, nil, "")
	if err != nil {
		return out, nil, err
	}
	if err = r.Decode(&out); err != nil {
		return out, nil, err
	}
	return out, r, nil
}

// emitMergedFolder prints the merged result of --all: one JSON envelope with
// every page's items, or the human table.
func (a *App) emitMergedFolder(r *Response, page driveFolderPage) {
	if a.g.json {
		b, err := json.Marshal(map[string]any{
			"data": map[string]any{
				"folder":          page.Folder,
				"path":            page.Path,
				"items":           page.Items,
				"next_page_token": nil,
			},
			"request_id": r.RequestID,
		})
		if err != nil {
			fmt.Fprintln(a.errw(), a.style.bad("error:"), err.Error())
			return
		}
		fmt.Fprintln(a.out(), string(b))
		return
	}
	a.printFolder(page)
}

// printFolder renders the breadcrumb and one row per item. Every Drive name
// goes through Sanitize; width padding is measured on plain text.
func (a *App) printFolder(page driveFolderPage) {
	if len(page.Path) > 0 {
		names := make([]string, 0, len(page.Path))
		for _, p := range page.Path {
			names = append(names, Sanitize(p.Name))
		}
		fmt.Fprintln(a.out(), strings.Join(names, " / "))
	}
	items := append([]driveFileView(nil), page.Items...)
	sort.SliceStable(items, func(i, j int) bool {
		fi, fj := items[i].Kind == "folder", items[j].Kind == "folder"
		if fi != fj {
			return fi
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	for _, it := range items {
		name := Sanitize(it.Name)
		size := formatBytes(it.Size)
		if it.Kind == "folder" {
			name += "/"
			size = "-"
		}
		sizeCell := fmt.Sprintf("%8s", size)
		dateCell := fmt.Sprintf("%-10s", formatDriveDate(it.ModifiedAt))
		shown := name
		if it.Kind == "folder" {
			shown = a.style.bold(name)
		}
		fmt.Fprintf(a.out(), "%s  %s  %s\n", a.style.dim(sizeCell), a.style.dim(dateCell), shown)
	}
}

// formatBytes renders a Drive size as B/KB/MB; unknown sizes are "-".
func formatBytes(size *int64) string {
	if size == nil || *size < 0 {
		return "-"
	}
	n := *size
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

// formatDriveDate renders an RFC3339 timestamp as YYYY-MM-DD.
func formatDriveDate(raw *string) string {
	if raw == nil || *raw == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, *raw)
	if err != nil {
		return "-"
	}
	return t.Format("2006-01-02")
}

// info prints the display metadata of one Drive item.
func (a *App) infoCmd() *cobra.Command {
	return &cobra.Command{Use: "info LINK|ID", Short: "Show Drive metadata for one file or folder",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, key, err := driveRef(args[0])
			if err != nil {
				return err
			}
			q := url.Values{}
			if key != "" {
				q.Set("resource_key", key)
			}
			r, err := a.call(cmd.Context(), "GET", "/v1/drive/files/"+url.PathEscape(id), q, nil, "")
			if err != nil {
				return err
			}
			var f driveFileView
			if err = r.Decode(&f); err != nil {
				return err
			}
			a.emit(r, Sanitize(f.ID), func() { a.printDriveInfo(f) })
			return nil
		}}
}

func (a *App) printDriveInfo(f driveFileView) {
	owner := "-"
	if f.Owner != nil {
		owner = strings.TrimSpace(f.Owner.Name)
		if f.Owner.Email != "" {
			owner = strings.TrimSpace(owner + " <" + f.Owner.Email + ">")
		}
	}
	link := f.WebURL
	if link == "" {
		link = "-"
	}
	kind := f.Kind
	if kind == "" {
		kind = "-"
	}
	printKV(a.out(), a.style, []kv{
		{"kind", Sanitize(kind)},
		{"size", formatBytes(f.Size)},
		{"modified", formatDriveDate(f.ModifiedAt)},
		{"created", formatDriveDate(f.CreatedAt)},
		{"owner", Sanitize(owner)},
		{"can copy", yesNo(f.CanCopy)},
		{"can add children", yesNo(f.CanWrite)},
		{"id", Sanitize(f.ID)},
		{"link", Sanitize(link)},
	})
}

// mkdir creates a folder in Drive with an idempotency key.
func (a *App) mkdirCmd() *cobra.Command {
	var parent, key string
	c := &cobra.Command{Use: "mkdir NAME", Short: "Create a Drive folder", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if strings.TrimSpace(name) == "" {
				return usage("a folder name is required")
			}
			parentID, _, err := driveRef(parent)
			if err != nil {
				return usage("--in must be one Drive link, Drive ID or \"root\"")
			}
			if key == "" {
				key = NewKey()
			}
			r, err := a.call(cmd.Context(), "POST", "/v1/drive/folders", nil,
				map[string]any{"parent_id": parentID, "name": name}, key)
			if err != nil {
				return err
			}
			var f driveFileView
			if err = r.Decode(&f); err != nil {
				return err
			}
			a.emit(r, Sanitize(f.ID), func() {
				printKV(a.out(), a.style, []kv{
					{"id", Sanitize(f.ID)},
					{"link", Sanitize(f.WebURL)},
				})
			})
			return nil
		}}
	f := c.Flags()
	f.StringVar(&parent, "in", "", "parent folder: link, ID or root (default root)")
	f.StringVar(&key, "idempotency-key", "", "explicit key for this create (default: a random key)")
	return c
}

// autoCopyName is the default folder name for --auto-name.
func autoCopyName(source string) string {
	return fmt.Sprintf("%s (copy %s)", source, todayFunc().Format("2006-01-02"))
}
