// Package install owns the exact, authorized upgrade of an Axiom-owned
// release installation (binary, receipt, Codex skill files) with explicit
// partial truth.
package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/rgomids/axiom/internal/codexruntime"
	"github.com/rgomids/axiom/internal/compatibility"
	"github.com/rgomids/axiom/internal/local"
	"golang.org/x/sys/unix"
)

const (
	binaryName      = "axiom"
	receiptName     = "installation.receipt"
	lockName        = ".axiom-install.lock"
	markerName      = ".axiom-install-operation"
	binaryStage     = ".axiom-binary-stage."
	receiptStage    = ".axiom-install-receipt."
	maxReceiptBytes = 64 << 10
)

const (
	SkillsNotConfigured = "not_configured"
	SkillsMatch         = "matches"
	SkillsPublish       = "publish"
)

// The Codex skill-set receipt is derived from the running binary's runtime
// constants, which release archive format 1 does not carry for the candidate.
// The upgrade therefore never writes it and reports when it must be refreshed.
const (
	SkillReceiptUnchanged       = "unchanged"
	SkillReceiptRefreshRequired = "refresh_required"
)

var (
	receiptFields    = []string{"formatVersion", "destination", "sha256", "product", "version", "revision", "sourceState", "release", "platform", "goos", "architecture", "skillSetVersion", "archiveSha256", "skillManifestSha256", "installedAt"}
	installedPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`)
)

// Error carries a fixed presentation-safe category; it never wraps paths or
// operating-system text for display.
type Error struct{ Category string }

func (e *Error) Error() string { return "upgrade: " + e.Category }

// Target names the explicit owned roots. Axiom skill files under SkillsRoot
// are replaced only when owned; State is inspected read-only and never written.
type Target struct {
	BinaryDir  string
	ReceiptDir string
	SkillsRoot string
	State      compatibility.Roots
}

type Effect struct {
	Kind     string `json:"kind"`
	Name     string `json:"name,omitempty"`
	Target   string `json:"target"`
	Expected string `json:"expected"`
	Next     string `json:"next"`
}

type Preview struct {
	SourceVersion  string                       `json:"sourceVersion"`
	TargetVersion  string                       `json:"targetVersion"`
	ArchiveSHA256  string                       `json:"archiveSha256"`
	Resume         bool                         `json:"resume"`
	Effects        []Effect                     `json:"effects"`
	Leftovers      []string                     `json:"leftovers"`
	State          compatibility.Classification `json:"stateClassification"`
	StateDigest    string                       `json:"stateDigest"`
	Skills         string                       `json:"skills"`
	RequiredBytes  int64                        `json:"requiredBytes"`
	AvailableBytes uint64                       `json:"availableBytes"`
	Digest         string                       `json:"digest"`
	target         Target
	candidate      Candidate
	currentReceipt []byte
	nextReceipt    []byte
	markerSkills   map[string]string
}

type Authority struct{ digest string }

type LedgerEntry struct {
	Kind      string `json:"kind"`
	Name      string `json:"name,omitempty"`
	Target    string `json:"target"`
	Revision  string `json:"revision"`
	Confirmed bool   `json:"confirmed"`
}

type Result struct {
	Status       string        `json:"status"`
	Ledger       []LedgerEntry `json:"ledger"`
	Skills       string        `json:"skills"`
	SkillReceipt string        `json:"skillReceipt,omitempty"`
}

// Service holds deterministic seams; production uses NewService.
type Service struct {
	afterEffect    func(string) error
	availableSpace func(string) (uint64, error)
}

func NewService() Service { return Service{} }

func (s Service) Preview(ctx context.Context, target Target, candidate Candidate) (Preview, error) {
	return s.preview(ctx, target, candidate, false)
}

func (s Service) preview(ctx context.Context, target Target, candidate Candidate, lockHeld bool) (Preview, error) {
	if err := ctx.Err(); err != nil {
		return Preview{}, err
	}
	for _, directory := range []string{target.BinaryDir, target.ReceiptDir} {
		if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || directory == string(filepath.Separator) {
			return Preview{}, &Error{Category: "invalid_target"}
		}
	}
	// The binary directory may be a pre-existing user directory such as
	// ~/.local/bin, which only has to be safe from other principals; the
	// receipt directory is Axiom-owned state and stays owner-only.
	if local.CheckPublicationDirectory(target.BinaryDir) != nil || local.CheckPrivateDirectory(target.ReceiptDir) != nil {
		return Preview{}, &Error{Category: "unsafe_target"}
	}
	if len(candidate.Binary) == 0 || candidate.ArchiveSHA256 == "" || candidate.Values == nil {
		return Preview{}, &Error{Category: "invalid_candidate"}
	}
	row := candidate.Values["platform"] + ":" + candidate.Values["goos"] + ":" + candidate.Values["architecture"]
	if hostRow() != row {
		return Preview{}, &Error{Category: "unsupported_host"}
	}
	if !lockHeld {
		if _, err := os.Lstat(filepath.Join(target.ReceiptDir, lockName)); !os.IsNotExist(err) {
			return Preview{}, &Error{Category: "installation_busy_or_interrupted"}
		}
	}
	preview := Preview{TargetVersion: candidate.Version, ArchiveSHA256: candidate.ArchiveSHA256, Effects: []Effect{}, Leftovers: []string{}, target: target, candidate: candidate}
	if marker, err := readMarker(target.ReceiptDir); err == nil {
		if marker["archiveSha256"] != candidate.ArchiveSHA256 || marker["operation"] != "upgrade" {
			return Preview{}, &Error{Category: "recovery_required"}
		}
		preview.Resume = true
		preview.markerSkills = recordedSkills(marker)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Preview{}, &Error{Category: "recovery_required"}
	}
	currentReceipt, err := local.ReadOwnedFile(target.ReceiptDir, receiptName, maxReceiptBytes)
	if err != nil {
		return Preview{}, &Error{Category: "owned_receipt_required"}
	}
	values, err := parseReceipt(currentReceipt)
	destination := filepath.Join(target.BinaryDir, binaryName)
	if err != nil || values["destination"] != destination {
		return Preview{}, &Error{Category: "receipt_invalid"}
	}
	binary, err := local.ReadPublishedFile(target.BinaryDir, binaryName, maxBinaryBytes)
	if err != nil {
		return Preview{}, &Error{Category: "unsafe_binary"}
	}
	currentDigest, candidateDigest := digest(binary), digest(candidate.Binary)
	if currentDigest != values["sha256"] && !(preview.Resume && currentDigest == candidateDigest) {
		return Preview{}, &Error{Category: "binary_modified"}
	}
	preview.currentReceipt = currentReceipt
	preview.nextReceipt = expectedReceipt(destination, candidate, values["installedAt"])
	receiptAlreadyNext := bytes.Equal(currentReceipt, preview.nextReceipt)
	preview.SourceVersion = values["version"]
	comparison := compareSemver(candidate.Version, values["version"])
	switch {
	case receiptAlreadyNext:
	case comparison < 0:
		return Preview{}, &Error{Category: "downgrade_refused"}
	case comparison == 0 && candidateDigest != values["sha256"]:
		return Preview{}, &Error{Category: "divergent_equivalent_version"}
	}
	state, err := compatibility.Inspect(ctx, target.State)
	if err != nil {
		return Preview{}, &Error{Category: "state_inspection_failed"}
	}
	preview.State, preview.StateDigest = state.Classification, state.Digest
	if state.Classification != compatibility.AbsentV1 && state.Classification != compatibility.ValidV1 {
		return preview, &Error{Category: "state_incompatible"}
	}
	if currentDigest != candidateDigest {
		preview.Effects = append(preview.Effects, Effect{Kind: "binary", Target: destination, Expected: currentDigest, Next: candidateDigest})
		preview.RequiredBytes += int64(len(candidate.Binary))
	}
	if !receiptAlreadyNext {
		preview.Effects = append(preview.Effects, Effect{Kind: "receipt", Target: filepath.Join(target.ReceiptDir, receiptName), Expected: digest(currentReceipt), Next: digest(preview.nextReceipt)})
		preview.RequiredBytes += int64(len(preview.nextReceipt))
	}
	skills, err := planSkills(ctx, target.SkillsRoot, candidate, values["skillManifestSha256"], preview.markerSkills, preview.Resume)
	if err != nil {
		return preview, err
	}
	preview.Skills = skills.state
	preview.Effects = append(preview.Effects, skills.effects...)
	if preview.Resume {
		preview.Leftovers = append(stageLeftovers(target), skills.leftovers...)
		sort.Strings(preview.Leftovers)
	}
	space := s.availableSpace
	if space == nil {
		space = statfsAvailable
	}
	if len(preview.Effects) != len(skills.effects) {
		available, err := space(target.BinaryDir)
		if err != nil || uint64(preview.RequiredBytes)+maxReceiptBytes > available {
			return preview, &Error{Category: "insufficient_space"}
		}
		preview.AvailableBytes = available
	}
	if len(skills.effects) != 0 {
		available, err := space(target.SkillsRoot)
		if err != nil || uint64(skills.bytes)+maxReceiptBytes > available {
			return preview, &Error{Category: "insufficient_space"}
		}
		preview.RequiredBytes += skills.bytes
	}
	preview.Digest = previewDigest(preview)
	return preview, nil
}

func previewDigest(preview Preview) string {
	wire, _ := json.Marshal(struct {
		Source, Target, Archive string
		Resume                  bool
		Effects                 []Effect
		Leftovers               []string
		State                   compatibility.Classification
		StateDigest, Skills     string
	}{preview.SourceVersion, preview.TargetVersion, preview.ArchiveSHA256, preview.Resume, preview.Effects, preview.Leftovers, preview.State, preview.StateDigest, preview.Skills})
	return digest(wire)
}

func Authorize(preview Preview, reviewedDigest string) (Authority, error) {
	// A resumed operation with no remaining publication still needs authority
	// to clear its owned marker; otherwise the installation stays blocked.
	if len(preview.Effects) == 0 && len(preview.Leftovers) == 0 && !preview.Resume || preview.Digest == "" || preview.Digest != reviewedDigest {
		return Authority{}, &Error{Category: "authority_denied"}
	}
	return Authority{digest: preview.Digest}, nil
}

// Apply publishes each effect separately in preview order (binary, receipt,
// skill files); there is no cross-root transaction. A later failure is
// reported as partial with every confirmed effect listed, and the owned
// operation marker keeps the installation resumable.
//
// ReceiptDir is opened exactly once, as an anchored, validated directory
// object: the install lock, every marker write and its final removal, and the
// receipt file publication all resolve through that same object for the rest
// of this call, never by re-deriving ReceiptDir's pathname. BinaryDir is
// opened the same way, lazily, the first time it is needed. This closes the
// gap where validating a directory by pathname and later mutating it by
// pathname again leaves a window in which the two operations could land on
// different objects if the directory (or one of its ancestors) were replaced
// in between (ADR-0005 property 3, controlled ancestor/leaf replacement).
//
// Binding to the object is not by itself truthful: each anchored directory
// must also still be the object visible at its authorized pathname at every
// inspection and commit boundary. The lock, the marker, every stage and
// rename refuse once either directory or one of its ancestors was replaced;
// each published effect is confirmed at its pathname after the commit; and
// success is declared only after both directories are proven, once more, to
// be the objects the operation was authorized for. A replacement is reported
// as target_changed before a commit and never as a confirmed effect after one.
func (s Service) Apply(ctx context.Context, preview Preview, authority Authority) (Result, error) {
	result := Result{Status: "denied_authority", Ledger: []LedgerEntry{}}
	if authority.digest == "" || authority.digest != preview.Digest {
		return result, &Error{Category: "authority_denied"}
	}
	receiptDir, err := local.OpenOwnedDirectory(preview.target.ReceiptDir)
	if err != nil {
		result.Status = "failure"
		return result, &Error{Category: "unsafe_target"}
	}
	defer receiptDir.Close()
	if err := receiptDir.Mkdir(lockName); err != nil {
		result.Status = "failure"
		if errors.Is(err, local.ErrReplaced) {
			return result, &Error{Category: "target_changed"}
		}
		return result, &Error{Category: "installation_busy_or_interrupted"}
	}
	defer func() { _ = receiptDir.Remove(lockName) }()
	var skills codexruntime.Service
	var skillSession *codexruntime.UpgradeSession
	if touchesSkills(preview) {
		var err error
		if skills, err = codexruntime.New(preview.target.SkillsRoot); err != nil {
			result.Status = "failure"
			return result, &Error{Category: "skill_inspection_failed"}
		}
		skillSession, err = skills.LockForUpgrade()
		if err != nil {
			result.Status = "failure"
			return result, &Error{Category: "skill_set_busy_or_interrupted"}
		}
		defer skillSession.Close()
	}
	current, err := s.preview(ctx, preview.target, preview.candidate, true)
	if err != nil || current.Digest != preview.Digest {
		return result, &Error{Category: "authority_denied"}
	}
	result.Status = "failure"
	// binaryDir is opened lazily against current.target.BinaryDir, the first
	// time an effect or cleanup step needs it, and kept open (and reused) for
	// the rest of this call.
	var binaryDir local.AnchoredDirectory
	binaryDirOpen := false
	openBinaryDir := func() (local.AnchoredDirectory, error) {
		if binaryDirOpen {
			return binaryDir, nil
		}
		opened, err := local.OpenPublicationDirectory(current.target.BinaryDir)
		if err != nil {
			return local.AnchoredDirectory{}, err
		}
		binaryDir, binaryDirOpen = opened, true
		return binaryDir, nil
	}
	defer func() {
		if binaryDirOpen {
			_ = binaryDir.Close()
		}
	}()
	recorded := current.markerSkills
	if !current.Resume {
		recorded = map[string]string{}
		for _, effect := range current.Effects {
			if effect.Kind == "skill" {
				recorded[effect.Name] = effect.Expected
			}
		}
		if err := writeMarker(receiptDir, current.candidate.ArchiveSHA256, "prepare", true, recorded); err != nil {
			if errors.Is(err, local.ErrReplaced) {
				return result, &Error{Category: "target_changed"}
			}
			return result, &Error{Category: "marker_unavailable"}
		}
	}
	for _, effect := range current.Effects {
		if err := ctx.Err(); err != nil {
			return partial(result), err
		}
		var err error
		switch effect.Kind {
		case "binary":
			directory, openErr := openBinaryDir()
			if openErr != nil {
				err = &Error{Category: "unsafe_target"}
				break
			}
			err = publishEffect(directory, binaryName, binaryStage, current.candidate.Binary, effect, 0o700, maxBinaryBytes)
		case "receipt":
			err = publishEffect(receiptDir, receiptName, receiptStage, current.nextReceipt, effect, 0o600, maxReceiptBytes)
		case "skill":
			expected := effect.Expected
			if expected == absentRevision {
				expected = ""
			}
			if skillSession.PublishSkill(effect.Name, current.candidate.SkillFiles[effect.Name], expected) != nil {
				err = &Error{Category: "skill_publication_failed"}
			}
		default:
			err = &Error{Category: "invalid_effect"}
		}
		if err != nil {
			return partial(result), err
		}
		result.Ledger = append(result.Ledger, LedgerEntry{Kind: effect.Kind, Name: effect.Name, Target: effect.Target, Revision: effect.Next, Confirmed: true})
		if effect.Kind == "binary" {
			if err := writeMarker(receiptDir, current.candidate.ArchiveSHA256, "binary_committed", false, recorded); errors.Is(err, local.ErrReplaced) {
				return partial(result), &Error{Category: "target_changed"}
			} else if err != nil {
				return partial(result), &Error{Category: "marker_unavailable"}
			}
		}
		if s.afterEffect != nil {
			label := effect.Kind
			if effect.Name != "" {
				label += ":" + effect.Name
			}
			if err := s.afterEffect(label); err != nil {
				return partial(result), err
			}
		}
	}
	for _, leftover := range current.Leftovers {
		switch parent := filepath.Dir(leftover); {
		case parent == current.target.BinaryDir:
			if directory, err := openBinaryDir(); err == nil {
				_ = directory.Remove(filepath.Base(leftover))
			}
		case parent == current.target.ReceiptDir:
			_ = receiptDir.Remove(filepath.Base(leftover))
		case skillSession != nil && filepath.Dir(parent) == current.target.SkillsRoot:
			_ = skillSession.RemoveSkillLeftover(filepath.Base(parent), filepath.Base(leftover))
		}
	}
	// The operation marker is recovery state for the authorized objects: it
	// is cleared only while both directories are still those objects.
	if receiptDir.StillAtPath() != nil || binaryDirOpen && binaryDir.StillAtPath() != nil {
		return partial(result), &Error{Category: "final_verification_failed"}
	}
	if err := receiptDir.Remove(markerName); err != nil && !errors.Is(err, os.ErrNotExist) {
		return partial(result), &Error{Category: "marker_cleanup_failed"}
	}
	_ = receiptDir.Sync()
	final, err := compatibility.Inspect(ctx, current.target.State)
	if err != nil || final.Classification != compatibility.AbsentV1 && final.Classification != compatibility.ValidV1 {
		return partial(result), &Error{Category: "final_verification_failed"}
	}
	verified, err := planSkills(ctx, current.target.SkillsRoot, current.candidate, "", nil, false)
	if err != nil || len(verified.effects) != 0 || len(verified.leftovers) != 0 {
		return partial(result), &Error{Category: "final_verification_failed"}
	}
	if receiptDir.StillAtPath() != nil || binaryDirOpen && binaryDir.StillAtPath() != nil {
		return partial(result), &Error{Category: "final_verification_failed"}
	}
	result.Skills, result.Status = verified.state, "success"
	if verified.state != SkillsNotConfigured {
		// Skill files changed by this operation, including an interrupted
		// earlier run, leave the skill-set receipt for the upgraded binary.
		result.SkillReceipt = SkillReceiptUnchanged
		if len(recorded) != 0 {
			result.SkillReceipt, result.Status = SkillReceiptRefreshRequired, "partial"
		}
	}
	return result, nil
}

func touchesSkills(preview Preview) bool {
	for _, effect := range preview.Effects {
		if effect.Kind == "skill" {
			return true
		}
	}
	for _, leftover := range preview.Leftovers {
		if strings.HasPrefix(filepath.Base(leftover), codexruntime.UpgradeStagePrefix) {
			return true
		}
	}
	return len(preview.markerSkills) != 0
}

func partial(result Result) Result {
	if len(result.Ledger) > 0 {
		result.Status = "partial"
	}
	return result
}

// publishEffect stages private bytes inside dir, rechecks the exact expected
// current revision, renames, and confirms the published revision, all
// through the same anchored directory object: dir must already be the
// directory whose identity was just validated by its caller (OpenOwnedDirectory
// or OpenPublicationDirectory), so this never re-derives the target by
// pathname and cannot be redirected by a later replacement of dir at its
// pathname. A replacement of dir or an ancestor observed before the commit
// is refused as target_changed with nothing renamed; one observed after the
// commit is publication_uncertain, never a confirmed effect.
func publishEffect(dir local.AnchoredDirectory, name, prefix string, wire []byte, effect Effect, mode os.FileMode, limit int) error {
	stage, err := dir.Stage(prefix, wire, mode)
	if errors.Is(err, local.ErrReplaced) {
		return &Error{Category: "target_changed"}
	} else if err != nil {
		return &Error{Category: "stage_unavailable"}
	}
	committed := false
	defer func() {
		if !committed {
			_ = dir.Remove(stage)
		}
	}()
	staged, err := dir.ReadFile(stage, limit)
	if err != nil || digest(staged) != effect.Next {
		return &Error{Category: "stage_verification_failed"}
	}
	current, err := dir.ReadFile(name, limit)
	if err != nil || digest(current) != effect.Expected {
		return &Error{Category: "target_changed"}
	}
	if err := dir.Rename(stage, name); errors.Is(err, local.ErrReplaced) {
		return &Error{Category: "target_changed"}
	} else if err != nil {
		return &Error{Category: "publication_failed"}
	}
	committed = true
	_ = dir.Sync()
	confirmed, err := dir.ReadFile(name, limit)
	if err != nil || digest(confirmed) != effect.Next || dir.StillAtPath() != nil {
		return &Error{Category: "publication_uncertain"}
	}
	return nil
}

const absentRevision = "absent"

type skillPlan struct {
	state     string
	effects   []Effect
	leftovers []string
	bytes     int64
}

// planSkills derives Axiom skill effects from the verified candidate. A
// present skill may be replaced only when owned: its whole set matches the
// installation receipt's skill manifest, it is known to this binary, or a
// resumed operation recorded it as the authorized expected revision.
func planSkills(ctx context.Context, root string, candidate Candidate, installedManifest string, recorded map[string]string, resume bool) (skillPlan, error) {
	plan := skillPlan{state: SkillsNotConfigured, effects: []Effect{}, leftovers: []string{}}
	if root == "" {
		return plan, nil
	}
	service, err := codexruntime.New(root)
	if err != nil {
		return plan, &Error{Category: "skill_inspection_failed"}
	}
	inventory, err := service.InspectUpgrade(ctx)
	if errors.Is(err, codexruntime.ErrUpgradeConflict) {
		return plan, &Error{Category: "skill_conflict"}
	} else if err != nil {
		return plan, &Error{Category: "skill_inspection_failed"}
	}
	if !inventory.Configured {
		return plan, nil
	}
	setOwned := installedManifest != "" && installedSkillManifest(inventory) == installedManifest
	for _, skill := range inventory.Skills {
		if len(skill.Leftovers) != 0 && !resume {
			return plan, &Error{Category: "recovery_required"}
		}
		plan.leftovers = append(plan.leftovers, skill.Leftovers...)
		current := skill.SHA256
		if current == "" {
			current = absentRevision
		}
		next, content := candidate.Skills[skill.Name], candidate.SkillFiles[skill.Name]
		if next == "" || digest(content) != next {
			return plan, &Error{Category: "invalid_candidate"}
		}
		if expected, ok := recorded[skill.Name]; ok && current != expected && current != next {
			return plan, &Error{Category: "recovery_required"}
		}
		if current == next {
			continue
		}
		_, recordedSkill := recorded[skill.Name]
		if current != absentRevision && !setOwned && !skill.Owned && !recordedSkill {
			return plan, &Error{Category: "skill_conflict"}
		}
		plan.effects = append(plan.effects, Effect{Kind: "skill", Name: skill.Name, Target: filepath.Join(root, skill.Name, "SKILL.md"), Expected: current, Next: next})
		plan.bytes += int64(len(content))
	}
	plan.state = SkillsMatch
	if len(plan.effects) != 0 {
		plan.state = SkillsPublish
	}
	return plan, nil
}

// installedSkillManifest reconstructs the release skill manifest that
// build-release-archives.sh writes for the observed skill digests, so its
// digest can be compared with the installation receipt.
func installedSkillManifest(inventory codexruntime.UpgradeInventory) string {
	var builder strings.Builder
	builder.WriteString("formatVersion=1\nskillSetVersion=1\nbinaryCompatibility=1\n")
	for _, skill := range inventory.Skills {
		if skill.SHA256 == "" {
			return ""
		}
		builder.WriteString("skill." + skill.Name + "=" + skill.SHA256 + "\n")
	}
	return digest([]byte(builder.String()))
}

func expectedReceipt(destination string, candidate Candidate, installedAt string) []byte {
	var builder strings.Builder
	builder.WriteString("formatVersion=1\n")
	builder.WriteString("destination=" + destination + "\n")
	builder.WriteString("sha256=" + digest(candidate.Binary) + "\n")
	for _, line := range candidate.Metadata {
		if !strings.HasPrefix(line, "formatVersion=") {
			builder.WriteString(line + "\n")
		}
	}
	builder.WriteString("archiveSha256=" + candidate.ArchiveSHA256 + "\n")
	builder.WriteString("skillManifestSha256=" + candidate.SkillManifestSHA256 + "\n")
	builder.WriteString("installedAt=" + installedAt + "\n")
	return []byte(builder.String())
}

func parseReceipt(wire []byte) (map[string]string, error) {
	lines := strings.Split(strings.TrimSuffix(string(wire), "\n"), "\n")
	if len(lines) != len(receiptFields) || !strings.HasSuffix(string(wire), "\n") {
		return nil, errors.New("receipt schema")
	}
	allowed := map[string]bool{}
	for _, field := range receiptFields {
		allowed[field] = true
	}
	values := map[string]string{}
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok || !allowed[key] || value == "" || values[key] != "" {
			return nil, errors.New("receipt schema")
		}
		values[key] = value
	}
	if values["formatVersion"] != "1" || values["product"] != "Axiom" || !digestPattern.MatchString(values["sha256"]) || !semverPattern.MatchString(values["version"]) || !installedPattern.MatchString(values["installedAt"]) {
		return nil, errors.New("unsupported receipt")
	}
	return values, nil
}

func readMarker(directory string) (map[string]string, error) {
	if _, err := os.Lstat(filepath.Join(directory, markerName)); os.IsNotExist(err) {
		return nil, os.ErrNotExist
	}
	wire, err := local.ReadOwnedFile(directory, markerName, 4096)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(wire), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || values[key] != "" {
			return nil, errors.New("marker schema")
		}
		if name, isSkill := strings.CutPrefix(key, "skill."); isSkill && (!slices.Contains(skillNames, name) || value != absentRevision && !digestPattern.MatchString(value)) {
			return nil, errors.New("marker schema")
		}
		values[key] = value
	}
	if values["formatVersion"] != "1" || !digestPattern.MatchString(values["archiveSha256"]) || values["stage"] != "prepare" && values["stage"] != "binary_committed" {
		return nil, errors.New("marker schema")
	}
	return values, nil
}

// recordedSkills returns the authorized expected skill revisions an upgrade
// recorded before its first effect.
func recordedSkills(marker map[string]string) map[string]string {
	recorded := map[string]string{}
	for key, value := range marker {
		if name, ok := strings.CutPrefix(key, "skill."); ok {
			recorded[name] = value
		}
	}
	return recorded
}

// writeMarker uses the release installer's marker name and fields so the
// installer and the upgrade path refuse each other's interrupted state. It
// also records each skill's authorized expected revision so a resumed upgrade
// can prove ownership of skills an interruption left unpublished.
// writeMarker publishes the operation marker inside receiptDir, the same
// anchored directory object Apply validated once and holds open for the rest
// of the call, never by re-deriving ReceiptDir's pathname.
func writeMarker(receiptDir local.AnchoredDirectory, archive, stage string, create bool, skills map[string]string) error {
	var builder strings.Builder
	fmt.Fprintf(&builder, "formatVersion=1\nstage=%s\narchiveSha256=%s\noperation=upgrade\n", stage, archive)
	for _, name := range skillNames {
		if expected, ok := skills[name]; ok {
			builder.WriteString("skill." + name + "=" + expected + "\n")
		}
	}
	wire := []byte(builder.String())
	if create {
		if err := receiptDir.CreateExclusive(markerName, wire, 0o600); err != nil {
			return err
		}
		if err := receiptDir.Sync(); err != nil {
			return err
		}
		return receiptDir.StillAtPath()
	}
	staged, err := receiptDir.Stage(markerName+".", wire, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = receiptDir.Remove(staged)
		}
	}()
	if err := receiptDir.Rename(staged, markerName); err != nil {
		return err
	}
	committed = true
	if err := receiptDir.Sync(); err != nil {
		return err
	}
	return receiptDir.StillAtPath()
}

func stageLeftovers(target Target) []string {
	leftovers := []string{}
	for _, check := range []struct{ directory, prefix string }{{target.BinaryDir, binaryStage}, {target.ReceiptDir, receiptStage}} {
		entries, err := os.ReadDir(check.directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), check.prefix) && entry.Type().IsRegular() {
				leftovers = append(leftovers, filepath.Join(check.directory, entry.Name()))
			}
		}
	}
	sort.Strings(leftovers)
	return leftovers
}

func syncDirectory(path string) {
	if directory, err := os.Open(path); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
}

func statfsAvailable(path string) (uint64, error) {
	var filesystem unix.Statfs_t
	if err := unix.Statfs(path, &filesystem); err != nil {
		return 0, err
	}
	return uint64(filesystem.Bavail) * uint64(filesystem.Bsize), nil
}

// compareSemver implements SemVer 2.0.0 precedence; build metadata is ignored.
// Numeric identifiers are compared as unbounded decimal strings so that values
// beyond any machine integer keep their SemVer order.
func compareSemver(left, right string) int {
	split := func(value string) ([]string, []string) {
		value, _, _ = strings.Cut(value, "+")
		core, pre, _ := strings.Cut(value, "-")
		identifiers := []string(nil)
		if pre != "" {
			identifiers = strings.Split(pre, ".")
		}
		return strings.Split(core, "."), identifiers
	}
	leftCore, leftPre := split(left)
	rightCore, rightPre := split(right)
	for index := 0; index < 3; index++ {
		if comparison := compareNumericIdentifier(leftCore[index], rightCore[index]); comparison != 0 {
			return comparison
		}
	}
	switch {
	case len(leftPre) == 0 && len(rightPre) == 0:
		return 0
	case len(leftPre) == 0:
		return 1
	case len(rightPre) == 0:
		return -1
	}
	for index := 0; index < len(leftPre) && index < len(rightPre); index++ {
		a, b := leftPre[index], rightPre[index]
		aNumeric, bNumeric := isNumericIdentifier(a), isNumericIdentifier(b)
		switch {
		case aNumeric && bNumeric:
			if comparison := compareNumericIdentifier(a, b); comparison != 0 {
				return comparison
			}
		case aNumeric != bNumeric:
			if aNumeric {
				return -1
			}
			return 1
		case a != b:
			if a < b {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(leftPre) < len(rightPre):
		return -1
	case len(leftPre) > len(rightPre):
		return 1
	}
	return 0
}

// isNumericIdentifier reports whether value is a non-empty run of ASCII digits.
func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

// compareNumericIdentifier orders two digit strings by numeric value without
// converting them to a bounded integer type. Leading zeros are ignored
// defensively; SemVer forbids them but callers need not rely on that.
func compareNumericIdentifier(left, right string) int {
	left, right = strings.TrimLeft(left, "0"), strings.TrimLeft(right, "0")
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	}
	return strings.Compare(left, right)
}
