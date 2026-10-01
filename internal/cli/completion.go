package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/projectapp"
	"github.com/rgomids/axiom/internal/provenance"
	"github.com/rgomids/axiom/internal/workflow"
	"github.com/rgomids/axiom/internal/workitem"
)

const MaxCompletionOutputBytes = 16 * 1024

type CompletionFormat string

const (
	CompletionHuman CompletionFormat = "human"
	CompletionJSON  CompletionFormat = "json"
)

type completionEvent struct {
	Status     completion.Status `json:"status"`
	Result     string            `json:"result"`
	References []string          `json:"references,omitempty"`
	Next       string            `json:"next,omitempty"`
	Details    string            `json:"details,omitempty"`
	Provenance provenanceEvent   `json:"provenance"`
}

type provenanceEvent struct {
	Product     string                 `json:"product"`
	Version     string                 `json:"version"`
	Revision    string                 `json:"revision"`
	SourceState provenance.SourceState `json:"sourceState"`
}

func WriteCompletion(writer io.Writer, format CompletionFormat, result completion.Result) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	content, err := renderCompletion(format, result)
	if err != nil || len(content) > MaxCompletionOutputBytes {
		return ExitFailure
	}
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}

func emitCompletion(writer io.Writer, mode outputMode, result completion.Result) int {
	if mode == jsonOutput {
		return WriteCompletion(writer, CompletionJSON, result)
	}
	return WriteCompletion(writer, CompletionHuman, result)
}

type setupCompletionEvent struct {
	completionEvent
	Setup projectapp.SetupPreview `json:"setup"`
}

type projectCompletionEvent struct {
	completionEvent
	Project ProjectView `json:"project"`
}

type runtimeCompletionEvent struct {
	completionEvent
	Runtime RuntimeView `json:"runtime"`
}

type workItemCompletionEvent struct {
	completionEvent
	Draft     *workitem.DraftPreview     `json:"draft,omitempty"`
	Selection *workitem.SelectionPreview `json:"selection,omitempty"`
	WorkItem  *WorkItemView              `json:"workItem,omitempty"`
	Questions []workitem.Question        `json:"questions,omitempty"`
}

type workflowCompletionEvent struct {
	completionEvent
	Workflow   *WorkflowView               `json:"workflow,omitempty"`
	Projection *workflow.ProjectionPreview `json:"projection,omitempty"`
}

const maxWorkItemPreviewOutputBytes = 128 * 1024

func emitWorkItemCompletion(writer io.Writer, mode outputMode, result completion.Result, response Result) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	base := completionEvent{Status: result.Status(), Result: result.Result().String(), References: result.References(), Next: result.Next().String(), Details: result.Details(), Provenance: provenanceEvent{Product: result.Provenance().Product(), Version: result.Provenance().Version(), Revision: result.Provenance().Revision(), SourceState: result.Provenance().SourceState()}}
	value := workItemCompletionEvent{completionEvent: base, Draft: response.Draft, Selection: response.Selection, WorkItem: response.WorkItem}
	if len(response.Questions) != 0 {
		value.Questions = response.Questions
	}
	if mode == humanOutput {
		content := renderCompletionHuman(result)
		extra, err := marshalWorkItemValue(value, true)
		if err != nil {
			return ExitFailure
		}
		content = append(content, "preview: "...)
		content = append(content, extra...)
		if len(content) > maxWorkItemPreviewOutputBytes {
			return ExitFailure
		}
		written, err := writer.Write(content)
		if err != nil || written != len(content) {
			return ExitFailure
		}
		return completionExitCode(result.Status())
	}
	content, err := marshalWorkItemValue(value, false)
	if err != nil {
		return ExitFailure
	}
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}

func emitWorkflowCompletion(writer io.Writer, mode outputMode, result completion.Result, response Result) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	base := completionEvent{Status: result.Status(), Result: result.Result().String(), References: result.References(), Next: result.Next().String(), Details: result.Details(), Provenance: provenanceEvent{Product: result.Provenance().Product(), Version: result.Provenance().Version(), Revision: result.Provenance().Revision(), SourceState: result.Provenance().SourceState()}}
	value := workflowCompletionEvent{completionEvent: base, Workflow: response.Workflow, Projection: response.Projection}
	content, err := json.Marshal(value)
	if err != nil || len(content)+1 > maxWorkItemPreviewOutputBytes {
		return ExitFailure
	}
	content = append(content, '\n')
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}

func marshalWorkItemValue(value any, indented bool) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if indented {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	if output.Len() > maxWorkItemPreviewOutputBytes {
		return nil, errors.New("work item preview exceeds output limit")
	}
	return output.Bytes(), nil
}

func emitRuntimeCompletion(writer io.Writer, mode outputMode, result completion.Result, runtime RuntimeView) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	if mode == humanOutput {
		content := renderCompletionHuman(result)
		var extra bytes.Buffer
		emitRuntimeHuman(&extra, runtime)
		content = append(content, extra.Bytes()...)
		if len(content) > MaxCompletionOutputBytes {
			return ExitFailure
		}
		written, err := writer.Write(content)
		if err != nil || written != len(content) {
			return ExitFailure
		}
		return completionExitCode(result.Status())
	}
	base := completionEvent{Status: result.Status(), Result: result.Result().String(), References: result.References(), Next: result.Next().String(), Details: result.Details(), Provenance: provenanceEvent{Product: result.Provenance().Product(), Version: result.Provenance().Version(), Revision: result.Provenance().Revision(), SourceState: result.Provenance().SourceState()}}
	content, err := json.Marshal(runtimeCompletionEvent{completionEvent: base, Runtime: runtime})
	if err != nil || len(content)+1 > MaxCompletionOutputBytes {
		return ExitFailure
	}
	content = append(content, '\n')
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}

type bootstrapCompletionEvent struct {
	completionEvent
	FirstRun BootstrapView `json:"firstRun"`
}

func emitBootstrapCompletion(writer io.Writer, mode outputMode, result completion.Result, bootstrap BootstrapView) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	var content []byte
	if mode == humanOutput {
		content = renderCompletionHuman(result)
		var extra bytes.Buffer
		for _, runtime := range bootstrap.Runtimes {
			fmt.Fprintf(&extra, "runtime: %s present=%t state=%s reason=%s", runtime.Runtime, runtime.Present, runtime.State, runtime.Reason)
			if runtime.ConfigurationWithoutExecutable {
				extra.WriteString(" note=configuration_without_executable")
			}
			extra.WriteString("\n")
			for _, skill := range runtime.Skills {
				fmt.Fprintf(&extra, "  skill: %s sha256=%s state=%s\n", skill.Name, skill.SHA256, skill.State)
			}
		}
		content = append(content, extra.Bytes()...)
	} else {
		base := completionEvent{Status: result.Status(), Result: result.Result().String(), References: result.References(), Next: result.Next().String(), Details: result.Details(), Provenance: provenanceEvent{Product: result.Provenance().Product(), Version: result.Provenance().Version(), Revision: result.Provenance().Revision(), SourceState: result.Provenance().SourceState()}}
		encoded, err := json.Marshal(bootstrapCompletionEvent{completionEvent: base, FirstRun: bootstrap})
		if err != nil {
			return ExitFailure
		}
		content = append(encoded, '\n')
	}
	if len(content) > MaxCompletionOutputBytes {
		return ExitFailure
	}
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}

func emitRuntimeHuman(writer io.Writer, runtime RuntimeView) {
	fmt.Fprintf(writer, "skill-set: %s binary-compatibility=%s\n", runtime.SkillSetVersion, runtime.BinaryCompatibility)
	for _, skill := range runtime.Skills {
		fmt.Fprintf(writer, "skill: %s sha256=%s state=%s\n", skill.Name, skill.SHA256, skill.State)
	}
}

func emitSetupCompletion(writer io.Writer, mode outputMode, result completion.Result, setup projectapp.SetupPreview) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	if mode == humanOutput {
		content := renderCompletionHuman(result)
		var extra bytes.Buffer
		fmt.Fprintf(&extra, "preview-digest: %s\n", setup.Digest)
		fmt.Fprintf(&extra, "project: %s [%s]\n", setup.Slug, setup.ProjectID)
		fmt.Fprintf(&extra, "portable-destination: %s revision=%s\n", setup.PortableDestination, setup.PortableRevision)
		fmt.Fprintf(&extra, "local-destination: %s revision=%s\n", setup.LocalDestination, setup.LocalRevision)
		fmt.Fprintf(&extra, "capability: %s provider=%s readiness=%s\n", setup.Capability.Capability, setup.Capability.Provider, setup.Capability.Readiness)
		for _, repository := range setup.Repositories {
			fmt.Fprintf(&extra, "repository: %s local=%q revision=%s\n", repository.Key, repository.LocalPath, repository.LocalRevision)
		}
		for _, effect := range setup.Effects {
			fmt.Fprintf(&extra, "effect: %s\n", effect)
		}
		content = append(content, extra.Bytes()...)
		if len(content) > MaxCompletionOutputBytes {
			return ExitFailure
		}
		written, err := writer.Write(content)
		if err != nil || written != len(content) {
			return ExitFailure
		}
		return completionExitCode(result.Status())
	}
	base := completionEvent{Status: result.Status(), Result: result.Result().String(), References: result.References(), Next: result.Next().String(), Details: result.Details(), Provenance: provenanceEvent{Product: result.Provenance().Product(), Version: result.Provenance().Version(), Revision: result.Provenance().Revision(), SourceState: result.Provenance().SourceState()}}
	content, err := json.Marshal(setupCompletionEvent{completionEvent: base, Setup: setup})
	if err != nil || len(content)+1 > MaxCompletionOutputBytes {
		return ExitFailure
	}
	content = append(content, '\n')
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}

type editCompletionEvent struct {
	completionEvent
	Edit projectapp.EditPreview `json:"edit"`
}

// An EDIT preview carries the complete portable manifest, so it uses the
// larger bounded preview budget rather than the summary-only limit.
func emitEditCompletion(writer io.Writer, mode outputMode, result completion.Result, edit projectapp.EditPreview) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	var content []byte
	if mode == humanOutput {
		content = renderCompletionHuman(result)
		var extra bytes.Buffer
		fmt.Fprintf(&extra, "mode: %s\n", edit.Mode)
		fmt.Fprintf(&extra, "preview-digest: %s\n", edit.Digest)
		fmt.Fprintf(&extra, "project: %s [%s] name=%q\n", edit.Slug, edit.ProjectID, edit.Name)
		fmt.Fprintf(&extra, "portable-destination: %s revision=%s\n", edit.PortableDestination, edit.PortableRevision)
		fmt.Fprintf(&extra, "local-destination: %s revision=%s\n", edit.LocalDestination, edit.LocalRevision)
		fmt.Fprintf(&extra, "capability: %s provider=%s readiness=%s\n", edit.Capability.Capability, edit.Capability.Provider, edit.Capability.Readiness)
		for _, repository := range edit.Repositories {
			fmt.Fprintf(&extra, "repository: %s change=%s", repository.Key, repository.Change)
			if repository.LocalPath != "" {
				fmt.Fprintf(&extra, " local=%q", repository.LocalPath)
			}
			if repository.LocalRevision != "" {
				fmt.Fprintf(&extra, " revision=%s", repository.LocalRevision)
			}
			extra.WriteString("\n")
		}
		for _, effect := range edit.Effects {
			fmt.Fprintf(&extra, "effect: %s %s", effect.Scope, effect.Code)
			if effect.Key != "" {
				fmt.Fprintf(&extra, " key=%s", effect.Key)
			}
			extra.WriteString("\n")
		}
		extra.WriteString("portable-manifest:\n")
		for _, line := range strings.Split(strings.TrimSuffix(edit.PortableManifest, "\n"), "\n") {
			fmt.Fprintf(&extra, "  %s\n", line)
		}
		content = append(content, extra.Bytes()...)
	} else {
		base := completionEvent{Status: result.Status(), Result: result.Result().String(), References: result.References(), Next: result.Next().String(), Details: result.Details(), Provenance: provenanceEvent{Product: result.Provenance().Product(), Version: result.Provenance().Version(), Revision: result.Provenance().Revision(), SourceState: result.Provenance().SourceState()}}
		encoded, err := marshalWorkItemValue(editCompletionEvent{completionEvent: base, Edit: edit}, false)
		if err != nil {
			return ExitFailure
		}
		content = encoded
	}
	if len(content) > maxWorkItemPreviewOutputBytes {
		return ExitFailure
	}
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}

func emitProjectCompletion(writer io.Writer, mode outputMode, result completion.Result, project ProjectView) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	if mode == humanOutput {
		content := renderCompletionHuman(result)
		var extra bytes.Buffer
		fmt.Fprintf(&extra, "project: %s [%s] source=%s\n", project.Slug, project.ID, project.Source)
		for _, repository := range project.Repositories {
			fmt.Fprintf(&extra, "repository: %s path=%q\n", repository.Key, repository.Path)
		}
		content = append(content, extra.Bytes()...)
		if len(content) > MaxCompletionOutputBytes {
			return ExitFailure
		}
		written, err := writer.Write(content)
		if err != nil || written != len(content) {
			return ExitFailure
		}
		return completionExitCode(result.Status())
	}
	base := completionEvent{Status: result.Status(), Result: result.Result().String(), References: result.References(), Next: result.Next().String(), Details: result.Details(), Provenance: provenanceEvent{Product: result.Provenance().Product(), Version: result.Provenance().Version(), Revision: result.Provenance().Revision(), SourceState: result.Provenance().SourceState()}}
	content, err := json.Marshal(projectCompletionEvent{completionEvent: base, Project: project})
	if err != nil || len(content)+1 > MaxCompletionOutputBytes {
		return ExitFailure
	}
	content = append(content, '\n')
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}

func renderCompletion(format CompletionFormat, result completion.Result) ([]byte, error) {
	if format == CompletionJSON {
		return renderCompletionJSON(result)
	}
	if format == CompletionHuman {
		return renderCompletionHuman(result), nil
	}
	return nil, errors.New("unsupported completion format")
}

func renderCompletionJSON(result completion.Result) ([]byte, error) {
	value := completionEvent{
		Status:     result.Status(),
		Result:     result.Result().String(),
		References: result.References(),
		Next:       result.Next().String(),
		Details:    result.Details(),
		Provenance: provenanceEvent{
			Product:     result.Provenance().Product(),
			Version:     result.Provenance().Version(),
			Revision:    result.Provenance().Revision(),
			SourceState: result.Provenance().SourceState(),
		},
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func renderCompletionHuman(result completion.Result) []byte {
	var output bytes.Buffer
	fmt.Fprintf(&output, "status: %s\n", result.Status())
	fmt.Fprintf(&output, "result: %s\n", result.Result().String())
	for _, reference := range result.References() {
		fmt.Fprintf(&output, "reference: %s\n", reference)
	}
	if !result.Next().Empty() {
		fmt.Fprintf(&output, "next: %s\n", result.Next().String())
	}
	if result.Details() != "" {
		fmt.Fprintf(&output, "details: %s\n", result.Details())
	}
	fmt.Fprintf(&output, "provenance: %s %s revision=%s source=%s\n", result.Provenance().Product(), result.Provenance().Version(), result.Provenance().Revision(), result.Provenance().SourceState())
	return output.Bytes()
}

func completionExitCode(status completion.Status) int {
	if status == completion.Success {
		return ExitSuccess
	}
	if status == completion.Interrupted {
		return ExitCancelled
	}
	return ExitFailure
}
