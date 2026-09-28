package runtimebootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/codexruntime"
)

var skillNames = []string{"axiom-project-configure", "axiom-project-show", "axiom-work-item-create", "axiom-work-item-run", "axiom-work-item-status"}

type machine struct {
	home        string
	codexSkills string
	claudeRoot  string
	executables map[string]bool
	lookups     []string
}

func newMachine(t *testing.T, executables ...string) *machine {
	t.Helper()
	home := t.TempDir()
	m := &machine{home: home, codexSkills: filepath.Join(home, ".agents", "skills"), claudeRoot: filepath.Join(home, ".claude"), executables: map[string]bool{}}
	for _, name := range executables {
		m.executables[name] = true
	}
	return m
}

func (m *machine) lookPath(name string) (string, error) {
	m.lookups = append(m.lookups, name)
	if m.executables[name] {
		return filepath.Join("/opt/runtimes/bin", name), nil
	}
	return "", exec.ErrNotFound
}

func (m *machine) runtimes(t *testing.T) []Runtime {
	t.Helper()
	claudeSkills, err := ClaudeSkillsRoot(func(string) string { return "" }, m.home)
	if err != nil {
		t.Fatal(err)
	}
	return []Runtime{
		{ID: "codex", Executable: "codex", ConfigurationRoot: filepath.Join(m.home, ".codex"), Integration: func() (Integration, error) { return codexruntime.New(m.codexSkills) }},
		{ID: "claude", Executable: "claude", ConfigurationRoot: m.claudeRoot, Integration: func() (Integration, error) { return codexruntime.NewClaude(claudeSkills) }},
	}
}

func (m *machine) run(t *testing.T) Report {
	t.Helper()
	return Run(context.Background(), m.lookPath, m.runtimes(t))
}

func state(t *testing.T, report Report, id string) RuntimeReport {
	t.Helper()
	for _, runtime := range report.Runtimes {
		if runtime.ID == id {
			return runtime
		}
	}
	t.Fatalf("runtime %s not reported", id)
	return RuntimeReport{}
}

// tree digests every path below root, so a no-effect claim is checkable.
func tree(t *testing.T, root string) string {
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

func withoutLock(listing string) string {
	var kept []string
	for _, line := range strings.Split(listing, "\n") {
		if !strings.HasPrefix(line, "/skills/.axiom-skill-set.lock ") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func skillFile(root, name string) string { return filepath.Join(root, name, "SKILL.md") }

func writePrivate(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertConfigured(t *testing.T, root string) {
	t.Helper()
	for _, name := range skillNames {
		content, err := os.ReadFile(skillFile(root, name))
		if err != nil || !strings.Contains(string(content), "name: "+name) || !strings.Contains(string(content), "axiom --json") {
			t.Fatalf("%s not installed in %s: %v", name, root, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".axiom-skill-set.receipt")); err != nil {
		t.Fatalf("receipt missing in %s: %v", root, err)
	}
}

func TestNeitherRuntimeIsAValidStateWithNoEffects(t *testing.T) {
	m := newMachine(t)
	before := tree(t, m.home)
	report := m.run(t)
	if report.Detected() != 0 || report.Failed() != 0 {
		t.Fatalf("report = %+v", report)
	}
	for _, id := range []string{"codex", "claude"} {
		if got := state(t, report, id); got.Present || got.State != Absent || got.Category != id+"_absent" || got.Result != nil {
			t.Fatalf("%s = %+v", id, got)
		}
	}
	if tree(t, m.home) != before {
		t.Fatal("zero-Runtime first run changed the machine")
	}
	if strings.Join(m.lookups, ",") != "codex,claude" {
		t.Fatalf("lookups = %v", m.lookups)
	}
}

func TestCodexOnlyConfiguresCodexAndReportsClaudeAbsent(t *testing.T) {
	m := newMachine(t, "codex")
	report := m.run(t)
	if got := state(t, report, "codex"); got.State != Configured || got.Category != "codex_configured" {
		t.Fatalf("codex = %+v", got)
	}
	if got := state(t, report, "claude"); got.Present || got.State != Absent {
		t.Fatalf("claude = %+v", got)
	}
	assertConfigured(t, m.codexSkills)
	if _, err := os.Lstat(m.claudeRoot); !os.IsNotExist(err) {
		t.Fatal("absent Claude root was touched")
	}
}

func TestClaudeOnlyConfiguresClaudeGlobalSkillsAndReportsCodexAbsent(t *testing.T) {
	m := newMachine(t, "claude")
	report := m.run(t)
	if got := state(t, report, "claude"); got.State != Configured || got.Category != "claude_configured" {
		t.Fatalf("claude = %+v", got)
	}
	if got := state(t, report, "codex"); got.Present || got.State != Absent {
		t.Fatalf("codex = %+v", got)
	}
	skills := filepath.Join(m.claudeRoot, "skills")
	assertConfigured(t, skills)
	receipt, err := os.ReadFile(filepath.Join(skills, ".axiom-skill-set.receipt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"runtime=claude\n", "skillsRoot=" + skills + "\n", "skillSetVersion=2\n"} {
		if !strings.Contains(string(receipt), expected) {
			t.Fatalf("receipt lacks %q:\n%s", expected, receipt)
		}
	}
	for _, name := range skillNames {
		content, _ := os.ReadFile(skillFile(skills, name))
		sum := sha256.Sum256(content)
		if !strings.Contains(string(receipt), "skill."+name+"="+hex.EncodeToString(sum[:])+"\n") {
			t.Fatalf("receipt lacks %s digest", name)
		}
	}
	if _, err := os.Lstat(m.codexSkills); !os.IsNotExist(err) {
		t.Fatal("absent Codex root was touched")
	}
}

func TestBothRuntimesConvergeIndependentlyAndRerunIsIdempotent(t *testing.T) {
	m := newMachine(t, "codex", "claude")
	first := m.run(t)
	if first.Detected() != 2 || first.Failed() != 0 || state(t, first, "codex").State != Configured || state(t, first, "claude").State != Configured {
		t.Fatalf("first = %+v", first)
	}
	before := tree(t, m.home)
	second := m.run(t)
	if second.Failed() != 0 || state(t, second, "codex").Category != "codex_already_configured" || state(t, second, "claude").Category != "claude_already_configured" {
		t.Fatalf("second = %+v", second)
	}
	if tree(t, m.home) != before {
		t.Fatal("idempotent rerun changed the machine")
	}
}

func TestConfigurationDirectoryWithoutExecutableIsAbsentAndUntouched(t *testing.T) {
	m := newMachine(t)
	writePrivate(t, filepath.Join(m.claudeRoot, "settings.json"), "{}\n")
	writePrivate(t, filepath.Join(m.home, ".codex", "config.toml"), "\n")
	before := tree(t, m.home)
	report := m.run(t)
	for _, id := range []string{"codex", "claude"} {
		if got := state(t, report, id); got.Present || got.State != Absent || !got.ConfigurationWithoutExecutable {
			t.Fatalf("%s = %+v", id, got)
		}
	}
	if tree(t, m.home) != before {
		t.Fatal("stale configuration was changed")
	}
}

func TestExecutableWithoutConfigurationDirectoryIsPresent(t *testing.T) {
	m := newMachine(t, "claude", "codex")
	report := m.run(t)
	for _, id := range []string{"codex", "claude"} {
		if got := state(t, report, id); !got.Present || got.ConfigurationWithoutExecutable || got.State != Configured {
			t.Fatalf("%s = %+v", id, got)
		}
	}
	info, err := os.Stat(filepath.Join(m.claudeRoot, "skills"))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("Claude skill root = %v, %v", info, err)
	}
}

func TestRecognizedPreviousCodexSkillIsUpgraded(t *testing.T) {
	m := newMachine(t, "codex")
	legacy, err := os.ReadFile(filepath.Join("..", "compatibility", "testdata", "poc-v0.1.0-poc.1", "skills", "axiom-project-show", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	writePrivate(t, skillFile(m.codexSkills, "axiom-project-show"), string(legacy))
	report := m.run(t)
	if got := state(t, report, "codex"); got.State != Configured {
		t.Fatalf("codex = %+v", got)
	}
	assertConfigured(t, m.codexSkills)
}

func TestClaudeHasNoInventedHistory(t *testing.T) {
	// Content that is a previous Codex-owned Axiom revision was never
	// installed into a Claude root by Axiom, so it is not Claude-owned.
	m := newMachine(t, "claude")
	legacy, err := os.ReadFile(filepath.Join("..", "compatibility", "testdata", "poc-v0.1.0-poc.1", "skills", "axiom-project-show", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	path := skillFile(filepath.Join(m.claudeRoot, "skills"), "axiom-project-show")
	writePrivate(t, path, string(legacy))
	report := m.run(t)
	if got := state(t, report, "claude"); got.State != Failed || got.Category != "claude_skill_conflict" {
		t.Fatalf("claude = %+v", got)
	}
	if content, _ := os.ReadFile(path); string(content) != string(legacy) {
		t.Fatal("unowned Claude skill was overwritten")
	}
}

func TestForeignSkillIsPreservedAndFailsThatRuntimeOnly(t *testing.T) {
	m := newMachine(t, "codex", "claude")
	path := skillFile(filepath.Join(m.claudeRoot, "skills"), "axiom-work-item-run")
	writePrivate(t, path, "---\nname: axiom-work-item-run\n---\nforeign\n")
	before := tree(t, m.claudeRoot)
	report := m.run(t)
	if got := state(t, report, "claude"); got.State != Failed || got.Category != "claude_skill_conflict" {
		t.Fatalf("claude = %+v", got)
	}
	if got := state(t, report, "codex"); got.State != Configured {
		t.Fatalf("codex result was not preserved: %+v", got)
	}
	if report.Failed() != 1 || report.Detected() != 2 {
		t.Fatalf("report = %+v", report)
	}
	// Only the persistent Axiom install lock (the existing flock contract)
	// may appear; the foreign skill and everything else stay as they were.
	if after := withoutLock(tree(t, m.claudeRoot)); after != before {
		t.Fatalf("conflicting Claude root changed:\n%s\n---\n%s", before, after)
	}
	assertConfigured(t, m.codexSkills)
}

func TestModifiedOwnedSkillIsNotRepairedDespiteReceipt(t *testing.T) {
	m := newMachine(t, "claude")
	if got := state(t, m.run(t), "claude"); got.State != Configured {
		t.Fatalf("install = %+v", got)
	}
	skills := filepath.Join(m.claudeRoot, "skills")
	path := skillFile(skills, "axiom-project-configure")
	content, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(content, []byte("user edit\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	before := tree(t, skills)
	report := m.run(t)
	if got := state(t, report, "claude"); got.State != Failed || got.Category != "claude_skill_conflict" {
		t.Fatalf("claude = %+v", got)
	}
	if tree(t, skills) != before {
		t.Fatal("modified owned skill or receipt changed")
	}
}

func TestIntegrationUnavailableFailsOnlyThatRuntime(t *testing.T) {
	m := newMachine(t, "codex", "claude")
	runtimes := m.runtimes(t)
	runtimes[1].Integration = func() (Integration, error) { return nil, ErrUnsafeConfigurationRoot }
	report := Run(context.Background(), m.lookPath, runtimes)
	if got := state(t, report, "claude"); got.State != Failed || got.Category != "claude_skill_root_unavailable" {
		t.Fatalf("claude = %+v", got)
	}
	if state(t, report, "codex").State != Configured {
		t.Fatal("codex did not converge")
	}
}

func TestRelativePathResolutionIsAbsence(t *testing.T) {
	report := Run(context.Background(), func(string) (string, error) { return "codex", exec.ErrDot }, []Runtime{{ID: "codex", Executable: "codex"}})
	if report.Runtimes[0].Present {
		t.Fatal("relative PATH match counted as present")
	}
	report = Run(context.Background(), func(string) (string, error) { return "bin/codex", nil }, []Runtime{{ID: "codex", Executable: "codex"}})
	if report.Runtimes[0].Present {
		t.Fatal("relative executable path counted as present")
	}
}

func TestClaudeConfigurationRootResolution(t *testing.T) {
	none := func(string) string { return "" }
	if root, err := ClaudeSkillsRoot(none, "/home/u"); err != nil || root != "/home/u/.claude/skills" {
		t.Fatalf("default = %q, %v", root, err)
	}
	override := func(key string) string {
		if key == "CLAUDE_CONFIG_DIR" {
			return "/srv/claude-work/"
		}
		return ""
	}
	if root, err := ClaudeSkillsRoot(override, "/home/u"); err != nil || root != "/srv/claude-work/skills" {
		t.Fatalf("override = %q, %v", root, err)
	}
	for _, value := range []string{"relative/claude", "/", "/tmp/a\nb"} {
		bad := func(string) string { return value }
		if _, err := ClaudeSkillsRoot(bad, "/home/u"); !errors.Is(err, ErrUnsafeConfigurationRoot) {
			t.Fatalf("%q accepted", value)
		}
	}
	if _, err := ClaudeSkillsRoot(none, ""); !errors.Is(err, ErrUnsafeConfigurationRoot) {
		t.Fatal("missing home accepted")
	}
}

func TestPartialPriorOwnedInstallConverges(t *testing.T) {
	m := newMachine(t, "claude", "codex")
	if report := m.run(t); report.Failed() != 0 {
		t.Fatalf("install = %+v", report)
	}
	claudeSkills := filepath.Join(m.claudeRoot, "skills")
	// One owned skill removed, and an install interrupted before its receipt.
	if err := os.RemoveAll(filepath.Join(claudeSkills, "axiom-work-item-create")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(m.codexSkills, ".axiom-skill-set.receipt")); err != nil {
		t.Fatal(err)
	}
	report := m.run(t)
	if report.Failed() != 0 || state(t, report, "claude").State != Configured || state(t, report, "codex").State != Configured {
		t.Fatalf("rerun = %+v", report)
	}
	assertConfigured(t, claudeSkills)
	assertConfigured(t, m.codexSkills)
}
