package executiongraph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const MaxCapturedOutputBytes = 256 << 10

var ErrInvalidInvocation = errors.New("invalid runtime invocation")

type Invocation struct {
	RuntimeID string
	Argv      []string
	CWD       string
	Env       []string
	OutputMax int
}

type InvocationResolver interface {
	ResolveInvocation(context.Context, ChildExecution) (Invocation, error)
}

type ProcessResult struct {
	ExitCode        int
	Output          []byte
	ResultReference string
	EffectAmbiguous bool
	Stopped         bool
}

type ProcessRunner interface {
	Run(context.Context, Invocation) ProcessResult
}

type WorkspaceValidator interface {
	ValidateWorkspace(context.Context, ChildExecution) error
}

type GraphAttemptStore interface {
	Save(context.Context, Graph) (Graph, error)
}

type DispatchRequest struct {
	Graph           Graph
	RetryChildIDs   map[string]bool
	CancelRequested bool
}

type DispatchRecord struct {
	ChildID, AttemptID string
	Status             AttemptStatus
	StartedAt, EndedAt time.Time
}

type DispatchResult struct {
	Graph   Graph
	Records []DispatchRecord
	Blocked map[string]string
}

type Scheduler struct {
	invocations InvocationResolver
	runner      ProcessRunner
	workspaces  WorkspaceValidator
	store       GraphAttemptStore
	allocateID  func() (string, error)
	now         func() time.Time
}

func NewScheduler(invocations InvocationResolver, runner ProcessRunner, workspaces WorkspaceValidator, store GraphAttemptStore, allocateID func() (string, error), now func() time.Time) Scheduler {
	if allocateID == nil {
		allocateID = randomOpaqueID
	}
	if now == nil {
		now = time.Now
	}
	return Scheduler{invocations: invocations, runner: runner, workspaces: workspaces, store: store, allocateID: allocateID, now: now}
}

func (s Scheduler) DispatchReady(ctx context.Context, request DispatchRequest) (DispatchResult, error) {
	result := DispatchResult{Graph: request.Graph, Blocked: map[string]string{}}
	if !ValidGraph(result.Graph) || s.invocations == nil || s.runner == nil || s.workspaces == nil {
		return result, ErrInvalidGraph
	}
	if request.CancelRequested || ctx.Err() != nil {
		for _, child := range result.Graph.Children {
			if len(child.Attempts) == 0 {
				result.Blocked[child.ExecutionID] = "cancelled_before_dispatch"
			}
		}
		return result, nil
	}
	ready := readyChildren(result.Graph, request.RetryChildIDs, result.Blocked)
	batch := nonConflictingBatch(result.Graph, ready, result.Blocked)
	if len(batch) == 0 {
		return result, nil
	}
	type completed struct {
		index   int
		attempt Attempt
		record  DispatchRecord
	}
	type planned struct {
		index      int
		child      ChildExecution
		invocation Invocation
		attemptID  string
		number     uint32
		started    time.Time
	}
	plans := make([]planned, 0, len(batch))
	for _, index := range batch {
		child := result.Graph.Children[index]
		if err := s.workspaces.ValidateWorkspace(ctx, child); err != nil {
			result.Blocked[child.ExecutionID] = "invalid_workspace"
			continue
		}
		invocation, err := s.invocations.ResolveInvocation(ctx, child)
		if err != nil || !validInvocation(child, invocation) {
			result.Blocked[child.ExecutionID] = "invalid_invocation"
			continue
		}
		attemptID, err := s.allocateID()
		if err != nil || !validOpaqueID(attemptID) {
			return result, ErrInvalidGraph
		}
		started := s.now().UTC()
		attemptNumber := uint32(len(child.Attempts) + 1)
		running := Attempt{AttemptID: attemptID, Number: attemptNumber, Status: AttemptRunning, StartedAt: &started}
		result.Graph.Children[index].Attempts = append(result.Graph.Children[index].Attempts, running)
		plans = append(plans, planned{index: index, child: child, invocation: invocation, attemptID: attemptID, number: attemptNumber, started: started})
	}
	if len(plans) == 0 {
		return result, nil
	}
	if s.store != nil {
		persisted, err := s.store.Save(ctx, result.Graph)
		if err != nil {
			return result, err
		}
		result.Graph = persisted
	}
	completedCh := make(chan completed, len(plans))
	var wg sync.WaitGroup
	for _, plan := range plans {
		wg.Add(1)
		go func(index int, child ChildExecution, invocation Invocation, attemptID string, attemptNumber uint32, started time.Time) {
			defer wg.Done()
			attemptCtx, cancel := context.WithTimeout(ctx, child.Envelope.Controls.Timeout)
			defer cancel()
			process := s.runner.Run(attemptCtx, invocation)
			ended := s.now().UTC()
			status := AttemptSucceeded
			if attemptCtx.Err() != nil {
				if process.Stopped {
					status = AttemptCancelled
				} else {
					status = AttemptUnknown
				}
			} else if process.ExitCode != 0 {
				status = AttemptFailed
			}
			digest := sha256.Sum256(process.Output)
			attempt := Attempt{AttemptID: attemptID, Number: attemptNumber, Status: status, StartedAt: &started, FinishedAt: &ended, ResultReference: process.ResultReference, OutputDigest: hex.EncodeToString(digest[:]), ExitCode: process.ExitCode, AmbiguousEffect: process.EffectAmbiguous, CancellationSeen: attemptCtx.Err() != nil}
			completedCh <- completed{index: index, attempt: attempt, record: DispatchRecord{ChildID: child.ExecutionID, AttemptID: attemptID, Status: status, StartedAt: started, EndedAt: ended}}
		}(plan.index, plan.child, plan.invocation, plan.attemptID, plan.number, plan.started)
	}
	wg.Wait()
	close(completedCh)
	items := make([]completed, 0, len(batch))
	for item := range completedCh {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return result.Graph.Children[items[i].index].ExecutionID < result.Graph.Children[items[j].index].ExecutionID
	})
	for _, item := range items {
		attemptIndex := len(result.Graph.Children[item.index].Attempts) - 1
		result.Graph.Children[item.index].Attempts[attemptIndex] = item.attempt
		result.Records = append(result.Records, item.record)
	}
	if len(items) > 0 && s.store != nil {
		persisted, err := s.store.Save(ctx, result.Graph)
		if err != nil {
			return result, err
		}
		result.Graph = persisted
	}
	return result, nil
}

func readyChildren(graph Graph, retries map[string]bool, blocked map[string]string) []int {
	succeeded := map[string]bool{}
	for _, child := range graph.Children {
		if len(child.Attempts) > 0 && child.Attempts[len(child.Attempts)-1].Status == AttemptSucceeded {
			succeeded[child.ExecutionID] = true
		}
	}
	ready := make([]int, 0, len(graph.Children))
	for index, child := range graph.Children {
		if child.Envelope.IntegrationOwner {
			blocked[child.ExecutionID] = "integration_pending"
			continue
		}
		if succeeded[child.ExecutionID] {
			continue
		}
		dependenciesReady := true
		for _, dependency := range child.Envelope.Dependencies {
			if !succeeded[dependency] {
				dependenciesReady = false
				break
			}
		}
		if !dependenciesReady {
			blocked[child.ExecutionID] = "dependencies_unsatisfied"
			continue
		}
		if len(child.Attempts) > 0 {
			last := child.Attempts[len(child.Attempts)-1]
			if last.AmbiguousEffect {
				blocked[child.ExecutionID] = "ambiguous_effect"
				continue
			}
			if uint32(len(child.Attempts)) >= child.Envelope.Controls.MaximumAttempts {
				blocked[child.ExecutionID] = "attempts_exhausted"
				continue
			}
			if !retries[child.ExecutionID] {
				blocked[child.ExecutionID] = "retry_not_authorized"
				continue
			}
		}
		ready = append(ready, index)
	}
	sort.Slice(ready, func(i, j int) bool {
		return graph.Children[ready[i]].ExecutionID < graph.Children[ready[j]].ExecutionID
	})
	return ready
}

func nonConflictingBatch(graph Graph, ready []int, blocked map[string]string) []int {
	batch := make([]int, 0, len(ready))
	for _, candidate := range ready {
		conflict := false
		for _, selected := range batch {
			if childEffectsConflict(graph.Children[candidate], graph.Children[selected]) {
				conflict = true
				break
			}
		}
		if conflict {
			blocked[graph.Children[candidate].ExecutionID] = "effect_conflict_serialized"
			continue
		}
		batch = append(batch, candidate)
	}
	return batch
}

func childEffectsConflict(left, right ChildExecution) bool {
	leftNode := ProposedNode{Scope: left.Envelope.Scope, Effects: left.Envelope.AllowedEffects}
	rightNode := ProposedNode{Scope: right.Envelope.Scope, Effects: right.Envelope.AllowedEffects}
	return effectsConflict(leftNode, rightNode)
}

func validInvocation(child ChildExecution, invocation Invocation) bool {
	if invocation.RuntimeID != child.Envelope.Resolution.RuntimeID || len(invocation.Argv) == 0 || len(invocation.Argv) > 64 || invocation.CWD != child.Envelope.Workspace || !filepath.IsAbs(invocation.CWD) || invocation.OutputMax <= 0 || invocation.OutputMax > MaxCapturedOutputBytes || len(invocation.Env) > 64 {
		return false
	}
	base := strings.ToLower(filepath.Base(invocation.Argv[0]))
	if !filepath.IsAbs(invocation.Argv[0]) || base == "sh" || base == "bash" || base == "zsh" || base == "fish" || base == "cmd" || base == "powershell" || base == "pwsh" {
		return false
	}
	for _, argument := range invocation.Argv {
		if argument == "" || strings.ContainsRune(argument, '\x00') {
			return false
		}
	}
	seen := map[string]bool{}
	for _, item := range invocation.Env {
		key, _, ok := strings.Cut(item, "=")
		if !ok || !validEnvironmentKey(key) || seen[key] || strings.ContainsRune(item, '\x00') {
			return false
		}
		seen[key] = true
	}
	return true
}

func validEnvironmentKey(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, char := range value {
		if !((char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9')) {
			return false
		}
	}
	return true
}

type OSProcessRunner struct{}

func (OSProcessRunner) Run(ctx context.Context, invocation Invocation) ProcessResult {
	command := exec.CommandContext(ctx, invocation.Argv[0], invocation.Argv[1:]...)
	command.Dir = invocation.CWD
	command.Env = append([]string(nil), invocation.Env...)
	output := &limitedBuffer{remaining: invocation.OutputMax}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	// CommandContext observes its direct child. Descendant processes or external
	// effects can remain uncertain, so timeout/cancellation is not called stopped.
	return ProcessResult{ExitCode: exitCode, Output: output.Bytes(), Stopped: false}
}

type limitedBuffer struct {
	bytes.Buffer
	remaining int
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	if len(value) > b.remaining {
		value = value[:b.remaining]
	}
	if len(value) > 0 {
		_, _ = b.Buffer.Write(value)
		b.remaining -= len(value)
	}
	return original, nil
}
