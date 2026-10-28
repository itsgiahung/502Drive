package i18n

import "testing"

func TestErrorTextsExistInBothLanguages(t *testing.T) {
	for code := range errorsVI {
		if _, ok := errorsEN[code]; !ok {
			t.Errorf("%s has no English text", code)
		}
	}
	for code := range errorsEN {
		if _, ok := errorsVI[code]; !ok {
			t.Errorf("%s has no Vietnamese text", code)
		}
	}
	if s, ok := ErrorText("vi", "invalid_transition"); !ok || s == "" {
		t.Fatal("invalid_transition needs a text")
	}
}
