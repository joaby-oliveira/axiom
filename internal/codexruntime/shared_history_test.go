package codexruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/rgomids/axiom/internal/testfs"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// publishedSharedRevisions pins the manifest digest of every skill set Axiom
// published through the shared Runtime integration. Each must remain either
// the embedded skill set or an entry of sharedSkillHistory, so no Runtime
// loses ownership of a revision it received. Add the new embedded manifest
// digest here whenever the skill text changes.
var publishedSharedRevisions = []string{
	"98c861ec8f4448f5e63299d576a11fd41c6cf3f8d43270bb060bd77e9ba327ec",
	"9edcd211cdf251a9d8ffd6e20e89809ed2ae51ac5d54ce0b9c0d927431fa2a5f",
}

func TestEveryPublishedSharedRevisionStaysOwned(t *testing.T) {
	current, err := currentRevision()
	if err != nil {
		t.Fatal(err)
	}
	currentDigest, err := current.manifestDigest()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{currentDigest: true}
	for _, revision := range sharedSkillHistory {
		digest, err := revision.manifestDigest()
		if err != nil {
			t.Fatalf("incomplete shared revision: %v", err)
		}
		known[digest] = true
	}
	for _, digest := range publishedSharedRevisions {
		if !known[digest] {
			t.Fatalf("published skill set %s is neither embedded nor in sharedSkillHistory (embedded manifest is %s): append the replaced revision to sharedSkillHistory", digest, currentDigest)
		}
	}
	if publishedSharedRevisions[len(publishedSharedRevisions)-1] != currentDigest {
		t.Fatalf("embedded skill set %s is not pinned in publishedSharedRevisions", currentDigest)
	}
}

// The Codex-only history predates the shared history and is frozen, so a
// revision cannot be registered for Codex alone.
func TestRuntimeOnlyHistoryIsFrozen(t *testing.T) {
	count := 0
	for _, digests := range legacySkillDigests {
		count += len(digests)
	}
	if count != 28 || len(legacyReceiptWires) != 4 {
		t.Fatalf("Codex-only history changed (%d skills, %d receipts): register new revisions in sharedSkillHistory", count, len(legacyReceiptWires))
	}
	if len(claudeIntegration.legacySkills) != 0 || len(claudeIntegration.legacyReceipts) != 0 {
		t.Fatal("Claude gained a Runtime-only history")
	}
}

// nextRelease simulates the binary of Axiom N+1: every skill text changes and
// revision N, the embedded skill set, is appended to sharedSkillHistory as the
// release discipline requires. It returns the N+1 skill contents.
func nextRelease(t *testing.T) map[string][]byte {
	t.Helper()
	previous, err := currentRevision()
	if err != nil {
		t.Fatal(err)
	}
	next := fstest.MapFS{}
	contents := map[string][]byte{}
	for _, name := range skillNames {
		content, err := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		changed := append(append([]byte(nil), content...), []byte("\nRevision N+1 wording.\n")...)
		next["skills/"+name+"/SKILL.md"] = &fstest.MapFile{Data: changed, Mode: 0o600}
		contents[name] = changed
	}
	savedFiles, savedHistory := skillFiles, sharedSkillHistory
	skillFiles = next
	sharedSkillHistory = append(append([]skillSetRevision(nil), savedHistory...), previous)
	t.Cleanup(func() { skillFiles, sharedSkillHistory = savedFiles, savedHistory })
	return contents
}

func skillTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		line := strings.TrimPrefix(path, root) + " " + info.Mode().String()
		if info.Mode().IsRegular() {
			content, _ := os.ReadFile(path)
			sum := sha256.Sum256(content)
			line += " " + hex.EncodeToString(sum[:])
		}
		lines = append(lines, line)
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func installOrFail(t *testing.T, service Service, status Status) {
	t.Helper()
	if got := service.Install(context.Background()); got.Status != status {
		t.Fatalf("%s install = %#v, want %s", service.Runtime(), got, status)
	}
}

func assertConverged(t *testing.T, service Service, root string, contents map[string][]byte) {
	t.Helper()
	for name, content := range contents {
		actual, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
		if err != nil || string(actual) != string(content) {
			t.Fatalf("%s %s not at N+1: %v", service.Runtime(), name, err)
		}
	}
	if got := service.Inspect(context.Background()); got.Status != Ready {
		t.Fatalf("%s inspect = %#v", service.Runtime(), got)
	}
	receipt, err := service.integration.receipt(root)
	if err != nil || !matchesPrivateFile(filepath.Join(root, receiptName), receipt) {
		t.Fatalf("%s receipt not refreshed to N+1: %v", service.Runtime(), err)
	}
	if inventory, err := service.Inventory(context.Background()); err != nil || inventory.State != SkillSetCurrent {
		t.Fatalf("%s inventory = %#v, %v", service.Runtime(), inventory, err)
	}
}

func TestClaudeRevisionNConvergesAfterUpgradeToNPlusOne(t *testing.T) {
	root := filepath.Join(t.TempDir(), "claude", "skills")
	claude, err := NewClaude(root)
	if err != nil {
		t.Fatal(err)
	}
	installOrFail(t, claude, Applied)
	contents := nextRelease(t)
	if inventory, _ := claude.Inventory(context.Background()); inventory.State != SkillSetUpgradable || inventory.Receipt != "legacy" {
		t.Fatalf("revision N inventory under N+1 = %#v", inventory)
	}
	installOrFail(t, claude, Applied)
	assertConverged(t, claude, root, contents)
	before := skillTree(t, root)
	installOrFail(t, claude, Unchanged)
	if skillTree(t, root) != before {
		t.Fatal("idempotent rerun changed the Claude root")
	}
}

func TestModifiedClaudeRevisionNIsRefusedAfterUpgrade(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root string)
	}{
		{"user edit", func(t *testing.T, root string) {
			path := filepath.Join(root, "axiom-work-item-run", "SKILL.md")
			content, _ := os.ReadFile(path)
			if err := os.WriteFile(path, append(content, []byte("my local change\n")...), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"foreign skill", func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "axiom-work-item-run", "SKILL.md"), []byte("---\nname: axiom-work-item-run\n---\nforeign\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"ambiguous ownership", func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "axiom-work-item-run", "notes.md"), []byte("extra\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsafe mode", func(t *testing.T, root string) {
			if err := testfs.SharedMode(filepath.Join(root, "axiom-work-item-run", "SKILL.md"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "claude", "skills")
			claude, err := NewClaude(root)
			if err != nil {
				t.Fatal(err)
			}
			installOrFail(t, claude, Applied)
			test.mutate(t, root)
			before := skillTree(t, root)
			nextRelease(t)
			if got := claude.Install(context.Background()); got.Status != Failed || got.Category != "claude_skill_conflict" {
				t.Fatalf("install = %#v", got)
			}
			if skillTree(t, root) != before {
				t.Fatalf("refused upgrade changed the Claude root:\n%s\n---\n%s", before, skillTree(t, root))
			}
		})
	}
}

// Axiom N publishes both Runtimes; `axiom upgrade` to N+1 publishes only the
// Codex root and leaves its receipt at N; first-run with N+1 converges both.
func TestCodexAndClaudeConvergeTogetherAfterUpgrade(t *testing.T) {
	home := t.TempDir()
	codexRoot, claudeRoot := filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".claude", "skills")
	codex, err := New(codexRoot)
	if err != nil {
		t.Fatal(err)
	}
	claude, err := NewClaude(claudeRoot)
	if err != nil {
		t.Fatal(err)
	}
	installOrFail(t, codex, Applied)
	installOrFail(t, claude, Applied)
	previous, err := currentRevision()
	if err != nil {
		t.Fatal(err)
	}
	contents := nextRelease(t)
	session, err := codex.LockForUpgrade()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range skillNames {
		if err := session.PublishSkill(name, contents[name], previous.skills[name]); err != nil {
			t.Fatalf("upgrade publish %s: %v", name, err)
		}
	}
	session.Close()
	if got := codex.Inspect(context.Background()); got.Status != Partial {
		t.Fatalf("codex after upgrade = %#v, want stale receipt", got)
	}
	for _, service := range []Service{codex, claude} {
		installOrFail(t, service, Applied)
	}
	assertConverged(t, codex, codexRoot, contents)
	assertConverged(t, claude, claudeRoot, contents)
	codexBefore, claudeBefore := skillTree(t, codexRoot), skillTree(t, claudeRoot)
	for _, service := range []Service{codex, claude} {
		installOrFail(t, service, Unchanged)
	}
	if skillTree(t, codexRoot) != codexBefore || skillTree(t, claudeRoot) != claudeBefore {
		t.Fatal("idempotent rerun changed a Runtime root")
	}
}

// An interrupted Claude install or upgrade leaves some skills at N, one
// already at N+1, one missing and no receipt; the N+1 rerun converges it.
func TestPartialClaudeStateConvergesAfterUpgrade(t *testing.T) {
	root := filepath.Join(t.TempDir(), "claude", "skills")
	claude, err := NewClaude(root)
	if err != nil {
		t.Fatal(err)
	}
	installOrFail(t, claude, Applied)
	contents := nextRelease(t)
	if err := os.Remove(filepath.Join(root, receiptName)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "axiom-work-item-create")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "axiom-project-show", "SKILL.md"), contents["axiom-project-show"], 0o600); err != nil {
		t.Fatal(err)
	}
	installOrFail(t, claude, Applied)
	assertConverged(t, claude, root, contents)
	installOrFail(t, claude, Unchanged)
}

func TestClaudeReceiptMustBeAxiomEvidenceForThisRoot(t *testing.T) {
	previous, err := currentRevision()
	if err != nil {
		t.Fatal(err)
	}
	elsewhere, err := claudeReceiptBytes("/elsewhere/.claude/skills", previous)
	if err != nil {
		t.Fatal(err)
	}
	codexWire, err := codexReceiptBytes("", previous)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		wire []byte
		mode os.FileMode
	}{
		{"receipt of another root", elsewhere, 0o600},
		{"Codex receipt", codexWire, 0o600},
		{"foreign receipt", []byte("formatVersion=1\nruntime=claude\n"), 0o600},
		{"unsafe receipt mode", nil, 0o644},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "claude", "skills")
			claude, err := NewClaude(root)
			if err != nil {
				t.Fatal(err)
			}
			installOrFail(t, claude, Applied)
			path := filepath.Join(root, receiptName)
			if test.wire != nil {
				if err := os.WriteFile(path, test.wire, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := testfs.SharedMode(path, test.mode); err != nil {
				t.Fatal(err)
			}
			before := skillTree(t, root)
			nextRelease(t)
			if got := claude.Install(context.Background()); got.Status != Failed || got.Category != "claude_skill_conflict" {
				t.Fatalf("install = %#v", got)
			}
			if skillTree(t, root) != before {
				t.Fatal("skills or receipt changed beside an unrecognized receipt")
			}
		})
	}
}

// A pre-shared Codex-only revision is still not Claude-owned after an upgrade.
func TestSharedHistoryDoesNotAdoptCodexOnlyRevisionsForClaude(t *testing.T) {
	legacy, err := os.ReadFile(filepath.Join("..", "compatibility", "testdata", "poc-v0.1.0-poc.1", "skills", "axiom-project-show", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "claude", "skills")
	if err := os.MkdirAll(filepath.Join(root, "axiom-project-show"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "axiom-project-show", "SKILL.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	nextRelease(t)
	claude, err := NewClaude(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := claude.Install(context.Background()); got.Status != Failed || got.Category != "claude_skill_conflict" {
		t.Fatalf("install = %#v", got)
	}
	if content, _ := os.ReadFile(filepath.Join(root, "axiom-project-show", "SKILL.md")); string(content) != string(legacy) {
		t.Fatal("Codex-only revision was replaced in the Claude root")
	}
}
