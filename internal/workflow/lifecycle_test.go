package workflow

import (
	"context"
	"testing"
	"time"
)

const testDigest = "0000000000000000000000000000000000000000000000000000000000000000"

func TestLifecycleCompleteGateFactMatrix(t *testing.T) {
	state := lifecycleTestState()
	assertLifecycle(t, state, LifecycleIntake)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecycleSpecifying)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecycleSpecifying)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecycleSpecified)
	addLifecycleFact(&state, FactPlanningAuthority, true, "decision")
	assertLifecycle(t, state, LifecyclePlanning)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecyclePlanning)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecyclePlanned)
	addLifecycleFact(&state, FactImplementationAuthority, true, "plan")
	assertLifecycle(t, state, LifecycleImplementing)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecycleImplemented)
	addLifecycleFact(&state, FactReviewStarted, true, "evidence")
	assertLifecycle(t, state, LifecycleReviewing)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecycleReviewing)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecycleReviewing)
	advanceLifecycleState(&state)
	assertLifecycle(t, state, LifecycleReviewed)
	completeLifecycleState(&state)
	assertLifecycle(t, state, LifecycleReviewed)
	addLifecycleFact(&state, FactHumanAcceptance, true, "evidence")
	assertLifecycle(t, state, LifecycleAccepted)
}

func TestRecordFactAuthorityAndBlockedTransitionHaveZeroUnauthorizedEffects(t *testing.T) {
	service, store, provider := newS4Service(t)
	service.references = acceptingLifecycleReferences{}
	target := s4Target()
	service.Start(context.Background(), target)
	for _, gate := range []Stage{Intake, Specification, Clarification} {
		state := store.state
		result := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: state.Revision, Stage: gate, Outcome: OutcomePassed})
		if result.Status != Succeeded {
			t.Fatalf("advance %s = %#v", gate, result)
		}
	}
	reference := Reference{Kind: "specification", ID: "spec", Digest: testDigest}
	denied := service.RecordLifecycleFact(context.Background(), target, LifecycleFactInput{ExpectedRevision: store.state.Revision, Kind: FactPlanningAuthority, Active: true, Reference: reference}, false)
	if denied.Status != Denied || store.state.Revision != 4 {
		t.Fatalf("denied fact = %#v state=%#v", denied, store.state)
	}
	authorized := service.RecordLifecycleFact(context.Background(), target, LifecycleFactInput{ExpectedRevision: 4, Kind: FactPlanningAuthority, Active: true, Reference: reference}, true)
	if authorized.Status != Succeeded || store.state.Revision != 5 {
		t.Fatalf("authorized fact = %#v", authorized)
	}
	blocked := service.RecordLifecycleFact(context.Background(), target, LifecycleFactInput{ExpectedRevision: 5, Kind: FactBlocked, Active: true, Reference: Reference{Kind: "evidence", ID: "blocker", Digest: testDigest}}, true)
	if blocked.Status != Succeeded || store.state.Revision != 6 {
		t.Fatalf("blocked fact = %#v", blocked)
	}
	transition := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 6, Stage: Plan, Outcome: OutcomePassed})
	if transition.Status != Denied || transition.Category != "workflow_blocked" || store.state.Revision != 6 || len(provider.effects) != 0 {
		t.Fatalf("blocked transition = %#v state=%#v effects=%v", transition, store.state, provider.effects)
	}
}

func TestLifecycleFlagCombinationMatrix(t *testing.T) {
	for mask := 0; mask < 16; mask++ {
		conditions := LifecycleConditions{Blocked: mask&1 != 0, NeedsDecision: mask&2 != 0, NeedsApproval: mask&4 != 0, RecoveryRequired: mask&8 != 0}
		flags := lifecycleFlags(conditions)
		if contains(flags, FlagBlocked) != conditions.Blocked || contains(flags, FlagNeedsDecision) != conditions.NeedsDecision || contains(flags, FlagNeedsApproval) != conditions.NeedsApproval || contains(flags, FlagRecoveryRequired) != conditions.RecoveryRequired {
			t.Fatalf("mask %d flags=%v", mask, flags)
		}
	}
}

func TestProjectionUsesDerivedStageAndIndependentFlags(t *testing.T) {
	service, store, provider := newS4Service(t)
	service.references = acceptingLifecycleReferences{}
	target := s4Target()
	service.Start(context.Background(), target)
	service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	service.RecordLifecycleFact(context.Background(), target, LifecycleFactInput{ExpectedRevision: 2, Kind: FactBlocked, Active: true, Reference: Reference{Kind: "evidence", ID: "blocker", Digest: testDigest}}, true)
	previewed := service.PrepareProjection(context.Background(), target, store.state.Revision)
	if previewed.Status != Succeeded || previewed.Preview == nil || previewed.Preview.LifecycleStage != LifecycleSpecifying || previewed.Preview.Label != "axiom:stage:specifying" {
		t.Fatalf("preview = %#v", previewed)
	}
	if !previewHasEffect(previewed.Preview.Effects, AddStageLabel, FlagBlocked) || !previewHasEffect(previewed.Preview.Effects, AddStageLabel, "axiom:stage:specifying") {
		t.Fatalf("flag effects = %#v", previewed.Preview.Effects)
	}
	if len(provider.effects) != 0 {
		t.Fatalf("preview mutated provider: %v", provider.effects)
	}
}

func previewHasEffect(effects []ProjectionEffect, kind ProjectionEffectKind, value string) bool {
	for _, effect := range effects {
		if effect.Kind == kind && effect.Value == value {
			return true
		}
	}
	return false
}

type acceptingLifecycleReferences struct{}

func (acceptingLifecycleReferences) Validate(context.Context, string, string, Reference) error {
	return nil
}

func TestLifecycleBlockedIsOrthogonalAndInconsistencyFailsClosed(t *testing.T) {
	state := lifecycleTestState()
	advanceLifecycleState(&state)
	addLifecycleFact(&state, FactBlocked, true, "evidence")
	derived, err := DeriveLifecycle(state)
	if err != nil || derived.Stage != LifecycleSpecifying || !derived.Conditions.Blocked {
		t.Fatalf("blocked lifecycle = %#v err=%v", derived, err)
	}

	bad := cloneState(state)
	bad.Transitions[len(bad.Transitions)-1].Fact.ScopeDigest = testDigest
	if _, err := DeriveLifecycle(bad); err == nil {
		t.Fatal("scope-mismatched fact accepted")
	}

	tasks := lifecycleTestState()
	for tasks.Stage != Tasks {
		advanceLifecycleState(&tasks)
	}
	if _, err := DeriveLifecycle(tasks); err == nil {
		t.Fatal("tasks without planning authority accepted")
	}
}

func TestLifecycleRejectsStaleDuplicateMissingReferenceAndSkippedHistory(t *testing.T) {
	service, store, _ := newS4Service(t)
	service.references = acceptingLifecycleReferences{}
	target := s4Target()
	service.Start(context.Background(), target)
	stale := service.RecordLifecycleFact(context.Background(), target, LifecycleFactInput{ExpectedRevision: 2, Kind: FactBlocked, Active: true, Reference: Reference{Kind: "evidence", ID: "blocker", Digest: testDigest}}, true)
	if stale.Status != Denied || store.state.Revision != 1 {
		t.Fatalf("stale fact = %#v state=%#v", stale, store.state)
	}

	state := lifecycleTestState()
	for state.Stage != Plan {
		advanceLifecycleState(&state)
	}
	addLifecycleFact(&state, FactPlanningAuthority, true, "specification")
	addLifecycleFact(&state, FactPlanningAuthority, true, "decision")
	if _, err := DeriveLifecycle(state); err == nil {
		t.Fatal("duplicate authority fact accepted")
	}

	missingReference := lifecycleTestState()
	for missingReference.Stage != Plan {
		advanceLifecycleState(&missingReference)
	}
	for index := range missingReference.Transitions {
		missingReference.Transitions[index].References = nil
	}
	addLifecycleFact(&missingReference, FactPlanningAuthority, true, "evidence")
	if _, err := DeriveLifecycle(missingReference); err == nil {
		t.Fatal("reference-missing authority fact accepted")
	}

	skipped := lifecycleTestState()
	skipped.Stage = Tasks
	if _, err := DeriveLifecycle(skipped); err == nil {
		t.Fatal("skipped canonical history accepted")
	}
}

func TestBlockedConditionIsValidAtEveryCanonicalStage(t *testing.T) {
	state := lifecycleTestState()
	for {
		candidate := cloneState(state)
		addLifecycleFact(&candidate, FactBlocked, true, "evidence")
		derived, err := DeriveLifecycle(candidate)
		if err != nil || !derived.Conditions.Blocked {
			t.Fatalf("blocked at %s = %#v err=%v", state.Stage, derived, err)
		}
		if state.Stage == Completion {
			break
		}
		if state.Stage == Plan {
			addLifecycleFact(&state, FactPlanningAuthority, true, "specification")
		}
		if state.Stage == Implementation {
			addLifecycleFact(&state, FactImplementationAuthority, true, "plan")
		}
		if state.Stage == Review {
			addLifecycleFact(&state, FactReviewStarted, true, "evidence")
		}
		advanceLifecycleState(&state)
	}
}

func TestLifecycleLabelsAreExactAndExternalSignalsGrantNothing(t *testing.T) {
	want := []string{
		"axiom:stage:intake", "axiom:stage:specifying", "axiom:stage:specified",
		"axiom:stage:planning", "axiom:stage:planned", "axiom:stage:implementing",
		"axiom:stage:implemented", "axiom:stage:reviewing", "axiom:stage:reviewed",
		"axiom:stage:accepted",
	}
	for index, stage := range LifecycleStages() {
		if got := lifecycleLabel(stage); got != want[index] {
			t.Fatalf("stage %d label = %q", index, got)
		}
	}
	state := lifecycleTestState()
	baseline, err := DeriveLifecycle(state)
	if err != nil {
		t.Fatal(err)
	}
	_ = ProjectionObservation{IssueState: "CLOSED", IssueLabels: []string{"axiom:stage:accepted"}}
	after, err := DeriveLifecycle(state)
	if err != nil || after != baseline {
		t.Fatalf("external signals changed lifecycle: before=%#v after=%#v err=%v", baseline, after, err)
	}
}

func TestLifecycleProjectionRejectsZeroMultipleAndUnknownStages(t *testing.T) {
	for name, labels := range map[string][]string{
		"zero":     {"external"},
		"multiple": {"axiom:stage:intake", "axiom:stage:specifying"},
		"unknown":  {"axiom:stage:invented"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := observedLifecycleStage(labels); err == nil {
				t.Fatal("drift accepted")
			}
		})
	}
}

func TestLegacyS4ProjectionRecordRemainsReadableWithoutRewrite(t *testing.T) {
	state := lifecycleTestState()
	advanceLifecycleState(&state)
	key := projectionKey(state.ExecutionID, state.Revision)
	legacyLabel := "axiom:stage:specification"
	state.Projections = []ProjectionRecord{{
		ExecutionRevision: state.Revision,
		Key:               key,
		Intended: []ProjectionEffect{
			{Kind: AddStageLabel, Value: legacyLabel},
			{Kind: PostTransitionComment, Value: legacyTransitionComment(state, key)},
		},
	}}
	if !ValidState(state) {
		t.Fatal("valid historical S4 projection record became unreadable")
	}
	if got := state.Projections[0].Intended[0].Value; got != legacyLabel {
		t.Fatalf("historical label rewritten to %q", got)
	}
}

func lifecycleTestState() State {
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	return State{
		ExecutionID: "018f4a44-7c31-7dd4-9d00-111111111111", FormatVersion: FormatVersion, WorkflowVersion: WorkflowVersion,
		ProjectID: "123e4567-e89b-42d3-a456-426614174000", RepositoryKey: "main",
		WorkItem:  WorkItem{Provider: "github", Resource: "owner/repo", ExternalID: "94", URL: "https://github.com/owner/repo/issues/94", State: "OPEN"},
		RuntimeID: "codex", Stage: Intake, Revision: 1, Status: ExecutionActive, CreatedAt: at, UpdatedAt: at,
		Provenance: Identity{Product: "Axiom", Version: "development", Revision: "abc123", SourceState: "clean"},
	}
}

func advanceLifecycleState(state *State) {
	next := stages[stageIndex(state.Stage)+1]
	at := state.UpdatedAt.Add(time.Second)
	event := Transition{Revision: state.Revision + 1, From: state.Stage, To: next, Outcome: OutcomePassed, RequestDigest: digest(state.Revision), References: lifecycleReferences(), CommittedAt: at, Provenance: state.Provenance}
	state.Revision++
	state.Stage, state.UpdatedAt = next, at
	state.Transitions = append(state.Transitions, event)
}

func completeLifecycleState(state *State) {
	at := state.UpdatedAt.Add(time.Second)
	event := Transition{Revision: state.Revision + 1, From: Completion, To: Completion, Outcome: OutcomePassed, RequestDigest: digest(state.Revision), References: lifecycleReferences(), CommittedAt: at, Provenance: state.Provenance}
	state.Revision++
	state.Status, state.UpdatedAt = ExecutionCompleted, at
	state.Terminal = &Terminal{Status: Succeeded, ConfirmedEffects: []string{"local_execution_transition"}}
	state.Transitions = append(state.Transitions, event)
}

func addLifecycleFact(state *State, kind LifecycleFactKind, active bool, referenceKind string) {
	at := state.UpdatedAt.Add(time.Second)
	reference := Reference{Kind: referenceKind, ID: "ref", Digest: testDigest}
	fact := LifecycleFact{Kind: kind, Active: active, ScopeDigest: executionScopeDigest(*state), Reference: reference}
	event := Transition{Revision: state.Revision + 1, From: state.Stage, To: state.Stage, Outcome: OutcomeFact, RequestDigest: digest(fact), References: []Reference{reference}, CommittedAt: at, Provenance: state.Provenance, Fact: &fact}
	state.Revision++
	state.UpdatedAt = at
	state.Transitions = append(state.Transitions, event)
}

func lifecycleReferences() []Reference {
	result := []Reference{}
	for _, kind := range []string{"specification", "decision", "plan", "tasks", "artifact", "evidence", "pull_request"} {
		result = append(result, Reference{Kind: kind, ID: kind, Digest: testDigest})
	}
	return result
}

func assertLifecycle(t *testing.T, state State, want WorkItemLifecycleStage) {
	t.Helper()
	got, err := DeriveLifecycle(state)
	if err != nil || got.Stage != want {
		t.Fatalf("lifecycle at %s = %#v err=%v want=%s", state.Stage, got, err, want)
	}
}
