package codexruntime

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/rgomids/axiom/internal/windowsfs"
	"golang.org/x/sys/windows"
)

const noFollow = 0

// Final ownership, permissions and link count are checked on the open handle.
func forbiddenPermissions(os.FileInfo, os.FileMode) bool { return false }
func ownedByUser(info os.FileInfo) bool                  { return info != nil && info.Mode()&os.ModeSymlink == 0 }
func privateRegularInfo(info os.FileInfo) bool           { return info != nil && info.Mode().IsRegular() }
func volumeRoot(path string) string                      { return filepath.VolumeName(path) + string(filepath.Separator) }
func trustedCanonical(path string) (string, error)       { return windowsfs.Canonical(path) }
func checkPrivateACL(file *os.File) error                { return windowsfs.Check(file, true) }
func safeAncestor(root *os.Root, _ os.FileInfo) bool {
	f, e := root.Open(".")
	if e != nil {
		return false
	}
	defer f.Close()
	return windowsfs.Check(f, false) == nil
}
func mkdirPrivate(root *os.Root, name string) error { return windowsfs.Mkdir(root, name) }
func lockFile(file *os.File) error                  { return windowsfs.LockFile(file) }
func lockContended(err error) bool                  { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }
func availableBytes(file *os.File) (uint64, error)  { return windowsfs.Available(file.Name()) }
