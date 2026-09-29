package workflow

import "testing"

func TestMissingLocalStateNeverSynthesizesExecution(t *testing.T) {
	provider := ProjectionObservation{Provider: "github", Resource: "owner/repo", IssueExternalID: "94", IssueURL: "https://github.com/owner/repo/issues/94", IssueState: "OPEN", IssueLabels: []string{"axiom:stage:reviewed"}}
	result := InspectReconciliation(ReconciliationInput{Provider: &provider, RepositoryArtifacts: lifecycleReferences()})
	if result.Kind != ReconciliationRequired || !result.NeedsHuman || result.Recovery != nil {
		t.Fatalf("provider-only result = %#v", result)
	}
}

func TestValidatedLocalGenerationProducesExactPlanAndFreshAuthority(t *testing.T) {
	state := lifecycleTestState()
	digest := GenerationDigest(state)
	result := InspectReconciliation(ReconciliationInput{Generations: []LocalGeneration{{Name: "prior-1", State: state, Digest: digest, Validated: true}}})
	if result.Kind != ReconciliationRecoveryPlan || result.Recovery == nil {
		t.Fatalf("recovery result = %#v", result)
	}
	if result.Authorizes(RecoveryAuthority{GenerationName: "prior-1", Digest: testDigest}) {
		t.Fatal("stale recovery authority accepted")
	}
	if !result.Authorizes(RecoveryAuthority{GenerationName: "prior-1", Digest: digest}) {
		t.Fatal("exact recovery authority rejected")
	}
}

func TestCurrentLocalTruthReturnsSharedProjectionPreview(t *testing.T) {
	state := lifecycleTestState()
	advanceLifecycleState(&state)
	provider := alignedReconciliationObservation(state)
	result := InspectReconciliation(ReconciliationInput{Current: &state, Provider: &provider})
	if result.Kind != ReconciliationCurrentTruth || result.Lifecycle == nil || result.Lifecycle.Stage != LifecycleSpecifying || result.Projection == nil || len(result.Projection.Effects) != 0 {
		t.Fatalf("aligned result = %#v", result)
	}

	addLifecycleFact(&state, FactBlocked, true, "evidence")
	provider = alignedReconciliationObservation(state)
	provider.RepositoryLabels = []string{result.Projection.Label}
	provider.IssueLabels = []string{result.Projection.Label}
	result = InspectReconciliation(ReconciliationInput{Current: &state, Provider: &provider})
	if result.Kind != ReconciliationCurrentTruth || result.Projection == nil {
		t.Fatalf("missing flag result = %#v", result)
	}
	assertOnlyProjectionEffects(t, result.Projection.Effects,
		ProjectionEffect{Kind: CreateStageLabel, Value: FlagBlocked},
		ProjectionEffect{Kind: AddStageLabel, Value: FlagBlocked},
	)
}

func TestReconciliationUsesSamePreviewAsProjectionService(t *testing.T) {
	service, _, provider := newS4Service(t)
	target := s4Target()
	service.Start(t.Context(), target)
	transitioned := service.Transition(t.Context(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	projected := service.PrepareProjection(t.Context(), target, transitioned.State.Revision)
	reconciled := InspectReconciliation(ReconciliationInput{Current: &transitioned.State, Provider: &provider.observation})
	if projected.Preview == nil || reconciled.Projection == nil || reconciled.Projection.Digest != projected.Preview.Digest {
		t.Fatalf("projection preview = %#v reconciliation preview = %#v", projected.Preview, reconciled.Projection)
	}
}

func TestCurrentLocalTruthPreviewsRecognizedProjectionDrift(t *testing.T) {
	state := lifecycleTestState()
	advanceLifecycleState(&state)

	for name, mutate := range map[string]func(*ProjectionObservation){
		"obsolete flag": func(provider *ProjectionObservation) {
			provider.IssueLabels = append(provider.IssueLabels, FlagBlocked)
		},
		"legacy stage": func(provider *ProjectionObservation) {
			provider.IssueLabels = []string{"axiom:stage:specification"}
		},
		"missing comment": func(provider *ProjectionObservation) {
			provider.CommentPresent = false
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := alignedReconciliationObservation(state)
			mutate(&provider)
			result := InspectReconciliation(ReconciliationInput{Current: &state, Provider: &provider})
			if result.Kind != ReconciliationCurrentTruth || result.Projection == nil {
				t.Fatalf("result = %#v", result)
			}
			switch name {
			case "obsolete flag":
				assertOnlyProjectionEffects(t, result.Projection.Effects, ProjectionEffect{Kind: RemoveStageLabel, Value: FlagBlocked})
			case "legacy stage":
				assertOnlyProjectionEffects(t, result.Projection.Effects,
					ProjectionEffect{Kind: AddStageLabel, Value: "axiom:stage:specifying"},
					ProjectionEffect{Kind: RemoveStageLabel, Value: "axiom:stage:specification"},
				)
			case "missing comment":
				assertOnlyProjectionEffects(t, result.Projection.Effects, ProjectionEffect{Kind: PostTransitionComment, Value: result.Projection.Comment})
			}
		})
	}
}

func TestZeroUnknownOrMultipleProviderLifecycleLabelsRequireRecovery(t *testing.T) {
	state := lifecycleTestState()
	advanceLifecycleState(&state)
	for name, labels := range map[string][]string{
		"zero":     {"external"},
		"unknown":  {"axiom:stage:invented"},
		"multiple": {"axiom:stage:intake", "axiom:stage:specifying"},
	} {
		t.Run(name, func(t *testing.T) {
			provider := alignedReconciliationObservation(state)
			provider.IssueLabels = labels
			result := InspectReconciliation(ReconciliationInput{Current: &state, Provider: &provider})
			if result.Kind != ReconciliationRequired || result.Projection != nil || !result.NeedsHuman {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestSemanticallyInvalidLocalGenerationRequiresRecovery(t *testing.T) {
	state := lifecycleTestState()
	for state.Stage != Tasks {
		advanceLifecycleState(&state)
	}
	if !ValidState(state) {
		t.Fatal("fixture must remain structurally valid")
	}
	if _, err := DeriveLifecycle(state); err == nil {
		t.Fatal("fixture must be semantically invalid")
	}
	result := InspectReconciliation(ReconciliationInput{Generations: []LocalGeneration{{Name: "prior-1", State: state, Digest: GenerationDigest(state), Validated: true}}})
	if result.Kind != ReconciliationRequired || result.Recovery != nil || !result.NeedsHuman {
		t.Fatalf("result = %#v", result)
	}
}

func TestReconciliationContradictionsStayReadOnlyAndRequireHumanDecision(t *testing.T) {
	state := lifecycleTestState()
	advanceLifecycleState(&state)
	before := digest(state)
	provider := alignedReconciliationObservation(state)
	provider.IssueLabels = []string{"axiom:stage:invented"}
	providerBefore := digest(provider)
	for name, input := range map[string]ReconciliationInput{
		"repository only":   {RepositoryArtifacts: lifecycleReferences()},
		"changed artifacts": {Current: &state, ArtifactsChanged: true},
		"unknown label":     {Current: &state, Provider: &provider},
		"multiple generations": {
			Generations: []LocalGeneration{
				{Name: "prior", State: state, Digest: GenerationDigest(state), Validated: true},
				{Name: "staged", State: state, Digest: GenerationDigest(state), Validated: true},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := InspectReconciliation(input)
			if result.Kind != ReconciliationRequired || !result.NeedsHuman || result.Recovery != nil {
				t.Fatalf("result = %#v", result)
			}
		})
	}
	if after := digest(state); after != before {
		t.Fatalf("inspection mutated state: before=%s after=%s", before, after)
	}
	if after := digest(provider); after != providerBefore {
		t.Fatalf("inspection mutated provider observation: before=%s after=%s", providerBefore, after)
	}
}

func TestReconciliationPreviewInspectionIsReadOnly(t *testing.T) {
	state := lifecycleTestState()
	advanceLifecycleState(&state)
	addLifecycleFact(&state, FactNeedsApproval, true, "evidence")
	addLifecycleFact(&state, FactBlocked, true, "evidence")
	provider := alignedReconciliationObservation(state)
	stateBefore, providerBefore := digest(state), digest(provider)

	result := InspectReconciliation(ReconciliationInput{Current: &state, Provider: &provider})
	if result.Kind != ReconciliationCurrentTruth || result.Projection == nil || len(result.Projection.Effects) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if after := digest(state); after != stateBefore {
		t.Fatalf("inspection mutated state: before=%s after=%s", stateBefore, after)
	}
	if after := digest(provider); after != providerBefore {
		t.Fatalf("inspection mutated provider observation: before=%s after=%s", providerBefore, after)
	}
}

func alignedReconciliationObservation(state State) ProjectionObservation {
	lifecycle, err := DeriveLifecycle(state)
	if err != nil {
		panic(err)
	}
	label := lifecycleLabel(lifecycle.Stage)
	labels := append([]string{label}, lifecycleFlags(lifecycle.Conditions)...)
	return ProjectionObservation{
		Provider:         state.WorkItem.Provider,
		Resource:         state.WorkItem.Resource,
		IssueExternalID:  state.WorkItem.ExternalID,
		IssueURL:         state.WorkItem.URL,
		IssueState:       state.WorkItem.State,
		RepositoryLabels: append([]string(nil), labels...),
		IssueLabels:      append([]string(nil), labels...),
		CommentPresent:   true,
	}
}

func assertOnlyProjectionEffects(t *testing.T, got []ProjectionEffect, want ...ProjectionEffect) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("effects = %#v want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("effects = %#v want %#v", got, want)
		}
	}
}
