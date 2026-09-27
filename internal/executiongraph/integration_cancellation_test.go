package executiongraph

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// blockingIntegrator blocks until the attempt context ends and reports the
// context error it observed, like an apply interrupted mid-effect.
type blockingIntegrator struct {
	mu       sync.Mutex
	calls    int
	deadline time.Duration
	observed error
	started  chan struct{}
}

func newBlockingIntegrator() *blockingIntegrator {
	return &blockingIntegrator{started: make(chan struct{})}
}

func (f *blockingIntegrator) Apply(ctx context.Context, child ChildExecution, _ IntegrationPreview) (AppliedIntegration, error) {
	f.mu.Lock()
	f.calls++
	if deadline, ok := ctx.Deadline(); ok {
		f.deadline = time.Until(deadline)
	}
	f.mu.Unlock()
	close(f.started)
	<-ctx.Done()
	f.mu.Lock()
	f.observed = ctx.Err()
	f.mu.Unlock()
	return AppliedIntegration{References: []string{"git-workspace:" + child.ExecutionID}}, ctx.Err()
}

// blockingValidator blocks until the attempt context ends.
type blockingValidator struct {
	mu       sync.Mutex
	calls    int
	observed error
	started  chan struct{}
}

func newBlockingValidator() *blockingValidator {
	return &blockingValidator{started: make(chan struct{})}
}

func (f *blockingValidator) ValidateCombined(ctx context.Context, _ ChildExecution, _ string) ([]ValidationResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	close(f.started)
	<-ctx.Done()
	f.mu.Lock()
	f.observed = ctx.Err()
	f.mu.Unlock()
	return nil, ctx.Err()
}

// recordingValidator records whether it was reached and the deadline it saw.
type recordingValidator struct {
	validatorFake
	calls    int
	deadline time.Duration
}

func (f *recordingValidator) ValidateCombined(ctx context.Context, child ChildExecution, tree string) ([]ValidationResult, error) {
	f.calls++
	if deadline, ok := ctx.Deadline(); ok {
		f.deadline = time.Until(deadline)
	}
	return f.validatorFake.ValidateCombined(ctx, child, tree)
}

// cancelAfterApplyIntegrator confirms its effect and then observes caller
// cancellation before returning, so combined validation cannot start.
type cancelAfterApplyIntegrator struct {
	*integratorFake
	cancel context.CancelFunc
}

func (f cancelAfterApplyIntegrator) Apply(ctx context.Context, child ChildExecution, preview IntegrationPreview) (AppliedIntegration, error) {
	applied, err := f.integratorFake.Apply(ctx, child, preview)
	f.cancel()
	<-ctx.Done()
	return applied, err
}

// contextAwareStore rejects saves on a done context like local.GraphStore.
type contextAwareStore struct {
	attemptStoreFake
}

func (s *contextAwareStore) Save(ctx context.Context, graph Graph) (Graph, error) {
	if err := ctx.Err(); err != nil {
		return graph, err
	}
	return s.attemptStoreFake.Save(ctx, graph)
}

func withIntegrationControls(t *testing.T, graph Graph, controls ExecutionControls) Graph {
	t.Helper()
	index, ok := integrationChildIndex(graph)
	if !ok {
		t.Fatal("integration child missing")
	}
	graph.Children = append([]ChildExecution(nil), graph.Children...)
	graph.Children[index].Envelope.Controls = controls
	if !ValidGraph(graph) {
		t.Fatal("invalid integration controls")
	}
	return graph
}

func executeIntegrationWith(t *testing.T, ctx context.Context, service IntegrationService, graph Graph, results []ChildResult, revision, tree string) (IntegrationResult, error) {
	t.Helper()
	preview, err := service.Preview(graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results)
	if err != nil {
		t.Fatal(err)
	}
	return service.Execute(ctx, graph, preview, IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: revision, Effects: preview.Effects, Reference: "authority:integration"})
}

func integrationEvidence(graph Graph, revision string, rollup ParentRollup) Evidence {
	return Evidence{FormatVersion: 1, BaseRevision: revision, ConfigurationDigest: digestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Rollup: rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"integration attempt cancelled"}}
}

// assertInterruptedIntegration checks the invariants every cancelled or timed
// out Integration/Reconciliation attempt must satisfy.
func assertInterruptedIntegration(t *testing.T, result IntegrationResult, err error, revision string, wantStatus AttemptStatus, wantAmbiguous bool) Attempt {
	t.Helper()
	attempts := integrationAttempts(result.Graph)
	if !errors.Is(err, ErrIntegrationFailed) || len(attempts) != 1 {
		t.Fatalf("attempts=%+v err=%v", attempts, err)
	}
	attempt := attempts[0]
	if attempt.Status != wantStatus || attempt.AmbiguousEffect != wantAmbiguous || !attempt.CancellationSeen || attempt.FinishedAt == nil || attempt.Number != 1 {
		t.Fatalf("attempt=%+v", attempt)
	}
	if result.Rollup.Status != "partial" || integrationOutcome(result.Rollup) != wantStatus {
		t.Fatalf("rollup=%+v", result.Rollup)
	}
	if _, err := BuildEvidence(result.Graph, integrationEvidence(result.Graph, revision, result.Rollup), nil); err != nil {
		t.Fatalf("truthful interrupted roll-up rejected: %v", err)
	}
	forged := result.Rollup
	forged.Status = "success"
	forged.ValidationResults = passingValidator().results
	if forged.IntegrationResultTree == "" {
		forged.IntegrationResultTree = digestOf("integrated")
	}
	if _, err := BuildEvidence(result.Graph, integrationEvidence(result.Graph, revision, forged), nil); err == nil {
		t.Fatal("success accepted over interrupted integration attempt")
	}
	return attempt
}

func TestIntegrationTimeoutDuringApplyIsNeverSuccess(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	graph = withIntegrationControls(t, graph, ExecutionControls{Timeout: 50 * time.Millisecond, MaximumAttempts: 2})
	integrator := newBlockingIntegrator()
	validator := &recordingValidator{validatorFake: passingValidator()}
	store := &attemptStoreFake{}
	service := NewIntegrationService(integrator, validator, store, sequenceAllocatorFrom(900), nil)
	started := time.Now()
	result, err := executeIntegrationWith(t, context.Background(), service, graph, results, revision, tree)
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("child timeout did not bound the attempt: %v", elapsed)
	}
	if integrator.calls != 1 || !errors.Is(integrator.observed, context.DeadlineExceeded) || integrator.deadline <= 0 || integrator.deadline > 50*time.Millisecond {
		t.Fatalf("calls=%d observed=%v deadline=%v", integrator.calls, integrator.observed, integrator.deadline)
	}
	if validator.calls != 0 {
		t.Fatal("combined validation ran after interrupted apply")
	}
	attempt := assertInterruptedIntegration(t, result, err, revision, AttemptUnknown, true)
	if attempt.ResultReference != "" || result.Rollup.IntegrationResultTree != "" {
		t.Fatalf("unconfirmed apply reported a result: attempt=%+v rollup=%+v", attempt, result.Rollup)
	}
	if len(store.saves) != 2 || integrationAttempts(store.saves[0])[0].Status != AttemptRunning {
		t.Fatalf("saves=%+v", store.saves)
	}
	if _, err := service.Preview(result.Graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results); !errors.Is(err, ErrIntegrationBlocked) {
		t.Fatalf("new attempt allowed over ambiguous timed-out apply: %v", err)
	}
}

func TestIntegrationExternalCancellationDuringApplyIsRecorded(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	graph = withIntegrationControls(t, graph, ExecutionControls{Timeout: time.Hour, MaximumAttempts: 3})
	integrator := newBlockingIntegrator()
	store := &contextAwareStore{}
	service := NewIntegrationService(integrator, passingValidator(), store, sequenceAllocatorFrom(900), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-integrator.started
		cancel()
	}()
	result, err := executeIntegrationWith(t, ctx, service, graph, results, revision, tree)
	if integrator.calls != 1 || !errors.Is(integrator.observed, context.Canceled) {
		t.Fatalf("calls=%d observed=%v", integrator.calls, integrator.observed)
	}
	assertInterruptedIntegration(t, result, err, revision, AttemptUnknown, true)
	// The terminal attempt is persisted even though the caller context is done.
	if len(store.saves) != 2 || integrationAttempts(store.saves[1])[0].Status != AttemptUnknown || !integrationAttempts(store.saves[1])[0].CancellationSeen {
		t.Fatalf("saves=%+v", store.saves)
	}
	if _, err := service.Preview(result.Graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results); !errors.Is(err, ErrIntegrationBlocked) {
		t.Fatalf("new attempt allowed over cancelled ambiguous apply: %v", err)
	}
}

func TestIntegrationCancellationDuringCombinedValidationPreservesConfirmedEffect(t *testing.T) {
	for _, test := range []struct {
		name     string
		timeout  time.Duration
		external bool
		want     error
	}{
		{"timeout", 50 * time.Millisecond, false, context.DeadlineExceeded},
		{"external cancellation", time.Hour, true, context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph, results, revision, tree := integrationFixture(t)
			graph = withIntegrationControls(t, graph, ExecutionControls{Timeout: test.timeout, MaximumAttempts: 3})
			integrator := confirmedIntegrator(results, tree)
			validator := newBlockingValidator()
			store := &contextAwareStore{}
			service := NewIntegrationService(integrator, validator, store, sequenceAllocatorFrom(900), nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.external {
				go func() {
					<-validator.started
					cancel()
				}()
			}
			result, err := executeIntegrationWith(t, ctx, service, graph, results, revision, tree)
			if !integrator.applied || validator.calls != 1 || !errors.Is(validator.observed, test.want) || !errors.Is(err, test.want) {
				t.Fatalf("applied=%v calls=%d observed=%v err=%v", integrator.applied, validator.calls, validator.observed, err)
			}
			attempt := assertInterruptedIntegration(t, result, err, revision, AttemptUnknown, true)
			// The confirmed integration effect stays recorded; nothing is rolled back.
			if attempt.ResultReference != IntegrationResultReference(tree) || result.Rollup.IntegrationResultTree != tree || len(result.Rollup.References) == 0 || result.Rollup.References[0] != "integration:1" {
				t.Fatalf("confirmed effect lost: attempt=%+v rollup=%+v", attempt, result.Rollup)
			}
			if len(store.saves) != 2 {
				t.Fatalf("saves=%+v", store.saves)
			}
			if _, err := service.Preview(result.Graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results); !errors.Is(err, ErrIntegrationBlocked) {
				t.Fatalf("new attempt allowed over unreconciled validation: %v", err)
			}
		})
	}
}

func TestIntegrationCancellationAfterConfirmedApplySkipsValidation(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	graph = withIntegrationControls(t, graph, ExecutionControls{Timeout: time.Hour, MaximumAttempts: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	integrator := confirmedIntegrator(results, tree)
	validator := &recordingValidator{validatorFake: passingValidator()}
	service := NewIntegrationService(cancelAfterApplyIntegrator{integratorFake: integrator, cancel: cancel}, validator, &contextAwareStore{}, sequenceAllocatorFrom(900), nil)
	result, err := executeIntegrationWith(t, ctx, service, graph, results, revision, tree)
	if !integrator.applied || validator.calls != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("applied=%v validations=%d err=%v", integrator.applied, validator.calls, err)
	}
	attempt := assertInterruptedIntegration(t, result, err, revision, AttemptCancelled, false)
	if attempt.ResultReference != IntegrationResultReference(tree) || result.Rollup.IntegrationResultTree != tree || len(result.Rollup.ValidationResults) != 0 {
		t.Fatalf("attempt=%+v rollup=%+v", attempt, result.Rollup)
	}
	// The explicit maximum of one attempt is exhausted; no attempt is added.
	if _, err := service.Preview(result.Graph, IntegrationObservation{TargetRevision: revision, TargetTree: tree}, results); !errors.Is(err, ErrIntegrationBlocked) {
		t.Fatalf("maximum attempts expanded: %v", err)
	}
}

func TestIntegrationWithinChildTimeoutSucceeds(t *testing.T) {
	graph, results, revision, tree := integrationFixture(t)
	graph = withIntegrationControls(t, graph, ExecutionControls{Timeout: time.Minute, MaximumAttempts: 1})
	validator := &recordingValidator{validatorFake: passingValidator()}
	service := NewIntegrationService(confirmedIntegrator(results, tree), validator, &contextAwareStore{}, sequenceAllocatorFrom(900), nil)
	result, err := executeIntegrationWith(t, context.Background(), service, graph, results, revision, tree)
	attempts := integrationAttempts(result.Graph)
	if err != nil || result.Rollup.Status != "success" || len(attempts) != 1 || attempts[0].Status != AttemptSucceeded || attempts[0].CancellationSeen || attempts[0].AmbiguousEffect {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if validator.calls != 1 || validator.deadline <= 0 || validator.deadline > time.Minute {
		t.Fatalf("validation not bounded by child timeout: calls=%d deadline=%v", validator.calls, validator.deadline)
	}
	evidence, err := BuildEvidence(result.Graph, integrationEvidence(result.Graph, revision, result.Rollup), nil)
	if err != nil || evidence.Status != "deterministic_preparation_only" {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
}
