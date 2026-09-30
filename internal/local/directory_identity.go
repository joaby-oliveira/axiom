//go:build !windows

package local

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// DirectoryIdentity observes one explicit local directory without following a
// symlink at the selected path. The value binds setup authority and later local
// resolution to the same filesystem object; it is not portable Project intent.
func DirectoryIdentity(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("directory path is not absolute")
	}
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("directory is unavailable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("directory identity is unavailable")
	}
	facts := clean + "\x00" + strconv.FormatUint(uint64(stat.Dev), 10) + "\x00" + strconv.FormatUint(uint64(stat.Ino), 10)
	digest := sha256.Sum256([]byte(facts))
	return "fs:" + hex.EncodeToString(digest[:]), nil
}
