package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/workflow"
)

type CleanupResult struct {
	RecordID       string
	Removed        []detailartifact.CleanupEffect
	Preserved      []string
	ReclaimedBytes int64
	Partial        bool
	AuditRecorded  bool
}

type cleanupRecord struct {
	FormatVersion    int                            `json:"formatVersion"`
	RecordID         string                         `json:"recordId"`
	Status           string                         `json:"status"`
	PreviewDigest    string                         `json:"previewDigest"`
	ObservedAt       string                         `json:"observedAt"`
	CompletedAt      string                         `json:"completedAt,omitempty"`
	Planned          int                            `json:"planned"`
	Removed          []detailartifact.CleanupEffect `json:"removed"`
	PreservedSummary map[string]int                 `json:"preservedSummary"`
	ReclaimedBytes   int64                          `json:"reclaimedBytes"`
}

// PreviewCleanup is read-only. Eligibility uses authoritative Execution
// references observed while the state-root lock excludes every writer.
func (s ArtifactStore) PreviewCleanup(ctx context.Context, now time.Time) (detailartifact.CleanupPreview, error) {
	if err := ctx.Err(); err != nil {
		return detailartifact.CleanupPreview{}, err
	}
	root, objects, err := s.openObjects(false)
	if errors.Is(err, ErrNotFound) {
		return detailartifact.PlanCleanup(now, nil, nil, nil, nil, nil)
	}
	if err != nil {
		return detailartifact.CleanupPreview{}, err
	}
	defer root.Close()
	defer objects.Close()
	cleanup, err := openCleanupDirectory(root, false)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return detailartifact.CleanupPreview{}, err
	}
	retirements, err := openRetirementDirectory(root, false)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return detailartifact.CleanupPreview{}, err
	}
	roots := []*os.Root{root, objects}
	if cleanup != nil {
		defer cleanup.Close()
		roots = append(roots, cleanup)
	}
	if retirements != nil {
		defer retirements.Close()
		roots = append(roots, retirements)
	}
	locks, err := lockRoots(false, roots...)
	if err != nil {
		return detailartifact.CleanupPreview{}, err
	}
	defer closeFiles(locks)
	return planCleanupLocked(now, root, objects, cleanup, retirements)
}

func planCleanupLocked(now time.Time, root, objects, cleanup, retirements *os.Root) (detailartifact.CleanupPreview, error) {
	artifacts, revisions, err := inspectArtifacts(objects)
	if err != nil {
		return detailartifact.CleanupPreview{}, err
	}
	references, err := scanArtifactReferences(root, artifacts)
	if err != nil {
		return detailartifact.CleanupPreview{}, err
	}
	var records []detailartifact.RecordObservation
	if cleanup != nil {
		records, err = inspectCleanupRecords(cleanup)
		if err != nil {
			return detailartifact.CleanupPreview{}, err
		}
	}
	var retired map[string]detailartifact.RetirementObservation
	if retirements != nil {
		retired, err = inspectRetirements(retirements)
		if err != nil {
			return detailartifact.CleanupPreview{}, err
		}
	}
	return detailartifact.PlanCleanup(now, artifacts, references, revisions, records, retired)
}

// ApplyCleanup revalidates the exact preview under exclusive locks, records a
// bounded audit record before any removal, and reports partial truthfully.
func (s ArtifactStore) ApplyCleanup(ctx context.Context, preview detailartifact.CleanupPreview, authority detailartifact.CleanupAuthority) (CleanupResult, error) {
	if !detailartifact.CleanupAuthorized(preview, authority) || len(preview.Effects) == 0 {
		return CleanupResult{}, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return CleanupResult{}, err
	}
	root, objects, err := s.openObjects(false)
	if err != nil {
		return CleanupResult{}, err
	}
	defer root.Close()
	defer objects.Close()
	cleanup, err := openCleanupDirectory(root, true)
	if err != nil {
		return CleanupResult{}, err
	}
	defer cleanup.Close()
	retirements, err := openRetirementDirectory(root, false)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return CleanupResult{}, err
	}
	roots := []*os.Root{root, objects, cleanup}
	if retirements != nil {
		defer retirements.Close()
		roots = append(roots, retirements)
	}
	locks, err := lockRoots(true, roots...)
	if err != nil {
		return CleanupResult{}, err
	}
	defer closeFiles(locks)
	current, err := planCleanupLocked(preview.ObservedAt, root, objects, cleanup, retirements)
	if err != nil {
		return CleanupResult{}, err
	}
	if current.Digest != preview.Digest {
		return CleanupResult{}, ErrConflict
	}
	recordID := "cleanup-" + preview.Digest[:32] + ".json"
	record := cleanupRecord{FormatVersion: 1, RecordID: recordID, Status: "started", PreviewDigest: preview.Digest, ObservedAt: preview.ObservedAt.UTC().Format(timeFormat), Planned: len(preview.Effects), Removed: []detailartifact.CleanupEffect{}, PreservedSummary: preview.PreservedSummary}
	started, err := encodeCleanupRecord(record)
	if err != nil {
		return CleanupResult{}, err
	}
	// The audit record is published before removal; failure here is pre-effect.
	if err := publishFile(ctx, cleanup, recordID, nil, started, true, s.hooks); err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{RecordID: recordID, Preserved: current.Preserved()}
	var removalErr error
	for index, effect := range preview.Effects {
		if err := ctx.Err(); err != nil {
			removalErr = err
			break
		}
		if s.beforeCleanupRemoval != nil {
			if err := s.beforeCleanupRemoval(index); err != nil {
				removalErr = err
				break
			}
		}
		if err := removeCleanupEffect(objects, cleanup, retirements, effect); err != nil {
			result.Preserved = append(result.Preserved, effect.ID+":revalidation_failed")
			removalErr = err
			break
		}
		result.Removed = append(result.Removed, effect)
		result.ReclaimedBytes += effect.Bytes
	}
	status := "confirmed"
	if removalErr != nil {
		status = "partial"
		result.Partial = true
	}
	completed, clockErr := s.now()
	if clockErr != nil {
		completed = preview.ObservedAt
	}
	record.Status, record.CompletedAt = status, completed.UTC().Format(timeFormat)
	record.Removed, record.ReclaimedBytes = append([]detailartifact.CleanupEffect{}, result.Removed...), result.ReclaimedBytes
	final, err := encodeCleanupRecord(record)
	if err == nil {
		err = publishFile(ctx, cleanup, recordID, started, final, false, s.hooks)
	}
	if err != nil {
		// Removals already confirmed stay confirmed; the audit record keeps
		// its last truthful state and the result is partial, never success.
		result.Partial = true
		return result, errors.Join(removalErr, err)
	}
	result.AuditRecorded = true
	return result, removalErr
}

// scanArtifactReferences reads every Execution record while the caller holds
// the state-root lock. Any unreadable or pending Execution makes reference
// state uncertain, so cleanup fails closed instead of guessing.
func scanArtifactReferences(root *os.Root, artifacts []detailartifact.Artifact) (map[string][]string, error) {
	references := map[string][]string{}
	executions, err := existingPrivateChild(root, "executions")
	if errors.Is(err, ErrNotFound) {
		return references, nil
	}
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	defer executions.Close()
	versions, err := readDirectoryNamesBounded(executions, maxLocalDirectoryEntries)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	for _, name := range versions {
		if name != "v1" {
			return nil, ErrRecoveryRequired
		}
	}
	if len(versions) == 0 {
		return references, nil
	}
	version, err := existingPrivateChild(executions, "v1")
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	defer version.Close()
	projects, err := readDirectoryNamesBounded(version, maxLocalDirectoryEntries)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	sort.Strings(projects)
	open := map[string]bool{}
	for _, projectID := range projects {
		if !validUUID(projectID) {
			return nil, ErrRecoveryRequired
		}
		projectRoot, err := existingPrivateChild(version, projectID)
		if err != nil {
			return nil, ErrRecoveryRequired
		}
		names, err := readDirectoryNamesBounded(projectRoot, maxLocalDirectoryEntries)
		if err != nil {
			projectRoot.Close()
			return nil, ErrRecoveryRequired
		}
		sort.Strings(names)
		for _, name := range names {
			if !hexDigestName.MatchString(name) {
				projectRoot.Close()
				return nil, ErrRecoveryRequired
			}
			wire, err := readPrivateFile(projectRoot, name)
			if err != nil {
				projectRoot.Close()
				return nil, ErrRecoveryRequired
			}
			state, err := decodeExecution(wire)
			if err != nil {
				projectRoot.Close()
				return nil, ErrRecoveryRequired
			}
			if state.Status != workflow.ExecutionCompleted {
				open[state.ExecutionID] = true
			}
			for _, transition := range state.Transitions {
				for _, reference := range transition.References {
					if reference.Kind == "artifact" {
						references[reference.ID] = append(references[reference.ID], "execution:"+state.ExecutionID)
					}
				}
			}
		}
		projectRoot.Close()
	}
	for _, artifact := range artifacts {
		if open[artifact.ExecutionID] {
			references[artifact.ID] = append(references[artifact.ID], "open-execution:"+artifact.ExecutionID)
		}
	}
	return references, nil
}

func inspectArtifacts(objects *os.Root) ([]detailartifact.Artifact, map[string]string, error) {
	shards, err := readDirectoryNamesBounded(objects, 256)
	if err != nil {
		return nil, nil, ErrRecoveryRequired
	}
	sort.Strings(shards)
	artifacts := make([]detailartifact.Artifact, 0)
	revisions := map[string]string{}
	for _, shardName := range shards {
		if len(shardName) != 2 || !lowerHex(shardName) {
			return nil, nil, ErrRecoveryRequired
		}
		shard, err := existingPrivateChild(objects, shardName)
		if err != nil {
			return nil, nil, err
		}
		pending, pendingErr := protocolStatePresent(shard)
		if pendingErr != nil || pending {
			shard.Close()
			return nil, nil, ErrRecoveryRequired
		}
		ids, err := readDirectoryNamesBounded(shard, detailartifact.MaxLiveArtifacts-len(artifacts))
		if err != nil {
			shard.Close()
			return nil, nil, ErrRecoveryRequired
		}
		sort.Strings(ids)
		for _, id := range ids {
			if !detailartifact.ValidID(id) || id[:2] != shardName {
				shard.Close()
				return nil, nil, ErrRecoveryRequired
			}
			artifact, revision, err := readArtifactRevision(shard, id)
			if err != nil {
				shard.Close()
				return nil, nil, ErrRecoveryRequired
			}
			revisions[id] = revision
			artifacts = append(artifacts, artifact)
		}
		shard.Close()
	}
	return artifacts, revisions, nil
}

func readArtifactRevision(shard *os.Root, id string) (detailartifact.Artifact, string, error) {
	object, err := existingPrivateChild(shard, id)
	if err != nil {
		return detailartifact.Artifact{}, "", err
	}
	defer object.Close()
	artifact, err := readArtifactObject(object, id)
	if err != nil {
		return detailartifact.Artifact{}, "", err
	}
	metadata, err := readPrivateFileBounded(object, "metadata.json", detailartifact.MaxMetadataBytes)
	if err != nil {
		return detailartifact.Artifact{}, "", err
	}
	revision := sha256.Sum256(append(metadata, artifact.Markdown...))
	return artifact, hex.EncodeToString(revision[:]), nil
}

func inspectCleanupRecords(cleanup *os.Root) ([]detailartifact.RecordObservation, error) {
	if pending, err := protocolStatePresent(cleanup); err != nil || pending {
		return nil, ErrRecoveryRequired
	}
	names, err := readDirectoryNamesBounded(cleanup, maxLocalDirectoryEntries)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	sort.Strings(names)
	records := make([]detailartifact.RecordObservation, 0, len(names))
	for _, name := range names {
		if !detailartifact.ValidCleanupRecordID(name) {
			return nil, ErrRecoveryRequired
		}
		observation := detailartifact.RecordObservation{ID: name}
		wire, err := readPrivateFileBounded(cleanup, name, detailartifact.MaxCleanupRecord)
		if err == nil {
			digest := sha256.Sum256(wire)
			observation.Revision, observation.Bytes = hex.EncodeToString(digest[:]), int64(len(wire))
			if record, err := decodeCleanupRecord(wire); err == nil && record.RecordID == name {
				observation.Status, observation.Valid = record.Status, true
				if record.CompletedAt != "" {
					observation.CompletedAt, err = time.Parse(timeFormat, record.CompletedAt)
					observation.Valid = err == nil
				}
			}
		}
		records = append(records, observation)
	}
	return records, nil
}

func removeCleanupEffect(objects, cleanup, retirements *os.Root, effect detailartifact.CleanupEffect) error {
	switch effect.Kind {
	case detailartifact.EffectArtifact:
		return removeExactArtifact(objects, retirements, effect)
	case detailartifact.EffectCleanupRecord:
		if !detailartifact.ValidCleanupRecordID(effect.ID) {
			return ErrUnsafe
		}
		wire, err := readPrivateFileBounded(cleanup, effect.ID, detailartifact.MaxCleanupRecord)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(wire)
		if hex.EncodeToString(digest[:]) != effect.Revision {
			return ErrConflict
		}
		if err := cleanup.Remove(effect.ID); err != nil {
			return err
		}
		return syncRoot(cleanup)
	default:
		return ErrUnsafe
	}
}

func removeExactArtifact(objects, retirements *os.Root, effect detailartifact.CleanupEffect) error {
	if !detailartifact.ValidID(effect.ID) {
		return ErrUnsafe
	}
	shard, err := existingPrivateChild(objects, effect.ID[:2])
	if err != nil {
		return err
	}
	defer shard.Close()
	if pending, err := protocolStatePresent(shard); err != nil || pending {
		return ErrRecoveryRequired
	}
	artifact, revision, err := readArtifactRevision(shard, effect.ID)
	if err != nil {
		return err
	}
	if revision != effect.Revision || hex.EncodeToString(artifact.Digest[:]) != effect.Digest || int64(artifact.ContentBytes) != effect.Bytes {
		return ErrConflict
	}
	if len(artifact.LiveReferences) != 0 {
		return ErrConflict
	}
	switch artifact.Retention {
	case detailartifact.Diagnostic:
		if effect.Retirement != "" {
			return ErrConflict
		}
	case detailartifact.Evidence:
		// The exact retirement is consumed first: an interruption after this
		// point leaves unretired Evidence, which is always preserved.
		if err := removeRetirementExact(retirements, effect.ID, effect.Retirement); err != nil {
			return err
		}
	default:
		return ErrConflict
	}
	object, err := existingPrivateChild(shard, effect.ID)
	if err != nil {
		return err
	}
	names, err := readDirectoryNamesBounded(object, 2)
	if err != nil {
		object.Close()
		return ErrConflict
	}
	for _, name := range names {
		if name != "metadata.json" && name != "details.md" {
			object.Close()
			return ErrConflict
		}
	}
	for _, name := range []string{"details.md", "metadata.json"} {
		if err := object.Remove(name); err != nil {
			object.Close()
			return err
		}
	}
	if err := object.Close(); err != nil {
		return err
	}
	if err := shard.Remove(effect.ID); err != nil {
		return err
	}
	return syncRoot(shard)
}

func openCleanupDirectory(root *os.Root, create bool) (*os.Root, error) {
	artifacts, err := existingPrivateChild(root, "artifacts")
	if err != nil {
		return nil, err
	}
	v1, err := existingPrivateChild(artifacts, "v1")
	artifacts.Close()
	if err != nil {
		return nil, err
	}
	defer v1.Close()
	if create {
		return privateChild(v1, "cleanup")
	}
	return existingPrivateChild(v1, "cleanup")
}

func encodeCleanupRecord(record cleanupRecord) ([]byte, error) {
	if record.PreservedSummary == nil {
		record.PreservedSummary = map[string]int{}
	}
	wire, err := json.Marshal(record)
	if err != nil || len(wire)+1 > detailartifact.MaxCleanupRecord {
		return nil, ErrUnsafe
	}
	return append(wire, '\n'), nil
}

func decodeCleanupRecord(wire []byte) (cleanupRecord, error) {
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(wire), detailartifact.MaxCleanupRecord+1))
	decoder.DisallowUnknownFields()
	var record cleanupRecord
	if err := decoder.Decode(&record); err != nil {
		return cleanupRecord{}, ErrUnsafe
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return cleanupRecord{}, ErrUnsafe
	}
	if record.FormatVersion != 1 || !detailartifact.ValidCleanupRecordID(record.RecordID) || !strings.HasPrefix(record.RecordID, "cleanup-"+record.PreviewDigest[:min(32, len(record.PreviewDigest))]) {
		return cleanupRecord{}, ErrUnsafe
	}
	switch record.Status {
	case "started":
		if record.CompletedAt != "" {
			return cleanupRecord{}, ErrUnsafe
		}
	case "partial", "confirmed":
		if record.CompletedAt == "" {
			return cleanupRecord{}, ErrUnsafe
		}
	default:
		return cleanupRecord{}, ErrUnsafe
	}
	if _, err := time.Parse(timeFormat, record.ObservedAt); err != nil || record.Planned < 0 || record.Planned > detailartifact.MaxCleanupBatch || len(record.Removed) > record.Planned || record.ReclaimedBytes < 0 {
		return cleanupRecord{}, ErrUnsafe
	}
	return record, nil
}
