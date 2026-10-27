//go:build windows

package app

import (
	"golang.org/x/sys/windows"
)

func defaultDiskSpace(path string) (uint64, uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	err = windows.GetDiskFreeSpaceEx(p, &freeBytesAvailable, &totalBytes, &totalFreeBytes)
	return freeBytesAvailable, totalBytes, err
}

func defaultFDLimit() (uint64, uint64, error) {
	return 2048, 2048, nil
}
