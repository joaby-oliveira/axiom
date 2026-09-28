package main

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/rgomids/axiom/internal/cli"
	"github.com/rgomids/axiom/internal/compatibility"
	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/install"
	"github.com/rgomids/axiom/internal/local"
)

const maxViewItems = 64

func (s lifecycleService) roots() compatibility.Roots {
	return compatibility.Roots{Projects: s.projectsRoot, State: s.stateRoot, Skills: s.skillsRoot}
}

func (s lifecycleService) maintenanceResult(facts completion.Facts, message string, references []string, next string, view any) cli.Result {
	result := canonicalCompletion(facts, message, references, next, s.provenance)
	result.Maintenance = view
	return result
}

func (s lifecycleService) CompatibilityInspect(ctx context.Context) cli.Result {
	report, err := compatibility.Inspect(ctx, s.roots())
	if err != nil {
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, "Compatibility inspection could not read the owned roots", nil, "Use absolute, non-root LINGO_PROJECTS_ROOT, LINGO_STATE_ROOT, and Codex skill roots", nil)
	}
	return s.maintenanceResult(completion.Facts{Completed: true}, "Compatibility: "+string(report.Classification), []string{"compatibility:" + report.Digest}, report.Next[0], report)
}

type transferView struct {
	Preview      compatibility.TransferPreview `json:"preview"`
	EffectsTotal int                           `json:"effectsTotal"`
	Result       *compatibility.TransferResult `json:"result,omitempty"`
}

func boundedTransfer(preview compatibility.TransferPreview, result *compatibility.TransferResult) transferView {
	view := transferView{Preview: preview, EffectsTotal: len(preview.Effects), Result: result}
	if len(view.Preview.Effects) > maxViewItems {
		view.Preview.Effects = view.Preview.Effects[:maxViewItems]
	}
	if len(view.Preview.Omitted) > maxViewItems {
		view.Preview.Omitted = view.Preview.Omitted[:maxViewItems]
	}
	if result != nil && len(result.Written) > maxViewItems {
		copied := *result
		copied.Written = copied.Written[:maxViewItems]
		view.Result = &copied
	}
	return view
}

func (s lifecycleService) CompatibilityBackup(ctx context.Context, input cli.MaintenanceInput) cli.Result {
	return s.transfer(ctx, compatibility.Backup, input)
}

func (s lifecycleService) CompatibilityExport(ctx context.Context, input cli.MaintenanceInput) cli.Result {
	return s.transfer(ctx, compatibility.Export, input)
}

func (s lifecycleService) transfer(ctx context.Context, kind compatibility.TransferKind, input cli.MaintenanceInput) cli.Result {
	label := map[compatibility.TransferKind]string{compatibility.Backup: "POC backup", compatibility.Export: "Portable export"}[kind]
	preview, err := compatibility.PreviewTransfer(ctx, kind, s.roots(), input.Target)
	switch {
	case errors.Is(err, compatibility.ErrTransferSource):
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, label+" requires a complete recognized POC source", nil, "Run `axiom compatibility inspect`; only recognized_poc state is preserved this way", nil)
	case errors.Is(err, compatibility.ErrTransferCapacity):
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, label+" target lacks observed free space", nil, "Free space or choose another absent target on the same filesystem", boundedTransfer(preview, nil))
	case err != nil:
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, label+" target is unsafe or occupied", nil, "Choose an absent absolute target outside every owned root, on the same local filesystem", nil)
	}
	references := []string{"transfer:" + preview.Digest}
	if preview.State == compatibility.TransferComplete {
		return s.maintenanceResult(completion.Facts{Completed: true}, label+" already complete at the target", references, "No further effect is required", boundedTransfer(preview, nil))
	}
	if !input.AuthorizeLocal {
		return s.maintenanceResult(completion.Facts{Completed: true}, label+" preview ready", references, "Review effects, then repeat with --preview-digest "+preview.Digest+" --authorize-local", boundedTransfer(preview, nil))
	}
	authority, err := compatibility.AuthorizeTransfer(preview, input.PreviewDigest)
	if err != nil {
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, label+" authority is missing or stale", nil, "Review the current preview and authorize its exact digest", boundedTransfer(preview, nil))
	}
	result, err := compatibility.ApplyTransfer(ctx, preview, authority)
	switch {
	case err == nil:
		next := "Keep the manifest with the preserved copy"
		if kind == compatibility.Export {
			next = "Point LINGO_PROJECTS_ROOT at " + filepath.Join(preview.Target, "projects") + " with a separate clean LINGO_STATE_ROOT, then run `axiom project configure` explicitly"
		}
		return s.maintenanceResult(completion.Facts{Completed: true}, label+" completed and verified", references, next, boundedTransfer(preview, &result))
	case result.Status == "partial":
		return s.maintenanceResult(completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}, label+" is incomplete; source state is unchanged", references, "The target has no manifest and is incomplete; retry into a new absent target", boundedTransfer(preview, &result))
	case errors.Is(err, compatibility.ErrTransferStale):
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, label+" source or target changed after review", nil, "Prepare and review a fresh preview", nil)
	default:
		return s.maintenanceResult(completion.Facts{Failed: true}, label+" failed before any object was written", nil, "Inspect the target location and retry with a fresh preview", boundedTransfer(preview, &result))
	}
}

type cleanupView struct {
	Preview detailartifact.CleanupPreview `json:"preview"`
	Result  *cleanupResultView            `json:"result,omitempty"`
}

type cleanupResultView struct {
	RecordID       string                         `json:"recordId"`
	Removed        []detailartifact.CleanupEffect `json:"removed"`
	ReclaimedBytes int64                          `json:"reclaimedBytes"`
	Partial        bool                           `json:"partial"`
	AuditRecorded  bool                           `json:"auditRecorded"`
}

func (s lifecycleService) ArtifactCleanup(ctx context.Context, input cli.MaintenanceInput) cli.Result {
	store, err := local.NewArtifactStore(s.stateRoot)
	if err != nil {
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, "Artifact root is unsafe", nil, "Use an absolute non-root LINGO_STATE_ROOT", nil)
	}
	preview, err := store.PreviewCleanup(ctx, time.Now().UTC())
	if err != nil {
		if errors.Is(err, local.ErrConflict) {
			return s.maintenanceResult(completion.Facts{RetrySafeFailure: true}, "Artifact state is locked by another operation", nil, "Retry after the other operation completes", nil)
		}
		return s.maintenanceResult(completion.Facts{Failed: true}, "Artifact or reference state is uncertain; nothing is eligible", nil, "Run `axiom recovery inspect` and resolve preserved state before cleanup", nil)
	}
	view := cleanupView{Preview: preview}
	references := []string{"cleanup:" + preview.Digest}
	if len(preview.Effects) == 0 {
		return s.maintenanceResult(completion.Facts{Completed: true}, "No artifact is eligible for cleanup", references, "No cleanup effect is required", view)
	}
	if !input.AuthorizeLocal {
		return s.maintenanceResult(completion.Facts{Completed: true}, "Artifact cleanup preview ready", references, "Review effects, then repeat with --preview-digest "+preview.Digest+" --authorize-local", view)
	}
	authority, err := detailartifact.AuthorizeCleanup(preview, input.PreviewDigest)
	if err != nil {
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Artifact cleanup authority is missing or stale", nil, "Review the current preview and authorize its exact digest", view)
	}
	result, err := store.ApplyCleanup(ctx, preview, authority)
	view.Result = &cleanupResultView{RecordID: result.RecordID, Removed: result.Removed, ReclaimedBytes: result.ReclaimedBytes, Partial: result.Partial, AuditRecorded: result.AuditRecorded}
	switch {
	case err == nil:
		next := "Cleanup record retained; no further effect is required"
		if preview.RemainingEligible > 0 {
			next = "Preview again to review the next bounded cleanup batch"
		}
		return s.maintenanceResult(completion.Facts{Completed: true}, "Artifact cleanup confirmed", append(references, "cleanup-record:"+result.RecordID), next, view)
	case len(result.Removed) > 0:
		return s.maintenanceResult(completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}, "Artifact cleanup partially completed", references, "Removed identities are listed; preview again before any further cleanup", view)
	case errors.Is(err, local.ErrConflict):
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Artifact state changed after review", nil, "Prepare and review a fresh preview", nil)
	default:
		return s.maintenanceResult(completion.Facts{Failed: true}, "Artifact cleanup failed before removal", nil, "Inspect preserved state and retry with a fresh preview", view)
	}
}

type retirementView struct {
	Preview detailartifact.RetirementPreview `json:"preview"`
	Result  *local.RetirementResult          `json:"result,omitempty"`
}

// ArtifactRetire records an explicit Evidence retirement: preview, exact
// authority, and revalidation under lock. Nothing is removed; the 365-day
// cleanup window starts at the published retirement.
func (s lifecycleService) ArtifactRetire(ctx context.Context, input cli.MaintenanceInput) cli.Result {
	store, err := local.NewArtifactStore(s.stateRoot)
	if err != nil {
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, "Artifact root is unsafe", nil, "Use an absolute non-root LINGO_STATE_ROOT", nil)
	}
	preview, err := store.PreviewRetirement(ctx, time.Now().UTC(), input.ArtifactID)
	switch {
	case errors.Is(err, local.ErrUnsafe):
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, "Artifact identity is invalid", nil, "Provide an artifact identity from a Lingo detail reference", nil)
	case errors.Is(err, local.ErrNotFound):
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, "Artifact was not found", nil, "Provide an existing artifact identity", nil)
	case errors.Is(err, local.ErrConflict):
		return s.maintenanceResult(completion.Facts{RetrySafeFailure: true}, "Artifact state is locked by another operation", nil, "Retry after the other operation completes", nil)
	case err != nil:
		return s.maintenanceResult(completion.Facts{Failed: true}, "Artifact or reference state is uncertain; retirement is denied", nil, "Run `axiom recovery inspect` and resolve preserved state before retirement", nil)
	}
	view := retirementView{Preview: preview}
	references := []string{"retirement:" + preview.Digest}
	if preview.Denied != "" {
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Artifact retirement denied: "+preview.Denied, references, "Only unreferenced, unretired Evidence can be retired", view)
	}
	if !input.AuthorizeLocal {
		return s.maintenanceResult(completion.Facts{Completed: true}, "Artifact retirement preview ready", references, "Review the artifact, then repeat with --preview-digest "+preview.Digest+" --authorize-local", view)
	}
	authority, err := detailartifact.AuthorizeRetirement(preview, input.PreviewDigest)
	if err != nil {
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Artifact retirement authority is missing or stale", nil, "Review the current preview and authorize its exact digest", view)
	}
	result, err := store.ApplyRetirement(ctx, preview, authority)
	switch {
	case err == nil:
		view.Result = &result
		return s.maintenanceResult(completion.Facts{Completed: true}, "Evidence retirement recorded", append(references, "retirement-record:"+result.RecordID), "Evidence becomes cleanup-eligible 365 days after retirement unless referenced again", view)
	case errors.Is(err, local.ErrConflict):
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Artifact state changed after review", nil, "Prepare and review a fresh preview", nil)
	default:
		return s.maintenanceResult(completion.Facts{Failed: true}, "Artifact retirement was not confirmed", nil, "Run `axiom recovery inspect` before any further retirement or cleanup", nil)
	}
}

type recoveryView struct {
	Plans      []local.RecoveryPlan  `json:"plans"`
	PlansTotal int                   `json:"plansTotal"`
	Digest     string                `json:"digest"`
	Result     *local.RecoveryResult `json:"result,omitempty"`
}

func (s lifecycleService) recoveryRoots() local.RecoveryRoots {
	return local.RecoveryRoots{State: s.stateRoot, Projects: s.projectsRoot}
}

func (s lifecycleService) RecoveryInspect(ctx context.Context) cli.Result {
	report, err := local.InspectRecovery(ctx, s.recoveryRoots())
	if err != nil {
		if errors.Is(err, local.ErrConflict) {
			return s.maintenanceResult(completion.Facts{RetrySafeFailure: true}, "Owned state is locked by another operation", nil, "Retry after the other operation completes", nil)
		}
		return s.maintenanceResult(completion.Facts{Failed: true}, "Recovery inspection could not read owned roots safely", nil, "Run `axiom compatibility inspect` and preserve state for operator review", nil)
	}
	view := recoveryView{Plans: report.Plans, PlansTotal: len(report.Plans), Digest: report.Digest}
	if len(view.Plans) > maxViewItems {
		view.Plans = view.Plans[:maxViewItems]
	}
	if len(report.Plans) == 0 {
		return s.maintenanceResult(completion.Facts{Completed: true}, "No interrupted local publication found", []string{"recovery:" + report.Digest}, "No recovery is required", view)
	}
	next := "Every plan requires operator review; no automatic action is available"
	for _, plan := range report.Plans {
		if plan.Action != local.PreservedReview {
			next = "Review one plan, then run `axiom recovery apply --preview-digest <plan-digest> --authorize-local`"
			break
		}
	}
	return s.maintenanceResult(completion.Facts{Completed: true}, "Interrupted local publication found", []string{"recovery:" + report.Digest}, next, view)
}

func (s lifecycleService) RecoveryApply(ctx context.Context, input cli.MaintenanceInput) cli.Result {
	report, err := local.InspectRecovery(ctx, s.recoveryRoots())
	if err != nil {
		return s.maintenanceResult(completion.Facts{Failed: true}, "Recovery inspection could not read owned roots safely", nil, "Preserve state for operator review", nil)
	}
	var selected *local.RecoveryPlan
	for index := range report.Plans {
		if report.Plans[index].Digest == input.PreviewDigest {
			selected = &report.Plans[index]
		}
	}
	if selected == nil {
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Recovery plan is missing or stale", nil, "Run `axiom recovery inspect` and authorize a current plan digest", nil)
	}
	if selected.Action == local.PreservedReview {
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, "Recovery plan requires operator review", nil, "Preserve the state; no automatic recovery action is supported", recoveryView{Plans: []local.RecoveryPlan{*selected}, PlansTotal: 1})
	}
	authority, err := local.AuthorizeRecovery(*selected, input.PreviewDigest)
	if err != nil {
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Recovery authority is missing or stale", nil, "Run `axiom recovery inspect` and authorize a current plan digest", nil)
	}
	result, err := local.ApplyRecovery(ctx, s.recoveryRoots(), *selected, authority)
	view := recoveryView{Plans: []local.RecoveryPlan{*selected}, PlansTotal: 1, Result: &result}
	switch {
	case err == nil:
		return s.maintenanceResult(completion.Facts{Completed: true}, "Recovery applied: "+string(result.Action), []string{"recovery-plan:" + selected.Digest}, "Run `axiom recovery inspect` to confirm no interrupted state remains", view)
	case errors.Is(err, local.ErrConflict):
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Recovery state changed or is locked", nil, "Run `axiom recovery inspect` and review a fresh plan", nil)
	case len(result.Removed) > 0:
		return s.maintenanceResult(completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}, "Recovery partially applied; state remains recovery_required", []string{"recovery-plan:" + selected.Digest}, "Run `axiom recovery inspect`; residual protocol objects are preserved", view)
	default:
		return s.maintenanceResult(completion.Facts{Failed: true}, "Recovery failed before any effect", nil, "Run `axiom recovery inspect`; state is preserved", view)
	}
}

type upgradeView struct {
	Preview install.Preview `json:"preview"`
	Result  *install.Result `json:"result,omitempty"`
}

func (s lifecycleService) Upgrade(ctx context.Context, input cli.MaintenanceInput) cli.Result {
	for _, path := range []string{input.Archive, input.Checksums, input.BinaryDir, input.ReceiptDir} {
		if !filepath.IsAbs(path) {
			return s.maintenanceResult(completion.Facts{ValidationFailed: true}, "Upgrade input is invalid", nil, "Provide absolute archive, checksum, binary, and receipt paths", nil)
		}
	}
	candidate, err := install.LoadCandidate(input.Archive, input.Checksums)
	if err != nil {
		return s.maintenanceResult(completion.Facts{ValidationFailed: true}, "Upgrade candidate failed verification: "+upgradeCategory(err), nil, "Use the exact published archive and SHA256SUMS", nil)
	}
	target := install.Target{BinaryDir: filepath.Clean(input.BinaryDir), ReceiptDir: filepath.Clean(input.ReceiptDir), SkillsRoot: s.skillsRoot, State: compatibility.Roots{Projects: s.projectsRoot, State: s.stateRoot}}
	service := install.NewService()
	preview, err := service.Preview(ctx, target, candidate)
	if err != nil {
		category := upgradeCategory(err)
		facts := completion.Facts{ValidationFailed: true}
		if category == "installation_busy_or_interrupted" {
			facts = completion.Facts{RetrySafeFailure: true}
		}
		return s.maintenanceResult(facts, "Upgrade blocked before any effect: "+category, nil, upgradeNext(category), upgradeView{Preview: preview})
	}
	references := []string{"upgrade:" + preview.Digest}
	if len(preview.Effects) == 0 && len(preview.Leftovers) == 0 && !preview.Resume {
		return s.maintenanceResult(completion.Facts{Completed: true}, "Installation already matches the candidate", references, "No upgrade effect is required", upgradeView{Preview: preview})
	}
	if !input.AuthorizeLocal {
		return s.maintenanceResult(completion.Facts{Completed: true}, "Upgrade preview ready", references, "Review effects, then repeat with --preview-digest "+preview.Digest+" --authorize-local", upgradeView{Preview: preview})
	}
	authority, err := install.Authorize(preview, input.PreviewDigest)
	if err != nil {
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Upgrade authority is missing or stale", nil, "Review the current preview and authorize its exact digest", upgradeView{Preview: preview})
	}
	result, err := service.Apply(ctx, preview, authority)
	view := upgradeView{Preview: preview, Result: &result}
	switch {
	case err == nil && result.Status == "success":
		return s.maintenanceResult(completion.Facts{Completed: true}, "Upgrade confirmed", references, "Run `axiom version` to verify the upgraded binary", view)
	case err == nil:
		return s.maintenanceResult(completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}, "Binary, receipt, and Codex skill files upgraded; the Codex skill-set receipt was not refreshed", references, "Run `axiom first-run` with the upgraded binary to refresh the skill-set receipt and converge every detected Runtime, or `axiom runtime codex install` when Codex is not on PATH", view)
	case len(result.Ledger) > 0:
		return s.maintenanceResult(completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}, "Upgrade partially applied: "+upgradeCategory(err), references, "Repeat `axiom upgrade` with the same archive to preview the resumable remaining effects", view)
	case upgradeCategory(err) == "authority_denied":
		return s.maintenanceResult(completion.Facts{AuthorityDenied: true}, "Upgrade state changed after review", nil, "Prepare and review a fresh preview", nil)
	default:
		return s.maintenanceResult(completion.Facts{Failed: true}, "Upgrade failed before any confirmed effect: "+upgradeCategory(err), nil, upgradeNext(upgradeCategory(err)), view)
	}
}

func upgradeCategory(err error) string {
	var upgradeErr *install.Error
	if errors.As(err, &upgradeErr) {
		return upgradeErr.Category
	}
	return "failure"
}

func upgradeNext(category string) string {
	switch category {
	case "state_incompatible":
		return "Run `axiom compatibility inspect`; only absent_v1 or valid_v1 state can be upgraded"
	case "recovery_required", "installation_busy_or_interrupted":
		return "Resume with the same archive, or inspect the receipt directory for another operation"
	case "downgrade_refused", "divergent_equivalent_version":
		return "Downgrade and divergent same-version replacement are not supported"
	case "owned_receipt_required", "receipt_invalid", "binary_modified", "unsafe_binary", "unsafe_target":
		return "Only an unmodified owned release installation can be upgraded; preserve it for review"
	case "unsupported_host":
		return "Use the archive for this exact approved OS, version, and architecture"
	case "insufficient_space":
		return "Free space in the binary directory and Codex skill root, then preview again"
	case "skill_conflict":
		return "Axiom skills in the Codex skill root are modified or not owned; preserve them for review"
	case "skill_set_busy_or_interrupted":
		return "Wait for the running Codex skill install to finish, then prepare a fresh preview"
	}
	return "Inspect the installation and retry with a fresh preview"
}
