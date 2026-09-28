package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rgomids/axiom/internal/local"
)

// publishTestEffect builds the Effect publishEffect expects for one
// stage/rename of expected -> content inside dir/name.
func publishTestEffect(name string, expected, content []byte) Effect {
	return Effect{Kind: "binary", Name: "", Target: name, Expected: digest(expected), Next: digest(content)}
}

// These tests prove the review's core object-identity requirement directly
// against publishEffect, the function Apply uses for both the binary and the
// receipt file: once dir (an already-opened, validated local.AnchoredDirectory)
// is handed to publishEffect, replacing the directory at its pathname between
// validation and this call cannot redirect the stage, the expected-revision
// reread, the rename, or the confirmation to a different object. This is a
// real OS-level guarantee (os.Root binds later operations to the directory
// object it opened, not to the name that led to it), not a simulated seam.
func TestPublishEffectMutatesTheValidatedDirectoryDespiteReplacement(t *testing.T) {
	for _, test := range []struct {
		name    string
		replace func(t *testing.T, path string) (searchRoot string)
	}{
		{"root replaced by another directory", func(t *testing.T, path string) string {
			if err := os.Rename(path, path+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			return filepath.Dir(path)
		}},
		{"root replaced by a symlink", func(t *testing.T, path string) string {
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
		{"ancestor replaced", func(t *testing.T, path string) string {
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
			home := t.TempDir()
			binaryDir := filepath.Join(home, "bin")
			if err := os.Mkdir(binaryDir, 0o755); err != nil {
				t.Fatal(err)
			}
			old := []byte("old-binary\n")
			if err := os.WriteFile(filepath.Join(binaryDir, binaryName), old, 0o700); err != nil {
				t.Fatal(err)
			}
			dir, err := local.OpenPublicationDirectory(binaryDir)
			if err != nil {
				t.Fatal(err)
			}
			defer dir.Close()

			searchRoot := test.replace(t, binaryDir)

			next := []byte("new-binary\n")
			effect := publishTestEffect(binaryName, old, next)
			if err := publishEffect(dir, binaryName, binaryStage, next, effect, 0o700, maxBinaryBytes); err != nil {
				t.Fatalf("publish after replacement: %v", err)
			}

			// The publication must land in the ORIGINALLY validated
			// directory, wherever it now lives, never in whatever object
			// currently occupies the original pathname.
			found := false
			_ = filepath.Walk(searchRoot, func(p string, info os.FileInfo, err error) error {
				if err == nil && info.Name() == binaryName && info.Mode().IsRegular() {
					data, readErr := os.ReadFile(p)
					if readErr == nil && string(data) == string(next) {
						found = true
					}
				}
				return nil
			})
			if !found {
				t.Fatalf("published binary not found under %s", searchRoot)
			}
			if data, err := os.ReadFile(filepath.Join(binaryDir, binaryName)); err == nil && string(data) == string(next) {
				t.Fatal("publication leaked into the object now at the original pathname")
			}
		})
	}
}

// The target file itself changing between validation and publish (not the
// directory) is refused by the existing expected-revision check, independent
// of the object-identity fix: no misleading success.
func TestPublishEffectRefusesWhenTargetFileChangedAfterOpen(t *testing.T) {
	binaryDir := t.TempDir()
	if err := os.Chmod(binaryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("old-binary\n")
	if err := os.WriteFile(filepath.Join(binaryDir, binaryName), old, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := local.OpenPublicationDirectory(binaryDir)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()

	// Another process races in and changes the target's content directly.
	raced := []byte("raced-binary\n")
	if err := os.WriteFile(filepath.Join(binaryDir, binaryName), raced, 0o700); err != nil {
		t.Fatal(err)
	}

	next := []byte("new-binary\n")
	effect := publishTestEffect(binaryName, old, next)
	if err := publishEffect(dir, binaryName, binaryStage, next, effect, 0o700, maxBinaryBytes); category(err) != "target_changed" {
		t.Fatalf("error=%v", err)
	}
	if data, err := os.ReadFile(filepath.Join(binaryDir, binaryName)); err != nil || string(data) != string(raced) {
		t.Fatalf("target mutated despite refusal: %q, %v", data, err)
	}
	entries, err := os.ReadDir(binaryDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("stage leftover not cleaned up: %v, %v", entries, err)
	}
}

// A foreign directory that happens to sit beside the target during a
// replacement is never touched: publishEffect only ever creates or renames
// its own randomly named stage entry and the exact target name.
func TestPublishEffectNeverTouchesForeignTargetDuringReplacement(t *testing.T) {
	home := t.TempDir()
	binaryDir := filepath.Join(home, "bin")
	if err := os.Mkdir(binaryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("old-binary\n")
	if err := os.WriteFile(filepath.Join(binaryDir, binaryName), old, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := local.OpenPublicationDirectory(binaryDir)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()

	if err := os.Rename(binaryDir, binaryDir+"-moved"); err != nil {
		t.Fatal(err)
	}
	foreignDir := binaryDir
	if err := os.Mkdir(foreignDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreignContent := []byte("attacker-controlled\n")
	if err := os.WriteFile(filepath.Join(foreignDir, binaryName), foreignContent, 0o755); err != nil {
		t.Fatal(err)
	}

	next := []byte("new-binary\n")
	effect := publishTestEffect(binaryName, old, next)
	if err := publishEffect(dir, binaryName, binaryStage, next, effect, 0o700, maxBinaryBytes); err != nil {
		t.Fatalf("publish after replacement: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(foreignDir, binaryName))
	if err != nil || string(data) != string(foreignContent) {
		t.Fatalf("foreign target modified: %q, %v", data, err)
	}
	entries, err := os.ReadDir(foreignDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != binaryName {
		t.Fatalf("foreign directory gained unexpected entries: %v, %v", entries, err)
	}
}
