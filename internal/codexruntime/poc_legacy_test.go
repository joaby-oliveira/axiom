package codexruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// The v0.1.0-poc.1 skill set is a known owned Axiom skill set: each file must
// be recognized as an older version the authorized install flow may replace.
func TestHistoricalPOCSkillSetIsKnownLegacy(t *testing.T) {
	fixture := filepath.Join("..", "compatibility", "testdata", "poc-v0.1.0-poc.1", "skills")
	root := filepath.Join(t.TempDir(), "skills")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range skillNames {
		wire, err := os.ReadFile(filepath.Join(fixture, name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(wire)
		if !contains(legacySkillDigests[name], hex.EncodeToString(digest[:])) {
			t.Fatalf("POC skill %s digest %x is not a known legacy version", name, digest)
		}
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "SKILL.md"), wire, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := service.Inventory(context.Background())
	if err != nil || inventory.State != SkillSetUpgradable {
		t.Fatalf("inventory=%+v err=%v", inventory, err)
	}
	if result := service.Install(context.Background()); result.Status != Applied {
		t.Fatalf("authorized install did not upgrade the POC skill set: %+v", result)
	}
	if inventory, _ := service.Inventory(context.Background()); inventory.State != SkillSetCurrent {
		t.Fatalf("post-install inventory=%+v", inventory)
	}
}

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}
