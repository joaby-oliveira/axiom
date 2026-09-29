package local

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/provenance"
	"github.com/rgomids/axiom/internal/workflow"
)

func TestExecutionStoreRoundTripsClosedPrivateState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := NewWorkflowStore(root)
	if err != nil {
		t.Fatal(err)
	}
	state := validExecution()
	if err := store.Create(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem)
	if err != nil || loaded.ExecutionID != state.ExecutionID || loaded.Stage != workflow.Intake || loaded.StorageRevision == ([32]byte{}) {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}
	record := filepath.Join(root, "executions", "v1", state.ProjectID, executionName(state.RepositoryKey, state.WorkItem))
	info, err := os.Stat(record)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode = %v, %v", info, err)
	}
}

func TestExecutionStoreRejectsUnknownNewerAndMalformedState(t *testing.T) {
	for _, mutate := range []func([]byte) []byte{
		func(wire []byte) []byte {
			return bytes.Replace(wire, []byte(`"status":`), []byte(`"unknown":true,"status":`), 1)
		},
		func(wire []byte) []byte {
			return bytes.Replace(wire, []byte(`"formatVersion":1`), []byte(`"formatVersion":2`), 1)
		},
		func([]byte) []byte { return []byte("{not-json}\n") },
	} {
		root := filepath.Join(t.TempDir(), "state")
		store, _ := NewWorkflowStore(root)
		state := validExecution()
		if err := store.Create(context.Background(), state); err != nil {
			t.Fatal(err)
		}
		record := filepath.Join(root, "executions", "v1", state.ProjectID, executionName(state.RepositoryKey, state.WorkItem))
		wire, _ := os.ReadFile(record)
		if err := os.WriteFile(record, mutate(wire), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem); err == nil {
			t.Fatal("unsafe state accepted")
		}
	}
}

func TestExecutionStoreRequiresStorageRevisionAndRejectsStaleWriter(t *testing.T) {
	store, _ := NewWorkflowStore(filepath.Join(t.TempDir(), "state"))
	state := validExecution()
	if err := store.Create(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	first, _ := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem)
	stale := first
	first = advancedExecution(first)
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	stale = interruptedExecution(stale)
	if err := store.Save(context.Background(), stale); !errors.Is(err, workflow.ErrConflict) {
		t.Fatalf("stale save = %v", err)
	}
	loaded, _ := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem)
	if loaded.Stage != workflow.Specification || loaded.Revision != 2 {
		t.Fatalf("winning state = %#v", loaded)
	}
}

func TestExecutionStoreFailsClosedAcrossF0F8(t *testing.T) {
	for index, stage := range []FaultStage{FaultF0, FaultF1, FaultF2, FaultF3, FaultF4, FaultF5, FaultF6, FaultF7, FaultF8} {
		t.Run(string(stage), func(t *testing.T) {
			store, _ := NewWorkflowStore(filepath.Join(t.TempDir(), "state"))
			state := validExecution()
			if err := store.Create(context.Background(), state); err != nil {
				t.Fatal(err)
			}
			loaded, _ := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem)
			loaded = advancedExecution(loaded)
			store.hooks.fault = func(current FaultStage) error {
				if current == stage {
					return ErrSimulatedInterruption
				}
				return nil
			}
			err := store.Save(context.Background(), loaded)
			var publication *PublicationError
			if !errors.As(err, &publication) || publication.Committed != (index >= 6) {
				t.Fatalf("fault outcome = %v", err)
			}
			_, readErr := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem)
			if stage == FaultF0 || stage == FaultF1 {
				if readErr != nil {
					t.Fatalf("prior authority lost: %v", readErr)
				}
				return
			}
			if !errors.Is(readErr, workflow.ErrRecoveryRequired) {
				t.Fatalf("reader = %v", readErr)
			}
		})
	}
}

func validExecution() workflow.State {
	source, _ := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "abc123", SourceState: provenance.Clean}, nil)
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	return workflow.State{ExecutionID: "018f4a44-7c31-7dd4-9d00-111111111111", FormatVersion: workflow.FormatVersion, WorkflowVersion: workflow.WorkflowVersion, ProjectID: "123e4567-e89b-42d3-a456-426614174000", RepositoryKey: "main", WorkItem: workflow.WorkItem{Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}, RuntimeID: "codex", Stage: workflow.Intake, Revision: 1, Status: workflow.ExecutionActive, CreatedAt: now, UpdatedAt: now, Provenance: workflow.Identity{Product: source.Product(), Version: source.Version(), Revision: source.Revision(), SourceState: string(source.SourceState())}}
}

func advancedExecution(state workflow.State) workflow.State {
	now := state.UpdatedAt.Add(time.Second)
	state.Revision, state.Stage, state.UpdatedAt = 2, workflow.Specification, now
	state.Transitions = []workflow.Transition{{Revision: 2, From: workflow.Intake, To: workflow.Specification, Outcome: workflow.OutcomePassed, RequestDigest: string(bytes.Repeat([]byte{'a'}, 64)), CommittedAt: now, Provenance: state.Provenance}}
	return state
}

func interruptedExecution(state workflow.State) workflow.State {
	now := state.UpdatedAt.Add(time.Second)
	state.Revision, state.Status, state.UpdatedAt = 2, workflow.ExecutionInterrupted, now
	state.Transitions = []workflow.Transition{{Revision: 2, From: workflow.Intake, To: workflow.Intake, Outcome: workflow.OutcomeFailed, RequestDigest: string(bytes.Repeat([]byte{'b'}, 64)), CommittedAt: now, Provenance: state.Provenance}}
	return state
}
