//go:build !windows

package local

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const noFollow = unix.O_NOFOLLOW

func platformEntryName(string) bool { return true }

func volumeRoot(string) string { return string(filepath.Separator) }
func forbiddenPermissions(info os.FileInfo, mask os.FileMode) bool {
	return info.Mode().Perm()&mask != 0
}
func safeAncestor(_ *os.Root, info os.FileInfo) bool { return ancestorSafe(info) }
func mkdirPrivate(root *os.Root, name string) error  { return root.Mkdir(name, 0o700) }
func singleLink(_ *os.File, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
func availableBytes(file *os.File) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}

func ancestorSafe(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	if stat.Uid != 0 && int(stat.Uid) != os.Geteuid() {
		return false
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return false
	}
	return true
}

func ownedByUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}

func trustedCanonical(path string) (string, error) {
	clean := filepath.Clean(path)
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(clean, current), current) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", ErrUnsafe
		}
		if info.Mode()&os.ModeSymlink != 0 {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != 0 {
				return "", ErrUnsafe
			}
		}
	}
	probe := clean
	var suffix []string
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", ErrUnsafe
		}
		suffix = append(suffix, filepath.Base(probe))
		probe = filepath.Dir(probe)
	}
	resolved, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", ErrUnsafe
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	return resolved, nil
}

func syncRoot(root *os.Root) error {
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func lockDirectory(root *os.Root, exclusive bool) (*os.File, error) {
	file, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	mode := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		mode = syscall.LOCK_EX | syscall.LOCK_NB
	}
	if err := syscall.Flock(int(file.Fd()), mode); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return file, nil
}
