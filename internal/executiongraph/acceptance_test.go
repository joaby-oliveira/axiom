package executiongraph

import "testing"

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
	evidence, err := BuildEvidence(graph, Evidence{FormatVersion: 1, Status: "real_run_recorded", BaseRevision: "8a9ca19fd260bc19f5bb288b449e8dc6df618194", ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Rollup: rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"real Codex plus Claude run not executed"}}, nil)
	if err != nil || evidence.Status != "deterministic_preparation_only" || evidence.Digest == "" {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
}

func TestBuildEvidenceRejectsCallerAssertedCoordination(t *testing.T) {
	graph := mustGraph(t)
	child := graph.Children[0]
	base := Evidence{FormatVersion: 1, BaseRevision: "8a9ca19fd260bc19f5bb288b449e8dc6df618194", ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Rollup: rollupGraph(graph), ValidationReferences: []string{"go-test"}, Limitations: []string{"usage unavailable"}}
	asserted := base
	asserted.Coordination = []CoordinationEvidence{{RecordID: "00000000-0000-4000-8000-000000000701", Kind: CoordinationQuestionRequest, ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ChildID: child.ExecutionID, Digest: digestOf("question")}}
	if _, err := BuildEvidence(graph, asserted, nil); err == nil {
		t.Fatal("caller-asserted coordination accepted")
	}
	unverified := base
	unverified.CoordinationRecords = [][]byte{[]byte("{}\n")}
	if _, err := BuildEvidence(graph, unverified, nil); err == nil {
		t.Fatal("coordination records accepted without a verifier")
	}
}
