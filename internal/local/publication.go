package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type FaultStage string

const (
	FaultF0 FaultStage = "F0"
	FaultF1 FaultStage = "F1"
	FaultF2 FaultStage = "F2"
	FaultF3 FaultStage = "F3"
	FaultF4 FaultStage = "F4"
	FaultF5 FaultStage = "F5"
	FaultF6 FaultStage = "F6"
	FaultF7 FaultStage = "F7"
	FaultF8 FaultStage = "F8"
)

var ErrSimulatedInterruption = errors.New("simulated process interruption")
var ErrCapacity = errors.New("local capacity exhausted")

type PublicationError struct {
	Stage     FaultStage
	Committed bool
	Err       error
}

func (e *PublicationError) Error() string {
	return fmt.Sprintf("publication %s committed=%t: %v", e.Stage, e.Committed, e.Err)
}

func (e *PublicationError) Unwrap() error { return e.Err }

func (e *PublicationError) EffectCommitted() bool { return e.Committed }

type protocolMarker struct {
	FormatVersion  int        `json:"formatVersion"`
	OperationID    string     `json:"operationId"`
	Object         string     `json:"object"`
	Stage          FaultStage `json:"stage"`
	PriorPresent   bool       `json:"priorPresent"`
	PriorRevision  string     `json:"priorRevision"`
	NewPresent     bool       `json:"newPresent"`
	NewRevision    string     `json:"newRevision"`
	Staging        string     `json:"staging"`
	CommitProtocol string     `json:"commitProtocol"`
}

type publicationHooks struct {
	fault        func(FaultStage) error
	remove       func(*os.Root, string) error
	sync         func(*os.Root) error
	write        func(*os.Root, string, []byte) error
	stagePrefix  string
	afterStage   func()
	beforeCommit func() error
	afterCommit  func()
}

func (h publicationHooks) at(stage FaultStage) error {
	if h.fault == nil {
		return nil
	}
	return h.fault(stage)
}

func (h publicationHooks) removeName(root *os.Root, name string) error {
	if h.remove != nil {
		return h.remove(root, name)
	}
	return root.Remove(name)
}

func (h publicationHooks) syncRoot(root *os.Root) error {
	if h.sync != nil {
		return h.sync(root)
	}
	return syncRoot(root)
}

func (h publicationHooks) writeFile(root *os.Root, name string, content []byte) error {
	if h.write != nil {
		return h.write(root, name, content)
	}
	return writePrivateFile(root, name, content)
}

func protocolStatePresent(root *os.Root) (bool, error) {
	return protocolStatePresentBounded(root, maxLocalDirectoryEntries)
}

func protocolStatePresentBounded(root *os.Root, limit int) (bool, error) {
	return directoryPrefixPresentBounded(root, limit, ".axiom-stage-", ".axiom-recovery-")
}

func writeProtocolMarker(root *os.Root, object, staging string, priorPresent bool, prior [32]byte, next [32]byte, hooks publicationHooks) (string, error) {
	operation, err := temporaryName("")
	if err != nil {
		return "", err
	}
	name := ".axiom-recovery-" + operation
	marker := protocolMarker{
		FormatVersion: 1, OperationID: operation, Object: object, Stage: FaultF3,
		PriorPresent: priorPresent, PriorRevision: hex.EncodeToString(prior[:]),
		NewPresent: true, NewRevision: hex.EncodeToString(next[:]), Staging: staging,
		CommitProtocol: "rename",
	}
	wire, err := json.Marshal(marker)
	if err != nil {
		return "", err
	}
	wire = append(wire, '\n')
	if len(wire) > 64<<10 {
		return "", ErrUnsafe
	}
	if err := writePrivateFile(root, name, wire); err != nil {
		return "", err
	}
	if err := hooks.syncRoot(root); err != nil {
		return name, ErrRecoveryRequired
	}
	return name, nil
}

func updateProtocolStage(root *os.Root, name string, marker protocolMarker, stage FaultStage, hooks publicationHooks) error {
	marker.Stage = stage
	wire, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	wire = append(wire, '\n')
	temporary, err := temporaryName(".axiom-stage-marker-")
	if err != nil {
		return err
	}
	defer hooks.removeName(root, temporary)
	if err := writePrivateFile(root, temporary, wire); err != nil {
		return err
	}
	if err := root.Rename(temporary, name); err != nil {
		return err
	}
	return hooks.syncRoot(root)
}

func readProtocolMarker(root *os.Root, name string) (protocolMarker, error) {
	wire, err := readPrivateFileBounded(root, name, 64<<10)
	if err != nil {
		return protocolMarker{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(wire)))
	decoder.DisallowUnknownFields()
	var marker protocolMarker
	if err := decoder.Decode(&marker); err != nil || !validProtocolMarker(marker) {
		return protocolMarker{}, ErrRecoveryRequired
	}
	return marker, nil
}

func validProtocolMarker(marker protocolMarker) bool {
	if marker.FormatVersion != 1 || marker.OperationID == "" || marker.Object == "" || marker.Staging == "" || marker.CommitProtocol != "rename" || !marker.NewPresent {
		return false
	}
	switch marker.Stage {
	case FaultF3, FaultF4, FaultF5, FaultF6, FaultF7, FaultF8:
	default:
		return false
	}
	for _, revision := range []string{marker.PriorRevision, marker.NewRevision} {
		decoded, err := hex.DecodeString(revision)
		if err != nil || len(decoded) != sha256.Size {
			return false
		}
	}
	if !marker.PriorPresent && marker.PriorRevision != strings.Repeat("0", sha256.Size*2) {
		return false
	}
	return true
}

func digestBytes(value []byte) [32]byte { return sha256.Sum256(value) }

func publicationFailure(stage FaultStage, committed bool, err error) error {
	if err == nil {
		return nil
	}
	return &PublicationError{Stage: stage, Committed: committed, Err: err}
}

func cleanupFailure(stage FaultStage, original, cleanup error) error {
	if cleanup == nil {
		return publicationFailure(stage, false, original)
	}
	return publicationFailure(stage, false, errors.Join(ErrRecoveryRequired, original, cleanup))
}

func cleanupPreCommit(root *os.Root, marker string, removeStage func() error, hooks publicationHooks) error {
	if err := removeStage(); err != nil {
		return err
	}
	// Keep the durable marker until staged-object removal is confirmed. A sync
	// failure therefore leaves explicit recovery evidence for the next reader.
	if err := hooks.syncRoot(root); err != nil {
		return err
	}
	return removeProtocolState(root, marker, hooks)
}

func removeProtocolState(root *os.Root, name string, hooks publicationHooks) error {
	wire, err := readPrivateFileBounded(root, name, 64<<10)
	if err != nil {
		return ErrRecoveryRequired
	}
	if err := hooks.removeName(root, name); err != nil {
		return ErrRecoveryRequired
	}
	if err := hooks.syncRoot(root); err == nil {
		return nil
	}
	// Removal is already visible in this namespace even when its directory sync
	// fails. Restore the exact bounded owned marker so subsequent readers remain
	// fail-closed; canonical bytes and the commit classification stay unchanged.
	if _, err := root.Lstat(name); os.IsNotExist(err) {
		_ = writePrivateFile(root, name, wire)
	}
	_ = hooks.syncRoot(root)
	return ErrRecoveryRequired
}

func cleanupStagedFile(root *os.Root, stage string, hooks publicationHooks) error {
	if err := hooks.removeName(root, stage); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return hooks.syncRoot(root)
}

func publishFile(ctx context.Context, root *os.Root, name string, expected, next []byte, create bool, hooks publicationHooks) error {
	if err := hooks.at(FaultF0); err != nil {
		return publicationFailure(FaultF0, false, err)
	}
	if pending, err := protocolStatePresent(root); err != nil || pending {
		if err != nil {
			return err
		}
		return ErrRecoveryRequired
	}
	current, readErr := readPrivateFile(root, name)
	if create {
		if readErr == nil {
			return ErrConflict
		}
		if !os.IsNotExist(readErr) {
			return readErr
		}
	} else {
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return ErrNotFound
			}
			return readErr
		}
		if len(expected) == 0 || !bytes.Equal(current, expected) {
			return ErrConflict
		}
	}
	stagePrefix := hooks.stagePrefix
	if stagePrefix == "" {
		stagePrefix = ".axiom-stage-file-"
	}
	stage, err := temporaryName(stagePrefix)
	if err != nil {
		return err
	}
	removeStage := func() error { return cleanupStagedFile(root, stage, hooks) }
	if err := hooks.at(FaultF1); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			if cleanup := removeStage(); cleanup != nil && !os.IsNotExist(cleanup) {
				return cleanupFailure(FaultF1, err, cleanup)
			}
		}
		return publicationFailure(FaultF1, false, err)
	}
	if err := hooks.writeFile(root, stage, next); err != nil {
		return cleanupFailure(FaultF1, err, removeStage())
	}
	if hooks.afterStage != nil {
		hooks.afterStage()
	}
	if err := hooks.at(FaultF2); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF2, err, removeStage())
		}
		return publicationFailure(FaultF2, false, err)
	}
	if err := verifyPreparedFile(root, stage, next); err != nil {
		return cleanupFailure(FaultF2, err, removeStage())
	}
	priorRevision := [32]byte{}
	if !create {
		priorRevision = digestBytes(current)
	}
	markerName, err := writeProtocolMarker(root, name, stage, !create, priorRevision, digestBytes(next), hooks)
	if err != nil {
		return publicationFailure(FaultF3, false, err)
	}
	marker, err := readProtocolMarker(root, markerName)
	if err != nil {
		return publicationFailure(FaultF3, false, err)
	}
	cleanup := func() error {
		return cleanupPreCommit(root, markerName, func() error { return hooks.removeName(root, stage) }, hooks)
	}
	if err := hooks.at(FaultF3); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF3, err, cleanup())
		}
		return publicationFailure(FaultF3, false, err)
	}
	if err := ctx.Err(); err != nil {
		return cleanupFailure(FaultF3, err, cleanup())
	}
	if hooks.beforeCommit != nil {
		if err := hooks.beforeCommit(); err != nil {
			return cleanupFailure(FaultF3, err, cleanup())
		}
	}
	if !create {
		observed, err := readPrivateFile(root, name)
		if err != nil || !bytes.Equal(observed, expected) {
			return cleanupFailure(FaultF3, ErrConflict, cleanup())
		}
	}
	if err := hooks.at(FaultF4); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF4, err, cleanup())
		}
		return publicationFailure(FaultF4, false, err)
	}
	if err := updateProtocolStage(root, markerName, marker, FaultF5, hooks); err != nil {
		return publicationFailure(FaultF4, false, ErrRecoveryRequired)
	}
	if err := hooks.at(FaultF5); err != nil {
		if !errors.Is(err, ErrSimulatedInterruption) {
			return cleanupFailure(FaultF5, err, cleanup())
		}
		return publicationFailure(FaultF5, false, err)
	}
	if create {
		err = renameNoReplace(root, stage, name)
	} else {
		err = root.Rename(stage, name)
	}
	if err != nil {
		return cleanupFailure(FaultF5, err, cleanup())
	}
	if hooks.afterCommit != nil {
		hooks.afterCommit()
	}
	if err := updateProtocolStage(root, markerName, marker, FaultF6, hooks); err != nil {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	if err := hooks.at(FaultF6); err != nil {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	confirmed, err := readPrivateFile(root, name)
	if err != nil || !bytes.Equal(confirmed, next) {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	if err := hooks.syncRoot(root); err != nil {
		return publicationFailure(FaultF6, true, ErrRecoveryRequired)
	}
	if err := updateProtocolStage(root, markerName, marker, FaultF7, hooks); err != nil {
		return publicationFailure(FaultF7, true, ErrRecoveryRequired)
	}
	if err := hooks.at(FaultF7); err != nil {
		return publicationFailure(FaultF7, true, err)
	}
	if err := updateProtocolStage(root, markerName, marker, FaultF8, hooks); err != nil {
		return publicationFailure(FaultF8, true, ErrRecoveryRequired)
	}
	if err := hooks.at(FaultF8); err != nil {
		return publicationFailure(FaultF8, true, ErrRecoveryRequired)
	}
	if err := removeProtocolState(root, markerName, hooks); err != nil {
		return publicationFailure(FaultF8, true, ErrRecoveryRequired)
	}
	return nil
}

func readPublishedFile(root *os.Root, name string) ([]byte, error) {
	pending, err := protocolStatePresent(root)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, ErrRecoveryRequired
	}
	return readPrivateFile(root, name)
}
