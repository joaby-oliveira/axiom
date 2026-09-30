//go:build !windows

package testfs

import "os"

func symlinkPrivilegeMissing(error) bool             { return false }
func pinnedDirectory(error) bool                     { return false }
func SharedMode(path string, mode os.FileMode) error { return os.Chmod(path, mode) }
func PrivateMode(path string, mode os.FileMode) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().Perm() == mode
}
