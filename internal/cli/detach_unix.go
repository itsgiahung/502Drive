//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// spawnDetached starts argv in its own session, without a terminal, with its
// output appended to logPath. Closing the terminal that launched it (SIGHUP)
// therefore cannot reach it.
func spawnDetached(argv []string, logPath string) error {
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
