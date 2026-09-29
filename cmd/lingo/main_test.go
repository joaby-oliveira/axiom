package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/cli"
	"github.com/rgomids/axiom/internal/codexruntime"
	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/local"
	"github.com/rgomids/axiom/internal/projectapp"
	"github.com/rgomids/axiom/internal/provenance"
	"github.com/rgomids/axiom/internal/workflow"
	"github.com/rgomids/axiom/internal/workitem"
)

func TestComposedCLICompletesMinimalPortableLifecycle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()

	runCLI(t, service, []string{"project", "init", "--slug", "sample", "--name", "Sample"}, cli.ExitSuccess, "applied")
	runCanonicalCLI(t, service, []string{"project", "validate", "--slug", "sample"}, cli.ExitSuccess, "success", "Project is valid")
	runCLI(t, service, []string{"project", "reopen", "--slug", "sample"}, cli.ExitSuccess, "reopened_without_local_state")
	beforeInstall, err := os.ReadFile(filepath.Join(root, "sample", "axiom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	runCLI(t, service, []string{"project", "install", "--source", filepath.Join(root, "sample")}, cli.ExitSuccess, "installed")
	runCLI(t, service, []string{"project", "reopen", "--slug", "sample"}, cli.ExitSuccess, "reopened_with_local_state")
	afterInstall, err := os.ReadFile(filepath.Join(root, "sample", "axiom.yaml"))
	if err != nil || string(beforeInstall) != string(afterInstall) {
		t.Fatalf("install changed portable manifest: %v", err)
	}
	records, err := filepath.Glob(filepath.Join(state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("installation record paths = %v, %v", records, err)
	}
	runCLI(t, service, []string{"project", "update", "--slug", "sample", "--name", "Changed"}, cli.ExitSuccess, "applied")

	manifest, err := os.ReadFile(filepath.Join(root, "sample", "axiom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "name: Changed") {
		t.Fatalf("updated manifest does not contain new name: %q", manifest)
	}
}

func TestComposedCLIRejectsRelativeRoot(t *testing.T) {
	t.Setenv("LINGO_PROJECTS_ROOT", "relative")
	var output bytes.Buffer
	if code := cli.Run(context.Background(), []string{"project", "init", "--slug", "sample", "--name", "Sample"}, compose(), currentProvenance(), &output); code != cli.ExitFailure {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(output.String(), "application_unavailable") {
		t.Fatalf("unexpected error event: %q", output.String())
	}
}

func TestWorkflowSelectorAmbiguityFailsBeforeFallbackOrEffects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	runtimeRoot := filepath.Join(t.TempDir(), "skills")
	providerLedger := filepath.Join(t.TempDir(), "provider-called")
	ghBinary := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(ghBinary, []byte("#!/bin/sh\nprintf called >\"$AXIOM_TEST_PROVIDER_LEDGER\"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	t.Setenv("AXIOM_CODEX_SKILLS_ROOT", runtimeRoot)
	t.Setenv("AXIOM_GH_BIN", ghBinary)
	t.Setenv("AXIOM_TEST_PROVIDER_LEDGER", providerLedger)
	for _, id := range []string{"123e4567-e89b-42d3-a456-426614174000", "123e4567-e89b-42d3-a456-426614174001"} {
		source := filepath.Join(t.TempDir(), id)
		if err := os.Mkdir(source, 0o700); err != nil {
			t.Fatal(err)
		}
		record, issues := local.NewRecord(local.RecordState{
			ProjectID:        id,
			ObservedSlug:     "duplicate",
			SourceLocation:   source,
			PortableRevision: projectapp.RecordedPortableRevision([32]byte{1}),
			ArtifactDigests:  []projectapp.ArtifactDigest{{Name: "axiom.yaml", Digest: [32]byte{1}}},
		})
		if len(issues) != 0 {
			t.Fatal(issues)
		}
		wire, issues := local.EncodeRecord(record)
		if len(issues) != 0 {
			t.Fatal(issues)
		}
		directory := filepath.Join(state, "projects", id)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "installation.json"), wire, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotTrees(t, state)
	runCanonicalCLI(t, compose(), []string{"workflow", "status", "--project", "duplicate", "--repository", "main", "--work-item", "github:owner/repo#7", "--execution", "018f4a44-7c31-7dd4-9d00-111111111111"}, cli.ExitFailure, "validation_failure", "Execution workflow operation did not complete")
	after := snapshotTrees(t, state)
	if !bytes.Equal(before, after) {
		t.Fatalf("ambiguous selector mutated local state\nbefore=%s\nafter=%s", before, after)
	}
	for _, path := range []string{root, runtimeRoot, providerLedger} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("ambiguous selector used fallback or caused effect at %s: %v", path, err)
		}
	}
}

func TestConfigurePublishesPortableKeysAndLocalPathsThenResolves(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	configureProject(t, service, "configured", "Configured", "main="+repository, "github")
	preview := previewProject(t, service, "configured", "Configured", "main="+repository, "github")
	if len(preview.Effects) != 0 {
		t.Fatalf("equivalent replay effects = %v", preview.Effects)
	}
	runCLI(t, service, []string{"project", "resolve", "--selector", "configured"}, cli.ExitSuccess, "project_resolved")
	var shown bytes.Buffer
	if code := cli.Run(context.Background(), []string{"project", "show", "--selector", "configured"}, service, currentProvenance(), &shown); code != cli.ExitSuccess {
		t.Fatalf("project show exit=%d output=%s", code, shown.String())
	}
	var showEvent struct {
		Project *struct {
			Slug         string `json:"slug"`
			Repositories []struct {
				Key string `json:"key"`
			} `json:"repositories"`
		} `json:"project"`
	}
	if err := json.Unmarshal(shown.Bytes(), &showEvent); err != nil {
		t.Fatalf("project show JSON: %v: %s", err, shown.String())
	}
	if showEvent.Project == nil || showEvent.Project.Slug != "configured" || len(showEvent.Project.Repositories) != 1 || showEvent.Project.Repositories[0].Key != "main" {
		t.Fatalf("project show payload = %+v", showEvent.Project)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(root, "configured", "axiom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifestText := string(manifestBytes)
	if !strings.Contains(manifestText, "key: main") || strings.Contains(manifestText, repository) {
		t.Fatalf("portable/local boundary violated: %s", manifestText)
	}
	records, err := filepath.Glob(filepath.Join(state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %v, %v", records, err)
	}
	recordBytes, err := os.ReadFile(records[0])
	if err != nil || !bytes.Contains(recordBytes, []byte(repository)) {
		t.Fatalf("local repository binding absent: %s, %v", recordBytes, err)
	}
}

func TestConfigureInvalidRepositoriesPublishNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	runCanonicalCLI(t, service, []string{"project", "configure", "--slug", "configured", "--name", "Configured", "--repository", "main=" + repository, "--repository", "main=" + repository}, cli.ExitFailure, "validation_failure", "Project setup input is invalid")
	if _, err := os.Stat(filepath.Join(root, "configured")); !os.IsNotExist(err) {
		t.Fatalf("invalid configuration published portable state: %v", err)
	}
}

func TestGuidedSetupDenialPublishesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	input := strings.NewReader("denied\nDenied\nmain=" + repository + "\n\ngithub\nno\n")
	var output, prompts bytes.Buffer
	code := cli.RunInteractive(context.Background(), []string{"--json", "project", "configure"}, compose(), currentProvenance(), input, &output, &prompts)
	if code != cli.ExitFailure || !strings.Contains(output.String(), `"status":"denied_authority"`) {
		t.Fatalf("denial code=%d output=%s prompts=%s", code, output.String(), prompts.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("denied setup created portable root: %v", err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("denied setup created local root: %v", err)
	}
}

func TestConfigurePreviewIsReadOnlyAndAuthorityBindsExactDigest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	api := filepath.Join(t.TempDir(), "api")
	web := filepath.Join(t.TempDir(), "web")
	for _, path := range []string{api, web} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	preview := previewProject(t, service, "multi", "Multi", "web="+web, "github")
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("preview created portable root: %v", err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("preview created local root: %v", err)
	}
	staleArgs := []string{"project", "configure", "--project-id", preview.ProjectID, "--slug", "multi", "--name", "Multi", "--repository", "web=" + web, "--repository", "api=" + api, "--work-item-provider", "github", "--preview-digest", preview.Digest, "--authorize-local"}
	runCanonicalCLI(t, service, staleArgs, cli.ExitFailure, "denied_authority", "Project setup authority is missing or stale")
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("stale authority created portable root: %v", err)
	}
	freshResult := service.Configure(context.Background(), cli.ConfigureInput{ProjectID: preview.ProjectID, Slug: "multi", Name: "Multi", Repositories: []cli.RepositoryInput{{Key: "web", Path: web}, {Key: "api", Path: api}}, WorkItemProvider: "github"})
	if freshResult.Setup == nil || freshResult.Completion == nil || freshResult.Completion.Status() != completion.Success {
		t.Fatalf("fresh preview = %#v", freshResult)
	}
	args := []string{"project", "configure", "--project-id", preview.ProjectID, "--slug", "multi", "--name", "Multi", "--repository", "web=" + web, "--repository", "api=" + api, "--work-item-provider", "github", "--preview-digest", freshResult.Setup.Digest, "--authorize-local"}
	runCanonicalCLI(t, service, args, cli.ExitSuccess, "success", "Project setup published")
	manifestBytes, err := os.ReadFile(filepath.Join(root, "multi", "axiom.yaml"))
	if err != nil || bytes.Contains(manifestBytes, []byte(api)) || bytes.Contains(manifestBytes, []byte(web)) {
		t.Fatalf("portable/local separation failed: %v, %s", err, manifestBytes)
	}
	records, err := filepath.Glob(filepath.Join(state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("local records = %v, %v", records, err)
	}
	recordBytes, err := os.ReadFile(records[0])
	if err != nil || !bytes.Contains(recordBytes, []byte(api)) || !bytes.Contains(recordBytes, []byte(web)) {
		t.Fatalf("independent bindings missing: %v, %s", err, recordBytes)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	runCLI(t, service, []string{"project", "resolve", "--selector", preview.ProjectID}, cli.ExitSuccess, "project_resolved")
	if err := os.Rename(api, api+"-prior"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(api, 0o700); err != nil {
		t.Fatal(err)
	}
	runCanonicalCLI(t, service, []string{"project", "show", "--selector", preview.ProjectID}, cli.ExitFailure, "retryable_failure", "Project repository is unavailable")
}

func TestConfigureRefusesRepositoryReplacementAfterPreview(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	preview := previewProject(t, service, "replaced", "Replaced", "main="+repository, "github")
	prior := repository + "-prior"
	if err := os.Rename(repository, prior); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	changed := time.Unix(1_900_000_000, 0)
	if err := os.Chtimes(repository, changed, changed); err != nil {
		t.Fatal(err)
	}
	args := []string{"project", "configure", "--project-id", preview.ProjectID, "--slug", "replaced", "--name", "Replaced", "--repository", "main=" + repository, "--work-item-provider", "github", "--preview-digest", preview.Digest, "--authorize-local"}
	runCanonicalCLI(t, service, args, cli.ExitFailure, "denied_authority", "Project setup authority is missing or stale")
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("repository replacement published state: %v", err)
	}
}

func TestConfigureReportsCommittedPortableStateWhenLocalPublicationFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose().(lifecycleService)
	preview := previewProject(t, service, "configured", "Configured", "main="+repository, "github")
	service.beforeLocalPublication = func() {
		if err := os.Symlink(t.TempDir(), state); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"project", "configure", "--project-id", preview.ProjectID, "--slug", "configured", "--name", "Configured", "--repository", "main=" + repository, "--work-item-provider", "github", "--preview-digest", preview.Digest, "--authorize-local"}
	runCanonicalCLI(t, service, args, cli.ExitFailure, "partial", "Portable Project published; local bindings did not complete")
	if _, err := os.Stat(filepath.Join(root, "configured", "axiom.yaml")); err != nil {
		t.Fatalf("committed portable state not reported truthfully: %v", err)
	}
}

func TestWorkItemJourneyRequiresReadyConfiguredCapability(t *testing.T) {
	for _, provider := range []string{"", "linear"} {
		t.Run("provider-"+provider, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "projects")
			state := filepath.Join(t.TempDir(), "state")
			repository := filepath.Join(t.TempDir(), "repository")
			if err := os.Mkdir(repository, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("LINGO_PROJECTS_ROOT", root)
			t.Setenv("LINGO_STATE_ROOT", state)
			service := compose()
			configureProject(t, service, "capability", "Capability", "main="+repository, provider)
			result := service.WorkItemCreate(context.Background(), cli.WorkItemInput{Project: "capability", Repository: "main", ProviderRepository: "owner/repo", Intent: "Blocked", AuthorizeExternal: true})
			if result.Completion == nil || result.Completion.Status() != completion.ValidationFailure || result.Category != "work_item_capability_unavailable" {
				t.Fatalf("provider %q work item result = %#v", provider, result)
			}
		})
	}
}

func TestFirstRunReportsMissingReadyAndIncompatibleSkillStates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	skills := filepath.Join(t.TempDir(), "skills")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	t.Setenv("AXIOM_CODEX_SKILLS_ROOT", skills)
	service := compose().(lifecycleService)
	missing := service.RuntimeCodexStatus(context.Background())
	if missing.Completion == nil || missing.Completion.Status() != completion.ValidationFailure || missing.Runtime == nil || len(missing.Runtime.Skills) != 5 || missing.Runtime.Skills[0].State != "missing" {
		t.Fatalf("missing first run = %#v", missing)
	}
	if installed := service.RuntimeCodexInstall(context.Background()); installed.Status != cli.Succeeded || installed.Runtime == nil || len(installed.Runtime.Skills) != 5 {
		t.Fatalf("skill install = %#v", installed)
	}
	ready := service.RuntimeCodexStatus(context.Background())
	if ready.Completion == nil || ready.Completion.Status() != completion.Success || ready.Runtime == nil || ready.Runtime.Skills[0].State != "equivalent" {
		t.Fatalf("ready first run = %#v", ready)
	}
	incompatibleRuntime, err := codexruntime.NewForBinary(skills, "3")
	if err != nil {
		t.Fatal(err)
	}
	service.codex = incompatibleRuntime
	incompatible := service.RuntimeCodexStatus(context.Background())
	if incompatible.Completion == nil || incompatible.Completion.Status() != completion.ValidationFailure || incompatible.Runtime == nil || incompatible.Runtime.BinaryCompatibility != "3" {
		t.Fatalf("incompatible first run = %#v", incompatible)
	}
}

func TestVersionReportsAxiomSourceMetadata(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "version")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	source, err := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: provenance.Unavailable, SourceState: provenance.Unknown}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code := writeVersion(file, cli.CompletionJSON, source); code != cli.ExitSuccess {
		t.Fatalf("version exit code = %d", code)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"status":"success"`, `"result":"Axiom build information"`, `"product":"Axiom"`, `"version":"development"`, `"revision":"unavailable"`, `"sourceState":"unknown"`} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Fatalf("version output missing %q: %s", expected, data)
		}
	}
}

func TestProjectShowClassifiesResolutionCauses(t *testing.T) {
	tests := []struct {
		category   string
		wantStatus completion.Status
		wantResult string
		wantNext   string
	}{
		{"project_not_found", completion.ValidationFailure, "Project was not found", "Provide an existing Project UUID or slug"},
		{"repository_unavailable", completion.RetryableFailure, "Project repository is unavailable", "Restore the configured repository binding and retry inspection"},
		{"invalid_existing_local_state", completion.ValidationFailure, "Local Project state is invalid", "Repair or reconfigure local Project state before retrying inspection"},
		{"recovery_required", completion.ValidationFailure, "Local Project state requires recovery", "Review preserved local recovery state before retrying inspection"},
		{"cancelled", completion.Interrupted, "Project inspection was interrupted", "Retry Project inspection"},
		{"storage_failure", completion.Failure, "Project inspection failed", "Inspect local storage and application availability before retrying"},
		{"application_unavailable", completion.Failure, "Project inspection failed", "Inspect local storage and application availability before retrying"},
	}

	for _, test := range tests {
		t.Run(test.category, func(t *testing.T) {
			result := projectShowFailure(test.category, currentProvenance())
			if result.Completion == nil {
				t.Fatal("canonical completion absent")
			}
			if result.Completion.Status() != test.wantStatus || result.Completion.Result().String() != test.wantResult || result.Completion.Next().String() != test.wantNext {
				t.Fatalf("completion = status=%s result=%q next=%q", result.Completion.Status(), result.Completion.Result().String(), result.Completion.Next().String())
			}
		})
	}
}

func TestWorkItemFailureRendersCommittedExternalState(t *testing.T) {
	result := workItemResult(workitem.Result{
		Status:   completion.Partial,
		Category: "provider_confirmed_local_failed",
		Link: workitem.Link{
			ProjectID: "123e4567-e89b-42d3-a456-426614174000", RepositoryKey: "main",
			Provider: "github", Resource: "owner/repo", ExternalID: "7",
			URL: "https://github.com/owner/repo/issues/7", State: "CLOSED",
		},
	}, currentProvenance())
	if result.WorkItem == nil || result.WorkItem.ProjectID == "" || result.WorkItem.RepositoryKey != "main" || result.WorkItem.Provider != "github" || result.WorkItem.Resource != "owner/repo" || result.WorkItem.ExternalID != "7" || result.WorkItem.State != "CLOSED" {
		t.Fatalf("result = %#v", result)
	}
	payload, err := json.Marshal(result.WorkItem)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"projectId":"123e4567-e89b-42d3-a456-426614174000"`, `"repositoryKey":"main"`, `"provider":"github"`, `"resource":"owner/repo"`, `"externalId":"7"`, `"url":"https://github.com/owner/repo/issues/7"`, `"state":"CLOSED"`} {
		if !bytes.Contains(payload, []byte(expected)) {
			t.Fatalf("work item payload missing %q: %s", expected, payload)
		}
	}
}

func TestWorkflowPartialRendersCommittedExternalState(t *testing.T) {
	result := workflowResult(workflow.Result{
		Status:   workflow.Partial,
		Category: "provider_confirmed_projection_bookkeeping_failed",
		State: workflow.State{
			ExecutionID: "018f4a44-7c31-7dd4-9d00-111111111111",
			ProjectID:   "123e4567-e89b-42d3-a456-426614174000", RepositoryKey: "main",
			WorkItem: workflow.WorkItem{Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"},
		},
	}, currentProvenance())
	if result.Workflow == nil || result.Workflow.WorkItem.ProjectID == "" || result.Workflow.WorkItem.RepositoryKey != "main" || result.Workflow.WorkItem.Provider != "github" || result.Workflow.WorkItem.Resource != "owner/repo" || result.Workflow.WorkItem.ExternalID != "7" || result.Workflow.WorkItem.State != "OPEN" {
		t.Fatalf("result = %#v", result)
	}
}

func TestStateRootOverride(t *testing.T) {
	t.Setenv("LINGO_STATE_ROOT", "/tmp/lingo-state/../lingo-state")
	got, err := stateRoot()
	if err != nil || got != "/tmp/lingo-state" {
		t.Fatalf("stateRoot override = %q, %v", got, err)
	}
}

func TestStateRootRejectsInvalidOverride(t *testing.T) {
	for _, value := range []string{"", "relative/state", "/"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("LINGO_STATE_ROOT", value)
			if root, err := stateRoot(); err == nil || root != "" {
				t.Fatalf("stateRoot accepted invalid override: %q, %v", root, err)
			}
		})
	}
}

func TestCompositionRejectsOverlappingRootsBeforeCreation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", filepath.Join(root, "state"))
	var output bytes.Buffer
	if code := cli.Run(context.Background(), []string{"project", "init", "--slug", "sample", "--name", "Sample"}, compose(), currentProvenance(), &output); code != cli.ExitFailure {
		t.Fatalf("overlapping roots accepted: code=%d", code)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("portable root created before root validation: %v", err)
	}
}

func TestCompositionRejectsSymlinkAliasBeforeCreation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", filepath.Join(alias, "state"))
	var output bytes.Buffer
	if code := cli.Run(context.Background(), []string{"project", "init", "--slug", "sample", "--name", "Sample"}, compose(), currentProvenance(), &output); code != cli.ExitFailure {
		t.Fatalf("alias accepted: %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "state")); !os.IsNotExist(err) {
		t.Fatalf("state created through alias: %v", err)
	}
}

func TestInstallRejectsUnsafeSourceWithoutChangingPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	runCLI(t, service, []string{"project", "init", "--slug", "sample", "--name", "Sample"}, cli.ExitSuccess, "applied")
	source := filepath.Join(root, "sample")
	if err := os.Chmod(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitFailure, "unsafe_source")
	info, err := os.Stat(source)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("install mutated source mode: %v, %v", info, err)
	}
}

func TestInstallRejectsSymlinkRecordWithoutWritingOutside(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	runCLI(t, service, []string{"project", "init", "--slug", "sample", "--name", "Sample"}, cli.ExitSuccess, "applied")
	source := filepath.Join(root, "sample")
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitSuccess, "installed")
	records, err := filepath.Glob(filepath.Join(state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %v, %v", records, err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(records[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, records[0]); err != nil {
		t.Fatal(err)
	}
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitFailure, "invalid_existing_local_state")
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatalf("outside changed: %q, %v", data, err)
	}
}

func TestInstallRejectsHardLinkedRecordWithoutWritingOutside(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	runCLI(t, service, []string{"project", "init", "--slug", "sample", "--name", "Sample"}, cli.ExitSuccess, "applied")
	source := filepath.Join(root, "sample")
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitSuccess, "installed")
	records, err := filepath.Glob(filepath.Join(state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %v, %v", records, err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(records[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, records[0]); err != nil {
		t.Fatal(err)
	}
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitFailure, "invalid_existing_local_state")
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatalf("outside changed: %q, %v", data, err)
	}
}

func TestInstallPreservesUnknownLocalArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	runCLI(t, service, []string{"project", "init", "--slug", "sample", "--name", "Sample"}, cli.ExitSuccess, "applied")
	source := filepath.Join(root, "sample")
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitSuccess, "installed")
	records, err := filepath.Glob(filepath.Join(state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %v, %v", records, err)
	}
	before, err := os.ReadFile(records[0])
	if err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(filepath.Dir(records[0]), "unknown")
	if err := os.WriteFile(unknown, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitFailure, "invalid_existing_local_state")
	runCLI(t, service, []string{"project", "reopen", "--slug", "sample"}, cli.ExitFailure, "invalid_existing_local_state")
	if after, err := os.ReadFile(records[0]); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("record changed: %v", err)
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "keep" {
		t.Fatalf("unknown artifact changed: %q, %v", data, err)
	}
}

func TestValidateDoesNotCreateRootsOrLockFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	runCanonicalCLI(t, service, []string{"project", "validate", "--slug", "missing"}, cli.ExitFailure, "validation_failure", "Project state is invalid")
	for _, path := range []string{root, state} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("validate created %q: %v", path, err)
		}
	}
	runCLI(t, service, []string{"project", "init", "--slug", "sample", "--name", "Sample"}, cli.ExitSuccess, "applied")
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	runCanonicalCLI(t, service, []string{"project", "validate", "--slug", "sample"}, cli.ExitSuccess, "success", "Project is valid")
	after, err := os.ReadDir(root)
	if err != nil || len(after) != len(before) {
		t.Fatalf("validate changed root entries: %v, %v", after, err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("validate created local state: %v", err)
	}
}

func TestCanonicalReadOnlySurfacesDoNotMutateState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	configureProject(t, service, "sample", "Sample", "main="+repository, "github")
	before := snapshotTrees(t, root, state, repository)

	for _, args := range [][]string{
		{"--human", "project", "validate", "--slug", "sample"},
		{"--json", "project", "validate", "--slug", "sample"},
		{"--human", "project", "show", "--selector", "sample"},
		{"--json", "project", "show", "--selector", "sample"},
	} {
		var output bytes.Buffer
		if code := cli.RunInteractive(context.Background(), args, service, currentProvenance(), nil, &output, &bytes.Buffer{}); code != cli.ExitSuccess {
			t.Fatalf("%v: exit=%d output=%q", args, code, output.String())
		}
		if !strings.Contains(output.String(), "Axiom") {
			t.Fatalf("%v: provenance absent from %q", args, output.String())
		}
	}
	after := snapshotTrees(t, root, state, repository)
	if !bytes.Equal(before, after) {
		t.Fatalf("read-only canonical surfaces mutated state\nbefore=%s\nafter=%s", before, after)
	}
}

func snapshotTrees(t *testing.T, roots ...string) []byte {
	t.Helper()
	entries := make([]string, 0)
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			entry := root + ":" + relative + ":" + info.Mode().String()
			if info.Mode().IsRegular() {
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				entry += ":" + string(content)
			}
			entries = append(entries, entry)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(entries)
	return []byte(strings.Join(entries, "\n"))
}

func TestInterruptedInstallRequiresRecoveryAndPreservesPortableBytes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("LINGO_PROJECTS_ROOT", root)
	t.Setenv("LINGO_STATE_ROOT", state)
	service := compose()
	runCLI(t, service, []string{"project", "init", "--slug", "sample", "--name", "Sample"}, cli.ExitSuccess, "applied")
	source := filepath.Join(root, "sample")
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitSuccess, "installed")
	manifest, err := os.ReadFile(filepath.Join(source, "axiom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := filepath.Glob(filepath.Join(state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %v, %v", records, err)
	}
	stage := filepath.Join(filepath.Dir(records[0]), ".lingo-install-interrupted")
	if err := os.WriteFile(stage, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	runCLI(t, service, []string{"project", "reopen", "--slug", "sample"}, cli.ExitFailure, "recovery_required")
	runCLI(t, service, []string{"project", "install", "--source", source}, cli.ExitFailure, "recovery_required")
	if current, err := os.ReadFile(filepath.Join(source, "axiom.yaml")); err != nil || !bytes.Equal(current, manifest) {
		t.Fatalf("portable bytes changed: %v", err)
	}
	if current, err := os.ReadFile(stage); err != nil || string(current) != "partial" {
		t.Fatalf("unknown attempt removed: %q, %v", current, err)
	}
}

func runCLI(t *testing.T, service cli.Service, args []string, wantCode int, wantCategory string) {
	t.Helper()
	var output bytes.Buffer
	if code := cli.Run(context.Background(), args, service, currentProvenance(), &output); code != wantCode {
		t.Fatalf("%v: exit code = %d, output=%q", args, code, output.String())
	}
	if !strings.Contains(output.String(), `"category":"`+wantCategory+`"`) {
		t.Fatalf("%v: category absent from %q", args, output.String())
	}
}

func runCanonicalCLI(t *testing.T, service cli.Service, args []string, wantCode int, wantStatus, wantResult string) {
	t.Helper()
	var output bytes.Buffer
	if code := cli.Run(context.Background(), args, service, currentProvenance(), &output); code != wantCode {
		t.Fatalf("%v: exit code = %d, output=%q", args, code, output.String())
	}
	for _, expected := range []string{`"status":"` + wantStatus + `"`, `"result":"` + wantResult + `"`, `"provenance":{`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("%v: %q absent from %q", args, expected, output.String())
		}
	}
}

func previewProject(t *testing.T, service cli.Service, slug, name, repository, provider string) projectapp.SetupPreview {
	t.Helper()
	args := []string{"project", "configure", "--slug", slug, "--name", name, "--repository", repository}
	if provider != "" {
		args = append(args, "--work-item-provider", provider)
	}
	var output bytes.Buffer
	if code := cli.Run(context.Background(), args, service, currentProvenance(), &output); code != cli.ExitSuccess {
		t.Fatalf("preview: code=%d output=%s", code, output.String())
	}
	var event struct {
		Setup projectapp.SetupPreview `json:"setup"`
	}
	if err := json.Unmarshal(output.Bytes(), &event); err != nil || event.Setup.Digest == "" || event.Setup.ProjectID == "" {
		t.Fatalf("preview output = %s, %v", output.String(), err)
	}
	return event.Setup
}

func configureProject(t *testing.T, service cli.Service, slug, name, repository, provider string) projectapp.SetupPreview {
	t.Helper()
	preview := previewProject(t, service, slug, name, repository, provider)
	args := []string{"project", "configure", "--project-id", preview.ProjectID, "--slug", slug, "--name", name, "--repository", repository, "--preview-digest", preview.Digest, "--authorize-local"}
	if provider != "" {
		args = append(args, "--work-item-provider", provider)
	}
	var output bytes.Buffer
	if code := cli.Run(context.Background(), args, service, currentProvenance(), &output); code != cli.ExitSuccess || !strings.Contains(output.String(), `"status":"success"`) {
		t.Fatalf("publish: code=%d output=%s", code, output.String())
	}
	return preview
}
