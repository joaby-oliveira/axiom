package graphapplication_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/executiongraph"
	"github.com/rgomids/axiom/internal/gitworkspace"
	"github.com/rgomids/axiom/internal/graphapplication"
)

func TestLocalServiceRunsConcurrentChildrenAndConcreteIntegration(t *testing.T) {
	root := canonicalTempDir(t)
	repository, revision := initRepository(t, filepath.Join(root, "repository"))
	workspaceRoot := filepath.Join(root, "workspaces")
	graph := buildGraph(t, workspaceRoot)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	service, err := graphapplication.NewLocalService(context.Background(), graphapplication.LocalConfiguration{
		Repository: repository, WorkspaceRoot: workspaceRoot, BaseRevision: revision,
		Graph: graph, Invocations: helperInvocations{executable: executable},
		Validators:        []gitworkspace.ValidationCommand{{Reference: "git-diff-check", Argv: []string{gitPath, "diff", "--check"}, Env: []string{"LC_ALL=C"}, OutputMax: 4096}},
		AllocateAttemptID: sequenceAllocator(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareWorkspaces(context.Background()); err != nil {
		t.Fatal(err)
	}
	sharedBefore := sharedState(t, repository)
	dispatch, err := service.DispatchReady(context.Background(), nil, false)
	if err != nil || len(dispatch.Records) != 2 {
		t.Fatalf("dispatch=%+v err=%v", dispatch, err)
	}
	if !overlap(dispatch.Records[0], dispatch.Records[1]) {
		t.Fatalf("children did not overlap: %+v", dispatch.Records)
	}
	for _, record := range dispatch.Records {
		if record.Status != executiongraph.AttemptSucceeded {
			t.Fatalf("record=%+v", record)
		}
	}
	preview, observations, err := service.PreviewIntegration(context.Background(), nil)
	if err != nil || len(observations) != 3 || preview.Digest == "" {
		t.Fatalf("preview=%+v observations=%+v err=%v", preview, observations, err)
	}
	rollup, err := service.ExecuteIntegration(context.Background(), preview, executiongraph.IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: preview.TargetRevision, Effects: preview.Effects, Reference: "authority:integration"})
	if err != nil || rollup.Status != "success" || len(rollup.ValidationResults) != 1 {
		t.Fatalf("rollup=%+v err=%v", rollup, err)
	}
	if after := sharedState(t, repository); after != sharedBefore {
		t.Fatalf("shared checkout changed\nbefore=%s\nafter=%s", sharedBefore, after)
	}
	t.Logf("base=%s target_tree=%s child_trees=%s,%s preview=%s authority=%s result_tree=%s validation=%s exit=%d overlap=true shared_unchanged=true", revision, preview.TargetTree, preview.Sources[0].ResultTree, preview.Sources[1].ResultTree, preview.Digest, preview.Digest, rollup.IntegrationResultTree, rollup.ValidationResults[0].OutputDigest, rollup.ValidationResults[0].ExitCode)
	integration := childByKey(service.Graph(), "integrate")
	if readFile(t, filepath.Join(integration.Envelope.Workspace, "a.txt")) != "a\n" || readFile(t, filepath.Join(integration.Envelope.Workspace, "b.txt")) != "b\n" {
		t.Fatal("integrated content mismatch")
	}
}

func TestGraphApplicationHelperProcess(t *testing.T) {
	if os.Getenv("AXIOM_GRAPH_HELPER") != "1" {
		return
	}
	delay, err := strconv.Atoi(os.Getenv("AXIOM_GRAPH_DELAY_MS"))
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("AXIOM_GRAPH_TARGET"), []byte(os.Getenv("AXIOM_GRAPH_CONTENT")+"\n"), 0o600); err != nil {
		os.Exit(3)
	}
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

type helperInvocations struct{ executable string }

func (h helperInvocations) ResolveInvocation(_ context.Context, child executiongraph.ChildExecution) (executiongraph.Invocation, error) {
	target := child.NodeKey + ".txt"
	return executiongraph.Invocation{
		RuntimeID: child.Envelope.Resolution.RuntimeID,
		Argv:      []string{h.executable, "-test.run=TestGraphApplicationHelperProcess"},
		CWD:       child.Envelope.Workspace,
		Env:       []string{"AXIOM_GRAPH_HELPER=1", "AXIOM_GRAPH_DELAY_MS=200", "AXIOM_GRAPH_TARGET=" + target, "AXIOM_GRAPH_CONTENT=" + child.NodeKey},
		OutputMax: 4096,
	}, nil
}

func overlap(left, right executiongraph.DispatchRecord) bool {
	return left.StartedAt.Before(right.EndedAt) && right.StartedAt.Before(left.EndedAt)
}

func buildGraph(t *testing.T, workspaceRoot string) executiongraph.Graph {
	t.Helper()
	controls := executiongraph.ExecutionControls{Timeout: 5 * time.Second, MaximumAttempts: 2}
	plan := executiongraph.ApprovedPlan{Approved: true, PlanRevision: "plan-1", PlanDigest: digest("plan"), MaximumNodes: 3, Work: []executiongraph.WorkUnit{
		{Key: "a", Capability: capability("implementation"), Inputs: []string{"plan"}, Outputs: []string{"a-result"}, Scope: scope("a.txt"), Effects: []executiongraph.Effect{{Kind: "repository-write", Target: "a.txt"}}, Controls: controls},
		{Key: "b", Capability: capability("documentation"), Inputs: []string{"plan"}, Outputs: []string{"b-result"}, Scope: scope("b.txt"), Effects: []executiongraph.Effect{{Kind: "repository-write", Target: "b.txt"}}, Controls: controls},
		{Key: "integrate", Capability: capability("integration"), Dependencies: []string{"a", "b"}, Inputs: []string{"a-result", "b-result"}, Outputs: []string{"delivery"}, Scope: executiongraph.Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{"a.txt", "b.txt"}}, Effects: []executiongraph.Effect{{Kind: "integration", Target: "a.txt"}, {Kind: "integration", Target: "b.txt"}}, ValidationOwner: true, IntegrationOwner: true, Controls: controls},
	}}
	proposal, err := executiongraph.NewPlanner(capabilityOK{}).Propose(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	workspaces := map[string]string{"a": filepath.Join(workspaceRoot, "a"), "b": filepath.Join(workspaceRoot, "b"), "integrate": filepath.Join(workspaceRoot, "integrate")}
	authorities := map[string][]executiongraph.Effect{}
	references := map[string]string{}
	resolutions := map[string]executiongraph.Resolution{}
	parentEffects := []executiongraph.Effect{}
	for _, node := range proposal.Nodes {
		authorities[node.Key] = append([]executiongraph.Effect(nil), node.Effects...)
		parentEffects = append(parentEffects, node.Effects...)
		references[node.Key] = "authority:" + node.Key
		resolutions[node.Key] = executiongraph.Resolution{RuntimeID: "codex", ModelProfileID: "profile-" + node.Key, ConfigurationRevision: 1, ObservationRevision: 1}
	}
	store := &memoryGraphStore{}
	graph, err := executiongraph.NewGraphService(store, sequenceAllocator(0), func() time.Time { return time.Unix(10, 0).UTC() }).Publish(context.Background(), executiongraph.PublicationRequest{Proposal: proposal, ExpectedDigest: proposal.Digest, GraphRevision: 1, ParentAuthority: parentEffects, ChildAuthorities: authorities, AuthorityReferences: references, Workspaces: workspaces, Resolutions: resolutions})
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

type capabilityOK struct{}

func (capabilityOK) ValidateCapability(context.Context, executiongraph.CapabilityRequest) error {
	return nil
}

type memoryGraphStore struct{ wire []byte }

func (s *memoryGraphStore) Create(_ context.Context, graph executiongraph.Graph) error {
	wire, err := executiongraph.EncodeGraph(graph)
	if err == nil {
		s.wire = wire
	}
	return err
}

func (s *memoryGraphStore) Load(context.Context, string, string) (executiongraph.Graph, error) {
	return executiongraph.DecodeGraph(s.wire)
}

func sequenceAllocator(start uint64) func() (string, error) {
	next := start
	return func() (string, error) {
		next++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", next), nil
	}
}

func initRepository(t *testing.T, repository string) (string, string) {
	t.Helper()
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, repository, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(repository, "README.md"), "base\n")
	git(t, repository, "add", "README.md")
	git(t, repository, "-c", "user.name=Axiom Test", "-c", "user.email=axiom@example.invalid", "commit", "-m", "base")
	return repository, git(t, repository, "rev-parse", "HEAD")
}

func sharedState(t *testing.T, repository string) string {
	t.Helper()
	return git(t, repository, "rev-parse", "HEAD") + git(t, repository, "rev-parse", "HEAD^{tree}") + git(t, repository, "status", "--porcelain=v1", "--untracked-files=all")
}

func childByKey(graph executiongraph.Graph, key string) executiongraph.ChildExecution {
	for _, child := range graph.Children {
		if child.NodeKey == key {
			return child
		}
	}
	panic("child not found")
}

func capability(role string) executiongraph.CapabilityRequest {
	return executiongraph.CapabilityRequest{Role: role, Complexity: "low", Capabilities: []string{"go"}}
}

func scope(target string) executiongraph.Scope {
	return executiongraph.Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{target}}
}

func digest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func git(t *testing.T, directory string, args ...string) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(gitPath, args...)
	command.Dir = directory
	command.Env = []string{"LC_ALL=C", "LANG=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z"}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	value, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	value, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return value
}
