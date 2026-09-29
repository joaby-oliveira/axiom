package workflow

import (
	"crypto/sha256"
	"encoding/hex"
)

// WorkItemLifecycleStage is a read-only summary of canonical Execution truth.
// It is deliberately absent from State and from the persisted execution DTO.
type WorkItemLifecycleStage string

const (
	LifecycleIntake       WorkItemLifecycleStage = "intake"
	LifecycleSpecifying   WorkItemLifecycleStage = "specifying"
	LifecycleSpecified    WorkItemLifecycleStage = "specified"
	LifecyclePlanning     WorkItemLifecycleStage = "planning"
	LifecyclePlanned      WorkItemLifecycleStage = "planned"
	LifecycleImplementing WorkItemLifecycleStage = "implementing"
	LifecycleImplemented  WorkItemLifecycleStage = "implemented"
	LifecycleReviewing    WorkItemLifecycleStage = "reviewing"
	LifecycleReviewed     WorkItemLifecycleStage = "reviewed"
	LifecycleAccepted     WorkItemLifecycleStage = "accepted"
)

var lifecycleStages = [...]WorkItemLifecycleStage{
	LifecycleIntake, LifecycleSpecifying, LifecycleSpecified, LifecyclePlanning,
	LifecyclePlanned, LifecycleImplementing, LifecycleImplemented,
	LifecycleReviewing, LifecycleReviewed, LifecycleAccepted,
}

func LifecycleStages() []WorkItemLifecycleStage {
	result := make([]WorkItemLifecycleStage, len(lifecycleStages))
	copy(result, lifecycleStages[:])
	return result
}

func (s WorkItemLifecycleStage) Valid() bool {
	for _, candidate := range lifecycleStages {
		if s == candidate {
			return true
		}
	}
	return false
}

type LifecycleFactKind string

const (
	FactPlanningAuthority       LifecycleFactKind = "planning_authority"
	FactImplementationAuthority LifecycleFactKind = "implementation_authority"
	FactReviewStarted           LifecycleFactKind = "review_started"
	FactHumanAcceptance         LifecycleFactKind = "human_acceptance"
	FactBlocked                 LifecycleFactKind = "blocked"
	FactNeedsDecision           LifecycleFactKind = "needs_decision"
	FactNeedsApproval           LifecycleFactKind = "needs_approval"
)

type LifecycleFact struct {
	Kind        LifecycleFactKind `json:"kind"`
	Active      bool              `json:"active"`
	ScopeDigest string            `json:"scopeDigest"`
	Reference   Reference         `json:"reference"`
}

type LifecycleConditions struct {
	Blocked, NeedsDecision, NeedsApproval, RecoveryRequired bool
}

type LifecycleProjection struct {
	Stage      WorkItemLifecycleStage
	Conditions LifecycleConditions
}

// DeriveLifecycle validates all revisioned facts before selecting one stage.
// Any invalid fact/history combination returns recovery_required and no stage.
func DeriveLifecycle(state State) (LifecycleProjection, error) {
	if !ValidState(state) {
		return LifecycleProjection{}, ErrRecoveryRequired
	}
	facts, conditions, ok := lifecycleFacts(state)
	if !ok || !validLifecycleReferences(state) {
		return LifecycleProjection{}, ErrRecoveryRequired
	}
	has := func(kind LifecycleFactKind) bool { return facts[kind] }

	var stage WorkItemLifecycleStage
	switch state.Stage {
	case Intake:
		stage = LifecycleIntake
	case Specification, Clarification:
		stage = LifecycleSpecifying
	case Plan:
		stage = LifecycleSpecified
		if has(FactPlanningAuthority) {
			stage = LifecyclePlanning
		}
	case Tasks:
		if !has(FactPlanningAuthority) {
			return LifecycleProjection{}, ErrRecoveryRequired
		}
		stage = LifecyclePlanning
	case Implementation:
		if !has(FactPlanningAuthority) {
			return LifecycleProjection{}, ErrRecoveryRequired
		}
		stage = LifecyclePlanned
		if has(FactImplementationAuthority) {
			stage = LifecycleImplementing
		}
	case Review:
		if !has(FactPlanningAuthority) || !has(FactImplementationAuthority) {
			return LifecycleProjection{}, ErrRecoveryRequired
		}
		stage = LifecycleImplemented
		if has(FactReviewStarted) {
			stage = LifecycleReviewing
		}
	case Evidence, Reconciliation:
		if !has(FactPlanningAuthority) || !has(FactImplementationAuthority) || !has(FactReviewStarted) {
			return LifecycleProjection{}, ErrRecoveryRequired
		}
		stage = LifecycleReviewing
	case Completion:
		if !has(FactPlanningAuthority) || !has(FactImplementationAuthority) || !has(FactReviewStarted) {
			return LifecycleProjection{}, ErrRecoveryRequired
		}
		stage = LifecycleReviewed
		if has(FactHumanAcceptance) {
			if state.Status != ExecutionCompleted || state.Terminal == nil {
				return LifecycleProjection{}, ErrRecoveryRequired
			}
			stage = LifecycleAccepted
		}
	default:
		return LifecycleProjection{}, ErrRecoveryRequired
	}
	return LifecycleProjection{Stage: stage, Conditions: conditions}, nil
}

func lifecycleFacts(state State) (map[LifecycleFactKind]bool, LifecycleConditions, bool) {
	facts := make(map[LifecycleFactKind]bool)
	conditions := LifecycleConditions{}
	scope := executionScopeDigest(state)
	for _, event := range state.Transitions {
		if event.Fact == nil {
			continue
		}
		fact := *event.Fact
		if fact.ScopeDigest != scope || !validReference(fact.Reference) || !factAllowedAt(fact.Kind, event.From) {
			return nil, LifecycleConditions{}, false
		}
		switch fact.Kind {
		case FactPlanningAuthority, FactImplementationAuthority, FactReviewStarted, FactHumanAcceptance:
			if !fact.Active || facts[fact.Kind] {
				return nil, LifecycleConditions{}, false
			}
			facts[fact.Kind] = true
		case FactBlocked:
			conditions.Blocked = fact.Active
		case FactNeedsDecision:
			conditions.NeedsDecision = fact.Active
		case FactNeedsApproval:
			conditions.NeedsApproval = fact.Active
		default:
			return nil, LifecycleConditions{}, false
		}
	}
	return facts, conditions, true
}

func factAllowedAt(kind LifecycleFactKind, stage Stage) bool {
	switch kind {
	case FactPlanningAuthority:
		return stageIndex(stage) >= stageIndex(Plan)
	case FactImplementationAuthority:
		return stageIndex(stage) >= stageIndex(Implementation)
	case FactReviewStarted:
		return stageIndex(stage) >= stageIndex(Review)
	case FactHumanAcceptance:
		return stage == Completion
	case FactBlocked, FactNeedsDecision, FactNeedsApproval:
		return stage.Valid()
	default:
		return false
	}
}

func validLifecycleReferences(state State) bool {
	// Older valid records remain readable. References become mandatory only once
	// the corresponding amended lifecycle boundary is claimed by a fact.
	for _, event := range state.Transitions {
		if event.Fact == nil {
			continue
		}
		if event.Fact.Kind == FactPlanningAuthority && !historyHasReference(state, event.Revision, "specification", "decision") {
			return false
		}
		if event.Fact.Kind == FactImplementationAuthority && !historyHasReference(state, event.Revision, "plan", "tasks") {
			return false
		}
		if event.Fact.Kind == FactReviewStarted && !historyHasReference(state, event.Revision, "artifact", "evidence", "pull_request") {
			return false
		}
		if event.Fact.Kind == FactHumanAcceptance && !historyHasReference(state, event.Revision, "evidence") {
			return false
		}
	}
	return true
}

func historyHasReference(state State, before uint64, kinds ...string) bool {
	for _, event := range state.Transitions {
		if event.Revision > before {
			break
		}
		for _, reference := range event.References {
			for _, kind := range kinds {
				if reference.Kind == kind {
					return true
				}
			}
		}
		if event.Fact != nil {
			for _, kind := range kinds {
				if event.Fact.Reference.Kind == kind {
					return true
				}
			}
		}
	}
	return false
}

func executionScopeDigest(state State) string {
	sum := sha256.Sum256([]byte(state.ExecutionID + "\x00" + state.ProjectID + "\x00" + state.RepositoryKey + "\x00" + state.WorkItem.Provider + "\x00" + state.WorkItem.Resource + "\x00" + state.WorkItem.ExternalID))
	return hex.EncodeToString(sum[:])
}

func lifecycleLabel(stage WorkItemLifecycleStage) string { return stagePrefix + string(stage) }

const (
	FlagBlocked          = "axiom:blocked"
	FlagNeedsDecision    = "axiom:needs-decision"
	FlagNeedsApproval    = "axiom:needs-approval"
	FlagRecoveryRequired = "axiom:recovery-required"
)

func lifecycleFlags(conditions LifecycleConditions) []string {
	var flags []string
	if conditions.Blocked {
		flags = append(flags, FlagBlocked)
	}
	if conditions.NeedsDecision {
		flags = append(flags, FlagNeedsDecision)
	}
	if conditions.NeedsApproval {
		flags = append(flags, FlagNeedsApproval)
	}
	if conditions.RecoveryRequired {
		flags = append(flags, FlagRecoveryRequired)
	}
	return flags
}

func validLifecycleFlag(value string) bool {
	return value == FlagBlocked || value == FlagNeedsDecision || value == FlagNeedsApproval || value == FlagRecoveryRequired
}

func ValidProjectionLabel(value string) bool { return validProjectionLabel(value) }
func ValidDesiredProjectionLabel(value string) bool {
	return validStageEffect(value) || validLifecycleFlag(value)
}
