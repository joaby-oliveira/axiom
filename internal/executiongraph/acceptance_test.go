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
	evidence, err := BuildEvidence(graph, Evidence{FormatVersion: 1, Status: "real_run_recorded", BaseRevision: "8a9ca19fd260bc19f5bb288b449e8dc6df618194", ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Rollup: rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"real Codex plus Claude run not executed"}})
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

func TestBuildEvidenceRequiresObservedConcurrentExecution(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Graph, *Evidence, string, string)
		status string
	}{
		{"independent overlapping", func(*Graph, *Evidence, string, string) {}, "real_run_recorded"},
		{"independent sequential", func(graph *Graph, evidence *Evidence, codex, claude string) {
			setAttemptWindow(graph, evidence, codex, 30, 40)
			setAttemptWindow(graph, evidence, claude, 50, 60)
		}, "deterministic_preparation_only"},
		{"end equals start", func(graph *Graph, evidence *Evidence, codex, claude string) {
			setAttemptWindow(graph, evidence, codex, 30, 40)
			setAttemptWindow(graph, evidence, claude, 40, 50)
		}, "deterministic_preparation_only"},
		{"start equals end", func(graph *Graph, evidence *Evidence, codex, claude string) {
			setAttemptWindow(graph, evidence, codex, 40, 50)
			setAttemptWindow(graph, evidence, claude, 30, 40)
		}, "deterministic_preparation_only"},
		{"dependent overlapping", func(graph *Graph, _ *Evidence, codex, claude string) {
			for index := range graph.Children {
				if graph.Children[index].ExecutionID == claude {
					graph.Children[index].Envelope.Dependencies = append(append([]string(nil), graph.Children[index].Envelope.Dependencies...), codex)
				}
			}
		}, "deterministic_preparation_only"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, evidence := completeRuntimeEvidenceFixture(t)
			test.mutate(&graph, &evidence, evidence.RuntimeExecutions[0].ChildID, evidence.RuntimeExecutions[1].ChildID)
			built, err := BuildEvidence(graph, evidence)
			if err != nil || built.Status != test.status {
				t.Fatalf("built=%+v err=%v", built, err)
			}
		})
	}
}

func TestBuildEvidenceRequiresStructuredCoordinationExchange(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Evidence)
		status string
	}{
		{"question and correlated answer", func(*Evidence) {}, "real_run_recorded"},
		{"contract proposal and correlated acceptance", func(evidence *Evidence) {
			evidence.Coordination[0].Kind = CoordinationContractProposal
			evidence.Coordination[1].Kind = CoordinationContractAcceptance
			evidence.Coordination[1].Decision = CoordinationAccepted
		}, "real_run_recorded"},
		{"only question", func(evidence *Evidence) { evidence.Coordination = evidence.Coordination[:1] }, "deterministic_preparation_only"},
		{"only answer", func(evidence *Evidence) { evidence.Coordination = evidence.Coordination[1:] }, "deterministic_preparation_only"},
		{"proposal without acceptance", func(evidence *Evidence) {
			evidence.Coordination[0].Kind = CoordinationContractProposal
			evidence.Coordination = evidence.Coordination[:1]
		}, "deterministic_preparation_only"},
		{"proposal with rejected decision", func(evidence *Evidence) {
			evidence.Coordination[0].Kind = CoordinationContractProposal
			evidence.Coordination[1].Kind = CoordinationContractAcceptance
			evidence.Coordination[1].Decision = "rejected"
		}, "deterministic_preparation_only"},
		{"answer to uncorrelated question", func(evidence *Evidence) {
			evidence.Coordination[1].Reference = "00000000-0000-4000-8000-000000000999"
		}, "deterministic_preparation_only"},
		{"answer of mismatched kind", func(evidence *Evidence) {
			evidence.Coordination[1].Kind = CoordinationContractAcceptance
			evidence.Coordination[1].Decision = CoordinationAccepted
		}, "deterministic_preparation_only"},
		{"same child exchange", func(evidence *Evidence) {
			evidence.Coordination[1].ChildID = evidence.Coordination[0].ChildID
			evidence.Coordination[1].AttemptID = evidence.Coordination[0].AttemptID
		}, "deterministic_preparation_only"},
		{"exchange outside runtime attempts", func(evidence *Evidence) {
			evidence.Coordination[0].AttemptID = ""
			evidence.Coordination[1].AttemptID = ""
		}, "deterministic_preparation_only"},
		{"no coordination", func(evidence *Evidence) { evidence.Coordination = nil }, "deterministic_preparation_only"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, evidence := completeRuntimeEvidenceFixture(t)
			test.mutate(&evidence)
			built, err := BuildEvidence(graph, evidence)
			if err != nil || built.Status != test.status {
				t.Fatalf("built=%+v err=%v", built, err)
			}
		})
	}
}

func TestBuildEvidenceRejectsForeignOrUnstructuredCoordination(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Graph, *CoordinationEvidence)
	}{
		{"foreign parent", func(_ *Graph, record *CoordinationEvidence) { record.ParentID = "00000000-0000-4000-8000-000000000999" }},
		{"foreign graph revision", func(_ *Graph, record *CoordinationEvidence) { record.GraphRevision++ }},
		{"foreign child", func(_ *Graph, record *CoordinationEvidence) { record.ChildID = "00000000-0000-4000-8000-000000000999" }},
		{"foreign attempt", func(_ *Graph, record *CoordinationEvidence) {
			record.AttemptID = "00000000-0000-4000-8000-000000000999"
		}},
		{"attempt of another child", func(graph *Graph, record *CoordinationEvidence) {
			for _, child := range graph.Children {
				if child.ExecutionID != record.ChildID && len(child.Attempts) > 0 {
					record.AttemptID = child.Attempts[0].AttemptID
				}
			}
		}},
		{"arbitrary digest only", func(_ *Graph, record *CoordinationEvidence) {
			*record = CoordinationEvidence{Digest: digestOf("coordination")}
		}},
		{"missing kind", func(_ *Graph, record *CoordinationEvidence) { record.Kind = "" }},
		{"invalid digest", func(_ *Graph, record *CoordinationEvidence) { record.Digest = "coordination" }},
		{"invalid reference", func(_ *Graph, record *CoordinationEvidence) { record.Reference = "question" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, evidence := completeRuntimeEvidenceFixture(t)
			test.mutate(&graph, &evidence.Coordination[1])
			if _, err := BuildEvidence(graph, evidence); err == nil {
				t.Fatal("foreign or unstructured coordination accepted")
			}
		})
	}
	graph, evidence := completeRuntimeEvidenceFixture(t)
	evidence.Coordination = append(evidence.Coordination, evidence.Coordination[1])
	if _, err := BuildEvidence(graph, evidence); err == nil {
		t.Fatal("duplicate coordination record accepted")
	}
}

func setAttemptWindow(graph *Graph, evidence *Evidence, childID string, start, end int64) {
	startedAt, finishedAt := time.Unix(start, 0).UTC(), time.Unix(end, 0).UTC()
	for index := range graph.Children {
		if graph.Children[index].ExecutionID == childID {
			graph.Children[index].Attempts[0].StartedAt = &startedAt
			graph.Children[index].Attempts[0].FinishedAt = &finishedAt
		}
	}
	for index := range evidence.Dispatch {
		if evidence.Dispatch[index].ChildID == childID {
			evidence.Dispatch[index].StartedAt = startedAt
			evidence.Dispatch[index].EndedAt = finishedAt
		}
	}
}

func completeRuntimeEvidenceFixture(t *testing.T) (Graph, Evidence) {
	t.Helper()
	graph := mustGraph(t)
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
		startedAt := time.Unix(int64(30+5*runtimeIndex), 0).UTC()
		finishedAt := startedAt.Add(10 * time.Second)
		child.Envelope.Resolution.RuntimeID = runtimeID
		child.Envelope.Resolution.ModelProfileID = runtimeID + "-profile"
		attemptID := fmt.Sprintf("00000000-0000-4000-8000-%012d", 500+runtimeIndex)
		resultReference := "result:" + child.NodeKey
		child.Attempts = []Attempt{{AttemptID: attemptID, Number: 1, Status: AttemptSucceeded, StartedAt: &startedAt, FinishedAt: &finishedAt, ResultReference: resultReference, OutputDigest: digestOf("output-" + runtimeID)}}
		dispatch = append(dispatch, DispatchRecord{ChildID: child.ExecutionID, AttemptID: attemptID, Status: AttemptSucceeded, StartedAt: startedAt, EndedAt: finishedAt})
		runtimeExecutions = append(runtimeExecutions, RuntimeExecutionEvidence{ChildID: child.ExecutionID, AttemptID: attemptID, RuntimeID: runtimeID, ModelProfileID: child.Envelope.Resolution.ModelProfileID, RuntimeVersion: runtimeID + "-1.0", ConfigurationRevision: child.Envelope.Resolution.ConfigurationRevision, ObservationRevision: child.Envelope.Resolution.ObservationRevision, InvocationDigest: digestOf("invocation-" + runtimeID), ResultReference: resultReference})
		runtimeIndex++
	}
	if !ValidGraph(graph) || runtimeIndex < 2 {
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
	question := CoordinationEvidence{RecordID: "00000000-0000-4000-8000-000000000701", Kind: CoordinationQuestionRequest, ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ChildID: runtimeExecutions[0].ChildID, AttemptID: runtimeExecutions[0].AttemptID, Digest: digestOf("question")}
	answer := CoordinationEvidence{RecordID: "00000000-0000-4000-8000-000000000702", Kind: CoordinationAnswer, ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ChildID: runtimeExecutions[1].ChildID, AttemptID: runtimeExecutions[1].AttemptID, Digest: digestOf("answer"), Reference: question.RecordID}
	evidence := Evidence{FormatVersion: 1, BaseRevision: "8a9ca19fd260bc19f5bb288b449e8dc6df618194", ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Dispatch: dispatch, RuntimeExecutions: runtimeExecutions, Coordination: []CoordinationEvidence{question, answer}, Rollup: rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"usage unavailable"}}
	return graph, evidence
}
