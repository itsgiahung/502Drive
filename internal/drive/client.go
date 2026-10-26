package drive

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	gdrive "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const fileFields = "id,name,mimeType,parents,size,md5Checksum,version,resourceKey,driveId,trashed,shortcutDetails(targetId,targetMimeType,targetResourceKey),capabilities(canCopy,canAddChildren),appProperties,createdTime,modifiedTime,owners(displayName,emailAddress),webViewLink"

// Client is the Google Drive v3 implementation of Provider.
type Client struct{ svc *gdrive.Service }

// NewClient builds a Client from a refreshing token source. endpoint may be
// empty (production) or point at a test server.
func NewClient(ctx context.Context, ts oauth2.TokenSource, endpoint string) (*Client, error) {
	hc := oauth2.NewClient(context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 60 * time.Second}), ts)
	opts := []option.ClientOption{option.WithHTTPClient(hc)}
	if endpoint != "" {
		opts = append(opts, option.WithEndpoint(endpoint))
	}
	svc, err := gdrive.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc}, nil
}

func keyHeader(h http.Header, id, key string) {
	if key != "" {
		h.Set("X-Goog-Drive-Resource-Keys", id+"/"+key)
	}
}

func convert(f *gdrive.File) File {
	out := File{ID: f.Id, Name: f.Name, MimeType: f.MimeType, Parents: f.Parents, MD5: f.Md5Checksum, Version: strconv.FormatInt(f.Version, 10), ResourceKey: f.ResourceKey, DriveID: f.DriveId, Trashed: f.Trashed, AppProperties: f.AppProperties}
	if f.Size > 0 || (f.Size == 0 && f.Md5Checksum != "") {
		s := f.Size
		out.Size = &s
	}
	if f.ShortcutDetails != nil {
		out.ShortcutTargetID, out.ShortcutTargetMime, out.ShortcutTargetKey = f.ShortcutDetails.TargetId, f.ShortcutDetails.TargetMimeType, f.ShortcutDetails.TargetResourceKey
	}
	if f.Capabilities != nil {
		out.CanCopy, out.CanAddChildren = f.Capabilities.CanCopy, f.Capabilities.CanAddChildren
	}
	out.Created, _ = time.Parse(time.RFC3339, f.CreatedTime)
	out.Modified, _ = time.Parse(time.RFC3339, f.ModifiedTime)
	if len(f.Owners) > 0 && f.Owners[0] != nil {
		out.OwnerName, out.OwnerEmail = f.Owners[0].DisplayName, f.Owners[0].EmailAddress
	}
	out.WebLink = f.WebViewLink
	return out
}

// wrap maps API errors into Error without ever including response bodies.
func wrap(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: KindUnavailable, Err: err}
	}
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Op == "dial" {
		return &Error{Kind: KindUnreachable, Err: err}
	}
	var ge *googleapi.Error
	if !errors.As(err, &ge) {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) || errors.Is(err, ErrReconnectRequired) || errors.Is(err, ErrNotConnected) {
			return &Error{Kind: KindAuth, Err: err}
		}
		return &Error{Kind: KindUnavailable, Err: err}
	}
	e := &Error{Status: ge.Code, Err: err}
	if len(ge.Errors) > 0 {
		e.Reason = ge.Errors[0].Reason
	}
	if ra := ge.Header.Get("Retry-After"); ra != "" {
		if n, perr := strconv.Atoi(ra); perr == nil {
			e.RetryAfter = time.Duration(n) * time.Second
		}
	}
	switch {
	case ge.Code == 401:
		e.Kind = KindAuth
	case ge.Code == 404:
		e.Kind = KindNotFound
	case ge.Code == 429, ge.Code == 403 && (e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded"):
		e.Kind = KindThrottled
	case ge.Code == 403 && (e.Reason == "storageQuotaExceeded" || e.Reason == "dailyLimitExceeded" || strings.Contains(e.Reason, "uota")):
		e.Kind = KindQuota
	case ge.Code == 403:
		e.Kind = KindPermission
	case ge.Code >= 500:
		e.Kind = KindUnavailable
	default:
		e.Kind = KindInvalid
	}
	return e
}

func (c *Client) Account(ctx context.Context) (Account, error) {
	a, err := c.svc.About.Get().Fields("user(emailAddress,displayName)").Context(ctx).Do()
	if err != nil {
		return Account{}, wrap(err)
	}
	if a.User == nil {
		return Account{}, &Error{Kind: KindInvalid, Reason: "no_user"}
	}
	return Account{Email: a.User.EmailAddress, Name: a.User.DisplayName}, nil
}

func (c *Client) GetFile(ctx context.Context, id, resourceKey string) (File, error) {
	call := c.svc.Files.Get(id).SupportsAllDrives(true).Fields(fileFields).Context(ctx)
	keyHeader(call.Header(), id, resourceKey)
	f, err := call.Do()
	if err != nil {
		return File{}, wrap(err)
	}
	return convert(f), nil
}

func (c *Client) ListChildren(ctx context.Context, folderID, resourceKey, pageToken string) ([]File, string, error) {
	call := c.svc.Files.List().
		Q(fmt.Sprintf("'%s' in parents and trashed=false", QueryEscape(folderID))).
		Corpora("allDrives").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).
		PageSize(200).OrderBy("folder,name").Fields("nextPageToken,files(" + fileFields + ")").Context(ctx)
	keyHeader(call.Header(), folderID, resourceKey)
	if pageToken != "" {
		call.PageToken(pageToken)
	}
	r, err := call.Do()
	if err != nil {
		return nil, "", wrap(err)
	}
	out := make([]File, 0, len(r.Files))
	for _, f := range r.Files {
		out = append(out, convert(f))
	}
	return out, r.NextPageToken, nil
}

func intentProps(intent string) map[string]string {
	if intent == "" {
		return nil
	}
	return map[string]string{IntentProperty: intent}
}

func (c *Client) CreateFolder(ctx context.Context, name, parentID, intent string) (File, error) {
	f, err := c.svc.Files.Create(&gdrive.File{Name: name, MimeType: MimeFolder, Parents: []string{parentID}, AppProperties: intentProps(intent)}).
		SupportsAllDrives(true).Fields(fileFields).Context(ctx).Do()
	if err != nil {
		return File{}, wrap(err)
	}
	return convert(f), nil
}

func (c *Client) CopyFile(ctx context.Context, src File, parentID, intent string) (File, error) {
	call := c.svc.Files.Copy(src.ID, &gdrive.File{Name: src.Name, Parents: []string{parentID}, AppProperties: intentProps(intent)}).
		SupportsAllDrives(true).Fields(fileFields).Context(ctx)
	keyHeader(call.Header(), src.ID, src.ResourceKey)
	f, err := call.Do()
	if err != nil {
		return File{}, wrap(err)
	}
	return convert(f), nil
}

func (c *Client) CreateShortcut(ctx context.Context, name, parentID, targetID, targetKey, intent string) (File, error) {
	call := c.svc.Files.Create(&gdrive.File{Name: name, MimeType: MimeShortcut, Parents: []string{parentID}, AppProperties: intentProps(intent),
		ShortcutDetails: &gdrive.FileShortcutDetails{TargetId: targetID}}).SupportsAllDrives(true).Fields(fileFields).Context(ctx)
	keyHeader(call.Header(), targetID, targetKey)
	f, err := call.Do()
	if err != nil {
		return File{}, wrap(err)
	}
	return convert(f), nil
}

func (c *Client) FindByIntent(ctx context.Context, parentID, intent string) ([]File, error) {
	q := fmt.Sprintf("'%s' in parents and trashed=false and appProperties has { key='%s' and value='%s' }", QueryEscape(parentID), IntentProperty, QueryEscape(intent))
	var out []File
	token := ""
	for {
		call := c.svc.Files.List().Q(q).Corpora("allDrives").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).
			PageSize(10).Fields("nextPageToken,files(" + fileFields + ")").Context(ctx)
		if token != "" {
			call.PageToken(token)
		}
		r, err := call.Do()
		if err != nil {
			return nil, wrap(err)
		}
		for _, f := range r.Files {
			out = append(out, convert(f))
		}
		if token = r.NextPageToken; token == "" || len(out) > 20 {
			return out, nil
		}
	}
}

func (c *Client) RenameFile(ctx context.Context, id, name string) error {
	_, err := c.svc.Files.Update(id, &gdrive.File{Name: name}).SupportsAllDrives(true).Fields("id").Context(ctx).Do()
	return wrap(err)
}

func (c *Client) MoveFile(ctx context.Context, id, fromParent, toParent string) error {
	_, err := c.svc.Files.Update(id, &gdrive.File{}).AddParents(toParent).RemoveParents(fromParent).SupportsAllDrives(true).Fields("id").Context(ctx).Do()
	return wrap(err)
}

func (c *Client) TrashFile(ctx context.Context, id string) error {
	_, err := c.svc.Files.Update(id, &gdrive.File{Trashed: true}).SupportsAllDrives(true).Fields("id").Context(ctx).Do()
	return wrap(err)
}

func (c *Client) StartPageToken(ctx context.Context) (string, error) {
	r, err := c.svc.Changes.GetStartPageToken().SupportsAllDrives(true).Context(ctx).Do()
	if err != nil {
		return "", wrap(err)
	}
	return r.StartPageToken, nil
}

func (c *Client) ListChanges(ctx context.Context, pageToken string) ([]Change, string, string, error) {
	r, err := c.svc.Changes.List(pageToken).SupportsAllDrives(true).IncludeItemsFromAllDrives(true).IncludeRemoved(true).
		PageSize(200).Fields("nextPageToken,newStartPageToken,changes(fileId,removed,file(" + fileFields + "))").Context(ctx).Do()
	if err != nil {
		return nil, "", "", wrap(err)
	}
	out := make([]Change, 0, len(r.Changes))
	for _, ch := range r.Changes {
		x := Change{FileID: ch.FileId, Removed: ch.Removed}
		if ch.File != nil {
			f := convert(ch.File)
			x.File = &f
			x.Removed = x.Removed || f.Trashed
		}
		out = append(out, x)
	}
	return out, r.NextPageToken, r.NewStartPageToken, nil
}
