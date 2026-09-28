package codexruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// These tests prove the review's object-identity requirement for the T40
// skill-root mutation path: once a Runtime skill root (or, for Install, an
// individual skill directory reached through it) is opened and validated,
// replacing it at its pathname before the mutation completes must never
// redirect that mutation to a different, unvalidated object — the mutation
// either lands on the originally validated object (immune to the
// replacement, the same real os.Root guarantee proven in internal/local) or
// is refused; a foreign object is never silently accepted as success.

func TestUpgradeSessionPublishesToValidatedRootDespiteReplacement(t *testing.T) {
	for _, test := range []struct {
		name    string
		replace func(t *testing.T, root string) (searchRoot string)
	}{
		{"root replaced by another directory", func(t *testing.T, root string) string {
			if err := os.Rename(root, root+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			return filepath.Dir(root)
		}},
		{"root replaced by a symlink", func(t *testing.T, root string) string {
			foreign := filepath.Join(filepath.Dir(root), "foreign-root")
			if err := os.Mkdir(foreign, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(root, root+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(foreign, root); err != nil {
				t.Fatal(err)
			}
			return filepath.Dir(root)
		}},
		{"ancestor replaced", func(t *testing.T, root string) string {
			parent := filepath.Dir(root)
			if err := os.Rename(parent, parent+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			return filepath.Dir(parent)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "skills")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			service, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			session, err := service.LockForUpgrade()
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()

			searchRoot := test.replace(t, root)

			name := skillNames[0]
			content := []byte("skill content\n")
			if err := session.PublishSkill(name, content, ""); err != nil {
				t.Fatalf("publish after replacement: %v", err)
			}

			found := false
			_ = filepath.Walk(searchRoot, func(p string, info os.FileInfo, err error) error {
				if err == nil && info.Name() == "SKILL.md" && info.Mode().IsRegular() {
					data, readErr := os.ReadFile(p)
					if readErr == nil && string(data) == string(content) {
						found = true
					}
				}
				return nil
			})
			if !found {
				t.Fatalf("published skill not found under %s", searchRoot)
			}
			if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
				t.Fatal("publication leaked into the object now at the original pathname")
			}
		})
	}
}

// A foreign skill (unexpected content, not a known Axiom revision) already
// present is refused regardless of any root replacement games, and stays
// byte-for-byte untouched; no receipt or other skill is published either.
func TestInstallRefusesForeignSkillUntouchedDespiteRootReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "skills")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	foreignName := skillNames[2]
	foreignDir := filepath.Join(root, foreignName)
	if err := os.Mkdir(foreignDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignContent := []byte("not an Axiom skill\n")
	if err := os.WriteFile(filepath.Join(foreignDir, "SKILL.md"), foreignContent, 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "codex_skill_conflict" {
		t.Fatalf("install = %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(foreignDir, "SKILL.md"))
	if err != nil || string(data) != string(foreignContent) {
		t.Fatalf("foreign skill modified: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(root, receiptName)); !os.IsNotExist(err) {
		t.Fatal("receipt published despite a refused foreign skill")
	}
	for _, name := range skillNames {
		if name == foreignName {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("skill %s installed despite an unrelated conflict", name)
		}
	}
}

// A skill directory silently replaced by a symlink between LockForUpgrade
// and PublishSkill is refused, not adopted: privateChild rejects a symlink
// leaf, so the publication never follows it.
func TestPublishSkillRefusesSkillDirectoryReplacedBySymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	name := skillNames[0]
	directory := filepath.Join(root, name)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.LockForUpgrade()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	foreign := filepath.Join(root, "foreign-skill-dir")
	if err := os.Mkdir(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, directory); err != nil {
		t.Fatal(err)
	}

	if err := session.PublishSkill(name, []byte("attacker payload\n"), digestOf([]byte("original\n"))); err != ErrUpgradeConflict {
		t.Fatalf("symlinked skill directory accepted: %v", err)
	}
	if entries, err := os.ReadDir(foreign); err != nil || len(entries) != 0 {
		t.Fatalf("foreign directory received content: %v, %v", entries, err)
	}
}
