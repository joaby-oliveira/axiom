package graphapplication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	"github.com/rgomids/axiom/internal/runtimeadapter"
)

func TestNewLocalServiceWiresConcreteRuntimeAdapter(t *testing.T) {
	root := canonicalTempDir(t)
	repository, revision := initRepository(t, filepath.Join(root, "repository"))
	graph := buildGraph(t, filepath.Join(root, "workspaces"))
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	profiles := make([]runtimeadapter.CommandProfile, 0, len(graph.Children))
	for _, child := range graph.Children {
		profiles = append(profiles, runtimeadapter.CommandProfile{RuntimeID: "codex", ModelProfileID: child.Envelope.Resolution.ModelProfileID, Executable: filepath.Join(root, "codex"), Model: "local-test-profile", OutputMax: 4096})
	}
	store := &memoryGraphStore{wire: mustEncodeGraph(t, graph)}
	service, err := NewLocalService(context.Background(), LocalConfiguration{
		Repository: repository, WorkspaceRoot: filepath.Join(root, "workspaces"), BaseRevision: revision,
		Graph: graph, GraphStore: store, RuntimeProfiles: profiles,
		Validators: []gitworkspace.ValidationCommand{{Reference: "git-diff-check", Argv: []string{gitPath, "diff", "--check"}, Env: []string{"LC_ALL=C"}, OutputMax: 4096}},
	})
	if err != nil || service == nil {
		t.Fatalf("service=%v err=%v", service, err)
	}
}

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
	store := &memoryGraphStore{wire: mustEncodeGraph(t, graph)}
	service, err := newLocalService(context.Background(), LocalConfiguration{
		Repository: repository, WorkspaceRoot: workspaceRoot, BaseRevision: revision,
		Graph:             graph,
		GraphStore:        store,
		Validators:        []gitworkspace.ValidationCommand{{Reference: "git-diff-check", Argv: []string{gitPath, "diff", "--check"}, Env: []string{"LC_ALL=C"}, OutputMax: 4096}},
		AllocateAttemptID: sequenceAllocator(100),
	}, helperInvocations{executable: executable})
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
	persisted, err := store.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, graph := range []executiongraph.Graph{service.Graph(), persisted} {
		attempts := childByKey(graph, "integrate").Attempts
		if len(attempts) != 1 || attempts[0].Status != executiongraph.AttemptSucceeded || attempts[0].ResultReference != executiongraph.IntegrationResultReference(rollup.IntegrationResultTree) {
			t.Fatalf("integration attempts=%+v", attempts)
		}
	}
	evidence, err := service.BuildEvidence(executiongraph.Evidence{FormatVersion: 1, BaseRevision: revision, ConfigurationDigest: digest("config"), ParentID: persisted.Parent.ExecutionID, GraphRevision: persisted.Parent.GraphRevision, Dispatch: dispatch.Records, Rollup: rollup, ValidationReferences: []string{"git-diff-check"}, Limitations: []string{"real Codex plus Claude run not executed"}})
	if err != nil || evidence.Status != "deterministic_preparation_only" {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
}

func TestLocalServiceIntegrationPersistenceFailureAfterEffectsStaysRecoveryRequired(t *testing.T) {
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
	// Two dispatch saves plus the running integration save succeed; the
	// terminal integration save fails after confirmed apply and validation.
	store := &failingGraphStore{memoryGraphStore: memoryGraphStore{wire: mustEncodeGraph(t, graph)}, failOn: 4}
	service, err := newLocalService(context.Background(), LocalConfiguration{
		Repository: repository, WorkspaceRoot: workspaceRoot, BaseRevision: revision,
		Graph: graph, GraphStore: store,
		Validators:        []gitworkspace.ValidationCommand{{Reference: "git-diff-check", Argv: []string{gitPath, "diff", "--check"}, Env: []string{"LC_ALL=C"}, OutputMax: 4096}},
		AllocateAttemptID: sequenceAllocator(100),
	}, helperInvocations{executable: executable})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareWorkspaces(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DispatchReady(context.Background(), nil, false); err != nil {
		t.Fatal(err)
	}
	preview, _, err := service.PreviewIntegration(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	authority := executiongraph.IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: preview.TargetRevision, Effects: preview.Effects, Reference: "authority:integration"}
	rollup, err := service.ExecuteIntegration(context.Background(), preview, authority)
	if !errors.Is(err, executiongraph.ErrIntegrationRecoveryRequired) || rollup.Status != "recovery_required" {
		t.Fatalf("rollup=%+v err=%v", rollup, err)
	}
	integration := childByKey(service.Graph(), "integrate")
	if len(integration.Attempts) != 1 || integration.Attempts[0].Status != executiongraph.AttemptRunning {
		t.Fatalf("integration attempts=%+v", integration.Attempts)
	}
	if readFile(t, filepath.Join(integration.Envelope.Workspace, "a.txt")) != "a\n" {
		t.Fatal("confirmed integration effect missing")
	}
	persisted, err := store.Load(context.Background(), "", "")
	if err != nil || childByKey(persisted, "integrate").Attempts[0].Status != executiongraph.AttemptRunning {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
	if _, err := service.BuildEvidence(executiongraph.Evidence{FormatVersion: 1, BaseRevision: revision, ConfigurationDigest: digest("config"), ParentID: persisted.Parent.ExecutionID, GraphRevision: persisted.Parent.GraphRevision, Rollup: rollup, ValidationReferences: []string{"git-diff-check"}, Limitations: []string{"integration attempt persistence failed"}}); err != nil {
		t.Fatalf("truthful recovery roll-up rejected: %v", err)
	}
	fabricated := rollup
	fabricated.Status = "success"
	fabricated.ChildOutcomes = append([]executiongraph.ChildOutcome(nil), rollup.ChildOutcomes...)
	for index := range fabricated.ChildOutcomes {
		if fabricated.ChildOutcomes[index].ChildID == fabricated.IntegrationChildID {
			fabricated.ChildOutcomes[index].Status = executiongraph.AttemptSucceeded
		}
	}
	if _, err := service.BuildEvidence(executiongraph.Evidence{FormatVersion: 1, BaseRevision: revision, ConfigurationDigest: digest("config"), ParentID: persisted.Parent.ExecutionID, GraphRevision: persisted.Parent.GraphRevision, Rollup: fabricated, ValidationReferences: []string{"git-diff-check"}, Limitations: []string{"integration attempt persistence failed"}}); err == nil {
		t.Fatal("fabricated success accepted over running integration attempt")
	}
	if _, err := service.ExecuteIntegration(context.Background(), preview, authority); !errors.Is(err, executiongraph.ErrIntegrationStale) {
		t.Fatalf("replay err=%v", err)
	}
}

type failingGraphStore struct {
	memoryGraphStore
	failOn, calls int
}

func (s *failingGraphStore) Save(ctx context.Context, graph executiongraph.Graph) (executiongraph.Graph, error) {
	s.calls++
	if s.calls == s.failOn {
		return executiongraph.Graph{}, errors.New("store unavailable")
	}
	return s.memoryGraphStore.Save(ctx, graph)
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
	controls := executiongraph.ExecutionControls{Timeout: 30 * time.Second, MaximumAttempts: 2}
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

func (s *memoryGraphStore) Save(_ context.Context, graph executiongraph.Graph) (executiongraph.Graph, error) {
	wire, err := executiongraph.EncodeGraph(graph)
	if err != nil {
		return executiongraph.Graph{}, err
	}
	s.wire = wire
	return executiongraph.DecodeGraph(wire)
}

func mustEncodeGraph(t *testing.T, graph executiongraph.Graph) []byte {
	t.Helper()
	wire, err := executiongraph.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	return wire
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
