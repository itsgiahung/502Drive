package httpapi

import (
	"strings"
	"testing"
)

// A finished copy, an audit over it, a dry-run repair — the whole surface
// against the OpenAPI document.
func TestAuditLifecycleConformsToOpenAPI(t *testing.T) {
	h := newHarness(t, true)
	h.setup()
	src := h.fake.Folder("Project", "root")
	h.fake.File("a.txt", src.ID, 4)

	r := h.do("POST", "/v1/plans", map[string]any{"source": src.ID}, "p1", true)
	if r.status != 201 {
		t.Fatalf("plan %d %s", r.status, r.body)
	}
	plan := h.data(r)
	r = h.do("POST", "/v1/jobs", map[string]any{"plan_id": plan["id"], "expected_revision": plan["revision"]}, "j1", true)
	if r.status != 202 {
		t.Fatalf("submit %d %s", r.status, r.body)
	}
	job := h.data(r)
	done := h.waitState(job["id"].(string), "succeeded", "partial", "failed", "blocked")
	col := done["collection_id"].(string)

	// Admit the audit: 202 + Location, idempotent replay.
	r = h.do("POST", "/v1/collections/"+col+"/audits", nil, "a1", true)
	if r.status != 202 || r.header.Get("Location") == "" {
		t.Fatalf("audit admit %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/audits", r)
	audit := h.data(r)
	r2 := h.do("POST", "/v1/collections/"+col+"/audits", nil, "a1", true)
	if h.data(r2)["id"] != audit["id"] || r2.header.Get("Idempotency-Replayed") != "true" {
		t.Fatal("audit replay")
	}

	auditID := audit["id"].(string)
	h.waitState(audit["job_id"].(string), "succeeded", "partial", "failed", "blocked")
	r = h.do("GET", "/v1/audits/"+auditID, nil, "", true)
	if r.status != 200 || h.data(r)["state"] != "completed" {
		t.Fatalf("audit get %d %s", r.status, r.body)
	}
	h.conform("GET", "/v1/audits/{audit_id}", r)
	if n, ok := h.data(r)["scanned_folders"].(float64); !ok || n < 1 {
		t.Fatalf("scanned folders = %v (%s)", h.data(r)["scanned_folders"], r.body)
	}

	// Findings page (empty tree is clean).
	r = h.do("GET", "/v1/audits/"+auditID+"/findings?limit=10", nil, "", true)
	if r.status != 200 {
		t.Fatalf("findings %d %s", r.status, r.body)
	}
	h.conform("GET", "/v1/audits/{audit_id}/findings", r)

	// The repair needs the audit revision; a stale one is refused.
	got := h.data(h.do("GET", "/v1/audits/"+auditID, nil, "", true))
	revision, ok := got["revision"].(float64)
	if !ok {
		t.Fatalf("audit response has no revision: %s", r.body)
	}
	r = h.do("POST", "/v1/audits/"+auditID+"/repair", map[string]any{"expected_revision": revision + 5, "dry_run": true}, "r0", true)
	if r.status != 409 {
		t.Fatalf("stale repair = %d", r.status)
	}
	h.conform("POST", "/v1/audits/{audit_id}/repair", r)

	r = h.do("POST", "/v1/audits/"+auditID+"/repair", map[string]any{"expected_revision": revision, "dry_run": true}, "r1", true)
	if r.status != 202 || r.header.Get("Location") == "" {
		t.Fatalf("repair admit %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/audits/{audit_id}/repair", r)
	rjob := h.data(r)
	if rjob["kind"] != "repair" || rjob["dry_run"] != true {
		t.Fatalf("repair job = %+v", rjob)
	}
	h.waitState(rjob["id"].(string), "succeeded", "partial", "failed", "blocked")

	// The dry run recorded the plan and bumped the revision again.
	r = h.do("GET", "/v1/audits/"+auditID, nil, "", true)
	final := h.data(r)
	if final["revision"].(float64) != float64(revision+1) {
		t.Fatalf("revision = %v, want %v", final["revision"], float64(revision+1))
	}
	if final["plan"] == nil {
		t.Fatal("the dry run stored no plan")
	}
}

func TestAuditScopesAndErrorPaths(t *testing.T) {
	h := newHarness(t, false)
	h.setup()

	// Missing idempotency key and unknown collection.
	r := h.do("POST", "/v1/collections/col_x/audits", nil, "", true)
	if r.status != 400 {
		t.Fatalf("no key = %d", r.status)
	}
	h.conform("POST", "/v1/collections/{collection_id}/audits", r)
	r = h.do("POST", "/v1/collections/col_x/audits", nil, "k", true)
	if r.status != 404 {
		t.Fatalf("unknown collection = %d", r.status)
	}
	r = h.do("GET", "/v1/audits/aud_x", nil, "", true)
	if r.status != 404 {
		t.Fatalf("unknown audit = %d", r.status)
	}
	r = h.do("GET", "/v1/audits/aud_x/findings?limit=10", nil, "", true)
	if r.status != 404 {
		t.Fatalf("findings of unknown audit = %d", r.status)
	}
	// Bad limit on a known-shaped request is refused before scope work.
	r = h.do("GET", "/v1/audits/aud_x/findings?limit=99999", nil, "", true)
	if r.status != 400 {
		t.Fatalf("bad limit = %d", r.status)
	}
	h.conform("GET", "/v1/audits/{audit_id}/findings", r)

	// Maintenance refuses new audits and repairs.
	h.srv.Maintenance = true
	r = h.do("POST", "/v1/collections/col_x/audits", nil, "k", true)
	if r.status != 503 || !strings.Contains(string(r.body), "maintenance") {
		t.Fatalf("maintenance audit = %d %s", r.status, r.body)
	}
	h.conform("POST", "/v1/collections/{collection_id}/audits", r)
}
