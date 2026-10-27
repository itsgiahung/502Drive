package tui

import (
	"strings"
	"testing"
)

func TestStreamSnapshotRefreshesVisibleDeepScan(t *testing.T) {
	f := newIFake()
	f.deepSteps = []Plan{{ID: "plan_1", Complete: true, Deep: &DeepScan{State: "complete", Complete: true, Files: 80}}}
	m := openPlan(t, f, "en", 100, 40)
	m = sendKeys(t, m, "i")
	m.watching = true
	before := f.count("getplan:")
	next, cmd := m.onSnapshot(snapshotMsg{snap: m.snap})
	m = run(t, next.(Model), cmd)
	if f.count("getplan:") != before+1 || !strings.Contains(m.View(), "Scan finished") {
		t.Fatal(m.View(), f.recorded())
	}
}
