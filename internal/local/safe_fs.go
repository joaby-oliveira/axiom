package local

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// privateRoot creates and opens one owner-only directory without following
// user-owned symlinks in its ancestry. The returned Root anchors later work to
// directory objects instead of repeating absolute-path resolution.
func privateRoot(path string) (*os.Root, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) == string(filepath.Separator) {
		return nil, ErrUnsafe
	}
	canonical, err := trustedCanonical(path)
	if err != nil {
		return nil, err
	}
	root, err := anchoredRoot(canonical, true)
	if err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err != nil || !ownedByUser(info) {
		root.Close()
		return nil, ErrUnsafe
	}
	if info.Mode().Perm()&0o077 != 0 {
		directory, err := root.Open(".")
		if err != nil {
			root.Close()
			return nil, ErrUnsafe
		}
		err = directory.Chmod(0o700)
		directory.Close()
		if err != nil {
			root.Close()
			return nil, ErrUnsafe
		}
	}
	directory, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, ErrUnsafe
	}
	err = checkPrivateACL(directory)
	directory.Close()
	if err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

// anchoredRoot opens canonical one directory object at a time from "/",
// rejecting a symlinked or replaced component. With create, missing
// components are created owner-only.
func anchoredRoot(canonical string, create bool) (*os.Root, error) {
	root, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(canonical, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		if create {
			if err := root.Mkdir(part, 0o700); err != nil && !os.IsExist(err) {
				root.Close()
				return nil, err
			}
		}
		info, err := root.Lstat(part)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			root.Close()
			return nil, ErrUnsafe
		}
		next, err := root.OpenRoot(part)
		root.Close()
		if err != nil {
			return nil, ErrUnsafe
		}
		actual, err := next.Stat(".")
		if err != nil || !os.SameFile(info, actual) {
			next.Close()
			return nil, ErrUnsafe
		}
		root = next
	}
	return root, nil
}

// existingPublicationRoot opens an existing directory that Axiom publishes
// an owned file into but does not own, such as a user bin directory. Only
// mutation by another principal is unsafe there: it must be a real directory
// owned by the user, without group or other write and without extended ACL.
// Read or search access for others is allowed, and nothing is created or
// re-moded. What Axiom publishes inside it stays owner-only.
func existingPublicationRoot(path string) (*os.Root, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) == string(filepath.Separator) {
		return nil, ErrUnsafe
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 || !ownedByUser(info) {
		return nil, ErrUnsafe
	}
	canonical, err := trustedCanonical(path)
	if err != nil {
		return nil, err
	}
	root, err := anchoredRoot(canonical, false)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) || !ownedByUser(opened) || opened.Mode().Perm()&0o022 != 0 {
		root.Close()
		return nil, ErrUnsafe
	}
	directory, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, ErrUnsafe
	}
	err = checkPrivateACL(directory)
	directory.Close()
	if err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

func existingPrivateRoot(path string) (*os.Root, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || !ownedByUser(info) {
		return nil, ErrUnsafe
	}
	return privateRoot(path)
}

// RootsOverlap resolves trusted system aliases before any root is created.
func RootsOverlap(first, second string) (bool, error) {
	a, err := trustedCanonical(first)
	if err != nil {
		return false, err
	}
	b, err := trustedCanonical(second)
	if err != nil {
		return false, err
	}
	return pathWithin(a, b) || pathWithin(b, a), nil
}

func pathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
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

func ownedByUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}

func stillAtPath(root *os.Root, path string) error {
	visible, err := os.Lstat(path)
	if err != nil || visible.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafe
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(visible, opened) {
		return ErrUnsafe
	}
	return nil
}

func privateChild(parent *os.Root, name string) (*os.Root, error) {
	if err := parent.Mkdir(name, 0o700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	info, err := parent.Lstat(name)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || !ownedByUser(info) {
		return nil, ErrUnsafe
	}
	return existingPrivateChild(parent, name)
}

func existingPrivateChild(parent *os.Root, name string) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || !ownedByUser(info) {
		return nil, ErrUnsafe
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, ErrUnsafe
	}
	actual, err := child.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		child.Close()
		return nil, ErrUnsafe
	}
	directory, err := child.Open(".")
	if err != nil {
		child.Close()
		return nil, ErrUnsafe
	}
	err = checkPrivateACL(directory)
	directory.Close()
	if err != nil {
		child.Close()
		return nil, err
	}
	return child, nil
}

func temporaryName(prefix string) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(entropy[:]), nil
}

func readPrivateFile(root *os.Root, name string) ([]byte, error) {
	return readPrivateFileBounded(root, name, MaxRecordBytes)
}

func readPrivateFileBounded(root *os.Root, name string, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, ErrUnsafe
	}
	if info, err := root.Lstat(name); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafe
	}
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, ErrUnsafe
		}
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !ownedByUser(info) || info.Mode().Perm()&0o077 != 0 {
		return nil, ErrUnsafe
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return nil, ErrUnsafe
	}
	if err := checkPrivateACL(file); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, ErrUnsafe
	}
	return data, nil
}

func verifyPreparedFile(root *os.Root, name string, expected []byte) error {
	actual, err := readPrivateFile(root, name)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, expected) {
		return ErrUnsafe
	}
	return nil
}

func writePrivateFile(root *os.Root, name string, content []byte) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if err := checkPrivateACL(file); err != nil {
		file.Close()
		return err
	}
	if err := writeComplete(file, content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func writeComplete(writer io.Writer, content []byte) error {
	for len(content) != 0 {
		written, err := writer.Write(content)
		if err != nil {
			return err
		}
		if written <= 0 || written > len(content) {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}

func syncRoot(root *os.Root) error {
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

const (
	directoryReadBatch       = 64
	maxLocalDirectoryEntries = 10_002
)

func walkDirectoryNamesBounded(root *os.Root, limit int, visit func(string) bool) error {
	if limit < 0 {
		return ErrUnsafe
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	seen := 0
	for {
		batch := directoryReadBatch
		if remaining := limit - seen + 1; remaining < batch {
			batch = remaining
		}
		names, readErr := directory.Readdirnames(batch)
		for _, name := range names {
			seen++
			if seen > limit {
				return ErrUnsafe
			}
			if visit != nil && !visit(name) {
				return nil
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func readDirectoryNamesBounded(root *os.Root, limit int) ([]string, error) {
	names := make([]string, 0, min(limit, directoryReadBatch))
	err := walkDirectoryNamesBounded(root, limit, func(name string) bool {
		names = append(names, name)
		return true
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

func directoryPrefixPresentBounded(root *os.Root, limit int, prefixes ...string) (bool, error) {
	present := false
	err := walkDirectoryNamesBounded(root, limit, func(name string) bool {
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				present = true
				return false
			}
		}
		return true
	})
	return present, err
}

func markAttempt(root *os.Root, prefix string) (string, error) {
	name, err := temporaryName(prefix)
	if err != nil {
		return "", err
	}
	wire, err := json.Marshal(struct {
		FormatVersion int    `json:"formatVersion"`
		OperationID   string `json:"operationId"`
		Stage         string `json:"stage"`
	}{FormatVersion: 1, OperationID: name, Stage: "pre_publication"})
	if err != nil {
		return "", err
	}
	wire = append(wire, '\n')
	if err := writePrivateFile(root, name, wire); err != nil {
		return "", err
	}
	if err := syncRoot(root); err != nil {
		return name, ErrRecoveryRequired
	}
	return name, nil
}

func clearAttempt(root *os.Root, name string) error {
	return removeProtocolState(root, name, publicationHooks{})
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

func lockRoots(exclusive bool, roots ...*os.Root) ([]*os.File, error) {
	locks := make([]*os.File, 0, len(roots))
	for _, root := range roots {
		lock, err := lockDirectory(root, exclusive)
		if err != nil {
			closeFiles(locks)
			return nil, err
		}
		locks = append(locks, lock)
	}
	return locks, nil
}

func closeFiles(files []*os.File) {
	for index := len(files) - 1; index >= 0; index-- {
		_ = files[index].Close()
	}
}
