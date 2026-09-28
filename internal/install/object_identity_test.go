package install

import (
	"context"
	"errors"
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

// replaceDirectory scenarios move the directory validated at path (or its
// parent) away and put another object at the authorized pathname. Each
// returns where the ORIGINALLY validated object now lives and the directory
// now visible at the authorized pathname.
var replaceDirectory = []struct {
	name    string
	replace func(t *testing.T, path string) (original, replacement string)
}{
	{"root replaced by another directory", func(t *testing.T, path string) (string, string) {
		if err := os.Rename(path, path+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return path + "-moved", path
	}},
	{"root replaced by a symlink", func(t *testing.T, path string) (string, string) {
		foreign := filepath.Join(filepath.Dir(path), "foreign")
		if err := os.Mkdir(foreign, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, path+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(foreign, path); err != nil {
			t.Fatal(err)
		}
		return path + "-moved", foreign
	}},
	{"ancestor replaced", func(t *testing.T, path string) (string, string) {
		parent := filepath.Dir(path)
		if err := os.Rename(parent, parent+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(parent+"-moved", filepath.Base(path)), path
	}},
}

// These tests prove the review's object-identity requirement directly
// against publishEffect, the function Apply uses for both the binary and the
// receipt file: once dir (an already-opened, validated local.AnchoredDirectory)
// is handed to publishEffect, replacing the directory or an ancestor at its
// authorized pathname is DETECTED AND REFUSED as target_changed. Nothing is
// committed in the displaced original object (which keeps its prior binary
// and no stage) nor in the replacement (which stays byte-for-byte intact),
// and no success is reported (ADR-0005 property 3, ADR-0007 invariant 7).
func TestPublishEffectRefusesReplacedDirectory(t *testing.T) {
	for _, test := range replaceDirectory {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			binaryDir := filepath.Join(home, "parent", "bin")
			if err := os.MkdirAll(binaryDir, 0o755); err != nil {
				t.Fatal(err)
			}
			old := []byte("old-binary\n")
			writeFile(t, filepath.Join(binaryDir, binaryName), old, 0o700)
			dir, err := local.OpenPublicationDirectory(binaryDir)
			if err != nil {
				t.Fatal(err)
			}
			defer dir.Close()

			original, replacement := test.replace(t, binaryDir)
			foreign := []byte("attacker-controlled\n")
			writeFile(t, filepath.Join(replacement, binaryName), foreign, 0o755)
			before := snapshot(t, replacement)

			next := []byte("new-binary\n")
			effect := publishTestEffect(binaryName, old, next)
			if err := publishEffect(dir, binaryName, binaryStage, next, effect, 0o700, maxBinaryBytes); category(err) != "target_changed" {
				t.Fatalf("publish after replacement: %v", err)
			}
			if got := read(t, filepath.Join(original, binaryName)); got != string(old) {
				t.Fatalf("displaced original object was published: %q", got)
			}
			if entries, err := os.ReadDir(original); err != nil || len(entries) != 1 {
				t.Fatalf("displaced original object gained entries: %v, %v", entries, err)
			}
			if after := snapshot(t, replacement); after != before {
				t.Fatal("replacement object was modified")
			}
		})
	}
}

// The target file itself changing between validation and publish (not the
// directory) is refused by the existing expected-revision check, independent
// of the object-identity check: no misleading success.
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

// End to end through Apply: ReceiptDir is replaced after the binary commit.
// The next marker/receipt step must refuse as target_changed, the result
// must stay partial with only the binary confirmed, the displaced original
// receipt must keep its prior content, and the replacement must stay empty
// (no receipt, marker, stage, or lock published into it).
func TestApplyRefusesReceiptDirectoryReplacedMidOperation(t *testing.T) {
	installed := install(t, newBundle("1.0.0", []byte("old-binary\n")))
	candidate := installed.candidate(t, newBundle("1.1.0", []byte("new-binary\n")))
	service := NewService()
	preview, err := service.Preview(context.Background(), installed.target, candidate)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := Authorize(preview, preview.Digest)
	receiptDir := installed.target.ReceiptDir
	priorReceipt := read(t, filepath.Join(receiptDir, receiptName))
	service.afterEffect = func(kind string) error {
		if kind == "binary" {
			if err := os.Rename(receiptDir, receiptDir+"-moved"); err != nil {
				return err
			}
			return os.Mkdir(receiptDir, 0o700)
		}
		return nil
	}
	result, err := service.Apply(context.Background(), preview, authority)
	if category(err) != "target_changed" || result.Status != "partial" || len(result.Ledger) != 1 || result.Ledger[0].Kind != "binary" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got := read(t, filepath.Join(receiptDir+"-moved", receiptName)); got != priorReceipt {
		t.Fatal("receipt published into the displaced original directory")
	}
	if entries, err := os.ReadDir(receiptDir); err != nil || len(entries) != 0 {
		t.Fatalf("replacement receipt directory mutated: %v, %v", entries, err)
	}
}

// End to end through Apply: BinaryDir is replaced after every effect was
// confirmed but before the operation reports completion. Apply must not
// declare success for an installation whose canonical binary pathname no
// longer shows the object it published into; the replacement stays intact.
func TestApplyDoesNotDeclareSuccessAfterBinaryDirectoryReplaced(t *testing.T) {
	installed := install(t, newBundle("1.0.0", []byte("old-binary\n")))
	candidate := installed.candidate(t, newBundle("1.1.0", []byte("new-binary\n")))
	service := NewService()
	preview, err := service.Preview(context.Background(), installed.target, candidate)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := Authorize(preview, preview.Digest)
	binaryDir := installed.target.BinaryDir
	service.afterEffect = func(kind string) error {
		if kind == "receipt" {
			if err := os.Rename(binaryDir, binaryDir+"-moved"); err != nil {
				return err
			}
			return os.Mkdir(binaryDir, 0o700)
		}
		return nil
	}
	result, err := service.Apply(context.Background(), preview, authority)
	if category(err) != "final_verification_failed" || result.Status != "partial" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if entries, err := os.ReadDir(binaryDir); err != nil || len(entries) != 0 {
		t.Fatalf("replacement binary directory mutated: %v, %v", entries, err)
	}
	if _, err := os.Lstat(filepath.Join(installed.target.ReceiptDir, markerName)); err != nil {
		t.Fatalf("operation marker cleared despite the replacement: %v", err)
	}
}

// The ErrReplaced sentinel stays an ErrUnsafe so existing unsafe-target
// handling keeps classifying it conservatively.
func TestReplacedIsUnsafe(t *testing.T) {
	if !errors.Is(local.ErrReplaced, local.ErrUnsafe) {
		t.Fatal("ErrReplaced must wrap ErrUnsafe")
	}
}

func TestApplyKeepsMarkerWhenSkillRootReplacedAfterPublication(t *testing.T) {
	old := newBundle("1.0.0", []byte("old"))
	installed := install(t, old)
	root := installed.withSkillsRoot(t, old.skills)
	next := newBundle("1.1.0", []byte("new"))
	candidate := installed.candidate(t, next)
	service := NewService()
	preview, err := service.Preview(context.Background(), installed.target, candidate)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := Authorize(preview, preview.Digest)
	var original, replacement string
	service.afterEffect = func(kind string) error {
		if kind != "skill:"+skillNames[len(skillNames)-1] {
			return nil
		}
		if err := os.Rename(root, root+"-old"); err != nil {
			return err
		}
		// Even a complete matching set at B cannot confirm publication in A.
		for _, name := range skillNames {
			if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
				return err
			}
			writeFile(t, filepath.Join(root, name, "SKILL.md"), next.skills[name], 0600)
		}
		original, replacement = snapshot(t, root+"-old"), snapshot(t, root)
		return nil
	}
	result, err := service.Apply(context.Background(), preview, authority)
	if category(err) != "final_verification_failed" || result.Status != "partial" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if snapshot(t, root+"-old") != original || snapshot(t, root) != replacement {
		t.Fatal("replacement caused new mutation")
	}
	if _, err := os.Lstat(filepath.Join(installed.target.ReceiptDir, markerName)); err != nil {
		t.Fatal("recovery marker removed")
	}
}
