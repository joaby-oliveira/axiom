package executiongraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

var (
	ErrIntegrationBlocked = errors.New("integration blocked")
	ErrIntegrationStale   = errors.New("integration preview stale")
	ErrIntegrationFailed  = errors.New("integration failed")
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
	TargetRevision, TargetTree   string
	Sources                      []ChildResult
	OptionalWaivers              []OptionalWaiver
	Effects                      []Effect
	Digest                       string
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

type IntegrationService struct {
	integrator Integrator
	validator  CombinedValidator
}

func NewIntegrationService(integrator Integrator, validator CombinedValidator) IntegrationService {
	return IntegrationService{integrator: integrator, validator: validator}
}

func (s IntegrationService) Preview(graph Graph, observation IntegrationObservation, results []ChildResult) (IntegrationPreview, error) {
	if !ValidGraph(graph) || !validDigest(observation.TargetRevision) || !validDigest(observation.TargetTree) || len(observation.ForeignChanges) > 0 || len(observation.Conflicts) > 0 {
		return IntegrationPreview{}, ErrIntegrationBlocked
	}
	integration, ok := integrationChild(graph)
	if !ok {
		return IntegrationPreview{}, ErrIntegrationBlocked
	}
	byChild := make(map[string]ChildResult, len(results))
	for _, result := range results {
		if _, exists := byChild[result.ChildID]; exists || !validChildResult(graph, result) {
			return IntegrationPreview{}, ErrIntegrationBlocked
		}
		byChild[result.ChildID] = result
	}
	preview := IntegrationPreview{ParentID: graph.Parent.ExecutionID, IntegrationChildID: integration.ExecutionID, GraphRevision: graph.Parent.GraphRevision, TargetRevision: observation.TargetRevision, TargetTree: observation.TargetTree}
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

func (s IntegrationService) Execute(ctx context.Context, graph Graph, preview IntegrationPreview, authority IntegrationAuthority) (ParentRollup, error) {
	rollup := rollupGraph(graph)
	if s.integrator == nil || s.validator == nil || authority.PreviewDigest != preview.Digest || authority.TargetRevision != preview.TargetRevision || !sameEffects(authority.Effects, preview.Effects) || !validTarget(authority.Reference) {
		rollup.Status = "recovery_required"
		return rollup, ErrIntegrationStale
	}
	digest, err := integrationPreviewDigest(preview)
	if err != nil || digest != preview.Digest || preview.ParentID != graph.Parent.ExecutionID || preview.GraphRevision != graph.Parent.GraphRevision || preview.IntegrationChildID != graph.Parent.IntegrationChild {
		rollup.Status = "recovery_required"
		return rollup, ErrIntegrationStale
	}
	integration, ok := integrationChild(graph)
	if !ok {
		rollup.Status = "failure"
		return rollup, ErrIntegrationBlocked
	}
	applied, err := s.integrator.Apply(ctx, integration, preview)
	if err != nil || !applied.Confirmed || !validDigest(applied.ResultTree) || !sameEffects(applied.AppliedEffects, preview.Effects) {
		rollup.Status = "partial"
		setIntegrationOutcome(&rollup, AttemptUnknown)
		rollup.References = append(rollup.References, applied.References...)
		return rollup, ErrIntegrationFailed
	}
	validations, err := s.validator.ValidateCombined(ctx, integration, applied.ResultTree)
	rollup.IntegrationResultTree = applied.ResultTree
	rollup.References = append(rollup.References, applied.References...)
	rollup.ValidationResults = validations
	if err != nil || !validValidationResults(validations) {
		rollup.Status = "partial"
		setIntegrationOutcome(&rollup, AttemptFailed)
		return rollup, ErrIntegrationFailed
	}
	rollup.Status = "success"
	setIntegrationOutcome(&rollup, AttemptSucceeded)
	return rollup, nil
}

func validChildResult(graph Graph, result ChildResult) bool {
	if result.ParentID != graph.Parent.ExecutionID || result.GraphRevision != graph.Parent.GraphRevision || !validOpaqueID(result.ChildID) || !validOpaqueID(result.AttemptID) || !validDigest(result.EnvelopeDigest) || !validDigest(result.BaseRevision) || !validDigest(result.ResultTree) || len(result.Artifacts) > maxListItems {
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
	for _, child := range graph.Children {
		if child.ExecutionID == graph.Parent.IntegrationChild && child.Envelope.IntegrationOwner {
			return child, true
		}
	}
	return ChildExecution{}, false
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

func setIntegrationOutcome(rollup *ParentRollup, status AttemptStatus) {
	for index := range rollup.ChildOutcomes {
		if rollup.ChildOutcomes[index].ChildID == rollup.IntegrationChildID {
			rollup.ChildOutcomes[index].Status = status
			return
		}
	}
}
