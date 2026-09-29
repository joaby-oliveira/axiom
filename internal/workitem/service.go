// Package workitem implements provider-neutral Work Item draft and linkage use cases.
package workitem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/provenance"
)

const (
	maxFieldBytes = 8 * 1024
	maxDraftBytes = 24 * 1024
)

var (
	ErrNotFound         = errors.New("work item not found")
	ErrConflict         = errors.New("work item conflict")
	ErrRecoveryRequired = errors.New("work item recovery required")
)

type Repository struct{ Key, Path string }
type Project struct {
	ID, Provider string
	Repositories []Repository
}

type Resolver interface {
	Resolve(context.Context, string) (Project, string)
}

type External struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	State string `json:"state"`
}

type ProviderErrorKind string

const (
	ProviderUnavailable     ProviderErrorKind = "provider_unavailable"
	ProviderUnauthenticated ProviderErrorKind = "provider_unauthenticated"
	ProviderRateLimited     ProviderErrorKind = "provider_rate_limited"
	ProviderInvalidResponse ProviderErrorKind = "provider_invalid_response"
	ProviderAmbiguous       ProviderErrorKind = "provider_ambiguous"
)

type ProviderError struct {
	Kind               ProviderErrorKind
	Retryable          bool
	Ambiguous          bool
	EffectNotCommitted bool
}

func (e *ProviderError) Error() string { return string(e.Kind) }

type ProviderDocument struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type CreateRequest struct {
	Resource, Correlation string
	Document              ProviderDocument
}

// Capability is operation-shaped. Provider formatting and transport stay behind
// this boundary; application owns draft, authority, and outcome semantics.
type Capability interface {
	ProviderID() string
	ValidResource(string) bool
	ValidExternal(string, string, External) bool
	Render(Draft, DraftTarget, string, provenance.Value) (ProviderDocument, error)
	Create(context.Context, CreateRequest) (External, error)
	ReconcileCreate(context.Context, string, string) ([]External, error)
	Read(context.Context, string, string) (External, error)
}

// LegacyProjection preserves historical POC-only workflow behavior. Slice 3
// never calls these methods or treats them as its contract.
type LegacyProjection interface {
	Comment(context.Context, string, string, string) error
	Close(context.Context, string, string) (External, error)
}

type Link struct {
	ProjectID, RepositoryKey       string
	Provider, Resource, ExternalID string
	URL, State                     string
	Revision                       [32]byte
}

type CreateAttemptState string

const (
	// Pending means an external create may have happened. It never authorizes
	// another create; only reconciliation may resolve it.
	CreateAttemptPending      CreateAttemptState = "pending"
	CreateAttemptRetryAllowed CreateAttemptState = "retry_allowed"
	CreateAttemptConfirmed    CreateAttemptState = "confirmed"
)

type CreateAttempt struct {
	Target        DraftTarget
	Correlation   string
	PreviewDigest string
	State         CreateAttemptState
	ExternalID    string
	Revision      [32]byte
}

type Store interface {
	Save(context.Context, Link) error
	Load(context.Context, string, string, string, string, string) (Link, error)
	SaveCreateAttempt(context.Context, CreateAttempt) error
	LoadCreateAttempt(context.Context, DraftTarget) (CreateAttempt, error)
}

type Target struct {
	ProjectSelector, RepositoryKey, ProviderResource string
}

type SectionInput struct {
	Supplied, Elaborated string
}

type DraftInput struct {
	Target                                  Target
	Intent                                  string
	Problem, DesiredOutcome, Context, Scope SectionInput
	Constraints, NonGoals, Acceptance       SectionInput
	Cancelled                               bool
}

type DraftSection struct {
	Name       string                `json:"name"`
	Content    string                `json:"content"`
	Authorship provenance.Authorship `json:"authorship"`
}

type Draft struct {
	Sections []DraftSection `json:"sections"`
}

type DraftTarget struct {
	ProjectID     string `json:"projectId"`
	RepositoryKey string `json:"repositoryKey"`
	Provider      string `json:"provider"`
	Resource      string `json:"resource"`
}

type ExpectedRevisions struct {
	Local string `json:"local"`
}

type DraftPreview struct {
	Target            DraftTarget       `json:"target"`
	Draft             Draft             `json:"draft"`
	ProviderDocument  ProviderDocument  `json:"providerDocument"`
	Effects           []string          `json:"effects"`
	ExpectedRevisions ExpectedRevisions `json:"expectedRevisions"`
	Correlation       string            `json:"correlation"`
	Digest            string            `json:"digest"`
}

type Question struct {
	Field  string `json:"field"`
	Prompt string `json:"prompt"`
}

type SelectionPreview struct {
	Target            DraftTarget       `json:"target"`
	External          External          `json:"external"`
	Effects           []string          `json:"effects"`
	ExpectedRevisions ExpectedRevisions `json:"expectedRevisions"`
	Digest            string            `json:"digest"`
}

type Result struct {
	Status    completion.Status
	Category  string
	Link      Link
	Draft     *DraftPreview
	Selection *SelectionPreview
	Questions []Question
}

const (
	Succeeded = completion.Success
	Failed    = completion.Failure
)

type Service struct {
	resolver   Resolver
	capability Capability
	legacy     LegacyProjection
	store      Store
	source     provenance.Value
}

func New(resolver Resolver, capability Capability, legacy LegacyProjection, store Store, source provenance.Value) Service {
	return Service{resolver: resolver, capability: capability, legacy: legacy, store: store, source: source}
}

func (s Service) Prepare(ctx context.Context, input DraftInput) Result {
	if input.Cancelled || ctx.Err() != nil {
		return result(completion.Interrupted, "draft_cancelled")
	}
	project, target, failure := s.resolve(ctx, input.Target)
	if failure.Category != "" {
		return failure
	}
	draft, questions, category := normalizeDraft(input)
	if category != "" {
		return result(completion.ValidationFailure, category)
	}
	if len(questions) != 0 {
		value := result(completion.ValidationFailure, "draft_incomplete")
		value.Questions = questions
		return value
	}
	target.ProjectID = project.ID
	preview, err := s.preview(draft, target)
	if err != nil {
		return result(completion.ValidationFailure, "draft_invalid")
	}
	return Result{Status: completion.Success, Category: "work_item_draft_ready", Draft: &preview}
}

func (s Service) Create(ctx context.Context, input DraftInput, previewDigest string, authorized bool) Result {
	prepared := s.Prepare(ctx, input)
	if prepared.Status != completion.Success || prepared.Draft == nil {
		return prepared
	}
	if !authorized || previewDigest == "" || previewDigest != prepared.Draft.Digest {
		prepared.Status = completion.DeniedAuthority
		prepared.Category = "external_authority_denied"
		return prepared
	}
	if ctx.Err() != nil {
		return result(completion.Interrupted, "create_cancelled")
	}
	attempt, attemptErr := s.store.LoadCreateAttempt(ctx, prepared.Draft.Target)
	if attemptErr == nil {
		return s.resumeCreate(ctx, *prepared.Draft, attempt)
	}
	if !errors.Is(attemptErr, ErrNotFound) {
		return createAttemptFailure(attemptErr)
	}
	reconciled, reconciliation := s.reconcile(ctx, *prepared.Draft)
	if reconciliation.Category != "" {
		return reconciliation
	}
	if reconciled.ID != "" {
		return s.persistConfirmed(ctx, prepared.Draft.Target, reconciled)
	}
	attempt = CreateAttempt{
		Target: prepared.Draft.Target, Correlation: prepared.Draft.Correlation,
		PreviewDigest: prepared.Draft.Digest, State: CreateAttemptPending,
	}
	if err := s.store.SaveCreateAttempt(ctx, attempt); err != nil {
		return createAttemptFailure(err)
	}
	attempt, attemptErr = s.store.LoadCreateAttempt(ctx, prepared.Draft.Target)
	if attemptErr != nil {
		return createAttemptFailure(attemptErr)
	}
	return s.executeCreate(ctx, *prepared.Draft, attempt)
}

func (s Service) resumeCreate(ctx context.Context, preview DraftPreview, attempt CreateAttempt) Result {
	if attempt.State == CreateAttemptPending {
		return s.reconcileAttempt(ctx, attempt)
	}
	if attempt.State == CreateAttemptConfirmed && attempt.Correlation == preview.Correlation && attempt.PreviewDigest == preview.Digest {
		link, err := s.store.Load(ctx, attempt.Target.ProjectID, attempt.Target.RepositoryKey, attempt.Target.Provider, attempt.Target.Resource, attempt.ExternalID)
		if err == nil {
			return Result{Status: completion.Success, Category: "work_item_already_linked", Link: link}
		}
		if !errors.Is(err, ErrNotFound) {
			return createAttemptFailure(err)
		}
		return s.reconcileAttempt(ctx, attempt)
	}
	attempt.Correlation = preview.Correlation
	attempt.PreviewDigest = preview.Digest
	attempt.State = CreateAttemptPending
	attempt.ExternalID = ""
	if err := s.store.SaveCreateAttempt(ctx, attempt); err != nil {
		return createAttemptFailure(err)
	}
	attempt, err := s.store.LoadCreateAttempt(ctx, preview.Target)
	if err != nil {
		return createAttemptFailure(err)
	}
	return s.executeCreate(ctx, preview, attempt)
}

func (s Service) executeCreate(ctx context.Context, preview DraftPreview, attempt CreateAttempt) Result {
	external, err := s.capability.Create(ctx, CreateRequest{Resource: preview.Target.Resource, Correlation: preview.Correlation, Document: preview.ProviderDocument})
	if err == nil {
		if !s.capability.ValidExternal(preview.Target.Resource, external.ID, external) {
			return result(completion.Failure, "invalid_provider_response")
		}
		return s.persistAndConfirmAttempt(ctx, attempt, external)
	}
	var provider *ProviderError
	if errors.As(err, &provider) && provider.EffectNotCommitted {
		attempt.State = CreateAttemptRetryAllowed
		if saveErr := s.store.SaveCreateAttempt(ctx, attempt); saveErr != nil {
			return createAttemptFailure(saveErr)
		}
		return providerFailure(err, "provider_create_failed")
	}
	return s.reconcileAttempt(ctx, attempt)
}

func (s Service) reconcileAttempt(ctx context.Context, attempt CreateAttempt) Result {
	preview := DraftPreview{Target: attempt.Target, Correlation: attempt.Correlation}
	reconciled, reconciliation := s.reconcile(ctx, preview)
	if reconciliation.Category != "" {
		return reconciliation
	}
	if reconciled.ID == "" {
		return result(completion.RetryableFailure, "provider_create_ambiguous")
	}
	return s.persistAndConfirmAttempt(ctx, attempt, reconciled)
}

func (s Service) persistAndConfirmAttempt(ctx context.Context, attempt CreateAttempt, external External) Result {
	persisted := s.persistConfirmed(ctx, attempt.Target, external)
	if persisted.Status != completion.Success {
		return persisted
	}
	current, err := s.store.LoadCreateAttempt(ctx, attempt.Target)
	if err != nil {
		persisted.Status = completion.Partial
		persisted.Category = "provider_confirmed_create_attempt_recovery_required"
		return persisted
	}
	current.State = CreateAttemptConfirmed
	current.ExternalID = external.ID
	if err := s.store.SaveCreateAttempt(ctx, current); err != nil {
		persisted.Status = completion.Partial
		persisted.Category = "provider_confirmed_create_attempt_recovery_required"
	}
	return persisted
}

func (s Service) PreviewSelect(ctx context.Context, target Target, selector string) Result {
	project, resolved, failure := s.resolve(ctx, target)
	if failure.Category != "" {
		return failure
	}
	if selector == "" {
		return result(completion.ValidationFailure, "invalid_work_item_input")
	}
	external, err := s.capability.Read(ctx, resolved.Resource, selector)
	if err != nil {
		return providerFailure(err, "provider_read_failed")
	}
	if !s.capability.ValidExternal(resolved.Resource, selector, external) {
		return result(completion.Failure, "invalid_provider_response")
	}
	link, loadErr := s.store.Load(ctx, project.ID, resolved.RepositoryKey, resolved.Provider, resolved.Resource, selector)
	expected := "missing"
	effects := []string{"publish_local_work_item_link"}
	if loadErr == nil {
		expected = hex.EncodeToString(link.Revision[:])
		if equivalentExternal(link, external, resolved.Provider, resolved.Resource) {
			effects = nil
		}
	} else if !errors.Is(loadErr, ErrNotFound) {
		if errors.Is(loadErr, ErrRecoveryRequired) {
			return result(completion.Failure, "recovery_required")
		}
		return result(completion.Failure, "local_work_item_read_failed")
	}
	resolved.ProjectID = project.ID
	preview := SelectionPreview{Target: resolved, External: external, Effects: effects, ExpectedRevisions: ExpectedRevisions{Local: expected}}
	preview.Digest = digestValue(preview)
	return Result{Status: completion.Success, Category: "work_item_selection_ready", Selection: &preview}
}

func (s Service) Select(ctx context.Context, target Target, selector, previewDigest string, authorized bool) Result {
	previewed := s.PreviewSelect(ctx, target, selector)
	if previewed.Status != completion.Success || previewed.Selection == nil {
		return previewed
	}
	if len(previewed.Selection.Effects) == 0 {
		link, _ := s.store.Load(ctx, previewed.Selection.Target.ProjectID, previewed.Selection.Target.RepositoryKey, previewed.Selection.Target.Provider, previewed.Selection.Target.Resource, selector)
		return Result{Status: completion.Success, Category: "work_item_already_linked", Link: link, Selection: previewed.Selection}
	}
	if !authorized || previewDigest == "" || previewDigest != previewed.Selection.Digest {
		previewed.Status = completion.DeniedAuthority
		previewed.Category = "local_authority_denied"
		return previewed
	}
	revision := [32]byte{}
	if previewed.Selection.ExpectedRevisions.Local != "missing" {
		decoded, _ := hex.DecodeString(previewed.Selection.ExpectedRevisions.Local)
		copy(revision[:], decoded)
	}
	link := linkFrom(previewed.Selection.Target, previewed.Selection.External, revision)
	return s.persistLocal(ctx, link, false)
}

func (s Service) Show(ctx context.Context, target Target, selector string) Result {
	project, repositoryKey, failure := s.resolveLinked(ctx, target)
	if failure.Category != "" {
		return failure
	}
	provider := ""
	if target.ProviderResource != "" {
		provider = s.capability.ProviderID()
	}
	link, err := s.store.Load(ctx, project.ID, repositoryKey, provider, target.ProviderResource, selector)
	if err != nil {
		if errors.Is(err, ErrRecoveryRequired) {
			return result(completion.Failure, "recovery_required")
		}
		if errors.Is(err, ErrConflict) {
			return result(completion.Failure, "work_item_ambiguous")
		}
		return result(completion.Failure, "work_item_not_found")
	}
	return Result{Status: completion.Success, Category: "work_item_loaded", Link: link}
}

func (s Service) resolveLinked(ctx context.Context, input Target) (Project, string, Result) {
	if s.resolver == nil || s.store == nil || !s.source.Valid() || input.ProjectSelector == "" || input.RepositoryKey == "" {
		return Project{}, "", result(completion.ValidationFailure, "invalid_work_item_input")
	}
	project, category := s.resolver.Resolve(ctx, input.ProjectSelector)
	if category != "" {
		return Project{}, "", result(completion.ValidationFailure, category)
	}
	if input.ProviderResource != "" && (s.capability == nil || project.Provider != s.capability.ProviderID() || !s.capability.ValidResource(input.ProviderResource)) {
		return Project{}, "", result(completion.ValidationFailure, "invalid_work_item_input")
	}
	for _, repository := range project.Repositories {
		if repository.Key == input.RepositoryKey {
			return project, input.RepositoryKey, Result{}
		}
	}
	return Project{}, "", result(completion.ValidationFailure, "repository_not_configured")
}

// Comment and Complete remain only for historical POC workflow compatibility.
func (s Service) Comment(ctx context.Context, target Target, selector, message string, authorized bool) Result {
	if !authorized {
		return result(completion.DeniedAuthority, "external_mutation_denied")
	}
	shown := s.Show(ctx, target, selector)
	if shown.Status != completion.Success || s.legacy == nil {
		return shown
	}
	if err := s.legacy.Comment(ctx, shown.Link.Resource, selector, message); err != nil {
		return result(completion.Failure, "github_mutation_failed")
	}
	shown.Category = "work_item_commented"
	return shown
}

func (s Service) Complete(ctx context.Context, target Target, selector string, authorized bool) Result {
	if !authorized {
		return result(completion.DeniedAuthority, "external_mutation_denied")
	}
	shown := s.Show(ctx, target, selector)
	if shown.Status != completion.Success || s.legacy == nil {
		return shown
	}
	external, err := s.legacy.Close(ctx, shown.Link.Resource, selector)
	if err != nil {
		return result(completion.Failure, "github_mutation_failed")
	}
	link := linkFrom(DraftTarget{ProjectID: shown.Link.ProjectID, RepositoryKey: shown.Link.RepositoryKey, Provider: shown.Link.Provider, Resource: shown.Link.Resource}, external, shown.Link.Revision)
	persisted := s.persistLocal(ctx, link, true)
	if persisted.Status == completion.Success {
		persisted.Category = "work_item_completed"
	}
	return persisted
}

func (s Service) resolve(ctx context.Context, input Target) (Project, DraftTarget, Result) {
	if s.resolver == nil || s.store == nil || !s.source.Valid() || input.ProjectSelector == "" || input.RepositoryKey == "" || input.ProviderResource == "" {
		return Project{}, DraftTarget{}, result(completion.ValidationFailure, "invalid_work_item_input")
	}
	project, category := s.resolver.Resolve(ctx, input.ProjectSelector)
	if category != "" {
		return Project{}, DraftTarget{}, result(completion.ValidationFailure, category)
	}
	if s.capability == nil || project.Provider != s.capability.ProviderID() {
		return Project{}, DraftTarget{}, result(completion.ValidationFailure, "work_item_capability_unavailable")
	}
	if !s.capability.ValidResource(input.ProviderResource) {
		return Project{}, DraftTarget{}, result(completion.ValidationFailure, "invalid_provider_target")
	}
	for _, repository := range project.Repositories {
		if repository.Key == input.RepositoryKey {
			return project, DraftTarget{RepositoryKey: input.RepositoryKey, Provider: project.Provider, Resource: input.ProviderResource}, Result{}
		}
	}
	return Project{}, DraftTarget{}, result(completion.ValidationFailure, "repository_not_configured")
}

func (s Service) preview(draft Draft, target DraftTarget) (DraftPreview, error) {
	effects := []string{"publish_local_create_attempt_fence", "create_provider_work_item", "publish_local_work_item_link"}
	semantic := struct {
		Target     DraftTarget
		Draft      Draft
		Effects    []string
		Expected   ExpectedRevisions
		Provenance string
	}{target, draft, effects, ExpectedRevisions{Local: "missing"}, provenanceIdentity(s.source)}
	correlation := digestValue(semantic)
	document, err := s.capability.Render(draft, target, correlation, s.source)
	if err != nil {
		return DraftPreview{}, err
	}
	preview := DraftPreview{Target: target, Draft: draft, ProviderDocument: document, Effects: effects, ExpectedRevisions: ExpectedRevisions{Local: "missing"}, Correlation: correlation}
	preview.Digest = digestValue(preview)
	return preview, nil
}

func (s Service) reconcile(ctx context.Context, preview DraftPreview) (External, Result) {
	items, err := s.capability.ReconcileCreate(ctx, preview.Target.Resource, preview.Correlation)
	if err != nil {
		return External{}, providerFailure(err, "provider_reconciliation_failed")
	}
	if len(items) == 0 {
		return External{}, Result{}
	}
	if len(items) != 1 || !s.capability.ValidExternal(preview.Target.Resource, items[0].ID, items[0]) {
		return External{}, result(completion.Failure, "provider_reconciliation_ambiguous")
	}
	return items[0], Result{}
}

func (s Service) persistConfirmed(ctx context.Context, target DraftTarget, external External) Result {
	link := linkFrom(target, external, [32]byte{})
	if existing, err := s.store.Load(ctx, target.ProjectID, target.RepositoryKey, target.Provider, target.Resource, external.ID); err == nil {
		if equivalentExternal(existing, external, target.Provider, target.Resource) {
			return Result{Status: completion.Success, Category: "work_item_already_linked", Link: existing}
		}
		return Result{Status: completion.Partial, Category: "provider_confirmed_local_conflict", Link: link}
	} else if !errors.Is(err, ErrNotFound) {
		if errors.Is(err, ErrRecoveryRequired) {
			return Result{Status: completion.Partial, Category: "provider_confirmed_local_recovery_required", Link: link}
		}
		return Result{Status: completion.Partial, Category: "provider_confirmed_local_read_failed", Link: link}
	}
	return s.persistLocal(ctx, link, true)
}

func (s Service) persistLocal(ctx context.Context, link Link, providerConfirmed bool) Result {
	err := s.store.Save(ctx, link)
	if err == nil {
		return Result{Status: completion.Success, Category: "work_item_linked", Link: link}
	}
	status := completion.Failure
	category := "local_work_item_write_failed"
	if providerConfirmed {
		status = completion.Partial
		category = "provider_confirmed_local_failed"
	}
	if errors.Is(err, ErrConflict) {
		category = "local_work_item_conflict"
		if providerConfirmed {
			category = "provider_confirmed_local_conflict"
		}
	}
	if errors.Is(err, ErrRecoveryRequired) {
		category = "local_work_item_recovery_required"
		if providerConfirmed {
			category = "provider_confirmed_local_recovery_required"
		}
	}
	if committed(err) {
		status = completion.Partial
		category = "local_link_committed_recovery_required"
	}
	return Result{Status: status, Category: category, Link: link}
}

func normalizeDraft(input DraftInput) (Draft, []Question, string) {
	fields := []struct {
		name, prompt string
		input        SectionInput
	}{
		{"problem", "What problem should this Work Item solve?", input.Problem},
		{"desired_outcome", "What outcome should be observable?", input.DesiredOutcome},
		{"context", "What context materially changes this work?", input.Context},
		{"scope", "What is in scope?", input.Scope},
		{"constraints", "What constraints must be preserved?", input.Constraints},
		{"non_goals", "What is explicitly out of scope?", input.NonGoals},
		{"acceptance_expectations", "What evidence would demonstrate completion?", input.Acceptance},
	}
	if fields[0].input.Supplied == "" && fields[0].input.Elaborated == "" {
		fields[0].input.Supplied = input.Intent
	}
	draft := Draft{Sections: make([]DraftSection, 0, len(fields))}
	questions := make([]Question, 0)
	total := 0
	for _, field := range fields {
		content := field.input.Supplied
		authorship := provenance.UserAuthored
		if content == "" {
			content = field.input.Elaborated
			authorship = provenance.AxiomAuthored
		}
		if content == "" {
			questions = append(questions, Question{Field: field.name, Prompt: field.prompt})
			continue
		}
		if !validDraftText(content) {
			return Draft{}, nil, "invalid_draft_input"
		}
		if looksSensitive(content) {
			return Draft{}, nil, "secret_rejected"
		}
		total += len(content)
		draft.Sections = append(draft.Sections, DraftSection{Name: field.name, Content: content, Authorship: authorship})
	}
	if total > maxDraftBytes {
		return Draft{}, nil, "draft_too_large"
	}
	return draft, questions, ""
}

func validDraftText(value string) bool {
	if value == "" || len(value) > maxFieldBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, current := range value {
		if current == '\x00' || current == '\x7f' || (unicode.IsControl(current) && current != '\n' && current != '\t') {
			return false
		}
	}
	return true
}

func looksSensitive(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"-----begin private key-----", "github_pat_", "ghp_", "gho_", "ghu_", "ghs_", "ghr_"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, name := range []string{"token", "access_token", "refresh_token", "password", "passwd", "pwd", "api_key", "apikey", "client_secret", "authorization", "credential", "secret"} {
		for _, separator := range []string{"=", ":"} {
			if strings.Contains(lower, name+separator) || strings.Contains(lower, name+" "+separator) {
				return true
			}
		}
	}
	return false
}

func providerFailure(err error, fallback string) Result {
	var provider *ProviderError
	if !errors.As(err, &provider) {
		return result(completion.Failure, fallback)
	}
	if provider.Retryable {
		return result(completion.RetryableFailure, string(provider.Kind))
	}
	return result(completion.Failure, string(provider.Kind))
}

func createAttemptFailure(err error) Result {
	if errors.Is(err, ErrRecoveryRequired) {
		return result(completion.Failure, "local_create_attempt_recovery_required")
	}
	if errors.Is(err, ErrConflict) {
		return result(completion.Failure, "local_create_attempt_conflict")
	}
	return result(completion.Failure, "local_create_attempt_failed")
}

func result(status completion.Status, category string) Result {
	return Result{Status: status, Category: category}
}

func digestValue(value any) string {
	wire, _ := json.Marshal(value)
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:])
}

func provenanceIdentity(value provenance.Value) string {
	return strings.Join([]string{value.Product(), value.Version(), value.Revision(), string(value.SourceState())}, ":")
}

func linkFrom(target DraftTarget, external External, revision [32]byte) Link {
	return Link{ProjectID: target.ProjectID, RepositoryKey: target.RepositoryKey, Provider: target.Provider, Resource: target.Resource, ExternalID: external.ID, URL: external.URL, State: external.State, Revision: revision}
}

func equivalentExternal(link Link, external External, provider, resource string) bool {
	return link.Provider == provider && link.Resource == resource && link.ExternalID == external.ID && link.URL == external.URL && link.State == external.State
}

var providerToken = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func ValidProviderID(value string) bool {
	return value != "." && value != ".." && len(value) <= 64 && providerToken.MatchString(value)
}

func ValidExternalID(value string) bool {
	return value != "." && value != ".." && len(value) <= 128 && providerToken.MatchString(value)
}

func committed(err error) bool {
	type committedEffect interface{ EffectCommitted() bool }
	var effect committedEffect
	return errors.As(err, &effect) && effect.EffectCommitted()
}
