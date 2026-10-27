package drive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestRestoreFileSendsExplicitFalseAndSharedDriveSupport(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.URL.Query().Get("supportsAllDrives") != "true" {
			t.Errorf("request %s %s", r.Method, r.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if v, ok := body["trashed"]; !ok || v != false {
			t.Errorf("restore payload %v", body)
		}
		fmt.Fprint(w, `{"id":"owned"}`)
	})
	if err := c.RestoreFile(t.Context(), "owned"); err != nil {
		t.Fatal(err)
	}
}
