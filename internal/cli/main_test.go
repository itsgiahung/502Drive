package cli

import (
	"errors"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	openSetupPage = func(string) error { return errors.New("browser disabled during tests") }
	os.Exit(m.Run())
}
