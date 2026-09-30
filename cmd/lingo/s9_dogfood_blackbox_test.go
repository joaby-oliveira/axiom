package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitHubStateVariable switches the test binary into a stateful fake `gh`
// so the executable exercises the real GitHub adapter without a network.
const fakeGitHubStateVariable = "AXIOM_TEST_FAKE_GH_STATE"

func TestMain(m *testing.M) {
	if path := os.Getenv(fakeGitHubStateVariable); path != "" {
		os.Exit(runFakeGitHub(path, os.Args[1:], os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

type fakeGitHub struct {
	RepositoryLabels []string `json:"repositoryLabels"`
	IssueLabels      []string `json:"issueLabels"`
	Comments         []string `json:"comments"`
	IssueCreated     bool     `json:"issueCreated"`
	Mutations        []string `json:"mutations"`
}

func runFakeGitHub(path string, args []string, stdin io.Reader, stdout io.Writer) int {
	wire, err := os.ReadFile(path)
	if err != nil {
		return 1
	}
	var state fakeGitHub
	if err := json.Unmarshal(wire, &state); err != nil {
		return 1
	}
	method, endpoint := "GET", ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "api", "--include":
		case "--method":
			index++
			method = args[index]
		case "--input", "-f":
			index++
		default:
			if endpoint == "" {
				endpoint = args[index]
			}
		}
	}
	body, _ := io.ReadAll(stdin)
	const issue = "repos/owner/repo/issues/7"
	names := func(values []string) []map[string]string {
		result := make([]map[string]string, 0, len(values))
		for _, value := range values {
			result = append(result, map[string]string{"name": value})
		}
		return result
	}
	var response any
	switch {
	case method == "GET" && endpoint == "search/issues":
		response = map[string]any{"total_count": 0, "items": []any{}}
	case method == "POST" && endpoint == "repos/owner/repo/issues":
		state.IssueCreated = true
		state.Mutations = append(state.Mutations, "create_issue")
		response = map[string]any{"number": 7, "html_url": "https://github.com/owner/repo/issues/7", "state": "open"}
	case !state.IssueCreated:
		return 1
	case method == "GET" && endpoint == "repos/owner/repo/labels?per_page=100":
		response = names(state.RepositoryLabels)
	case method == "POST" && endpoint == "repos/owner/repo/labels":
		var label struct{ Name string }
		if json.Unmarshal(body, &label) != nil || label.Name == "" {
			return 1
		}
		state.RepositoryLabels = append(state.RepositoryLabels, label.Name)
		state.Mutations = append(state.Mutations, "create_label:"+label.Name)
		response = map[string]string{"name": label.Name}
	case method == "GET" && endpoint == issue:
		response = map[string]any{"number": 7, "html_url": "https://github.com/owner/repo/issues/7", "state": "open", "labels": names(state.IssueLabels)}
	case method == "POST" && endpoint == issue+"/labels":
		var request struct{ Labels []string }
		if json.Unmarshal(body, &request) != nil || len(request.Labels) != 1 {
			return 1
		}
		state.IssueLabels = append(state.IssueLabels, request.Labels[0])
		state.Mutations = append(state.Mutations, "add_label:"+request.Labels[0])
		response = names(state.IssueLabels)
	case method == "DELETE" && strings.HasPrefix(endpoint, issue+"/labels/"):
		name := strings.TrimPrefix(endpoint, issue+"/labels/")
		kept := []string{}
		for _, label := range state.IssueLabels {
			if label != name {
				kept = append(kept, label)
			}
		}
		if len(kept) == len(state.IssueLabels) {
			return 1
		}
		state.IssueLabels = kept
		state.Mutations = append(state.Mutations, "remove_label:"+name)
		response = names(state.IssueLabels)
	case method == "GET" && endpoint == issue+"/comments?per_page=100":
		comments := make([]map[string]string, 0, len(state.Comments))
		for _, comment := range state.Comments {
			comments = append(comments, map[string]string{"body": comment})
		}
		response = comments
	case method == "POST" && endpoint == issue+"/comments":
		var comment struct{ Body string }
		if json.Unmarshal(body, &comment) != nil || comment.Body == "" {
			return 1
		}
		state.Comments = append(state.Comments, comment.Body)
		state.Mutations = append(state.Mutations, "comment")
		response = map[string]int{"id": len(state.Comments)}
	default:
		return 1
	}
	updated, err := json.Marshal(state)
	if err != nil || os.WriteFile(path, updated, 0o600) != nil {
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(response); err != nil {
		return 1
	}
	return 0
}

// TestExecutableClaudeRuntimeAndFirstProjectionOfCreatedWorkItem reproduces
// both Major findings of the S9 dogfood against v0.1.2-rc.1: an Execution
// conducted by Claude was persisted as codex, and `work-item create` →
// `workflow start` → first transition → `workflow reconcile` required
// recovery because the new Issue carried no axiom:stage:* marker.
func TestExecutableClaudeRuntimeAndFirstProjectionOfCreatedWorkItem(t *testing.T) {
	binary := filepath.Join(t.TempDir(), testExecutableName("axiom"))
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build executable: %v: %s", err, output)
	}
	fakeGH, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "github.json")
	writeFake := func(state fakeGitHub) {
		t.Helper()
		wire, err := json.Marshal(state)
		if err != nil || os.WriteFile(statePath, wire, 0o600) != nil {
			t.Fatalf("write fake GitHub state: %v", err)
		}
	}
	readFake := func() fakeGitHub {
		t.Helper()
		wire, err := os.ReadFile(statePath)
		var state fakeGitHub
		if err != nil || json.Unmarshal(wire, &state) != nil {
			t.Fatalf("read fake GitHub state: %v", err)
		}
		return state
	}
	writeFake(fakeGitHub{RepositoryLabels: []string{"bug"}})

	portable := filepath.Join(t.TempDir(), "portable")
	state := filepath.Join(t.TempDir(), "state")
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	environment := append(os.Environ(), "HOME="+t.TempDir(), "PATH="+t.TempDir(), "CLAUDE_CONFIG_DIR=", "LINGO_PROJECTS_ROOT="+portable, "LINGO_STATE_ROOT="+state, "AXIOM_CODEX_SKILLS_ROOT="+filepath.Join(t.TempDir(), "skills"), "AXIOM_GH_BIN="+fakeGH, fakeGitHubStateVariable+"="+statePath)
	type event struct {
		canonicalEvent
		Workflow *struct {
			ExecutionID, CurrentGate, RuntimeID string
			Revision                            uint64
		} `json:"workflow"`
	}
	run := func(wantCode int, wantStatus string, args ...string) event {
		t.Helper()
		command := exec.Command(binary, append([]string{"--json"}, args...)...)
		command.Env = environment
		command.Dir = t.TempDir()
		output, err := command.Output()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		var decoded event
		if err := json.Unmarshal(bytes.TrimSpace(output), &decoded); err != nil {
			t.Fatalf("%v: invalid JSON %q: %v", args, output, err)
		}
		if code != wantCode || decoded.Status != wantStatus {
			t.Fatalf("%v: code=%d output=%s", args, code, output)
		}
		return decoded
	}

	setup := run(0, "success", "project", "configure", "--slug", "configured", "--name", "Configured", "--repository", "main="+repository, "--work-item-provider", "github")
	run(0, "success", "project", "configure", "--project-id", setup.Setup.ProjectID, "--slug", "configured", "--name", "Configured", "--repository", "main="+repository, "--work-item-provider", "github", "--preview-digest", setup.Setup.Digest, "--authorize-local")
	draftArgs := []string{"work-item", "create", "--project", "configured", "--repository", "main", "--provider-repository", "owner/repo", "--intent", "Dogfood regression", "--desired-outcome", "First projection converges", "--context", "S9 dogfood", "--scope", "Bounded change", "--constraints", "Fail closed on drift", "--non-goals", "No release", "--acceptance", "Tests pass"}
	draft := run(0, "success", draftArgs...)
	created := run(0, "success", append(draftArgs, "--preview-digest", draft.Draft.Digest, "--authorize-external")...)
	if created.WorkItem == nil || created.WorkItem.ExternalID != "7" {
		t.Fatalf("created Work Item = %+v", created.WorkItem)
	}
	// A human adds a foreign label after creation; still no axiom:stage:*.
	github := readFake()
	github.IssueLabels = []string{"triage"}
	writeFake(github)

	selector := []string{"--project", "configured", "--repository", "main", "--work-item", "github:owner/repo#7"}
	executions := filepath.Join(state, "executions", "v1", setup.Setup.ProjectID, "*.json")
	for _, invalid := range [][]string{{"--runtime", "gemini"}, {"--runtime", ""}, {"--runtime", "Claude"}, {"--runtime", "claude", "--runtime", "codex"}} {
		run(1, "validation_failure", append(append([]string{"workflow", "start"}, selector...), invalid...)...)
	}
	if records, _ := filepath.Glob(executions); len(records) != 0 {
		t.Fatalf("invalid Runtime selector created Executions: %v", records)
	}

	started := run(0, "success", append(append([]string{"workflow", "start"}, selector...), "--runtime", "claude")...)
	if started.Workflow == nil || started.Workflow.RuntimeID != "claude" || started.Workflow.Revision != 1 {
		t.Fatalf("started = %+v", started.Workflow)
	}
	execution := append(append([]string{}, selector...), "--execution", started.Workflow.ExecutionID)
	records, err := filepath.Glob(executions)
	if err != nil || len(records) != 1 {
		t.Fatalf("Execution records = %v, %v", records, err)
	}
	persisted, err := os.ReadFile(records[0])
	if err != nil || !strings.Contains(string(persisted), `"runtimeId":"claude"`) {
		t.Fatalf("persisted Execution does not carry Claude: %s %v", persisted, err)
	}
	// The Runtime of an existing Execution cannot be switched or re-selected.
	run(1, "validation_failure", append(append([]string{"workflow", "start"}, selector...), "--runtime", "codex")...)
	run(1, "validation_failure", append([]string{"workflow", "start"}, selector...)...)
	run(1, "validation_failure", append(append([]string{"workflow", "status"}, execution...), "--runtime", "codex")...)
	if unchanged, _ := os.ReadFile(records[0]); !bytes.Equal(unchanged, persisted) {
		t.Fatal("Runtime conflict changed the persisted Execution")
	}
	if status := run(0, "success", append([]string{"workflow", "status"}, execution...)...); status.Workflow.RuntimeID != "claude" {
		t.Fatalf("status Runtime = %q", status.Workflow.RuntimeID)
	}

	advanced := run(0, "success", append(append([]string{"workflow", "advance"}, execution...), "--expected-revision", "1", "--gate", "intake", "--outcome", "pass")...)
	if advanced.Workflow.RuntimeID != "claude" || advanced.Workflow.Revision != 2 {
		t.Fatalf("advanced = %+v", advanced.Workflow)
	}
	reconcile := append(append([]string{"workflow", "reconcile"}, execution...), "--expected-revision", "2")
	preview := run(0, "success", reconcile...)
	if preview.Projection == nil || preview.Workflow.RuntimeID != "claude" {
		t.Fatalf("preview = %+v", preview)
	}
	var kinds []string
	for _, effect := range preview.Projection.Effects {
		kinds = append(kinds, effect.Kind+":"+strings.SplitN(effect.Value, "\n", 2)[0])
	}
	want := []string{"create_stage_label:axiom:stage:specifying", "add_stage_label:axiom:stage:specifying", "post_transition_comment:<!-- axiom:workflow-projection:" + preview.Projection.ProjectionKey + " -->"}
	if strings.Join(kinds, "|") != strings.Join(want, "|") {
		t.Fatalf("preview effects = %v want %v", kinds, want)
	}
	if mutations := readFake().Mutations; len(mutations) != 1 {
		t.Fatalf("preview mutated GitHub: %v", mutations)
	}

	authorized := append(append([]string{}, reconcile...), "--preview-digest", preview.Projection.Digest, "--authorize-external")
	applied := run(0, "success", authorized...)
	if applied.Workflow.RuntimeID != "claude" {
		t.Fatalf("applied = %+v", applied.Workflow)
	}
	github = readFake()
	if strings.Join(github.IssueLabels, ",") != "triage,axiom:stage:specifying" || len(github.Comments) != 1 || strings.Join(github.Mutations, ",") != "create_issue,create_label:axiom:stage:specifying,add_label:axiom:stage:specifying,comment" {
		t.Fatalf("GitHub after projection = %+v", github)
	}

	run(0, "success", authorized...)
	if replay := run(0, "success", reconcile...); replay.Projection == nil || len(replay.Projection.Effects) != 0 {
		t.Fatalf("replay preview = %+v", replay.Projection)
	}
	if after := readFake(); len(after.Mutations) != len(github.Mutations) || len(after.Comments) != 1 {
		t.Fatalf("replay duplicated effects: %+v", after)
	}

	// Losing the established marker later is drift, never a new bootstrap.
	github.IssueLabels = []string{"triage"}
	writeFake(github)
	run(1, "failure", reconcile...)
	if after := readFake(); len(after.Mutations) != len(github.Mutations) {
		t.Fatalf("drift produced effects: %+v", after)
	}
}
