package cli

import "testing"

func TestCopyModeAnswersAnExistingCopy(t *testing.T) {
	cases := []struct {
		name                      string
		update, newCopy, terminal bool
		want                      string
	}{
		{"script default refreshes", false, false, false, "update"},
		{"terminal asks", false, false, true, ""},
		{"explicit update", true, false, true, "update"},
		{"explicit new", false, true, false, "new"},
	}
	for _, c := range cases {
		if got := copyMode(c.update, c.newCopy, c.terminal); got != c.want {
			t.Errorf("%s: copyMode = %q, want %q", c.name, got, c.want)
		}
	}
}
