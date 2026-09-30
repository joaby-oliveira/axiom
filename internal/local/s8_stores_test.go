package local

import (
	"context"
	"errors"
	"fmt"
	"github.com/rgomids/axiom/internal/testfs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/coordination"
	"github.com/rgomids/axiom/internal/executiongraph"
	"github.com/rgomids/axiom/internal/runtimeprofile"
)

func TestRuntimeProfileStorePreservesRevisionAndCompletePriorGeneration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := NewRuntimeProfileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	first := runtimeConfiguration(1)
	if err := store.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := runtimeConfiguration(2)
	store.hooks.fault = func(stage FaultStage) error {
		if stage == FaultF2 {
			return ErrSimulatedInterruption
		}
		return nil
	}
	if err := store.Save(context.Background(), second); err == nil {
		t.Fatal("faulted update succeeded")
	}
	store.hooks = publicationHooks{}
	if _, err := store.Load(context.Background()); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("interrupted reader err=%v", err)
	}
	wire, err := os.ReadFile(filepath.Join(root, "runtime-profiles", "v1", "configuration.json"))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := runtimeprofile.Decode(wire)
	if err != nil || loaded.Revision != 1 {
		t.Fatalf("canonical=%+v err=%v", loaded, err)
	}
	root = filepath.Join(t.TempDir(), "state")
	store, err = NewRuntimeProfileStore(root)
	if err != nil {
		t.Fatalf("fresh store err=%v", err)
	}
	if err = store.Create(context.Background(), first); err != nil {
		t.Fatalf("fresh create err=%v", err)
	}
	second.Revision = 3
	if err := store.Save(context.Background(), second); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision err=%v", err)
	}
}

func TestGraphAndCoordinationStoresPublishAndReadSeparatelyFromSequentialExecution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	graphStore, err := NewGraphStore(root)
	if err != nil {
		t.Fatal(err)
	}
	graph := localGraphFixture(t)
	if err := graphStore.Create(context.Background(), graph); err != nil {
		t.Fatal(err)
	}
	loaded, err := graphStore.Load(context.Background(), "project-1", graph.Parent.ExecutionID)
	if err != nil || loaded.Parent.ExecutionID != graph.Parent.ExecutionID || loaded.StorageRevision == ([32]byte{}) {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	if _, err := os.Stat(filepath.Join(root, "executions")); !os.IsNotExist(err) {
		t.Fatalf("graph publication synthesized ADR-0008 state: %v", err)
	}
	coordinationStore, err := NewCoordinationStore(root, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	record := coordinationRecord(graph)
	if err := coordinationStore.Publish(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	latest, exists, err := coordinationStore.Latest(context.Background(), graph.Parent.ExecutionID, graph.Children[0].ExecutionID)
	if err != nil || !exists || latest.Digest != record.Digest {
		t.Fatalf("latest=%+v exists=%t err=%v", latest, exists, err)
	}
}

func TestGraphStorePublicationFaultMatrixNeverExposesMixedGraph(t *testing.T) {
	stages := []FaultStage{FaultF0, FaultF1, FaultF2, FaultF3, FaultF4, FaultF5, FaultF6, FaultF7, FaultF8}
	for _, stage := range stages {
		t.Run(string(stage), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "state")
			store, err := NewGraphStore(root)
			if err != nil {
				t.Fatal(err)
			}
			graph := localGraphFixture(t)
			store.hooks.fault = func(observed FaultStage) error {
				if observed == stage {
					return ErrSimulatedInterruption
				}
				return nil
			}
			if err := store.Create(context.Background(), graph); err == nil {
				t.Fatalf("stage %s did not interrupt", stage)
			}
			store.hooks = publicationHooks{}
			loaded, err := store.Load(context.Background(), "project-1", graph.Parent.ExecutionID)
			if err == nil {
				if loaded.Parent.ExecutionID != graph.Parent.ExecutionID || !executiongraph.ValidGraph(loaded) {
					t.Fatalf("stage %s exposed invalid graph: %+v", stage, loaded)
				}
				return
			}
			if !errors.Is(err, ErrRecoveryRequired) && !errors.Is(err, ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stage %s returned unclassified error: %v", stage, err)
			}
		})
	}
}

func runtimeConfiguration(revision uint64) runtimeprofile.Configuration {
	return runtimeprofile.Configuration{FormatVersion: 1, Revision: revision,
		Runtimes:      []runtimeprofile.Runtime{{ID: "codex", Adapter: "codex", Enabled: true, AllowlistedProfileIDs: []string{"codex-default"}}, {ID: "claude", Adapter: "claude", Enabled: true, AllowlistedProfileIDs: []string{"claude-default"}}},
		ModelProfiles: []runtimeprofile.ModelProfile{{ID: "codex-default", RuntimeID: "codex", Model: "local-codex", Capabilities: []string{"go"}}, {ID: "claude-default", RuntimeID: "claude", Model: "local-claude", Capabilities: []string{"go"}}},
	}
}

func localGraphFixture(t *testing.T) executiongraph.Graph {
	t.Helper()
	plan := executiongraph.ApprovedPlan{Approved: true, PlanRevision: "plan-1", PlanDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", MaximumNodes: 2, Work: []executiongraph.WorkUnit{
		{Key: "implementation", Capability: executiongraph.CapabilityRequest{Role: "implementation", Complexity: "low", Capabilities: []string{"go"}}, Inputs: []string{"plan"}, Outputs: []string{"result"}, Scope: executiongraph.Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{"internal/api"}}, Effects: []executiongraph.Effect{{Kind: "repository-write", Target: "internal/api"}}, Controls: executiongraph.ExecutionControls{Timeout: time.Minute, MaximumAttempts: 2}},
		{Key: "integration", Capability: executiongraph.CapabilityRequest{Role: "integration", Complexity: "high", Capabilities: []string{"validation"}}, Dependencies: []string{"implementation"}, Inputs: []string{"result"}, Outputs: []string{"delivery"}, Scope: executiongraph.Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{"internal/api"}}, Effects: []executiongraph.Effect{{Kind: "integration", Target: "internal/api"}}, ValidationOwner: true, IntegrationOwner: true, Controls: executiongraph.ExecutionControls{Timeout: time.Minute, MaximumAttempts: 2}},
	}}
	planner := executiongraph.NewPlanner(allCapabilities{})
	proposal, err := planner.Propose(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	request := executiongraph.PublicationRequest{Proposal: proposal, ExpectedDigest: proposal.Digest, GraphRevision: 1, ChildAuthorities: map[string][]executiongraph.Effect{}, AuthorityReferences: map[string]string{}, Workspaces: map[string]string{}, Resolutions: map[string]executiongraph.Resolution{}}
	for _, node := range proposal.Nodes {
		request.ParentAuthority = append(request.ParentAuthority, node.Effects...)
		request.ChildAuthorities[node.Key] = node.Effects
		request.AuthorityReferences[node.Key] = "authority:" + node.Key
		request.Workspaces[node.Key] = filepath.Join(t.TempDir(), node.Key)
		request.Resolutions[node.Key] = executiongraph.Resolution{RuntimeID: "codex", ModelProfileID: "profile-1", ConfigurationRevision: 1, ObservationRevision: 1}
	}
	next := 0
	memory := &noopGraphStore{}
	graph, err := executiongraph.NewGraphService(memory, func() (string, error) { next++; return opaqueID(next), nil }, func() time.Time { return time.Unix(1, 0).UTC() }).Publish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

type allCapabilities struct{}

func (allCapabilities) ValidateCapability(context.Context, executiongraph.CapabilityRequest) error {
	return nil
}

type noopGraphStore struct{ graph executiongraph.Graph }

func (s *noopGraphStore) Create(_ context.Context, graph executiongraph.Graph) error {
	s.graph = graph
	return nil
}
func (s *noopGraphStore) Load(context.Context, string, string) (executiongraph.Graph, error) {
	return s.graph, nil
}

func opaqueID(value int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", value) }

func coordinationRecord(graph executiongraph.Graph) coordination.Record {
	record := coordination.Record{FormatVersion: 1, RecordID: "00000000-0000-4000-8000-000000000010", Kind: coordination.Progress, ParentID: graph.Parent.ExecutionID, ChildID: graph.Children[0].ExecutionID, GraphRevision: graph.Parent.GraphRevision, Revision: 1, Provenance: coordination.Provenance{Product: "Axiom", Version: "dev", Revision: "abc", SourceState: "clean"}, Fields: []coordination.Field{{Name: "phase", Value: "testing"}}, CreatedAt: time.Unix(2, 0).UTC()}
	// Service is responsible for the digest; use it against a memory-like store.
	service := coordination.New(&recordCapture{}, func() (string, error) { return record.RecordID, nil }, func() time.Time { return record.CreatedAt })
	published, _ := service.Publish(context.Background(), graph, coordination.Input{Kind: record.Kind, ParentID: record.ParentID, ChildID: record.ChildID, GraphRevision: record.GraphRevision, Provenance: record.Provenance, Fields: record.Fields})
	return published
}

type recordCapture struct{}

func (*recordCapture) Latest(context.Context, string, string) (coordination.Record, bool, error) {
	return coordination.Record{}, false, nil
}
func (*recordCapture) Publish(context.Context, coordination.Record) error { return nil }

func TestCoordinationServiceBootstrapsMissingStoreHierarchy(t *testing.T) {
	for _, level := range []string{"absent", "root", "coordination", "version", "project"} {
		t.Run(level, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "state")
			paths := []string{root, filepath.Join(root, "coordination"), filepath.Join(root, "coordination", "v1"), filepath.Join(root, "coordination", "v1", "project-1")}
			for index, name := range []string{"root", "coordination", "version", "project"} {
				if level == "absent" {
					break
				}
				if err := os.Mkdir(paths[index], 0o700); err != nil {
					t.Fatal(err)
				}
				if level == name {
					break
				}
			}
			store, err := NewCoordinationStore(root, "project-1")
			if err != nil {
				t.Fatal(err)
			}
			graph := localGraphFixture(t)
			if _, exists, err := store.Latest(context.Background(), graph.Parent.ExecutionID, graph.Children[0].ExecutionID); err != nil || exists {
				t.Fatalf("fresh latest exists=%t err=%v", exists, err)
			}
			if level == "absent" {
				if _, err := os.Stat(root); !os.IsNotExist(err) {
					t.Fatalf("read created root: %v", err)
				}
			}
			record, err := coordination.New(store, nil, nil).Publish(context.Background(), graph, coordination.Input{
				Kind: coordination.QuestionRequest, ParentID: graph.Parent.ExecutionID, ChildID: graph.Children[0].ExecutionID, GraphRevision: graph.Parent.GraphRevision,
				Provenance: coordination.Provenance{Product: "lingo", Version: "development", Revision: "test", SourceState: "clean"}, Fields: []coordination.Field{{Name: "question", Value: "Which contract?"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			latest, exists, err := store.Latest(context.Background(), graph.Parent.ExecutionID, graph.Children[0].ExecutionID)
			if err != nil || !exists || latest.Digest != record.Digest {
				t.Fatalf("round trip exists=%t err=%v", exists, err)
			}
		})
	}
}

func TestCoordinationLatestRejectsUnsafeHierarchy(t *testing.T) {
	testfs.POSIXModes(t)
	root := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewCoordinationStore(root, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	graph := localGraphFixture(t)
	if _, exists, err := store.Latest(context.Background(), graph.Parent.ExecutionID, graph.Children[0].ExecutionID); !errors.Is(err, ErrUnsafe) || exists {
		t.Fatalf("unsafe latest exists=%t err=%v", exists, err)
	}
}
