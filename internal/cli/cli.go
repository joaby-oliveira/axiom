// Package cli is Lingo's presentation boundary for Project lifecycle actions.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"io"
	"strconv"
	"strings"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/projectapp"
	"github.com/rgomids/axiom/internal/provenance"
	"github.com/rgomids/axiom/internal/workflow"
	"github.com/rgomids/axiom/internal/workitem"
)

const (
	ExitSuccess   = 0
	ExitFailure   = 1
	ExitCancelled = 2
)

// Service is deliberately operation-shaped. Presentation can request a lifecycle
// action but cannot issue a generic command or a raw filesystem mutation.
type Service interface {
	Init(context.Context, InitInput) Result
	Validate(context.Context, ProjectInput) Result
	Reopen(context.Context, ProjectInput) Result
	Update(context.Context, UpdateInput) Result
	Install(context.Context, InstallInput) Result
	RuntimeCodexInstall(context.Context) Result
	RuntimeCodexStatus(context.Context) Result
	RuntimeClaudeInstall(context.Context) Result
	RuntimeClaudeStatus(context.Context) Result
	FirstRun(context.Context) Result
	Resolve(context.Context, ResolveInput) Result
	Show(context.Context, ResolveInput) Result
	Configure(context.Context, ConfigureInput) Result
	WorkItemCreate(context.Context, WorkItemInput) Result
	WorkItemSelect(context.Context, WorkItemInput) Result
	WorkItemShow(context.Context, WorkItemInput) Result
	WorkItemComment(context.Context, WorkItemInput) Result
	WorkItemComplete(context.Context, WorkItemInput) Result
	WorkflowStart(context.Context, WorkflowInput) Result
	WorkflowAdvance(context.Context, WorkflowInput) Result
	WorkflowResume(context.Context, WorkflowInput) Result
	WorkflowStatus(context.Context, WorkflowInput) Result
	WorkflowEvidence(context.Context, WorkflowInput) Result
	WorkflowReconcile(context.Context, WorkflowInput) Result
}

type InitInput struct {
	Slug string
	Name string
}

type ProjectInput struct{ Slug string }

type UpdateInput struct {
	Slug string
	Name string
}
type InstallInput struct{ Source string }
type ResolveInput struct{ Selector string }
type RepositoryInput struct{ Key, Path string }
type ConfigureInput struct {
	ProjectID, Slug, Name string
	Repositories          []RepositoryInput
	WorkItemProvider      string
	PreviewDigest         string
	AuthorizeLocal        bool
}
type WorkItemInput struct {
	Project, Repository, WorkItem            string
	Provider, ProviderRepository, ExternalID string
	Intent, Problem, DesiredOutcome, Context string
	Scope, Constraints, NonGoals, Acceptance string
	Message, PreviewDigest                   string
	Number                                   int
	AuthorizeExternal, AuthorizeLocal        bool
	Cancelled                                bool
}
type WorkflowInput struct {
	Project, Repository, WorkItem, Provider, ProviderRepository string
	ExternalID, Execution, Gate, Outcome, Reference, Next       string
	Fact                                                        string
	Number                                                      int
	ExpectedRevision                                            uint64
	PreviewDigest                                               string
	AuthorizeExternal, AuthorizeLocal                           bool
	Active                                                      bool
}

type LifecycleFactService interface {
	WorkflowFact(context.Context, WorkflowInput) Result
}

type Status string

const (
	Succeeded Status = "success"
	Failed    Status = "error"
	Cancelled Status = "cancelled"
)

// Result only permits fixed, presentation-safe categories. Application adapters
// must not return raw paths, input values, parser output, or operating-system
// error strings for this surface.
type Result struct {
	Status     Status
	Category   string
	Project    *ProjectView
	WorkItem   *WorkItemView
	Workflow   *WorkflowView
	Completion *completion.Result
	Setup      *projectapp.SetupPreview
	Runtime    *RuntimeView
	Bootstrap  *BootstrapView
	Draft      *workitem.DraftPreview
	Selection  *workitem.SelectionPreview
	Questions  []workitem.Question
	Projection *workflow.ProjectionPreview
	// Maintenance is a bounded, content-free view for compatibility,
	// cleanup, recovery, and upgrade previews and results.
	Maintenance any
}

type RuntimeSkillView struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	State  string `json:"state"`
}
type RuntimeView struct {
	SkillSetVersion     string             `json:"skillSetVersion"`
	BinaryCompatibility string             `json:"binaryCompatibility"`
	Skills              []RuntimeSkillView `json:"skills"`
}

// RuntimeBootstrapView is one supported Runtime in a first-run report.
type RuntimeBootstrapView struct {
	Runtime                        string             `json:"runtime"`
	Executable                     string             `json:"executable"`
	Present                        bool               `json:"present"`
	ConfigurationWithoutExecutable bool               `json:"configurationWithoutExecutable"`
	State                          string             `json:"state"`
	Reason                         string             `json:"reason"`
	SkillSetVersion                string             `json:"skillSetVersion,omitempty"`
	Skills                         []RuntimeSkillView `json:"skills,omitempty"`
}

// BootstrapView reports every supported Runtime after first run.
type BootstrapView struct {
	Detected int                    `json:"detected"`
	Failed   int                    `json:"failed"`
	Runtimes []RuntimeBootstrapView `json:"runtimes"`
}

type RepositoryView struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}
type ProjectView struct {
	ID           string           `json:"id"`
	Slug         string           `json:"slug"`
	Source       string           `json:"source"`
	Repositories []RepositoryView `json:"repositories"`
}
type WorkItemView struct {
	ProjectID     string `json:"projectId"`
	RepositoryKey string `json:"repositoryKey"`
	Provider      string `json:"provider"`
	Resource      string `json:"resource"`
	ExternalID    string `json:"externalId"`
	URL           string `json:"url"`
	State         string `json:"state"`
}
type WorkflowStepView struct {
	Revision    uint64 `json:"revision"`
	From        string `json:"from"`
	To          string `json:"to"`
	Outcome     string `json:"outcome"`
	CommittedAt string `json:"committedAt"`
}
type WorkflowView struct {
	ExecutionID     string             `json:"executionId"`
	WorkflowVersion string             `json:"workflowVersion"`
	Status          string             `json:"status"`
	CurrentGate     string             `json:"currentGate"`
	LifecycleStage  string             `json:"lifecycleStage,omitempty"`
	Blocked         bool               `json:"blocked,omitempty"`
	NeedsDecision   bool               `json:"needsDecision,omitempty"`
	NeedsApproval   bool               `json:"needsApproval,omitempty"`
	Revision        uint64             `json:"revision"`
	RepositoryKey   string             `json:"repositoryKey"`
	WorkItem        WorkItemView       `json:"workItem"`
	RuntimeID       string             `json:"runtimeId"`
	Transitions     []WorkflowStepView `json:"transitions"`
}

// Run parses one CLI action, delegates it, and emits one safe structured event.
// It never turns a parser error into user-visible text because parser text may
// contain rejected input.
func Run(ctx context.Context, args []string, service Service, source provenance.Value, stdout io.Writer) int {
	structured := append([]string{"--json"}, args...)
	return RunInteractive(ctx, structured, service, source, nil, stdout, io.Discard)
}

// RunInteractive adds the bounded prompt path used by `project configure` and
// selects human or machine-readable presentation without changing application behavior.
func RunInteractive(ctx context.Context, args []string, service Service, source provenance.Value, stdin io.Reader, stdout, stderr io.Writer) int {
	mode, args := parseOutputMode(args)
	if service == nil {
		return emit(stdout, mode, event{Operation: "unknown", Status: Failed, Category: "application_unavailable"})
	}
	if operation, rest, ok := maintenanceAction(args); ok {
		return runMaintenance(ctx, mode, operation, rest, service, source, stdout)
	}
	if len(args) >= 2 && args[0] == "project" && args[1] == "configure" && stdin != nil {
		values, ok := flags(configureAction, args[2:])
		if !ok {
			return emitParserFailure(stdout, mode, configureAction, "invalid_input", source)
		}
		if values.slug == "" || values.name == "" || len(values.repositories) == 0 || !flagSupplied(args[2:], "--work-item-provider") {
			return runInteractiveConfiguration(ctx, mode, args[2:], service, stdin, stdout, stderr)
		}
	}
	if len(args) >= 2 && args[0] == "work-item" && args[1] == "create" && stdin != nil {
		values, ok := workItemFlags(workItemCreateAction, args[2:])
		if !ok {
			return emitParserFailure(stdout, mode, workItemCreateAction, "invalid_input", source)
		}
		if !completeWorkItemCreate(values) {
			return runInteractiveWorkItemCreate(ctx, mode, values, service, stdin, stdout, stderr)
		}
	}
	if len(args) >= 2 && args[0] == "work-item" && args[1] != "create" && stdin != nil {
		operation := action("work_item_" + args[1])
		values, ok := workItemFlags(operation, args[2:])
		if knownWorkItem(operation) && ok && values.number == 0 && (values.project == "" || values.repository == "" || values.workItem == "") {
			return runInteractiveSelectors(ctx, mode, operation, values, service, source, stdin, stdout, stderr)
		}
	}
	if len(args) >= 2 && args[0] == "workflow" && stdin != nil {
		operation := action("workflow_" + args[1])
		values, ok := workflowFlags(operation, args[2:])
		needsExecution := operation != workflowStartAction
		if knownWorkflow(operation) && ok && values.number == 0 && (values.project == "" || values.repository == "" || values.workItem == "" || needsExecution && values.execution == "") {
			return runInteractiveSelectors(ctx, mode, operation, values, service, source, stdin, stdout, stderr)
		}
	}
	operation, input, result := request(args, service)
	if result != nil {
		if operation == validateAction || operation == showAction || selectorAction(operation) || *result == "invalid_input" && (operation == configureAction || operation == resolveAction) {
			return emitParserFailure(stdout, mode, operation, *result, source)
		}
		return emit(stdout, mode, event{Operation: operation, Status: Failed, Category: *result})
	}
	response := dispatch(ctx, operation, input, service)
	return emitResponse(stdout, mode, operation, response)
}

func flagSupplied(args []string, name string) bool {
	for _, value := range args {
		if value == name || strings.HasPrefix(value, name+"=") {
			return true
		}
	}
	return false
}

func emitParserFailure(writer io.Writer, mode outputMode, operation action, issue string, source provenance.Value) int {
	message, next := parserFailureText(operation, issue)
	statement, err := provenance.NewText(message, provenance.AxiomAuthored)
	if err != nil {
		return ExitFailure
	}
	nextAction, err := provenance.NewText(next, provenance.AxiomAuthored)
	if err != nil {
		return ExitFailure
	}
	result, err := completion.NewValidationFailure(statement, nextAction, source)
	if err != nil {
		return ExitFailure
	}
	return emitCompletion(writer, mode, result)
}

func parserFailureText(operation action, issue string) (string, string) {
	if issue == "invalid_input" && (operation == showAction || operation == resolveAction || operation == configureAction) {
		return "Explicit selector input is invalid", "Remove unknown, duplicate, or conflicting inputs and retry"
	}
	if operation == validateAction && issue == "missing_required_input" {
		return "Project slug is required", "Provide a Project slug and retry validation"
	}
	if operation == showAction && issue == "missing_required_input" {
		return "Project selector is required", "Provide a Project UUID or slug and retry inspection"
	}
	if operation == validateAction {
		return "Project validation input is invalid", "Review supported validation flags and retry"
	}
	if selectorAction(operation) {
		if issue == "missing_required_input" {
			return "Explicit selectors are incomplete", "Provide only the missing Project, Repository, Work Item, or Execution selector"
		}
		return "Explicit selector input is invalid", "Remove unknown, duplicate, or conflicting inputs and retry"
	}
	return "Project inspection input is invalid", "Review supported inspection flags and retry"
}

func selectorAction(operation action) bool {
	return knownWorkItem(operation) || knownWorkflow(operation)
}

func emitResponse(writer io.Writer, mode outputMode, operation action, response Result) int {
	if response.Completion != nil {
		if response.Project != nil {
			return emitProjectCompletion(writer, mode, *response.Completion, *response.Project)
		}
		if response.Setup != nil {
			return emitSetupCompletion(writer, mode, *response.Completion, *response.Setup)
		}
		if response.Runtime != nil {
			return emitRuntimeCompletion(writer, mode, *response.Completion, *response.Runtime)
		}
		if response.Bootstrap != nil {
			return emitBootstrapCompletion(writer, mode, *response.Completion, *response.Bootstrap)
		}
		if response.Draft != nil || response.Selection != nil || response.WorkItem != nil || len(response.Questions) != 0 {
			return emitWorkItemCompletion(writer, mode, *response.Completion, response)
		}
		if response.Workflow != nil || response.Projection != nil {
			return emitWorkflowCompletion(writer, mode, *response.Completion, response)
		}
		return emitCompletion(writer, mode, *response.Completion)
	}
	return emit(writer, mode, eventFrom(operation, response))
}

type action string

const (
	initAction              action = "init"
	validateAction          action = "validate"
	reopenAction            action = "reopen"
	updateAction            action = "update"
	installAction           action = "install"
	resolveAction           action = "resolve"
	showAction              action = "show"
	configureAction         action = "configure"
	workItemCreateAction    action = "work_item_create"
	workItemSelectAction    action = "work_item_select"
	workItemShowAction      action = "work_item_show"
	workItemCommentAction   action = "work_item_comment"
	workItemCompleteAction  action = "work_item_complete"
	workflowStartAction     action = "workflow_start"
	workflowAdvanceAction   action = "workflow_advance"
	workflowFactAction      action = "workflow_fact"
	workflowResumeAction    action = "workflow_resume"
	workflowStatusAction    action = "workflow_status"
	workflowEvidenceAction  action = "workflow_evidence"
	workflowReconcileAction action = "workflow_reconcile"
	codexInstallAction      action = "runtime_codex_install"
	codexStatusAction       action = "runtime_codex_status"
	claudeInstallAction     action = "runtime_claude_install"
	claudeStatusAction      action = "runtime_claude_status"
	firstRunAction          action = "first_run"
)

type requestInput struct {
	slug                                     string
	name                                     string
	projectID                                string
	source                                   string
	selector                                 string
	repositories                             repositoryFlags
	workItemProvider                         string
	previewDigest                            string
	project, repository, workItem, execution string
	providerRepository                       string
	intent, problem, desiredOutcome, context string
	scope, constraints, nonGoals, acceptance string
	message                                  string
	gate, outcome, reference, next, fact     string
	number                                   int
	expectedRevision                         uint64
	authorizeExternal                        bool
	authorizeLocal                           bool
	active                                   bool
}

func request(args []string, service Service) (action, requestInput, *string) {
	if service == nil {
		return "unknown", requestInput{}, category("application_unavailable")
	}
	if len(args) == 1 && args[0] == "first-run" {
		return firstRunAction, requestInput{}, nil
	}
	if len(args) == 3 && args[0] == "runtime" && (args[1] == "codex" || args[1] == "claude") {
		operation := action("runtime_" + args[1] + "_" + args[2])
		if operation == codexInstallAction || operation == codexStatusAction || operation == claudeInstallAction || operation == claudeStatusAction {
			return operation, requestInput{}, nil
		}
		return "unknown", requestInput{}, category("invalid_command")
	}
	if len(args) >= 2 && args[0] == "work-item" {
		operation := action("work_item_" + args[1])
		if !knownWorkItem(operation) {
			return "unknown", requestInput{}, category("invalid_command")
		}
		values, ok := workItemFlags(operation, args[2:])
		if !ok {
			return operation, requestInput{}, category("invalid_input")
		}
		if issue := selectorRequestIssue(operation, values); issue != "" {
			return operation, values, category(issue)
		}
		return operation, values, nil
	}
	if len(args) >= 2 && args[0] == "workflow" {
		operation := action("workflow_" + args[1])
		if !knownWorkflow(operation) {
			return "unknown", requestInput{}, category("invalid_command")
		}
		values, ok := workflowFlags(operation, args[2:])
		if !ok {
			return operation, requestInput{}, category("invalid_input")
		}
		if issue := selectorRequestIssue(operation, values); issue != "" {
			return operation, values, category(issue)
		}
		return operation, values, nil
	}
	if len(args) < 2 || args[0] != "project" {
		return "unknown", requestInput{}, category("invalid_command")
	}
	operation := action(args[1])
	if !known(operation) {
		return "unknown", requestInput{}, category("invalid_command")
	}
	values, ok := flags(operation, args[2:])
	if !ok {
		return operation, requestInput{}, category("invalid_input")
	}
	if operation == initAction && (values.slug == "" || values.name == "") {
		return operation, values, category("missing_required_input")
	}
	if operation == installAction && values.source == "" {
		return operation, values, category("missing_required_input")
	}
	if (operation == resolveAction || operation == showAction) && values.selector == "" {
		return operation, values, category("missing_required_input")
	}
	if operation == configureAction && (values.slug == "" || values.name == "" || len(values.repositories) == 0) {
		return operation, values, category("missing_required_input")
	}
	if operation != initAction && operation != installAction && operation != resolveAction && operation != showAction && values.slug == "" {
		return operation, values, category("missing_required_input")
	}
	if operation == updateAction && values.name == "" {
		return operation, values, category("missing_required_input")
	}
	return operation, values, nil
}

func selectorRequestIssue(operation action, values requestInput) string {
	if values.workItem != "" && (values.number != 0 || values.providerRepository != "") {
		return "invalid_input"
	}
	if values.project == "" || values.repository == "" {
		return "missing_required_input"
	}
	if knownWorkItem(operation) {
		if operation == workItemCreateAction {
			if values.workItem != "" {
				return "invalid_input"
			}
			if values.providerRepository == "" {
				return "missing_required_input"
			}
			return ""
		}
		if values.workItem == "" && values.number <= 0 {
			return "missing_required_input"
		}
		if values.workItem != "" {
			if _, _, _, ok := parseWorkItemSelector(values.workItem); !ok {
				return "invalid_input"
			}
		}
		if operation == workItemSelectAction && values.workItem == "" && values.providerRepository == "" {
			return "missing_required_input"
		}
		if operation == workItemCommentAction && values.message == "" {
			return "missing_required_input"
		}
		return ""
	}
	if values.workItem == "" && values.number <= 0 {
		return "missing_required_input"
	}
	if values.workItem != "" {
		if _, _, _, ok := parseWorkItemSelector(values.workItem); !ok {
			return "invalid_input"
		}
		if operation == workflowStartAction && values.execution != "" {
			return "invalid_input"
		}
		if operation != workflowStartAction && values.execution == "" {
			return "missing_required_input"
		}
	}
	needsRevision := operation == workflowAdvanceAction || operation == workflowFactAction || operation == workflowResumeAction || operation == workflowReconcileAction
	if needsRevision && values.expectedRevision == 0 || operation == workflowAdvanceAction && (values.gate == "" || values.outcome == "") {
		return "missing_required_input"
	}
	if operation == workflowFactAction && (values.fact == "" || values.reference == "") {
		return "missing_required_input"
	}
	return ""
}

func flags(operation action, args []string) (requestInput, bool) {
	set := flag.NewFlagSet(string(operation), flag.ContinueOnError)
	set.SetOutput(io.Discard)
	var values requestInput
	set.StringVar(&values.slug, "slug", "", "")
	if operation == initAction || operation == updateAction {
		set.StringVar(&values.name, "name", "", "")
	}
	if operation == installAction {
		set.StringVar(&values.source, "source", "", "")
	}
	if operation == resolveAction || operation == showAction {
		set.StringVar(&values.selector, "selector", "", "")
	}
	if operation == configureAction {
		set.StringVar(&values.projectID, "project-id", "", "")
		set.StringVar(&values.name, "name", "", "")
		set.Var(&values.repositories, "repository", "")
		set.StringVar(&values.workItemProvider, "work-item-provider", "", "")
		set.StringVar(&values.previewDigest, "preview-digest", "", "")
		set.BoolVar(&values.authorizeLocal, "authorize-local", false, "")
	}
	if invalidFlagSyntax(set, args, map[string]bool{"repository": true}) {
		return requestInput{}, false
	}
	if err := set.Parse(args); err != nil || set.NArg() != 0 {
		return requestInput{}, false
	}
	return values, true
}

func workItemFlags(operation action, args []string) (requestInput, bool) {
	set := flag.NewFlagSet(string(operation), flag.ContinueOnError)
	set.SetOutput(io.Discard)
	var values requestInput
	set.StringVar(&values.project, "project", "", "")
	set.StringVar(&values.repository, "repository", "", "")
	set.StringVar(&values.providerRepository, "provider-repository", "", "")
	set.StringVar(&values.workItem, "work-item", "", "")
	set.StringVar(&values.previewDigest, "preview-digest", "", "")
	set.BoolVar(&values.authorizeExternal, "authorize-external", false, "")
	set.BoolVar(&values.authorizeLocal, "authorize-local", false, "")
	if operation == workItemCreateAction {
		set.StringVar(&values.intent, "intent", "", "")
		set.StringVar(&values.problem, "problem", "", "")
		set.StringVar(&values.desiredOutcome, "desired-outcome", "", "")
		set.StringVar(&values.context, "context", "", "")
		set.StringVar(&values.scope, "scope", "", "")
		set.StringVar(&values.constraints, "constraints", "", "")
		set.StringVar(&values.nonGoals, "non-goals", "", "")
		set.StringVar(&values.acceptance, "acceptance", "", "")
	} else {
		set.IntVar(&values.number, "number", 0, "")
	}
	if operation == workItemCommentAction {
		set.StringVar(&values.message, "message", "", "")
	}
	if invalidFlagSyntax(set, args, nil) {
		return requestInput{}, false
	}
	if err := set.Parse(args); err != nil || set.NArg() != 0 || values.workItem != "" && (values.providerRepository != "" || values.number != 0) {
		return requestInput{}, false
	}
	return values, true
}

func knownWorkItem(operation action) bool {
	return operation == workItemCreateAction || operation == workItemSelectAction || operation == workItemShowAction || operation == workItemCommentAction || operation == workItemCompleteAction
}

func workflowFlags(operation action, args []string) (requestInput, bool) {
	set := flag.NewFlagSet(string(operation), flag.ContinueOnError)
	set.SetOutput(io.Discard)
	var values requestInput
	set.StringVar(&values.project, "project", "", "")
	set.StringVar(&values.repository, "repository", "", "")
	set.StringVar(&values.workItem, "work-item", "", "")
	set.StringVar(&values.execution, "execution", "", "")
	set.IntVar(&values.number, "number", 0, "")
	if operation == workflowAdvanceAction || operation == workflowFactAction || operation == workflowResumeAction || operation == workflowReconcileAction {
		set.Uint64Var(&values.expectedRevision, "expected-revision", 0, "")
	}
	if operation == workflowReconcileAction {
		set.StringVar(&values.previewDigest, "preview-digest", "", "")
		set.BoolVar(&values.authorizeExternal, "authorize-external", false, "")
	}
	if operation == workflowAdvanceAction {
		set.StringVar(&values.gate, "gate", "", "")
		set.StringVar(&values.outcome, "outcome", "", "")
		set.StringVar(&values.reference, "reference", "", "")
		set.StringVar(&values.next, "next", "", "")
	}
	if operation == workflowFactAction {
		set.StringVar(&values.fact, "fact", "", "")
		set.BoolVar(&values.active, "active", false, "")
		set.StringVar(&values.reference, "reference", "", "")
		set.BoolVar(&values.authorizeLocal, "authorize-local", false, "")
	}
	if invalidFlagSyntax(set, args, nil) {
		return requestInput{}, false
	}
	if err := set.Parse(args); err != nil || set.NArg() != 0 || values.workItem != "" && values.number != 0 {
		return requestInput{}, false
	}
	return values, true
}

func invalidFlagSyntax(set *flag.FlagSet, args []string, repeatable map[string]bool) bool {
	seen := make(map[string]bool)
	for index := 0; index < len(args); index++ {
		value := args[index]
		// Go flag accepts single-hyphen aliases and silently overwrites duplicates.
		// Reject unsupported syntax before handing any arguments to it.
		if value == "--" || !strings.HasPrefix(value, "--") {
			return true
		}
		name := strings.TrimPrefix(value, "--")
		hasValue := false
		if separator := strings.IndexByte(name, '='); separator >= 0 {
			name = name[:separator]
			hasValue = true
		}
		current := set.Lookup(name)
		if current == nil {
			return true
		}
		if !repeatable[name] && seen[name] {
			return true
		}
		seen[name] = true
		boolean, isBoolean := current.Value.(interface{ IsBoolFlag() bool })
		if hasValue || isBoolean && boolean.IsBoolFlag() {
			continue
		}
		if index+1 >= len(args) {
			return true
		}
		index++
	}
	return false
}

func parseWorkItemSelector(value string) (string, string, string, bool) {
	provider, target, found := strings.Cut(value, ":")
	resource, externalID, numbered := strings.Cut(target, "#")
	if !found || !numbered || provider != "github" || !validProviderResource(resource) {
		return "", "", "", false
	}
	number, err := strconv.Atoi(externalID)
	if err != nil || number <= 0 || strconv.Itoa(number) != externalID {
		return "", "", "", false
	}
	return provider, resource, externalID, true
}

func validProviderResource(value string) bool {
	owner, repository, found := strings.Cut(value, "/")
	if !found || owner == "" || repository == "" || strings.Contains(repository, "/") || owner == "." || owner == ".." || repository == "." || repository == ".." {
		return false
	}
	for _, part := range []string{owner, repository} {
		for _, character := range part {
			if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("_.-", character) {
				continue
			}
			return false
		}
	}
	return true
}

func knownWorkflow(operation action) bool {
	return operation == workflowStartAction || operation == workflowAdvanceAction || operation == workflowFactAction || operation == workflowResumeAction || operation == workflowStatusAction || operation == workflowEvidenceAction || operation == workflowReconcileAction
}

func known(operation action) bool {
	return operation == initAction || operation == validateAction || operation == reopenAction || operation == updateAction || operation == installAction || operation == resolveAction || operation == showAction || operation == configureAction
}

func dispatch(ctx context.Context, operation action, input requestInput, service Service) Result {
	if err := ctx.Err(); err != nil {
		if operation == validateAction {
			return service.Validate(ctx, ProjectInput{Slug: input.slug})
		}
		if operation == showAction {
			return service.Show(ctx, ResolveInput{Selector: input.selector})
		}
		return Result{Status: Cancelled, Category: "cancelled"}
	}
	switch operation {
	case initAction:
		return service.Init(ctx, InitInput{Slug: input.slug, Name: input.name})
	case validateAction:
		return service.Validate(ctx, ProjectInput{Slug: input.slug})
	case reopenAction:
		return service.Reopen(ctx, ProjectInput{Slug: input.slug})
	case updateAction:
		return service.Update(ctx, UpdateInput{Slug: input.slug, Name: input.name})
	case installAction:
		return service.Install(ctx, InstallInput{Source: input.source})
	case resolveAction:
		return service.Resolve(ctx, ResolveInput{Selector: input.selector})
	case showAction:
		return service.Show(ctx, ResolveInput{Selector: input.selector})
	case configureAction:
		repositories, ok := parseRepositories(input.repositories)
		if !ok {
			return Result{Status: Failed, Category: "invalid_input"}
		}
		provider := input.workItemProvider
		if provider == "none" {
			provider = ""
		}
		return service.Configure(ctx, ConfigureInput{ProjectID: input.projectID, Slug: input.slug, Name: input.name, Repositories: repositories, WorkItemProvider: provider, PreviewDigest: input.previewDigest, AuthorizeLocal: input.authorizeLocal})
	case workItemCreateAction, workItemSelectAction, workItemShowAction, workItemCommentAction, workItemCompleteAction:
		provider, resource, externalID, ok := parseWorkItemSelector(input.workItem)
		if input.workItem != "" && !ok {
			return Result{Status: Failed, Category: "invalid_input"}
		}
		value := WorkItemInput{Project: input.project, Repository: input.repository, WorkItem: input.workItem, Provider: provider, ProviderRepository: resource, ExternalID: externalID, Intent: input.intent, Problem: input.problem, DesiredOutcome: input.desiredOutcome, Context: input.context, Scope: input.scope, Constraints: input.constraints, NonGoals: input.nonGoals, Acceptance: input.acceptance, Message: input.message, PreviewDigest: input.previewDigest, Number: input.number, AuthorizeExternal: input.authorizeExternal, AuthorizeLocal: input.authorizeLocal}
		if input.workItem == "" {
			value.ProviderRepository = input.providerRepository
			value.ExternalID = strconv.Itoa(input.number)
		}
		switch operation {
		case workItemCreateAction:
			return service.WorkItemCreate(ctx, value)
		case workItemSelectAction:
			return service.WorkItemSelect(ctx, value)
		case workItemShowAction:
			return service.WorkItemShow(ctx, value)
		case workItemCommentAction:
			return service.WorkItemComment(ctx, value)
		default:
			return service.WorkItemComplete(ctx, value)
		}
	case workflowStartAction, workflowAdvanceAction, workflowFactAction, workflowResumeAction, workflowStatusAction, workflowEvidenceAction, workflowReconcileAction:
		provider, resource, externalID, ok := parseWorkItemSelector(input.workItem)
		if input.workItem != "" && !ok {
			return Result{Status: Failed, Category: "invalid_input"}
		}
		value := WorkflowInput{Project: input.project, Repository: input.repository, WorkItem: input.workItem, Provider: provider, ProviderRepository: resource, ExternalID: externalID, Execution: input.execution, Number: input.number, Gate: input.gate, Outcome: input.outcome, Reference: input.reference, Next: input.next, Fact: input.fact, Active: input.active, ExpectedRevision: input.expectedRevision, PreviewDigest: input.previewDigest, AuthorizeExternal: input.authorizeExternal, AuthorizeLocal: input.authorizeLocal}
		switch operation {
		case workflowStartAction:
			return service.WorkflowStart(ctx, value)
		case workflowAdvanceAction:
			return service.WorkflowAdvance(ctx, value)
		case workflowFactAction:
			factService, ok := service.(LifecycleFactService)
			if !ok {
				return Result{Status: Failed, Category: "application_unavailable"}
			}
			return factService.WorkflowFact(ctx, value)
		case workflowResumeAction:
			return service.WorkflowResume(ctx, value)
		case workflowStatusAction:
			return service.WorkflowStatus(ctx, value)
		case workflowReconcileAction:
			return service.WorkflowReconcile(ctx, value)
		default:
			return service.WorkflowEvidence(ctx, value)
		}
	case codexInstallAction:
		return service.RuntimeCodexInstall(ctx)
	case codexStatusAction:
		return service.RuntimeCodexStatus(ctx)
	case claudeInstallAction:
		return service.RuntimeClaudeInstall(ctx)
	case claudeStatusAction:
		return service.RuntimeClaudeStatus(ctx)
	case firstRunAction:
		return service.FirstRun(ctx)
	}
	return Result{Status: Failed, Category: "invalid_command"}
}

type event struct {
	Operation  action                      `json:"operation"`
	Status     Status                      `json:"status"`
	Category   string                      `json:"category"`
	Project    *ProjectView                `json:"project,omitempty"`
	WorkItem   *WorkItemView               `json:"workItem,omitempty"`
	Workflow   *WorkflowView               `json:"workflow,omitempty"`
	Setup      *projectapp.SetupPreview    `json:"setup,omitempty"`
	Runtime    *RuntimeView                `json:"runtime,omitempty"`
	Projection *workflow.ProjectionPreview `json:"projection,omitempty"`
}

type outputMode string

const (
	humanOutput outputMode = "human"
	jsonOutput  outputMode = "json"
)

func parseOutputMode(args []string) (outputMode, []string) {
	if len(args) > 0 && args[0] == "--json" {
		return jsonOutput, args[1:]
	}
	if len(args) > 0 && args[0] == "--human" {
		return humanOutput, args[1:]
	}
	return humanOutput, args
}

func eventFrom(operation action, result Result) event {
	return event{Operation: operation, Status: result.Status, Category: result.Category, Project: result.Project, WorkItem: result.WorkItem, Workflow: result.Workflow, Setup: result.Setup, Runtime: result.Runtime, Projection: result.Projection}
}

func emit(writer io.Writer, mode outputMode, value event) int {
	if writer == nil {
		return ExitFailure
	}
	if mode == jsonOutput {
		_ = json.NewEncoder(writer).Encode(value)
	} else {
		emitHuman(writer, value)
	}
	if value.Status == Succeeded {
		return ExitSuccess
	}
	if value.Status == Cancelled {
		return ExitCancelled
	}
	return ExitFailure
}

func emitHuman(writer io.Writer, value event) {
	_, _ = io.WriteString(writer, string(value.Status)+": "+value.Category+" ("+string(value.Operation)+")\n")
	if value.Project != nil {
		_, _ = io.WriteString(writer, "project "+value.Project.Slug+" ["+value.Project.ID+"]\n")
		for _, repository := range value.Project.Repositories {
			_, _ = io.WriteString(writer, "repository "+repository.Key+" "+strconv.Quote(repository.Path)+"\n")
		}
	}
	if value.WorkItem != nil {
		_, _ = io.WriteString(writer, "work-item "+value.WorkItem.URL+" ["+value.WorkItem.State+"] project="+value.WorkItem.ProjectID+" repository-key="+value.WorkItem.RepositoryKey+" provider="+value.WorkItem.Provider+" resource="+value.WorkItem.Resource+" external-id="+value.WorkItem.ExternalID+"\n")
	}
	if value.Workflow != nil {
		_, _ = io.WriteString(writer, "execution "+value.Workflow.ExecutionID+" "+value.Workflow.Status+" current="+value.Workflow.CurrentGate+" revision="+strconv.FormatUint(value.Workflow.Revision, 10)+"\n")
	}
	if value.Runtime != nil {
		emitRuntimeHuman(writer, *value.Runtime)
	}
}

func category(value string) *string { return &value }

type repositoryFlags []string

func (r *repositoryFlags) String() string { return strings.Join(*r, ",") }
func (r *repositoryFlags) Set(value string) error {
	*r = append(*r, value)
	return nil
}

func parseRepositories(values []string) ([]RepositoryInput, bool) {
	result := make([]RepositoryInput, 0, len(values))
	for _, value := range values {
		key, path, ok := strings.Cut(value, "=")
		if !ok || key == "" || path == "" {
			return nil, false
		}
		result = append(result, RepositoryInput{Key: key, Path: path})
	}
	return result, true
}

func completeWorkItemCreate(values requestInput) bool {
	return values.project != "" && values.repository != "" && values.providerRepository != "" &&
		(values.intent != "" || values.problem != "") && values.desiredOutcome != "" && values.context != "" &&
		values.scope != "" && values.constraints != "" && values.nonGoals != "" && values.acceptance != ""
}

func runInteractiveSelectors(ctx context.Context, mode outputMode, operation action, values requestInput, service Service, source provenance.Value, input io.Reader, stdout, prompts io.Writer) int {
	scanner := bufio.NewScanner(input)
	fields := []struct {
		value  *string
		prompt string
	}{
		{&values.project, "Project UUID or slug: "},
		{&values.repository, "Project repository key: "},
		{&values.workItem, "Work Item selector (github:owner/repository#number): "},
	}
	if knownWorkflow(operation) && operation != workflowStartAction {
		fields = append(fields, struct {
			value  *string
			prompt string
		}{&values.execution, "Execution ID: "})
	}
	for _, field := range fields {
		if *field.value != "" {
			continue
		}
		value, ok := readPromptLine(scanner, prompts, field.prompt, true)
		if !ok {
			return emitParserFailure(stdout, mode, operation, "missing_required_input", source)
		}
		*field.value = value
	}
	if issue := selectorRequestIssue(operation, values); issue != "" {
		return emitParserFailure(stdout, mode, operation, issue, source)
	}
	return emitResponse(stdout, mode, operation, dispatch(ctx, operation, values, service))
}

func runInteractiveWorkItemCreate(ctx context.Context, mode outputMode, values requestInput, service Service, input io.Reader, stdout, prompts io.Writer) int {
	scanner := bufio.NewScanner(input)
	fields := []struct {
		value  *string
		prompt string
	}{
		{&values.project, "Project UUID or slug: "},
		{&values.repository, "Project repository key: "},
		{&values.providerRepository, "GitHub repository owner/name: "},
	}
	for _, field := range fields {
		if *field.value != "" {
			continue
		}
		value, ok := readPromptLine(scanner, prompts, field.prompt, true)
		if !ok {
			return emitResponse(stdout, mode, workItemCreateAction, service.WorkItemCreate(ctx, WorkItemInput{Project: values.project, Repository: values.repository, ProviderRepository: values.providerRepository, Cancelled: true}))
		}
		*field.value = value
	}
	if values.intent == "" && values.problem == "" {
		value, ok := readPromptLine(scanner, prompts, "Intent or problem: ", true)
		if !ok {
			return emitResponse(stdout, mode, workItemCreateAction, service.WorkItemCreate(ctx, WorkItemInput{Project: values.project, Repository: values.repository, ProviderRepository: values.providerRepository, Cancelled: true}))
		}
		values.intent = value
	}
	sections := []struct {
		value  *string
		prompt string
	}{
		{&values.desiredOutcome, "Desired outcome: "}, {&values.context, "Context: "}, {&values.scope, "Scope: "},
		{&values.constraints, "Constraints: "}, {&values.nonGoals, "Non-goals: "}, {&values.acceptance, "Acceptance expectations: "},
	}
	for _, field := range sections {
		if *field.value != "" {
			continue
		}
		value, ok := readPromptLine(scanner, prompts, field.prompt, true)
		if !ok {
			return emitResponse(stdout, mode, workItemCreateAction, service.WorkItemCreate(ctx, workItemInput(values, true)))
		}
		*field.value = value
	}
	preview := service.WorkItemCreate(ctx, workItemInput(values, false))
	if preview.Draft == nil || preview.Completion == nil || preview.Completion.Status() != completion.Success {
		return emitResponse(stdout, mode, workItemCreateAction, preview)
	}
	if prompts != nil {
		wire, err := marshalWorkItemValue(preview.Draft, true)
		if err != nil {
			return ExitFailure
		}
		_, _ = prompts.Write(wire)
	}
	answer, ok := readPromptLine(scanner, prompts, "Publish this create-attempt fence, create this exact GitHub Issue, and publish its local link? [yes/no]: ", false)
	if !ok || answer != "yes" {
		cancelled := workItemInput(values, true)
		return emitResponse(stdout, mode, workItemCreateAction, service.WorkItemCreate(ctx, cancelled))
	}
	authorized := workItemInput(values, false)
	authorized.PreviewDigest = preview.Draft.Digest
	authorized.AuthorizeExternal = true
	return emitResponse(stdout, mode, workItemCreateAction, service.WorkItemCreate(ctx, authorized))
}

func workItemInput(values requestInput, cancelled bool) WorkItemInput {
	return WorkItemInput{
		Project: values.project, Repository: values.repository, ProviderRepository: values.providerRepository,
		Intent: values.intent, Problem: values.problem, DesiredOutcome: values.desiredOutcome, Context: values.context,
		Scope: values.scope, Constraints: values.constraints, NonGoals: values.nonGoals, Acceptance: values.acceptance,
		PreviewDigest: values.previewDigest, AuthorizeExternal: values.authorizeExternal, Cancelled: cancelled,
	}
}

func runInteractiveConfiguration(ctx context.Context, mode outputMode, args []string, service Service, input io.Reader, stdout, prompts io.Writer) int {
	values, ok := flags(configureAction, args)
	if !ok {
		return emit(stdout, mode, event{Operation: configureAction, Status: Failed, Category: "invalid_input"})
	}
	repositories, ok := parseRepositories(values.repositories)
	if !ok {
		return emit(stdout, mode, event{Operation: configureAction, Status: Failed, Category: "invalid_input"})
	}
	scanner := bufio.NewScanner(input)
	configuration, ok := promptConfiguration(scanner, prompts, ConfigureInput{
		ProjectID: values.projectID, Slug: values.slug, Name: values.name,
		Repositories: repositories, WorkItemProvider: values.workItemProvider,
		PreviewDigest: values.previewDigest, AuthorizeLocal: values.authorizeLocal,
	})
	if !ok {
		return emit(stdout, mode, event{Operation: configureAction, Status: Failed, Category: "missing_required_input"})
	}
	if configuration.AuthorizeLocal {
		return emitResponse(stdout, mode, configureAction, service.Configure(ctx, configuration))
	}
	preview := service.Configure(ctx, configuration)
	if preview.Setup == nil || preview.Completion == nil || preview.Completion.Status() != completion.Success {
		return emitResponse(stdout, mode, configureAction, preview)
	}
	if prompts != nil {
		wire, _ := json.MarshalIndent(preview.Setup, "", "  ")
		_, _ = prompts.Write(append(wire, '\n'))
	}
	answer, ok := readPromptLine(scanner, prompts, "Publish this exact proposal? [yes/no]: ", false)
	if !ok || answer != "yes" {
		configuration.ProjectID = preview.Setup.ProjectID
		configuration.PreviewDigest = ""
		configuration.AuthorizeLocal = true
		return emitResponse(stdout, mode, configureAction, service.Configure(ctx, configuration))
	}
	configuration.ProjectID = preview.Setup.ProjectID
	configuration.PreviewDigest = preview.Setup.Digest
	configuration.AuthorizeLocal = true
	return emitResponse(stdout, mode, configureAction, service.Configure(ctx, configuration))
}

func promptConfiguration(scanner *bufio.Scanner, prompts io.Writer, current ConfigureInput) (ConfigureInput, bool) {
	var ok bool
	if current.Slug == "" {
		current.Slug, ok = readPromptLine(scanner, prompts, "Project slug: ", true)
		if !ok {
			return ConfigureInput{}, false
		}
	}
	if current.Name == "" {
		current.Name, ok = readPromptLine(scanner, prompts, "Project name: ", true)
		if !ok {
			return ConfigureInput{}, false
		}
	}
	if len(current.Repositories) == 0 {
		for {
			value, read := readPromptLine(scanner, prompts, "Repository key=absolute-path (blank to finish): ", false)
			if !read {
				return ConfigureInput{}, false
			}
			if value == "" {
				break
			}
			repositories, valid := parseRepositories([]string{value})
			if !valid {
				return ConfigureInput{}, false
			}
			current.Repositories = append(current.Repositories, repositories[0])
		}
		if len(current.Repositories) == 0 {
			return ConfigureInput{}, false
		}
	}
	if current.WorkItemProvider == "" {
		current.WorkItemProvider, ok = readPromptLine(scanner, prompts, "Work Item provider (github or none): ", true)
		if !ok {
			return ConfigureInput{}, false
		}
		if current.WorkItemProvider == "none" {
			current.WorkItemProvider = ""
		}
	}
	return current, true
}

func readPromptLine(scanner *bufio.Scanner, output io.Writer, prompt string, required bool) (string, bool) {
	if output != nil {
		_, _ = io.WriteString(output, prompt)
	}
	if !scanner.Scan() {
		return "", false
	}
	value := strings.TrimSpace(scanner.Text())
	return value, !required || value != ""
}

// UnavailableService makes the executable fail closed until its composition root
// receives the authorized application use cases in subsequent POC delivery work.
type UnavailableService struct{ source provenance.Value }

func NewUnavailableService(source provenance.Value) UnavailableService {
	return UnavailableService{source: source}
}

func (UnavailableService) Init(context.Context, InitInput) Result {
	return unavailable()
}
func (s UnavailableService) Validate(context.Context, ProjectInput) Result {
	return s.canonicalUnavailable("Project validation unavailable")
}
func (UnavailableService) Reopen(context.Context, ProjectInput) Result {
	return unavailable()
}
func (UnavailableService) Update(context.Context, UpdateInput) Result {
	return unavailable()
}
func (UnavailableService) Install(context.Context, InstallInput) Result { return unavailable() }
func (UnavailableService) RuntimeCodexInstall(context.Context) Result   { return unavailable() }
func (UnavailableService) RuntimeCodexStatus(context.Context) Result    { return unavailable() }
func (UnavailableService) RuntimeClaudeInstall(context.Context) Result  { return unavailable() }
func (UnavailableService) RuntimeClaudeStatus(context.Context) Result   { return unavailable() }
func (UnavailableService) FirstRun(context.Context) Result              { return unavailable() }
func (UnavailableService) Resolve(context.Context, ResolveInput) Result { return unavailable() }
func (s UnavailableService) Show(context.Context, ResolveInput) Result {
	return s.canonicalUnavailable("Project inspection unavailable")
}
func (UnavailableService) Configure(context.Context, ConfigureInput) Result     { return unavailable() }
func (UnavailableService) WorkItemCreate(context.Context, WorkItemInput) Result { return unavailable() }
func (UnavailableService) WorkItemSelect(context.Context, WorkItemInput) Result { return unavailable() }
func (UnavailableService) WorkItemShow(context.Context, WorkItemInput) Result   { return unavailable() }
func (UnavailableService) WorkItemComment(context.Context, WorkItemInput) Result {
	return unavailable()
}
func (UnavailableService) WorkItemComplete(context.Context, WorkItemInput) Result {
	return unavailable()
}
func (UnavailableService) WorkflowStart(context.Context, WorkflowInput) Result { return unavailable() }
func (UnavailableService) WorkflowAdvance(context.Context, WorkflowInput) Result {
	return unavailable()
}
func (UnavailableService) WorkflowResume(context.Context, WorkflowInput) Result { return unavailable() }
func (UnavailableService) WorkflowStatus(context.Context, WorkflowInput) Result { return unavailable() }
func (UnavailableService) WorkflowEvidence(context.Context, WorkflowInput) Result {
	return unavailable()
}
func (UnavailableService) WorkflowReconcile(context.Context, WorkflowInput) Result {
	return unavailable()
}
func unavailable() Result { return Result{Status: Failed, Category: "application_unavailable"} }

func (s UnavailableService) canonicalUnavailable(message string) Result {
	if !s.source.Valid() {
		return unavailable()
	}
	statement, err := provenance.NewText(message, provenance.AxiomAuthored)
	if err != nil {
		return unavailable()
	}
	result, err := completion.New(completion.Facts{Failed: true}, statement, nil, provenance.Text{}, "", s.source)
	if err != nil {
		return unavailable()
	}
	return Result{Completion: &result}
}
