package workflow

import "crypto/sha256"

type ReconciliationKind string

const (
	ReconciliationCurrentTruth ReconciliationKind = "current_truth"
	ReconciliationRecoveryPlan ReconciliationKind = "local_generation_recovery_plan"
	ReconciliationRequired     ReconciliationKind = "recovery_required"
)

type LocalGeneration struct {
	Name      string
	State     State
	Digest    string
	Validated bool
}

type ReconciliationInput struct {
	Current             *State
	Generations         []LocalGeneration
	Provider            *ProjectionObservation
	RepositoryArtifacts []Reference
	ArtifactsChanged    bool
}

type RecoveryEffect struct {
	GenerationName, Digest string
}

type ReconciliationResult struct {
	Kind       ReconciliationKind
	Lifecycle  *LifecycleProjection
	Projection *ProjectionPreview
	Recovery   *RecoveryEffect
	Reason     string
	NeedsHuman bool
}

// InspectReconciliation is pure and read-only. Provider and Repository evidence
// can corroborate or contradict local truth, never synthesize it.
func InspectReconciliation(input ReconciliationInput) ReconciliationResult {
	if input.ArtifactsChanged || !validRepositoryArtifacts(input.RepositoryArtifacts) {
		return recoveryRequired("repository_artifacts_changed")
	}
	if input.Current != nil {
		lifecycle, err := DeriveLifecycle(*input.Current)
		if err != nil {
			return recoveryRequired("invalid_current_local_truth")
		}
		if !referencesBelongToState(input.RepositoryArtifacts, *input.Current) {
			return recoveryRequired("repository_reference_mismatch")
		}
		if input.Provider != nil {
			preview, err := prepareProjectionPreview(*input.Current, lifecycle, *input.Provider)
			if err != nil {
				return recoveryRequired("provider_projection_drift")
			}
			return ReconciliationResult{Kind: ReconciliationCurrentTruth, Lifecycle: &lifecycle, Projection: &preview}
		}
		return ReconciliationResult{Kind: ReconciliationCurrentTruth, Lifecycle: &lifecycle}
	}

	var candidate *LocalGeneration
	for index := range input.Generations {
		generation := &input.Generations[index]
		if !validGeneration(*generation) {
			continue
		}
		if candidate != nil {
			return recoveryRequired("ambiguous_local_generations")
		}
		candidate = generation
	}
	if candidate == nil {
		return recoveryRequired("missing_local_execution")
	}
	if input.Provider != nil {
		lifecycle, err := DeriveLifecycle(candidate.State)
		if err != nil || !validProjectionObservation(candidate.State.WorkItem, *input.Provider) {
			return recoveryRequired("contradictory_recovery_history")
		}
		observed, legacy, err := observedLifecycleStage(input.Provider.IssueLabels)
		if err != nil || legacy || observed != lifecycleLabel(lifecycle.Stage) || !providerFlagsAligned(input.Provider.IssueLabels, lifecycle.Conditions) {
			return recoveryRequired("contradictory_recovery_history")
		}
	}
	if !referencesBelongToState(input.RepositoryArtifacts, candidate.State) {
		return recoveryRequired("repository_reference_mismatch")
	}
	return ReconciliationResult{
		Kind:     ReconciliationRecoveryPlan,
		Recovery: &RecoveryEffect{GenerationName: candidate.Name, Digest: candidate.Digest},
		Reason:   "fresh_exact_recovery_authority_required",
	}
}

func providerFlagsAligned(labels []string, conditions LifecycleConditions) bool {
	desired := lifecycleFlags(conditions)
	for _, flag := range []string{FlagBlocked, FlagNeedsDecision, FlagNeedsApproval, FlagRecoveryRequired} {
		if contains(labels, flag) != contains(desired, flag) {
			return false
		}
	}
	return true
}

type RecoveryAuthority struct {
	GenerationName, Digest string
}

func (result ReconciliationResult) Authorizes(authority RecoveryAuthority) bool {
	return result.Kind == ReconciliationRecoveryPlan && result.Recovery != nil &&
		authority.GenerationName == result.Recovery.GenerationName && authority.Digest == result.Recovery.Digest
}

func validGeneration(generation LocalGeneration) bool {
	if !generation.Validated || generation.Name == "" || !validDigest(generation.Digest) || !ValidState(generation.State) {
		return false
	}
	if _, err := DeriveLifecycle(generation.State); err != nil {
		return false
	}
	wire := cloneState(generation.State)
	wire.StorageRevision = [sha256.Size]byte{}
	return generation.Digest == digest(wire)
}

func validRepositoryArtifacts(references []Reference) bool {
	if len(references) > maxReferences {
		return false
	}
	for _, reference := range references {
		if !validReference(reference) {
			return false
		}
	}
	return true
}

func referencesBelongToState(references []Reference, state State) bool {
	for _, wanted := range references {
		found := false
		for _, event := range state.Transitions {
			for _, reference := range event.References {
				if reference == wanted {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func recoveryRequired(reason string) ReconciliationResult {
	return ReconciliationResult{Kind: ReconciliationRequired, Reason: reason, NeedsHuman: true}
}

func GenerationDigest(state State) string {
	state.StorageRevision = [sha256.Size]byte{}
	return digest(state)
}
