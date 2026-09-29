package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/rgomids/axiom/internal/cli"
	"github.com/rgomids/axiom/internal/codexruntime"
	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/githubissues"
	"github.com/rgomids/axiom/internal/local"
	"github.com/rgomids/axiom/internal/manifest"
	"github.com/rgomids/axiom/internal/project"
	"github.com/rgomids/axiom/internal/projectapp"
	"github.com/rgomids/axiom/internal/provenance"
	"github.com/rgomids/axiom/internal/workflow"
	"github.com/rgomids/axiom/internal/workitem"
)

var (
	buildVersion     = provenance.Development
	buildRevision    = provenance.Unavailable
	buildSourceState = string(provenance.Unknown)
	buildRelease     = "false"
)

func main() {
	source := currentProvenance()
	if format, ok := versionFormat(os.Args[1:]); ok {
		os.Exit(writeVersion(os.Stdout, format, source))
	}
	if len(os.Args) == 2 && (os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h") {
		os.Exit(cli.Help(os.Stdout))
	}
	os.Exit(cli.RunInteractive(context.Background(), os.Args[1:], composeWithProvenance(source), source, os.Stdin, os.Stdout, os.Stderr))
}

func versionFormat(args []string) (cli.CompletionFormat, bool) {
	if len(args) == 1 && args[0] == "version" {
		return cli.CompletionHuman, true
	}
	if len(args) == 2 && args[1] == "version" && args[0] == "--json" {
		return cli.CompletionJSON, true
	}
	if len(args) == 2 && args[1] == "version" && args[0] == "--human" {
		return cli.CompletionHuman, true
	}
	return "", false
}

func writeVersion(output *os.File, format cli.CompletionFormat, source provenance.Value) int {
	result, ok := newCompletion(completion.Facts{Completed: true}, "Axiom build information", nil, "", source)
	if !ok {
		return cli.ExitFailure
	}
	return cli.WriteCompletion(output, format, *result.Completion)
}

func currentProvenance() provenance.Value {
	release, err := strconv.ParseBool(buildRelease)
	if err != nil {
		return unknownProvenance()
	}
	settings := make([]provenance.Setting, 0, 2)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" || setting.Key == "vcs.modified" {
				settings = append(settings, provenance.Setting{Key: setting.Key, Value: setting.Value})
			}
		}
	}
	value, err := provenance.FromBuild(provenance.Build{Release: release, Version: buildVersion, Revision: buildRevision, SourceState: provenance.SourceState(buildSourceState)}, settings)
	if err != nil {
		return unknownProvenance()
	}
	return value
}

func unknownProvenance() provenance.Value {
	value, _ := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: provenance.Unavailable, SourceState: provenance.Unknown}, nil)
	return value
}

func compose() cli.Service {
	return composeWithProvenance(currentProvenance())
}

func composeWithProvenance(source provenance.Value) cli.Service {
	root, err := projectsRoot()
	if err != nil {
		return cli.NewUnavailableService(source)
	}
	state, err := stateRoot()
	if err != nil {
		return cli.NewUnavailableService(source)
	}
	if overlap, err := local.RootsOverlap(root, state); err != nil || overlap {
		return cli.NewUnavailableService(source)
	}
	store, err := local.NewPortableStore(root)
	if err != nil {
		return cli.NewUnavailableService(source)
	}
	installation, err := local.NewInstallationStore(state)
	if err != nil {
		return cli.NewUnavailableService(source)
	}
	codex, err := codexruntime.New(codexSkillsRoot())
	if err != nil {
		return cli.NewUnavailableService(source)
	}
	workItems, err := local.NewWorkItemStore(state)
	if err != nil {
		return cli.NewUnavailableService(source)
	}
	github, githubErr := githubissues.New(os.Getenv("AXIOM_GH_BIN"))
	var capability workitem.Capability
	var legacy workitem.LegacyProjection
	if githubErr == nil {
		capability = github
		legacy = github
	}
	workItemService := workitem.New(workItemResolver{installation: installation, portable: store}, capability, legacy, workItems, source)
	workflows, err := local.NewWorkflowStore(state)
	if err != nil {
		return cli.NewUnavailableService(source)
	}
	artifacts, err := local.NewArtifactStore(state)
	if err != nil {
		return cli.NewUnavailableService(source)
	}
	references := local.NewWorkflowReferenceValidator(artifacts)
	workflowService := workflow.New(workflowResolver{installation}, workflowWorkItems{workItemService}, workflows, github, references, source, nil, nil)
	return lifecycleService{lifecycle: projectapp.NewLifecycle(store, manifest.Codec{}, local.IdentityAllocator{}), portable: store, installation: installation, codex: codex, workItems: workItemService, workflows: workflowService, projectsRoot: root, stateRoot: state, skillsRoot: codexSkillsRoot(), runtimes: discoverRuntimeRoots(), provenance: source}
}

func codexSkillsRoot() string {
	if override := os.Getenv("AXIOM_CODEX_SKILLS_ROOT"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agents", "skills")
}

func projectsRoot() (string, error) {
	if override := os.Getenv("LINGO_PROJECTS_ROOT"); override != "" {
		if !filepath.IsAbs(override) {
			return "", projectapp.ErrUnsafe
		}
		return filepath.Clean(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".axiom", "projects"), nil
}
func stateRoot() (string, error) {
	if override, set := os.LookupEnv("LINGO_STATE_ROOT"); set {
		if !filepath.IsAbs(override) || filepath.Clean(override) == string(filepath.Separator) {
			return "", projectapp.ErrUnsafe
		}
		return filepath.Clean(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return local.NativeStateRoot(runtime.GOOS, home, os.Getenv("XDG_STATE_HOME"))
}

type lifecycleService struct {
	lifecycle              projectapp.Lifecycle
	portable               local.PortableStore
	installation           local.InstallationStore
	codex                  codexruntime.Service
	workItems              workitem.Service
	workflows              workflow.Service
	projectsRoot           string
	stateRoot              string
	skillsRoot             string
	runtimes               runtimeRoots
	provenance             provenance.Value
	beforeLocalPublication func()
}

type workItemResolver struct {
	installation local.InstallationStore
	portable     local.PortableStore
}

type workflowResolver struct{ installation local.InstallationStore }
type workflowWorkItems struct{ service workitem.Service }

func (r workItemResolver) Resolve(ctx context.Context, selector string) (workitem.Project, string) {
	resolved := r.installation.Resolve(ctx, selector)
	if resolved.Status != local.ResolutionFound {
		return workitem.Project{}, resolved.Category
	}
	portable, err := r.portable.Inspect(ctx, resolved.Project.Slug)
	if err != nil || !portable.Exists || portable.Snapshot.Project().State().ID != resolved.Project.ID {
		return workitem.Project{}, "invalid_project_capability_state"
	}
	state := portable.Snapshot.Project().State()
	providers, providersConfigured := state.Providers.Value()
	integrations, integrationsConfigured := state.Integrations.Value()
	if !providersConfigured || !integrationsConfigured || !githubWorkItemCapability(providers, integrations) {
		return workitem.Project{}, "work_item_capability_unavailable"
	}
	project := workitem.Project{ID: resolved.Project.ID, Provider: "github", Repositories: make([]workitem.Repository, 0, len(resolved.Project.Repositories))}
	for _, repository := range resolved.Project.Repositories {
		project.Repositories = append(project.Repositories, workitem.Repository{Key: repository.Key, Path: repository.Path})
	}
	return project, ""
}

func githubWorkItemCapability(providers []project.Provider, integrations []project.Integration) bool {
	providerReady := false
	for _, provider := range providers {
		if provider.Key == "work-items" && provider.ID == "github" {
			providerReady = true
		}
	}
	if !providerReady {
		return false
	}
	for _, integration := range integrations {
		provider, configured := integration.ProviderRef.Value()
		capabilities, declared := integration.Capabilities.Value()
		if integration.Key != "work-items" || !configured || provider != "work-items" || !declared {
			continue
		}
		for _, capability := range capabilities {
			if capability == projectapp.WorkItemCapability {
				return true
			}
		}
	}
	return false
}

func (r workflowResolver) Resolve(ctx context.Context, selector string) (workflow.Project, string) {
	resolved := r.installation.Resolve(ctx, selector)
	if resolved.Status != local.ResolutionFound {
		return workflow.Project{}, resolved.Category
	}
	project := workflow.Project{ID: resolved.Project.ID, Repositories: make([]workflow.Repository, 0, len(resolved.Project.Repositories))}
	for _, repository := range resolved.Project.Repositories {
		project.Repositories = append(project.Repositories, workflow.Repository{Key: repository.Key, Path: repository.Path})
	}
	return project, ""
}

func (w workflowWorkItems) Load(ctx context.Context, project, repository, provider, resource, selector string) (workflow.WorkItem, error) {
	if provider != "" && provider != "github" {
		return workflow.WorkItem{}, workitem.ErrNotFound
	}
	result := w.service.Show(ctx, workitem.Target{ProjectSelector: project, RepositoryKey: repository, ProviderResource: resource}, selector)
	if result.Status != workitem.Succeeded {
		return workflow.WorkItem{}, workitem.ErrNotFound
	}
	return workflow.WorkItem{Provider: result.Link.Provider, Resource: result.Link.Resource, ExternalID: result.Link.ExternalID, URL: result.Link.URL, State: result.Link.State}, nil
}

func (s lifecycleService) RuntimeCodexInstall(ctx context.Context) cli.Result {
	return runtimeResult(s.codex.Install(ctx))
}
func (s lifecycleService) RuntimeCodexStatus(ctx context.Context) cli.Result {
	return runtimeStatus(s.codex.Inspect(ctx), "Codex", "codex", s.provenance)
}

func runtimeStatus(result codexruntime.Result, label, id string, source provenance.Value) cli.Result {
	if result.Status == codexruntime.Ready {
		response := canonicalCompletion(completion.Facts{Completed: true}, "Lingo and "+label+" skills are compatible", []string{"skill-set:" + result.SkillSetVersion}, "Run project configure with explicit Project inputs", source)
		response.Runtime = runtimeView(result)
		return response
	}
	if result.Status == codexruntime.Incompatible || result.Status == codexruntime.Missing || result.Status == codexruntime.Partial {
		response := canonicalCompletion(completion.Facts{ValidationFailed: true}, label+" skill compatibility is not ready", nil, "Run runtime "+id+" install, then project configure", source)
		response.Runtime = runtimeView(result)
		return response
	}
	response := canonicalCompletion(completion.Facts{Failed: true}, label+" compatibility inspection failed", nil, "Inspect the configured "+label+" skill root", source)
	response.Runtime = runtimeView(result)
	return response
}

func runtimeResult(result codexruntime.Result) cli.Result {
	status := cli.Failed
	if result.Status == codexruntime.Applied || result.Status == codexruntime.Unchanged || result.Status == codexruntime.Ready {
		status = cli.Succeeded
	}
	return cli.Result{Status: status, Category: result.Category, Runtime: runtimeView(result)}
}

func runtimeView(result codexruntime.Result) *cli.RuntimeView {
	view := &cli.RuntimeView{SkillSetVersion: result.SkillSetVersion, BinaryCompatibility: result.BinaryCompatibility, Skills: make([]cli.RuntimeSkillView, 0, len(result.Skills))}
	for _, skill := range result.Skills {
		view.Skills = append(view.Skills, cli.RuntimeSkillView{Name: skill.Name, SHA256: skill.Digest, State: skill.State})
	}
	return view
}

func (s lifecycleService) Init(ctx context.Context, input cli.InitInput) cli.Result {
	return cliResult(s.lifecycle.Init(ctx, projectapp.InitRequest{Slug: input.Slug, Name: input.Name}))
}
func (s lifecycleService) Validate(ctx context.Context, input cli.ProjectInput) cli.Result {
	result := s.lifecycle.Validate(ctx, projectapp.ProjectRequest{Slug: input.Slug})
	if result.Status == projectapp.LifecycleApplied || result.Status == projectapp.LifecycleUnchanged {
		return canonicalCompletion(completion.Facts{Completed: true}, "Project is valid", nil, "", s.provenance)
	}
	if result.Status == projectapp.LifecycleCancelled {
		return canonicalCompletion(completion.Facts{WasInterrupted: true}, "Project validation was interrupted", nil, "Retry Project validation", s.provenance)
	}
	if result.Category == "storage_failure" || result.Category == "application_unavailable" {
		return canonicalCompletion(completion.Facts{Failed: true}, "Project validation failed", nil, "", s.provenance)
	}
	return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Project state is invalid", nil, "Correct Project state and retry validation", s.provenance)
}
func (s lifecycleService) Reopen(ctx context.Context, input cli.ProjectInput) cli.Result {
	result := cliResult(s.lifecycle.Reopen(ctx, projectapp.ProjectRequest{Slug: input.Slug}))
	if result.Status != cli.Succeeded {
		return result
	}
	localResult := s.installation.Reopen(ctx, filepath.Join(s.projectsRoot, input.Slug))
	if localResult.Status == local.InstallationFailed || localResult.Status == local.InstallationConflict {
		return cli.Result{Status: cli.Failed, Category: localResult.Category}
	}
	return cli.Result{Status: cli.Succeeded, Category: localResult.Category}
}
func (s lifecycleService) Update(ctx context.Context, input cli.UpdateInput) cli.Result {
	return cliResult(s.lifecycle.Update(ctx, projectapp.UpdateRequest{Slug: input.Slug, Name: input.Name}))
}
func (s lifecycleService) Install(ctx context.Context, input cli.InstallInput) cli.Result {
	result := s.installation.Install(ctx, input.Source)
	status := cli.Failed
	if result.Status == local.InstallationApplied || result.Status == local.InstallationUnchanged {
		status = cli.Succeeded
	}
	return cli.Result{Status: status, Category: result.Category}
}
func (s lifecycleService) Resolve(ctx context.Context, input cli.ResolveInput) cli.Result {
	result := s.installation.Resolve(ctx, input.Selector)
	status := cli.Failed
	if result.Status == local.ResolutionFound {
		status = cli.Succeeded
	}
	response := cli.Result{Status: status, Category: result.Category}
	if result.Status == local.ResolutionFound {
		response.Project = projectView(result.Project)
	}
	return response
}

func (s lifecycleService) Show(ctx context.Context, input cli.ResolveInput) cli.Result {
	result := s.installation.Resolve(ctx, input.Selector)
	if result.Status != local.ResolutionFound {
		return projectShowFailure(result.Category, s.provenance)
	}
	references := []string{"project:" + result.Project.ID}
	for _, repository := range result.Project.Repositories {
		references = append(references, "repository:"+repository.Key)
	}
	response := canonicalCompletion(completion.Facts{Completed: true}, "Project resolved", references, "", s.provenance)
	response.Project = projectView(result.Project)
	return response
}

func projectShowFailure(category string, source provenance.Value) cli.Result {
	switch category {
	case "invalid_project_selector":
		return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Project selector is invalid", nil, "Provide a valid Project UUID or slug", source)
	case "project_not_found":
		return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Project was not found", nil, "Provide an existing Project UUID or slug", source)
	case "project_ambiguous":
		return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Project selector is ambiguous", nil, "Provide the exact Project UUID", source)
	case "invalid_existing_local_state":
		return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Local Project state is invalid", nil, "Repair or reconfigure local Project state before retrying inspection", source)
	case "recovery_required":
		return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Local Project state requires recovery", nil, "Review preserved local recovery state before retrying inspection", source)
	case "project_source_unavailable":
		return canonicalCompletion(completion.Facts{RetrySafeFailure: true}, "Project source is unavailable", nil, "Restore the configured Project source and retry inspection", source)
	case "repository_unavailable":
		return canonicalCompletion(completion.Facts{RetrySafeFailure: true}, "Project repository is unavailable", nil, "Restore the configured repository binding and retry inspection", source)
	case "cancelled":
		return canonicalCompletion(completion.Facts{WasInterrupted: true}, "Project inspection was interrupted", nil, "Retry Project inspection", source)
	default:
		return canonicalCompletion(completion.Facts{Failed: true}, "Project inspection failed", nil, "Inspect local storage and application availability before retrying", source)
	}
}

func projectView(project local.ResolvedProject) *cli.ProjectView {
	view := &cli.ProjectView{ID: project.ID, Slug: project.Slug, Source: project.Source, Repositories: make([]cli.RepositoryView, 0, len(project.Repositories))}
	for _, repository := range project.Repositories {
		view.Repositories = append(view.Repositories, cli.RepositoryView{Key: repository.Key, Path: repository.Path})
	}
	return view
}
func (s lifecycleService) Configure(ctx context.Context, input cli.ConfigureInput) cli.Result {
	repositories := make([]projectapp.SetupRepository, 0, len(input.Repositories))
	for _, repository := range input.Repositories {
		cleanPath := filepath.Clean(repository.Path)
		identity, err := local.DirectoryIdentity(cleanPath)
		if err != nil {
			return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Project setup input is invalid", nil, "Provide an existing absolute non-link Repository path", s.provenance)
		}
		repositories = append(repositories, projectapp.SetupRepository{Key: repository.Key, Path: cleanPath, Revision: identity})
	}
	portableObservation, err := s.portable.Inspect(ctx, input.Slug)
	if err != nil && !errors.Is(err, projectapp.ErrNotFound) {
		return canonicalCompletion(completion.Facts{Failed: true}, "Project setup inspection failed", nil, "Inspect portable Project state before retrying", s.provenance)
	}
	projectID := input.ProjectID
	if portableObservation.Exists {
		existingID := portableObservation.Snapshot.Project().State().ID
		if projectID != "" && projectID != existingID {
			return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Project identity conflicts with existing state", nil, "Use the existing Project identity or a different slug", s.provenance)
		}
		projectID = existingID
	}
	if projectID == "" {
		allocated, issues := (local.IdentityAllocator{}).NewID()
		if len(issues) != 0 {
			return canonicalCompletion(completion.Facts{Failed: true}, "Project identity allocation failed", nil, "Retry Project setup", s.provenance)
		}
		projectID = allocated
	}
	localObservation, category := s.installation.Inspect(ctx, projectID)
	if category != "" {
		return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Local Project state is not safe to configure", nil, "Inspect preserved local state before retrying", s.provenance)
	}
	observation := projectapp.SetupObservation{
		PortableDestination: filepath.Join(s.projectsRoot, input.Slug),
		LocalDestination:    filepath.Join(s.stateRoot, "projects", projectID),
		PortableRevision:    portableObservation.Revision,
		LocalRevision:       localObservation.Revision,
	}
	setupInput := projectapp.SetupInput{ProjectID: projectID, Slug: input.Slug, Name: input.Name, Repositories: repositories, WorkItemProvider: input.WorkItemProvider}
	proposal, issues := projectapp.PrepareSetup(manifest.Codec{}, setupInput, observation)
	if len(issues) != 0 {
		return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Project setup input is invalid", nil, "Correct Project identity, repositories, or capability declaration", s.provenance)
	}
	observation.PortableEquivalent = portableObservation.Exists && portableObservation.Snapshot.Project().Equivalent(proposal.Project())
	desiredSnapshot, snapshotIssues := projectapp.ReadSnapshot(manifest.Codec{}, proposal.Manifest(), nil)
	if len(snapshotIssues) != 0 {
		return canonicalCompletion(completion.Facts{Failed: true}, "Project setup proposal could not be encoded", nil, "Review application availability before retrying", s.provenance)
	}
	desiredRecord, recordIssues := local.NewRecord(local.RecordState{ProjectID: projectID, ObservedSlug: input.Slug, SourceLocation: filepath.Join(s.projectsRoot, input.Slug), PortableRevision: desiredSnapshot.Revision(), ArtifactDigests: desiredSnapshot.Digests(), Repositories: proposal.Bindings()})
	if len(recordIssues) != 0 {
		return canonicalCompletion(completion.Facts{ValidationFailed: true}, "Local Project proposal is invalid", nil, "Correct Repository bindings and retry", s.provenance)
	}
	desiredWire, encodeIssues := local.EncodeRecord(desiredRecord)
	if len(encodeIssues) != 0 {
		return canonicalCompletion(completion.Facts{Failed: true}, "Local Project proposal could not be encoded", nil, "Review application availability before retrying", s.provenance)
	}
	observation.LocalEquivalent = localObservation.Exists && bytes.Equal(localObservation.Wire, desiredWire)
	proposal, issues = projectapp.PrepareSetup(manifest.Codec{}, setupInput, observation)
	if len(issues) != 0 {
		return canonicalCompletion(completion.Facts{Failed: true}, "Project setup proposal failed", nil, "Retry Project setup", s.provenance)
	}
	preview := proposal.Preview()
	if portableObservation.Exists && !observation.PortableEquivalent {
		result := canonicalCompletion(completion.Facts{ValidationFailed: true}, "Project setup conflicts with existing portable state", nil, "Choose a different slug or use an explicit Project update", s.provenance)
		result.Setup = &preview
		return result
	}
	if !input.AuthorizeLocal {
		next := "Review preview, then repeat with --project-id, --preview-digest, and --authorize-local"
		if len(preview.Effects) == 0 {
			next = "No publication is required"
		}
		result := canonicalCompletion(completion.Facts{Completed: true}, "Project setup preview ready", []string{"project:" + projectID}, next, s.provenance)
		result.Setup = &preview
		return result
	}
	if !proposal.MatchesDigest(input.PreviewDigest) {
		result := canonicalCompletion(completion.Facts{AuthorityDenied: true}, "Project setup authority is missing or stale", nil, "Review the current preview and authorize its exact digest", s.provenance)
		result.Setup = &preview
		return result
	}
	portableResult := s.lifecycle.PublishConfigured(ctx, proposal.Project())
	if portableResult.Status != projectapp.LifecycleApplied && portableResult.Status != projectapp.LifecycleUnchanged {
		facts := completion.Facts{Failed: true}
		if portableResult.Status == projectapp.LifecycleConflict {
			facts = completion.Facts{AuthorityDenied: true}
		}
		result := canonicalCompletion(facts, "Portable Project publication failed", nil, "Review current state and prepare a fresh preview", s.provenance)
		result.Setup = &preview
		return result
	}
	if s.beforeLocalPublication != nil {
		s.beforeLocalPublication()
	}
	localResult := s.installation.InstallWithBindings(ctx, filepath.Join(s.projectsRoot, input.Slug), proposal.Bindings())
	if localResult.Status != local.InstallationApplied && localResult.Status != local.InstallationUnchanged {
		facts := completion.Facts{Failed: true}
		references := []string(nil)
		message := "Local Project publication failed"
		if portableResult.Status == projectapp.LifecycleApplied {
			facts = completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}
			references = []string{"project:" + projectID, "portable:" + observation.PortableDestination}
			message = "Portable Project published; local bindings did not complete"
		}
		result := canonicalCompletion(facts, message, references, "Inspect local state and resume with a fresh preview", s.provenance)
		result.Setup = &preview
		return result
	}
	message := "Project setup published"
	if portableResult.Status == projectapp.LifecycleUnchanged && localResult.Status == local.InstallationUnchanged {
		message = "Project setup already matches the authorized proposal"
	}
	next := "Project resolves by UUID or slug; Work Item capability is ready"
	if preview.Capability.Readiness != projectapp.CapabilityReady {
		next = "Project is valid; configure a supported Work Item capability before starting that journey"
	}
	result := canonicalCompletion(completion.Facts{Completed: true}, message, []string{"project:" + projectID}, next, s.provenance)
	result.Setup = &preview
	return result
}

func (s lifecycleService) WorkItemCreate(ctx context.Context, input cli.WorkItemInput) cli.Result {
	draft := workitem.DraftInput{
		Target: workitem.Target{ProjectSelector: input.Project, RepositoryKey: input.Repository, ProviderResource: input.ProviderRepository},
		Intent: input.Intent, Problem: workitem.SectionInput{Supplied: input.Problem}, DesiredOutcome: workitem.SectionInput{Supplied: input.DesiredOutcome},
		Context: workitem.SectionInput{Supplied: input.Context}, Scope: workitem.SectionInput{Supplied: input.Scope}, Constraints: workitem.SectionInput{Supplied: input.Constraints},
		NonGoals: workitem.SectionInput{Supplied: input.NonGoals}, Acceptance: workitem.SectionInput{Supplied: input.Acceptance}, Cancelled: input.Cancelled,
	}
	if !input.AuthorizeExternal && input.PreviewDigest == "" {
		return workItemResult(s.workItems.Prepare(ctx, draft), s.provenance)
	}
	return workItemResult(s.workItems.Create(ctx, draft, input.PreviewDigest, input.AuthorizeExternal), s.provenance)
}
func (s lifecycleService) WorkItemSelect(ctx context.Context, input cli.WorkItemInput) cli.Result {
	if input.Provider != "" && input.Provider != "github" {
		return workItemResult(workitem.Result{Status: completion.ValidationFailure, Category: "invalid_work_item_input"}, s.provenance)
	}
	target := workitem.Target{ProjectSelector: input.Project, RepositoryKey: input.Repository, ProviderResource: input.ProviderRepository}
	if !input.AuthorizeLocal && input.PreviewDigest == "" {
		return workItemResult(s.workItems.PreviewSelect(ctx, target, workItemExternalID(input)), s.provenance)
	}
	return workItemResult(s.workItems.Select(ctx, target, workItemExternalID(input), input.PreviewDigest, input.AuthorizeLocal), s.provenance)
}
func (s lifecycleService) WorkItemShow(ctx context.Context, input cli.WorkItemInput) cli.Result {
	if input.Provider != "" && input.Provider != "github" {
		return workItemResult(workitem.Result{Status: completion.ValidationFailure, Category: "invalid_work_item_input"}, s.provenance)
	}
	return workItemResult(s.workItems.Show(ctx, workitem.Target{ProjectSelector: input.Project, RepositoryKey: input.Repository, ProviderResource: input.ProviderRepository}, workItemExternalID(input)), s.provenance)
}
func (s lifecycleService) WorkItemComment(ctx context.Context, input cli.WorkItemInput) cli.Result {
	if input.Provider != "" && input.Provider != "github" {
		return workItemResult(workitem.Result{Status: completion.ValidationFailure, Category: "invalid_work_item_input"}, s.provenance)
	}
	return workItemResult(s.workItems.Comment(ctx, workitem.Target{ProjectSelector: input.Project, RepositoryKey: input.Repository, ProviderResource: input.ProviderRepository}, workItemExternalID(input), input.Message, input.AuthorizeExternal), s.provenance)
}
func (s lifecycleService) WorkItemComplete(ctx context.Context, input cli.WorkItemInput) cli.Result {
	if input.Provider != "" && input.Provider != "github" {
		return workItemResult(workitem.Result{Status: completion.ValidationFailure, Category: "invalid_work_item_input"}, s.provenance)
	}
	return workItemResult(s.workItems.Complete(ctx, workitem.Target{ProjectSelector: input.Project, RepositoryKey: input.Repository, ProviderResource: input.ProviderRepository}, workItemExternalID(input), input.AuthorizeExternal), s.provenance)
}

func workItemExternalID(input cli.WorkItemInput) string {
	if input.ExternalID != "" {
		return input.ExternalID
	}
	return strconv.Itoa(input.Number)
}

func workflowTarget(input cli.WorkflowInput) workflow.Target {
	externalID := input.ExternalID
	if externalID == "" {
		externalID = strconv.Itoa(input.Number)
	}
	return workflow.Target{ProjectSelector: input.Project, RepositoryKey: input.Repository, WorkItemProvider: input.Provider, WorkItemResource: input.ProviderRepository, WorkItem: externalID, ExecutionID: input.Execution, RuntimeID: "codex"}
}

func (s lifecycleService) WorkflowStart(ctx context.Context, input cli.WorkflowInput) cli.Result {
	return workflowResult(s.workflows.Start(ctx, workflowTarget(input)), s.provenance)
}
func (s lifecycleService) WorkflowAdvance(ctx context.Context, input cli.WorkflowInput) cli.Result {
	references, ok := workflowReferences(input.Reference)
	if !ok {
		return workflowResult(workflow.Result{Status: workflow.ValidationFailed, Category: "invalid_workflow_reference"}, s.provenance)
	}
	return workflowResult(s.workflows.Transition(ctx, workflowTarget(input), workflow.TransitionInput{ExpectedRevision: input.ExpectedRevision, Stage: workflow.Stage(input.Gate), Outcome: workflow.Outcome(input.Outcome), References: references, Next: input.Next}), s.provenance)
}
func (s lifecycleService) WorkflowFact(ctx context.Context, input cli.WorkflowInput) cli.Result {
	references, ok := workflowReferences(input.Reference)
	if !ok || len(references) != 1 {
		return workflowResult(workflow.Result{Status: workflow.ValidationFailed, Category: "invalid_workflow_reference"}, s.provenance)
	}
	kinds := map[string]workflow.LifecycleFactKind{
		"planning-authority":       workflow.FactPlanningAuthority,
		"implementation-authority": workflow.FactImplementationAuthority,
		"review-started":           workflow.FactReviewStarted,
		"human-acceptance":         workflow.FactHumanAcceptance,
		"blocked":                  workflow.FactBlocked,
		"needs-decision":           workflow.FactNeedsDecision,
		"needs-approval":           workflow.FactNeedsApproval,
	}
	kind := kinds[input.Fact]
	if kind == "" {
		return workflowResult(workflow.Result{Status: workflow.ValidationFailed, Category: "invalid_lifecycle_fact"}, s.provenance)
	}
	return workflowResult(s.workflows.RecordLifecycleFact(ctx, workflowTarget(input), workflow.LifecycleFactInput{ExpectedRevision: input.ExpectedRevision, Kind: kind, Active: input.Active, Reference: references[0]}, input.AuthorizeLocal), s.provenance)
}
func (s lifecycleService) WorkflowResume(ctx context.Context, input cli.WorkflowInput) cli.Result {
	return workflowResult(s.workflows.Resume(ctx, workflowTarget(input), input.ExpectedRevision), s.provenance)
}
func (s lifecycleService) WorkflowStatus(ctx context.Context, input cli.WorkflowInput) cli.Result {
	return workflowResult(s.workflows.Status(ctx, workflowTarget(input)), s.provenance)
}
func (s lifecycleService) WorkflowEvidence(ctx context.Context, input cli.WorkflowInput) cli.Result {
	result := s.workflows.Status(ctx, workflowTarget(input))
	if result.Status == workflow.Succeeded {
		result.Category = "workflow_evidence_ready"
	}
	return workflowResult(result, s.provenance)
}

func (s lifecycleService) WorkflowReconcile(ctx context.Context, input cli.WorkflowInput) cli.Result {
	if !input.AuthorizeExternal {
		return workflowResult(s.workflows.PrepareProjection(ctx, workflowTarget(input), input.ExpectedRevision), s.provenance)
	}
	return workflowResult(s.workflows.Project(ctx, workflowTarget(input), input.ExpectedRevision, input.PreviewDigest, true), s.provenance)
}

func workflowReferences(value string) ([]workflow.Reference, bool) {
	if value == "" {
		return nil, true
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return nil, false
	}
	return []workflow.Reference{{Kind: parts[0], ID: parts[1], Digest: parts[2]}}, true
}

func workflowResult(result workflow.Result, source provenance.Value) cli.Result {
	facts := factsForCompletionStatus(result.Status)
	response := canonicalCompletion(facts, workflowResultText(result), workflowResultReferences(result), workflowResultNext(result), source)
	response.Category = result.Category
	response.Projection = result.Preview
	if result.State.ProjectID != "" {
		view := &cli.WorkflowView{ExecutionID: result.State.ExecutionID, WorkflowVersion: result.State.WorkflowVersion, Status: string(result.State.Status), CurrentGate: string(result.State.Stage), Revision: result.State.Revision, RepositoryKey: result.State.RepositoryKey, RuntimeID: result.State.RuntimeID, WorkItem: cli.WorkItemView{ProjectID: result.State.ProjectID, RepositoryKey: result.State.RepositoryKey, Provider: result.State.WorkItem.Provider, Resource: result.State.WorkItem.Resource, ExternalID: result.State.WorkItem.ExternalID, URL: result.State.WorkItem.URL, State: result.State.WorkItem.State}, Transitions: make([]cli.WorkflowStepView, 0, len(result.State.Transitions))}
		if lifecycle, err := workflow.DeriveLifecycle(result.State); err == nil {
			view.LifecycleStage = string(lifecycle.Stage)
			view.Blocked = lifecycle.Conditions.Blocked
			view.NeedsDecision = lifecycle.Conditions.NeedsDecision
			view.NeedsApproval = lifecycle.Conditions.NeedsApproval
		}
		for _, step := range result.State.Transitions {
			view.Transitions = append(view.Transitions, cli.WorkflowStepView{Revision: step.Revision, From: string(step.From), To: string(step.To), Outcome: string(step.Outcome), CommittedAt: step.CommittedAt.Format("2006-01-02T15:04:05.999999999Z07:00")})
		}
		response.Workflow = view
	}
	return response
}

func workflowResultText(result workflow.Result) string {
	if result.Category == "workflow_cancelled" {
		return "Execution workflow operation was cancelled"
	}
	if result.Status == workflow.Succeeded {
		return "Execution workflow operation completed"
	}
	if result.Status == workflow.Partial {
		return "Provider effect confirmed but projection bookkeeping is incomplete"
	}
	if result.Status == workflow.Retryable {
		return "Provider projection requires bounded reconciliation"
	}
	if result.Status == workflow.Denied {
		return "Execution authority is stale or incomplete"
	}
	if result.Status == workflow.Interrupted {
		return "Execution remains at the current workflow stage"
	}
	return "Execution workflow operation did not complete"
}
func workflowResultReferences(result workflow.Result) []string {
	refs := []string{}
	if result.State.ExecutionID != "" {
		refs = append(refs, "execution:"+result.State.ExecutionID)
	}
	if result.State.WorkItem.URL != "" && result.Status == workflow.Partial {
		refs = append(refs, "provider:"+result.State.WorkItem.URL)
	}
	return refs
}
func workflowResultNext(result workflow.Result) string {
	if result.Category == "workflow_cancelled" {
		return "Read current Execution status before retrying"
	}
	if result.Status == workflow.Partial || result.Status == workflow.Retryable {
		return "Re-read Provider state and reconcile the same projection key before retrying mutation"
	}
	if result.Status == workflow.Denied {
		return "Read current Execution status and prepare fresh exact authority"
	}
	if result.Status == workflow.Interrupted {
		return "Resume from the exact committed Execution revision"
	}
	return ""
}

func workItemResult(result workitem.Result, source provenance.Value) cli.Result {
	message, next := workItemResultText(result)
	facts := factsForCompletionStatus(result.Status)
	references := []string{}
	if result.Link.ExternalID != "" {
		references = append(references, "provider:"+result.Link.Provider+":"+result.Link.Resource+"#"+result.Link.ExternalID)
	}
	response := canonicalCompletion(facts, message, references, next, source)
	response.Category = result.Category
	response.Draft = result.Draft
	response.Selection = result.Selection
	response.Questions = result.Questions
	if result.Link.ExternalID != "" {
		response.WorkItem = &cli.WorkItemView{ProjectID: result.Link.ProjectID, RepositoryKey: result.Link.RepositoryKey, Provider: result.Link.Provider, Resource: result.Link.Resource, ExternalID: result.Link.ExternalID, URL: result.Link.URL, State: result.Link.State}
	}
	return response
}

func factsForCompletionStatus(status completion.Status) completion.Facts {
	switch status {
	case completion.Success:
		return completion.Facts{Completed: true}
	case completion.ValidationFailure:
		return completion.Facts{ValidationFailed: true}
	case completion.DeniedAuthority:
		return completion.Facts{AuthorityDenied: true}
	case completion.Partial:
		return completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}
	case completion.Interrupted:
		return completion.Facts{WasInterrupted: true}
	case completion.RetryableFailure:
		return completion.Facts{RetrySafeFailure: true}
	default:
		return completion.Facts{Failed: true}
	}
}

func workItemResultText(result workitem.Result) (string, string) {
	switch result.Category {
	case "work_item_draft_ready":
		return "Work Item draft ready for review", "Repeat create with this preview digest and explicit external authority"
	case "draft_incomplete":
		return "Work Item intent is incomplete", "Provide answers only for the listed missing fields"
	case "draft_cancelled", "create_cancelled":
		return "Work Item operation cancelled", "Resume with the same explicit inputs when ready"
	case "work_item_selection_ready":
		return "GitHub Work Item selection ready for review", "Repeat select with this preview digest and explicit local authority"
	case "external_authority_denied", "local_authority_denied", "external_mutation_denied":
		return "Work Item authority denied", "Review the exact preview and grant only the required authority"
	case "work_item_linked", "work_item_already_linked":
		return "GitHub Work Item linked", "Inspect the local Work Item link before starting later workflow work"
	case "work_item_loaded":
		return "Work Item link loaded", "Use only separately authorized later operations"
	case "work_item_commented":
		return "Historical Work Item comment completed", "Treat this as POC behavior until the later Slice replaces it"
	case "work_item_completed":
		return "Historical Work Item completion completed", "Treat this as POC behavior until the later Slice replaces it"
	case "provider_create_ambiguous":
		return "GitHub create result is ambiguous", "Repeat the same reviewed draft to reconcile only; Axiom will not create again while the durable attempt is pending"
	case "provider_rate_limited", "provider_unavailable":
		return "GitHub capability is temporarily unavailable", "Retry the same operation after provider recovery"
	case "local_create_attempt_recovery_required", "local_create_attempt_conflict", "local_create_attempt_failed":
		return "Create-attempt state is not safely writable", "Inspect protected local state; no GitHub create was issued by this execution"
	case "work_item_ambiguous":
		return "Work Item identity is ambiguous", "Specify and reconcile the exact provider resource before continuing"
	case "provider_confirmed_local_failed", "provider_confirmed_local_conflict", "provider_confirmed_local_recovery_required", "provider_confirmed_local_read_failed", "provider_confirmed_create_attempt_recovery_required", "local_link_committed_recovery_required":
		return "GitHub effect confirmed but local linkage is incomplete", "Preserve the Issue reference and reconcile local linkage before retrying create"
	default:
		return "Work Item operation did not complete", "Review bounded validation details and retry safely"
	}
}

func cliResult(result projectapp.LifecycleResult) cli.Result {
	status := cli.Failed
	if result.Status == projectapp.LifecycleApplied || result.Status == projectapp.LifecycleUnchanged {
		status = cli.Succeeded
	}
	if result.Status == projectapp.LifecycleCancelled {
		status = cli.Cancelled
	}
	return cli.Result{Status: status, Category: result.Category}
}

func canonicalCompletion(facts completion.Facts, message string, references []string, next string, source provenance.Value) cli.Result {
	result, ok := newCompletion(facts, message, references, next, source)
	if !ok {
		return cli.Result{Status: cli.Failed, Category: "application_unavailable"}
	}
	return result
}

func newCompletion(facts completion.Facts, message string, references []string, next string, source provenance.Value) (cli.Result, bool) {
	statement, err := provenance.NewText(message, provenance.AxiomAuthored)
	if err != nil {
		return cli.Result{}, false
	}
	var nextAction provenance.Text
	if next != "" {
		nextAction, err = provenance.NewText(next, provenance.AxiomAuthored)
		if err != nil {
			return cli.Result{}, false
		}
	}
	canonical, err := completion.New(facts, statement, references, nextAction, "", source)
	if err != nil {
		return cli.Result{}, false
	}
	return cli.Result{Completion: &canonical}, true
}
