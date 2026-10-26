//go:build windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	createNoWindow        = 0x08000000
)

// spawnDetached starts argv without a console window and outside the launching
// console's process group, so closing that console does not stop it.
func spawnDetached(argv []string, logPath string) error {
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess | createNoWindow, HideWindow: true}
	if err = cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
