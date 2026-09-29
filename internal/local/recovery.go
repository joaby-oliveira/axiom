package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/manifest"
	"github.com/rgomids/axiom/internal/project"
)

type RecoveryAction string

const (
	RestorePrior      RecoveryAction = "restore_prior"
	FinalizeCommitted RecoveryAction = "finalize_committed"
	PreservedReview   RecoveryAction = "preserved_review"
)

const (
	RecoveryScopeState    = "state"
	RecoveryScopeProjects = "projects"
)

// RecoveryRoots names the only Axiom-owned roots recovery may inspect.
type RecoveryRoots struct {
	State    string
	Projects string
}

type RecoveryObject struct {
	Role     string `json:"role"`
	Name     string `json:"name"`
	Present  bool   `json:"present"`
	Revision string `json:"revision,omitempty"`
}

type RecoveryPlan struct {
	Scope       string           `json:"scope"`
	Directory   string           `json:"directory"`
	Protocol    string           `json:"protocol"`
	Marker      string           `json:"marker,omitempty"`
	Stage       FaultStage       `json:"stage,omitempty"`
	Action      RecoveryAction   `json:"action"`
	Objects     []RecoveryObject `json:"objects"`
	Explanation string           `json:"explanation"`
	Digest      string           `json:"digest"`
}

type RecoveryReport struct {
	Plans  []RecoveryPlan `json:"plans"`
	Digest string         `json:"digest"`
}

type RecoveryResult struct {
	Action  RecoveryAction `json:"action"`
	Removed []string       `json:"removed"`
}

type RecoveryAuthority struct{ digest string }

const (
	protocolFile      = "file"
	protocolDirectory = "directory"
	protocolAttempt   = "attempt"
	protocolUnknown   = "unknown"
)

// InspectRecovery is read-only. It lists recognized interrupted protocol
// state below the owned roots and never selects an action from incomplete facts.
func InspectRecovery(ctx context.Context, roots RecoveryRoots) (RecoveryReport, error) {
	report := RecoveryReport{Plans: []RecoveryPlan{}}
	for _, scope := range []struct{ name, path string }{{RecoveryScopeState, roots.State}, {RecoveryScopeProjects, roots.Projects}} {
		if scope.path == "" {
			continue
		}
		if !filepath.IsAbs(scope.path) || filepath.Clean(scope.path) == string(filepath.Separator) {
			return RecoveryReport{}, ErrUnsafe
		}
		directories, err := recoveryDirectories(ctx, scope.name, filepath.Clean(scope.path))
		if err != nil {
			return RecoveryReport{}, err
		}
		for _, directory := range directories {
			plan, found, err := inspectRecoveryDirectory(ctx, scope.name, filepath.Clean(scope.path), directory, false)
			if err != nil {
				return RecoveryReport{}, err
			}
			if found {
				report.Plans = append(report.Plans, plan)
			}
		}
	}
	wire, _ := json.Marshal(report.Plans)
	digest := sha256.Sum256(wire)
	report.Digest = hex.EncodeToString(digest[:])
	return report, nil
}

// recoveryDirectories enumerates only directories whose owning store uses
// the ADR-0007 protocol or a recognized S2 attempt protocol.
func recoveryDirectories(ctx context.Context, scope, path string) ([]string, error) {
	root, err := existingPrivateRoot(path)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directories := []string{"."}
	if scope == RecoveryScopeProjects {
		names, err := readDirectoryNamesBounded(root, maxLocalDirectoryEntries)
		if err != nil {
			return nil, ErrRecoveryRequired
		}
		for _, name := range names {
			if project.ValidSlug(name) {
				directories = append(directories, name)
			}
		}
		sort.Strings(directories)
		return directories, ctx.Err()
	}
	children := func(parts ...string) error {
		current := root
		opened := []*os.Root{}
		defer func() {
			for _, item := range opened {
				item.Close()
			}
		}()
		for _, part := range parts {
			next, err := existingPrivateChild(current, part)
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			opened = append(opened, next)
			current = next
		}
		names, err := readDirectoryNamesBounded(current, maxLocalDirectoryEntries)
		if err != nil {
			return ErrRecoveryRequired
		}
		prefix := strings.Join(parts, "/")
		for _, name := range names {
			if strings.HasPrefix(name, ".") {
				continue
			}
			if info, err := current.Lstat(name); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				directories = append(directories, prefix+"/"+name)
			}
		}
		return nil
	}
	for _, parts := range [][]string{{"projects"}, {"work-items"}, {"executions", "v1"}, {"artifacts", "v1", "objects"}} {
		if err := children(parts...); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"cleanup", "retirements"} {
		if chainExists(root, []string{"artifacts", "v1", name}) {
			directories = append(directories, "artifacts/v1/"+name)
		}
	}
	sort.Strings(directories)
	return directories, ctx.Err()
}

func chainExists(root *os.Root, parts []string) bool {
	current := root
	for _, part := range parts {
		next, err := existingPrivateChild(current, part)
		if current != root {
			current.Close()
		}
		if err != nil {
			return false
		}
		current = next
	}
	if current != root {
		current.Close()
	}
	return true
}

func splitDirectory(directory string) []string {
	if directory == "." {
		return nil
	}
	return strings.Split(directory, "/")
}

func validRecoveryDirectory(scope, directory string) bool {
	parts := splitDirectory(directory)
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") || strings.ContainsRune(part, filepath.Separator) {
			return false
		}
	}
	return scope == RecoveryScopeState || scope == RecoveryScopeProjects && len(parts) <= 1
}

// inspectRecoveryDirectory opens the chain scope root -> directory and takes
// the same top-down directory locks as the owning stores.
func inspectRecoveryDirectory(ctx context.Context, scope, path, directory string, exclusive bool) (RecoveryPlan, bool, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryPlan{}, false, err
	}
	if !validRecoveryDirectory(scope, directory) {
		return RecoveryPlan{}, false, ErrUnsafe
	}
	root, err := existingPrivateRoot(path)
	if err != nil {
		return RecoveryPlan{}, false, err
	}
	chain := []*os.Root{root}
	defer func() {
		for index := len(chain) - 1; index >= 0; index-- {
			chain[index].Close()
		}
	}()
	for _, part := range splitDirectory(directory) {
		next, err := existingPrivateChild(chain[len(chain)-1], part)
		if err != nil {
			return RecoveryPlan{}, false, err
		}
		chain = append(chain, next)
	}
	locks, err := lockRoots(exclusive, chain...)
	if err != nil {
		return RecoveryPlan{}, false, err
	}
	defer closeFiles(locks)
	return planRecovery(chain[len(chain)-1], scope, directory)
}

func recoveryProtocolName(name string) bool {
	return protocolName(name) || strings.HasPrefix(name, ".lingo-")
}

func planRecovery(target *os.Root, scope, directory string) (RecoveryPlan, bool, error) {
	names, err := readDirectoryNamesBounded(target, maxLocalDirectoryEntries)
	if err != nil {
		return RecoveryPlan{}, false, ErrRecoveryRequired
	}
	sort.Strings(names)
	protocol := make([]string, 0)
	for _, name := range names {
		if recoveryProtocolName(name) {
			protocol = append(protocol, name)
		}
	}
	if len(protocol) == 0 {
		return RecoveryPlan{}, false, nil
	}
	plan := RecoveryPlan{Scope: scope, Directory: directory, Protocol: protocolUnknown, Action: PreservedReview}
	markers := make([]string, 0, 1)
	for _, name := range protocol {
		if strings.HasPrefix(name, ".axiom-recovery-") {
			markers = append(markers, name)
		}
	}
	unknown := func(explanation string) (RecoveryPlan, bool, error) {
		plan.Objects = plan.Objects[:0]
		for _, name := range protocol {
			plan.Objects = append(plan.Objects, RecoveryObject{Role: "unrecognized", Name: name, Present: true})
		}
		plan.Action, plan.Explanation = PreservedReview, explanation
		plan.Digest = recoveryDigest(plan)
		return plan, true, nil
	}
	if len(markers) == 0 {
		plan.Protocol = protocolAttempt
		return unknown("interrupted attempt state lacks prior/new generation facts; preserved for operator review")
	}
	if len(markers) != 1 {
		return unknown("expected exactly one recognized recovery marker")
	}
	marker, err := readProtocolMarker(target, markers[0])
	if err != nil || !plainName(marker.Object) || strings.HasPrefix(marker.Object, ".") || !recognizedStagingName(marker.Staging, marker.Object) {
		return unknown("recovery marker is malformed, unsupported, or names an unsafe object")
	}
	plan.Marker, plan.Stage = markers[0], marker.Stage
	directoryProtocol := scope == RecoveryScopeProjects && directory == "." || scope == RecoveryScopeState && strings.HasPrefix(directory, "artifacts/v1/objects/")
	plan.Protocol = protocolFile
	if directoryProtocol {
		plan.Protocol = protocolDirectory
	}
	allowed := map[string]bool{markers[0]: true, marker.Staging: true}
	updates := make([]RecoveryObject, 0)
	for _, name := range protocol {
		if allowed[name] {
			continue
		}
		if strings.HasPrefix(name, ".axiom-stage-marker-") {
			if update, err := readProtocolMarker(target, name); err == nil && update.OperationID == marker.OperationID && update.Object == marker.Object && update.Staging == marker.Staging {
				updates = append(updates, RecoveryObject{Role: "marker_update", Name: name, Present: true})
				continue
			}
		}
		return unknown("unrecognized protocol object accompanies the recovery marker")
	}
	observe := observeRecoveryFile
	if directoryProtocol {
		observe = func(root *os.Root, role, name string) RecoveryObject {
			return observeRecoveryDirectory(root, role, name, scope)
		}
	}
	canonical := observe(target, "canonical", marker.Object)
	stage := observe(target, "stage", marker.Staging)
	plan.Objects = append([]RecoveryObject{{Role: "marker", Name: markers[0], Present: true}, canonical, stage}, updates...)
	if canonical.Revision == "invalid" || stage.Revision == "invalid" {
		plan.Explanation = "a generation is present but incomplete, corrupt, or unsafe"
		plan.Digest = recoveryDigest(plan)
		return plan, true, nil
	}
	prior := marker.PriorPresent && canonical.Present && canonical.Revision == marker.PriorRevision || !marker.PriorPresent && !canonical.Present
	newCanonical := canonical.Present && canonical.Revision == marker.NewRevision
	newStage := stage.Present && stage.Revision == marker.NewRevision
	switch marker.Stage {
	case FaultF3, FaultF4:
		if prior && (newStage || !stage.Present) {
			plan.Action, plan.Explanation = RestorePrior, "publication stopped before its commit point; prior generation remains canonical"
		}
	case FaultF5:
		switch {
		case prior && (newStage || !stage.Present):
			plan.Action, plan.Explanation = RestorePrior, "commit rename did not occur; prior generation remains canonical"
		case newCanonical && !stage.Present:
			plan.Action, plan.Explanation = FinalizeCommitted, "commit rename occurred; new complete generation is canonical"
		}
	case FaultF6, FaultF7, FaultF8:
		if newCanonical && !stage.Present {
			plan.Action, plan.Explanation = FinalizeCommitted, "commit was recorded; new complete generation is canonical"
		}
	}
	if plan.Action == PreservedReview {
		plan.Explanation = "generation facts are ambiguous or contradict the recorded protocol stage"
	}
	plan.Digest = recoveryDigest(plan)
	return plan, true, nil
}

// recognizedStagingName accepts only the staging prefixes the stores write.
func recognizedStagingName(staging, object string) bool {
	if !plainName(staging) {
		return false
	}
	for _, prefix := range []string{".axiom-stage-file-", ".axiom-stage-artifact-", ".lingo-manifest-", ".lingo-stage-" + object + "-"} {
		if strings.HasPrefix(staging, prefix) && len(staging) > len(prefix) {
			return true
		}
	}
	return false
}

func plainName(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\\x00")
}

func observeRecoveryFile(root *os.Root, role, name string) RecoveryObject {
	result := RecoveryObject{Role: role, Name: name}
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return result
	}
	result.Present = true
	if err != nil || !info.Mode().IsRegular() {
		result.Revision = "invalid"
		return result
	}
	wire, err := readPrivateFile(root, name)
	if err != nil {
		result.Revision = "invalid"
		return result
	}
	digest := sha256.Sum256(wire)
	result.Revision = hex.EncodeToString(digest[:])
	return result
}

// observeRecoveryDirectory recognizes only the complete generation layouts
// the stores publish: an artifact object or a portable Project directory.
func observeRecoveryDirectory(root *os.Root, role, name, scope string) RecoveryObject {
	result := RecoveryObject{Role: role, Name: name}
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return result
	}
	result.Present, result.Revision = true, "invalid"
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result
	}
	generation, err := existingPrivateChild(root, name)
	if err != nil {
		return result
	}
	defer generation.Close()
	names, err := readDirectoryNamesBounded(generation, 2)
	if err != nil {
		return result
	}
	sort.Strings(names)
	if scope == RecoveryScopeProjects {
		if len(names) != 1 || names[0] != manifestName {
			return result
		}
		wire, err := readPrivateFile(generation, manifestName)
		if err != nil {
			return result
		}
		if _, issues := manifest.Decode(wire); len(issues) != 0 {
			return result
		}
		digest := sha256.Sum256(wire)
		result.Revision = hex.EncodeToString(digest[:])
		return result
	}
	if len(names) != 2 || names[0] != "details.md" || names[1] != "metadata.json" {
		return result
	}
	metadata, err := readPrivateFileBounded(generation, "metadata.json", detailartifact.MaxMetadataBytes)
	if err != nil {
		return result
	}
	markdown, err := readPrivateFileBounded(generation, "details.md", detailartifact.MaxContentBytes)
	if err != nil {
		return result
	}
	if _, err := detailartifact.Decode(metadata, markdown); err != nil {
		return result
	}
	digest := sha256.Sum256(markdown)
	result.Revision = hex.EncodeToString(digest[:])
	return result
}

func recoveryDigest(plan RecoveryPlan) string {
	plan.Digest = ""
	wire, _ := json.Marshal(plan)
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:])
}

func AuthorizeRecovery(plan RecoveryPlan, reviewedDigest string) (RecoveryAuthority, error) {
	if plan.Action == PreservedReview || plan.Digest == "" || reviewedDigest != plan.Digest {
		return RecoveryAuthority{}, errors.New("recovery authority denied")
	}
	return RecoveryAuthority{digest: plan.Digest}, nil
}

// ApplyRecovery re-inspects the exact directory under exclusive locks and
// acts only when the fresh plan is identical to the authorized plan. A
// confirmed committed generation is never rolled back.
func ApplyRecovery(ctx context.Context, roots RecoveryRoots, plan RecoveryPlan, authority RecoveryAuthority) (RecoveryResult, error) {
	if authority.digest == "" || authority.digest != plan.Digest || plan.Action == PreservedReview {
		return RecoveryResult{}, ErrConflict
	}
	path := roots.State
	if plan.Scope == RecoveryScopeProjects {
		path = roots.Projects
	} else if plan.Scope != RecoveryScopeState {
		return RecoveryResult{}, ErrUnsafe
	}
	if !filepath.IsAbs(path) || !validRecoveryDirectory(plan.Scope, plan.Directory) {
		return RecoveryResult{}, ErrUnsafe
	}
	root, err := existingPrivateRoot(filepath.Clean(path))
	if err != nil {
		return RecoveryResult{}, err
	}
	chain := []*os.Root{root}
	defer func() {
		for index := len(chain) - 1; index >= 0; index-- {
			chain[index].Close()
		}
	}()
	for _, part := range splitDirectory(plan.Directory) {
		next, err := existingPrivateChild(chain[len(chain)-1], part)
		if err != nil {
			return RecoveryResult{}, err
		}
		chain = append(chain, next)
	}
	locks, err := lockRoots(true, chain...)
	if err != nil {
		return RecoveryResult{}, err
	}
	defer closeFiles(locks)
	target := chain[len(chain)-1]
	current, found, err := planRecovery(target, plan.Scope, plan.Directory)
	if err != nil {
		return RecoveryResult{}, err
	}
	if !found || current.Digest != plan.Digest || current.Action != plan.Action {
		return RecoveryResult{}, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return RecoveryResult{}, err
	}
	result := RecoveryResult{Action: plan.Action, Removed: []string{}}
	if plan.Action == RestorePrior {
		stage := recoveryRole(current.Objects, "stage")
		if stage != "" {
			if err := removeRecoveryGeneration(target, stage, plan.Protocol == protocolDirectory); err != nil {
				return result, ErrRecoveryRequired
			}
			result.Removed = append(result.Removed, stage)
		}
	}
	for _, object := range current.Objects {
		if object.Role != "marker_update" {
			continue
		}
		if err := target.Remove(object.Name); err != nil {
			return result, ErrRecoveryRequired
		}
		result.Removed = append(result.Removed, object.Name)
	}
	if err := syncRoot(target); err != nil {
		return result, ErrRecoveryRequired
	}
	if err := removeProtocolState(target, plan.Marker, publicationHooks{}); err != nil {
		return result, ErrRecoveryRequired
	}
	result.Removed = append(result.Removed, plan.Marker)
	return result, nil
}

func removeRecoveryGeneration(root *os.Root, name string, directory bool) error {
	if !directory {
		return root.Remove(name)
	}
	generation, err := existingPrivateChild(root, name)
	if err != nil {
		return err
	}
	names, err := readDirectoryNamesBounded(generation, 2)
	if err != nil {
		generation.Close()
		return err
	}
	for _, file := range names {
		if file != "metadata.json" && file != "details.md" && file != manifestName {
			generation.Close()
			return ErrUnsafe
		}
		if err := generation.Remove(file); err != nil {
			generation.Close()
			return err
		}
	}
	if err := generation.Close(); err != nil {
		return err
	}
	return root.Remove(name)
}

func recoveryRole(objects []RecoveryObject, role string) string {
	for _, object := range objects {
		if object.Role == role && object.Present {
			return object.Name
		}
	}
	return ""
}
