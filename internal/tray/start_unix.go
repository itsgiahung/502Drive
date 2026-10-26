//go:build !windows

package tray

import "os/exec"

// startDetached launches argv without waiting for it.
func startDetached(argv []string) error {
	if len(argv) == 0 {
		return errNoCommand
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
