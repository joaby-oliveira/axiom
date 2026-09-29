package workitem

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/provenance"
)

func TestPrepareCompleteDraftIsDeterministicReadOnlyAndPreservesAuthorship(t *testing.T) {
	provider := &fakeCapability{}
	service := testService(provider, newFakeStore())
	input := completeDraft()
	input.Context = SectionInput{Elaborated: "Axiom synthesized context"}
	first := service.Prepare(context.Background(), input)
	second := service.Prepare(context.Background(), input)
	if first.Status != completion.Success || first.Draft == nil || second.Draft == nil || first.Draft.Digest != second.Draft.Digest || provider.creates != 0 || provider.reads != 0 || provider.reconciles != 0 {
		t.Fatalf("prepare = %#v / %#v calls=%+v", first, second, provider)
	}
	if got := first.Draft.Draft.Sections[2].Authorship; got != provenance.AxiomAuthored {
		t.Fatalf("context authorship = %q", got)
	}
	if got := first.Draft.Draft.Sections[0]; got.Content != input.Intent || got.Authorship != provenance.UserAuthored {
		t.Fatalf("intent reuse = %#v", got)
	}
	wantEffects := []string{"publish_local_create_attempt_fence", "create_provider_work_item", "publish_local_work_item_link"}
	if !reflect.DeepEqual(first.Draft.Effects, wantEffects) || first.Draft.ExpectedRevisions.Local != "missing" {
		t.Fatalf("preview authority facts = %#v", first.Draft)
	}
}

func TestPrepareAsksOnlyMateriallyMissingFields(t *testing.T) {
	input := completeDraft()
	input.Context = SectionInput{}
	input.NonGoals = SectionInput{}
	result := testService(&fakeCapability{}, newFakeStore()).Prepare(context.Background(), input)
	want := []Question{{"context", "What context materially changes this work?"}, {"non_goals", "What is explicitly out of scope?"}}
	if result.Status != completion.ValidationFailure || result.Category != "draft_incomplete" || !reflect.DeepEqual(result.Questions, want) {
		t.Fatalf("questions = %#v", result)
	}
}

func TestPrepareRejectsSensitiveOversizedAndInvalidTextWithoutProviderEffects(t *testing.T) {
	provider := &fakeCapability{}
	service := testService(provider, newFakeStore())
	for name, value := range map[string]string{
		"secret":    "token=SYNTHETIC_REJECTED",
		"oversized": strings.Repeat("x", maxFieldBytes+1),
		"control":   "bad\x00value",
	} {
		t.Run(name, func(t *testing.T) {
			input := completeDraft()
			input.Scope.Supplied = value
			result := service.Prepare(context.Background(), input)
			if result.Status != completion.ValidationFailure || result.Draft != nil {
				t.Fatalf("result = %#v", result)
			}
		})
	}
	if provider.creates+provider.reads+provider.reconciles != 0 {
		t.Fatalf("provider effects = %+v", provider)
	}
}

func TestPrepareRejectsAggregateDraftAbovePreviewBudget(t *testing.T) {
	provider := &fakeCapability{}
	input := completeDraft()
	large := strings.Repeat("x", maxDraftBytes/4)
	input.DesiredOutcome.Supplied = large
	input.Context.Supplied = large
	input.Scope.Supplied = large
	input.Constraints.Supplied = large
	result := testService(provider, newFakeStore()).Prepare(context.Background(), input)
	if result.Status != completion.ValidationFailure || result.Category != "draft_too_large" || provider.creates+provider.reads+provider.reconciles != 0 {
		t.Fatalf("aggregate result=%#v provider=%+v", result, provider)
	}
}

func TestPrepareCancellationAndCapabilityMismatchHaveZeroEffects(t *testing.T) {
	provider := &fakeCapability{}
	input := completeDraft()
	input.Cancelled = true
	if result := testService(provider, newFakeStore()).Prepare(context.Background(), input); result.Status != completion.Interrupted {
		t.Fatalf("cancel = %#v", result)
	}
	service := New(fakeResolver{provider: "unsupported"}, provider, provider, newFakeStore(), testProvenance())
	input.Cancelled = false
	if result := service.Prepare(context.Background(), input); result.Category != "work_item_capability_unavailable" {
		t.Fatalf("capability = %#v", result)
	}
	if provider.creates+provider.reads+provider.reconciles != 0 {
		t.Fatal("provider called")
	}
}

func TestCreateRequiresExactPreviewAuthorityAndPersistsConfirmedIssue(t *testing.T) {
	provider := &fakeCapability{}
	store := newFakeStore()
	service := testService(provider, store)
	input := completeDraft()
	preview := service.Prepare(context.Background(), input)
	denied := service.Create(context.Background(), input, "stale", true)
	if denied.Status != completion.DeniedAuthority || provider.creates != 0 || store.saves != 0 {
		t.Fatalf("denied = %#v calls=%d saves=%d", denied, provider.creates, store.saves)
	}
	created := service.Create(context.Background(), input, preview.Draft.Digest, true)
	if created.Status != completion.Success || created.Link.ExternalID != "7" || provider.creates != 1 || store.saves != 1 {
		t.Fatalf("created = %#v calls=%d saves=%d", created, provider.creates, store.saves)
	}
}

func TestCreateReconcilesBeforeRetryAndNeverBlindlyDuplicates(t *testing.T) {
	provider := &fakeCapability{createErr: &ProviderError{Kind: ProviderAmbiguous, Ambiguous: true, Retryable: true}}
	service := testService(provider, newFakeStore())
	input := completeDraft()
	preview := service.Prepare(context.Background(), input)
	provider.reconcileSequence = [][]External{{}, {{ID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}}}
	result := service.Create(context.Background(), input, preview.Draft.Digest, true)
	if result.Status != completion.Success || provider.creates != 1 || provider.reconciles != 2 {
		t.Fatalf("reconciled = %#v provider=%+v", result, provider)
	}

	provider = &fakeCapability{reconcileSequence: [][]External{{{ID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}}}}
	service = testService(provider, newFakeStore())
	preview = service.Prepare(context.Background(), input)
	result = service.Create(context.Background(), input, preview.Draft.Digest, true)
	if result.Status != completion.Success || provider.creates != 0 || provider.reconciles != 1 {
		t.Fatalf("pre-create reconcile = %#v provider=%+v", result, provider)
	}
}

func TestCreateAmbiguousWithoutMatchReturnsSafeRetryBoundary(t *testing.T) {
	provider := &fakeCapability{createErr: &ProviderError{Kind: ProviderAmbiguous, Ambiguous: true, Retryable: true}, reconcileSequence: [][]External{{}, {}}}
	store := newFakeStore()
	service := testService(provider, store)
	input := completeDraft()
	preview := service.Prepare(context.Background(), input)
	result := service.Create(context.Background(), input, preview.Draft.Digest, true)
	if result.Status != completion.RetryableFailure || result.Category != "provider_create_ambiguous" || provider.creates != 1 {
		t.Fatalf("ambiguous = %#v", result)
	}
	second := testService(provider, store).Create(context.Background(), input, preview.Draft.Digest, true)
	if second.Status != completion.RetryableFailure || second.Category != "provider_create_ambiguous" || provider.creates != 1 {
		t.Fatalf("second execution = %#v provider=%+v", second, provider)
	}
}

func TestCreateSecondExecutionReconcilesAmbiguousAttemptAndPersistsLink(t *testing.T) {
	match := External{ID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}
	provider := &fakeCapability{
		createErr:         &ProviderError{Kind: ProviderAmbiguous, Ambiguous: true, Retryable: true},
		reconcileSequence: [][]External{{}, {}, {match}},
	}
	store := newFakeStore()
	input := completeDraft()
	firstService := testService(provider, store)
	preview := firstService.Prepare(context.Background(), input)
	first := firstService.Create(context.Background(), input, preview.Draft.Digest, true)
	if first.Status != completion.RetryableFailure {
		t.Fatalf("first execution = %#v", first)
	}
	second := testService(provider, store).Create(context.Background(), input, preview.Draft.Digest, true)
	if second.Status != completion.Success || second.Link.ExternalID != "7" || provider.creates != 1 || store.saves != 1 {
		t.Fatalf("second execution = %#v provider=%+v saves=%d", second, provider, store.saves)
	}
}

func TestCreatePendingAttemptWithMultipleMatchesFailsClosedAcrossExecutions(t *testing.T) {
	match := External{ID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}
	provider := &fakeCapability{
		createErr:         &ProviderError{Kind: ProviderAmbiguous, Ambiguous: true, Retryable: true},
		reconcileSequence: [][]External{{}, {}, {match, match}},
	}
	store := newFakeStore()
	input := completeDraft()
	service := testService(provider, store)
	preview := service.Prepare(context.Background(), input)
	if first := service.Create(context.Background(), input, preview.Draft.Digest, true); first.Status != completion.RetryableFailure {
		t.Fatalf("first execution = %#v", first)
	}
	second := testService(provider, store).Create(context.Background(), input, preview.Draft.Digest, true)
	if second.Status != completion.Failure || second.Category != "provider_reconciliation_ambiguous" || provider.creates != 1 {
		t.Fatalf("second execution = %#v provider=%+v", second, provider)
	}
}

func TestCreateRetriesOnlyAfterProviderProvesNoEffect(t *testing.T) {
	provider := &fakeCapability{createErr: &ProviderError{Kind: ProviderUnauthenticated, EffectNotCommitted: true}}
	store := newFakeStore()
	input := completeDraft()
	service := testService(provider, store)
	preview := service.Prepare(context.Background(), input)
	first := service.Create(context.Background(), input, preview.Draft.Digest, true)
	if first.Status != completion.Failure || provider.creates != 1 {
		t.Fatalf("definitive failure = %#v provider=%+v", first, provider)
	}
	provider.createErr = nil
	second := testService(provider, store).Create(context.Background(), input, preview.Draft.Digest, true)
	if second.Status != completion.Success || provider.creates != 2 {
		t.Fatalf("safe retry = %#v provider=%+v", second, provider)
	}
}

func TestCreateUntypedFailureRemainsReconcileOnly(t *testing.T) {
	provider := &fakeCapability{createErr: errors.New("unknown provider outcome")}
	store := newFakeStore()
	input := completeDraft()
	service := testService(provider, store)
	preview := service.Prepare(context.Background(), input)
	first := service.Create(context.Background(), input, preview.Draft.Digest, true)
	second := testService(provider, store).Create(context.Background(), input, preview.Draft.Digest, true)
	if first.Status != completion.RetryableFailure || second.Status != completion.RetryableFailure || provider.creates != 1 {
		t.Fatalf("unknown outcome = %#v / %#v provider=%+v", first, second, provider)
	}
}

func TestCreateRejectsMultipleReconciliationMatchesWithoutCreating(t *testing.T) {
	match := External{ID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}
	provider := &fakeCapability{reconcileSequence: [][]External{{match, match}}}
	service := testService(provider, newFakeStore())
	input := completeDraft()
	preview := service.Prepare(context.Background(), input)
	result := service.Create(context.Background(), input, preview.Draft.Digest, true)
	if result.Status != completion.Failure || result.Category != "provider_reconciliation_ambiguous" || provider.creates != 0 {
		t.Fatalf("result=%#v provider=%+v", result, provider)
	}
}

func TestConfirmedProviderEffectAndLocalFailureIsPartial(t *testing.T) {
	store := newFakeStore()
	store.failSave = true
	provider := &fakeCapability{}
	service := testService(provider, store)
	input := completeDraft()
	preview := service.Prepare(context.Background(), input)
	result := service.Create(context.Background(), input, preview.Draft.Digest, true)
	if result.Status != completion.Partial || result.Category != "provider_confirmed_local_failed" || result.Link.ExternalID != "7" {
		t.Fatalf("partial = %#v", result)
	}
}

func TestSelectRequiresReviewedLocalAuthorityAndUsesExpectedRevision(t *testing.T) {
	provider := &fakeCapability{}
	store := newFakeStore()
	service := testService(provider, store)
	target := completeDraft().Target
	preview := service.PreviewSelect(context.Background(), target, "7")
	if preview.Status != completion.Success || preview.Selection == nil || store.saves != 0 {
		t.Fatalf("preview = %#v", preview)
	}
	denied := service.Select(context.Background(), target, "7", "stale", true)
	if denied.Status != completion.DeniedAuthority || store.saves != 0 {
		t.Fatalf("denied = %#v", denied)
	}
	linked := service.Select(context.Background(), target, "7", preview.Selection.Digest, true)
	if linked.Status != completion.Success || linked.Link.ExternalID != "7" || store.saves != 1 {
		t.Fatalf("linked = %#v", linked)
	}
}

func TestSelectRejectsStaleLocalRevision(t *testing.T) {
	provider := &fakeCapability{}
	store := newFakeStore()
	store.links["github:owner/repo:7"] = Link{Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "CLOSED", Revision: [32]byte{2}}
	service := testService(provider, store)
	target := completeDraft().Target
	preview := service.PreviewSelect(context.Background(), target, "7")
	store.links["github:owner/repo:7"] = Link{Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "CLOSED", Revision: [32]byte{3}}
	result := service.Select(context.Background(), target, "7", preview.Selection.Digest, true)
	if result.Status != completion.DeniedAuthority || result.Category != "local_authority_denied" || store.saves != 0 {
		t.Fatalf("stale selection = %#v", result)
	}
}

func completeDraft() DraftInput {
	return DraftInput{
		Target: Target{ProjectSelector: "sample", RepositoryKey: "main", ProviderResource: "owner/repo"},
		Intent: "A reported behavior blocks delivery", DesiredOutcome: SectionInput{Supplied: "Delivery proceeds safely"},
		Context: SectionInput{Supplied: "Observed on supported hosts"}, Scope: SectionInput{Supplied: "Bounded application change"},
		Constraints: SectionInput{Supplied: "Preserve authority"}, NonGoals: SectionInput{Supplied: "No workflow execution"},
		Acceptance: SectionInput{Supplied: "Deterministic tests pass"},
	}
}

type fakeResolver struct{ provider string }

func (r fakeResolver) Resolve(context.Context, string) (Project, string) {
	provider := r.provider
	if provider == "" {
		provider = "github"
	}
	return Project{ID: "123e4567-e89b-42d3-a456-426614174000", Provider: provider, Repositories: []Repository{{Key: "main", Path: "/unused"}}}, ""
}

type fakeCapability struct {
	creates, reads, reconciles int
	createErr                  error
	reconcileSequence          [][]External
}

func (*fakeCapability) ProviderID() string              { return "github" }
func (*fakeCapability) ValidResource(value string) bool { return value == "owner/repo" }
func (*fakeCapability) ValidExternal(resource, selector string, external External) bool {
	return resource == "owner/repo" && selector == external.ID && external.URL == "https://github.com/owner/repo/issues/"+selector && (external.State == "OPEN" || external.State == "CLOSED")
}
func (*fakeCapability) Render(_ Draft, _ DraftTarget, correlation string, _ provenance.Value) (ProviderDocument, error) {
	return ProviderDocument{Title: "Axiom draft", Body: "<!-- axiom:work-item-draft:" + correlation + " -->"}, nil
}
func (p *fakeCapability) Create(context.Context, CreateRequest) (External, error) {
	p.creates++
	if p.createErr != nil {
		return External{}, p.createErr
	}
	return External{ID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}, nil
}
func (p *fakeCapability) ReconcileCreate(context.Context, string, string) ([]External, error) {
	p.reconciles++
	if len(p.reconcileSequence) == 0 {
		return nil, nil
	}
	result := p.reconcileSequence[0]
	p.reconcileSequence = p.reconcileSequence[1:]
	return result, nil
}
func (p *fakeCapability) Read(context.Context, string, string) (External, error) {
	p.reads++
	return External{ID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}, nil
}
func (*fakeCapability) Comment(context.Context, string, string, string) error { return nil }
func (*fakeCapability) Close(context.Context, string, string) (External, error) {
	return External{ID: "7", URL: "https://github.com/owner/repo/issues/7", State: "CLOSED"}, nil
}

type fakeStore struct {
	links    map[string]Link
	attempts map[string]CreateAttempt
	saves    int
	failSave bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{links: map[string]Link{}, attempts: map[string]CreateAttempt{}}
}
func (s *fakeStore) Save(_ context.Context, link Link) error {
	s.saves++
	if s.failSave {
		return errors.New("controlled write failure")
	}
	key := link.Provider + ":" + link.Resource + ":" + link.ExternalID
	if current, exists := s.links[key]; exists && link.Revision != current.Revision {
		return ErrConflict
	}
	link.Revision = [32]byte{1}
	s.links[key] = link
	return nil
}
func (s *fakeStore) Load(_ context.Context, _, _, provider, resource, externalID string) (Link, error) {
	if provider == "" && resource == "" {
		var found Link
		for _, link := range s.links {
			if link.ExternalID != externalID {
				continue
			}
			if found.ExternalID != "" {
				return Link{}, ErrConflict
			}
			found = link
		}
		if found.ExternalID != "" {
			return found, nil
		}
		return Link{}, ErrNotFound
	}
	link, ok := s.links[provider+":"+resource+":"+externalID]
	if !ok {
		return Link{}, ErrNotFound
	}
	return link, nil
}
func (s *fakeStore) SaveCreateAttempt(_ context.Context, attempt CreateAttempt) error {
	key := attempt.Target.Provider + ":" + attempt.Target.Resource
	current, exists := s.attempts[key]
	if exists && attempt.Revision != current.Revision {
		return ErrConflict
	}
	if !exists && attempt.Revision != ([32]byte{}) {
		return ErrConflict
	}
	attempt.Revision[0]++
	s.attempts[key] = attempt
	return nil
}
func (s *fakeStore) LoadCreateAttempt(_ context.Context, target DraftTarget) (CreateAttempt, error) {
	attempt, ok := s.attempts[target.Provider+":"+target.Resource]
	if !ok {
		return CreateAttempt{}, ErrNotFound
	}
	return attempt, nil
}

func testService(provider *fakeCapability, store *fakeStore) Service {
	return New(fakeResolver{}, provider, provider, store, testProvenance())
}

func testProvenance() provenance.Value {
	value, _ := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "abc123def456", SourceState: provenance.Clean}, nil)
	return value
}
