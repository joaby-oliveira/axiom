package executiongraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"
)

type integratorFake struct {
	applied bool
	result  AppliedIntegration
	err     error
}

func (f *integratorFake) Apply(_ context.Context, child ChildExecution, preview IntegrationPreview) (AppliedIntegration, error) {
	if !child.Envelope.IntegrationOwner {
		return AppliedIntegration{}, errors.New("not integration owner")
	}
	f.applied = true
	return f.result, f.err
}

type validatorFake struct {
	results []ValidationResult
	err     error
}

func (f validatorFake) ValidateCombined(_ context.Context, _ ChildExecution, _ string) ([]ValidationResult, error) {
	return f.results, f.err
}

func TestIntegrationPreviewApplyValidationAndRollup(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	integrator := &integratorFake{result: AppliedIntegration{Confirmed: true, ResultTree: tree, AppliedEffects: append([]Effect(nil), results[0].Effects...), References: []string{"integration:1"}}}
	for index := 1; index < len(results); index++ {
		integrator.result.AppliedEffects = append(integrator.result.AppliedEffects, results[index].Effects...)
	}
	integrator.result.AppliedEffects = sortedEffects(integrator.result.AppliedEffects)
	validationDigest := sha256.Sum256([]byte("validation"))
	service := NewIntegrationService(integrator, validatorFake{results: []ValidationResult{{CommandReference: "go-test", ExitCode: 0, OutputDigest: hex.EncodeToString(validationDigest[:])}}}, &attemptStoreFake{}, sequenceAllocatorFrom(900), nil)
	preview, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(context.Background(), graph, preview, IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: revision, Effects: preview.Effects, Reference: "authority:integration"})
	if err != nil || !integrator.applied || result.Rollup.Status != "success" || result.Rollup.IntegrationResultTree != tree {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestIntegrationBlocksMissingForgedStaleConflictAndForeignDrift(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	service := NewIntegrationService(&integratorFake{}, validatorFake{}, &attemptStoreFake{}, sequenceAllocatorFrom(900), nil)
	tests := []struct {
		name        string
		observation IntegrationObservation
		results     []ChildResult
	}{
		{"missing", IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results[:1]},
		{"forged", IntegrationObservation{TargetRevision: revision, TargetTree: tree}, mutateResult(results, func(result *ChildResult) { result.ParentID = "00000000-0000-4000-8000-999999999999" })},
		{"stale", IntegrationObservation{TargetRevision: digestOf("other"), TargetTree: tree}, results},
		{"conflict", IntegrationObservation{TargetRevision: revision, TargetTree: tree, Conflicts: []string{"internal/api"}}, results},
		{"foreign", IntegrationObservation{TargetRevision: revision, TargetTree: tree, ForeignChanges: []string{"README.md"}}, results},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Preview(graph, test.observation, test.results); !errors.Is(err, ErrIntegrationBlocked) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestIntegrationPreviewBindsOptionalWaiversDeterministically(t *testing.T) {
	graph, _, revision, tree := integrationFixture(t)
	optionalIDs := []string{}
	for index := range graph.Children {
		if graph.Children[index].Envelope.IntegrationOwner {
			continue
		}
		graph.Children[index].Envelope.Optional = true
		optionalIDs = append(optionalIDs, graph.Children[index].ExecutionID)
	}
	service := NewIntegrationService(&integratorFake{}, validatorFake{}, &attemptStoreFake{}, sequenceAllocatorFrom(900), nil)
	if _, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, nil); !errors.Is(err, ErrIntegrationBlocked) {
		t.Fatalf("missing waiver err=%v", err)
	}
	if _, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree, OptionalWaivers: map[string]string{optionalIDs[0]: "waiver:child-a"}}, nil); !errors.Is(err, ErrIntegrationBlocked) {
		t.Fatalf("child A waiver justified child B: %v", err)
	}
	if _, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree, OptionalWaivers: map[string]string{optionalIDs[0]: "", optionalIDs[1]: "waiver:child-b"}}, nil); !errors.Is(err, ErrIntegrationBlocked) {
		t.Fatalf("invalid waiver err=%v", err)
	}
	waivers := map[string]string{optionalIDs[0]: "waiver:child-a", optionalIDs[1]: "waiver:child-b"}
	preview, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree, OptionalWaivers: waivers}, nil)
	if err != nil || len(preview.OptionalWaivers) != 2 || preview.OptionalWaivers[0].ChildID > preview.OptionalWaivers[1].ChildID {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	changed, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree, OptionalWaivers: map[string]string{optionalIDs[0]: "waiver:child-a-revised", optionalIDs[1]: "waiver:child-b"}}, nil)
	if err != nil || changed.Digest == preview.Digest {
		t.Fatalf("changed=%+v err=%v", changed, err)
	}
	tampered := preview
	tampered.OptionalWaivers = append([]OptionalWaiver(nil), preview.OptionalWaivers...)
	tampered.OptionalWaivers[0].Reference = "waiver:tampered"
	if _, err := service.Execute(context.Background(), graph, tampered, IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: revision, Effects: preview.Effects, Reference: "authority:integration"}); !errors.Is(err, ErrIntegrationStale) {
		t.Fatalf("tampered preview err=%v", err)
	}
	reversed := map[string]string{}
	reversed[optionalIDs[1]] = "waiver:child-b"
	reversed[optionalIDs[0]] = "waiver:child-a"
	reordered, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree, OptionalWaivers: reversed}, nil)
	if err != nil || reordered.Digest != preview.Digest {
		t.Fatalf("reordered=%+v err=%v", reordered, err)
	}
}

func TestIntegrationStaleAuthorityAndValidationFailureStayNonSuccess(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	service := NewIntegrationService(&integratorFake{result: AppliedIntegration{Confirmed: true, ResultTree: tree}}, validatorFake{err: errors.New("failed")}, &attemptStoreFake{}, sequenceAllocatorFrom(900), nil)
	preview, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(context.Background(), graph, preview, IntegrationAuthority{PreviewDigest: digestOf("stale"), TargetRevision: revision, Effects: preview.Effects, Reference: "authority:integration"})
	if !errors.Is(err, ErrIntegrationStale) || result.Rollup.Status == "success" || len(integrationAttempts(result.Graph)) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func integrationFixture(t *testing.T) (Graph, []ChildResult, string, string) {
	t.Helper()
	graph := mustGraph(t)
	now := time.Unix(20, 0).UTC()
	results := []ChildResult{}
	for index := range graph.Children {
		child := &graph.Children[index]
		if child.Envelope.IntegrationOwner {
			continue
		}
		attemptID := fmt.Sprintf("00000000-0000-4000-8000-%012d", 100+index)
		child.Attempts = []Attempt{{AttemptID: attemptID, Number: 1, Status: AttemptSucceeded, StartedAt: &now, FinishedAt: &now, OutputDigest: digestOf("output")}}
		results = append(results, ChildResult{ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, AttemptID: attemptID, GraphRevision: graph.Parent.GraphRevision, EnvelopeDigest: EnvelopeDigest(child.Envelope), BaseRevision: digestOf("base"), ResultTree: digestOf(child.NodeKey), Effects: append([]Effect(nil), child.Envelope.AllowedEffects...)})
	}
	return graph, results, digestOf("base"), digestOf("integrated")
}

func digestOf(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func mutateResult(results []ChildResult, mutate func(*ChildResult)) []ChildResult {
	copyResults := append([]ChildResult(nil), results...)
	mutate(&copyResults[0])
	return copyResults
}

type failingAttemptStore struct {
	failOn int
	calls  int
	saves  []Graph
}

func (s *failingAttemptStore) Save(_ context.Context, graph Graph) (Graph, error) {
	s.calls++
	if s.calls == s.failOn {
		return Graph{}, errors.New("store unavailable")
	}
	s.saves = append(s.saves, graph)
	return graph, nil
}

func integrationAttempts(graph Graph) []Attempt {
	child, _ := integrationChild(graph)
	return child.Attempts
}

func confirmedIntegrator(results []ChildResult, tree string) *integratorFake {
	integrator := &integratorFake{result: AppliedIntegration{Confirmed: true, ResultTree: tree, References: []string{"integration:1"}}}
	for _, result := range results {
		integrator.result.AppliedEffects = append(integrator.result.AppliedEffects, result.Effects...)
	}
	return integrator
}

func passingValidator() validatorFake {
	return validatorFake{results: []ValidationResult{{CommandReference: "go-test", ExitCode: 0, OutputDigest: digestOf("validation")}}}
}

func executeIntegration(t *testing.T, service IntegrationService, graph Graph, results []ChildResult, revision, tree string) (IntegrationResult, IntegrationPreview, error) {
	t.Helper()
	preview, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(context.Background(), graph, preview, IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: revision, Effects: preview.Effects, Reference: "authority:integration"})
	return result, preview, err
}

func integrationOutcome(rollup ParentRollup) AttemptStatus {
	for _, outcome := range rollup.ChildOutcomes {
		if outcome.ChildID == rollup.IntegrationChildID {
			return outcome.Status
		}
	}
	return ""
}

func TestIntegrationPersistsOwnSucceededAttemptAndBlocksReplay(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	store := &attemptStoreFake{}
	service := NewIntegrationService(confirmedIntegrator(results, tree), passingValidator(), store, sequenceAllocatorFrom(900), nil)
	result, preview, err := executeIntegration(t, service, graph, results, revision, tree)
	if err != nil || result.Rollup.Status != "success" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(integrationAttempts(graph)) != 0 {
		t.Fatal("caller graph mutated")
	}
	if len(store.saves) != 2 || integrationAttempts(store.saves[0])[0].Status != AttemptRunning || integrationAttempts(store.saves[1])[0].Status != AttemptSucceeded {
		t.Fatalf("saves=%+v", store.saves)
	}
	attempts := integrationAttempts(result.Graph)
	if len(attempts) != 1 || attempts[0].AttemptID != "00000000-0000-4000-8000-000000000901" || attempts[0].Status != AttemptSucceeded || attempts[0].FinishedAt == nil || attempts[0].ResultReference != IntegrationResultReference(tree) || attempts[0].AmbiguousEffect || attempts[0].CancellationSeen {
		t.Fatalf("attempts=%+v", attempts)
	}
	for _, child := range result.Graph.Children {
		for _, attempt := range child.Attempts {
			if !child.Envelope.IntegrationOwner && attempt.AttemptID == attempts[0].AttemptID {
				t.Fatal("integration attempt reused a child attempt identity")
			}
		}
	}
	if integrationOutcome(result.Rollup) != AttemptSucceeded {
		t.Fatalf("rollup=%+v", result.Rollup)
	}
	evidence, err := BuildEvidence(result.Graph, Evidence{FormatVersion: 1, BaseRevision: revision, ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Rollup: result.Rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"real Codex plus Claude run not executed"}}, nil)
	if err != nil || evidence.Status != "deterministic_preparation_only" {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	replay, err := service.Execute(context.Background(), result.Graph, preview, IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: revision, Effects: preview.Effects, Reference: "authority:integration"})
	if !errors.Is(err, ErrIntegrationStale) || len(integrationAttempts(replay.Graph)) != 1 || len(store.saves) != 2 {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if _, err := service.Preview(result.Graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results); !errors.Is(err, ErrIntegrationBlocked) {
		t.Fatalf("preview after success err=%v", err)
	}
}

func TestIntegrationValidatorFailurePersistsFailedAttempt(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	service := NewIntegrationService(confirmedIntegrator(results, tree), validatorFake{results: []ValidationResult{{CommandReference: "go-test", ExitCode: 1, OutputDigest: digestOf("validation")}}, err: errors.New("exit 1")}, &attemptStoreFake{}, sequenceAllocatorFrom(900), nil)
	result, _, err := executeIntegration(t, service, graph, results, revision, tree)
	attempts := integrationAttempts(result.Graph)
	if !errors.Is(err, ErrIntegrationFailed) || result.Rollup.Status == "success" || len(attempts) != 1 || attempts[0].Status != AttemptFailed || integrationOutcome(result.Rollup) != AttemptFailed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestIntegrationApplyFailurePersistsUnknownAttemptAndBlocksRetry(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	for _, integrator := range []*integratorFake{{err: errors.New("apply interrupted")}, {result: AppliedIntegration{Confirmed: false, ResultTree: tree}}} {
		service := NewIntegrationService(integrator, passingValidator(), &attemptStoreFake{}, sequenceAllocatorFrom(900), nil)
		result, _, err := executeIntegration(t, service, graph, results, revision, tree)
		attempts := integrationAttempts(result.Graph)
		if !errors.Is(err, ErrIntegrationFailed) || result.Rollup.Status != "partial" || len(attempts) != 1 || attempts[0].Status != AttemptUnknown || !attempts[0].AmbiguousEffect || integrationOutcome(result.Rollup) != AttemptUnknown {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if _, err := service.Preview(result.Graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results); !errors.Is(err, ErrIntegrationBlocked) {
			t.Fatalf("retry after unknown err=%v", err)
		}
	}
}

func TestIntegrationPersistenceFailureNeverFabricatesSuccess(t *testing.T) {
	t.Run("before effects", func(t *testing.T) {
		graph, results, revision, tree := integrationFixture(t)
		integrator := confirmedIntegrator(results, tree)
		store := &failingAttemptStore{failOn: 1}
		result, _, err := executeIntegration(t, NewIntegrationService(integrator, passingValidator(), store, sequenceAllocatorFrom(900), nil), graph, results, revision, tree)
		if !errors.Is(err, ErrIntegrationRecoveryRequired) || integrator.applied || result.Rollup.Status != "recovery_required" || len(integrationAttempts(result.Graph)) != 0 || integrationOutcome(result.Rollup) != AttemptNotStarted {
			t.Fatalf("result=%+v applied=%v err=%v", result, integrator.applied, err)
		}
	})

	t.Run("after confirmed effects", func(t *testing.T) {
		graph, results, revision, tree := integrationFixture(t)
		integrator := confirmedIntegrator(results, tree)
		store := &failingAttemptStore{failOn: 2}
		result, _, err := executeIntegration(t, NewIntegrationService(integrator, passingValidator(), store, sequenceAllocatorFrom(900), nil), graph, results, revision, tree)
		attempts := integrationAttempts(result.Graph)
		if !errors.Is(err, ErrIntegrationRecoveryRequired) || !integrator.applied || result.Rollup.Status != "recovery_required" || len(attempts) != 1 || attempts[0].Status != AttemptRunning || integrationOutcome(result.Rollup) != AttemptRunning || result.Rollup.IntegrationResultTree != tree {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if _, err := BuildEvidence(result.Graph, Evidence{FormatVersion: 1, BaseRevision: revision, ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Rollup: result.Rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"integration attempt persistence failed"}}, nil); err != nil {
			t.Fatalf("truthful recovery roll-up rejected: %v", err)
		}
		if _, err := NewIntegrationService(integrator, passingValidator(), &attemptStoreFake{}, sequenceAllocatorFrom(950), nil).Preview(result.Graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results); !errors.Is(err, ErrIntegrationBlocked) {
			t.Fatalf("new attempt allowed over running attempt: %v", err)
		}
	})
}

func TestBuildEvidenceRejectsRollupDivergingFromPersistedIntegrationAttempt(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	result, _, err := executeIntegration(t, NewIntegrationService(confirmedIntegrator(results, tree), passingValidator(), &attemptStoreFake{}, sequenceAllocatorFrom(900), nil), graph, results, revision, tree)
	if err != nil {
		t.Fatal(err)
	}
	evidence := func(graph Graph, rollup ParentRollup) Evidence {
		return Evidence{FormatVersion: 1, BaseRevision: revision, ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Rollup: rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"usage unavailable"}}
	}
	failedGraph := result.Graph
	failedGraph.Children = append([]ChildExecution(nil), result.Graph.Children...)
	index, _ := integrationChildIndex(failedGraph)
	failedGraph.Children[index].Attempts = append([]Attempt(nil), result.Graph.Children[index].Attempts...)
	failedGraph.Children[index].Attempts[0].Status = AttemptFailed
	mismatchedTree := result.Rollup
	mismatchedTree.IntegrationResultTree = digestOf("other-tree")
	missingValidation := result.Rollup
	missingValidation.ValidationResults = nil
	successWithoutIntegration := rollupGraph(graph)
	successWithoutIntegration.Status = "success"
	successWithoutIntegration.IntegrationResultTree = tree
	successWithoutIntegration.ValidationResults = result.Rollup.ValidationResults
	assertedOutcome := successWithoutIntegration
	assertedOutcome.ChildOutcomes = append([]ChildOutcome(nil), successWithoutIntegration.ChildOutcomes...)
	for index := range assertedOutcome.ChildOutcomes {
		if assertedOutcome.ChildOutcomes[index].ChildID == assertedOutcome.IntegrationChildID {
			assertedOutcome.ChildOutcomes[index].Status = AttemptSucceeded
		}
	}
	tests := []struct {
		name   string
		graph  Graph
		rollup ParentRollup
	}{
		{"success over not-started integration", graph, successWithoutIntegration},
		{"asserted integration outcome", graph, assertedOutcome},
		{"success over failed attempt", failedGraph, result.Rollup},
		{"result tree differs from attempt", result.Graph, mismatchedTree},
		{"success without validation", result.Graph, missingValidation},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BuildEvidence(test.graph, evidence(test.graph, test.rollup), nil); err == nil {
				t.Fatal("divergent roll-up accepted")
			}
		})
	}
}
