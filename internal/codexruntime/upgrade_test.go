package codexruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func upgradeRoot(t *testing.T) (Service, string) {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "skills")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return service, root
}

func TestPublishUpgradeSkillRequiresExpectedRevision(t *testing.T) {
	service, root := upgradeRoot(t)
	session, err := service.LockForUpgrade()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	name := skillNames[0]
	if err := session.PublishSkill(name, []byte("first\n"), ""); err != nil {
		t.Fatalf("create absent skill: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, name))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("skill directory mode=%v err=%v", info.Mode(), err)
	}
	if err := session.PublishSkill(name, []byte("second\n"), digestOf([]byte("other\n"))); err != ErrUpgradeConflict {
		t.Fatalf("mismatched expected revision published: %v", err)
	}
	if err := session.PublishSkill(name, []byte("second\n"), ""); err != ErrUpgradeConflict {
		t.Fatalf("absent expectation replaced a present skill: %v", err)
	}
	if err := session.PublishSkill("foreign-skill", []byte("x\n"), ""); err != ErrUpgradeConflict {
		t.Fatalf("non-Axiom skill name accepted: %v", err)
	}
	if err := session.PublishSkill(name, []byte("second\n"), digestOf([]byte("first\n"))); err != nil {
		t.Fatalf("owned replacement: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, name))
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging left behind: %v %v", entries, err)
	}
	inventory, err := service.InspectUpgrade(context.Background())
	if err != nil || !inventory.Configured || inventory.Skills[0].SHA256 != digestOf([]byte("second\n")) || inventory.Skills[0].Owned {
		t.Fatalf("inventory=%+v err=%v", inventory, err)
	}
}

func TestInspectUpgradeReportsLeftoversAndRefusesUnknownEntries(t *testing.T) {
	service, root := upgradeRoot(t)
	directory := filepath.Join(root, skillNames[1])
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(directory, UpgradeStagePrefix+"interrupted")
	if err := os.WriteFile(stage, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	inventory, err := service.InspectUpgrade(context.Background())
	if err != nil || !inventory.Configured || inventory.Skills[1].SHA256 != "" || len(inventory.Skills[1].Leftovers) != 1 || inventory.Skills[1].Leftovers[0] != stage {
		t.Fatalf("inventory=%+v err=%v", inventory, err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".axiom-skill-update"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.InspectUpgrade(context.Background()); err != ErrUpgradeConflict {
		t.Fatalf("unknown entry accepted: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	absent, err := New(missing)
	if err != nil {
		t.Fatal(err)
	}
	if inventory, err := absent.InspectUpgrade(context.Background()); err != nil || inventory.Configured {
		t.Fatalf("absent root inventory=%+v err=%v", inventory, err)
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatal("inspection created the root")
	}
}
