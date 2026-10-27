package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t", TokenType: "Bearer"}), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRequestsCarryResourceKeysSharedDriveFlagsAndIntentTags(t *testing.T) {
	var got []*http.Request
	var bodies []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, r.Clone(context.Background()))
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"new1","name":"n","mimeType":"application/octet-stream","files":[],"version":"3"}`)
	})
	ctx := context.Background()
	if _, err := c.GetFile(ctx, "SRCID123456", "rk_1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ListChildren(ctx, "FOLDER'ID12", "rk_2", "PAGE2"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CopyFile(ctx, File{ID: "SRCID123456", Name: "doc", ResourceKey: "rk_1"}, "PARENT12345", "intent-abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateFolder(ctx, "dir", "PARENT12345", "intent-def"); err != nil {
		t.Fatal(err)
	}

	if g := got[0]; g.Header.Get("X-Goog-Drive-Resource-Keys") != "SRCID123456/rk_1" || g.URL.Query().Get("supportsAllDrives") != "true" {
		t.Errorf("get: %v %v", g.Header, g.URL.RawQuery)
	}
	q := got[1].URL.Query()
	if !strings.Contains(q.Get("q"), `'FOLDER\'ID12' in parents`) || !strings.Contains(q.Get("q"), "trashed=false") {
		t.Errorf("list query not escaped: %q", q.Get("q"))
	}
	if q.Get("pageToken") != "PAGE2" || q.Get("corpora") != "allDrives" || q.Get("includeItemsFromAllDrives") != "true" || got[1].Header.Get("X-Goog-Drive-Resource-Keys") != "FOLDER'ID12/rk_2" {
		t.Errorf("list params: %v %v", q, got[1].Header)
	}
	if got[2].Method != "POST" || !strings.HasSuffix(got[2].URL.Path, "/files/SRCID123456/copy") || got[2].Header.Get("X-Goog-Drive-Resource-Keys") != "SRCID123456/rk_1" {
		t.Errorf("copy request: %s %s", got[2].Method, got[2].URL.Path)
	}
	var body map[string]any
	json.Unmarshal([]byte(bodies[2]), &body)
	if body["name"] != "doc" || fmt.Sprint(body["parents"]) != "[PARENT12345]" || fmt.Sprint(body["appProperties"]) != "map["+IntentProperty+":intent-abc]" {
		t.Errorf("copy body: %v", body)
	}
	json.Unmarshal([]byte(bodies[3]), &body)
	if body["mimeType"] != MimeFolder {
		t.Errorf("folder body: %v", body)
	}
	for _, r := range got {
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Error("missing bearer")
		}
	}
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		status  int
		reason  string
		retry   string
		want    Kind
		retryOK time.Duration
	}{
		{429, "", "7", KindThrottled, 7 * time.Second},
		{403, "rateLimitExceeded", "", KindThrottled, 0},
		{403, "userRateLimitExceeded", "", KindThrottled, 0},
		{403, "storageQuotaExceeded", "", KindQuota, 0},
		{403, "forbidden", "", KindPermission, 0},
		{401, "", "", KindAuth, 0},
		{404, "", "", KindNotFound, 0},
		{500, "", "", KindServer, 0},
		{503, "", "", KindServer, 0},
		{400, "invalid", "", KindInvalid, 0},
	}
	for _, tc := range cases {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if tc.retry != "" {
				w.Header().Set("Retry-After", tc.retry)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tc.status)
			fmt.Fprintf(w, `{"error":{"code":%d,"message":"secret-body-text","errors":[{"reason":%q}]}}`, tc.status, tc.reason)
		})
		_, err := c.GetFile(context.Background(), "ID12345678", "")
		if KindOf(err) != tc.want {
			t.Errorf("%d/%s: kind %s want %s (%v)", tc.status, tc.reason, KindOf(err), tc.want, err)
		}
		if RetryAfterOf(err) != tc.retryOK {
			t.Errorf("%d: retry-after %v", tc.status, RetryAfterOf(err))
		}
		if strings.Contains(err.Error(), "secret-body-text") {
			t.Error("response body leaked into the error text")
		}
	}
	// nothing listening: the write was never sent, so it is safe to retry
	c, _ := NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}), "http://127.0.0.1:1")
	_, err := c.CreateFolder(context.Background(), "x", "P", "k")
	if KindOf(err) != KindUnreachable || !NotExecuted(err) {
		t.Errorf("dial failure: %v / %s", err, KindOf(err))
	}
	// a 5xx on a write is NOT proof that nothing happened
	if NotExecuted(&Error{Kind: KindUnavailable}) {
		t.Error("5xx must be treated as unknown outcome")
	}
}

func TestShortcutAndMetadataConversion(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"S1","name":"link","mimeType":"application/vnd.google-apps.shortcut","shortcutDetails":{"targetId":"T1","targetMimeType":"application/pdf","targetResourceKey":"rk"},"capabilities":{"canCopy":true,"canAddChildren":false},"version":"9"}`)
	})
	f, err := c.GetFile(context.Background(), "S1", "")
	if err != nil || !f.IsShortcut() || f.ShortcutTargetID != "T1" || f.ShortcutTargetKey != "rk" || !f.CanCopy || f.CanAddChildren || f.Version != "9" {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestBatchListUsesOneQueryWithMinimalFields(t *testing.T) {
	var got *http.Request
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"nextPageToken":"NEXT","files":[{"id":"f1","name":"a.txt","mimeType":"text/plain","parents":["A1"],"version":"2"},{"id":"f2","name":"b","mimeType":"application/vnd.google-apps.folder","parents":["B2"]}]}`)
	})
	files, next, err := c.ListChildrenBatch(context.Background(), []Parent{{ID: "A'1", ResourceKey: "k1"}, {ID: "B2", ResourceKey: "k2"}, {ID: "C3"}}, "P1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].ID != "f1" || files[1].Name != "b" || next != "NEXT" {
		t.Fatalf("files=%+v next=%q", files, next)
	}
	q := got.URL.Query()
	if q.Get("q") != `('A\'1' in parents or 'B2' in parents or 'C3' in parents) and trashed=false` {
		t.Fatalf("q = %q", q.Get("q"))
	}
	if q.Get("pageSize") != "1000" || q.Get("pageToken") != "P1" {
		t.Fatalf("paging = %v", q)
	}
	fields := q.Get("fields")
	if !strings.Contains(fields, "files(id,name,mimeType,parents,") || strings.Contains(fields, "owners") || strings.Contains(fields, "webViewLink") || strings.Contains(fields, "createdTime") {
		t.Fatalf("fields = %q", fields)
	}
	if keys := got.Header.Get("X-Goog-Drive-Resource-Keys"); keys != "A'1/k1,B2/k2" {
		t.Fatalf("keys = %q", keys)
	}
}

func TestBatchListRejectsMoreThanFortyParents(t *testing.T) {
	called := false
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	parents := make([]Parent, MaxBatchParents+1)
	for i := range parents {
		parents[i] = Parent{ID: fmt.Sprintf("p%d", i)}
	}
	if _, _, err := c.ListChildrenBatch(context.Background(), parents, ""); KindOf(err) != KindInvalid {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Fatal("an oversized batch was sent")
	}
}

// Every Files and Changes call must carry the shared-drive flags; a single
// missed call would make items in a Shared Drive invisible or unwritable.
func TestEveryFilesCallSendsSharedDriveFlags(t *testing.T) {
	var got []*http.Request
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Clone(context.Background()))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"x1","name":"n","mimeType":"application/octet-stream","files":[],"version":"3","changes":[],"startPageToken":"7","newStartPageToken":"8"}`)
	})
	ctx := context.Background()
	src := File{ID: "SRC1", Name: "doc", ResourceKey: "rk"}
	steps := []func() error{
		func() error { _, err := c.GetFile(ctx, "SRC1", "rk"); return err },
		func() error { _, _, err := c.ListChildren(ctx, "F1", "", ""); return err },
		func() error { _, _, err := c.ListChildrenBatch(ctx, []Parent{{ID: "F1"}, {ID: "F2"}}, ""); return err },
		func() error { _, err := c.CreateFolder(ctx, "d", "P1", "i1"); return err },
		func() error { _, err := c.CopyFile(ctx, src, "P1", "i2"); return err },
		func() error { _, err := c.CreateShortcut(ctx, "s", "P1", "T1", "", "i3"); return err },
		func() error { _, err := c.FindByIntent(ctx, "P1", "i2"); return err },
		func() error { _, err := c.FindChildrenByName(ctx, "P1", "doc"); return err },
		func() error { return c.RenameFile(ctx, "F1", "n2") },
		func() error { return c.MoveFile(ctx, "F1", "P1", "P2") },
		func() error { return c.TrashFile(ctx, "F1") },
		func() error { _, err := c.StartPageToken(ctx); return err },
		func() error { _, _, _, err := c.ListChanges(ctx, "7"); return err },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	if len(got) < len(steps) {
		t.Fatalf("requests = %d, want at least %d", len(got), len(steps))
	}
	for _, r := range got {
		q := r.URL.Query()
		if !strings.Contains(r.URL.Path, "/files") && !strings.Contains(r.URL.Path, "/changes") {
			continue
		}
		if q.Get("supportsAllDrives") != "true" {
			t.Errorf("%s %s without supportsAllDrives=true", r.Method, r.URL.Path)
		}
		if r.Method == "GET" && (strings.HasSuffix(r.URL.Path, "/files") || strings.HasSuffix(r.URL.Path, "/changes")) && q.Get("includeItemsFromAllDrives") != "true" {
			t.Errorf("%s %s without includeItemsFromAllDrives=true", r.Method, r.URL.Path)
		}
	}
}
