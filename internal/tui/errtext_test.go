package tui

import (
	"errors"
	"strings"
	"testing"
)

type codedErr struct{ code, msg string }

func (e codedErr) Error() string   { return e.msg }
func (e codedErr) ErrCode() string { return e.code }

func TestErrorsAreShownInPlainLanguageByCode(t *testing.T) {
	d := newDriveFake()
	m := driveModel(t, d, 100, 30)
	m.lang = "vi"
	got := m.errText(codedErr{"invalid_transition", "That action is not available in the current state."})
	if strings.Contains(got, "That action") || !strings.Contains(got, "chưa dùng được") {
		t.Fatalf("the Vietnamese UI must not show the English sentence: %q", got)
	}
	m.lang = "en"
	if got = m.errText(codedErr{"stale_revision", "x"}); !strings.Contains(got, "changed elsewhere") {
		t.Fatalf("english text = %q", got)
	}
	if got = m.errText(errors.New("disk full")); got != "disk full" {
		t.Fatalf("an unknown error keeps its message: %q", got)
	}
}
