package executiongraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

var (
	ErrIntegrationBlocked = errors.New("integration blocked")
	ErrIntegrationStale   = errors.New("integration preview stale")
	ErrIntegrationFailed  = errors.New("integration failed")
	// ErrIntegrationRecoveryRequired reports that the Integration/Reconciliation
	// attempt could not be persisted; confirmed effects are never reported as
	// success until the terminal attempt is canonical.
	ErrIntegrationRecoveryRequired = errors.New("integration recovery required")
)

type ArtifactReference struct {
	ID, Digest string
}

type ChildResult struct {
	ParentID, ChildID, AttemptID string
	GraphRevision                uint64
	EnvelopeDigest               string
	BaseRevision                 string
	ResultTree                   string
	Effects                      []Effect
	Artifacts                    []ArtifactReference
}

type IntegrationObservation struct {
	TargetRevision  string
	TargetTree      string
	ForeignChanges  []string
	Conflicts       []string
	OptionalWaivers map[string]string
}

type OptionalWaiver struct {
	ChildID   string
	Reference string
}

type IntegrationPreview struct {
	ParentID, IntegrationChildID string
	GraphRevision                uint64
	// IntegrationAttempts binds the preview to the integration child's
	// persisted attempt count, so one authority cannot start a second attempt.
	IntegrationAttempts        uint32
	TargetRevision, TargetTree string
	Sources                    []ChildResult
	OptionalWaivers            []OptionalWaiver
	Effects                    []Effect
	Digest                     string
}

type IntegrationAuthority struct {
	PreviewDigest  string
	TargetRevision string
	Effects        []Effect
	Reference      string
}

type AppliedIntegration struct {
	Confirmed      bool
	ResultTree     string
	AppliedEffects []Effect
	References     []string
}

type Integrator interface {
	Apply(context.Context, ChildExecution, IntegrationPreview) (AppliedIntegration, error)
}

type ValidationResult struct {
	CommandReference string
	ExitCode         int
	OutputDigest     string
}

type CombinedValidator interface {
	ValidateCombined(context.Context, ChildExecution, string) ([]ValidationResult, error)
}

type ChildOutcome struct {
	ChildID  string
	Status   AttemptStatus
	Required bool
}

type ParentRollup struct {
	Status                string
	ParentID              string
	GraphRevision         uint64
	ChildOutcomes         []ChildOutcome
	IntegrationChildID    string
	IntegrationResultTree string
	ValidationResults     []ValidationResult
	References            []string
}

// IntegrationResult carries the last persisted graph and the parent roll-up
// derived from it.
type IntegrationResult struct {
	Graph  Graph
	Rollup ParentRollup
}

type IntegrationService struct {
	integrator Integrator
	validator  CombinedValidator
	store      GraphAttemptStore
	allocateID func() (string, error)
	now        func() time.Time
}

func NewIntegrationService(integrator Integrator, validator CombinedValidator, store GraphAttemptStore, allocateID func() (string, error), now func() time.Time) IntegrationService {
	if allocateID == nil {
		allocateID = randomOpaqueID
	}
	if now == nil {
		now = time.Now
	}
	return IntegrationService{integrator: integrator, validator: validator, store: store, allocateID: allocateID, now: now}
}

func (s IntegrationService) Preview(graph Graph, observation IntegrationObservation, results []ChildResult) (IntegrationPreview, error) {
	if !ValidGraph(graph) || !validSourceRevision(observation.TargetRevision) || !validSourceRevision(observation.TargetTree) || len(observation.ForeignChanges) > 0 || len(observation.Conflicts) > 0 {
		return IntegrationPreview{}, ErrIntegrationBlocked
	}
	integration, ok := integrationChild(graph)
	if !ok || !integrationAttemptAvailable(integration) {
		return IntegrationPreview{}, ErrIntegrationBlocked
	}
	byChild := make(map[string]ChildResult, len(results))
	for _, result := range results {
		if _, exists := byChild[result.ChildID]; exists || !validChildResult(graph, result) {
			return IntegrationPreview{}, ErrIntegrationBlocked
		}
		byChild[result.ChildID] = result
	}
	preview := IntegrationPreview{ParentID: graph.Parent.ExecutionID, IntegrationChildID: integration.ExecutionID, GraphRevision: graph.Parent.GraphRevision, IntegrationAttempts: uint32(len(integration.Attempts)), TargetRevision: observation.TargetRevision, TargetTree: observation.TargetTree}
	usedWaivers := map[string]bool{}
	for _, child := range graph.Children {
		if child.Envelope.IntegrationOwner {
			continue
		}
		result, exists := byChild[child.ExecutionID]
		if !exists {
			waiverReference := observation.OptionalWaivers[child.ExecutionID]
			if child.Envelope.Optional && validTarget(waiverReference) {
				preview.OptionalWaivers = append(preview.OptionalWaivers, OptionalWaiver{ChildID: child.ExecutionID, Reference: waiverReference})
				usedWaivers[child.ExecutionID] = true
				continue
			}
			return IntegrationPreview{}, ErrIntegrationBlocked
		}
		if result.BaseRevision != observation.TargetRevision || !sameEffects(result.Effects, child.Envelope.AllowedEffects) {
			return IntegrationPreview{}, ErrIntegrationBlocked
		}
		preview.Sources = append(preview.Sources, result)
		preview.Effects = append(preview.Effects, result.Effects...)
	}
	if len(usedWaivers) != len(observation.OptionalWaivers) {
		return IntegrationPreview{}, ErrIntegrationBlocked
	}
	sort.Slice(preview.Sources, func(i, j int) bool { return preview.Sources[i].ChildID < preview.Sources[j].ChildID })
	sort.Slice(preview.OptionalWaivers, func(i, j int) bool { return preview.OptionalWaivers[i].ChildID < preview.OptionalWaivers[j].ChildID })
	preview.Effects = sortedEffects(preview.Effects)
	digest, err := integrationPreviewDigest(preview)
	if err != nil {
		return IntegrationPreview{}, err
	}
	preview.Digest = digest
	return preview, nil
}

// Execute runs the Integration/Reconciliation child as one persisted attempt.
// The running attempt is persisted before any effect; the terminal attempt is
// persisted before the roll-up may report it. Every roll-up is derived from the
// last persisted graph, so it never exceeds the canonical child outcomes.
func (s IntegrationService) Execute(ctx context.Context, graph Graph, preview IntegrationPreview, authority IntegrationAuthority) (IntegrationResult, error) {
	result := IntegrationResult{Graph: graph, Rollup: rollupGraph(graph)}
	if s.integrator == nil || s.validator == nil || s.store == nil || authority.PreviewDigest != preview.Digest || authority.TargetRevision != preview.TargetRevision || !sameEffects(authority.Effects, preview.Effects) || !validTarget(authority.Reference) {
		result.Rollup.Status = "recovery_required"
		return result, ErrIntegrationStale
	}
	index, ok := integrationChildIndex(graph)
	if !ok {
		result.Rollup.Status = "failure"
		return result, ErrIntegrationBlocked
	}
	digest, err := integrationPreviewDigest(preview)
	if err != nil || digest != preview.Digest || preview.ParentID != graph.Parent.ExecutionID || preview.GraphRevision != graph.Parent.GraphRevision || preview.IntegrationChildID != graph.Parent.IntegrationChild || preview.IntegrationAttempts != uint32(len(graph.Children[index].Attempts)) {
		result.Rollup.Status = "recovery_required"
		return result, ErrIntegrationStale
	}
	if !integrationAttemptAvailable(graph.Children[index]) {
		result.Rollup.Status = "recovery_required"
		return result, ErrIntegrationBlocked
	}
	attemptID, err := s.allocateID()
	if err != nil || !validOpaqueID(attemptID) {
		return result, ErrInvalidGraph
	}
	started := s.now().UTC()
	running := Attempt{AttemptID: attemptID, Number: uint32(len(graph.Children[index].Attempts) + 1), Status: AttemptRunning, StartedAt: &started}
	persisted, err := s.persistIntegrationAttempt(ctx, graph, index, running)
	if err != nil {
		result.Rollup.Status = "recovery_required"
		return result, err
	}
	result = IntegrationResult{Graph: persisted, Rollup: rollupGraph(persisted)}
	integration := persisted.Children[index]

	// The attempt is bounded by the integration child's own explicit timeout
	// and by caller cancellation; it is never extended or retried here.
	attemptCtx, cancel := context.WithTimeout(ctx, integration.Envelope.Controls.Timeout)
	defer cancel()
	final := running
	applied, err := s.integrator.Apply(attemptCtx, integration, preview)
	result.Rollup.References = append(result.Rollup.References, applied.References...)
	var executeErr error
	status := "partial"
	if err != nil || !applied.Confirmed || !validSourceRevision(applied.ResultTree) || !sameEffects(applied.AppliedEffects, preview.Effects) {
		// Interrupted or unconfirmed apply: the effect may be partial.
		final.Status, final.AmbiguousEffect = AttemptUnknown, true
		executeErr = ErrIntegrationFailed
		if attemptCtx.Err() != nil {
			final.CancellationSeen = true
			executeErr = fmt.Errorf("%w: %w", ErrIntegrationFailed, attemptCtx.Err())
		}
	} else if attemptCtx.Err() != nil {
		// Apply returned and confirmed its effect, but the attempt was cancelled
		// or timed out before combined validation could start. The confirmed
		// effect is preserved; nothing is rolled back and nothing else runs.
		result.Rollup.IntegrationResultTree = applied.ResultTree
		final.ResultReference = IntegrationResultReference(applied.ResultTree)
		final.Status, final.CancellationSeen = AttemptCancelled, true
		executeErr = fmt.Errorf("%w: %w", ErrIntegrationFailed, attemptCtx.Err())
	} else {
		validations, err := s.validator.ValidateCombined(attemptCtx, integration, applied.ResultTree)
		result.Rollup.IntegrationResultTree = applied.ResultTree
		result.Rollup.ValidationResults = validations
		final.ResultReference = IntegrationResultReference(applied.ResultTree)
		if wire, marshalErr := json.Marshal(validations); marshalErr == nil {
			outputDigest := sha256.Sum256(wire)
			final.OutputDigest = hex.EncodeToString(outputDigest[:])
		}
		switch {
		case attemptCtx.Err() != nil:
			// Validation commands were interrupted and the workspace could not be
			// re-inspected, so their stop and side effects are unobservable. The
			// confirmed integration stays referenced; the attempt stays unknown
			// and blocks new attempts until reconciliation.
			final.Status, final.AmbiguousEffect, final.CancellationSeen = AttemptUnknown, true, true
			executeErr = fmt.Errorf("%w: %w", ErrIntegrationFailed, attemptCtx.Err())
		case err != nil || !validValidationResults(validations):
			final.Status = AttemptFailed
			executeErr = ErrIntegrationFailed
		default:
			final.Status = AttemptSucceeded
			status = "success"
		}
	}
	finished := s.now().UTC()
	if finished.Before(started) {
		finished = started
	}
	final.FinishedAt = &finished
	// The terminal outcome is recorded even after cancellation; cancellation
	// stops the attempt's effects, not the record of what happened.
	saved, err := s.persistIntegrationAttempt(context.WithoutCancel(ctx), persisted, index, final)
	if err != nil {
		// Effects may be confirmed, but the canonical attempt is still running.
		result.Rollup.Status = "recovery_required"
		return result, err
	}
	rollup := rollupGraph(saved)
	rollup.Status = status
	rollup.IntegrationResultTree = result.Rollup.IntegrationResultTree
	rollup.ValidationResults = result.Rollup.ValidationResults
	rollup.References = result.Rollup.References
	return IntegrationResult{Graph: saved, Rollup: rollup}, executeErr
}

// persistIntegrationAttempt appends or replaces the integration child's last
// attempt on a copy of graph and requires the store to confirm exactly it.
func (s IntegrationService) persistIntegrationAttempt(ctx context.Context, graph Graph, index int, attempt Attempt) (Graph, error) {
	next := graph
	next.Children = append([]ChildExecution(nil), graph.Children...)
	attempts := append([]Attempt(nil), graph.Children[index].Attempts...)
	if len(attempts) > 0 && attempts[len(attempts)-1].AttemptID == attempt.AttemptID {
		attempts[len(attempts)-1] = attempt
	} else {
		attempts = append(attempts, attempt)
	}
	next.Children[index].Attempts = attempts
	if !ValidGraph(next) {
		return graph, fmt.Errorf("%w: %v", ErrIntegrationRecoveryRequired, ErrInvalidGraph)
	}
	persisted, err := s.store.Save(ctx, next)
	if err != nil {
		return graph, fmt.Errorf("%w: %v", ErrIntegrationRecoveryRequired, err)
	}
	if !ValidGraph(persisted) || persisted.Parent.ExecutionID != graph.Parent.ExecutionID || len(persisted.Children) != len(next.Children) || persisted.Children[index].ExecutionID != next.Children[index].ExecutionID {
		return graph, ErrIntegrationRecoveryRequired
	}
	confirmed := persisted.Children[index].Attempts
	if len(confirmed) != len(attempts) || confirmed[len(confirmed)-1].AttemptID != attempt.AttemptID || confirmed[len(confirmed)-1].Status != attempt.Status {
		return graph, ErrIntegrationRecoveryRequired
	}
	return persisted, nil
}

// IntegrationResultReference is the attempt result reference that binds a
// successful Integration/Reconciliation attempt to its result tree.
func IntegrationResultReference(resultTree string) string {
	return "integration-result-tree:" + resultTree
}

// integrationAttemptAvailable reports whether the integration child may start a
// new attempt: none is active or unreconciled, none succeeded, and the explicit
// maximum is not exhausted.
func integrationAttemptAvailable(child ChildExecution) bool {
	if uint32(len(child.Attempts)) >= child.Envelope.Controls.MaximumAttempts {
		return false
	}
	if len(child.Attempts) == 0 {
		return true
	}
	last := child.Attempts[len(child.Attempts)-1]
	return last.Status != AttemptRunning && last.Status != AttemptUnknown && last.Status != AttemptSucceeded && !last.AmbiguousEffect
}

func validChildResult(graph Graph, result ChildResult) bool {
	if result.ParentID != graph.Parent.ExecutionID || result.GraphRevision != graph.Parent.GraphRevision || !validOpaqueID(result.ChildID) || !validOpaqueID(result.AttemptID) || !validDigest(result.EnvelopeDigest) || !validSourceRevision(result.BaseRevision) || !validSourceRevision(result.ResultTree) || len(result.Artifacts) > maxListItems {
		return false
	}
	for _, child := range graph.Children {
		if child.ExecutionID != result.ChildID || child.Envelope.IntegrationOwner {
			continue
		}
		if EnvelopeDigest(child.Envelope) != result.EnvelopeDigest || len(child.Attempts) == 0 {
			return false
		}
		attempt := child.Attempts[len(child.Attempts)-1]
		if attempt.AttemptID != result.AttemptID || attempt.Status != AttemptSucceeded {
			return false
		}
		for _, artifact := range result.Artifacts {
			if !validToken(artifact.ID) || !validDigest(artifact.Digest) {
				return false
			}
		}
		return true
	}
	return false
}

func EnvelopeDigest(envelope ChildEnvelope) string {
	wire, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:])
}

func integrationPreviewDigest(preview IntegrationPreview) (string, error) {
	preview.Digest = ""
	wire, err := json.Marshal(preview)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func integrationChild(graph Graph) (ChildExecution, bool) {
	index, ok := integrationChildIndex(graph)
	if !ok {
		return ChildExecution{}, false
	}
	return graph.Children[index], true
}

func integrationChildIndex(graph Graph) (int, bool) {
	for index, child := range graph.Children {
		if child.ExecutionID == graph.Parent.IntegrationChild && child.Envelope.IntegrationOwner {
			return index, true
		}
	}
	return 0, false
}

func rollupGraph(graph Graph) ParentRollup {
	rollup := ParentRollup{Status: "partial", ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, IntegrationChildID: graph.Parent.IntegrationChild}
	for _, child := range graph.Children {
		status := AttemptNotStarted
		if len(child.Attempts) > 0 {
			status = child.Attempts[len(child.Attempts)-1].Status
		}
		rollup.ChildOutcomes = append(rollup.ChildOutcomes, ChildOutcome{ChildID: child.ExecutionID, Status: status, Required: !child.Envelope.Optional})
	}
	sort.Slice(rollup.ChildOutcomes, func(i, j int) bool { return rollup.ChildOutcomes[i].ChildID < rollup.ChildOutcomes[j].ChildID })
	return rollup
}

func validValidationResults(results []ValidationResult) bool {
	if len(results) == 0 || len(results) > maxListItems {
		return false
	}
	for _, result := range results {
		if !validTarget(result.CommandReference) || result.ExitCode != 0 || !validDigest(result.OutputDigest) {
			return false
		}
	}
	return true
}
