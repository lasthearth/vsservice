//go:build unix

package service

import "syscall"

// freeBytes is the space an unprivileged process may still write in dir.
func freeBytes(dir string) (int64, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, false
	}

	//nolint:gosec,unconvert // G115: block counts on a real disk fit in int64.
	// unconvert: Bsize is uint32 on darwin and int64 on linux, so the conversion is
	// required on one GOOS and redundant on the other. Lint runs on the host GOOS.
	return int64(stat.Bavail) * int64(stat.Bsize), true
}
