package executiongraph

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type invocationFake struct{ invalid bool }

func (f invocationFake) ResolveInvocation(_ context.Context, child ChildExecution) (Invocation, error) {
	executable := "/usr/bin/true"
	if f.invalid {
		executable = "/bin/sh"
	}
	return Invocation{RuntimeID: child.Envelope.Resolution.RuntimeID, Argv: []string{executable, "literal;not-shell"}, CWD: child.Envelope.Workspace, Env: []string{"PATH=/usr/bin"}, OutputMax: 1024}, nil
}

type workspaceFake struct{ reject string }

func (f workspaceFake) ValidateWorkspace(_ context.Context, child ChildExecution) error {
	if child.ExecutionID == f.reject {
		return fmt.Errorf("rejected")
	}
	return nil
}

type concurrentRunner struct {
	active, maximum atomic.Int32
	started         chan struct{}
	release         chan struct{}
	ambiguous       bool
	stopUnconfirmed bool
	exitCode        int
}

func (r *concurrentRunner) Run(ctx context.Context, _ Invocation) ProcessResult {
	active := r.active.Add(1)
	for {
		maximum := r.maximum.Load()
		if active <= maximum || r.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	r.started <- struct{}{}
	select {
	case <-r.release:
	case <-ctx.Done():
		r.active.Add(-1)
		return ProcessResult{ExitCode: -1, Stopped: !r.stopUnconfirmed}
	}
	r.active.Add(-1)
	return ProcessResult{ExitCode: r.exitCode, Output: []byte("bounded"), EffectAmbiguous: r.ambiguous}
}

type attemptStoreFake struct {
	mu    sync.Mutex
	graph Graph
	saves []Graph
}

func (s *attemptStoreFake) Save(_ context.Context, graph Graph) (Graph, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.graph = graph
	s.saves = append(s.saves, graph)
	return graph, nil
}

func TestSchedulerDispatchesIndependentChildrenConcurrentlyAndBlocksDependency(t *testing.T) {
	graph := mustGraph(t)
	runner := &concurrentRunner{started: make(chan struct{}, 2), release: make(chan struct{}, 2)}
	store := &attemptStoreFake{}
	scheduler := NewScheduler(invocationFake{}, runner, workspaceFake{}, store, sequenceAllocator(), time.Now)
	done := make(chan DispatchResult, 1)
	go func() {
		result, err := scheduler.DispatchReady(context.Background(), DispatchRequest{Graph: graph})
		if err != nil {
			t.Errorf("dispatch: %v", err)
		}
		done <- result
	}()
	<-runner.started
	<-runner.started
	if runner.maximum.Load() != 2 {
		t.Fatalf("maximum concurrency=%d", runner.maximum.Load())
	}
	store.mu.Lock()
	if len(store.saves) != 1 {
		store.mu.Unlock()
		t.Fatalf("pre-dispatch saves=%d", len(store.saves))
	}
	for _, child := range store.saves[0].Children {
		if child.Envelope.IntegrationOwner {
			continue
		}
		if len(child.Attempts) != 1 || child.Attempts[0].Status != AttemptRunning {
			store.mu.Unlock()
			t.Fatalf("running attempt not persisted: %+v", child.Attempts)
		}
	}
	store.mu.Unlock()
	runner.release <- struct{}{}
	runner.release <- struct{}{}
	result := <-done
	if len(result.Records) != 2 {
		t.Fatalf("records=%+v blocked=%v", result.Records, result.Blocked)
	}
	store.mu.Lock()
	if len(store.saves) != 2 {
		store.mu.Unlock()
		t.Fatalf("final saves=%d", len(store.saves))
	}
	store.mu.Unlock()
	if _, blocked := result.Blocked[graph.Parent.IntegrationChild]; !blocked {
		t.Fatalf("integration dependency not blocked: %v", result.Blocked)
	}
}

func TestSchedulerSerializesConflictingEffects(t *testing.T) {
	graph := mustGraph(t)
	var first, second int
	for index := range graph.Children {
		if graph.Children[index].Envelope.IntegrationOwner {
			continue
		}
		if first == 0 {
			first = index + 1
		} else {
			second = index + 1
		}
	}
	left, right := first-1, second-1
	graph.Children[right].Envelope.Scope.RepositoryKey = graph.Children[left].Envelope.Scope.RepositoryKey
	graph.Children[right].Envelope.AllowedEffects = append([]Effect(nil), graph.Children[left].Envelope.AllowedEffects...)
	runner := &concurrentRunner{started: make(chan struct{}, 1), release: make(chan struct{}, 1)}
	scheduler := NewScheduler(invocationFake{}, runner, workspaceFake{}, nil, sequenceAllocator(), time.Now)
	done := make(chan DispatchResult, 1)
	go func() {
		result, _ := scheduler.DispatchReady(context.Background(), DispatchRequest{Graph: graph})
		done <- result
	}()
	<-runner.started
	runner.release <- struct{}{}
	result := <-done
	if len(result.Records) != 1 || runner.maximum.Load() != 1 {
		t.Fatalf("records=%+v maximum=%d", result.Records, runner.maximum.Load())
	}
	conflictBlocks := 0
	for _, reason := range result.Blocked {
		if reason == "effect_conflict_serialized" {
			conflictBlocks++
		}
	}
	if conflictBlocks != 1 {
		t.Fatalf("blocked=%v", result.Blocked)
	}
}

func TestSchedulerRejectsShellInvocationBeforeProcess(t *testing.T) {
	graph := mustGraph(t)
	runner := &concurrentRunner{started: make(chan struct{}, 1), release: make(chan struct{}, 1)}
	result, err := NewScheduler(invocationFake{invalid: true}, runner, workspaceFake{}, nil, sequenceAllocator(), time.Now).DispatchReady(context.Background(), DispatchRequest{Graph: graph})
	if err != nil || len(result.Records) != 0 || runner.maximum.Load() != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestSchedulerTimeoutAndAmbiguousRetryBlock(t *testing.T) {
	graph := mustGraph(t)
	for index := range graph.Children {
		if !graph.Children[index].Envelope.IntegrationOwner {
			graph.Children[index].Envelope.Controls.Timeout = time.Millisecond
		}
	}
	runner := &concurrentRunner{started: make(chan struct{}, 2), release: make(chan struct{})}
	result, err := NewScheduler(invocationFake{}, runner, workspaceFake{}, nil, sequenceAllocator(), time.Now).DispatchReady(context.Background(), DispatchRequest{Graph: graph})
	if err != nil || len(result.Records) != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, record := range result.Records {
		if record.Status != AttemptCancelled {
			t.Fatalf("record=%+v", record)
		}
	}
	graph = result.Graph
	for index := range graph.Children {
		if len(graph.Children[index].Attempts) > 0 {
			graph.Children[index].Attempts[0].AmbiguousEffect = true
		}
	}
	retry := map[string]bool{}
	for _, child := range graph.Children {
		retry[child.ExecutionID] = true
	}
	blocked, err := NewScheduler(invocationFake{}, runner, workspaceFake{}, nil, sequenceAllocator(), time.Now).DispatchReady(context.Background(), DispatchRequest{Graph: graph, RetryChildIDs: retry})
	if err != nil || len(blocked.Records) != 0 {
		t.Fatalf("blocked=%+v err=%v", blocked, err)
	}
	for _, child := range graph.Children {
		if child.Envelope.IntegrationOwner {
			continue
		}
		if blocked.Blocked[child.ExecutionID] != "ambiguous_effect" {
			t.Fatalf("blocked=%v", blocked.Blocked)
		}
	}
}

func TestSchedulerUnknownTimeoutRequiresReconciliationEvenWhenFlagIsInconsistent(t *testing.T) {
	graph := mustGraph(t)
	for index := range graph.Children {
		if !graph.Children[index].Envelope.IntegrationOwner {
			graph.Children[index].Envelope.Controls.Timeout = time.Millisecond
		}
	}
	runner := &concurrentRunner{started: make(chan struct{}, 2), release: make(chan struct{}), stopUnconfirmed: true}
	result, err := NewScheduler(invocationFake{}, runner, workspaceFake{}, nil, sequenceAllocator(), time.Now).DispatchReady(context.Background(), DispatchRequest{Graph: graph})
	if err != nil || len(result.Records) != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	retry := map[string]bool{}
	for index := range result.Graph.Children {
		child := &result.Graph.Children[index]
		if child.Envelope.IntegrationOwner {
			continue
		}
		if len(child.Attempts) != 1 || child.Attempts[0].Status != AttemptUnknown || !child.Attempts[0].AmbiguousEffect {
			t.Fatalf("attempts=%+v", child.Attempts)
		}
		child.Attempts[0].AmbiguousEffect = false
		retry[child.ExecutionID] = true
	}
	blocked, err := NewScheduler(invocationFake{}, runner, workspaceFake{}, nil, sequenceAllocator(), time.Now).DispatchReady(context.Background(), DispatchRequest{Graph: result.Graph, RetryChildIDs: retry})
	if err != nil || len(blocked.Records) != 0 {
		t.Fatalf("blocked=%+v err=%v", blocked, err)
	}
	for _, child := range blocked.Graph.Children {
		if child.Envelope.IntegrationOwner {
			continue
		}
		if blocked.Blocked[child.ExecutionID] != "unknown_outcome_requires_reconciliation" || len(child.Attempts) != 1 {
			t.Fatalf("child=%+v blocked=%v", child, blocked.Blocked)
		}
	}
}

func TestSchedulerRetryPreservesPriorAttempt(t *testing.T) {
	graph := mustGraph(t)
	failedRunner := &concurrentRunner{started: make(chan struct{}, 2), release: make(chan struct{}, 2), exitCode: 1}
	failedRunner.release <- struct{}{}
	failedRunner.release <- struct{}{}
	first, err := NewScheduler(invocationFake{}, failedRunner, workspaceFake{}, nil, sequenceAllocatorFrom(100), time.Now).DispatchReady(context.Background(), DispatchRequest{Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	childIndex := -1
	for index, child := range first.Graph.Children {
		if !child.Envelope.IntegrationOwner {
			childIndex = index
			break
		}
	}
	prior := first.Graph.Children[childIndex].Attempts[0]
	successRunner := &concurrentRunner{started: make(chan struct{}, 1), release: make(chan struct{}, 1)}
	successRunner.release <- struct{}{}
	childID := first.Graph.Children[childIndex].ExecutionID
	second, err := NewScheduler(invocationFake{}, successRunner, workspaceFake{}, nil, sequenceAllocatorFrom(200), time.Now).DispatchReady(context.Background(), DispatchRequest{Graph: first.Graph, RetryChildIDs: map[string]bool{childID: true}})
	if err != nil || len(second.Records) != 1 {
		t.Fatalf("result=%+v err=%v", second, err)
	}
	attempts := second.Graph.Children[childIndex].Attempts
	if len(attempts) != 2 || !reflect.DeepEqual(attempts[0], prior) || attempts[1].Number != 2 || attempts[1].AttemptID == prior.AttemptID || attempts[1].Status != AttemptSucceeded {
		t.Fatalf("attempts=%+v prior=%+v", attempts, prior)
	}
}

func TestSchedulerCancellationStopsNewDispatch(t *testing.T) {
	graph := mustGraph(t)
	runner := &concurrentRunner{started: make(chan struct{}, 1), release: make(chan struct{}, 1)}
	result, err := NewScheduler(invocationFake{}, runner, workspaceFake{}, nil, sequenceAllocator(), time.Now).DispatchReady(context.Background(), DispatchRequest{Graph: graph, CancelRequested: true})
	if err != nil || len(result.Records) != 0 || runner.maximum.Load() != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func mustGraph(t *testing.T) Graph {
	t.Helper()
	proposal := mustProposal(t)
	graph, err := NewGraphService(&memoryGraphStore{}, sequenceAllocatorFrom(0), func() time.Time { return time.Unix(10, 0).UTC() }).Publish(context.Background(), publicationRequest(proposal))
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func sequenceAllocator() func() (string, error) {
	return sequenceAllocatorFrom(100)
}

func sequenceAllocatorFrom(start uint64) func() (string, error) {
	var next atomic.Uint64
	next.Store(start)
	return func() (string, error) {
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", next.Add(1)), nil
	}
}
