package local

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/rgomids/axiom/internal/windowsfs"
	"os"
	"strings"
)

func DirectoryIdentity(path string) (string, error) {
	clean, err := windowsfs.Canonical(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(clean)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		return "", ErrUnsafe
	}
	id, err := windowsfs.Identity(f)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(strings.ToLower(clean) + "\x00" + id))
	return "fs:" + hex.EncodeToString(digest[:]), nil
}
