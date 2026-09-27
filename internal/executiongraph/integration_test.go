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
	service := NewIntegrationService(integrator, validatorFake{results: []ValidationResult{{CommandReference: "go-test", ExitCode: 0, OutputDigest: hex.EncodeToString(validationDigest[:])}}})
	preview, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results)
	if err != nil {
		t.Fatal(err)
	}
	rollup, err := service.Execute(context.Background(), graph, preview, IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: revision, Effects: preview.Effects, Reference: "authority:integration"})
	if err != nil || !integrator.applied || rollup.Status != "success" || rollup.IntegrationResultTree != tree {
		t.Fatalf("rollup=%+v err=%v", rollup, err)
	}
}

func TestIntegrationBlocksMissingForgedStaleConflictAndForeignDrift(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	service := NewIntegrationService(&integratorFake{}, validatorFake{})
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

func TestIntegrationStaleAuthorityAndValidationFailureStayNonSuccess(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	service := NewIntegrationService(&integratorFake{result: AppliedIntegration{Confirmed: true, ResultTree: tree}}, validatorFake{err: errors.New("failed")})
	preview, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results)
	if err != nil {
		t.Fatal(err)
	}
	rollup, err := service.Execute(context.Background(), graph, preview, IntegrationAuthority{PreviewDigest: digestOf("stale"), TargetRevision: revision, Effects: preview.Effects, Reference: "authority:integration"})
	if !errors.Is(err, ErrIntegrationStale) || rollup.Status == "success" {
		t.Fatalf("rollup=%+v err=%v", rollup, err)
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
