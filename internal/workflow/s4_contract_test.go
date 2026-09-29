package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/provenance"
)

func TestS4StartConvergesAndConflictingScopeFails(t *testing.T) {
	service, store, _ := newS4Service(t)
	target := s4Target()

	first := service.Start(context.Background(), target)
	second := service.Start(context.Background(), target)
	if first.Category != "execution_started" || second.Category != "execution_already_started" {
		t.Fatalf("start results = %#v %#v", first, second)
	}
	if first.State.ExecutionID == "" || second.State.ExecutionID != first.State.ExecutionID || first.State.Revision != 1 {
		t.Fatalf("unstable execution lineage = %#v %#v", first.State, second.State)
	}

	store.state.RuntimeID = "other-runtime"
	conflict := service.Start(context.Background(), target)
	if conflict.Category != "execution_scope_conflict" {
		t.Fatalf("conflicting start = %#v", conflict)
	}
}

func TestS4SequentialTransitionRequiresExactRevisionAndReplayConverges(t *testing.T) {
	service, _, _ := newS4Service(t)
	target := s4Target()
	started := service.Start(context.Background(), target)

	skipped := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Specification, Outcome: OutcomePassed})
	if skipped.Category != "workflow_stage_conflict" {
		t.Fatalf("skipped transition = %#v", skipped)
	}
	advanced := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	if advanced.Category != "workflow_advanced" || advanced.State.Stage != Specification || advanced.State.Revision != 2 {
		t.Fatalf("advance = %#v", advanced)
	}
	replayed := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	if replayed.Category != "workflow_transition_replayed" || replayed.State.Revision != 2 {
		t.Fatalf("replay = %#v", replayed)
	}
	stale := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: started.State.Revision, Stage: Intake, Outcome: OutcomeFailed})
	if stale.Category != "stale_execution_revision" {
		t.Fatalf("stale transition = %#v", stale)
	}
}

func TestS4InterruptedTransitionDoesNotAdvanceAndResumeNeedsRevision(t *testing.T) {
	service, _, _ := newS4Service(t)
	target := s4Target()
	service.Start(context.Background(), target)

	interrupted := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomeFailed})
	if interrupted.Status != Interrupted || interrupted.State.Stage != Intake || interrupted.State.Status != ExecutionInterrupted || interrupted.State.Revision != 2 {
		t.Fatalf("interrupted = %#v", interrupted)
	}
	stale := service.Resume(context.Background(), target, 1)
	if stale.Category != "stale_execution_revision" {
		t.Fatalf("stale resume = %#v", stale)
	}
	resumed := service.Resume(context.Background(), target, 2)
	if resumed.Category != "execution_resumed" || resumed.State.Stage != Intake || resumed.State.Revision != 3 {
		t.Fatalf("resume = %#v", resumed)
	}
}

func TestS4ProjectionPreviewAuthorityReplayAndExternalPreservation(t *testing.T) {
	service, _, provider := newS4Service(t)
	target := s4Target()
	service.Start(context.Background(), target)
	transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	provider.observation.RepositoryLabels = []string{"bug"}
	provider.observation.IssueLabels = []string{"external", "external:stage:foreign", "axiom:stage:intake"}

	previewed := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
	if previewed.Preview == nil || previewed.Status != Succeeded {
		t.Fatalf("preview = %#v", previewed)
	}
	if got, want := effectKinds(previewed.Preview.Effects), []ProjectionEffectKind{CreateStageLabel, AddStageLabel, RemoveStageLabel, PostTransitionComment}; !equalEffectKinds(got, want) {
		t.Fatalf("effects = %v want %v", got, want)
	}
	denied := service.Project(context.Background(), target, transitioned.State.Revision, "wrong", true)
	if denied.Category != "projection_authority_denied" || len(provider.effects) != 0 {
		t.Fatalf("denied projection = %#v effects=%v", denied, provider.effects)
	}
	projected := service.Project(context.Background(), target, transitioned.State.Revision, previewed.Preview.Digest, true)
	if projected.Category != "projection_converged" || !provider.hasLabel("external") || !provider.hasLabel("external:stage:foreign") || provider.hasLabel("axiom:stage:intake") || !provider.hasLabel("axiom:stage:specifying") || provider.commentCount != 1 {
		t.Fatalf("projection = %#v provider=%#v", projected, provider)
	}
	replayed := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
	if replayed.Preview == nil || len(replayed.Preview.Effects) != 0 {
		t.Fatalf("replay preview = %#v", replayed)
	}
}

func TestS4ChangedIssueStateInvalidatesProjectionAuthority(t *testing.T) {
	service, _, provider := newS4Service(t)
	target := s4Target()
	service.Start(context.Background(), target)
	transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	previewed := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
	if previewed.Preview == nil {
		t.Fatalf("preview = %#v", previewed)
	}

	provider.observation.IssueState = "CLOSED"
	denied := service.Project(context.Background(), target, transitioned.State.Revision, previewed.Preview.Digest, true)
	if denied.Status != Denied || denied.Category != "projection_authority_denied" || len(provider.effects) != 0 {
		t.Fatalf("stale authority = %#v effects=%v", denied, provider.effects)
	}
	if denied.Preview == nil || denied.Preview.Digest == previewed.Preview.Digest {
		t.Fatalf("issue state did not change digest: before=%#v after=%#v", previewed.Preview, denied.Preview)
	}
}

func TestS4AmbiguousProviderEffectReconcilesWithoutDuplicate(t *testing.T) {
	service, _, provider := newS4Service(t)
	target := s4Target()
	service.Start(context.Background(), target)
	transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	previewed := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
	provider.ambiguousOnce = AddStageLabel

	result := service.Project(context.Background(), target, transitioned.State.Revision, previewed.Preview.Digest, true)
	if result.Category != "projection_converged" || provider.effectCount(AddStageLabel) != 1 || provider.commentCount != 1 {
		t.Fatalf("ambiguous reconcile = %#v effects=%v", result, provider.effects)
	}
}

func TestS4ProviderUnavailableDoesNotChangeLocalTruth(t *testing.T) {
	service, store, provider := newS4Service(t)
	target := s4Target()
	service.Start(context.Background(), target)
	transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	provider.inspectErr = &ProjectionError{Kind: ProjectionUnavailable, Retryable: true}
	result := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
	if result.Status != Retryable || store.state.Revision != 2 || len(store.state.Projections) != 0 {
		t.Fatalf("provider failure = %#v state=%#v", result, store.state)
	}
}

func TestS4ProjectionFailureClassification(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status Status
	}{
		{name: "rate_limited", err: &ProjectionError{Kind: ProjectionRateLimited, Retryable: true}, status: Retryable},
		{name: "invalid_response", err: &ProjectionError{Kind: ProjectionInvalidResponse}, status: Failed},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _, provider := newS4Service(t)
			target := s4Target()
			service.Start(context.Background(), target)
			transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
			provider.inspectErr = test.err
			result := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
			if result.Status != test.status {
				t.Fatalf("classification = %#v", result)
			}
		})
	}
}

func TestS4CancelledOperationHasNoEffect(t *testing.T) {
	service, store, provider := newS4Service(t)
	target := s4Target()
	service.Start(context.Background(), target)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transitioned := service.Transition(ctx, target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	projected := service.PrepareProjection(ctx, target, 1)
	if transitioned.Status != Interrupted || projected.Status != Interrupted || store.state.Revision != 1 || len(provider.effects) != 0 {
		t.Fatalf("cancelled results = %#v %#v state=%#v effects=%v", transitioned, projected, store.state, provider.effects)
	}
}

func TestS4ConfirmedProviderEffectThenBookkeepingFailureIsPartial(t *testing.T) {
	service, store, provider := newS4Service(t)
	target := s4Target()
	service.Start(context.Background(), target)
	transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	provider.observation.RepositoryLabels = []string{"axiom:stage:specifying"}
	provider.observation.IssueLabels = []string{"axiom:stage:specifying"}
	previewed := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
	store.failSaveAt = store.saves + 2
	result := service.Project(context.Background(), target, transitioned.State.Revision, previewed.Preview.Digest, true)
	if result.Status != Partial || result.Category != "provider_confirmed_projection_bookkeeping_failed" || provider.commentCount != 1 || store.state.Revision != 2 {
		t.Fatalf("partial = %#v provider=%#v state=%#v", result, provider, store.state)
	}
	if len(store.state.Projections) != 1 || store.state.Projections[0].Complete || len(store.state.Projections[0].Confirmed) != 0 {
		t.Fatalf("partial ledger = %#v", store.state.Projections)
	}

	retried := service.Project(context.Background(), target, transitioned.State.Revision, previewed.Preview.Digest, true)
	if retried.Status != Succeeded || retried.Category != "projection_converged" || provider.commentCount != 1 || provider.effectCount(PostTransitionComment) != 1 {
		t.Fatalf("retry = %#v provider=%#v", retried, provider)
	}
	if len(store.state.Projections) != 1 || !store.state.Projections[0].Complete || len(store.state.Projections[0].Intended) != 1 || len(store.state.Projections[0].Confirmed) != 1 {
		t.Fatalf("reconciled ledger = %#v", store.state.Projections)
	}
}

func TestS4ProviderSuccessWithoutObservedEffectIsRetryable(t *testing.T) {
	service, store, provider := newS4Service(t)
	target := s4Target()
	service.Start(context.Background(), target)
	transitioned := service.Transition(context.Background(), target, TransitionInput{ExpectedRevision: 1, Stage: Intake, Outcome: OutcomePassed})
	previewed := service.PrepareProjection(context.Background(), target, transitioned.State.Revision)
	provider.suppressEffect = CreateStageLabel

	result := service.Project(context.Background(), target, transitioned.State.Revision, previewed.Preview.Digest, true)
	if result.Status != Retryable || result.Category != "provider_projection_retryable" || len(store.state.Projections) != 1 || len(store.state.Projections[0].Confirmed) != 0 {
		t.Fatalf("unobserved success = %#v state=%#v", result, store.state)
	}
}

func TestS4ClosedStateRejectsIncoherentHistoryAndDuplicateProjection(t *testing.T) {
	_, store, _ := newS4Service(t)
	state := State{ExecutionID: "018f4a44-7c31-7dd4-9d00-111111111111", FormatVersion: FormatVersion, WorkflowVersion: WorkflowVersion, ProjectID: "123e4567-e89b-42d3-a456-426614174000", RepositoryKey: "main", WorkItem: WorkItem{Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}, RuntimeID: "codex", Stage: Specification, Revision: 2, Status: ExecutionActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Provenance: identity(newS4Source(t))}
	state.Transitions = []Transition{{Revision: 2, From: Intake, To: Plan, Outcome: OutcomePassed, RequestDigest: digest("request"), CommittedAt: state.UpdatedAt, Provenance: state.Provenance}}
	if ValidState(state) {
		t.Fatal("skipped stage accepted")
	}
	state.Transitions[0].To = Specification
	record := ProjectionRecord{ExecutionRevision: 2, Key: projectionKey(state.ExecutionID, 2)}
	state.Projections = []ProjectionRecord{record, record}
	if ValidState(state) {
		t.Fatal("duplicate projection revision accepted")
	}
	_ = store
}

func newS4Service(t *testing.T) (Service, *s4Store, *s4Projection) {
	t.Helper()
	source := newS4Source(t)
	store := &s4Store{}
	provider := &s4Projection{observation: s4ProjectionObservation()}
	service := New(
		s4Resolver{},
		s4WorkItems{},
		store,
		provider,
		nil,
		source,
		func() (string, error) { return "018f4a44-7c31-7dd4-9d00-111111111111", nil },
		func() time.Time { return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) },
	)
	return service, store, provider
}

func newS4Source(t *testing.T) provenance.Value {
	t.Helper()
	source, err := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "abc123", SourceState: provenance.Clean}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func s4Target() Target {
	return Target{ProjectSelector: "sample", RepositoryKey: "main", WorkItem: "7", RuntimeID: "codex"}
}

func s4ProjectionObservation() ProjectionObservation {
	return ProjectionObservation{
		Provider:         "github",
		Resource:         "owner/repo",
		IssueExternalID:  "7",
		IssueURL:         "https://github.com/owner/repo/issues/7",
		IssueState:       "OPEN",
		RepositoryLabels: []string{"axiom:stage:intake"},
		IssueLabels:      []string{"axiom:stage:intake"},
	}
}

type s4Resolver struct{}

func (s4Resolver) Resolve(context.Context, string) (Project, string) {
	return Project{ID: "123e4567-e89b-42d3-a456-426614174000", Repositories: []Repository{{Key: "main", Path: "/tmp/repository"}}}, ""
}

type s4WorkItems struct{}

func (s4WorkItems) Load(context.Context, string, string, string, string, string) (WorkItem, error) {
	return WorkItem{Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}, nil
}

type s4Store struct {
	state             State
	created           bool
	saves, failSaveAt int
}

func (s *s4Store) Create(_ context.Context, state State) error {
	if s.created {
		return ErrConflict
	}
	s.created = true
	s.state = cloneState(state)
	return nil
}

func (s *s4Store) Load(context.Context, string, string, WorkItem) (State, error) {
	if !s.created {
		return State{}, ErrNotFound
	}
	return cloneState(s.state), nil
}

func (s *s4Store) Save(_ context.Context, state State) error {
	s.saves++
	if s.failSaveAt != 0 && s.saves == s.failSaveAt {
		return errors.New("injected save failure")
	}
	if state.StorageRevision != s.state.StorageRevision {
		return ErrConflict
	}
	state.StorageRevision[0]++
	s.state = cloneState(state)
	return nil
}

type s4Projection struct {
	observation    ProjectionObservation
	effects        []ProjectionEffect
	ambiguousOnce  ProjectionEffectKind
	commentCount   int
	inspectErr     error
	suppressEffect ProjectionEffectKind
}

func (p *s4Projection) Inspect(context.Context, WorkItem, string, string) (ProjectionObservation, error) {
	if p.inspectErr != nil {
		return ProjectionObservation{}, p.inspectErr
	}
	return p.observation, nil
}

func (p *s4Projection) Apply(_ context.Context, _ WorkItem, effect ProjectionEffect) error {
	p.effects = append(p.effects, effect)
	if p.suppressEffect == effect.Kind {
		p.suppressEffect = ""
		return nil
	}
	switch effect.Kind {
	case CreateStageLabel:
		p.observation.RepositoryLabels = append(p.observation.RepositoryLabels, effect.Value)
	case AddStageLabel:
		if !p.hasLabel(effect.Value) {
			p.observation.IssueLabels = append(p.observation.IssueLabels, effect.Value)
		}
	case RemoveStageLabel:
		labels := p.observation.IssueLabels[:0]
		for _, label := range p.observation.IssueLabels {
			if label != effect.Value {
				labels = append(labels, label)
			}
		}
		p.observation.IssueLabels = labels
	case PostTransitionComment:
		p.observation.CommentPresent = true
		p.commentCount++
	}
	if p.ambiguousOnce == effect.Kind {
		p.ambiguousOnce = ""
		return &ProjectionError{Kind: ProjectionAmbiguous, Retryable: true, Ambiguous: true}
	}
	return nil
}

func (p *s4Projection) hasLabel(value string) bool {
	for _, label := range p.observation.IssueLabels {
		if label == value {
			return true
		}
	}
	return false
}

func (p *s4Projection) effectCount(kind ProjectionEffectKind) int {
	count := 0
	for _, effect := range p.effects {
		if effect.Kind == kind {
			count++
		}
	}
	return count
}

func effectKinds(effects []ProjectionEffect) []ProjectionEffectKind {
	result := make([]ProjectionEffectKind, len(effects))
	for index, effect := range effects {
		result[index] = effect.Kind
	}
	return result
}

func equalEffectKinds(left, right []ProjectionEffectKind) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
