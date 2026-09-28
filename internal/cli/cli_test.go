package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/workitem"
)

func TestRunDelegatesEachLifecycleOperation(t *testing.T) {
	service := &recordingService{}
	cases := []struct {
		name string
		args []string
		call string
	}{
		{"init", []string{"project", "init", "--slug", "alpha", "--name", "Alpha"}, "init:alpha:Alpha"},
		{"validate", []string{"project", "validate", "--slug", "alpha"}, "validate:alpha"},
		{"reopen", []string{"project", "reopen", "--slug", "alpha"}, "reopen:alpha"},
		{"update", []string{"project", "update", "--slug", "alpha", "--name", "Renamed"}, "update:alpha:Renamed"},
		{"runtime install", []string{"runtime", "codex", "install"}, "runtime-install"},
		{"runtime status", []string{"runtime", "codex", "status"}, "runtime-status"},
		{"claude install", []string{"runtime", "claude", "install"}, "runtime-claude-install"},
		{"claude status", []string{"runtime", "claude", "status"}, "runtime-claude-status"},
		{"first run", []string{"first-run"}, "first-run"},
		{"resolve", []string{"project", "resolve", "--selector", "alpha"}, "resolve:alpha"},
		{"show", []string{"project", "show", "--selector", "alpha"}, "resolve:alpha"},
		{"configure", []string{"project", "configure", "--slug", "alpha", "--name", "Alpha", "--repository", "main=/tmp/alpha"}, "configure:alpha:Alpha:main:/tmp/alpha"},
		{"work item select", []string{"work-item", "select", "--project", "alpha", "--repository", "main", "--provider-repository", "owner/repo", "--number", "7"}, "work-item-select:alpha:main:7"},
		{"work item exact", []string{"work-item", "show", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7"}, "work-item-show:alpha:main:github:owner/repo:7"},
		{"workflow advance", []string{"workflow", "advance", "--project", "alpha", "--repository", "main", "--number", "7", "--expected-revision", "1", "--gate", "intake", "--outcome", "pass"}, "workflow-advance:alpha:main:7:intake:pass:"},
		{"workflow exact", []string{"workflow", "status", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7", "--execution", "018f4a44-7c31-7dd4-9d00-111111111111"}, "workflow-status:alpha:main:github:owner/repo:7:018f4a44-7c31-7dd4-9d00-111111111111"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if code := Run(context.Background(), test.args, service, completionProvenance(t), &output); code != ExitSuccess {
				t.Fatalf("exit code = %d", code)
			}
			if service.call != test.call {
				t.Fatalf("call = %q, want %q", service.call, test.call)
			}
			operation := "first_run"
			if len(test.args) > 1 {
				operation = test.args[1]
			}
			if test.args[0] == "runtime" {
				operation = "runtime_" + test.args[1] + "_" + test.args[2]
			}
			if test.args[0] == "work-item" {
				operation = "work_item_" + test.args[1]
			}
			if test.args[0] == "workflow" {
				operation = "workflow_" + test.args[1]
			}
			assertEvent(t, output.String(), operation, Succeeded, "applied")
		})
	}
}

func TestStrictSelectorParserRejectsUnknownDuplicateAndConflictingInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown", []string{"workflow", "status", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7", "--execution", "execution", "--unknown", "value"}},
		{"duplicate", []string{"workflow", "status", "--project", "alpha", "--project", "other", "--repository", "main", "--work-item", "github:owner/repo#7", "--execution", "execution"}},
		{"single-hyphen duplicate project", []string{"workflow", "status", "-project", "alpha", "-project", "other", "--repository", "main", "--work-item", "github:owner/repo#7", "--execution", "execution"}},
		{"mixed-hyphen duplicate project", []string{"workflow", "status", "--project", "alpha", "-project", "other", "--repository", "main", "--work-item", "github:owner/repo#7", "--execution", "execution"}},
		{"single-hyphen duplicate execution", []string{"workflow", "status", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7", "-execution", "first", "-execution", "second"}},
		{"conflicting", []string{"workflow", "status", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7", "--number", "7", "--execution", "execution"}},
		{"unsafe selector", []string{"workflow", "status", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7;touch", "--execution", "execution"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &recordingService{}
			var output bytes.Buffer
			if code := Run(context.Background(), test.args, service, completionProvenance(t), &output); code != ExitFailure {
				t.Fatalf("exit=%d output=%s", code, output.String())
			}
			if service.call != "" {
				t.Fatalf("application called: %s", service.call)
			}
			var event completionEvent
			if err := json.Unmarshal(output.Bytes(), &event); err != nil || event.Status != completion.ValidationFailure {
				t.Fatalf("event=%+v err=%v output=%s", event, err, output.String())
			}
		})
	}
}

func TestStrictSelectorPromptsOnlyForMissingExecution(t *testing.T) {
	service := &recordingService{}
	var output, prompts bytes.Buffer
	input := strings.NewReader("018f4a44-7c31-7dd4-9d00-111111111111\n")
	code := RunInteractive(context.Background(), []string{"--json", "workflow", "status", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7"}, service, completionProvenance(t), input, &output, &prompts)
	if code != ExitSuccess || service.call != "workflow-status:alpha:main:github:owner/repo:7:018f4a44-7c31-7dd4-9d00-111111111111" {
		t.Fatalf("exit=%d call=%q output=%s", code, service.call, output.String())
	}
	if prompts.String() != "Execution ID: " {
		t.Fatalf("prompts=%q", prompts.String())
	}
}

func TestStrictSelectorCompleteInputAsksZeroQuestions(t *testing.T) {
	service := &recordingService{}
	var output, prompts bytes.Buffer
	code := RunInteractive(context.Background(), []string{"--json", "workflow", "status", "--project", "alpha", "--repository", "main", "--work-item", "github:owner/repo#7", "--execution", "018f4a44-7c31-7dd4-9d00-111111111111"}, service, completionProvenance(t), strings.NewReader(""), &output, &prompts)
	if code != ExitSuccess || prompts.Len() != 0 {
		t.Fatalf("exit=%d prompts=%q output=%s", code, prompts.String(), output.String())
	}
}

func TestRunFailsPromptlyWithoutRequiredInput(t *testing.T) {
	var output bytes.Buffer
	code := Run(context.Background(), []string{"project", "init", "--slug", "alpha"}, &recordingService{}, completionProvenance(t), &output)
	if code != ExitFailure {
		t.Fatalf("exit code = %d", code)
	}
	assertEvent(t, output.String(), "init", Failed, "missing_required_input")
}

func TestRunInteractiveGuidesProjectConfiguration(t *testing.T) {
	var output, prompts bytes.Buffer
	input := strings.NewReader("alpha\nAlpha\nmain=/tmp/alpha\n\nnone\n")
	code := RunInteractive(context.Background(), []string{"--json", "project", "configure"}, &recordingService{}, completionProvenance(t), input, &output, &prompts)
	if code != ExitSuccess {
		t.Fatalf("exit code = %d, output=%s", code, output.String())
	}
	assertEvent(t, output.String(), "configure", Succeeded, "applied")
	if !strings.Contains(prompts.String(), "Repository key=absolute-path") {
		t.Fatalf("prompts = %q", prompts.String())
	}
}

func TestRunInteractiveAsksOnlyMissingProvider(t *testing.T) {
	var output, prompts bytes.Buffer
	input := strings.NewReader("none\n")
	code := RunInteractive(context.Background(), []string{"--json", "project", "configure", "--slug", "alpha", "--name", "Alpha", "--repository", "main=/tmp/alpha"}, &recordingService{}, completionProvenance(t), input, &output, &prompts)
	if code != ExitSuccess {
		t.Fatalf("exit code = %d, output=%s", code, output.String())
	}
	if !strings.Contains(prompts.String(), "Work Item provider") || strings.Contains(prompts.String(), "Project slug") || strings.Contains(prompts.String(), "Project name") || strings.Contains(prompts.String(), "Repository key") {
		t.Fatalf("missing-only prompts = %q", prompts.String())
	}
}

func TestRunInteractiveGuidesWorkItemCreateAndBindsExactPreview(t *testing.T) {
	service := &guidedWorkItemService{completion: canonicalResult(t, completion.Success, []string{"operation:work_item_create"}, "Review result", completionProvenance(t))}
	input := strings.NewReader("alpha\nmain\nowner/repo\nObserved problem\nSafe outcome\nRelevant context\nBounded scope\nPreserve authority\nNo workflow\nTests pass\nyes\n")
	var output, prompts bytes.Buffer
	code := RunInteractive(context.Background(), []string{"--json", "work-item", "create"}, service, completionProvenance(t), input, &output, &prompts)
	if code != ExitSuccess || len(service.inputs) != 2 {
		t.Fatalf("exit=%d inputs=%#v output=%q", code, service.inputs, output.String())
	}
	if service.inputs[0].AuthorizeExternal || service.inputs[1].PreviewDigest != "reviewed-digest" || !service.inputs[1].AuthorizeExternal {
		t.Fatalf("authority inputs = %#v", service.inputs)
	}
	if !strings.Contains(prompts.String(), `"digest": "reviewed-digest"`) || !strings.Contains(prompts.String(), "Publish this create-attempt fence") {
		t.Fatalf("prompts = %q", prompts.String())
	}
}

func TestRunInteractiveWorkItemCancellationHasNoAuthority(t *testing.T) {
	service := &guidedWorkItemService{completion: canonicalResult(t, completion.Success, []string{"operation:work_item_create"}, "Review result", completionProvenance(t))}
	input := strings.NewReader("alpha\nmain\nowner/repo\nObserved problem\nSafe outcome\nRelevant context\nBounded scope\nPreserve authority\nNo workflow\nTests pass\nno\n")
	var output bytes.Buffer
	if code := RunInteractive(context.Background(), []string{"--json", "work-item", "create"}, service, completionProvenance(t), input, &output, io.Discard); code != ExitSuccess {
		t.Fatalf("exit=%d output=%q", code, output.String())
	}
	if len(service.inputs) != 2 || !service.inputs[1].Cancelled || service.inputs[1].AuthorizeExternal {
		t.Fatalf("cancellation inputs = %#v", service.inputs)
	}
}

func TestRunInteractiveDefaultsToHumanOutput(t *testing.T) {
	var output bytes.Buffer
	code := RunInteractive(context.Background(), []string{"project", "show", "--selector", "alpha"}, &recordingService{}, completionProvenance(t), nil, &output, io.Discard)
	if code != ExitSuccess || !strings.Contains(output.String(), "success: applied (show)") || !strings.Contains(output.String(), `repository main "/tmp/alpha"`) {
		t.Fatalf("human output = %q, code=%d", output.String(), code)
	}
}

func TestRunDoesNotExposeRejectedInput(t *testing.T) {
	const sentinel = "do-not-render-this-value"
	var output bytes.Buffer
	code := Run(context.Background(), []string{"project", "init", "--slug", "alpha", "--name", "Alpha", "--unexpected", sentinel}, &recordingService{}, completionProvenance(t), &output)
	if code != ExitFailure {
		t.Fatalf("exit code = %d", code)
	}
	if strings.Contains(output.String(), sentinel) {
		t.Fatalf("output exposed rejected input: %q", output.String())
	}
	assertEvent(t, output.String(), "init", Failed, "invalid_input")
}

func TestRunDoesNotExposeRejectedCommand(t *testing.T) {
	const sentinel = "do-not-render-this-command"
	var output bytes.Buffer
	code := Run(context.Background(), []string{"project", sentinel}, &recordingService{}, completionProvenance(t), &output)
	if code != ExitFailure {
		t.Fatalf("exit code = %d", code)
	}
	if strings.Contains(output.String(), sentinel) {
		t.Fatalf("output exposed rejected command: %q", output.String())
	}
	assertEvent(t, output.String(), "unknown", Failed, "invalid_command")
}

func TestCanonicalProjectParserFailuresDoNotCallApplicationServices(t *testing.T) {
	const sentinel = "do-not-render-this-value"
	tests := []struct {
		name       string
		args       []string
		wantResult string
		wantNext   string
	}{
		{"validate unknown flag", []string{"project", "validate", "--unknown", sentinel}, "Project validation input is invalid", "Review supported validation flags and retry"},
		{"show unknown flag", []string{"project", "show", "--unknown", sentinel}, "Explicit selector input is invalid", "Remove unknown, duplicate, or conflicting inputs and retry"},
		{"validate missing slug", []string{"project", "validate"}, "Project slug is required", "Provide a Project slug and retry validation"},
		{"show missing selector", []string{"project", "show"}, "Project selector is required", "Provide a Project UUID or slug and retry inspection"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &recordingService{}
			var structured bytes.Buffer
			code := RunInteractive(context.Background(), append([]string{"--json"}, test.args...), service, completionProvenance(t), nil, &structured, io.Discard)
			if code != ExitFailure {
				t.Fatalf("JSON exit code = %d", code)
			}
			if service.call != "" {
				t.Fatalf("application service called: %q", service.call)
			}
			if strings.Contains(structured.String(), sentinel) {
				t.Fatalf("JSON exposed rejected input: %q", structured.String())
			}
			var event completionEvent
			if err := json.Unmarshal(structured.Bytes(), &event); err != nil {
				t.Fatalf("canonical JSON = %q: %v", structured.String(), err)
			}
			if event.Status != completion.ValidationFailure || event.Result != test.wantResult || event.Next != test.wantNext {
				t.Fatalf("canonical event = %+v", event)
			}

			var human bytes.Buffer
			code = RunInteractive(context.Background(), append([]string{"--human"}, test.args...), service, completionProvenance(t), nil, &human, io.Discard)
			if code != ExitFailure || service.call != "" {
				t.Fatalf("human exit=%d application call=%q", code, service.call)
			}
			for _, expected := range []string{"status: validation_failure", "result: " + event.Result, "next: " + event.Next, "provenance: Axiom"} {
				if !strings.Contains(human.String(), expected) {
					t.Fatalf("human/JSON mismatch: %q absent from %q", expected, human.String())
				}
			}
		})
	}
}

func TestRunUsesCancelledExitCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canonical := canonicalResult(t, completion.Interrupted, []string{"operation:validation"}, "Retry Project validation", completionProvenance(t))
	service := &canonicalRecordingService{Result: Result{Completion: &canonical}}
	var output bytes.Buffer
	if code := Run(ctx, []string{"project", "validate", "--slug", "alpha"}, service, completionProvenance(t), &output); code != ExitCancelled {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(output.String(), `"status":"interrupted"`) {
		t.Fatalf("canonical interruption absent from %q", output.String())
	}
}

func assertEvent(t *testing.T, output, operation string, status Status, category string) {
	t.Helper()
	var value event
	if err := json.Unmarshal([]byte(output), &value); err != nil {
		t.Fatalf("output is not a JSON event: %v; output=%q", err, output)
	}
	if value.Operation != action(operation) || value.Status != status || value.Category != category {
		t.Fatalf("event = %#v", value)
	}
}

type recordingService struct{ call string }

type canonicalRecordingService struct {
	recordingService
	Result Result
}

type guidedWorkItemService struct {
	recordingService
	completion completion.Result
	inputs     []WorkItemInput
}

func (s *guidedWorkItemService) WorkItemCreate(_ context.Context, input WorkItemInput) Result {
	s.inputs = append(s.inputs, input)
	if input.Cancelled {
		return Result{Completion: &s.completion}
	}
	if !input.AuthorizeExternal {
		return Result{Completion: &s.completion, Draft: &workitem.DraftPreview{Digest: "reviewed-digest"}}
	}
	return Result{Completion: &s.completion}
}

func (s *canonicalRecordingService) Validate(context.Context, ProjectInput) Result { return s.Result }

func (s *recordingService) Init(_ context.Context, input InitInput) Result {
	s.call = "init:" + input.Slug + ":" + input.Name
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) Validate(_ context.Context, input ProjectInput) Result {
	s.call = "validate:" + input.Slug
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) Reopen(_ context.Context, input ProjectInput) Result {
	s.call = "reopen:" + input.Slug
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) Update(_ context.Context, input UpdateInput) Result {
	s.call = "update:" + input.Slug + ":" + input.Name
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) Install(_ context.Context, input InstallInput) Result {
	s.call = "install:" + input.Source
	return Result{Status: Succeeded, Category: "installed"}
}
func (s *recordingService) RuntimeCodexInstall(context.Context) Result {
	s.call = "runtime-install"
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) RuntimeCodexStatus(context.Context) Result {
	s.call = "runtime-status"
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) RuntimeClaudeInstall(context.Context) Result {
	s.call = "runtime-claude-install"
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) RuntimeClaudeStatus(context.Context) Result {
	s.call = "runtime-claude-status"
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) FirstRun(context.Context) Result {
	s.call = "first-run"
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) Resolve(_ context.Context, input ResolveInput) Result {
	s.call = "resolve:" + input.Selector
	return Result{Status: Succeeded, Category: "applied", Project: &ProjectView{ID: "123e4567-e89b-42d3-a456-426614174000", Slug: input.Selector, Repositories: []RepositoryView{{Key: "main", Path: "/tmp/alpha"}}}}
}
func (s *recordingService) Show(_ context.Context, input ResolveInput) Result {
	s.call = "resolve:" + input.Selector
	return Result{Status: Succeeded, Category: "applied", Project: &ProjectView{ID: "123e4567-e89b-42d3-a456-426614174000", Slug: input.Selector, Repositories: []RepositoryView{{Key: "main", Path: "/tmp/alpha"}}}}
}
func (s *recordingService) Configure(_ context.Context, input ConfigureInput) Result {
	repository := input.Repositories[0]
	s.call = "configure:" + input.Slug + ":" + input.Name + ":" + repository.Key + ":" + repository.Path
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkItemCreate(context.Context, WorkItemInput) Result {
	s.call = "work-item-create"
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkItemSelect(_ context.Context, input WorkItemInput) Result {
	s.call = "work-item-select:" + input.Project + ":" + input.Repository + ":" + fmt.Sprint(input.Number)
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkItemShow(_ context.Context, input WorkItemInput) Result {
	s.call = "work-item-show:" + input.Project + ":" + input.Repository + ":" + input.Provider + ":" + input.ProviderRepository + ":" + input.ExternalID
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkItemComment(context.Context, WorkItemInput) Result {
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkItemComplete(context.Context, WorkItemInput) Result {
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkflowStart(context.Context, WorkflowInput) Result {
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkflowAdvance(_ context.Context, input WorkflowInput) Result {
	s.call = "workflow-advance:" + input.Project + ":" + input.Repository + ":" + fmt.Sprint(input.Number) + ":" + input.Gate + ":" + input.Outcome + ":" + input.Reference
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkflowResume(context.Context, WorkflowInput) Result {
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkflowStatus(_ context.Context, input WorkflowInput) Result {
	s.call = "workflow-status:" + input.Project + ":" + input.Repository + ":" + input.Provider + ":" + input.ProviderRepository + ":" + input.ExternalID + ":" + input.Execution
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkflowEvidence(context.Context, WorkflowInput) Result {
	return Result{Status: Succeeded, Category: "applied"}
}
func (s *recordingService) WorkflowReconcile(context.Context, WorkflowInput) Result {
	return Result{Status: Succeeded, Category: "applied"}
}
