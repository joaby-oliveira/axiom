package gitworkspace

import (
	"github.com/rgomids/axiom/internal/windowsfs"
	"os"
)

func checkPrivateACL(file *os.File) error { return windowsfs.Check(file, true) }
func privateDirectory(path string) bool   { return privateObject(path, true) }
func privateRegularFile(path string) bool { return privateObject(path, false) }
func privateObject(path string, directory bool) bool {
	if _, err := windowsfs.Canonical(path); err != nil {
		return false
	}
	before, err := os.Lstat(path)
	if err != nil || before.IsDir() != directory || (!directory && !before.Mode().IsRegular()) {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	after, err := file.Stat()
	return err == nil && os.SameFile(before, after) && checkPrivateACL(file) == nil
}
