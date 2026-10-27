package telegram

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestFakeBotAPIIgnoresForeignPortProbes guards the harness itself: the fake
// listens on a real localhost port, and other local tools probe listening
// ports (opencode asks /api/mcp/install-info). Such a request is not a call
// from this bot, so it must not fail the test that happens to own that port.
func TestFakeBotAPIIgnoresForeignPortProbes(t *testing.T) {
	f := newFakeAPI(t)

	res, err := http.Get(f.srv.URL + "/api/mcp/install-info")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign probe status = %d", res.StatusCode)
	}

	// A real Bot API call on the same server still gets an answer.
	body, err := http.Post(f.srv.URL+"/bot"+f.token+"/getMe", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer body.Body.Close()
	raw, err := io.ReadAll(body.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"ok":true`) {
		t.Fatalf("getMe answer = %s", raw)
	}
}
