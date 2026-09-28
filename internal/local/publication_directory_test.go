package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A publication directory such as ~/.local/bin only has to be safe from
// mutation by another principal: any group or other write is refused, read
// and search access are not.
func TestPublicationDirectoryModeMatrix(t *testing.T) {
	for _, test := range []struct {
		mode os.FileMode
		safe bool
	}{
		{0o700, true}, {0o750, true}, {0o755, true}, {0o711, true},
		{0o702, false}, {0o720, false}, {0o770, false}, {0o775, false}, {0o777, false}, {0o757, false},
	} {
		t.Run(fmt.Sprintf("%04o", test.mode), func(t *testing.T) {
			directory := filepath.Join(privateTestRoot(t), "bin")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "axiom"), []byte("binary"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(directory, test.mode); err != nil {
				t.Fatal(err)
			}
			err := CheckPublicationDirectory(directory)
			_, readErr := ReadPublishedFile(directory, "axiom", 64)
			if test.safe && (err != nil || readErr != nil) {
				t.Fatalf("safe directory refused: %v, %v", err, readErr)
			}
			if !test.safe && (!errors.Is(err, ErrUnsafe) || !errors.Is(readErr, ErrUnsafe)) {
				t.Fatalf("unsafe directory accepted: %v, %v", err, readErr)
			}
			if info, _ := os.Stat(directory); info.Mode().Perm() != test.mode {
				t.Fatalf("directory mode changed to %v", info.Mode())
			}
			// The Axiom-owned private roots keep the owner-only rule.
			if test.mode != 0o700 && !errors.Is(CheckPrivateDirectory(directory), ErrUnsafe) {
				t.Fatal("private directory rule was relaxed")
			}
		})
	}
}

func TestPublicationDirectoryRefusesSymlinkForeignOwnerAndMissing(t *testing.T) {
	base := privateTestRoot(t)
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "bin")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(CheckPublicationDirectory(link), ErrUnsafe) {
		t.Fatal("symlinked directory accepted")
	}
	if err := os.Mkdir(filepath.Join(real, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(CheckPublicationDirectory(filepath.Join(link, "child")), ErrUnsafe) {
		t.Fatal("directory below a user-owned symlink accepted")
	}
	if !errors.Is(CheckPublicationDirectory(filepath.Join(base, "missing")), ErrNotFound) {
		t.Fatal("missing directory not reported absent")
	}
	for _, path := range []string{"relative/bin", "/"} {
		if !errors.Is(CheckPublicationDirectory(path), ErrUnsafe) {
			t.Fatalf("%q accepted", path)
		}
	}
	if os.Geteuid() != 0 {
		if !errors.Is(CheckPublicationDirectory("/usr/bin"), ErrUnsafe) {
			t.Fatal("directory owned by another user accepted")
		}
	}
}

// Files inside a publication directory keep the owner-only file rule.
func TestReadPublishedFileKeepsOwnerOnlyFileRule(t *testing.T) {
	directory := filepath.Join(privateTestRoot(t), "bin")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "axiom")
	if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if wire, err := ReadPublishedFile(directory, "axiom", 64); err != nil || string(wire) != "binary" {
		t.Fatalf("read = %q, %v", wire, err)
	}
	for _, name := range []string{"", ".", "..", "sub/axiom"} {
		if _, err := ReadPublishedFile(directory, name, 64); !errors.Is(err, ErrUnsafe) {
			t.Fatalf("name %q accepted", name)
		}
	}
	if _, err := ReadPublishedFile(directory, "axiom", 3); !errors.Is(err, ErrUnsafe) {
		t.Fatal("size bound ignored")
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPublishedFile(directory, "axiom", 64); !errors.Is(err, ErrUnsafe) {
		t.Fatal("shared-mode binary accepted")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(directory, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPublishedFile(directory, "axiom", 64); !errors.Is(err, ErrUnsafe) {
		t.Fatal("hard-linked binary accepted")
	}
	if err := os.Symlink(path, filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPublishedFile(directory, "link", 64); !errors.Is(err, ErrUnsafe) {
		t.Fatal("symlinked binary accepted")
	}
}
