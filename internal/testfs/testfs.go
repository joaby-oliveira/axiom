// Package testfs supplies host-aware filesystem assertions for tests.
package testfs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Path turns a synthetic POSIX absolute fixture into a host absolute path.
func Path(path string) string {
	if runtime.GOOS != "windows" {
		return path
	}
	absolute, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		panic(err)
	}
	return absolute
}

func POSIXModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode contract; Windows ACL contract has native tests")
	}
}

func POSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture; native executable workflow has separate black-box coverage")
	}
}

func Symlink(t *testing.T, target, link string) error {
	t.Helper()
	err := os.Symlink(target, link)
	if symlinkPrivilegeMissing(err) {
		t.Skip("Windows symlink privilege unavailable; junction refusal is tested separately")
	}
	return err
}

// RenameOrSkipPinned permits the host to prevent the replacement attack itself.
func RenameOrSkipPinned(t *testing.T, oldPath, newPath string) error {
	t.Helper()
	err := os.Rename(oldPath, newPath)
	if pinnedDirectory(err) {
		t.Skip("Windows pins this open directory and refuses the attempted replacement")
	}
	return err
}
