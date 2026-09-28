package local

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// fakeFileInfo lets ancestorSafe be tested against ownership combinations
// that this process cannot actually create on disk (a directory owned by a
// different, foreign UID), without needing root.
type fakeFileInfo struct {
	mode os.FileMode
	stat syscall.Stat_t
}

func (f fakeFileInfo) Name() string       { return "" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return &f.stat }

func TestAncestorSafeOwnershipAndStickyMatrix(t *testing.T) {
	self := uint32(os.Geteuid())
	foreign := self + 12345 + 1 // never equal to self or root
	for _, test := range []struct {
		name string
		uid  uint32
		mode os.FileMode
		safe bool
	}{
		{"root owned 0755", 0, 0o755, true},
		{"self owned 0755", self, 0o755, true},
		{"self owned 0700", self, 0o700, true},
		{"self owned 0711", self, 0o711, true},
		{"root owned group writable no sticky", 0, 0o775, false},
		{"self owned other writable no sticky", self, 0o757, false},
		{"self owned world writable no sticky", self, 0o777, false},
		{"root owned world writable sticky", 0, 0o777 | os.ModeSticky, true},
		{"self owned world writable sticky", self, 0o777 | os.ModeSticky, true},
		{"foreign owned 0755", foreign, 0o755, false},
		{"foreign owned 0700", foreign, 0o700, false},
		{"foreign owned world writable sticky", foreign, 0o777 | os.ModeSticky, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := fakeFileInfo{mode: os.ModeDir | test.mode, stat: syscall.Stat_t{Uid: test.uid}}
			if got := ancestorSafe(info); got != test.safe {
				t.Fatalf("ancestorSafe(uid=%d, mode=%v) = %v, want %v", test.uid, info.mode, got, test.safe)
			}
		})
	}
}

func chmodOrFatal(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// A real, on-disk ancestor that this process owns can still be unsafe: any
// group/other write without the sticky bit lets another local user replace
// the entry beneath it, regardless of who owns the ancestor itself.
func TestPublicationDirectoryRefusesUnsafeAncestorRealFilesystem(t *testing.T) {
	for _, test := range []struct {
		name        string
		ancestorFmt os.FileMode
		safe        bool
	}{
		{"ancestor 0755", 0o755, true},
		{"ancestor 0750", 0o750, true},
		{"ancestor 0775 group writable no sticky", 0o775, false},
		{"ancestor 0757 other writable no sticky", 0o757, false},
		{"ancestor 0777 world writable no sticky", 0o777, false},
		{"ancestor 1777 world writable sticky", 0o777 | os.ModeSticky, true},
		{"ancestor 1775 group writable sticky", 0o775 | os.ModeSticky, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := privateTestRoot(t)
			shared := filepath.Join(home, "shared")
			if err := os.Mkdir(shared, 0o755); err != nil {
				t.Fatal(err)
			}
			chmodOrFatal(t, shared, test.ancestorFmt)
			target := filepath.Join(shared, "bin")
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatal(err)
			}
			err := CheckPublicationDirectory(target)
			if test.safe && err != nil {
				t.Fatalf("safe ancestor refused: %v", err)
			}
			if !test.safe && !errors.Is(err, ErrUnsafe) {
				t.Fatalf("unsafe ancestor accepted: %v", err)
			}
			// The owner-only private rule is at least as strict.
			privateTarget := filepath.Join(shared, "state")
			if err := os.Mkdir(privateTarget, 0o700); err != nil {
				t.Fatal(err)
			}
			err = CheckPrivateDirectory(privateTarget)
			if test.safe && err != nil {
				t.Fatalf("safe ancestor refused private directory: %v", err)
			}
			if !test.safe && !errors.Is(err, ErrUnsafe) {
				t.Fatalf("unsafe ancestor accepted private directory: %v", err)
			}
		})
	}
}

// A two-level unsafe ancestor (unsafe grandparent, safe parent) is still
// refused: every container up to "/" is checked, not only the immediate
// parent.
func TestPublicationDirectoryRefusesUnsafeGrandparent(t *testing.T) {
	home := privateTestRoot(t)
	grandparent := filepath.Join(home, "grandparent")
	if err := os.Mkdir(grandparent, 0o755); err != nil {
		t.Fatal(err)
	}
	// Mkdir applies umask; chmod explicitly to get the exact unsafe mode.
	chmodOrFatal(t, grandparent, 0o777)
	parent := filepath.Join(grandparent, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "bin")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CheckPublicationDirectory(target); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("unsafe grandparent accepted: %v", err)
	}
}

// This is the review's core object-identity requirement: once a directory is
// opened and validated as an AnchoredDirectory, replacing it at its pathname
// (an ancestor/leaf replacement between validation and mutation) must not be
// able to redirect a later Stage/Rename/ReadFile to the new, unvalidated
// object. This is a real OS-level test of *os.Root's guarantee, not a
// simulated seam: Go's os.Root binds later operations to the directory
// object it opened, not to the name that led to it.
func TestAnchoredDirectoryMutatesTheValidatedObjectDespiteReplacement(t *testing.T) {
	for _, test := range []struct {
		name    string
		replace func(t *testing.T, path string) (searchRoot string)
	}{
		{"leaf renamed away and replaced by a new directory", func(t *testing.T, path string) string {
			if err := os.Rename(path, path+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			return filepath.Dir(path)
		}},
		{"leaf renamed away and replaced by a symlink", func(t *testing.T, path string) string {
			foreign := filepath.Join(filepath.Dir(path), "foreign")
			if err := os.Mkdir(foreign, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path, path+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(foreign, path); err != nil {
				t.Fatal(err)
			}
			return filepath.Dir(path)
		}},
		{"parent renamed away, leaf still reachable only through the old name", func(t *testing.T, path string) string {
			parent := filepath.Dir(path)
			if err := os.Rename(parent, parent+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			return filepath.Dir(parent)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := privateTestRoot(t)
			original := filepath.Join(home, "bin")
			if err := os.Mkdir(original, 0o755); err != nil {
				t.Fatal(err)
			}
			directory, err := OpenPublicationDirectory(original)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()

			searchRoot := test.replace(t, original)

			staged, err := directory.Stage(".axiom-test-stage.", []byte("payload"), 0o700)
			if err != nil {
				t.Fatalf("stage after replacement: %v", err)
			}
			if err := directory.Rename(staged, "axiom"); err != nil {
				t.Fatal(err)
			}
			confirmed, err := directory.ReadFile("axiom", 64)
			if err != nil || string(confirmed) != "payload" {
				t.Fatalf("confirm through the same handle: %q, %v", confirmed, err)
			}

			// The published file must exist in the ORIGINALLY validated
			// object, wherever it now lives, never in whatever object
			// currently occupies the original pathname.
			found := false
			_ = filepath.Walk(searchRoot, func(p string, info os.FileInfo, err error) error {
				if err == nil && info.Name() == "axiom" && info.Mode().IsRegular() {
					found = true
				}
				return nil
			})
			if !found {
				t.Fatalf("published file not found anywhere under %s", searchRoot)
			}
			if _, err := os.Lstat(filepath.Join(original, "axiom")); !os.IsNotExist(err) {
				t.Fatalf("mutation leaked into the object now at the original pathname: %v", err)
			}
		})
	}
}

// The same guarantee for an owner-only (Axiom-owned) anchored directory.
func TestAnchoredOwnedDirectoryMutatesTheValidatedObjectDespiteReplacement(t *testing.T) {
	home := privateTestRoot(t)
	original := filepath.Join(home, "state")
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenOwnedDirectory(original)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()

	if err := os.Rename(original, original+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := directory.CreateExclusive("marker", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(original+"-moved", "marker")); err != nil {
		t.Fatalf("marker did not land on the validated object: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(original, "marker")); !os.IsNotExist(err) {
		t.Fatal("marker leaked into the replacement directory")
	}
}
