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

// replacementScenarios move the directory validated at path (or one of its
// ancestors) away and put something else at the authorized pathname. Each
// returns the path where the ORIGINALLY validated object now lives and the
// path of the object now visible at the authorized pathname.
var replacementScenarios = []struct {
	name    string
	replace func(t *testing.T, path string) (original, replacement string)
}{
	{"leaf renamed away and replaced by a new directory", func(t *testing.T, path string) (string, string) {
		renameOrFatal(t, path, path+"-moved")
		mkdirOrFatal(t, path, 0o700)
		return path + "-moved", path
	}},
	{"leaf renamed away and replaced by a symlink", func(t *testing.T, path string) (string, string) {
		foreign := filepath.Join(filepath.Dir(path), "foreign")
		mkdirOrFatal(t, foreign, 0o700)
		renameOrFatal(t, path, path+"-moved")
		if err := os.Symlink(foreign, path); err != nil {
			t.Fatal(err)
		}
		return path + "-moved", foreign
	}},
	{"parent renamed away and the pathname recreated", func(t *testing.T, path string) (string, string) {
		parent := filepath.Dir(path)
		renameOrFatal(t, parent, parent+"-moved")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(parent+"-moved", filepath.Base(path)), path
	}},
	{"parent replaced while the original leaf is moved back under it", func(t *testing.T, path string) (string, string) {
		// The leaf object is again visible at its pathname, but through a
		// different ancestor: ancestor identity changed, which ADR-0005
		// property 3 also requires to be detected.
		parent := filepath.Dir(path)
		renameOrFatal(t, parent, parent+"-moved")
		mkdirOrFatal(t, parent, 0o700)
		renameOrFatal(t, filepath.Join(parent+"-moved", filepath.Base(path)), path)
		return path, path
	}},
}

func renameOrFatal(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

func mkdirOrFatal(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatal(err)
	}
}

func entryNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// This is the review's core object-identity requirement: once a directory is
// opened and validated as an AnchoredDirectory, replacing it (or an ancestor)
// at its authorized pathname must be DETECTED AND REFUSED at the next create
// or commit, not tolerated by completing the mutation in the displaced
// original object (ADR-0005 property 3, ADR-0007 invariant 7). Neither the
// original object nor the replacement may gain an entry.
func TestAnchoredDirectoryRefusesMutationAfterReplacement(t *testing.T) {
	for _, open := range []struct {
		name string
		open func(string) (AnchoredDirectory, error)
	}{
		{"publication", OpenPublicationDirectory},
		{"owned", OpenOwnedDirectory},
	} {
		for _, test := range replacementScenarios {
			t.Run(open.name+"/"+test.name, func(t *testing.T) {
				home := privateTestRoot(t)
				parent := filepath.Join(home, "parent")
				mkdirOrFatal(t, parent, 0o700)
				path := filepath.Join(parent, "target")
				mkdirOrFatal(t, path, 0o700)
				directory, err := open.open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer directory.Close()
				if err := directory.StillAtPath(); err != nil {
					t.Fatalf("unreplaced directory refused: %v", err)
				}

				original, replacement := test.replace(t, path)

				if err := directory.StillAtPath(); !errors.Is(err, ErrReplaced) || !errors.Is(err, ErrUnsafe) {
					t.Fatalf("replacement not detected: %v", err)
				}
				if _, err := directory.Stage(".axiom-test-stage.", []byte("payload"), 0o600); !errors.Is(err, ErrReplaced) {
					t.Fatalf("stage after replacement: %v", err)
				}
				if err := directory.CreateExclusive("marker", []byte("x"), 0o600); !errors.Is(err, ErrReplaced) {
					t.Fatalf("create after replacement: %v", err)
				}
				if err := directory.Mkdir("lock"); !errors.Is(err, ErrReplaced) {
					t.Fatalf("mkdir after replacement: %v", err)
				}
				if names := entryNames(t, original); len(names) != 0 {
					t.Fatalf("displaced original object mutated: %v", names)
				}
				if names := entryNames(t, replacement); len(names) != 0 {
					t.Fatalf("replacement object mutated: %v", names)
				}
			})
		}
	}
}

// The commit itself is guarded: a stage prepared while the directory was
// still authorized is not renamed into place once the directory was replaced,
// and the handle can still discard its own stage from the displaced object.
func TestAnchoredDirectoryRefusesCommitAfterReplacement(t *testing.T) {
	for _, test := range replacementScenarios {
		t.Run(test.name, func(t *testing.T) {
			home := privateTestRoot(t)
			parent := filepath.Join(home, "parent")
			mkdirOrFatal(t, parent, 0o700)
			path := filepath.Join(parent, "bin")
			mkdirOrFatal(t, path, 0o755)
			directory, err := OpenPublicationDirectory(path)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			staged, err := directory.Stage(".axiom-test-stage.", []byte("payload"), 0o700)
			if err != nil {
				t.Fatal(err)
			}

			original, replacement := test.replace(t, path)

			if err := directory.Rename(staged, "axiom"); !errors.Is(err, ErrReplaced) {
				t.Fatalf("commit after replacement: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(original, "axiom")); !os.IsNotExist(err) {
				t.Fatalf("commit landed in the displaced original object: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(replacement, "axiom")); !os.IsNotExist(err) {
				t.Fatalf("commit landed in the replacement object: %v", err)
			}
			if err := directory.Remove(staged); err != nil {
				t.Fatalf("own stage cleanup: %v", err)
			}
			if names := entryNames(t, original); len(names) != 0 {
				t.Fatalf("stage leftover in the original object: %v", names)
			}
		})
	}
}

// Moving the validated directory away and back restores the same object at
// the same pathname through the same ancestors: that is not a replacement.
func TestAnchoredDirectoryAcceptsSameObjectRestoredAtPath(t *testing.T) {
	home := privateTestRoot(t)
	path := filepath.Join(home, "state")
	mkdirOrFatal(t, path, 0o700)
	directory, err := OpenOwnedDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	renameOrFatal(t, path, path+"-moved")
	renameOrFatal(t, path+"-moved", path)
	if err := directory.CreateExclusive("marker", []byte("x"), 0o600); err != nil {
		t.Fatalf("same object refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "marker")); err != nil {
		t.Fatal(err)
	}
}
