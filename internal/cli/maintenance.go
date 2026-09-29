package cli

import (
	"context"
	"flag"
	"io"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/provenance"
)

// MaintenanceInput carries explicit targets and exact local authority for
// compatibility, cleanup, recovery, and upgrade operations.
type MaintenanceInput struct {
	Target, Archive, Checksums, BinaryDir, ReceiptDir string
	PreviewDigest                                     string
	AuthorizeLocal                                    bool
	ArtifactID                                        string
}

// ArtifactRetirementService is optional beside MaintenanceService so existing
// presentation fakes keep compiling; without it retirement is unavailable.
type ArtifactRetirementService interface {
	ArtifactRetire(context.Context, MaintenanceInput) Result
}

// MaintenanceService is optional so presentation fakes need not implement it;
// an application without it reports application_unavailable.
type MaintenanceService interface {
	CompatibilityInspect(context.Context) Result
	CompatibilityBackup(context.Context, MaintenanceInput) Result
	CompatibilityExport(context.Context, MaintenanceInput) Result
	ArtifactCleanup(context.Context, MaintenanceInput) Result
	RecoveryInspect(context.Context) Result
	RecoveryApply(context.Context, MaintenanceInput) Result
	Upgrade(context.Context, MaintenanceInput) Result
}

const (
	compatibilityInspectAction action = "compatibility_inspect"
	compatibilityBackupAction  action = "compatibility_backup"
	compatibilityExportAction  action = "compatibility_export"
	artifactCleanupAction      action = "artifact_cleanup"
	artifactRetireAction       action = "artifact_retire"
	recoveryInspectAction      action = "recovery_inspect"
	recoveryApplyAction        action = "recovery_apply"
	upgradeAction              action = "upgrade"
)

func maintenanceAction(args []string) (action, []string, bool) {
	if len(args) >= 1 && args[0] == "upgrade" {
		return upgradeAction, args[1:], true
	}
	if len(args) < 2 {
		return "", nil, false
	}
	switch args[0] + " " + args[1] {
	case "compatibility inspect":
		return compatibilityInspectAction, args[2:], true
	case "compatibility backup":
		return compatibilityBackupAction, args[2:], true
	case "compatibility export":
		return compatibilityExportAction, args[2:], true
	case "artifact cleanup":
		return artifactCleanupAction, args[2:], true
	case "artifact retire":
		return artifactRetireAction, args[2:], true
	case "recovery inspect":
		return recoveryInspectAction, args[2:], true
	case "recovery apply":
		return recoveryApplyAction, args[2:], true
	}
	if args[0] == "compatibility" || args[0] == "artifact" || args[0] == "recovery" {
		return "unknown", nil, true
	}
	return "", nil, false
}

func maintenanceFlags(operation action, args []string) (MaintenanceInput, string) {
	set := flag.NewFlagSet(string(operation), flag.ContinueOnError)
	set.SetOutput(io.Discard)
	var input MaintenanceInput
	mutating := operation != compatibilityInspectAction && operation != recoveryInspectAction
	if mutating {
		set.StringVar(&input.PreviewDigest, "preview-digest", "", "")
		set.BoolVar(&input.AuthorizeLocal, "authorize-local", false, "")
	}
	if operation == compatibilityBackupAction || operation == compatibilityExportAction {
		set.StringVar(&input.Target, "target", "", "")
	}
	if operation == artifactRetireAction {
		set.StringVar(&input.ArtifactID, "artifact", "", "")
	}
	if operation == upgradeAction {
		set.StringVar(&input.Archive, "archive", "", "")
		set.StringVar(&input.Checksums, "checksums", "", "")
		set.StringVar(&input.BinaryDir, "bin-dir", "", "")
		set.StringVar(&input.ReceiptDir, "receipt-dir", "", "")
	}
	if invalidFlagSyntax(set, args, nil) {
		return MaintenanceInput{}, "invalid_input"
	}
	if err := set.Parse(args); err != nil || set.NArg() != 0 {
		return MaintenanceInput{}, "invalid_input"
	}
	switch {
	case (operation == compatibilityBackupAction || operation == compatibilityExportAction) && input.Target == "":
		return input, "missing_required_input"
	case operation == upgradeAction && (input.Archive == "" || input.Checksums == "" || input.BinaryDir == "" || input.ReceiptDir == ""):
		return input, "missing_required_input"
	case operation == artifactRetireAction && input.ArtifactID == "":
		return input, "missing_required_input"
	case operation == recoveryApplyAction && (input.PreviewDigest == "" || !input.AuthorizeLocal):
		return input, "missing_required_input"
	case input.AuthorizeLocal && input.PreviewDigest == "":
		return input, "missing_required_input"
	}
	return input, ""
}

func runMaintenance(ctx context.Context, mode outputMode, operation action, args []string, service Service, source provenance.Value, stdout io.Writer) int {
	if operation == "unknown" {
		return emit(stdout, mode, event{Operation: "unknown", Status: Failed, Category: "invalid_command"})
	}
	input, issue := maintenanceFlags(operation, args)
	if issue != "" {
		return emitMaintenanceParserFailure(stdout, mode, issue, source)
	}
	maintenance, ok := service.(MaintenanceService)
	if !ok {
		return emit(stdout, mode, event{Operation: operation, Status: Failed, Category: "application_unavailable"})
	}
	if err := ctx.Err(); err != nil {
		return emit(stdout, mode, event{Operation: operation, Status: Cancelled, Category: "cancelled"})
	}
	var response Result
	switch operation {
	case compatibilityInspectAction:
		response = maintenance.CompatibilityInspect(ctx)
	case compatibilityBackupAction:
		response = maintenance.CompatibilityBackup(ctx, input)
	case compatibilityExportAction:
		response = maintenance.CompatibilityExport(ctx, input)
	case artifactCleanupAction:
		response = maintenance.ArtifactCleanup(ctx, input)
	case artifactRetireAction:
		retirement, ok := service.(ArtifactRetirementService)
		if !ok {
			return emit(stdout, mode, event{Operation: operation, Status: Failed, Category: "application_unavailable"})
		}
		response = retirement.ArtifactRetire(ctx, input)
	case recoveryInspectAction:
		response = maintenance.RecoveryInspect(ctx)
	case recoveryApplyAction:
		response = maintenance.RecoveryApply(ctx, input)
	default:
		response = maintenance.Upgrade(ctx, input)
	}
	if response.Completion == nil {
		return emit(stdout, mode, eventFrom(operation, response))
	}
	return emitMaintenanceCompletion(stdout, mode, *response.Completion, response.Maintenance)
}

func emitMaintenanceParserFailure(writer io.Writer, mode outputMode, issue string, source provenance.Value) int {
	message, next := "Maintenance input is invalid", "Remove unknown, duplicate, or conflicting flags and retry"
	if issue == "missing_required_input" {
		message, next = "Maintenance input is incomplete", "Provide every required absolute path, and pair --authorize-local with the exact --preview-digest"
	}
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

type maintenanceCompletionEvent struct {
	completionEvent
	Maintenance any `json:"maintenance,omitempty"`
}

func emitMaintenanceCompletion(writer io.Writer, mode outputMode, result completion.Result, view any) int {
	if writer == nil || !result.Valid() {
		return ExitFailure
	}
	base := completionEvent{Status: result.Status(), Result: result.Result().String(), References: result.References(), Next: result.Next().String(), Details: result.Details(), Provenance: provenanceEvent{Product: result.Provenance().Product(), Version: result.Provenance().Version(), Revision: result.Provenance().Revision(), SourceState: result.Provenance().SourceState()}}
	if mode == humanOutput {
		content := renderCompletionHuman(result)
		if view != nil {
			extra, err := marshalWorkItemValue(view, true)
			if err != nil {
				return ExitFailure
			}
			content = append(content, "details:\n"...)
			content = append(content, extra...)
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
	content, err := marshalWorkItemValue(maintenanceCompletionEvent{completionEvent: base, Maintenance: view}, false)
	if err != nil {
		return ExitFailure
	}
	written, err := writer.Write(content)
	if err != nil || written != len(content) {
		return ExitFailure
	}
	return completionExitCode(result.Status())
}
