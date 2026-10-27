//go:build windows

package tray

import (
	"os/exec"
	"syscall"
)

// Windows process creation flags: no console window for the tray-spawned
// daemon or terminal.
const (
	createNoWindow    = 0x08000000
	detachedProcess   = 0x00000008
	startDetachedFlag = createNoWindow | detachedProcess
)

// startDetached launches argv without waiting for it and without a console.
func startDetached(argv []string) error {
	if len(argv) == 0 {
		return errNoCommand
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: startDetachedFlag, HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func init() { consoleStarter = startConsole }
func startConsole(argv []string) error {
	if len(argv) == 0 {
		return errNoCommand
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
