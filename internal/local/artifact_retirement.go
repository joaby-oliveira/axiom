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
	"time"

	"github.com/rgomids/axiom/internal/detailartifact"
)

// retirementRecord is format 1 of artifacts/v1/retirements/<id>.json. It is
// separate from the closed metadata.json v1 schema.
type retirementRecord struct {
	FormatVersion    int    `json:"formatVersion"`
	ArtifactID       string `json:"artifactId"`
	ArtifactRevision string `json:"artifactRevision"`
	ArtifactDigest   string `json:"sha256"`
	RetiredAt        string `json:"retiredAt"`
	PreviewDigest    string `json:"previewDigest"`
	References       int    `json:"observedReferences"`
}

type RetirementResult struct {
	RecordID  string
	Revision  string
	RetiredAt time.Time
}

// PreviewRetirement is read-only. It observes the exact artifact revision and
// every authoritative reference while the state-root lock excludes writers.
func (s ArtifactStore) PreviewRetirement(ctx context.Context, now time.Time, id string) (detailartifact.RetirementPreview, error) {
	if err := ctx.Err(); err != nil {
		return detailartifact.RetirementPreview{}, err
	}
	if !detailartifact.ValidID(id) {
		return detailartifact.RetirementPreview{}, ErrUnsafe
	}
	root, objects, err := s.openObjects(false)
	if err != nil {
		return detailartifact.RetirementPreview{}, err
	}
	defer root.Close()
	defer objects.Close()
	retirements, err := openRetirementDirectory(root, false)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return detailartifact.RetirementPreview{}, err
	}
	roots := []*os.Root{root, objects}
	if retirements != nil {
		defer retirements.Close()
		roots = append(roots, retirements)
	}
	locks, err := lockRoots(false, roots...)
	if err != nil {
		return detailartifact.RetirementPreview{}, err
	}
	defer closeFiles(locks)
	return planRetirementLocked(now, id, root, objects, retirements)
}

func planRetirementLocked(now time.Time, id string, root, objects, retirements *os.Root) (detailartifact.RetirementPreview, error) {
	artifacts, revisions, err := inspectArtifacts(objects)
	if err != nil {
		return detailartifact.RetirementPreview{}, err
	}
	var target *detailartifact.Artifact
	for index := range artifacts {
		if artifacts[index].ID == id {
			target = &artifacts[index]
		}
	}
	if target == nil {
		return detailartifact.RetirementPreview{}, ErrNotFound
	}
	references, err := scanArtifactReferences(root, artifacts)
	if err != nil {
		return detailartifact.RetirementPreview{}, err
	}
	observed := append([]string(nil), references[id]...)
	for _, artifact := range artifacts {
		for _, reference := range artifact.References {
			if reference.Kind == "artifact" && reference.Value == id {
				observed = append(observed, "artifact:"+artifact.ID)
			}
		}
		if artifact.SupersededBy == id {
			observed = append(observed, "superseded-by:"+artifact.ID)
		}
	}
	var existing *detailartifact.RetirementObservation
	if retirements != nil {
		current, err := inspectRetirements(retirements)
		if err != nil {
			return detailartifact.RetirementPreview{}, err
		}
		if observation, ok := current[id]; ok {
			existing = &observation
		}
	}
	return detailartifact.PlanRetirement(now, *target, revisions[id], observed, existing)
}

// ApplyRetirement revalidates the exact preview under exclusive locks and
// publishes the retirement record through the create-only file protocol. The
// retirement moment is the publication clock, never artifact creation time.
func (s ArtifactStore) ApplyRetirement(ctx context.Context, preview detailartifact.RetirementPreview, authority detailartifact.RetirementAuthority) (RetirementResult, error) {
	if !detailartifact.RetirementAuthorized(preview, authority) {
		return RetirementResult{}, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return RetirementResult{}, err
	}
	root, objects, err := s.openObjects(false)
	if err != nil {
		return RetirementResult{}, err
	}
	defer root.Close()
	defer objects.Close()
	retirements, err := openRetirementDirectory(root, true)
	if err != nil {
		return RetirementResult{}, err
	}
	defer retirements.Close()
	locks, err := lockRoots(true, root, objects, retirements)
	if err != nil {
		return RetirementResult{}, err
	}
	defer closeFiles(locks)
	current, err := planRetirementLocked(preview.ObservedAt, preview.ArtifactID, root, objects, retirements)
	if err != nil {
		return RetirementResult{}, err
	}
	if current.Digest != preview.Digest || current.Denied != "" {
		return RetirementResult{}, ErrConflict
	}
	at, err := s.now()
	if err != nil {
		return RetirementResult{}, err
	}
	record := retirementRecord{FormatVersion: 1, ArtifactID: preview.ArtifactID, ArtifactRevision: preview.ArtifactRevision, ArtifactDigest: preview.ArtifactDigest, RetiredAt: at.UTC().Format(timeFormat), PreviewDigest: preview.Digest}
	wire, err := encodeRetirementRecord(record)
	if err != nil {
		return RetirementResult{}, err
	}
	name := preview.ArtifactID + ".json"
	if err := publishFile(ctx, retirements, name, nil, wire, true, s.hooks); err != nil {
		return RetirementResult{}, err
	}
	digest := sha256.Sum256(wire)
	return RetirementResult{RecordID: name, Revision: hex.EncodeToString(digest[:]), RetiredAt: at.UTC()}, nil
}

// supersedeRetirements removes the retirement of every artifact the new
// artifact references. It runs under the creator's exclusive locks and before
// publication, so a re-reference can never leave an old retirement clock
// running; an interrupted creation only preserves longer.
func (s ArtifactStore) supersedeRetirements(root *os.Root, artifact detailartifact.Artifact) error {
	targets := map[string]bool{}
	for _, reference := range artifact.References {
		if reference.Kind == "artifact" && detailartifact.ValidID(reference.Value) {
			targets[reference.Value] = true
		}
	}
	if detailartifact.ValidID(artifact.SupersededBy) {
		targets[artifact.SupersededBy] = true
	}
	if len(targets) == 0 {
		return nil
	}
	retirements, err := openRetirementDirectory(root, false)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	defer retirements.Close()
	lock, err := lockDirectory(retirements, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	if pending, err := protocolStatePresent(retirements); err != nil || pending {
		return ErrRecoveryRequired
	}
	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	removed := false
	for _, id := range ids {
		if _, err := retirements.Lstat(id + ".json"); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := s.hooks.removeName(retirements, id+".json"); err != nil {
			return err
		}
		removed = true
	}
	if removed {
		return s.hooks.syncRoot(retirements)
	}
	return nil
}

func inspectRetirements(retirements *os.Root) (map[string]detailartifact.RetirementObservation, error) {
	if pending, err := protocolStatePresent(retirements); err != nil || pending {
		return nil, ErrRecoveryRequired
	}
	names, err := readDirectoryNamesBounded(retirements, detailartifact.MaxLiveArtifacts)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	observations := make(map[string]detailartifact.RetirementObservation, len(names))
	for _, name := range names {
		if !detailartifact.ValidRetirementRecordID(name) {
			return nil, ErrRecoveryRequired
		}
		id := name[:len(name)-len(".json")]
		observation := detailartifact.RetirementObservation{ArtifactID: id}
		wire, err := readPrivateFileBounded(retirements, name, detailartifact.MaxRetirementRecord)
		if err == nil {
			digest := sha256.Sum256(wire)
			observation.Revision = hex.EncodeToString(digest[:])
			if record, err := decodeRetirementRecord(wire); err == nil && record.ArtifactID == id {
				retiredAt, _ := time.Parse(timeFormat, record.RetiredAt)
				observation.Retirement = detailartifact.Retirement{ArtifactID: record.ArtifactID, ArtifactRevision: record.ArtifactRevision, ArtifactDigest: record.ArtifactDigest, RetiredAt: retiredAt, PreviewDigest: record.PreviewDigest}
				observation.Valid = true
			}
		}
		observations[id] = observation
	}
	return observations, nil
}

func openRetirementDirectory(root *os.Root, create bool) (*os.Root, error) {
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
		return privateChild(v1, "retirements")
	}
	return existingPrivateChild(v1, "retirements")
}

// removeRetirementExact removes the retirement an Evidence removal depends on,
// only when its bytes still match the reviewed revision.
func removeRetirementExact(retirements *os.Root, id, revision string) error {
	if retirements == nil || revision == "" {
		return ErrConflict
	}
	wire, err := readPrivateFileBounded(retirements, id+".json", detailartifact.MaxRetirementRecord)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(wire)
	if hex.EncodeToString(digest[:]) != revision {
		return ErrConflict
	}
	if err := retirements.Remove(id + ".json"); err != nil {
		return err
	}
	return syncRoot(retirements)
}

func encodeRetirementRecord(record retirementRecord) ([]byte, error) {
	if _, err := validRetirementRecord(record); err != nil {
		return nil, err
	}
	wire, err := json.Marshal(record)
	if err != nil || len(wire)+1 > detailartifact.MaxRetirementRecord {
		return nil, ErrUnsafe
	}
	return append(wire, '\n'), nil
}

func decodeRetirementRecord(wire []byte) (retirementRecord, error) {
	if len(wire) == 0 || len(wire) > detailartifact.MaxRetirementRecord {
		return retirementRecord{}, ErrUnsafe
	}
	if _, issues := parseRecord(wire); len(issues) > 0 {
		return retirementRecord{}, ErrUnsafe
	}
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(wire), detailartifact.MaxRetirementRecord+1))
	decoder.DisallowUnknownFields()
	var record retirementRecord
	if err := decoder.Decode(&record); err != nil {
		return retirementRecord{}, ErrUnsafe
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return retirementRecord{}, ErrUnsafe
	}
	record, err := validRetirementRecord(record)
	if err != nil {
		return retirementRecord{}, err
	}
	// Only the canonical encoding is authoritative; any other spelling of the
	// same fields is treated as corrupt.
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(append(canonical, '\n'), wire) {
		return retirementRecord{}, ErrUnsafe
	}
	return record, nil
}

func validRetirementRecord(record retirementRecord) (retirementRecord, error) {
	retiredAt, err := time.Parse(timeFormat, record.RetiredAt)
	if err != nil || retiredAt.IsZero() || record.FormatVersion != 1 || !detailartifact.ValidID(record.ArtifactID) || record.References != 0 {
		return retirementRecord{}, ErrUnsafe
	}
	for _, value := range []string{record.ArtifactRevision, record.ArtifactDigest, record.PreviewDigest} {
		if len(value) != 64 || !lowerHex(value) {
			return retirementRecord{}, ErrUnsafe
		}
	}
	return record, nil
}
