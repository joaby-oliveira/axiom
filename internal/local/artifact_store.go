package local

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rgomids/axiom/internal/detailartifact"
)

type ArtifactStore struct {
	root     string
	now      func() (time.Time, error)
	allocate func() (string, error)
	capacity func(*os.Root) (int, int64, error)
	hooks    publicationHooks
	// beforeCleanupRemoval is a deterministic test seam between exact removals.
	beforeCleanupRemoval func(int) error
}

func NewArtifactStore(root string) (ArtifactStore, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return ArtifactStore{}, ErrUnsafe
	}
	canonical, err := trustedCanonical(root)
	if err != nil {
		return ArtifactStore{}, err
	}
	return ArtifactStore{root: canonical, now: func() (time.Time, error) { return time.Now().UTC(), nil }, allocate: artifactID, capacity: artifactCapacity}, nil
}

func (s ArtifactStore) Create(ctx context.Context, draft detailartifact.Draft) (detailartifact.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return detailartifact.Artifact{}, err
	}
	id, err := s.allocate()
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	at, err := s.now()
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	artifact, err := detailartifact.New(id, at, draft)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	metadata, err := detailartifact.EncodeMetadata(artifact)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	if err := s.create(ctx, artifact, metadata); err != nil {
		return detailartifact.Artifact{}, err
	}
	return artifact, nil
}

func (s ArtifactStore) Read(ctx context.Context, id string) (detailartifact.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return detailartifact.Artifact{}, err
	}
	if !detailartifact.ValidID(id) {
		return detailartifact.Artifact{}, ErrUnsafe
	}
	root, objects, err := s.openObjects(false)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	defer root.Close()
	defer objects.Close()
	rootLock, err := lockDirectory(root, false)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	defer rootLock.Close()
	objectsLock, err := lockDirectory(objects, false)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	defer objectsLock.Close()
	shard, err := existingPrivateChild(objects, id[:2])
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	defer shard.Close()
	if pending, err := protocolStatePresent(shard); err != nil || pending {
		if err != nil {
			return detailartifact.Artifact{}, err
		}
		return detailartifact.Artifact{}, ErrRecoveryRequired
	}
	object, err := existingPrivateChild(shard, id)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	defer object.Close()
	return readArtifactObject(object, id)
}

func (s ArtifactStore) create(ctx context.Context, artifact detailartifact.Artifact, metadata []byte) error {
	root, objects, err := s.openObjects(true)
	if err != nil {
		return err
	}
	defer root.Close()
	defer objects.Close()
	rootLock, err := lockDirectory(root, true)
	if err != nil {
		return err
	}
	defer rootLock.Close()
	objectsLock, err := lockDirectory(objects, true)
	if err != nil {
		return err
	}
	defer objectsLock.Close()
	if err := s.hooks.at(FaultF0); err != nil {
		return publicationFailure(FaultF0, false, err)
	}
	count, bytes, err := s.capacity(objects)
	if err != nil {
		return err
	}
	if count >= detailartifact.MaxLiveArtifacts || bytes+int64(artifact.ContentBytes) > detailartifact.MaxAggregateBytes {
		return ErrCapacity
	}
	shard, err := privateChild(objects, artifact.ID[:2])
	if err != nil {
		return err
	}
	defer shard.Close()
	if pending, err := protocolStatePresent(shard); err != nil || pending {
		if err != nil {
			return err
		}
		return ErrRecoveryRequired
	}
	if err := s.supersedeRetirements(root, artifact); err != nil {
		return err
	}
	if _, err := shard.Lstat(artifact.ID); err == nil {
		return ErrConflict
	} else if !os.IsNotExist(err) {
		return err
	}
	stageName, err := temporaryName(".axiom-stage-artifact-")
	if err != nil {
		return err
	}
	stage, err := privateChild(shard, stageName)
	if err != nil {
		return err
	}
	stageOpen := true
	defer func() {
		if stageOpen {
			stage.Close()
		}
	}()
	cleanupStage := func() error {
		var cleanupErr error
		for _, name := range []string{"metadata.json", "details.md"} {
			if err := stage.Remove(name); err != nil && !os.IsNotExist(err) {
				cleanupErr = errors.Join(cleanupErr, err)
			}
		}
		if err := stage.Close(); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		stageOpen = false
		if err := shard.Remove(stageName); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		return cleanupErr
	}
	if err := s.hooks.at(FaultF1); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF1, err, cleanupStage())
		}
		return publicationFailure(FaultF1, false, err)
	}
	if err := writePrivateFile(stage, "metadata.json", metadata); err != nil {
		return cleanupFailure(FaultF1, err, cleanupStage())
	}
	if err := writePrivateFile(stage, "details.md", artifact.Markdown); err != nil {
		return cleanupFailure(FaultF1, err, cleanupStage())
	}
	if err := syncRoot(stage); err != nil {
		return cleanupFailure(FaultF1, err, cleanupStage())
	}
	if err := s.hooks.at(FaultF2); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF2, err, cleanupStage())
		}
		return publicationFailure(FaultF2, false, err)
	}
	if _, err := readArtifactObject(stage, artifact.ID); err != nil {
		return cleanupFailure(FaultF2, err, cleanupStage())
	}
	markerName, err := writeProtocolMarker(shard, artifact.ID, stageName, false, [32]byte{}, artifact.Digest, s.hooks)
	if err != nil {
		return publicationFailure(FaultF3, false, err)
	}
	marker, err := readProtocolMarker(shard, markerName)
	if err != nil {
		return publicationFailure(FaultF3, false, err)
	}
	cleanup := func() error { return cleanupPreCommit(shard, markerName, cleanupStage, s.hooks) }
	if err := s.hooks.at(FaultF3); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF3, err, cleanup())
		}
		return publicationFailure(FaultF3, false, err)
	}
	if err := ctx.Err(); err != nil {
		return cleanupFailure(FaultF3, err, cleanup())
	}
	if err := s.hooks.at(FaultF4); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF4, err, cleanup())
		}
		return publicationFailure(FaultF4, false, err)
	}
	if err := updateProtocolStage(shard, markerName, marker, FaultF5, s.hooks); err != nil {
		return publicationFailure(FaultF5, false, ErrRecoveryRequired)
	}
	if err := s.hooks.at(FaultF5); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF5, err, cleanup())
		}
		return publicationFailure(FaultF5, false, err)
	}
	stage.Close()
	stageOpen = false
	if err := renameNoReplace(shard, stageName, artifact.ID); err != nil {
		return cleanupFailure(FaultF5, err, cleanup())
	}
	if err := updateProtocolStage(shard, markerName, marker, FaultF6, s.hooks); err != nil {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	if err := s.hooks.at(FaultF6); err != nil {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	canonical, err := existingPrivateChild(shard, artifact.ID)
	if err != nil {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	confirmed, readErr := readArtifactObject(canonical, artifact.ID)
	canonical.Close()
	if readErr != nil || confirmed.Digest != artifact.Digest {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	if err := syncRoot(shard); err != nil {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	if err := updateProtocolStage(shard, markerName, marker, FaultF7, s.hooks); err != nil {
		return publicationFailure(FaultF7, true, ErrRecoveryRequired)
	}
	if err := s.hooks.at(FaultF7); err != nil {
		return publicationFailure(FaultF7, true, err)
	}
	if err := updateProtocolStage(shard, markerName, marker, FaultF8, s.hooks); err != nil {
		return publicationFailure(FaultF8, true, ErrRecoveryRequired)
	}
	if err := s.hooks.at(FaultF8); err != nil {
		return publicationFailure(FaultF8, true, ErrRecoveryRequired)
	}
	if err := removeProtocolState(shard, markerName, s.hooks); err != nil {
		return publicationFailure(FaultF8, true, ErrRecoveryRequired)
	}
	return nil
}

func (s ArtifactStore) openObjects(create bool) (*os.Root, *os.Root, error) {
	openRoot := existingPrivateRoot
	if create {
		openRoot = privateRoot
	}
	root, err := openRoot(s.root)
	if err != nil {
		return nil, nil, err
	}
	openChild := existingPrivateChild
	if create {
		openChild = privateChild
	}
	artifacts, err := openChild(root, "artifacts")
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	v1, err := openChild(artifacts, "v1")
	artifacts.Close()
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	objects, err := openChild(v1, "objects")
	v1.Close()
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	return root, objects, nil
}

func readArtifactObject(root *os.Root, expectedID string) (detailartifact.Artifact, error) {
	names, err := readDirectoryNamesBounded(root, 2)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "details.md" || names[1] != "metadata.json" {
		return detailartifact.Artifact{}, ErrUnsafe
	}
	metadata, err := readPrivateFileBounded(root, "metadata.json", detailartifact.MaxMetadataBytes)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	markdown, err := readPrivateFileBounded(root, "details.md", detailartifact.MaxContentBytes)
	if err != nil {
		return detailartifact.Artifact{}, err
	}
	artifact, err := detailartifact.Decode(metadata, markdown)
	if err != nil || artifact.ID != expectedID {
		return detailartifact.Artifact{}, ErrUnsafe
	}
	return artifact, nil
}

func artifactCapacity(objects *os.Root) (int, int64, error) {
	shards, err := readDirectoryNamesBounded(objects, 256)
	if err != nil {
		return 0, 0, ErrRecoveryRequired
	}
	count := 0
	var total int64
	for _, name := range shards {
		if len(name) != 2 || !lowerHex(name) {
			return 0, 0, ErrRecoveryRequired
		}
		shard, err := existingPrivateChild(objects, name)
		if err != nil {
			return 0, 0, err
		}
		if pending, err := protocolStatePresent(shard); err != nil || pending {
			shard.Close()
			return 0, 0, ErrRecoveryRequired
		}
		ids, err := readDirectoryNamesBounded(shard, detailartifact.MaxLiveArtifacts-count)
		if err != nil {
			shard.Close()
			return 0, 0, ErrRecoveryRequired
		}
		for _, id := range ids {
			if !detailartifact.ValidID(id) || id[:2] != name {
				shard.Close()
				return 0, 0, ErrRecoveryRequired
			}
			object, err := existingPrivateChild(shard, id)
			if err != nil {
				shard.Close()
				return 0, 0, err
			}
			artifact, err := readArtifactObject(object, id)
			object.Close()
			if err != nil {
				shard.Close()
				return 0, 0, ErrRecoveryRequired
			}
			count++
			total += int64(artifact.ContentBytes)
		}
		shard.Close()
	}
	return count, total, nil
}

func lowerHex(value string) bool {
	for _, current := range value {
		if current < '0' || current > '9' && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}

func artifactID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
