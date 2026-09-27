package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/rgomids/axiom/internal/coordination"
)

type CoordinationStore struct {
	root, projectID string
	hooks           publicationHooks
}

func NewCoordinationStore(root, projectID string) (CoordinationStore, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) || !validGraphToken(projectID) {
		return CoordinationStore{}, ErrUnsafe
	}
	canonical, err := trustedCanonical(root)
	if err != nil {
		return CoordinationStore{}, err
	}
	return CoordinationStore{root: canonical, projectID: projectID}, nil
}

func (s CoordinationStore) Latest(ctx context.Context, parentID, childID string) (coordination.Record, bool, error) {
	if err := ctx.Err(); err != nil {
		return coordination.Record{}, false, err
	}
	root, streams, version, project, err := s.openProject(false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return coordination.Record{}, false, nil
		}
		return coordination.Record{}, false, err
	}
	defer root.Close()
	defer streams.Close()
	defer version.Close()
	defer project.Close()
	locks, err := lockRoots(false, root, streams, version, project)
	if err != nil {
		return coordination.Record{}, false, err
	}
	defer closeFiles(locks)
	wire, err := readPublishedFile(project, coordinationName(parentID, childID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) {
			return coordination.Record{}, false, nil
		}
		return coordination.Record{}, false, err
	}
	stream, err := coordination.DecodeStream(wire)
	if err != nil || stream.ParentID != parentID || stream.ChildID != childID {
		return coordination.Record{}, false, ErrUnsafe
	}
	return stream.Records[len(stream.Records)-1], true, nil
}

func (s CoordinationStore) Publish(ctx context.Context, record coordination.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !coordination.ValidRecord(record) {
		return coordination.ErrInvalidRecord
	}
	root, streams, version, project, err := s.openProject(true)
	if err != nil {
		return err
	}
	defer root.Close()
	defer streams.Close()
	defer version.Close()
	defer project.Close()
	locks, err := lockRoots(true, root, streams, version, project)
	if err != nil {
		return err
	}
	defer closeFiles(locks)
	name := coordinationName(record.ParentID, record.ChildID)
	expected, readErr := readPublishedFile(project, name)
	create := errors.Is(readErr, os.ErrNotExist) || errors.Is(readErr, ErrNotFound)
	stream := coordination.Stream{FormatVersion: coordination.FormatVersion, ParentID: record.ParentID, ChildID: record.ChildID, Records: []coordination.Record{record}}
	if !create {
		if readErr != nil {
			return readErr
		}
		stream, err = coordination.DecodeStream(expected)
		if err != nil {
			return ErrUnsafe
		}
		latest := stream.Records[len(stream.Records)-1]
		if record.Revision != latest.Revision+1 || record.PreviousDigest != latest.Digest {
			return coordination.ErrStaleRevision
		}
		stream.Records = append(stream.Records, record)
	}
	wire, err := coordination.EncodeStream(stream)
	if err != nil {
		return err
	}
	return publishFile(ctx, project, name, expected, wire, create, s.hooks)
}

func (s CoordinationStore) openProject(create bool) (*os.Root, *os.Root, *os.Root, *os.Root, error) {
	openRoot := existingPrivateRoot
	openChild := existingPrivateChild
	if create {
		openRoot = privateRoot
		openChild = privateChild
	}
	root, err := openRoot(s.root)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	streams, err := openChild(root, "coordination")
	if err != nil {
		root.Close()
		return nil, nil, nil, nil, err
	}
	version, err := openChild(streams, "v1")
	if err != nil {
		streams.Close()
		root.Close()
		return nil, nil, nil, nil, err
	}
	project, err := openChild(version, s.projectID)
	if err != nil {
		version.Close()
		streams.Close()
		root.Close()
		return nil, nil, nil, nil, err
	}
	return root, streams, version, project, nil
}

func coordinationName(parentID, childID string) string {
	digest := sha256.Sum256([]byte(parentID + "\x00" + childID))
	return hex.EncodeToString(digest[:]) + ".json"
}
