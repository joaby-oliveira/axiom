package executiongraph

import (
	"fmt"
	"testing"
	"time"
)

func TestPrepareRunEnvelopeBindsExactGraphProfilesCommandsAndCleanup(t *testing.T) {
	graph := mustGraph(t)
	envelope := RunEnvelope{FormatVersion: 1, Activity: "Add runtime profile validation command and documentation", BaseRevision: "8a9ca19fd260bc19f5bb288b449e8dc6df618194", ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ConfigurationRef: "runtime-profile-config:t36", ConfigurationDigest: digestOf("config"), Validators: []string{"go-test", "repository-validator"}, ExpectedEvidence: []string{"concurrency-trace", "integration-result", "coordination-records"}, CleanupDisposition: "preserve_pending_human_review"}
	for _, child := range graph.Children {
		envelope.Children = append(envelope.Children, ChildRunPlan{ChildID: child.ExecutionID, RuntimeID: child.Envelope.Resolution.RuntimeID, ModelProfileID: child.Envelope.Resolution.ModelProfileID, Repository: "/Users/example/axiom", Workspace: child.Envelope.Workspace, Scope: child.Envelope.Scope, Dependencies: child.Envelope.Dependencies, Effects: child.Envelope.AllowedEffects, Argv: []string{"/opt/bin/codex", "exec", "--model", "local-profile"}, Timeout: child.Envelope.Controls.Timeout.String(), MaximumAttempts: child.Envelope.Controls.MaximumAttempts})
	}
	prepared, err := PrepareRunEnvelope(graph, envelope)
	if err != nil || prepared.Digest == "" {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	envelope.Children[0].Effects = append(envelope.Children[0].Effects, Effect{Kind: "repository-write", Target: "outside"})
	if _, err := PrepareRunEnvelope(graph, envelope); err == nil {
		t.Fatal("expanded effects accepted")
	}
}

func TestBuildEvidenceCannotClaimRealRunFromPreparation(t *testing.T) {
	graph := mustGraph(t)
	rollup := rollupGraph(graph)
	evidence, err := BuildEvidence(graph, Evidence{FormatVersion: 1, Status: "real_run_recorded", BaseRevision: "8a9ca19fd260bc19f5bb288b449e8dc6df618194", ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, CoordinationDigests: []string{digestOf("coordination")}, Rollup: rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"real Codex plus Claude run not executed"}})
	if err != nil || evidence.Status != "deterministic_preparation_only" || evidence.Digest == "" {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
}

func TestBuildEvidenceDerivesRealRuntimeStatusFromCompleteJourney(t *testing.T) {
	graph, evidence := completeRuntimeEvidenceFixture(t)
	tests := []struct {
		name   string
		mutate func(*Evidence)
	}{
		{"only Codex", func(evidence *Evidence) { evidence.RuntimeExecutions = evidence.RuntimeExecutions[:1] }},
		{"only Claude", func(evidence *Evidence) { evidence.RuntimeExecutions = evidence.RuntimeExecutions[1:] }},
		{"incomplete rollup", func(evidence *Evidence) {
			evidence.Rollup.Status = "partial"
			evidence.Rollup.IntegrationResultTree = ""
			for index := range evidence.Rollup.ChildOutcomes {
				if evidence.Rollup.ChildOutcomes[index].ChildID == evidence.Rollup.IntegrationChildID {
					evidence.Rollup.ChildOutcomes[index].Status = AttemptUnknown
				}
			}
		}},
		{"missing combined validation", func(evidence *Evidence) { evidence.Rollup.ValidationResults = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := evidence
			candidate.RuntimeExecutions = append([]RuntimeExecutionEvidence(nil), evidence.RuntimeExecutions...)
			candidate.Rollup.ChildOutcomes = append([]ChildOutcome(nil), evidence.Rollup.ChildOutcomes...)
			candidate.Rollup.ValidationResults = append([]ValidationResult(nil), evidence.Rollup.ValidationResults...)
			test.mutate(&candidate)
			built, err := BuildEvidence(graph, candidate)
			if err != nil || built.Status != "deterministic_preparation_only" {
				t.Fatalf("built=%+v err=%v", built, err)
			}
		})
	}
	built, err := BuildEvidence(graph, evidence)
	if err != nil || built.Status != "real_run_recorded" || built.Digest == "" {
		t.Fatalf("built=%+v err=%v", built, err)
	}
}

func TestBuildEvidenceRejectsForeignRuntimeProfileOrAttempt(t *testing.T) {
	graph, evidence := completeRuntimeEvidenceFixture(t)
	tests := []struct {
		name   string
		mutate func(*RuntimeExecutionEvidence)
	}{
		{"runtime", func(record *RuntimeExecutionEvidence) { record.RuntimeID = "claude" }},
		{"profile", func(record *RuntimeExecutionEvidence) { record.ModelProfileID = "foreign-profile" }},
		{"attempt", func(record *RuntimeExecutionEvidence) { record.AttemptID = "00000000-0000-4000-8000-999999999999" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := evidence
			candidate.RuntimeExecutions = append([]RuntimeExecutionEvidence(nil), evidence.RuntimeExecutions...)
			test.mutate(&candidate.RuntimeExecutions[0])
			if _, err := BuildEvidence(graph, candidate); err == nil {
				t.Fatal("foreign runtime evidence accepted")
			}
		})
	}
}

func completeRuntimeEvidenceFixture(t *testing.T) (Graph, Evidence) {
	t.Helper()
	graph := mustGraph(t)
	now := time.Unix(30, 0).UTC()
	dispatch := []DispatchRecord{}
	runtimeExecutions := []RuntimeExecutionEvidence{}
	runtimeIndex := 0
	for index := range graph.Children {
		child := &graph.Children[index]
		if child.Envelope.IntegrationOwner {
			continue
		}
		runtimeID := "codex"
		if runtimeIndex == 1 {
			runtimeID = "claude"
		}
		child.Envelope.Resolution.RuntimeID = runtimeID
		child.Envelope.Resolution.ModelProfileID = runtimeID + "-profile"
		attemptID := fmt.Sprintf("00000000-0000-4000-8000-%012d", 500+runtimeIndex)
		resultReference := "result:" + child.NodeKey
		child.Attempts = []Attempt{{AttemptID: attemptID, Number: 1, Status: AttemptSucceeded, StartedAt: &now, FinishedAt: &now, ResultReference: resultReference, OutputDigest: digestOf("output-" + runtimeID)}}
		dispatch = append(dispatch, DispatchRecord{ChildID: child.ExecutionID, AttemptID: attemptID, Status: AttemptSucceeded, StartedAt: now, EndedAt: now})
		runtimeExecutions = append(runtimeExecutions, RuntimeExecutionEvidence{ChildID: child.ExecutionID, AttemptID: attemptID, RuntimeID: runtimeID, ModelProfileID: child.Envelope.Resolution.ModelProfileID, RuntimeVersion: runtimeID + "-1.0", ConfigurationRevision: child.Envelope.Resolution.ConfigurationRevision, ObservationRevision: child.Envelope.Resolution.ObservationRevision, InvocationDigest: digestOf("invocation-" + runtimeID), ResultReference: resultReference})
		runtimeIndex++
	}
	if !ValidGraph(graph) {
		t.Fatal("invalid complete runtime graph")
	}
	rollup := rollupGraph(graph)
	rollup.Status = "success"
	rollup.IntegrationResultTree = digestOf("integrated")
	rollup.ValidationResults = []ValidationResult{{CommandReference: "go-test", ExitCode: 0, OutputDigest: digestOf("validation")}}
	rollup.References = []string{"integration:result"}
	for index := range rollup.ChildOutcomes {
		if rollup.ChildOutcomes[index].ChildID == rollup.IntegrationChildID {
			rollup.ChildOutcomes[index].Status = AttemptSucceeded
		}
	}
	evidence := Evidence{FormatVersion: 1, BaseRevision: "8a9ca19fd260bc19f5bb288b449e8dc6df618194", ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Dispatch: dispatch, RuntimeExecutions: runtimeExecutions, CoordinationDigests: []string{digestOf("coordination")}, Rollup: rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"usage unavailable"}}
	return graph, evidence
}
