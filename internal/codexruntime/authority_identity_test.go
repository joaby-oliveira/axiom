package codexruntime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func authoritySnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out[rel] = info.Mode().String()
		if !entry.IsDir() {
			wire, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[rel] += string(wire)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReceiptAndSkillAuthorityUsesAnchoredObject(t *testing.T) {
	for _, integration := range []integration{codexIntegration, claudeIntegration} {
		for _, owned := range []bool{true, false} {
			t.Run(integration.runtime+map[bool]string{true: "/owned-A-foreign-B", false: "/foreign-A-owned-B"}[owned], func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "skills")
				service, _ := New(path)
				service.integration = integration
				if got := service.Install(context.Background()); got.Status != Applied {
					t.Fatal(got)
				}
				nextRelease(t)
				current, _ := integration.receipt(path)
				legacy, _ := integration.receiptFor(path, sharedSkillHistory[0])
				a, b := legacy, []byte("foreign receipt\n")
				if !owned {
					a, b = b, current
				}
				if err := os.WriteFile(filepath.Join(path, receiptName), a, 0600); err != nil {
					t.Fatal(err)
				}
				name := skillNames[0]
				skill, err := os.ReadFile(filepath.Join(path, name, "SKILL.md"))
				if err != nil {
					t.Fatal(err)
				}
				if !owned {
					if err := os.WriteFile(filepath.Join(path, name, "SKILL.md"), []byte("foreign skill"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				root, identity, err := ensureRoot(path)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				if err := os.Rename(path, path+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(path, name), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, receiptName), b, 0600); err != nil {
					t.Fatal(err)
				}
				replacementSkill := []byte("foreign skill")
				if !owned {
					replacementSkill = skill
				}
				if err := os.WriteFile(filepath.Join(path, name, "SKILL.md"), replacementSkill, 0600); err != nil {
					t.Fatal(err)
				}
				beforeA, beforeB := authoritySnapshot(t, path+"-old"), authoritySnapshot(t, path)
				if got := integration.receiptRecognizedIn(root, path); got != owned {
					t.Fatalf("receipt authority = %v, want A = %v", got, owned)
				}
				if got := integration.matchesLegacyReceiptIn(root, path); got != owned {
					t.Fatalf("legacy authority = %v, want A = %v", got, owned)
				}
				if got := matchesSkillIn(root, name, skill); got != owned {
					t.Fatalf("skill authority = %v, want A = %v", got, owned)
				}
				verify := func() error { return identity.verify(root) }
				if changed, confirmed := integration.publishReceiptIn(root, path, current, verify); changed || confirmed {
					t.Fatal("replacement authorized receipt")
				}
				if changed, _, err := integration.installOne(root, name, skill, verify); changed || err == nil {
					t.Fatal("replacement authorized skill")
				}
				if !reflect.DeepEqual(beforeA, authoritySnapshot(t, path+"-old")) || !reflect.DeepEqual(beforeB, authoritySnapshot(t, path)) {
					t.Fatal("replacement mutated A or B")
				}
			})
		}
	}
}

func TestInstallOwnershipInspectionDoesNotReadReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills")
	service, _ := NewClaude(path)
	if got := service.Install(context.Background()); got.Status != Applied {
		t.Fatal(got)
	}
	// An absent owned entry requires publication; B advertises conflicting evidence.
	if err := os.RemoveAll(filepath.Join(path, skillNames[len(skillNames)-1])); err != nil {
		t.Fatal(err)
	}
	var a, b map[string]string
	service.afterLock = func() {
		if err := os.Rename(path, path+"-old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, receiptName), []byte("foreign"), 0600); err != nil {
			t.Fatal(err)
		}
		a, b = authoritySnapshot(t, path+"-old"), authoritySnapshot(t, path)
	}
	got := service.Install(context.Background())
	if got.Status != Failed || got.Category != "claude_skill_root_unavailable" {
		t.Fatalf("replacement evidence influenced ownership: %+v", got)
	}
	if !reflect.DeepEqual(a, authoritySnapshot(t, path+"-old")) || !reflect.DeepEqual(b, authoritySnapshot(t, path)) {
		t.Fatal("replacement mutated A or B")
	}
}

func TestUpgradeSessionInspectionAndCleanupRefuseReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills")
	service, _ := New(path)
	if got := service.Install(context.Background()); got.Status != Applied {
		t.Fatal(got)
	}
	name, leftover := skillNames[0], UpgradeStagePrefix+"test"
	if err := os.WriteFile(filepath.Join(path, name, leftover), []byte("stage"), 0600); err != nil {
		t.Fatal(err)
	}
	session, err := service.LockForUpgrade()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.Inspect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, name, leftover), []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	a, b := authoritySnapshot(t, path+"-old"), authoritySnapshot(t, path)
	if _, err := session.Inspect(context.Background()); err == nil {
		t.Fatal("replacement inventory accepted")
	}
	if err := session.RemoveSkillLeftover(name, leftover); err == nil {
		t.Fatal("replacement cleanup accepted")
	}
	if !reflect.DeepEqual(a, authoritySnapshot(t, path+"-old")) || !reflect.DeepEqual(b, authoritySnapshot(t, path)) {
		t.Fatal("replacement cleanup mutated A or B")
	}
}

func TestInstallCleanupPreservesReplacementChild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills")
	root, _, err := ensureRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	name := skillNames[0]
	calls := 0
	verify := func() error {
		calls++
		if calls == 2 {
			if err := root.Rename(name, name+"-old"); err != nil {
				t.Fatal(err)
			}
			if err := root.Mkdir(name, 0700); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}
	changed, _, err := codexIntegration.installOne(root, name, []byte("new skill"), verify)
	if changed || err == nil {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	for _, directory := range []string{name, name + "-old"} {
		entries, err := os.ReadDir(filepath.Join(path, directory))
		if err != nil || len(entries) != 0 {
			t.Fatalf("cleanup changed %s: %v %v", directory, entries, err)
		}
	}
}
