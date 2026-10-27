package tui

import (
	"context"
	"strings"
	"testing"
)

func TestAuditBrowserFiltersAndPages(t *testing.T) {
	var queries []string
	acts := Actions{AuditFindings: func(_ context.Context, id, kind, cursor string) (FindingPage, error) {
		queries = append(queries, kind+":"+cursor)
		return FindingPage{Items: []Finding{{Kind: "missing_at_destination", Detail: "synthetic/report.pdf", Repairable: true}}, Next: "42"}, nil
	}}
	m := New(acts, Options{Language: "vi", NoMotion: true, Width: 110})
	m.snap.Collections = []Collection{{ID: "c"}}
	m.screen = screenCollection
	m.audits["c"] = &auditState{report: &AuditReport{ID: "a", State: "completed", Revision: 2, Repairable: 1}}
	m, cmd := m.openAuditBrowser(m.snap.Collections[0])
	m = run(t, m, cmd)
	if !strings.Contains(m.View(), "synthetic/report.pdf") {
		t.Fatal(m.View())
	}
	t.Log("U5 snapshot:\n" + m.View())
	m = sendKeys(t, m, "n", "t")
	if strings.Join(queries, ",") != ":,:42,missing_at_destination:" {
		t.Fatal(queries)
	}
}
func TestFailedDryRunNeverEnablesApply(t *testing.T) {
	m := New(Actions{}, Options{})
	m.snap.Jobs = []Job{{ID: "repair", State: "failed"}}
	m.audits["c"] = &auditState{repairJob: "repair", repairDry: true}
	next, _ := m.onAudit(auditMsg{collectionID: "c", rep: AuditReport{ID: "a", State: "completed"}})
	m = next.(Model)
	if m.audits["c"].planned {
		t.Fatal("failed dry run authorized apply")
	}
}
