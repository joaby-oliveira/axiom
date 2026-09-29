// Package workflow owns the bounded sequential MVP Execution and projection plan.
package workflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/provenance"
)

const (
	FormatVersion   = 1
	WorkflowVersion = "mvp-v1-sequential"
	maxEvents       = 32
	maxReferences   = 16
	maxTextBytes    = 512
	stagePrefix     = "axiom:stage:"
)

var (
	ErrNotFound         = errors.New("execution not found")
	ErrConflict         = errors.New("execution conflict")
	ErrRecoveryRequired = errors.New("execution recovery required")
)

type Stage string

const (
	Intake         Stage = "intake"
	Specification  Stage = "specification"
	Clarification  Stage = "clarification"
	Plan           Stage = "plan"
	Tasks          Stage = "tasks"
	Implementation Stage = "implementation"
	Review         Stage = "review"
	Evidence       Stage = "evidence"
	Reconciliation Stage = "reconciliation"
	Completion     Stage = "completion"
)

var stages = [...]Stage{Intake, Specification, Clarification, Plan, Tasks, Implementation, Review, Evidence, Reconciliation, Completion}

func Stages() []Stage       { result := make([]Stage, len(stages)); copy(result, stages[:]); return result }
func (s Stage) Valid() bool { return stageIndex(s) >= 0 }
func stageIndex(stage Stage) int {
	for index, candidate := range stages {
		if stage == candidate {
			return index
		}
	}
	return -1
}

type Outcome string

const (
	OutcomePassed  Outcome = "pass"
	OutcomeFailed  Outcome = "fail"
	OutcomeResumed Outcome = "resumed"
	OutcomeFact    Outcome = "fact"
)

type ExecutionStatus string

const (
	ExecutionActive      ExecutionStatus = "active"
	ExecutionInterrupted ExecutionStatus = "interrupted"
	ExecutionCompleted   ExecutionStatus = "completed"
)

type Repository struct{ Key, Path string }
type Project struct {
	ID           string
	Repositories []Repository
}
type Resolver interface {
	Resolve(context.Context, string) (Project, string)
}

type WorkItem struct {
	Provider, Resource, ExternalID string
	URL, State                     string
}
type WorkItems interface {
	Load(context.Context, string, string, string, string, string) (WorkItem, error)
}

type Identity struct{ Product, Version, Revision, SourceState string }
type Reference struct{ Kind, ID, Digest string }
type Transition struct {
	Revision      uint64
	From, To      Stage
	Outcome       Outcome
	RequestDigest string
	References    []Reference
	Next          string
	CommittedAt   time.Time
	Provenance    Identity
	Fact          *LifecycleFact `json:"fact,omitempty"`
}
type Terminal struct {
	Status           completion.Status
	ConfirmedEffects []string
}
type ProjectionRecord struct {
	ExecutionRevision   uint64
	Key                 string
	Intended, Confirmed []ProjectionEffect
	Complete            bool
}
type State struct {
	ExecutionID              string
	FormatVersion            int
	WorkflowVersion          string
	ProjectID, RepositoryKey string
	WorkItem                 WorkItem
	RuntimeID                string
	Stage                    Stage
	Revision                 uint64
	Status                   ExecutionStatus
	Transitions              []Transition
	CreatedAt, UpdatedAt     time.Time
	Provenance               Identity
	Terminal                 *Terminal
	Projections              []ProjectionRecord
	StorageRevision          [sha256.Size]byte
}
type Store interface {
	Create(context.Context, State) error
	Load(context.Context, string, string, WorkItem) (State, error)
	Save(context.Context, State) error
}

type ReferenceValidator interface {
	Validate(context.Context, string, string, Reference) error
}

type ProjectionEffectKind string

const (
	CreateStageLabel      ProjectionEffectKind = "create_stage_label"
	AddStageLabel         ProjectionEffectKind = "add_stage_label"
	RemoveStageLabel      ProjectionEffectKind = "remove_stage_label"
	PostTransitionComment ProjectionEffectKind = "post_transition_comment"
)

type ProjectionEffect struct {
	Kind  ProjectionEffectKind `json:"kind"`
	Value string               `json:"value"`
}
type ProjectionObservation struct {
	Provider         string   `json:"provider"`
	Resource         string   `json:"resource"`
	IssueExternalID  string   `json:"issueExternalId"`
	IssueURL         string   `json:"issueUrl"`
	IssueState       string   `json:"issueState"`
	RepositoryLabels []string `json:"repositoryLabels"`
	IssueLabels      []string `json:"issueLabels"`
	CommentPresent   bool     `json:"commentPresent"`
}
type ProjectionCapability interface {
	Inspect(context.Context, WorkItem, string, string) (ProjectionObservation, error)
	Apply(context.Context, WorkItem, ProjectionEffect) error
}
type ProjectionErrorKind string

const (
	ProjectionUnavailable     ProjectionErrorKind = "provider_unavailable"
	ProjectionUnauthenticated ProjectionErrorKind = "provider_unauthenticated"
	ProjectionRateLimited     ProjectionErrorKind = "provider_rate_limited"
	ProjectionInvalidResponse ProjectionErrorKind = "provider_invalid_response"
	ProjectionAmbiguous       ProjectionErrorKind = "provider_ambiguous"
)

type ProjectionError struct {
	Kind                                     ProjectionErrorKind
	Retryable, Ambiguous, EffectNotCommitted bool
}

func (e *ProjectionError) Error() string { return string(e.Kind) }

type ProjectionPreview struct {
	ExecutionID       string                 `json:"executionId"`
	ExecutionRevision uint64                 `json:"executionRevision"`
	ProjectionKey     string                 `json:"projectionKey"`
	Stage             Stage                  `json:"stage"`
	LifecycleStage    WorkItemLifecycleStage `json:"lifecycleStage"`
	Label             string                 `json:"label"`
	Comment           string                 `json:"comment"`
	Effects           []ProjectionEffect     `json:"effects"`
	Observation       ProjectionObservation  `json:"observation"`
	ObservationDigest string                 `json:"observationDigest"`
	Digest            string                 `json:"digest"`
}

type Status = completion.Status

const (
	Succeeded        = completion.Success
	Failed           = completion.Failure
	ValidationFailed = completion.ValidationFailure
	Denied           = completion.DeniedAuthority
	Partial          = completion.Partial
	Interrupted      = completion.Interrupted
	Retryable        = completion.RetryableFailure
)

type Result struct {
	Status   Status
	Category string
	State    State
	Preview  *ProjectionPreview
}
type Target struct {
	ProjectSelector, RepositoryKey               string
	WorkItemProvider, WorkItemResource, WorkItem string
	ExecutionID, RuntimeID                       string
}
type TransitionInput struct {
	ExpectedRevision uint64
	Stage            Stage
	Outcome          Outcome
	References       []Reference
	Next             string
}
type LifecycleFactInput struct {
	ExpectedRevision uint64
	Kind             LifecycleFactKind
	Active           bool
	Reference        Reference
}
type IDAllocator func() (string, error)
type Clock func() time.Time

type Service struct {
	resolver   Resolver
	workItems  WorkItems
	store      Store
	projection ProjectionCapability
	references ReferenceValidator
	source     provenance.Value
	allocateID IDAllocator
	now        Clock
}

func New(resolver Resolver, workItems WorkItems, store Store, projection ProjectionCapability, references ReferenceValidator, source provenance.Value, allocateID IDAllocator, now Clock) Service {
	if allocateID == nil {
		allocateID = randomID
	}
	if now == nil {
		now = time.Now
	}
	return Service{resolver: resolver, workItems: workItems, store: store, projection: projection, references: references, source: source, allocateID: allocateID, now: now}
}

func (s Service) Start(ctx context.Context, target Target) Result {
	if err := ctx.Err(); err != nil {
		return result(Interrupted, "workflow_cancelled", State{})
	}
	if target.ExecutionID != "" {
		return result(ValidationFailed, "execution_selector_conflict", State{})
	}
	project, repository, item, failed := s.resolve(ctx, target)
	if failed.Category != "" {
		return failed
	}
	existing, err := s.store.Load(ctx, project.ID, repository.Key, item)
	if err == nil {
		if equivalentStart(existing, project.ID, repository.Key, item, target.RuntimeID) {
			return result(Succeeded, "execution_already_started", existing)
		}
		return result(ValidationFailed, "execution_scope_conflict", existing)
	}
	if !errors.Is(err, ErrNotFound) {
		return storeFailure(err)
	}
	id, err := s.allocateID()
	if err != nil || !validOpaqueID(id) {
		return result(Failed, "execution_identity_failed", State{})
	}
	now := s.now().UTC()
	state := State{ExecutionID: id, FormatVersion: FormatVersion, WorkflowVersion: WorkflowVersion, ProjectID: project.ID, RepositoryKey: repository.Key, WorkItem: item, RuntimeID: target.RuntimeID, Stage: Intake, Revision: 1, Status: ExecutionActive, CreatedAt: now, UpdatedAt: now, Provenance: identity(s.source)}
	if !ValidState(state) {
		return result(ValidationFailed, "invalid_execution_state", State{})
	}
	if err := s.store.Create(ctx, state); err != nil {
		var committed interface{ EffectCommitted() bool }
		if errors.As(err, &committed) && committed.EffectCommitted() {
			existing, loadErr := s.store.Load(ctx, project.ID, repository.Key, item)
			if loadErr == nil && equivalentStart(existing, project.ID, repository.Key, item, target.RuntimeID) {
				return result(Succeeded, "execution_started", existing)
			}
		}
		if errors.Is(err, ErrConflict) {
			existing, loadErr := s.store.Load(ctx, project.ID, repository.Key, item)
			if loadErr == nil && equivalentStart(existing, project.ID, repository.Key, item, target.RuntimeID) {
				return result(Succeeded, "execution_already_started", existing)
			}
		}
		return storeFailure(err)
	}
	loaded, err := s.store.Load(ctx, project.ID, repository.Key, item)
	if err != nil {
		return storeFailure(err)
	}
	return result(Succeeded, "execution_started", loaded)
}

func (s Service) Status(ctx context.Context, target Target) Result {
	if err := ctx.Err(); err != nil {
		return result(Interrupted, "workflow_cancelled", State{})
	}
	state, failed := s.load(ctx, target)
	if failed.Category != "" {
		return failed
	}
	if _, err := DeriveLifecycle(state); err != nil {
		return result(Failed, "recovery_required", state)
	}
	return result(Succeeded, "execution_"+string(state.Status), state)
}

func (s Service) Resume(ctx context.Context, target Target, expectedRevision uint64) Result {
	if err := ctx.Err(); err != nil {
		return result(Interrupted, "workflow_cancelled", State{})
	}
	state, failed := s.load(ctx, target)
	if failed.Category != "" {
		return failed
	}
	if state.Revision != expectedRevision {
		return result(Denied, "stale_execution_revision", state)
	}
	if state.Status != ExecutionInterrupted {
		return result(Succeeded, "execution_not_interrupted", state)
	}
	input := TransitionInput{ExpectedRevision: expectedRevision, Stage: state.Stage, Outcome: OutcomeResumed}
	event := transitionFor(state, input, state.Stage, s.now().UTC(), s.source)
	state.Revision++
	state.Status = ExecutionActive
	state.UpdatedAt = event.CommittedAt
	state.Transitions = append(state.Transitions, event)
	return s.save(ctx, state, Succeeded, "execution_resumed")
}

func (s Service) Transition(ctx context.Context, target Target, input TransitionInput) Result {
	if err := ctx.Err(); err != nil {
		return result(Interrupted, "workflow_cancelled", State{})
	}
	state, repository, failed := s.loadResolved(ctx, target)
	if failed.Category != "" {
		return failed
	}
	requestDigest, valid := transitionDigest(input)
	if !valid {
		return result(ValidationFailed, "invalid_workflow_transition", state)
	}
	if state.Revision != input.ExpectedRevision {
		if replayed(state, input.ExpectedRevision, requestDigest) {
			return result(Succeeded, "workflow_transition_replayed", state)
		}
		return result(Denied, "stale_execution_revision", state)
	}
	if state.Status == ExecutionCompleted {
		return result(ValidationFailed, "execution_already_completed", state)
	}
	if state.Status == ExecutionInterrupted {
		return result(ValidationFailed, "execution_resume_required", state)
	}
	lifecycle, lifecycleErr := DeriveLifecycle(state)
	if lifecycleErr != nil {
		return result(Failed, "recovery_required", state)
	}
	if lifecycle.Conditions.Blocked && input.Outcome == OutcomePassed {
		return result(Denied, "workflow_blocked", state)
	}
	if input.Stage != state.Stage {
		return result(ValidationFailed, "workflow_stage_conflict", state)
	}
	for _, reference := range input.References {
		if s.references == nil || s.references.Validate(ctx, state.ExecutionID, repository.Path, reference) != nil {
			return result(ValidationFailed, "workflow_reference_unavailable", state)
		}
	}
	next := state.Stage
	status := Interrupted
	category := "workflow_interrupted"
	state.Status = ExecutionInterrupted
	if input.Outcome == OutcomePassed {
		status, category = Succeeded, "workflow_advanced"
		index := stageIndex(state.Stage)
		if index == len(stages)-1 {
			state.Status = ExecutionCompleted
			category = "workflow_completed"
			state.Terminal = &Terminal{Status: completion.Success, ConfirmedEffects: []string{"local_execution_transition"}}
		} else {
			next = stages[index+1]
			state.Status = ExecutionActive
		}
	}
	event := transitionFor(state, input, next, s.now().UTC(), s.source)
	event.RequestDigest = requestDigest
	state.Revision++
	state.Stage = next
	state.UpdatedAt = event.CommittedAt
	state.Transitions = append(state.Transitions, event)
	if _, err := DeriveLifecycle(state); err != nil {
		return result(Failed, "recovery_required", cloneState(stateWithoutLastTransition(state)))
	}
	return s.save(ctx, state, status, category)
}

func stateWithoutLastTransition(state State) State {
	if len(state.Transitions) == 0 {
		return state
	}
	last := state.Transitions[len(state.Transitions)-1]
	state.Transitions = state.Transitions[:len(state.Transitions)-1]
	state.Revision--
	state.Stage = last.From
	state.UpdatedAt = state.CreatedAt
	if len(state.Transitions) != 0 {
		state.UpdatedAt = state.Transitions[len(state.Transitions)-1].CommittedAt
	}
	state.Status = ExecutionActive
	state.Terminal = nil
	return state
}

// RecordLifecycleFact appends one revisioned local fact without changing the
// canonical gate. Provider observations and technical signals cannot call this
// operation implicitly; exact caller authority remains mandatory.
func (s Service) RecordLifecycleFact(ctx context.Context, target Target, input LifecycleFactInput, authorized bool) Result {
	if err := ctx.Err(); err != nil {
		return result(Interrupted, "workflow_cancelled", State{})
	}
	state, repository, failed := s.loadResolved(ctx, target)
	if failed.Category != "" {
		return failed
	}
	if state.Revision != input.ExpectedRevision {
		return result(Denied, "stale_execution_revision", state)
	}
	if !authorized {
		return result(Denied, "lifecycle_fact_authority_denied", state)
	}
	if !factAllowedAt(input.Kind, state.Stage) || !validReference(input.Reference) || state.Status == ExecutionInterrupted {
		return result(ValidationFailed, "invalid_lifecycle_fact", state)
	}
	if s.references == nil || s.references.Validate(ctx, state.ExecutionID, repository.Path, input.Reference) != nil {
		return result(ValidationFailed, "lifecycle_fact_reference_unavailable", state)
	}
	fact := LifecycleFact{Kind: input.Kind, Active: input.Active, ScopeDigest: executionScopeDigest(state), Reference: input.Reference}
	previous := cloneState(state)
	transitionInput := TransitionInput{ExpectedRevision: input.ExpectedRevision, Stage: state.Stage, Outcome: OutcomeFact, References: []Reference{input.Reference}}
	event := transitionFor(state, transitionInput, state.Stage, s.now().UTC(), s.source)
	event.RequestDigest = digest(input)
	event.Fact = &fact
	state.Revision++
	state.UpdatedAt = event.CommittedAt
	state.Transitions = append(state.Transitions, event)
	if _, err := DeriveLifecycle(state); err != nil {
		return result(ValidationFailed, "inconsistent_lifecycle_fact", previous)
	}
	return s.save(ctx, state, Succeeded, "lifecycle_fact_recorded")
}

func (s Service) PrepareProjection(ctx context.Context, target Target, expectedRevision uint64) Result {
	prepared, _ := s.prepareProjection(ctx, target, expectedRevision)
	return prepared
}

func (s Service) prepareProjection(ctx context.Context, target Target, expectedRevision uint64) (Result, ProjectionObservation) {
	if err := ctx.Err(); err != nil {
		return result(Interrupted, "workflow_cancelled", State{}), ProjectionObservation{}
	}
	state, failed := s.load(ctx, target)
	if failed.Category != "" {
		return failed, ProjectionObservation{}
	}
	if state.Revision != expectedRevision {
		return result(Denied, "stale_execution_revision", state), ProjectionObservation{}
	}
	if len(state.Transitions) == 0 || state.Transitions[len(state.Transitions)-1].Revision != expectedRevision {
		return result(ValidationFailed, "projection_transition_unavailable", state), ProjectionObservation{}
	}
	if s.projection == nil {
		return result(ValidationFailed, "projection_capability_unavailable", state), ProjectionObservation{}
	}
	preview, observation, err := s.projectionPreview(ctx, state)
	if err != nil {
		return projectionFailure(err, state, false), ProjectionObservation{}
	}
	return Result{Status: Succeeded, Category: "projection_preview_ready", State: state, Preview: &preview}, observation
}

func (s Service) Project(ctx context.Context, target Target, expectedRevision uint64, previewDigest string, authorized bool) Result {
	prepared, observation := s.prepareProjection(ctx, target, expectedRevision)
	if prepared.Status != Succeeded || prepared.Preview == nil {
		return prepared
	}
	if !authorized || previewDigest == "" {
		prepared.Status, prepared.Category = Denied, "projection_authority_denied"
		return prepared
	}
	state, found, changed := reconcileProjectionRecord(prepared.State, expectedRevision, observation)
	if changed {
		saved := s.save(ctx, state, Succeeded, "projection_effect_reconciled")
		if saved.Status != Succeeded {
			return result(Partial, "provider_confirmed_projection_bookkeeping_failed", state)
		}
		prepared.State = saved.State
	}
	if found && projectionComplete(prepared.State, expectedRevision) && len(prepared.Preview.Effects) == 0 {
		prepared.Category = "projection_converged"
		return prepared
	}
	if previewDigest != prepared.Preview.Digest {
		prepared.Status, prepared.Category = Denied, "projection_authority_denied"
		return prepared
	}
	if len(prepared.Preview.Effects) == 0 {
		prepared.Category = "projection_already_converged"
		return prepared
	}
	state = upsertProjection(prepared.State, projectionRecord(prepared.State, *prepared.Preview))
	saved := s.save(ctx, state, Succeeded, "projection_intent_recorded")
	if saved.Status != Succeeded {
		return saved
	}
	state = saved.State
	confirmedAny := false
	for _, effect := range prepared.Preview.Effects {
		err := s.projection.Apply(ctx, state.WorkItem, effect)
		if err != nil {
			var provider *ProjectionError
			if !errors.As(err, &provider) || !provider.Ambiguous {
				return projectionFailure(err, state, confirmedAny)
			}
		}
		observation, inspectErr := s.projection.Inspect(ctx, state.WorkItem, prepared.Preview.Label, prepared.Preview.ProjectionKey)
		if inspectErr != nil {
			return projectionFailure(errors.Join(err, inspectErr), state, confirmedAny)
		}
		if !validProjectionObservation(state.WorkItem, observation) {
			return projectionFailure(&ProjectionError{Kind: ProjectionInvalidResponse}, state, confirmedAny)
		}
		if !effectObserved(effect, observation) {
			return projectionFailure(&ProjectionError{Kind: ProjectionInvalidResponse, Retryable: true, Ambiguous: true}, state, confirmedAny)
		}
		confirmedAny = true
		state = confirmProjectionEffect(state, expectedRevision, effect)
		saved = s.save(ctx, state, Succeeded, "projection_effect_recorded")
		if saved.Status != Succeeded {
			return result(Partial, "provider_confirmed_projection_bookkeeping_failed", state)
		}
		state = saved.State
	}
	return Result{Status: Succeeded, Category: "projection_converged", State: state, Preview: prepared.Preview}
}

func (s Service) resolve(ctx context.Context, target Target) (Project, Repository, WorkItem, Result) {
	if s.resolver == nil || s.workItems == nil || s.store == nil || !s.source.Valid() || !validText(target.ProjectSelector) || !validText(target.RepositoryKey) || !validText(target.WorkItem) || !validText(target.RuntimeID) {
		return Project{}, Repository{}, WorkItem{}, result(ValidationFailed, "invalid_execution_input", State{})
	}
	project, category := s.resolver.Resolve(ctx, target.ProjectSelector)
	if category != "" {
		return Project{}, Repository{}, WorkItem{}, result(ValidationFailed, category, State{})
	}
	var repository Repository
	for _, candidate := range project.Repositories {
		if candidate.Key == target.RepositoryKey {
			repository = candidate
			break
		}
	}
	if repository.Key == "" {
		return Project{}, Repository{}, WorkItem{}, result(ValidationFailed, "repository_not_configured", State{})
	}
	item, err := s.workItems.Load(ctx, target.ProjectSelector, target.RepositoryKey, target.WorkItemProvider, target.WorkItemResource, target.WorkItem)
	if err != nil || !validWorkItem(item) {
		return Project{}, Repository{}, WorkItem{}, result(ValidationFailed, "work_item_not_linked", State{})
	}
	return project, repository, item, Result{}
}

func (s Service) load(ctx context.Context, target Target) (State, Result) {
	state, _, failed := s.loadResolved(ctx, target)
	return state, failed
}

func (s Service) loadResolved(ctx context.Context, target Target) (State, Repository, Result) {
	project, repository, item, failed := s.resolve(ctx, target)
	if failed.Category != "" {
		return State{}, Repository{}, failed
	}
	state, err := s.store.Load(ctx, project.ID, repository.Key, item)
	if err != nil {
		return State{}, Repository{}, storeFailure(err)
	}
	if !ValidState(state) {
		return State{}, Repository{}, result(ValidationFailed, "invalid_execution_state", State{})
	}
	if !equivalentStart(state, project.ID, repository.Key, item, target.RuntimeID) {
		return State{}, Repository{}, result(ValidationFailed, "execution_scope_conflict", state)
	}
	if target.ExecutionID != "" && state.ExecutionID != target.ExecutionID {
		return State{}, Repository{}, result(ValidationFailed, "execution_selector_conflict", State{})
	}
	return state, repository, Result{}
}

func (s Service) save(ctx context.Context, state State, status Status, category string) Result {
	if !ValidState(state) {
		return result(ValidationFailed, "invalid_execution_state", State{})
	}
	err := s.store.Save(ctx, state)
	if err != nil {
		var committed interface{ EffectCommitted() bool }
		if errors.As(err, &committed) && committed.EffectCommitted() {
			loaded, loadErr := s.store.Load(ctx, state.ProjectID, state.RepositoryKey, state.WorkItem)
			if loadErr == nil && equivalentPersistedState(loaded, state) {
				return result(status, category, loaded)
			}
		}
		return storeFailure(err)
	}
	loaded, err := s.store.Load(ctx, state.ProjectID, state.RepositoryKey, state.WorkItem)
	if err != nil {
		return storeFailure(err)
	}
	return result(status, category, loaded)
}

func (s Service) projectionPreview(ctx context.Context, state State) (ProjectionPreview, ProjectionObservation, error) {
	lifecycle, lifecycleErr := DeriveLifecycle(state)
	if lifecycleErr != nil {
		return ProjectionPreview{}, ProjectionObservation{}, ErrRecoveryRequired
	}
	label := lifecycleLabel(lifecycle.Stage)
	key := projectionKey(state.ExecutionID, state.Revision)
	observation, err := s.projection.Inspect(ctx, state.WorkItem, label, key)
	if err != nil {
		return ProjectionPreview{}, ProjectionObservation{}, err
	}
	preview, err := prepareProjectionPreview(state, lifecycle, observation)
	return preview, preview.Observation, err
}

func prepareProjectionPreview(state State, lifecycle LifecycleProjection, observation ProjectionObservation) (ProjectionPreview, error) {
	if !validProjectionObservation(state.WorkItem, observation) {
		return ProjectionPreview{}, &ProjectionError{Kind: ProjectionInvalidResponse}
	}
	observation.RepositoryLabels = sorted(observation.RepositoryLabels)
	observation.IssueLabels = sorted(observation.IssueLabels)
	observedStage, legacyStage, err := observedLifecycleStage(observation.IssueLabels)
	if err != nil {
		return ProjectionPreview{}, err
	}
	label := lifecycleLabel(lifecycle.Stage)
	key := projectionKey(state.ExecutionID, state.Revision)
	comment := transitionComment(state, key)
	if comment == "" {
		return ProjectionPreview{}, ErrRecoveryRequired
	}
	desired := append([]string{label}, lifecycleFlags(lifecycle.Conditions)...)
	effects := make([]ProjectionEffect, 0, 12)
	for _, wanted := range desired {
		if !contains(observation.RepositoryLabels, wanted) {
			effects = append(effects, ProjectionEffect{Kind: CreateStageLabel, Value: wanted})
		}
		if !contains(observation.IssueLabels, wanted) {
			effects = append(effects, ProjectionEffect{Kind: AddStageLabel, Value: wanted})
		}
	}
	if observedStage != label || legacyStage {
		effects = append(effects, ProjectionEffect{Kind: RemoveStageLabel, Value: observedStage})
	}
	for _, current := range observation.IssueLabels {
		if validLifecycleFlag(current) && !contains(desired, current) {
			effects = append(effects, ProjectionEffect{Kind: RemoveStageLabel, Value: current})
		}
	}
	if !observation.CommentPresent {
		effects = append(effects, ProjectionEffect{Kind: PostTransitionComment, Value: comment})
	}
	if len(effects) > 16 {
		return ProjectionPreview{}, ErrRecoveryRequired
	}
	for _, effect := range effects {
		if !validEffect(effect) {
			return ProjectionPreview{}, ErrRecoveryRequired
		}
	}
	observationDigest := digest(observation)
	preview := ProjectionPreview{ExecutionID: state.ExecutionID, ExecutionRevision: state.Revision, ProjectionKey: key, Stage: state.Stage, LifecycleStage: lifecycle.Stage, Label: label, Comment: comment, Effects: effects, Observation: observation, ObservationDigest: observationDigest}
	preview.Digest = digest(preview)
	return preview, nil
}

func reconcileProjectionRecord(state State, revision uint64, observation ProjectionObservation) (State, bool, bool) {
	for index := range state.Projections {
		record := &state.Projections[index]
		if record.ExecutionRevision != revision {
			continue
		}
		changed := false
		for _, effect := range record.Intended {
			if containsEffect(record.Confirmed, effect) || !effectObserved(effect, observation) {
				continue
			}
			record.Confirmed = append(record.Confirmed, effect)
			changed = true
		}
		complete := effectsConfirmed(record.Intended, record.Confirmed)
		if record.Complete != complete {
			record.Complete = complete
			changed = true
		}
		return state, true, changed
	}
	return state, false, false
}

func projectionComplete(state State, revision uint64) bool {
	for _, record := range state.Projections {
		if record.ExecutionRevision == revision {
			return record.Complete
		}
	}
	return false
}

func validProjectionObservation(item WorkItem, observation ProjectionObservation) bool {
	if observation.Provider != item.Provider || observation.Resource != item.Resource || observation.IssueExternalID != item.ExternalID || observation.IssueURL != item.URL || observation.IssueState != "OPEN" && observation.IssueState != "CLOSED" || len(observation.RepositoryLabels) > 100 || len(observation.IssueLabels) > 100 {
		return false
	}
	for _, labels := range [][]string{observation.RepositoryLabels, observation.IssueLabels} {
		seen := make(map[string]struct{}, len(labels))
		for _, label := range labels {
			if label == "" || len(label) > 256 {
				return false
			}
			if _, exists := seen[label]; exists {
				return false
			}
			seen[label] = struct{}{}
		}
	}
	return true
}

func projectionRecord(state State, preview ProjectionPreview) ProjectionRecord {
	for _, current := range state.Projections {
		if current.ExecutionRevision == preview.ExecutionRevision {
			for _, effect := range preview.Effects {
				if !containsEffect(current.Intended, effect) {
					current.Intended = append(current.Intended, effect)
				}
			}
			current.Complete = effectsConfirmed(current.Intended, current.Confirmed)
			return current
		}
	}
	return ProjectionRecord{ExecutionRevision: preview.ExecutionRevision, Key: preview.ProjectionKey, Intended: cloneEffects(preview.Effects)}
}
func upsertProjection(state State, record ProjectionRecord) State {
	for index := range state.Projections {
		if state.Projections[index].ExecutionRevision == record.ExecutionRevision {
			state.Projections[index] = record
			return state
		}
	}
	state.Projections = append(state.Projections, record)
	return state
}
func confirmProjectionEffect(state State, revision uint64, effect ProjectionEffect) State {
	for index := range state.Projections {
		record := &state.Projections[index]
		if record.ExecutionRevision != revision {
			continue
		}
		if !containsEffect(record.Confirmed, effect) {
			record.Confirmed = append(record.Confirmed, effect)
		}
		record.Complete = effectsConfirmed(record.Intended, record.Confirmed)
	}
	return state
}
func effectObserved(effect ProjectionEffect, observation ProjectionObservation) bool {
	switch effect.Kind {
	case CreateStageLabel:
		return contains(observation.RepositoryLabels, effect.Value)
	case AddStageLabel:
		return contains(observation.IssueLabels, effect.Value)
	case RemoveStageLabel:
		return !contains(observation.IssueLabels, effect.Value)
	case PostTransitionComment:
		return observation.CommentPresent
	}
	return false
}
func projectionFailure(err error, state State, confirmed bool) Result {
	if errors.Is(err, ErrRecoveryRequired) {
		return result(Failed, "recovery_required", state)
	}
	var provider *ProjectionError
	if errors.As(err, &provider) {
		if confirmed {
			return result(Partial, "provider_projection_partial", state)
		}
		if provider.Retryable || provider.Ambiguous || provider.Kind == ProjectionUnavailable || provider.Kind == ProjectionRateLimited {
			return result(Retryable, "provider_projection_retryable", state)
		}
		return result(Failed, "provider_projection_failed", state)
	}
	if confirmed {
		return result(Partial, "provider_projection_partial", state)
	}
	return result(Failed, "provider_projection_failed", state)
}
func storeFailure(err error) Result {
	switch {
	case errors.Is(err, ErrRecoveryRequired):
		return result(Failed, "recovery_required", State{})
	case errors.Is(err, ErrConflict):
		return result(Denied, "stale_execution_revision", State{})
	case errors.Is(err, ErrNotFound):
		return result(ValidationFailed, "execution_not_found", State{})
	default:
		return result(Failed, "execution_store_failed", State{})
	}
}

func transitionFor(state State, input TransitionInput, next Stage, now time.Time, source provenance.Value) Transition {
	return Transition{Revision: state.Revision + 1, From: state.Stage, To: next, Outcome: input.Outcome, RequestDigest: digest(input), References: cloneReferences(input.References), Next: input.Next, CommittedAt: now, Provenance: identity(source)}
}
func transitionDigest(input TransitionInput) (string, bool) {
	if input.ExpectedRevision == 0 || !input.Stage.Valid() || input.Outcome != OutcomePassed && input.Outcome != OutcomeFailed || len(input.References) > maxReferences || !validOptionalText(input.Next) {
		return "", false
	}
	for _, reference := range input.References {
		if !validReference(reference) {
			return "", false
		}
	}
	return digest(input), true
}
func replayed(state State, expected uint64, requestDigest string) bool {
	if len(state.Transitions) == 0 {
		return false
	}
	last := state.Transitions[len(state.Transitions)-1]
	return last.Revision == expected+1 && last.RequestDigest == requestDigest
}
func equivalentStart(state State, projectID, repositoryKey string, item WorkItem, runtimeID string) bool {
	return state.FormatVersion == FormatVersion && state.WorkflowVersion == WorkflowVersion && state.ProjectID == projectID && state.RepositoryKey == repositoryKey && state.RuntimeID == runtimeID && state.WorkItem == item
}

func ValidState(state State) bool {
	if !validOpaqueID(state.ExecutionID) || state.FormatVersion != FormatVersion || state.WorkflowVersion != WorkflowVersion || !validText(state.ProjectID) || !validText(state.RepositoryKey) || !validWorkItem(state.WorkItem) || !validText(state.RuntimeID) || !state.Stage.Valid() || state.Revision == 0 || !validExecutionStatus(state.Status) || state.CreatedAt.IsZero() || state.UpdatedAt.Before(state.CreatedAt) || !validIdentity(state.Provenance) || len(state.Transitions) > maxEvents || len(state.Projections) > maxEvents {
		return false
	}
	if uint64(len(state.Transitions))+1 != state.Revision {
		return false
	}
	derivedStage := Intake
	derivedStatus := ExecutionActive
	lastCommittedAt := state.CreatedAt
	for index, event := range state.Transitions {
		if event.Revision != uint64(index+2) || !event.From.Valid() || !event.To.Valid() || event.Outcome != OutcomePassed && event.Outcome != OutcomeFailed && event.Outcome != OutcomeResumed && event.Outcome != OutcomeFact || event.RequestDigest == "" || event.CommittedAt.IsZero() || !validIdentity(event.Provenance) || len(event.References) > maxReferences || !validOptionalText(event.Next) {
			return false
		}
		if event.From != derivedStage || !validTransitionShape(event, derivedStage, derivedStatus) {
			return false
		}
		if event.CommittedAt.Before(lastCommittedAt) {
			return false
		}
		lastCommittedAt = event.CommittedAt
		switch event.Outcome {
		case OutcomePassed:
			derivedStage = event.To
			if event.From == Completion {
				derivedStatus = ExecutionCompleted
			}
		case OutcomeFailed:
			derivedStatus = ExecutionInterrupted
		case OutcomeResumed:
			derivedStatus = ExecutionActive
		case OutcomeFact:
			if event.Fact == nil {
				return false
			}
		}
		for _, reference := range event.References {
			if !validReference(reference) {
				return false
			}
		}
		if event.Outcome != OutcomeFact && event.Fact != nil {
			return false
		}
	}
	if state.Stage != derivedStage || state.Status != derivedStatus {
		return false
	}
	if len(state.Transitions) != 0 && !state.UpdatedAt.Equal(lastCommittedAt) {
		return false
	}
	if state.Status == ExecutionCompleted && (state.Stage != Completion || state.Terminal == nil || state.Terminal.Status != completion.Success) {
		return false
	}
	if state.Status == ExecutionCompleted && (len(state.Terminal.ConfirmedEffects) != 1 || state.Terminal.ConfirmedEffects[0] != "local_execution_transition") {
		return false
	}
	if state.Status != ExecutionCompleted && state.Terminal != nil {
		return false
	}
	projectionRevisions := make(map[uint64]struct{}, len(state.Projections))
	for _, record := range state.Projections {
		if record.ExecutionRevision < 2 || record.ExecutionRevision > state.Revision || record.Key != projectionKey(state.ExecutionID, record.ExecutionRevision) || len(record.Intended) > 16 || len(record.Confirmed) > len(record.Intended) || record.Complete != effectsConfirmed(record.Intended, record.Confirmed) {
			return false
		}
		if _, exists := projectionRevisions[record.ExecutionRevision]; exists || hasDuplicateEffects(record.Intended) || hasDuplicateEffects(record.Confirmed) || !validProjectionRecord(state, record) {
			return false
		}
		projectionRevisions[record.ExecutionRevision] = struct{}{}
		for _, effect := range append(cloneEffects(record.Intended), record.Confirmed...) {
			if !validEffect(effect) {
				return false
			}
		}
	}
	return true
}

func validProjectionRecord(state State, record ProjectionRecord) bool {
	projected := projectionSnapshot(state, record.ExecutionRevision)
	lifecycle, err := DeriveLifecycle(projected)
	if err == nil {
		label := lifecycleLabel(lifecycle.Stage)
		desired := append([]string{label}, lifecycleFlags(lifecycle.Conditions)...)
		if validProjectionEffects(record, desired, transitionComment(projected, record.Key), validProjectionLabel) {
			return true
		}
	}

	// Persisted S4 records remain readable without rewriting their labels,
	// comments, or Evidence. New previews never create these legacy effects.
	legacyLabel := stagePrefix + string(projected.Stage)
	return validProjectionEffects(record, []string{legacyLabel}, legacyTransitionComment(projected, record.Key), validLegacyStageLabel)
}

func validProjectionEffects(record ProjectionRecord, desired []string, comment string, removable func(string) bool) bool {
	for _, effect := range append(cloneEffects(record.Intended), record.Confirmed...) {
		switch effect.Kind {
		case CreateStageLabel, AddStageLabel:
			if !contains(desired, effect.Value) {
				return false
			}
		case RemoveStageLabel:
			if !removable(effect.Value) || contains(desired, effect.Value) {
				return false
			}
		case PostTransitionComment:
			if effect.Value != comment {
				return false
			}
		}
	}
	return true
}

func projectionSnapshot(state State, revision uint64) State {
	projected := cloneState(state)
	projected.Stage = Intake
	projected.Status = ExecutionActive
	projected.Revision = revision
	projected.Transitions = projected.Transitions[:revision-1]
	projected.UpdatedAt = projected.CreatedAt
	projected.Terminal = nil
	projected.Projections = nil
	for _, event := range projected.Transitions {
		projected.Stage = event.To
		projected.UpdatedAt = event.CommittedAt
		switch event.Outcome {
		case OutcomePassed:
			projected.Status = ExecutionActive
			if event.From == Completion {
				projected.Status = ExecutionCompleted
				projected.Terminal = &Terminal{Status: completion.Success, ConfirmedEffects: []string{"local_execution_transition"}}
			}
		case OutcomeFailed:
			projected.Status = ExecutionInterrupted
		case OutcomeResumed:
			projected.Status = ExecutionActive
		}
	}
	return projected
}

func validTransitionShape(event Transition, stage Stage, status ExecutionStatus) bool {
	if event.Outcome == OutcomeFact {
		return event.To == stage && event.Fact != nil
	}
	if status == ExecutionCompleted {
		return false
	}
	if status == ExecutionInterrupted {
		return event.Outcome == OutcomeResumed && event.To == stage
	}
	if event.Outcome == OutcomeResumed {
		return false
	}
	if event.Outcome == OutcomeFailed {
		return event.To == stage
	}
	index := stageIndex(stage)
	if index == len(stages)-1 {
		return event.To == Completion
	}
	return event.To == stages[index+1]
}

func hasDuplicateEffects(effects []ProjectionEffect) bool {
	seen := make(map[ProjectionEffect]struct{}, len(effects))
	for _, effect := range effects {
		if _, exists := seen[effect]; exists {
			return true
		}
		seen[effect] = struct{}{}
	}
	return false
}

func transitionComment(state State, key string) string {
	event := state.Transitions[len(state.Transitions)-1]
	lifecycle, err := DeriveLifecycle(state)
	if err != nil {
		return ""
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "<!-- axiom:workflow-projection:%s -->\n", key)
	fmt.Fprintf(&builder, "Axiom lifecycle projection\n\n- Lifecycle: `%s`\n- Canonical gate: `%s`\n- Outcome: `%s`\n", lifecycle.Stage, state.Stage, event.Outcome)
	if len(event.References) != 0 {
		builder.WriteString("- References:")
		for _, reference := range event.References {
			fmt.Fprintf(&builder, " `%s:%s`", reference.Kind, reference.ID)
		}
		builder.WriteByte('\n')
	}
	if event.Next != "" {
		fmt.Fprintf(&builder, "- Next: %s\n", event.Next)
	}
	fmt.Fprintf(&builder, "\n_Axiom %s · %s · %s · %s_", event.Provenance.Version, event.Provenance.Revision, event.Provenance.SourceState, state.ExecutionID)
	return builder.String()
}
func legacyTransitionComment(state State, key string) string {
	event := state.Transitions[len(state.Transitions)-1]
	var builder strings.Builder
	fmt.Fprintf(&builder, "<!-- axiom:workflow-projection:%s -->\n", key)
	fmt.Fprintf(&builder, "Axiom workflow transition\n\n- Stage: `%s`\n- Outcome: `%s`\n", state.Stage, event.Outcome)
	if len(event.References) != 0 {
		builder.WriteString("- References:")
		for _, reference := range event.References {
			fmt.Fprintf(&builder, " `%s:%s`", reference.Kind, reference.ID)
		}
		builder.WriteByte('\n')
	}
	if event.Next != "" {
		fmt.Fprintf(&builder, "- Next: %s\n", event.Next)
	}
	fmt.Fprintf(&builder, "\n_Axiom %s · %s · %s · %s_", event.Provenance.Version, event.Provenance.Revision, event.Provenance.SourceState, state.ExecutionID)
	return builder.String()
}
func projectionKey(executionID string, revision uint64) string {
	value := sha256.Sum256([]byte(fmt.Sprintf("axiom:workflow-projection:v1\x00%s\x00%d", executionID, revision)))
	return hex.EncodeToString(value[:])
}
func identity(source provenance.Value) Identity {
	return Identity{Product: source.Product(), Version: source.Version(), Revision: source.Revision(), SourceState: string(source.SourceState())}
}
func validIdentity(value Identity) bool {
	return value.Product == provenance.Product && validText(value.Version) && validText(value.Revision) && (value.SourceState == string(provenance.Clean) || value.SourceState == string(provenance.Dirty) || value.SourceState == string(provenance.Unknown))
}
func validExecutionStatus(value ExecutionStatus) bool {
	return value == ExecutionActive || value == ExecutionInterrupted || value == ExecutionCompleted
}
func validWorkItem(item WorkItem) bool {
	return validText(item.Provider) && validText(item.Resource) && validText(item.ExternalID) && validText(item.URL) && (item.State == "OPEN" || item.State == "CLOSED")
}
func validReference(reference Reference) bool {
	switch reference.Kind {
	case "artifact", "evidence", "specification", "decision", "plan", "tasks", "pull_request":
		return validText(reference.ID) && validDigest(reference.Digest)
	default:
		return false
	}
}
func validEffect(effect ProjectionEffect) bool {
	if effect.Kind != CreateStageLabel && effect.Kind != AddStageLabel && effect.Kind != RemoveStageLabel && effect.Kind != PostTransitionComment {
		return false
	}
	if effect.Kind == PostTransitionComment {
		return validMultiline(effect.Value, 16*1024)
	}
	return validProjectionLabel(effect.Value)
}
func validStageEffect(value string) bool {
	if !strings.HasPrefix(value, stagePrefix) {
		return false
	}
	stage := WorkItemLifecycleStage(strings.TrimPrefix(value, stagePrefix))
	return stage.Valid() && value == lifecycleLabel(stage)
}
func validProjectionLabel(value string) bool {
	return validStageEffect(value) || validLifecycleFlag(value) || validLegacyStageLabel(value)
}
func validLegacyStageLabel(value string) bool {
	if !strings.HasPrefix(value, stagePrefix) {
		return false
	}
	stage := Stage(strings.TrimPrefix(value, stagePrefix))
	return stage.Valid() && value == stagePrefix+string(stage)
}
func observedLifecycleStage(labels []string) (string, bool, error) {
	var observed string
	legacy := false
	for _, label := range labels {
		if strings.HasPrefix(label, "axiom:") && !strings.HasPrefix(label, stagePrefix) && !validLifecycleFlag(label) {
			return "", false, ErrRecoveryRequired
		}
		if !strings.HasPrefix(label, stagePrefix) {
			continue
		}
		if !validStageEffect(label) && !validLegacyStageLabel(label) {
			return "", false, ErrRecoveryRequired
		}
		if observed != "" {
			return "", false, ErrRecoveryRequired
		}
		observed = label
		legacy = !validStageEffect(label)
	}
	if observed == "" {
		return "", false, ErrRecoveryRequired
	}
	return observed, legacy, nil
}
func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
func validText(value string) bool {
	return validMultiline(value, maxTextBytes) && !strings.Contains(value, "\n")
}
func validOptionalText(value string) bool { return value == "" || validText(value) }
func validMultiline(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) && current != '\n' {
			return false
		}
	}
	return true
}
func validOpaqueID(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return false
	}
	_, err := hex.DecodeString(strings.Join(parts, ""))
	return err == nil
}
func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
func digest(value any) string {
	wire, _ := json.Marshal(value)
	sum := sha256.Sum256(wire)
	return hex.EncodeToString(sum[:])
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func containsEffect(values []ProjectionEffect, wanted ProjectionEffect) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func effectsConfirmed(intended, confirmed []ProjectionEffect) bool {
	if len(intended) != len(confirmed) {
		return false
	}
	for _, effect := range intended {
		if !containsEffect(confirmed, effect) {
			return false
		}
	}
	return true
}
func equivalentPersistedState(left, right State) bool {
	left.StorageRevision = [sha256.Size]byte{}
	right.StorageRevision = [sha256.Size]byte{}
	return digest(left) == digest(right)
}
func sorted(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
func cloneEffects(values []ProjectionEffect) []ProjectionEffect {
	return append([]ProjectionEffect(nil), values...)
}
func cloneReferences(values []Reference) []Reference { return append([]Reference(nil), values...) }
func cloneState(state State) State {
	state.Transitions = append([]Transition(nil), state.Transitions...)
	for index := range state.Transitions {
		state.Transitions[index].References = cloneReferences(state.Transitions[index].References)
		if state.Transitions[index].Fact != nil {
			fact := *state.Transitions[index].Fact
			state.Transitions[index].Fact = &fact
		}
	}
	state.Projections = append([]ProjectionRecord(nil), state.Projections...)
	for index := range state.Projections {
		state.Projections[index].Intended = cloneEffects(state.Projections[index].Intended)
		state.Projections[index].Confirmed = cloneEffects(state.Projections[index].Confirmed)
	}
	if state.Terminal != nil {
		terminal := *state.Terminal
		terminal.ConfirmedEffects = append([]string(nil), state.Terminal.ConfirmedEffects...)
		state.Terminal = &terminal
	}
	return state
}
func result(status Status, category string, state State) Result {
	return Result{Status: status, Category: category, State: state}
}
