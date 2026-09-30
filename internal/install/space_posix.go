//go:build !windows

package install

import "golang.org/x/sys/unix"

const binaryName = "axiom"

func supportedWindowsHost() bool { return false }

func statfsAvailable(path string) (uint64, error) {
	var filesystem unix.Statfs_t
	if err := unix.Statfs(path, &filesystem); err != nil {
		return 0, err
	}
	return uint64(filesystem.Bavail) * uint64(filesystem.Bsize), nil
}
