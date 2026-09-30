package local

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/rgomids/axiom/internal/windowsfs"
	"golang.org/x/sys/windows"
)

// Windows does not implement POSIX mode/UID checks. These preliminary metadata
// checks are followed by checkPrivateACL on the same opened object, which proves
// ownership, DACL, reparse-point absence, and a single link for regular files.
const noFollow = 0

func platformEntryName(name string) bool { return windowsfs.ValidComponent(name) }

func forbiddenPermissions(os.FileInfo, os.FileMode) bool { return false }
func ownedByUser(info os.FileInfo) bool                  { return info != nil && info.Mode()&os.ModeSymlink == 0 }
func volumeRoot(path string) string                      { return filepath.VolumeName(path) + string(filepath.Separator) }
func trustedCanonical(path string) (string, error) {
	canonical, err := windowsfs.Canonical(path)
	if errors.Is(err, windowsfs.ErrUnsafe) {
		return "", ErrUnsafe
	}
	return canonical, err
}
func checkPrivateACL(file *os.File) error {
	if windowsfs.Check(file, true) != nil {
		return ErrUnsafe
	}
	return nil
}
func singleLink(file *os.File, _ os.FileInfo) bool { return windowsfs.Check(file, true) == nil }
func safeAncestor(root *os.Root, _ os.FileInfo) bool {
	f, e := root.Open(".")
	if e != nil {
		return false
	}
	defer f.Close()
	return windowsfs.Check(f, false) == nil
}
func mkdirPrivate(root *os.Root, name string) error { return windowsfs.Mkdir(root, name) }
func availableBytes(file *os.File) (uint64, error)  { return windowsfs.Available(file.Name()) }

// File payloads are flushed before publication. Windows has no portable
// directory fsync; the supported contract is process recovery, not power loss.
func syncRoot(root *os.Root) error { _, err := root.Stat("."); return err }
func lockDirectory(root *os.Root, exclusive bool) (*os.File, error) {
	f, err := windowsfs.LockDirectory(root, exclusive)
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return nil, ErrConflict
	}
	return f, err
}
