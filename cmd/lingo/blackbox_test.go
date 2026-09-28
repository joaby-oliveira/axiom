package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestExecutableGuidedProjectConfiguration(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "lingo")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build executable: %v: %s", err, output)
	}
	portable := filepath.Join(t.TempDir(), "portable")
	state := filepath.Join(t.TempDir(), "state")
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "--json", "project", "configure")
	command.Env = append(os.Environ(), "LINGO_PROJECTS_ROOT="+portable, "LINGO_STATE_ROOT="+state)
	command.Stdin = strings.NewReader("guided\nGuided Project\nmain=" + repository + "\n\ngithub\nyes\n")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("guided configure: %v: stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	var event canonicalEvent
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &event); err != nil || event.Status != "success" || event.Result != "Project setup published" {
		t.Fatalf("guided event = %+v, %v; output=%s", event, err, stdout.String())
	}
	if !strings.Contains(stderr.String(), "Repository key=absolute-path") || !strings.Contains(stderr.String(), `"digest"`) {
		t.Fatalf("guided prompts missing: %q", stderr.String())
	}
}

func TestExecutableVersionHumanJSONAndBuildProvenance(t *testing.T) {
	releaseBinary := filepath.Join(t.TempDir(), "lingo-release")
	build := exec.Command("go", "build", "-ldflags", "-X main.buildVersion=1.2.3 -X main.buildRevision=abc123def456 -X main.buildSourceState=clean -X main.buildRelease=true", "-o", releaseBinary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build release executable: %v: %s", err, output)
	}
	human, err := exec.Command(releaseBinary, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("human version: %v: %s", err, human)
	}
	for _, expected := range []string{"status: success", "result: Axiom build information", "provenance: Axiom 1.2.3 revision=abc123def456 source=clean"} {
		if !bytes.Contains(human, []byte(expected)) {
			t.Fatalf("human version missing %q: %s", expected, human)
		}
	}
	structured, err := exec.Command(releaseBinary, "--json", "version").CombinedOutput()
	if err != nil {
		t.Fatalf("JSON version: %v: %s", err, structured)
	}
	var release canonicalEvent
	if err := json.Unmarshal(bytes.TrimSpace(structured), &release); err != nil {
		t.Fatalf("JSON version: %v: %s", err, structured)
	}
	if release.Status != "success" || release.Result != "Axiom build information" || release.Provenance.Product != "Axiom" || release.Provenance.Version != "1.2.3" || release.Provenance.Revision != "abc123def456" || release.Provenance.SourceState != "clean" {
		t.Fatalf("release provenance = %+v", release)
	}

	dirtyBinary := filepath.Join(t.TempDir(), "lingo-dirty")
	build = exec.Command("go", "build", "-ldflags", "-X main.buildVersion=development -X main.buildRevision=def456abc123 -X main.buildSourceState=dirty -X main.buildRelease=false", "-o", dirtyBinary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build dirty executable: %v: %s", err, output)
	}
	dirtyOutput, err := exec.Command(dirtyBinary, "--json", "version").CombinedOutput()
	if err != nil {
		t.Fatalf("dirty version: %v: %s", err, dirtyOutput)
	}
	var dirty canonicalEvent
	if err := json.Unmarshal(bytes.TrimSpace(dirtyOutput), &dirty); err != nil {
		t.Fatalf("dirty JSON version: %v: %s", err, dirtyOutput)
	}
	if dirty.Provenance.Version != "development" || dirty.Provenance.Revision != "def456abc123" || dirty.Provenance.SourceState != "dirty" {
		t.Fatalf("dirty provenance = %+v", dirty.Provenance)
	}
}

type cliEvent struct {
	Operation string `json:"operation"`
	Status    string `json:"status"`
	Category  string `json:"category"`
	Project   *struct {
		Slug         string `json:"slug"`
		Repositories []struct {
			Key, Path string
		} `json:"repositories"`
	} `json:"project"`
	Workflow *struct {
		Status, CurrentGate, RepositoryPath string
	} `json:"workflow"`
	WorkItem *struct {
		URL, State, ExternalID string
	} `json:"workItem"`
}

type canonicalEvent struct {
	Status     string   `json:"status"`
	Result     string   `json:"result"`
	References []string `json:"references"`
	Provenance struct {
		Product, Version, Revision, SourceState string
	} `json:"provenance"`
	Setup struct {
		ProjectID string   `json:"projectId"`
		Digest    string   `json:"digest"`
		Effects   []string `json:"effects"`
	} `json:"setup"`
	Draft *struct {
		Digest string `json:"digest"`
	} `json:"draft"`
	Selection *struct {
		Digest string `json:"digest"`
	} `json:"selection"`
	WorkItem *struct {
		URL, State, ExternalID string
	} `json:"workItem"`
	Workflow *struct {
		ExecutionID, Status, CurrentGate, RepositoryKey string
		Revision                                        uint64
		WorkItem                                        struct{ Resource string }
	} `json:"workflow"`
	Projection *struct {
		Digest, ProjectionKey string
		Effects               []struct{ Kind, Value string }
	} `json:"projection"`
}

func TestExecutableRejectsSingleHyphenSelectorFlagsBeforeEffects(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "lingo")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build executable: %v: %s", err, output)
	}
	surfaces := []struct{ command, flags string }{
		{"project show", "selector"},
		{"project resolve", "selector"},
		{"project configure", "project-id slug"},
		{"work-item create", "project repository provider-repository"},
		{"work-item select", "project repository work-item"},
		{"work-item show", "project repository work-item"},
		{"work-item comment", "project repository work-item"},
		{"work-item complete", "project repository work-item"},
		{"workflow start", "project repository work-item execution"},
		{"workflow advance", "project repository work-item execution"},
		{"workflow resume", "project repository work-item execution"},
		{"workflow status", "project repository work-item execution"},
		{"workflow evidence", "project repository work-item execution"},
		{"workflow reconcile", "project repository work-item execution"},
	}
	var cases [][]string
	for _, surface := range surfaces {
		for _, name := range strings.Fields(surface.flags) {
			for _, suffix := range [][]string{
				{"-" + name, "alpha"},
				{"-" + name + "=alpha"},
				{"-" + name, "alpha", "-" + name, "beta"},
				{"--" + name, "alpha", "-" + name, "beta"},
				{"--" + name, "alpha", "--" + name, "beta"},
				{"--" + name + "=alpha", "--" + name + "=beta"},
			} {
				cases = append(cases, append(strings.Fields(surface.command), suffix...))
			}
		}
	}
	cases = append(cases,
		[]string{"project", "configure", "-repository", "main=/tmp/main"},
		[]string{"project", "configure", "--repository", "main=/tmp/main", "-repository", "other=/tmp/other"},
		[]string{"project", "configure", "--unknown", "value"},
		[]string{"work-item", "create", "--unknown", "value"},
		[]string{"work-item", "show", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7", "--number", "7"},
		[]string{"workflow", "status", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7;touch", "--execution", "execution"},
	)
	for _, existing := range []bool{false, true} {
		t.Run(strconv.FormatBool(existing), func(t *testing.T) {
			root := t.TempDir()
			portable, state := filepath.Join(root, "portable"), filepath.Join(root, "state")
			skills, home, cwd := filepath.Join(root, "skills"), filepath.Join(root, "home"), filepath.Join(root, "unrelated-cwd")
			bin := filepath.Join(root, "bin")
			for _, path := range []string{home, cwd, bin} {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			for name, ledger := range map[string]string{"gh": "provider-ledger", "codex": "runtime-ledger", "git": "git-ledger"} {
				script := "#!/bin/sh\nprintf called >>\"$AXIOM_TEST_LEDGER_ROOT/" + ledger + "\"\nexit 1\n"
				if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if existing {
				// Byte sentinels test preservation, not validity of domain records.
				for _, path := range []string{
					filepath.Join(portable, "preserved.json"),
					filepath.Join(state, "executions", "v1", "preserved.json"),
					filepath.Join(skills, "preserved.md"),
					filepath.Join(root, "provider-ledger"),
					filepath.Join(root, "runtime-ledger"),
					filepath.Join(root, "git-ledger"),
				} {
					if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("preserved\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := snapshotTrees(t, root)
			for _, args := range cases {
				t.Run(strings.Join(args, " "), func(t *testing.T) {
					command := exec.Command(binary, append([]string{"--json"}, args...)...)
					command.Dir = cwd
					command.Env = append(os.Environ(), "HOME="+home, "PATH="+bin, "LINGO_PROJECTS_ROOT="+portable, "LINGO_STATE_ROOT="+state, "AXIOM_CODEX_SKILLS_ROOT="+skills, "AXIOM_GH_BIN="+filepath.Join(bin, "gh"), "AXIOM_TEST_LEDGER_ROOT="+root)
					var output, prompts bytes.Buffer
					command.Stdout, command.Stderr = &output, &prompts
					err := command.Run()
					exit, ok := err.(*exec.ExitError)
					if !ok || exit.ExitCode() != 1 {
						t.Fatalf("err=%v output=%s", err, output.String())
					}
					var event struct{ Status, Result, Next string }
					if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil || event.Status != "validation_failure" || event.Result != "Explicit selector input is invalid" || event.Next != "Remove unknown, duplicate, or conflicting inputs and retry" || prompts.Len() != 0 {
						t.Fatalf("event=%+v err=%v output=%s prompts=%s", event, err, output.String(), prompts.String())
					}
					if after := snapshotTrees(t, root); !bytes.Equal(before, after) {
						t.Fatal("invalid input changed portable/local/Execution/skills roots or Provider/Runtime/Git ledgers")
					}
				})
			}
		})
	}
}

func TestExecutableProjectConfigureRepeatableRepository(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "lingo")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, output)
	}
	main, other := filepath.Join(root, "main"), filepath.Join(root, "other")
	for _, repository := range []string{main, other} {
		if err := os.Mkdir(repository, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotTrees(t, root)
	command := exec.Command(binary, "--json", "project", "configure", "--slug", "alpha", "--name", "Alpha", "--repository", "main="+main, "--repository=other="+other, "--work-item-provider", "none")
	command.Env = append(os.Environ(), "LINGO_PROJECTS_ROOT="+filepath.Join(root, "portable"), "LINGO_STATE_ROOT="+filepath.Join(root, "state"), "AXIOM_CODEX_SKILLS_ROOT="+filepath.Join(root, "skills"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("configure: %v: %s", err, output)
	}
	var event struct {
		Status string
		Setup  struct {
			Repositories []struct{ Key, LocalPath string }
		}
	}
	if err := json.Unmarshal(output, &event); err != nil || event.Status != "success" || len(event.Setup.Repositories) != 2 {
		t.Fatalf("event=%+v err=%v", event, err)
	}
	if event.Setup.Repositories[0].Key != "main" || event.Setup.Repositories[0].LocalPath != main || event.Setup.Repositories[1].Key != "other" || event.Setup.Repositories[1].LocalPath != other {
		t.Fatalf("repositories=%+v", event.Setup.Repositories)
	}
	if after := snapshotTrees(t, root); !bytes.Equal(before, after) {
		t.Fatal("configuration preview changed state")
	}
}

func TestExecutableMinimalLifecycleAndFailurePaths(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "lingo")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build executable: %v: %s", err, output)
	}
	portable := filepath.Join(t.TempDir(), "portable")
	state := filepath.Join(t.TempDir(), "state")
	skills := filepath.Join(t.TempDir(), "skills")
	unrelatedCWD := t.TempDir()
	// Runtime discovery reads PATH, HOME and CLAUDE_CONFIG_DIR: isolate all
	// three so first-run can never see or touch the host's real Runtimes.
	home := t.TempDir()
	runtimeBin := t.TempDir()
	executed := filepath.Join(t.TempDir(), "runtime-executed")
	environment := append(os.Environ(), "HOME="+home, "PATH="+runtimeBin, "CLAUDE_CONFIG_DIR=", "LINGO_PROJECTS_ROOT="+portable, "LINGO_STATE_ROOT="+state, "AXIOM_CODEX_SKILLS_ROOT="+skills)
	run := func(wantCode int, wantStatus, wantCategory string, args ...string) cliEvent {
		t.Helper()
		command := exec.Command(binary, append([]string{"--json"}, args...)...)
		command.Env = environment
		command.Dir = unrelatedCWD
		output, err := command.CombinedOutput()
		code := 0
		if err != nil {
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("%v: process error: %v", args, err)
			}
			code = exit.ExitCode()
		}
		var event cliEvent
		if err := json.Unmarshal(bytes.TrimSpace(output), &event); err != nil {
			t.Fatalf("%v: invalid JSON event %q: %v", args, output, err)
		}
		if code != wantCode || event.Status != wantStatus || event.Category != wantCategory {
			t.Fatalf("%v: code=%d event=%+v", args, code, event)
		}
		return event
	}
	runCanonical := func(wantCode int, wantStatus, wantResult string, args ...string) canonicalEvent {
		t.Helper()
		command := exec.Command(binary, append([]string{"--json"}, args...)...)
		command.Env = environment
		command.Dir = unrelatedCWD
		output, err := command.CombinedOutput()
		code := 0
		if err != nil {
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("%v: process error: %v", args, err)
			}
			code = exit.ExitCode()
		}
		var event canonicalEvent
		if err := json.Unmarshal(bytes.TrimSpace(output), &event); err != nil {
			t.Fatalf("%v: invalid canonical JSON %q: %v", args, output, err)
		}
		if code != wantCode || event.Status != wantStatus || event.Result != wantResult || event.Provenance.Product != "Axiom" {
			t.Fatalf("%v: code=%d event=%+v", args, code, event)
		}
		return event
	}
	runCanonical(0, "success", "No supported Runtime is currently available", "first-run")
	if _, err := os.Lstat(skills); !os.IsNotExist(err) {
		t.Fatal("first-run without a Runtime touched the Codex skill root")
	}
	runCanonical(1, "validation_failure", "Codex skill compatibility is not ready", "runtime", "codex", "status")
	// A Runtime is present when its executable resolves; it is never run.
	if err := os.WriteFile(filepath.Join(runtimeBin, "codex"), []byte("#!/bin/sh\ntouch "+executed+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCanonical(0, "success", "Axiom integration is configured for every detected Runtime", "first-run")
	runCanonical(0, "success", "Lingo and Codex skills are compatible", "runtime", "codex", "status")
	run(0, "success", "codex_already_configured", "runtime", "codex", "install")
	runCanonical(0, "success", "Axiom integration is configured for every detected Runtime", "first-run")
	if _, err := os.Lstat(executed); !os.IsNotExist(err) {
		t.Fatal("first-run executed the Runtime")
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatal("first-run touched the absent Claude configuration")
	}
	installed, err := filepath.Glob(filepath.Join(skills, "axiom-*", "SKILL.md"))
	if err != nil || len(installed) != 5 {
		t.Fatalf("installed Codex skills = %v, %v", installed, err)
	}
	repository := filepath.Join(t.TempDir(), "configured-repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	referenceContent := []byte("S6 synthetic lifecycle reference\n")
	if err := os.WriteFile(filepath.Join(repository, "s6-reference.md"), referenceContent, 0o600); err != nil {
		t.Fatal(err)
	}
	referenceDigest := fmt.Sprintf("%x", sha256.Sum256(referenceContent))
	factArgs := func(revision uint64, fact, kind string) []string {
		return []string{"workflow", "fact", "--project", "configured", "--repository", "main", "--number", "7", "--expected-revision", strconv.FormatUint(revision, 10), "--fact", fact, "--active", "--reference", kind + ":s6-reference.md:" + referenceDigest, "--authorize-local"}
	}
	ghBinary := filepath.Join(t.TempDir(), "gh")
	createCount := filepath.Join(t.TempDir(), "create-count")
	if err := os.WriteFile(createCount, []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ghScript := `#!/bin/sh
case "$*" in
  *search/issues*) printf '%s\n' '{"total_count":0,"items":[]}' ;;
  *issues/7/comments*) printf '%s\n' '{"id":1}' ;;
  *PATCH*issues/7*) printf '%s\n' '{"number":7,"html_url":"https://github.com/owner/repo/issues/7","state":"closed"}' ;;
  *issues/7*) printf '%s\n' '{"number":7,"html_url":"https://github.com/owner/repo/issues/7","state":"open"}' ;;
  *issues/8*) printf '%s\n' '{"number":8,"html_url":"https://github.com/owner/repo/issues/8","state":"open"}' ;;
  *POST*repos/owner/ambiguous/issues*) count=$(awk '{print $1}' "$AXIOM_TEST_CREATE_COUNT"); count=$((count + 1)); printf '%s\n' "$count" > "$AXIOM_TEST_CREATE_COUNT"; cat >/dev/null; exit 1 ;;
  *POST*issues*) cat >/dev/null; printf '%s\n' '{"number":7,"html_url":"https://github.com/owner/repo/issues/7","state":"open"}' ;;
  *) exit 1 ;;
esac
exit 0
`
	if err := os.WriteFile(ghBinary, []byte(ghScript), 0o700); err != nil {
		t.Fatal(err)
	}
	environment = append(environment, "AXIOM_GH_BIN="+ghBinary, "AXIOM_TEST_CREATE_COUNT="+createCount)
	preview := runCanonical(0, "success", "Project setup preview ready", "project", "configure", "--slug", "configured", "--name", "Configured", "--repository", "main="+repository, "--work-item-provider", "github")
	if preview.Setup.ProjectID == "" || preview.Setup.Digest == "" {
		t.Fatalf("setup preview = %+v", preview.Setup)
	}
	runCanonical(0, "success", "Project setup published", "project", "configure", "--project-id", preview.Setup.ProjectID, "--slug", "configured", "--name", "Configured", "--repository", "main="+repository, "--work-item-provider", "github", "--preview-digest", preview.Setup.Digest, "--authorize-local")
	raceRepository := filepath.Join(t.TempDir(), "race-repository")
	if err := os.Mkdir(raceRepository, 0o700); err != nil {
		t.Fatal(err)
	}
	racePreview := runCanonical(0, "success", "Project setup preview ready", "project", "configure", "--slug", "race", "--name", "Race", "--repository", "main="+raceRepository, "--work-item-provider", "github")
	raceArgs := []string{"--json", "project", "configure", "--project-id", racePreview.Setup.ProjectID, "--slug", "race", "--name", "Race", "--repository", "main=" + raceRepository, "--work-item-provider", "github", "--preview-digest", racePreview.Setup.Digest, "--authorize-local"}
	commands := []*exec.Cmd{exec.Command(binary, raceArgs...), exec.Command(binary, raceArgs...)}
	outputs := []*bytes.Buffer{{}, {}}
	for index, command := range commands {
		command.Env = environment
		command.Stdout = outputs[index]
		command.Stderr = outputs[index]
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range commands {
		_ = command.Wait()
	}
	statuses := map[string]int{}
	for _, output := range outputs {
		var event canonicalEvent
		if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil {
			t.Fatalf("concurrent setup output %q: %v", output.String(), err)
		}
		statuses[event.Status]++
	}
	losers := statuses["denied_authority"] + statuses["failure"]
	if statuses["success"] != 1 || losers != 1 {
		t.Fatalf("concurrent setup statuses = %v; outputs=%q / %q", statuses, outputs[0], outputs[1])
	}
	run(0, "success", "project_resolved", "project", "resolve", "--selector", "configured")
	shown := runCanonical(0, "success", "Project resolved", "project", "show", "--selector", "configured")
	if len(shown.References) != 2 || !strings.HasPrefix(shown.References[0], "project:") || shown.References[1] != "repository:main" {
		t.Fatalf("project references = %+v", shown.References)
	}
	draftArgs := []string{"work-item", "create", "--project", "configured", "--repository", "main", "--provider-repository", "owner/repo", "--intent", "Delivery is blocked", "--desired-outcome", "Delivery proceeds", "--context", "Supported host", "--scope", "Bounded change", "--constraints", "Preserve authority", "--non-goals", "No workflow", "--acceptance", "Tests pass"}
	draft := runCanonical(0, "success", "Work Item draft ready for review", draftArgs...)
	if draft.Draft == nil || draft.Draft.Digest == "" {
		t.Fatalf("draft preview = %+v", draft.Draft)
	}
	runCanonical(1, "denied_authority", "Work Item authority denied", append(draftArgs, "--preview-digest", "stale", "--authorize-external")...)
	created := runCanonical(0, "success", "GitHub Work Item linked", append(draftArgs, "--preview-digest", draft.Draft.Digest, "--authorize-external")...)
	if created.WorkItem == nil || created.WorkItem.ExternalID != "7" || created.WorkItem.State != "OPEN" {
		t.Fatalf("work item payload = %+v", created.WorkItem)
	}
	ambiguousArgs := []string{"work-item", "create", "--project", "configured", "--repository", "main", "--provider-repository", "owner/ambiguous", "--intent", "Ambiguous delivery", "--desired-outcome", "Safe reconciliation", "--context", "Separate process", "--scope", "Bounded change", "--constraints", "No duplicate", "--non-goals", "No workflow", "--acceptance", "One POST"}
	ambiguousDraft := runCanonical(0, "success", "Work Item draft ready for review", ambiguousArgs...)
	for range 2 {
		runCanonical(1, "retryable_failure", "GitHub create result is ambiguous", append(ambiguousArgs, "--preview-digest", ambiguousDraft.Draft.Digest, "--authorize-external")...)
	}
	count, err := os.ReadFile(createCount)
	if err != nil || strings.TrimSpace(string(count)) != "1" {
		t.Fatalf("ambiguous create count = %q, %v", count, err)
	}
	runCanonical(0, "success", "Work Item link loaded", "work-item", "show", "--project", "configured", "--repository", "main", "--number", "7")
	selection := runCanonical(0, "success", "GitHub Work Item selection ready for review", "work-item", "select", "--project", "configured", "--repository", "main", "--provider-repository", "owner/repo", "--number", "8")
	if selection.Selection == nil || selection.Selection.Digest == "" {
		t.Fatalf("selection preview = %+v", selection.Selection)
	}
	selected := runCanonical(0, "success", "GitHub Work Item linked", "work-item", "select", "--project", "configured", "--repository", "main", "--provider-repository", "owner/repo", "--number", "8", "--preview-digest", selection.Selection.Digest, "--authorize-local")
	if selected.WorkItem == nil || selected.WorkItem.ExternalID != "8" {
		t.Fatalf("selected item = %+v", selected.WorkItem)
	}
	runCanonical(0, "success", "Historical Work Item comment completed", "work-item", "comment", "--project", "configured", "--repository", "main", "--number", "7", "--message", "Evidence", "--authorize-external")
	started := runCanonical(0, "success", "Execution workflow operation completed", "workflow", "start", "--project", "configured", "--repository", "main", "--number", "7")
	if started.Workflow == nil || started.Workflow.ExecutionID == "" || started.Workflow.CurrentGate != "intake" || started.Workflow.Revision != 1 {
		t.Fatalf("started workflow = %+v", started.Workflow)
	}
	exactWorkItem := "github:owner/repo#7"
	runCanonical(0, "success", "Work Item link loaded", "work-item", "show", "--project", preview.Setup.ProjectID, "--repository", "main", "--work-item", exactWorkItem)
	exactStatus := runCanonical(0, "success", "Execution workflow operation completed", "workflow", "status", "--project", "configured", "--repository", "main", "--work-item", exactWorkItem, "--execution", started.Workflow.ExecutionID)
	if exactStatus.Workflow == nil || exactStatus.Workflow.ExecutionID != started.Workflow.ExecutionID || exactStatus.Workflow.WorkItem.Resource != "owner/repo" || exactStatus.Workflow.RepositoryKey != "main" {
		t.Fatalf("exact selector result = %+v", exactStatus.Workflow)
	}
	fullySpecified := exec.Command(binary, "--json", "workflow", "status", "--project", preview.Setup.ProjectID, "--repository", "main", "--work-item", exactWorkItem, "--execution", started.Workflow.ExecutionID)
	fullySpecified.Env = environment
	fullySpecified.Dir = unrelatedCWD
	var fullOutput, fullPrompts bytes.Buffer
	fullySpecified.Stdout = &fullOutput
	fullySpecified.Stderr = &fullPrompts
	if err := fullySpecified.Run(); err != nil || fullPrompts.Len() != 0 {
		t.Fatalf("full selector err=%v prompts=%q output=%s", err, fullPrompts.String(), fullOutput.String())
	}
	partial := exec.Command(binary, "--json", "workflow", "status", "--project", "configured", "--repository", "main", "--work-item", exactWorkItem)
	partial.Env = environment
	partial.Dir = unrelatedCWD
	partial.Stdin = strings.NewReader(started.Workflow.ExecutionID + "\n")
	var partialOutput, partialPrompts bytes.Buffer
	partial.Stdout = &partialOutput
	partial.Stderr = &partialPrompts
	if err := partial.Run(); err != nil || partialPrompts.String() != "Execution ID: " {
		t.Fatalf("partial err=%v prompts=%q output=%s", err, partialPrompts.String(), partialOutput.String())
	}
	executionRecords, err := filepath.Glob(filepath.Join(state, "executions", "v1", preview.Setup.ProjectID, "*.json"))
	if err != nil || len(executionRecords) != 1 {
		t.Fatalf("execution records=%v err=%v", executionRecords, err)
	}
	beforeInvalid, err := os.ReadFile(executionRecords[0])
	if err != nil {
		t.Fatal(err)
	}
	runCanonical(1, "validation_failure", "Execution workflow operation did not complete", "workflow", "status", "--project", "configured", "--repository", "main", "--work-item", exactWorkItem, "--execution", "018f4a44-7c31-7dd4-9d00-222222222222")
	runCanonical(1, "validation_failure", "Execution workflow operation did not complete", "workflow", "status", "--project", "configured", "--repository", "missing", "--work-item", exactWorkItem, "--execution", started.Workflow.ExecutionID)
	runCanonical(1, "validation_failure", "Execution workflow operation did not complete", "workflow", "status", "--project", "configured", "--repository", "main", "--work-item", "github:owner/repo#999", "--execution", started.Workflow.ExecutionID)
	runCanonical(1, "validation_failure", "Explicit selector input is invalid", "workflow", "status", "--project", "configured", "--project", preview.Setup.ProjectID, "--repository", "main", "--work-item", exactWorkItem, "--execution", started.Workflow.ExecutionID)
	afterInvalid, err := os.ReadFile(executionRecords[0])
	if err != nil || !bytes.Equal(beforeInvalid, afterInvalid) {
		t.Fatalf("invalid selector changed execution: %v", err)
	}
	revision := uint64(1)
	for _, gate := range []string{"intake", "specification", "clarification"} {
		advanced := runCanonical(0, "success", "Execution workflow operation completed", "workflow", "advance", "--project", "configured", "--repository", "main", "--number", "7", "--expected-revision", strconv.FormatUint(revision, 10), "--gate", gate, "--outcome", "pass")
		revision++
		if advanced.Workflow == nil || advanced.Workflow.Revision != revision {
			t.Fatalf("advanced workflow = %+v", advanced.Workflow)
		}
	}
	runCanonical(0, "success", "Execution workflow operation completed", factArgs(revision, "planning-authority", "specification")...)
	revision++
	for _, gate := range []string{"plan", "tasks"} {
		advanced := runCanonical(0, "success", "Execution workflow operation completed", "workflow", "advance", "--project", "configured", "--repository", "main", "--number", "7", "--expected-revision", strconv.FormatUint(revision, 10), "--gate", gate, "--outcome", "pass")
		revision++
		if advanced.Workflow == nil || advanced.Workflow.Revision != revision {
			t.Fatalf("advanced workflow = %+v", advanced.Workflow)
		}
	}
	runCanonical(0, "success", "Execution workflow operation completed", factArgs(revision, "implementation-authority", "plan")...)
	revision++
	interrupted := runCanonical(2, "interrupted", "Execution remains at the current workflow stage", "workflow", "advance", "--project", "configured", "--repository", "main", "--number", "7", "--expected-revision", strconv.FormatUint(revision, 10), "--gate", "implementation", "--outcome", "fail")
	revision++
	if interrupted.Workflow == nil || interrupted.Workflow.Status != "interrupted" || interrupted.Workflow.CurrentGate != "implementation" || interrupted.Workflow.Revision != revision {
		t.Fatalf("workflow payload = %+v", interrupted.Workflow)
	}
	resumed := runCanonical(0, "success", "Execution workflow operation completed", "workflow", "resume", "--project", "configured", "--repository", "main", "--number", "7", "--expected-revision", strconv.FormatUint(revision, 10))
	revision++
	if resumed.Workflow == nil || resumed.Workflow.Status != "active" || resumed.Workflow.Revision != revision {
		t.Fatalf("resumed = %+v", resumed.Workflow)
	}
	advancedToReview := runCanonical(0, "success", "Execution workflow operation completed", "workflow", "advance", "--project", "configured", "--repository", "main", "--number", "7", "--expected-revision", strconv.FormatUint(revision, 10), "--gate", "implementation", "--outcome", "pass")
	revision++
	if advancedToReview.Workflow == nil || advancedToReview.Workflow.Revision != revision {
		t.Fatalf("advanced workflow = %+v", advancedToReview.Workflow)
	}
	runCanonical(0, "success", "Execution workflow operation completed", factArgs(revision, "review-started", "evidence")...)
	revision++
	for _, gate := range []string{"review", "evidence", "reconciliation", "completion"} {
		advanced := runCanonical(0, "success", "Execution workflow operation completed", "workflow", "advance", "--project", "configured", "--repository", "main", "--number", "7", "--expected-revision", strconv.FormatUint(revision, 10), "--gate", gate, "--outcome", "pass")
		revision++
		if advanced.Workflow == nil || advanced.Workflow.Revision != revision {
			t.Fatalf("advanced workflow = %+v", advanced.Workflow)
		}
	}
	runCanonical(0, "success", "Execution workflow operation completed", factArgs(revision, "human-acceptance", "evidence")...)
	revision++
	runCanonical(0, "success", "Execution workflow operation completed", "workflow", "evidence", "--project", "configured", "--repository", "main", "--number", "7")
	run(1, "error", "missing_required_input", "project", "init", "--slug", "sample")
	run(0, "success", "applied", "project", "init", "--slug", "sample", "--name", "Sample")
	run(0, "success", "already_initialized", "project", "init", "--slug", "sample", "--name", "Sample")
	run(1, "error", "explicit_update_required", "project", "init", "--slug", "sample", "--name", "Other")
	runCanonical(0, "success", "Project is valid", "project", "validate", "--slug", "sample")
	run(0, "success", "reopened_without_local_state", "project", "reopen", "--slug", "sample")
	source := filepath.Join(portable, "sample")
	before, err := os.ReadFile(filepath.Join(source, "axiom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	run(0, "success", "installed", "project", "install", "--source", source)
	run(0, "success", "already_installed", "project", "install", "--source", source)
	run(0, "success", "reopened_with_local_state", "project", "reopen", "--slug", "sample")
	after, err := os.ReadFile(filepath.Join(source, "axiom.yaml"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("install changed portable bytes: %v", err)
	}
	records, err := filepath.Glob(filepath.Join(state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 3 {
		t.Fatalf("local records = %v, %v", records, err)
	}
	run(0, "success", "applied", "project", "update", "--slug", "sample", "--name", "Changed")
	run(1, "error", "local_state_revalidation_required", "project", "reopen", "--slug", "sample")
	runCanonical(1, "validation_failure", "Project state is invalid", "project", "validate", "--slug", "missing")
}
