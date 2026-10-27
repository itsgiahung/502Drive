package filter

import "testing"

func TestSizeRejectsOverflow(t *testing.T) {
	if _, err := ParseSize("9223372036854775807TB"); err == nil {
		t.Fatal("overflow accepted")
	}
}
