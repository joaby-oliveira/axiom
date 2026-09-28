package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type firstRunEvent struct {
	Status     string   `json:"status"`
	Result     string   `json:"result"`
	References []string `json:"references"`
	FirstRun   struct {
		Detected int `json:"detected"`
		Failed   int `json:"failed"`
		Runtimes []struct {
			Runtime                        string `json:"runtime"`
			Present                        bool   `json:"present"`
			ConfigurationWithoutExecutable bool   `json:"configurationWithoutExecutable"`
			State                          string `json:"state"`
			Reason                         string `json:"reason"`
			Skills                         []struct {
				Name, State string
			} `json:"skills"`
		} `json:"runtimes"`
	} `json:"firstRun"`
}

type firstRunMachine struct {
	t        *testing.T
	binary   string
	home     string
	bin      string
	executed string
	extra    []string
}

func newFirstRunMachine(t *testing.T, binary string, executables ...string) *firstRunMachine {
	t.Helper()
	m := &firstRunMachine{t: t, binary: binary, home: t.TempDir(), bin: t.TempDir(), executed: filepath.Join(t.TempDir(), "executed")}
	for _, name := range executables {
		// Discovery only resolves the executable; running it would leave a mark.
		if err := os.WriteFile(filepath.Join(m.bin, name), []byte("#!/bin/sh\ntouch "+m.executed+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func (m *firstRunMachine) run(wantCode int, args ...string) (firstRunEvent, string) {
	m.t.Helper()
	command := exec.Command(m.binary, args...)
	command.Dir = m.t.TempDir()
	command.Env = append([]string{"HOME=" + m.home, "PATH=" + m.bin, "CLAUDE_CONFIG_DIR="}, m.extra...)
	command.Env = append(command.Env, "LINGO_PROJECTS_ROOT="+filepath.Join(m.home, "projects"), "LINGO_STATE_ROOT="+filepath.Join(m.home, "state"))
	output, err := command.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		m.t.Fatal(err)
	}
	if code != wantCode {
		m.t.Fatalf("%v: code=%d want %d: %s", args, code, wantCode, output)
	}
	var event firstRunEvent
	if len(args) > 0 && args[0] == "--json" {
		if err := json.Unmarshal(bytes.TrimSpace(output), &event); err != nil {
			m.t.Fatalf("%v: invalid JSON %q: %v", args, output, err)
		}
	}
	return event, string(output)
}

func (m *firstRunMachine) runtime(event firstRunEvent, id string) (present bool, state, category string, stale bool) {
	m.t.Helper()
	for _, runtime := range event.FirstRun.Runtimes {
		if runtime.Runtime == id {
			return runtime.Present, runtime.State, runtime.Reason, runtime.ConfigurationWithoutExecutable
		}
	}
	m.t.Fatalf("runtime %s missing: %+v", id, event)
	return false, "", "", false
}

// files lists every path under HOME, so credential or Runtime-install
// effects outside the two skill roots would be visible.
func (m *firstRunMachine) files() []string {
	m.t.Helper()
	var paths []string
	_ = filepath.Walk(m.home, func(path string, info os.FileInfo, err error) error {
		if err == nil && path != m.home {
			paths = append(paths, strings.TrimPrefix(path, m.home))
		}
		return nil
	})
	sort.Strings(paths)
	return paths
}

func (m *firstRunMachine) assertNoRuntimeExecution() {
	m.t.Helper()
	if _, err := os.Lstat(m.executed); !os.IsNotExist(err) {
		m.t.Fatal("first-run executed a Runtime")
	}
}

// assertOnlySkillRoots proves the only effects are Axiom skill roots: no
// Runtime installation, credential, settings or secret files appear.
func (m *firstRunMachine) assertOnlySkillRoots(allowed ...string) {
	m.t.Helper()
	for _, path := range m.files() {
		ok := false
		for _, prefix := range allowed {
			ok = ok || path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(prefix, path+"/")
		}
		if !ok {
			m.t.Fatalf("unexpected effect under HOME: %s", path)
		}
	}
}

func buildFirstRunBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "axiom")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, output)
	}
	return binary
}

func TestExecutableFirstRunRuntimeMatrix(t *testing.T) {
	binary := buildFirstRunBinary(t)

	t.Run("neither", func(t *testing.T) {
		m := newFirstRunMachine(t, binary)
		event, _ := m.run(0, "--json", "first-run")
		if event.Status != "success" || event.Result != "No supported Runtime is currently available" || event.FirstRun.Detected != 0 || len(event.FirstRun.Runtimes) != 2 {
			t.Fatalf("event = %+v", event)
		}
		if len(m.files()) != 0 {
			t.Fatalf("zero-Runtime first run wrote %v", m.files())
		}
		_, human := m.run(0, "first-run")
		if !strings.Contains(human, "runtime: codex present=false state=absent") || !strings.Contains(human, "runtime: claude present=false state=absent") {
			t.Fatalf("human output = %s", human)
		}
	})

	t.Run("codex only", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "codex")
		event, _ := m.run(0, "--json", "first-run")
		if present, state, category, _ := m.runtime(event, "codex"); !present || state != "configured" || category != "codex_configured" {
			t.Fatalf("codex = %v %s %s", present, state, category)
		}
		if present, state, _, _ := m.runtime(event, "claude"); present || state != "absent" {
			t.Fatalf("claude = %v %s", present, state)
		}
		m.assertNoRuntimeExecution()
		m.assertOnlySkillRoots("/.agents/skills")
	})

	t.Run("claude only with default configuration root", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "claude")
		event, _ := m.run(0, "--json", "first-run")
		if present, state, _, _ := m.runtime(event, "claude"); !present || state != "configured" {
			t.Fatalf("claude = %+v", event)
		}
		if present, _, _, _ := m.runtime(event, "codex"); present {
			t.Fatal("codex reported present")
		}
		if _, err := os.Stat(filepath.Join(m.home, ".claude", "skills", "axiom-work-item-run", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		m.assertNoRuntimeExecution()
		m.assertOnlySkillRoots("/.claude/skills")
		status, _ := m.run(0, "--json", "runtime", "claude", "status")
		if status.Result != "Lingo and Claude skills are compatible" {
			t.Fatalf("claude status = %+v", status)
		}
	})

	t.Run("claude honors CLAUDE_CONFIG_DIR", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "claude")
		configuration := filepath.Join(t.TempDir(), "claude-work")
		m.extra = []string{"CLAUDE_CONFIG_DIR=" + configuration}
		event, _ := m.run(0, "--json", "first-run")
		if _, state, _, _ := m.runtime(event, "claude"); state != "configured" {
			t.Fatalf("event = %+v", event)
		}
		if _, err := os.Stat(filepath.Join(configuration, "skills", "axiom-project-show", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		if len(m.files()) != 0 {
			t.Fatalf("default ~/.claude was used: %v", m.files())
		}
	})

	t.Run("both converge and rerun is idempotent", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "codex", "claude")
		event, _ := m.run(0, "--json", "first-run")
		if event.FirstRun.Detected != 2 || event.FirstRun.Failed != 0 || strings.Join(event.References, ",") != "runtime:claude:configured,runtime:codex:configured" {
			t.Fatalf("event = %+v", event)
		}
		before := strings.Join(m.files(), "\n")
		again, _ := m.run(0, "--json", "first-run")
		if _, state, _, _ := m.runtime(again, "codex"); state != "already_configured" {
			t.Fatalf("rerun = %+v", again)
		}
		if _, state, _, _ := m.runtime(again, "claude"); state != "already_configured" {
			t.Fatalf("rerun = %+v", again)
		}
		if strings.Join(m.files(), "\n") != before {
			t.Fatal("rerun changed HOME")
		}
		m.assertNoRuntimeExecution()
		m.assertOnlySkillRoots("/.agents/skills", "/.claude/skills")
	})

	t.Run("one converges while the other conflicts", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "codex", "claude")
		foreign := filepath.Join(m.home, ".claude", "skills", "axiom-work-item-status", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(foreign), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(foreign, []byte("user skill\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		event, _ := m.run(1, "--json", "first-run")
		if event.Status != "partial" || event.FirstRun.Failed != 1 || event.FirstRun.Detected != 2 {
			t.Fatalf("event = %+v", event)
		}
		if _, state, category, _ := m.runtime(event, "claude"); state != "failed" || category != "claude_skill_conflict" {
			t.Fatalf("claude = %s %s", state, category)
		}
		if _, state, _, _ := m.runtime(event, "codex"); state != "configured" {
			t.Fatal("codex result was not preserved")
		}
		if content, _ := os.ReadFile(foreign); string(content) != "user skill\n" {
			t.Fatal("foreign Claude skill was overwritten")
		}
		if entries, _ := os.ReadDir(filepath.Dir(foreign)); len(entries) != 1 {
			t.Fatalf("foreign skill directory changed: %v", entries)
		}
	})

	t.Run("Runtime-created 0755 skill roots are configured", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "codex", "claude")
		userSkill := filepath.Join(m.home, ".claude", "skills", "my-skill", "SKILL.md")
		for _, directory := range []string{".claude", ".claude/skills", ".claude/skills/my-skill", ".agents", ".agents/skills"} {
			if err := os.Mkdir(filepath.Join(m.home, directory), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(m.home, directory), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(userSkill, []byte("user skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		event, _ := m.run(0, "--json", "first-run")
		for _, id := range []string{"codex", "claude"} {
			if _, state, category, _ := m.runtime(event, id); state != "configured" {
				t.Fatalf("%s = %s %s", id, state, category)
			}
		}
		if content, err := os.ReadFile(userSkill); err != nil || string(content) != "user skill\n" {
			t.Fatalf("user skill changed: %q %v", content, err)
		}
		for _, root := range []string{".claude/skills", ".agents/skills"} {
			info, err := os.Lstat(filepath.Join(m.home, root))
			if err != nil || info.Mode().Perm() != 0o755 {
				t.Fatalf("%s mode changed: %v %v", root, info, err)
			}
			skill, err := os.Lstat(filepath.Join(m.home, root, "axiom-work-item-run"))
			if err != nil || skill.Mode().Perm() != 0o700 {
				t.Fatalf("%s Axiom skill = %v %v", root, skill, err)
			}
		}
		again, _ := m.run(0, "--json", "first-run")
		for _, id := range []string{"codex", "claude"} {
			if _, state, _, _ := m.runtime(again, id); state != "already_configured" {
				t.Fatalf("%s rerun = %+v", id, again)
			}
		}
	})

	t.Run("group-writable skill root fails that Runtime unchanged", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "codex", "claude")
		root := filepath.Join(m.home, ".claude", "skills")
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o775); err != nil {
			t.Fatal(err)
		}
		event, _ := m.run(1, "--json", "first-run")
		if _, state, category, _ := m.runtime(event, "claude"); state != "failed" || category != "claude_skill_root_unavailable" {
			t.Fatalf("claude = %s %s", state, category)
		}
		if _, state, _, _ := m.runtime(event, "codex"); state != "configured" {
			t.Fatalf("codex = %+v", event)
		}
		if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
			t.Fatalf("unsafe root written: %v %v", entries, err)
		}
	})

	t.Run("configuration directory without executable is absent", func(t *testing.T) {
		m := newFirstRunMachine(t, binary)
		for _, directory := range []string{".claude", ".codex"} {
			if err := os.MkdirAll(filepath.Join(m.home, directory), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		event, _ := m.run(0, "--json", "first-run")
		for _, id := range []string{"codex", "claude"} {
			if present, state, _, stale := m.runtime(event, id); present || state != "absent" || !stale {
				t.Fatalf("%s = %+v", id, event)
			}
		}
		if strings.Join(m.files(), ",") != "/.claude,/.codex" {
			t.Fatalf("stale configuration changed: %v", m.files())
		}
	})

	t.Run("unsafe CLAUDE_CONFIG_DIR fails only Claude", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "codex", "claude")
		m.extra = []string{"CLAUDE_CONFIG_DIR=relative/claude"}
		event, _ := m.run(1, "--json", "first-run")
		if _, state, category, _ := m.runtime(event, "claude"); state != "failed" || category != "claude_skill_root_unavailable" {
			t.Fatalf("claude = %+v", event)
		}
		if _, state, _, _ := m.runtime(event, "codex"); state != "configured" {
			t.Fatalf("codex = %+v", event)
		}
	})

	t.Run("invalid usage keeps the canonical usage error", func(t *testing.T) {
		m := newFirstRunMachine(t, binary, "codex")
		_, output := m.run(1, "--json", "first-run", "--unexpected")
		if !strings.Contains(output, "invalid_command") {
			t.Fatalf("output = %s", output)
		}
		if len(m.files()) != 0 {
			t.Fatal("invalid usage had effects")
		}
	})
}
