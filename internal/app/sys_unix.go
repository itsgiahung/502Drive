//go:build !windows

package app

import (
	"golang.org/x/sys/unix"
)

func defaultDiskSpace(path string) (uint64, uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	free := stat.Bavail * uint64(stat.Bsize)
	total := stat.Blocks * uint64(stat.Bsize)
	return free, total, nil
}

func defaultFDLimit() (uint64, uint64, error) {
	var rlim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rlim); err != nil {
		return 0, 0, err
	}
	return rlim.Cur, rlim.Max, nil
}
