package projectapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"

	"github.com/rgomids/axiom/internal/project"
)

// Issue #132 EDIT preview (I132-T01). Presentation captures presence only; this
// file owns selection orchestration, partial-intent merge, and complete
// portable/local candidate construction. It has no write capability.

const (
	CreateMode = "create"
	EditMode   = "edit"

	workItemsKey = "work-items"
)

// OptionalText distinguishes an omitted value from an explicitly supplied one.
type OptionalText struct {
	Supplied bool
	Value    string
}

type RepositoryUpsert struct{ Key, Path string }

// EditIntent is partial explicit intent. Omission always means preservation.
// It carries no replay or authority input: EDIT publication is not delivered.
type EditIntent struct {
	Selector               string
	Name                   OptionalText
	WorkItemProvider       OptionalText
	RemoveWorkItemProvider bool
	RepositoryUpserts      []RepositoryUpsert
	RepositoryRemovals     []string
}

// LocalRecordState is the complete machine-local record content observed by the
// local adapter. Every field is carried into the internal candidate so that
// omission from the user-visible preview can never erase it.
type LocalRecordState struct {
	ProjectID, ObservedSlug, SourceLocation string
	PortableRevision                        PortableRevision
	ArtifactDigests                         []ArtifactDigest
	Repositories                            []RepositoryBinding
	Credentials                             []CredentialBinding
	Runtime                                 RuntimeBinding
	Attempt                                 AttemptMetadata
}

func cloneLocalRecord(s LocalRecordState) LocalRecordState {
	s.ArtifactDigests = append([]ArtifactDigest(nil), s.ArtifactDigests...)
	s.Repositories = append([]RepositoryBinding(nil), s.Repositories...)
	s.Credentials = append([]CredentialBinding(nil), s.Credentials...)
	return s
}

// EditSelection is one exactly selected installed Project with its protected
// portable and local observations. Selection never validates per-binding
// availability, so a broken binding can still be repaired or removed.
type EditSelection struct {
	Portable            ArtifactSnapshot
	Local               LocalRecordState
	LocalWire           []byte
	PortableDestination string
	LocalDestination    string
	PortableRevision    string
	LocalRevision       string
}

type EditFailure uint8

const (
	EditOK EditFailure = iota
	EditInvalidIntent
	EditProjectNotFound
	EditProjectAmbiguous
	EditStateUnsafe
	EditRecoveryRequired
	EditUnknownRepository
	EditRepositoryUnavailable
	EditCandidateInvalid
	EditCancelled
	EditUnavailable
)

// EditSource resolves an exact UUID/slug selector and loads its coherent
// protected snapshots. Unknown and ambiguous selection are explicit failures.
type EditSource interface {
	SelectForEdit(context.Context, string) (EditSelection, EditFailure)
}

// LocalRecordCodec validates and encodes a complete local candidate using the
// local adapter's exact wire contract. It performs no I/O.
type LocalRecordCodec interface {
	EncodeLocal(LocalRecordState) ([]byte, []Issue)
}

type EditPorts struct {
	Source    EditSource
	Checkouts CheckoutObserver
	Manifest  ManifestCodec
	Local     LocalRecordCodec
}

const (
	RepositoryAdded     = "added"
	RepositoryUpdated   = "updated"
	RepositoryUnchanged = "unchanged"
	RepositoryPreserved = "preserved"
	RepositoryRemoved   = "removed"
)

// EditRepositoryPreview is the safe user-visible projection of one resulting
// or removed binding. Observation metadata is never projected, and preserved
// bindings expose only their key.
type EditRepositoryPreview struct {
	Key           string `json:"key"`
	Change        string `json:"change"`
	LocalPath     string `json:"localPath,omitempty"`
	LocalRevision string `json:"localRevision,omitempty"`
}

const (
	PortableScope = "portable"
	LocalScope    = "local"
)

type EditEffect struct {
	Scope string `json:"scope"`
	Code  string `json:"code"`
	Key   string `json:"key,omitempty"`
}

// EditPreview is the complete resulting configuration, never a patch.
type EditPreview struct {
	Mode                string                  `json:"mode"`
	ProjectID           string                  `json:"projectId"`
	Slug                string                  `json:"slug"`
	Name                string                  `json:"name"`
	Capability          SetupCapabilityPreview  `json:"capability"`
	PortableManifest    string                  `json:"portableManifest"`
	Repositories        []EditRepositoryPreview `json:"repositories"`
	PortableDestination string                  `json:"portableDestination"`
	LocalDestination    string                  `json:"localDestination"`
	PortableRevision    string                  `json:"portableRevision"`
	LocalRevision       string                  `json:"localRevision"`
	Effects             []EditEffect            `json:"effects"`
	Digest              string                  `json:"digest"`
}

// EditProposal retains the complete internal candidates behind a safe preview.
type EditProposal struct {
	project   project.Project
	manifest  []byte
	local     LocalRecordState
	localWire []byte
	preview   EditPreview
}

func (p EditProposal) Project() project.Project { return p.project }
func (p EditProposal) Manifest() []byte         { return append([]byte(nil), p.manifest...) }
func (p EditProposal) Local() LocalRecordState  { return cloneLocalRecord(p.local) }
func (p EditProposal) LocalWire() []byte        { return append([]byte(nil), p.localWire...) }
func (p EditProposal) Preview() EditPreview {
	preview := p.preview
	preview.Repositories = append([]EditRepositoryPreview(nil), p.preview.Repositories...)
	preview.Effects = append([]EditEffect(nil), p.preview.Effects...)
	return preview
}

// ValidateEditIntent rejects syntactic conflicts before any state read.
func ValidateEditIntent(intent EditIntent) EditFailure {
	if !boundedSetupText(intent.Selector, maxSetupDestinationBytes) {
		return EditInvalidIntent
	}
	if intent.Name.Supplied && !boundedSetupText(intent.Name.Value, maxSetupNameBytes) {
		return EditInvalidIntent
	}
	if intent.WorkItemProvider.Supplied && (intent.RemoveWorkItemProvider || !boundedSetupKey(intent.WorkItemProvider.Value)) {
		return EditInvalidIntent
	}
	if len(intent.RepositoryUpserts) > maxSetupRepositoryCount || len(intent.RepositoryRemovals) > maxSetupRepositoryCount {
		return EditInvalidIntent
	}
	seen := map[string]bool{}
	for _, upsert := range intent.RepositoryUpserts {
		if !boundedSetupKey(upsert.Key) || !boundedSetupText(upsert.Path, maxSetupPathBytes) || !filepath.IsAbs(upsert.Path) || seen[upsert.Key] {
			return EditInvalidIntent
		}
		seen[upsert.Key] = true
	}
	removed := map[string]bool{}
	for _, key := range intent.RepositoryRemovals {
		if !boundedSetupKey(key) || seen[key] || removed[key] {
			return EditInvalidIntent
		}
		removed[key] = true
	}
	return EditOK
}

// PreviewEdit resolves one existing Project, materializes the complete result
// of explicit partial intent, validates it, and returns a digest-bound preview.
// It never creates a missing Project and never writes.
func PreviewEdit(ctx context.Context, ports EditPorts, intent EditIntent) (EditProposal, EditFailure) {
	if ports.Source == nil || ports.Checkouts == nil || ports.Manifest == nil || ports.Local == nil {
		return EditProposal{}, EditUnavailable
	}
	if failure := ValidateEditIntent(intent); failure != EditOK {
		return EditProposal{}, failure
	}
	if ctx.Err() != nil {
		return EditProposal{}, EditCancelled
	}
	selection, failure := ports.Source.SelectForEdit(ctx, intent.Selector)
	if failure != EditOK {
		return EditProposal{}, failure
	}
	current := selection.Portable.Project()
	if failure := coherentSelection(selection); failure != EditOK {
		return EditProposal{}, failure
	}
	candidate, failure := proposePortable(current, intent)
	if failure != EditOK {
		return EditProposal{}, failure
	}
	manifest := selection.Portable.Manifest()
	revision, digests := selection.Local.PortableRevision, selection.Local.ArtifactDigests
	portableChanged := !candidate.Equivalent(current)
	if portableChanged {
		encoded, issues := ports.Manifest.Encode(candidate)
		if len(issues) != 0 {
			return EditProposal{}, EditCandidateInvalid
		}
		snapshot, issues := ReadSnapshot(ports.Manifest, encoded, nil)
		if len(issues) != 0 {
			return EditProposal{}, EditCandidateInvalid
		}
		manifest, revision, digests = encoded, snapshot.Revision(), snapshot.Digests()
	}
	localCandidate, repositories, bindingEffects, failure := proposeLocal(ctx, ports.Checkouts, selection.Local, intent)
	if failure != EditOK {
		return EditProposal{}, failure
	}
	localCandidate.PortableRevision, localCandidate.ArtifactDigests = revision, digests
	localWire, issues := ports.Local.EncodeLocal(localCandidate)
	if len(issues) != 0 {
		return EditProposal{}, EditCandidateInvalid
	}
	effects := make([]EditEffect, 0, len(bindingEffects)+2)
	if portableChanged {
		effects = append(effects, EditEffect{Scope: PortableScope, Code: "update_portable_project"})
	}
	effects = append(effects, portableRepositoryEffects(current, candidate)...)
	if ObserveLocalRevision(localWire) != ObserveLocalRevision(selection.LocalWire) {
		effects = append(effects, EditEffect{Scope: LocalScope, Code: "update_local_record"})
	}
	effects = append(effects, bindingEffects...)
	state := candidate.State()
	preview := EditPreview{
		Mode: EditMode, ProjectID: state.ID, Slug: state.Slug, Name: state.Name,
		Capability: workItemCapability(state), PortableManifest: string(manifest), Repositories: repositories,
		PortableDestination: selection.PortableDestination, LocalDestination: selection.LocalDestination,
		PortableRevision: selection.PortableRevision, LocalRevision: selection.LocalRevision, Effects: effects,
	}
	preview.Digest = editEnvelopeDigest(preview, manifest, localWire)
	return EditProposal{project: candidate, manifest: manifest, local: localCandidate, localWire: localWire, preview: preview}, EditOK
}

// coherentSelection fails closed on stale or conflicting portable/local
// relationships instead of guessing which side is authoritative.
func coherentSelection(selection EditSelection) EditFailure {
	if !selection.Portable.valid() || len(selection.LocalWire) == 0 {
		return EditStateUnsafe
	}
	state := selection.Portable.Project().State()
	local := selection.Local
	observed := sha256.Sum256(selection.LocalWire)
	portableDigest, _ := selection.Portable.Revision().Digest()
	if local.ProjectID != state.ID || local.ObservedSlug != state.Slug || local.SourceLocation != selection.PortableDestination || local.PortableRevision != selection.Portable.Revision() {
		return EditStateUnsafe
	}
	if selection.LocalRevision != hex.EncodeToString(observed[:]) || selection.PortableRevision != hex.EncodeToString(portableDigest[:]) {
		return EditStateUnsafe
	}
	if !boundedSetupText(selection.PortableDestination, maxSetupDestinationBytes) || !boundedSetupText(selection.LocalDestination, maxSetupDestinationBytes) {
		return EditStateUnsafe
	}
	repositories, _ := state.Repositories.Value()
	keys := map[string]bool{}
	for _, repository := range repositories {
		keys[repository.Key] = true
	}
	if len(keys) != len(local.Repositories) {
		return EditStateUnsafe
	}
	for _, binding := range local.Repositories {
		if !keys[binding.RepositoryKey] {
			return EditStateUnsafe
		}
		delete(keys, binding.RepositoryKey)
	}
	return EditOK
}

// proposePortable starts from the complete current Project and supplies only
// explicitly changed top-level fields to Project.Propose.
func proposePortable(current project.Project, intent EditIntent) (project.Project, EditFailure) {
	state := current.State()
	var change project.Intent
	if intent.Name.Supplied {
		change.Name = project.Set(intent.Name.Value)
	}
	if intent.WorkItemProvider.Supplied {
		providers, integrations := setWorkItemProvider(state, intent.WorkItemProvider.Value)
		change.Providers, change.Integrations = project.Set(providers), project.Set(integrations)
	}
	if intent.RemoveWorkItemProvider {
		if providers, integrations, configured := removeWorkItemProvider(state); configured {
			change.Providers, change.Integrations = project.Set(providers), project.Set(integrations)
		}
	}
	if len(intent.RepositoryUpserts) != 0 || len(intent.RepositoryRemovals) != 0 {
		repositories, _ := state.Repositories.Value()
		byKey := make(map[string]project.Repository, len(repositories))
		for _, repository := range repositories {
			byKey[repository.Key] = repository
		}
		for _, key := range intent.RepositoryRemovals {
			if _, exists := byKey[key]; !exists {
				return project.Project{}, EditUnknownRepository
			}
			delete(byKey, key)
		}
		for _, upsert := range intent.RepositoryUpserts {
			if _, exists := byKey[upsert.Key]; !exists {
				byKey[upsert.Key] = project.Repository{Key: upsert.Key}
			}
		}
		if len(byKey) > maxSetupRepositoryCount {
			return project.Project{}, EditInvalidIntent
		}
		result := make([]project.Repository, 0, len(byKey))
		for _, repository := range byKey {
			result = append(result, repository)
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
		change.Repositories = project.Set(project.Configured(result))
	}
	candidate, issues := current.Propose(change)
	if len(issues) != 0 {
		return project.Project{}, EditCandidateInvalid
	}
	return candidate, EditOK
}

// setWorkItemProvider updates the canonical work-items Provider and Integration
// together, retaining unrelated declarations and existing integration fields.
func setWorkItemProvider(state project.State, id string) (project.Declaration[[]project.Provider], project.Declaration[[]project.Integration]) {
	current, _ := state.Providers.Value()
	providers := make([]project.Provider, 0, len(current)+1)
	for _, provider := range current {
		if provider.Key != workItemsKey {
			providers = append(providers, provider)
		}
	}
	providers = append(providers, project.Provider{Key: workItemsKey, ID: id})
	existing, _ := state.Integrations.Value()
	integrations := make([]project.Integration, 0, len(existing)+1)
	found := false
	for _, integration := range existing {
		if integration.Key == workItemsKey {
			found = true
			integration.ProviderRef = project.Configured(workItemsKey)
			capabilities, _ := integration.Capabilities.Value()
			if !containsText(capabilities, WorkItemCapability) {
				integration.Capabilities = project.Configured(append(append([]string(nil), capabilities...), WorkItemCapability))
			}
		}
		integrations = append(integrations, integration)
	}
	if !found {
		integrations = append(integrations, project.Integration{Key: workItemsKey, ProviderRef: project.Configured(workItemsKey), Capabilities: project.Configured([]string{WorkItemCapability})})
	}
	return project.Configured(providers), project.Configured(integrations)
}

// removeWorkItemProvider unconfigures the canonical work-items pair. Removing
// an already unconfigured provider is a no-op; the result state already holds.
func removeWorkItemProvider(state project.State) (project.Declaration[[]project.Provider], project.Declaration[[]project.Integration], bool) {
	current, _ := state.Providers.Value()
	providers := make([]project.Provider, 0, len(current))
	configured := false
	for _, provider := range current {
		if provider.Key == workItemsKey {
			configured = true
			continue
		}
		providers = append(providers, provider)
	}
	existing, _ := state.Integrations.Value()
	integrations := make([]project.Integration, 0, len(existing))
	for _, integration := range existing {
		if integration.Key == workItemsKey {
			configured = true
			continue
		}
		integrations = append(integrations, integration)
	}
	providerDeclaration, integrationDeclaration := project.Unconfigured[[]project.Provider](), project.Unconfigured[[]project.Integration]()
	if len(providers) != 0 {
		providerDeclaration = project.Configured(providers)
	}
	if len(integrations) != 0 {
		integrationDeclaration = project.Configured(integrations)
	}
	return providerDeclaration, integrationDeclaration, configured
}

func containsText(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// proposeLocal clones the complete local record and changes only bindings named
// by explicit intent. Only newly supplied paths are observed.
func proposeLocal(ctx context.Context, checkouts CheckoutObserver, current LocalRecordState, intent EditIntent) (LocalRecordState, []EditRepositoryPreview, []EditEffect, EditFailure) {
	candidate := cloneLocalRecord(current)
	byKey := make(map[string]RepositoryBinding, len(candidate.Repositories))
	for _, binding := range candidate.Repositories {
		byKey[binding.RepositoryKey] = binding
	}
	changes := map[string]EditRepositoryPreview{}
	for _, key := range intent.RepositoryRemovals {
		prior, exists := byKey[key]
		if !exists {
			return LocalRecordState{}, nil, nil, EditUnknownRepository
		}
		delete(byKey, key)
		changes[key] = EditRepositoryPreview{Key: key, Change: RepositoryRemoved, LocalPath: prior.ExplicitPath}
	}
	for _, upsert := range intent.RepositoryUpserts {
		if ctx.Err() != nil {
			return LocalRecordState{}, nil, nil, EditCancelled
		}
		path := filepath.Clean(upsert.Path)
		facts, issues := checkouts.ObserveCheckout(ctx, CheckoutRequest{RepositoryKey: upsert.Key, ExplicitPath: path})
		if len(issues) != 0 || facts.CanonicalIdentity == "" || !boundedSetupText(facts.CanonicalIdentity, maxSetupRevisionBytes) {
			return LocalRecordState{}, nil, nil, EditRepositoryUnavailable
		}
		next := RepositoryBinding{RepositoryKey: upsert.Key, ExplicitPath: path, CanonicalIdentity: facts.CanonicalIdentity, Observation: facts.Observation}
		change := RepositoryAdded
		if prior, exists := byKey[upsert.Key]; exists {
			change = RepositoryUpdated
			if prior.ExplicitPath == path && prior.CanonicalIdentity == facts.CanonicalIdentity {
				change, next = RepositoryUnchanged, prior
			}
		}
		byKey[upsert.Key] = next
		changes[upsert.Key] = EditRepositoryPreview{Key: upsert.Key, Change: change, LocalPath: next.ExplicitPath, LocalRevision: next.CanonicalIdentity}
	}
	candidate.Repositories = make([]RepositoryBinding, 0, len(byKey))
	for _, binding := range byKey {
		candidate.Repositories = append(candidate.Repositories, binding)
		if _, changed := changes[binding.RepositoryKey]; !changed {
			changes[binding.RepositoryKey] = EditRepositoryPreview{Key: binding.RepositoryKey, Change: RepositoryPreserved}
		}
	}
	sort.Slice(candidate.Repositories, func(i, j int) bool {
		return candidate.Repositories[i].RepositoryKey < candidate.Repositories[j].RepositoryKey
	})
	repositories := make([]EditRepositoryPreview, 0, len(changes))
	for _, change := range changes {
		repositories = append(repositories, change)
	}
	sort.Slice(repositories, func(i, j int) bool { return repositories[i].Key < repositories[j].Key })
	effects := make([]EditEffect, 0, len(repositories))
	codes := map[string]string{RepositoryAdded: "add_local_binding", RepositoryUpdated: "update_local_binding", RepositoryRemoved: "remove_local_binding"}
	for _, repository := range repositories {
		if code, ok := codes[repository.Change]; ok {
			effects = append(effects, EditEffect{Scope: LocalScope, Code: code, Key: repository.Key})
		}
	}
	return candidate, repositories, effects, EditOK
}

// portableRepositoryEffects reports portable association additions/removals by
// key so a Repository removal is visible separately in portable and local scope.
func portableRepositoryEffects(current, candidate project.Project) []EditEffect {
	before, _ := current.State().Repositories.Value()
	after, _ := candidate.State().Repositories.Value()
	prior, next := map[string]bool{}, map[string]bool{}
	keys := []string{}
	for _, repository := range before {
		prior[repository.Key] = true
		keys = append(keys, repository.Key)
	}
	for _, repository := range after {
		next[repository.Key] = true
		if !prior[repository.Key] {
			keys = append(keys, repository.Key)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	effects := []EditEffect{}
	for _, key := range keys {
		switch {
		case prior[key] && !next[key]:
			effects = append(effects, EditEffect{Scope: PortableScope, Code: "remove_portable_repository", Key: key})
		case !prior[key] && next[key]:
			effects = append(effects, EditEffect{Scope: PortableScope, Code: "add_portable_repository", Key: key})
		}
	}
	return effects
}

// workItemCapability reports readiness by the same contract Work Item
// execution enforces, so a preserved incomplete declaration is never shown
// as ready: a github work-items Provider without its declaring Integration is
// missing the capability.
func workItemCapability(state project.State) SetupCapabilityPreview {
	capability := SetupCapabilityPreview{Capability: WorkItemCapability, Readiness: CapabilityMissing}
	providers, _ := state.Providers.Value()
	for _, provider := range providers {
		if provider.Key != workItemsKey {
			continue
		}
		capability.Provider = provider.ID
		capability.Readiness = CapabilityUnsupported
		if provider.ID == "github" {
			capability.Readiness = CapabilityMissing
			if GitHubWorkItemCapability(state) {
				capability.Readiness = CapabilityReady
			}
		}
	}
	return capability
}

// GitHubWorkItemCapability is the Work Item capability contract: the
// work-items Provider is github and the work-items Integration references it
// and declares the work-item capability.
func GitHubWorkItemCapability(state project.State) bool {
	providers, _ := state.Providers.Value()
	providerReady := false
	for _, provider := range providers {
		if provider.Key == workItemsKey && provider.ID == "github" {
			providerReady = true
		}
	}
	if !providerReady {
		return false
	}
	integrations, _ := state.Integrations.Value()
	for _, integration := range integrations {
		provider, configured := integration.ProviderRef.Value()
		capabilities, declared := integration.Capabilities.Value()
		if integration.Key == workItemsKey && configured && provider == workItemsKey && declared && containsText(capabilities, WorkItemCapability) {
			return true
		}
	}
	return false
}

// editEnvelope binds mode, identity, destinations, exact observations, the
// canonical portable candidate, the complete internal local candidate, the
// ordered effects, and the safe preview. Local fields absent from the preview
// still change the digest.
type editEnvelope struct {
	Preview           EditPreview `json:"preview"`
	PortableCandidate []byte      `json:"portableCandidate"`
	LocalCandidate    []byte      `json:"localCandidate"`
}

func editEnvelopeDigest(preview EditPreview, portable, local []byte) string {
	preview.Digest = ""
	wire, _ := json.Marshal(editEnvelope{Preview: preview, PortableCandidate: portable, LocalCandidate: local})
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:])
}
