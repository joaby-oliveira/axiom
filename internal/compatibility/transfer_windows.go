package compatibility

import (
	"github.com/rgomids/axiom/internal/local"
	"github.com/rgomids/axiom/internal/windowsfs"
	"os"
	"path/filepath"
	"strings"
)

const noFollow = 0

func nativeAvailableSpace(path string) (uint64, error)       { return windowsfs.Available(path) }
func makeTransferDirectory(root *os.Root, name string) error { return windowsfs.Mkdir(root, name) }
func makeTransferRoot(path string) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	return windowsfs.Mkdir(root, filepath.Base(path))
}
func syncTransferDirectory(file *os.File) error { _, err := file.Stat(); return err }
func transferPrivate(root *os.Root, name string, info os.FileInfo) bool {
	f, err := root.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	actual, err := f.Stat()
	return err == nil && os.SameFile(info, actual) && windowsfs.Check(f, true) == nil
}
func canonicalTransferTarget(target string, roots Roots) (string, error) {
	clean, err := windowsfs.Canonical(target)
	if err != nil || filepath.Dir(clean) == clean || strings.HasPrefix(filepath.Base(clean), ".") {
		return "", ErrTransferTarget
	}
	dir, err := local.OpenPublicationDirectory(filepath.Dir(clean))
	if err != nil {
		return "", ErrTransferTarget
	}
	dir.Close()
	for _, source := range []string{roots.Projects, roots.State, roots.Skills} {
		if source != "" && (within(source, clean) || within(clean, source)) {
			return "", ErrTransferTarget
		}
	}
	return clean, nil
}
func checkTransferFilesystem(target string, roots Roots) (uint64, error) {
	parent := filepath.Dir(target)
	file, err := os.Open(parent)
	if err != nil {
		return 0, ErrTransferTarget
	}
	defer file.Close()
	parentID, err := windowsfs.Identity(file)
	if err != nil {
		return 0, ErrTransferTarget
	}
	volume, _, _ := strings.Cut(parentID, ":")
	for _, source := range []string{roots.Projects, roots.State, roots.Skills} {
		if source == "" {
			continue
		}
		f, err := os.Open(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 0, ErrTransferTarget
		}
		id, err := windowsfs.Identity(f)
		f.Close()
		v, _, _ := strings.Cut(id, ":")
		if err != nil || v != volume {
			return 0, ErrTransferTarget
		}
	}
	return availableSpace(parent)
}
