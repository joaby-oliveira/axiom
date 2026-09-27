package gitworkspace_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/executiongraph"
	"github.com/rgomids/axiom/internal/gitworkspace"
)

func TestManagerCreatesAndValidatesConfinedRealWorktrees(t *testing.T) {
	fixture := newGitFixture(t, graphOptions{})
	sharedBefore := fixture.sharedState(t)
	observations, err := fixture.manager.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 3 || observations[0].Workspace == observations[1].Workspace {
		t.Fatalf("observations=%+v", observations)
	}
	t.Logf("repository=temp-local-git base=%s tree=%s workspaces=<temp>/workspaces/{a,b,integrate} child_ids=%s,%s,%s", fixture.revision, fixture.tree, observations[0].ChildID, observations[1].ChildID, observations[2].ChildID)
	for _, child := range fixture.graph.Children {
		if err := fixture.manager.ValidateWorkspace(context.Background(), child); err != nil {
			t.Fatalf("validate %s: %v", child.NodeKey, err)
		}
	}
	if after := fixture.sharedState(t); after != sharedBefore {
		t.Fatalf("shared checkout changed\nbefore=%s\nafter=%s", sharedBefore, after)
	}
}

func TestManagerRejectsDirtyWrongHeadWrongRepositoryAndCollision(t *testing.T) {
	t.Run("foreign mutation", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		child := fixture.child("a")
		writeFile(t, filepath.Join(child.Envelope.Workspace, "foreign.txt"), "foreign\n")
		if err := fixture.manager.ValidateWorkspace(context.Background(), child); !errors.Is(err, gitworkspace.ErrForeignMutation) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("wrong head", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		child := fixture.child("a")
		writeFile(t, filepath.Join(child.Envelope.Workspace, "a.txt"), "changed\n")
		git(t, child.Envelope.Workspace, "add", "a.txt")
		git(t, child.Envelope.Workspace, "-c", "user.name=Axiom Test", "-c", "user.email=axiom@example.invalid", "commit", "-m", "foreign")
		if err := fixture.manager.ValidateWorkspace(context.Background(), child); !errors.Is(err, gitworkspace.ErrWorkspaceInvalid) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("wrong repository", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		other := initRepository(t, filepath.Join(t.TempDir(), "other"))
		otherWorktree := filepath.Join(t.TempDir(), "other-worktree")
		git(t, other.repository, "worktree", "add", "--detach", otherWorktree, other.revision)
		foreignGitFile, err := os.ReadFile(filepath.Join(otherWorktree, ".git"))
		if err != nil {
			t.Fatal(err)
		}
		child := fixture.child("a")
		writeFile(t, filepath.Join(child.Envelope.Workspace, ".git"), string(foreignGitFile))
		if err := fixture.manager.ValidateWorkspace(context.Background(), child); !errors.Is(err, gitworkspace.ErrWorkspaceInvalid) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("wrong ownership", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		child := fixture.child("a")
		owner := filepath.Join(fixture.root, ".axiom-workspace-owners", child.ExecutionID+".json")
		wire, err := os.ReadFile(owner)
		if err != nil {
			t.Fatal(err)
		}
		tampered := strings.Replace(string(wire), child.ParentID, opaqueID(999), 1)
		writeFile(t, owner, tampered)
		if err := fixture.manager.ValidateWorkspace(context.Background(), child); !errors.Is(err, gitworkspace.ErrWorkspaceInvalid) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("collision", func(t *testing.T) {
		root := canonicalTempDir(t)
		repository := initRepository(t, filepath.Join(root, "repository"))
		workspaceRoot := filepath.Join(root, "workspaces")
		graph := buildGraph(t, workspaceRoot, graphOptions{})
		writeFile(t, graph.Children[0].Envelope.Workspace, "occupied\n")
		manager, err := gitworkspace.NewManager(context.Background(), repository.repository, workspaceRoot, repository.revision, graph)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Prepare(context.Background()); !errors.Is(err, gitworkspace.ErrWorkspaceCollision) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestManagerRejectsOutsideRootAndSymlinkEscape(t *testing.T) {
	t.Run("outside root", func(t *testing.T) {
		root := canonicalTempDir(t)
		repository := initRepository(t, filepath.Join(root, "repository"))
		workspaceRoot := filepath.Join(root, "workspaces")
		outside := filepath.Join(root, "outside")
		graph := buildGraph(t, workspaceRoot, graphOptions{workspaceOverride: map[string]string{"a": outside}})
		manager, err := gitworkspace.NewManager(context.Background(), repository.repository, workspaceRoot, repository.revision, graph)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Prepare(context.Background()); !errors.Is(err, gitworkspace.ErrWorkspaceInvalid) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("symlink escape", func(t *testing.T) {
		root := canonicalTempDir(t)
		repository := initRepository(t, filepath.Join(root, "repository"))
		workspaceRoot := filepath.Join(root, "workspaces")
		if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(root, "outside")
		if err := os.MkdirAll(outside, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(workspaceRoot, "escape")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		graph := buildGraph(t, workspaceRoot, graphOptions{workspaceOverride: map[string]string{"a": filepath.Join(link, "a")}})
		manager, err := gitworkspace.NewManager(context.Background(), repository.repository, workspaceRoot, repository.revision, graph)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Prepare(context.Background()); !errors.Is(err, gitworkspace.ErrWorkspaceInvalid) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestConcreteIntegrationAppliesTwoResultsValidatesAndLeavesSharedCheckoutUntouched(t *testing.T) {
	fixture := newGitFixture(t, graphOptions{})
	fixture.prepare(t)
	sharedBefore := fixture.sharedState(t)
	graph := fixture.succeed(t, "a", "b")
	writeFile(t, filepath.Join(fixture.childFrom(graph, "a").Envelope.Workspace, "a.txt"), "child-a\n")
	writeFile(t, filepath.Join(fixture.childFrom(graph, "b").Envelope.Workspace, "b.txt"), "child-b\n")

	observation, results, _, err := fixture.manager.ObserveIntegration(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	validator := fixture.validator(t, "/usr/bin/git", "diff", "--check")
	service := executiongraph.NewIntegrationService(fixture.manager, validator)
	preview, err := service.Preview(graph, observation, results)
	if err != nil {
		t.Fatal(err)
	}
	rollup, err := service.Execute(context.Background(), graph, preview, executiongraph.IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: preview.TargetRevision, Effects: preview.Effects, Reference: "authority:integration"})
	if err != nil || rollup.Status != "success" || len(rollup.ValidationResults) != 1 || rollup.ValidationResults[0].ExitCode != 0 {
		t.Fatalf("rollup=%+v err=%v", rollup, err)
	}
	integration := fixture.childFrom(graph, "integrate")
	if got := readFile(t, filepath.Join(integration.Envelope.Workspace, "a.txt")); got != "child-a\n" {
		t.Fatalf("a.txt=%q", got)
	}
	if got := readFile(t, filepath.Join(integration.Envelope.Workspace, "b.txt")); got != "child-b\n" {
		t.Fatalf("b.txt=%q", got)
	}
	if after := fixture.sharedState(t); after != sharedBefore {
		t.Fatalf("shared checkout changed\nbefore=%s\nafter=%s", sharedBefore, after)
	}
}

func TestConcreteIntegrationBlocksMissingStaleForeignConflictAndEffectMismatch(t *testing.T) {
	t.Run("missing child", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		graph := fixture.succeed(t, "a")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "a").Envelope.Workspace, "a.txt"), "a\n")
		observation, results, _, err := fixture.manager.ObserveIntegration(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := executiongraph.NewIntegrationService(fixture.manager, fixture.validator(t, "/usr/bin/git", "diff", "--check")).Preview(graph, observation, results); !errors.Is(err, executiongraph.ErrIntegrationBlocked) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("foreign target", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		writeFile(t, filepath.Join(fixture.child("integrate").Envelope.Workspace, "foreign.txt"), "foreign\n")
		if _, _, _, err := fixture.manager.ObserveIntegration(context.Background(), nil); !errors.Is(err, gitworkspace.ErrForeignMutation) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("stale base", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		integration := fixture.child("integrate")
		git(t, integration.Envelope.Workspace, "-c", "user.name=Axiom Test", "-c", "user.email=axiom@example.invalid", "commit", "--allow-empty", "-m", "stale")
		if _, _, _, err := fixture.manager.ObserveIntegration(context.Background(), nil); !errors.Is(err, gitworkspace.ErrWorkspaceInvalid) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("real conflict", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{conflict: true})
		fixture.prepare(t)
		graph := fixture.succeed(t, "a", "b")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "a").Envelope.Workspace, "shared.txt"), "from-a\n")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "b").Envelope.Workspace, "shared.txt"), "from-b\n")
		observation, _, _, err := fixture.manager.ObserveIntegration(context.Background(), nil)
		if err != nil || len(observation.Conflicts) != 1 {
			t.Fatalf("observation=%+v err=%v", observation, err)
		}
	})

	t.Run("effect outside preview", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		graph := fixture.succeed(t, "a", "b")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "a").Envelope.Workspace, "outside.txt"), "outside\n")
		if _, _, _, err := fixture.manager.ObserveIntegration(context.Background(), nil); !errors.Is(err, gitworkspace.ErrForeignMutation) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestConcreteIntegrationWaiverAuthorityTamperAndValidationFailure(t *testing.T) {
	t.Run("optional waiver digest bound", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{optionalB: true})
		fixture.prepare(t)
		graph := fixture.succeed(t, "a")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "a").Envelope.Workspace, "a.txt"), "a\n")
		service := executiongraph.NewIntegrationService(fixture.manager, fixture.validator(t, "/usr/bin/git", "diff", "--check"))
		observation, results, _, err := fixture.manager.ObserveIntegration(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Preview(graph, observation, results); !errors.Is(err, executiongraph.ErrIntegrationBlocked) {
			t.Fatalf("missing waiver err=%v", err)
		}
		observation, results, _, err = fixture.manager.ObserveIntegration(context.Background(), map[string]string{fixture.childFrom(graph, "b").ExecutionID: "waiver:reviewed"})
		if err != nil {
			t.Fatal(err)
		}
		preview, err := service.Preview(graph, observation, results)
		if err != nil || len(preview.OptionalWaivers) != 1 {
			t.Fatalf("preview=%+v err=%v", preview, err)
		}
		tampered := preview
		tampered.OptionalWaivers[0].Reference = "waiver:tampered"
		rollup, err := service.Execute(context.Background(), graph, tampered, executiongraph.IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: preview.TargetRevision, Effects: preview.Effects, Reference: "authority:integration"})
		if !errors.Is(err, executiongraph.ErrIntegrationStale) || rollup.Status != "recovery_required" {
			t.Fatalf("rollup=%+v err=%v", rollup, err)
		}
	})

	t.Run("validator failure never succeeds", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		graph := fixture.succeed(t, "a", "b")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "a").Envelope.Workspace, "a.txt"), "a\n")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "b").Envelope.Workspace, "b.txt"), "b\n")
		observation, results, _, err := fixture.manager.ObserveIntegration(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		falsePath, err := exec.LookPath("false")
		if err != nil {
			t.Fatal(err)
		}
		service := executiongraph.NewIntegrationService(fixture.manager, fixture.validator(t, falsePath))
		preview, err := service.Preview(graph, observation, results)
		if err != nil {
			t.Fatal(err)
		}
		rollup, err := service.Execute(context.Background(), graph, preview, executiongraph.IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: preview.TargetRevision, Effects: preview.Effects, Reference: "authority:integration"})
		if !errors.Is(err, executiongraph.ErrIntegrationFailed) || rollup.Status == "success" || len(rollup.ValidationResults) != 1 || rollup.ValidationResults[0].ExitCode == 0 {
			t.Fatalf("rollup=%+v err=%v", rollup, err)
		}
	})
}

type graphOptions struct {
	conflict          bool
	optionalB         bool
	workspaceOverride map[string]string
}

type gitFixture struct {
	repository, revision, tree, root string
	graph                            executiongraph.Graph
	manager                          *gitworkspace.Manager
}

func newGitFixture(t *testing.T, options graphOptions) *gitFixture {
	t.Helper()
	root := canonicalTempDir(t)
	repository := initRepository(t, filepath.Join(root, "repository"))
	workspaceRoot := filepath.Join(root, "workspaces")
	graph := buildGraph(t, workspaceRoot, options)
	manager, err := gitworkspace.NewManager(context.Background(), repository.repository, workspaceRoot, repository.revision, graph)
	if err != nil {
		t.Fatal(err)
	}
	return &gitFixture{repository: repository.repository, revision: repository.revision, tree: repository.tree, root: workspaceRoot, graph: graph, manager: manager}
}

func (f *gitFixture) prepare(t *testing.T) {
	t.Helper()
	if _, err := f.manager.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (f *gitFixture) child(key string) executiongraph.ChildExecution {
	return f.childFrom(f.graph, key)
}

func (f *gitFixture) childFrom(graph executiongraph.Graph, key string) executiongraph.ChildExecution {
	for _, child := range graph.Children {
		if child.NodeKey == key {
			return child
		}
	}
	panic("child not found: " + key)
}

func (f *gitFixture) succeed(t *testing.T, keys ...string) executiongraph.Graph {
	t.Helper()
	graph := f.graph
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[key] = true
	}
	now := time.Unix(20, 0).UTC()
	for index := range graph.Children {
		if wanted[graph.Children[index].NodeKey] {
			digest := sha256.Sum256([]byte(graph.Children[index].NodeKey))
			graph.Children[index].Attempts = []executiongraph.Attempt{{AttemptID: opaqueID(uint64(100 + index)), Number: 1, Status: executiongraph.AttemptSucceeded, StartedAt: &now, FinishedAt: &now, ResultReference: "result:" + graph.Children[index].NodeKey, OutputDigest: hex.EncodeToString(digest[:])}}
		}
	}
	if err := f.manager.BindGraph(graph); err != nil {
		t.Fatal(err)
	}
	f.graph = graph
	return graph
}

func (f *gitFixture) validator(t *testing.T, argv ...string) *gitworkspace.CombinedValidator {
	t.Helper()
	validator, err := gitworkspace.NewCombinedValidator(f.manager, []gitworkspace.ValidationCommand{{Reference: "validator:one", Argv: argv, Env: []string{"LC_ALL=C"}, OutputMax: 4096}})
	if err != nil {
		t.Fatal(err)
	}
	return validator
}

func (f *gitFixture) sharedState(t *testing.T) string {
	t.Helper()
	return git(t, f.repository, "rev-parse", "HEAD") + git(t, f.repository, "rev-parse", "HEAD^{tree}") + git(t, f.repository, "status", "--porcelain=v1", "--untracked-files=all")
}

type repositoryFixture struct{ repository, revision, tree string }

func initRepository(t *testing.T, repository string) repositoryFixture {
	t.Helper()
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, repository, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(repository, "README.md"), "base\n")
	writeFile(t, filepath.Join(repository, "shared.txt"), "base\n")
	git(t, repository, "add", "README.md", "shared.txt")
	git(t, repository, "-c", "user.name=Axiom Test", "-c", "user.email=axiom@example.invalid", "commit", "-m", "base")
	return repositoryFixture{repository: repository, revision: git(t, repository, "rev-parse", "HEAD"), tree: git(t, repository, "rev-parse", "HEAD^{tree}")}
}

func buildGraph(t *testing.T, workspaceRoot string, options graphOptions) executiongraph.Graph {
	t.Helper()
	aTarget, bTarget := "a.txt", "b.txt"
	bDependencies := []string{}
	if options.conflict {
		aTarget, bTarget = "shared.txt", "shared.txt"
		bDependencies = []string{"a"}
	}
	controls := executiongraph.ExecutionControls{Timeout: time.Minute, MaximumAttempts: 2}
	integrationPaths := []string{aTarget, bTarget}
	integrationEffects := []executiongraph.Effect{{Kind: "integration", Target: aTarget}, {Kind: "integration", Target: bTarget}}
	if aTarget == bTarget {
		integrationPaths = []string{aTarget}
		integrationEffects = []executiongraph.Effect{{Kind: "integration", Target: aTarget}}
	}
	plan := executiongraph.ApprovedPlan{Approved: true, PlanRevision: "plan-1", PlanDigest: digest("plan"), MaximumNodes: 3, Work: []executiongraph.WorkUnit{
		{Key: "a", Capability: capability("implementation"), Inputs: []string{"plan"}, Outputs: []string{"a-result"}, Scope: scope(aTarget), Effects: []executiongraph.Effect{{Kind: "repository-write", Target: aTarget}}, Controls: controls},
		{Key: "b", Capability: capability("documentation"), Dependencies: bDependencies, Inputs: []string{"plan"}, Outputs: []string{"b-result"}, Scope: scope(bTarget), Effects: []executiongraph.Effect{{Kind: "repository-write", Target: bTarget}}, Optional: options.optionalB, Controls: controls},
		{Key: "integrate", Capability: capability("integration"), Dependencies: []string{"a", "b"}, Inputs: []string{"a-result", "b-result"}, Outputs: []string{"delivery"}, Scope: executiongraph.Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: integrationPaths}, Effects: integrationEffects, ValidationOwner: true, IntegrationOwner: true, Controls: controls},
	}}
	planner := executiongraph.NewPlanner(capabilityOK{})
	proposal, err := planner.Propose(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	workspaces := map[string]string{"a": filepath.Join(workspaceRoot, "a"), "b": filepath.Join(workspaceRoot, "b"), "integrate": filepath.Join(workspaceRoot, "integrate")}
	for key, value := range options.workspaceOverride {
		workspaces[key] = value
	}
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
	next := uint64(0)
	graph, err := executiongraph.NewGraphService(store, func() (string, error) { next++; return opaqueID(next), nil }, func() time.Time { return time.Unix(10, 0).UTC() }).Publish(context.Background(), executiongraph.PublicationRequest{Proposal: proposal, ExpectedDigest: proposal.Digest, GraphRevision: 1, ParentAuthority: parentEffects, ChildAuthorities: authorities, AuthorityReferences: references, Workspaces: workspaces, Resolutions: resolutions})
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

func capability(role string) executiongraph.CapabilityRequest {
	return executiongraph.CapabilityRequest{Role: role, Complexity: "low", Capabilities: []string{"go"}}
}

func scope(target string) executiongraph.Scope {
	return executiongraph.Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{target}}
}

func opaqueID(value uint64) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", value) }

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
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
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

func TestFixtureGraphStable(t *testing.T) {
	fixture := newGitFixture(t, graphOptions{})
	if !executiongraph.ValidGraph(fixture.graph) || reflect.DeepEqual(fixture.child("a"), fixture.child("b")) {
		t.Fatal("invalid fixture")
	}
}
