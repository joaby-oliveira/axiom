package workflow

import (
	"context"
	"testing"
)

// Regressions for the two Major findings of the S9 dogfood against
// v0.1.2-rc.1: the Execution Runtime was always persisted as codex, and the
// first projection of an Issue without any axiom:stage:* marker required
// recovery.

func TestS9StartPersistsTheExplicitlySelectedRuntime(t *testing.T) {
	for _, runtimeID := range []string{"claude", "codex"} {
		t.Run(runtimeID, func(t *testing.T) {
			service, store, _ := newS4Service(t)
			target := s4Target()
			target.RuntimeID = runtimeID
			started := service.Start(context.Background(), target)
			if started.Category != "execution_started" || started.State.RuntimeID != runtimeID || store.state.RuntimeID != runtimeID {
				t.Fatalf("start = %#v persisted=%q", started, store.state.RuntimeID)
			}
		})
	}
}

func TestS9StartRefusesUnsupportedRuntimeWithoutCreatingExecution(t *testing.T) {
	for name, runtimeID := range map[string]string{"empty": "", "unknown": "gemini", "case": "Claude", "space": " claude"} {
		t.Run(name, func(t *testing.T) {
			service, store, _ := newS4Service(t)
			target := s4Target()
			target.RuntimeID = runtimeID
			refused := service.Start(context.Background(), target)
			if refused.Status != ValidationFailed || refused.Category != "invalid_execution_input" || store.created {
				t.Fatalf("start = %#v created=%v", refused, store.created)
			}
		})
	}
}

func TestS9LaterOperationsKeepThePersistedRuntime(t *testing.T) {
	service, store, provider := newS4Service(t)
	service.references = acceptingLifecycleReferences{}
	start := s4Target()
	start.RuntimeID = "claude"
	service.Start(context.Background(), start)
	// Later operations carry no Runtime selector, exactly as the CLI sends them.
	later := s4Target()
	later.RuntimeID = ""
	provider.observation.RepositoryLabels, provider.observation.IssueLabels = nil, []string{"external"}

	ordered := []struct {
		name string
		run  func() Result
	}{
		{"status", func() Result { return service.Status(context.Background(), later) }},
		{"advance", func() Result {
			return service.Transition(context.Background(), later, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
		}},
		{"reconcile preview", func() Result { return service.PrepareProjection(context.Background(), later, store.state.Revision) }},
		{"reconcile apply", func() Result {
			preview := service.PrepareProjection(context.Background(), later, store.state.Revision)
			if preview.Preview == nil {
				return preview
			}
			return service.Project(context.Background(), later, store.state.Revision, preview.Preview.Digest, true)
		}},
		{"fact", func() Result {
			return service.RecordLifecycleFact(context.Background(), later, LifecycleFactInput{ExpectedRevision: store.state.Revision, Kind: FactNeedsDecision, Active: true, Reference: Reference{Kind: "evidence", ID: "decision", Digest: testDigest}}, true)
		}},
		{"interrupt", func() Result {
			return service.Transition(context.Background(), later, TransitionInput{ExpectedRevision: store.state.Revision, Stage: store.state.Stage, Outcome: OutcomeFailed})
		}},
		{"resume", func() Result { return service.Resume(context.Background(), later, store.state.Revision) }},
	}
	for _, step := range ordered {
		got := step.run()
		if got.Status != Succeeded && !(step.name == "interrupt" && got.Status == Interrupted) || got.State.RuntimeID != "claude" || store.state.RuntimeID != "claude" {
			t.Fatalf("%s = %#v persisted=%q", step.name, got, store.state.RuntimeID)
		}
	}
}

func TestS9ExistingExecutionRuntimeCannotBeReassigned(t *testing.T) {
	for _, pair := range [][2]string{{"claude", "codex"}, {"codex", "claude"}} {
		t.Run(pair[0]+"_to_"+pair[1], func(t *testing.T) {
			service, store, _ := newS4Service(t)
			original := s4Target()
			original.RuntimeID = pair[0]
			started := service.Start(context.Background(), original)
			other := s4Target()
			other.RuntimeID = pair[1]
			restart := service.Start(context.Background(), other)
			status := service.Status(context.Background(), other)
			advance := service.Transition(context.Background(), other, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
			for name, got := range map[string]Result{"start": restart, "status": status, "advance": advance} {
				if got.Status != ValidationFailed || got.Category != "execution_scope_conflict" {
					t.Fatalf("%s = %#v", name, got)
				}
			}
			if store.state.RuntimeID != pair[0] || store.state.Revision != started.State.Revision {
				t.Fatalf("persisted Execution changed: %#v", store.state)
			}
			same := service.Start(context.Background(), original)
			if same.Category != "execution_already_started" || same.State.RuntimeID != pair[0] {
				t.Fatalf("idempotent start = %#v", same)
			}
		})
	}
}

// newIssueObservation reproduces an Issue created by `work-item create`: no
// axiom:stage:* marker on the Issue and no stage label in the repository.
func newIssueObservation() ProjectionObservation {
	observation := s4ProjectionObservation()
	observation.RepositoryLabels = []string{"bug", "external"}
	observation.IssueLabels = []string{"external"}
	return observation
}

func TestS9FirstProjectionOfNewIssueBootstrapsStageAndConverges(t *testing.T) {
	service, store, provider := newS4Service(t)
	provider.observation = newIssueObservation()
	target := s4Target()
	service.Start(context.Background(), target)
	transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	revision := transitioned.State.Revision

	previewed := service.PrepareProjection(context.Background(), target, revision)
	if previewed.Status != Succeeded || previewed.Preview == nil {
		t.Fatalf("first preview = %#v", previewed)
	}
	assertOnlyProjectionEffects(t, previewed.Preview.Effects,
		ProjectionEffect{Kind: CreateStageLabel, Value: "axiom:stage:specifying"},
		ProjectionEffect{Kind: AddStageLabel, Value: "axiom:stage:specifying"},
		ProjectionEffect{Kind: PostTransitionComment, Value: previewed.Preview.Comment},
	)
	projected := service.Project(context.Background(), target, revision, previewed.Preview.Digest, true)
	if projected.Category != "projection_converged" || !provider.hasLabel("axiom:stage:specifying") || !provider.hasLabel("external") || provider.commentCount != 1 || provider.effectCount(RemoveStageLabel) != 0 {
		t.Fatalf("projection = %#v provider=%#v", projected, provider)
	}
	if !projectionComplete(store.state, revision) {
		t.Fatalf("projection ledger incomplete: %#v", store.state.Projections)
	}

	applied := len(provider.effects)
	replayPreview := service.PrepareProjection(context.Background(), target, revision)
	if replayPreview.Preview == nil || len(replayPreview.Preview.Effects) != 0 {
		t.Fatalf("replay preview = %#v", replayPreview)
	}
	replayed := service.Project(context.Background(), target, revision, previewed.Preview.Digest, true)
	if replayed.Status != Succeeded || replayed.Category != "projection_converged" || len(provider.effects) != applied || provider.commentCount != 1 {
		t.Fatalf("replay = %#v effects=%v", replayed, provider.effects)
	}
}

func TestS9BootstrapStillRefusesInvalidOrMultipleStageMarkers(t *testing.T) {
	for name, labels := range map[string][]string{
		"invalid stage":   {"external", "axiom:stage:invented"},
		"multiple stages": {"axiom:stage:intake", "axiom:stage:specifying"},
		"unknown axiom":   {"axiom:unknown-marker"},
	} {
		t.Run(name, func(t *testing.T) {
			service, _, provider := newS4Service(t)
			provider.observation = newIssueObservation()
			provider.observation.IssueLabels = labels
			target := s4Target()
			service.Start(context.Background(), target)
			transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
			previewed := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
			if previewed.Status != Failed || previewed.Category != "recovery_required" || previewed.Preview != nil || len(provider.effects) != 0 {
				t.Fatalf("preview = %#v effects=%v", previewed, provider.effects)
			}
		})
	}
}

func TestS9LosingAnEstablishedStageMarkerIsDriftNotBootstrap(t *testing.T) {
	service, _, provider := newS4Service(t)
	provider.observation = newIssueObservation()
	target := s4Target()
	service.Start(context.Background(), target)
	first := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	previewed := service.PrepareProjection(context.Background(), target, first.State.Revision)
	if projected := service.Project(context.Background(), target, first.State.Revision, previewed.Preview.Digest, true); projected.Category != "projection_converged" {
		t.Fatalf("bootstrap projection = %#v", projected)
	}

	// An external actor removes the confirmed marker.
	provider.observation.IssueLabels = []string{"external"}
	applied := len(provider.effects)
	same := service.PrepareProjection(context.Background(), target, first.State.Revision)
	if same.Status != Failed || same.Category != "recovery_required" {
		t.Fatalf("same-revision drift = %#v", same)
	}
	next := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: first.State.Revision, Stage: Specification, Outcome: OutcomePassed})
	later := service.PrepareProjection(context.Background(), target, next.State.Revision)
	if later.Status != Failed || later.Category != "recovery_required" {
		t.Fatalf("later-revision drift = %#v", later)
	}
	if forced := service.Project(context.Background(), target, next.State.Revision, "any", true); forced.Category != "recovery_required" || len(provider.effects) != applied {
		t.Fatalf("drift projection = %#v effects=%v", forced, provider.effects)
	}
}

func TestS9PartialBootstrapBeforeStageAdditionRemainsBootstrap(t *testing.T) {
	service, store, provider := newS4Service(t)
	provider.observation = newIssueObservation()
	provider.suppressEffect = AddStageLabel
	target := s4Target()
	service.Start(context.Background(), target)
	transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	revision := transitioned.State.Revision
	previewed := service.PrepareProjection(context.Background(), target, revision)
	interrupted := service.Project(context.Background(), target, revision, previewed.Preview.Digest, true)
	if interrupted.Status != Partial || provider.hasLabel("axiom:stage:specifying") || projectionEstablished(store.state) {
		t.Fatalf("interrupted bootstrap = %#v ledger=%#v", interrupted, store.state.Projections)
	}
	retry := service.PrepareProjection(context.Background(), target, revision)
	if retry.Status != Succeeded || retry.Preview == nil {
		t.Fatalf("retry preview = %#v", retry)
	}
	assertOnlyProjectionEffects(t, retry.Preview.Effects,
		ProjectionEffect{Kind: AddStageLabel, Value: "axiom:stage:specifying"},
		ProjectionEffect{Kind: PostTransitionComment, Value: retry.Preview.Comment},
	)
	if converged := service.Project(context.Background(), target, revision, retry.Preview.Digest, true); converged.Category != "projection_converged" || provider.effectCount(CreateStageLabel) != 1 || provider.commentCount != 1 {
		t.Fatalf("retry projection = %#v effects=%v", converged, provider.effects)
	}
}
