//go:build !windows

package compatibility

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const noFollow = unix.O_NOFOLLOW

func nativeAvailableSpace(path string) (uint64, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return 0, err
	}
	return uint64(s.Bavail) * uint64(s.Bsize), nil
}
func makeTransferRoot(path string) error                     { return os.Mkdir(path, 0o700) }
func makeTransferDirectory(root *os.Root, name string) error { return root.Mkdir(name, 0o700) }
func syncTransferDirectory(file *os.File) error              { return file.Sync() }
func transferPrivate(_ *os.Root, _ string, info os.FileInfo) bool {
	if info.IsDir() {
		return info.Mode().Perm() == 0o700
	}
	return info.Mode().Perm()&0o077 == 0
}

func checkTransferFilesystem(target string, roots Roots) (uint64, error) {
	parent := filepath.Dir(target)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return 0, ErrTransferTarget
	}
	parentStat, ok := parentInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, ErrTransferTarget
	}
	for _, source := range []string{roots.Projects, roots.State, roots.Skills} {
		if source == "" {
			continue
		}
		info, err := os.Stat(source)
		if os.IsNotExist(err) {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if err != nil || !ok || stat.Dev != parentStat.Dev {
			return 0, ErrTransferTarget
		}
	}
	available, err := availableSpace(parent)
	if err != nil {
		return 0, ErrTransferTarget
	}
	return available, nil
}

func canonicalTransferTarget(target string, roots Roots) (string, error) {
	if !filepath.IsAbs(target) {
		return "", ErrTransferTarget
	}
	clean := filepath.Clean(target)
	base := filepath.Base(clean)
	if clean == string(filepath.Separator) || base == "." || base == ".." || strings.HasPrefix(base, ".") {
		return "", ErrTransferTarget
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return "", ErrTransferTarget
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() {
		return "", ErrTransferTarget
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return "", ErrTransferTarget
	}
	canonical := filepath.Join(parent, base)
	for _, source := range []string{roots.Projects, roots.State, roots.Skills} {
		if source == "" {
			continue
		}
		resolved := filepath.Clean(source)
		if evaluated, err := filepath.EvalSymlinks(resolved); err == nil {
			resolved = evaluated
		}
		if within(resolved, canonical) || within(canonical, resolved) {
			return "", ErrTransferTarget
		}
	}
	return canonical, nil
}
