package codexruntime

import (
	"context"
	"errors"
	"github.com/rgomids/axiom/internal/testfs"
	"os"
	"path/filepath"
	"testing"
)

// These tests prove the review's object-identity requirement for the T40
// skill-root mutation path: once a Runtime skill root (or a skill directory
// reached through it) is opened and validated, replacing it or an ancestor at
// its authorized pathname before the mutation completes must be DETECTED AND
// REFUSED. The mutation neither lands in the replacement nor completes in the
// displaced original object, and no success is reported (ADR-0005 property 3,
// ADR-0007 invariant 7).

var replaceSkillRoot = []struct {
	name    string
	replace func(t *testing.T, root string) (original, replacement string)
}{
	{"root replaced by another directory", func(t *testing.T, root string) (string, string) {
		if err := testfs.RenameOrSkipPinned(t, root, root+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		return root + "-moved", root
	}},
	{"root replaced by a symlink", func(t *testing.T, root string) (string, string) {
		foreign := filepath.Join(filepath.Dir(root), "foreign-root")
		if err := os.Mkdir(foreign, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := testfs.RenameOrSkipPinned(t, root, root+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := testfs.Symlink(t, foreign, root); err != nil {
			t.Fatal(err)
		}
		return root + "-moved", foreign
	}},
	{"ancestor replaced", func(t *testing.T, root string) (string, string) {
		parent := filepath.Dir(root)
		if err := testfs.RenameOrSkipPinned(t, parent, parent+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(parent+"-moved", filepath.Base(root)), root
	}},
}

func directoryNames(t *testing.T, path string) []string {
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

func TestUpgradeSessionRefusesReplacedRoot(t *testing.T) {
	for _, test := range replaceSkillRoot {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "parent", "skills")
			if err := os.MkdirAll(root, 0o755); err != nil {
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

			original, replacement := test.replace(t, root)

			name := skillNames[0]
			if err := session.PublishSkill(name, []byte("skill content\n"), ""); !errors.Is(err, ErrTargetReplaced) {
				t.Fatalf("publish after replacement: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(original, name)); !os.IsNotExist(err) {
				t.Fatalf("skill published into the displaced original root: %v", err)
			}
			if names := directoryNames(t, replacement); len(names) != 0 {
				t.Fatalf("replacement root mutated: %v", names)
			}
		})
	}
}

// Install detects a skill root replaced between two skills: with no change
// committed yet it fails without changes, and it never completes the install
// (remaining skills, receipt) in the displaced original root nor writes into
// the replacement.
func TestInstallRefusesRootReplacedBeforeAnyChange(t *testing.T) {
	root := filepath.Join(t.TempDir(), "parent", "skills")
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Applied {
		t.Fatalf("initial install = %#v", got)
	}
	// Make the last skill an earlier state (absent) so the rerun would
	// have to change something after the replacement.
	last := skillNames[len(skillNames)-1]
	if err := os.RemoveAll(filepath.Join(root, last)); err != nil {
		t.Fatal(err)
	}
	replaced := false
	service.afterSkill = func(string) {
		if !replaced {
			replaced = true
			if err := testfs.RenameOrSkipPinned(t, root, root+"-moved"); err != nil {
				t.Error(err)
			}
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Error(err)
			}
		}
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "codex_skill_root_unavailable" {
		t.Fatalf("install after replacement = %#v", got)
	}
	if _, err := os.Lstat(filepath.Join(root+"-moved", last)); !os.IsNotExist(err) {
		t.Fatalf("install completed in the displaced original root: %v", err)
	}
	if names := directoryNames(t, root); len(names) != 0 {
		t.Fatalf("replacement root mutated: %v", names)
	}
}

// With a skill already changed by this run, a later replacement makes the
// install partial, never applied, and the receipt is not published anywhere.
func TestInstallIsPartialWhenRootReplacedAfterAChange(t *testing.T) {
	root := filepath.Join(t.TempDir(), "parent", "skills")
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	replaced := false
	service.afterSkill = func(string) {
		if !replaced {
			replaced = true
			if err := testfs.RenameOrSkipPinned(t, root, root+"-moved"); err != nil {
				t.Error(err)
			}
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Error(err)
			}
		}
	}
	if got := service.Install(context.Background()); got.Status != Partial || got.Category != "codex_skill_install_partial" {
		t.Fatalf("install after replacement = %#v", got)
	}
	for _, directory := range []string{root, root + "-moved"} {
		if _, err := os.Lstat(filepath.Join(directory, receiptName)); !os.IsNotExist(err) {
			t.Fatalf("receipt published in %s despite the replacement: %v", directory, err)
		}
	}
	if names := directoryNames(t, root); len(names) != 0 {
		t.Fatalf("replacement root mutated: %v", names)
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
	if err := testfs.Symlink(t, foreign, directory); err != nil {
		t.Fatal(err)
	}

	if err := session.PublishSkill(name, []byte("attacker payload\n"), digestOf([]byte("original\n"))); err != ErrUpgradeConflict {
		t.Fatalf("symlinked skill directory accepted: %v", err)
	}
	if entries, err := os.ReadDir(foreign); err != nil || len(entries) != 0 {
		t.Fatalf("foreign directory received content: %v, %v", entries, err)
	}
}
