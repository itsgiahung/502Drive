package tui

import (
	"strings"
	"testing"
)

func TestPlainErrorJobSnapshot(t *testing.T) {
	m := New(Actions{}, Options{Language: "vi", NoMotion: true, Width: 110})
	m.screen = screenJob
	m.snap.Jobs = []Job{{ID: "job_synthetic", SourceName: "Tài liệu mẫu", State: "blocked", LastError: "synthetic", LastErrorCode: "unknown_remote_outcome"}}
	v := m.View()
	if !strings.Contains(v, "đối soát") || !strings.Contains(v, "chưa xác nhận") {
		t.Fatal(v)
	}
	t.Log("U3 snapshot:\n" + v)
}
