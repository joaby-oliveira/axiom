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
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/manifest"
	"github.com/rgomids/axiom/internal/project"
)

// InventoryKind is the read-only compatibility fact observed for one owned
// entry. It never grants authority to mutate the entry.
type InventoryKind string

const (
	InventoryInstallation     InventoryKind = "installation_record"
	InventoryWorkItem         InventoryKind = "work_item_link"
	InventoryCreateAttempt    InventoryKind = "work_item_create_attempt"
	InventoryExecution        InventoryKind = "execution"
	InventoryArtifact         InventoryKind = "artifact_object"
	InventoryCleanupRecord    InventoryKind = "artifact_cleanup_record"
	InventoryRetirementRecord InventoryKind = "artifact_retirement_record"
	InventoryPortableManifest InventoryKind = "portable_manifest"
	InventoryPOCWorkflow      InventoryKind = "poc_workflow"
	InventoryRecovery         InventoryKind = "recovery_state"
	InventoryNewer            InventoryKind = "unsupported_newer"
	InventoryOlder            InventoryKind = "unsupported_older"
	InventoryMalformed        InventoryKind = "malformed"
	InventoryUnsafe           InventoryKind = "unsafe"
	InventoryUnknown          InventoryKind = "unknown"
)

// V1Only reports kinds that no historical POC revision could have written.
func (k InventoryKind) V1Only() bool {
	return k == InventoryCreateAttempt || k == InventoryExecution || k == InventoryArtifact || k == InventoryCleanupRecord || k == InventoryRetirementRecord
}

// Supported reports kinds a v1 reader accepts or the POC signature recognizes.
func (k InventoryKind) Supported() bool {
	switch k {
	case InventoryInstallation, InventoryWorkItem, InventoryCreateAttempt, InventoryExecution, InventoryArtifact, InventoryCleanupRecord, InventoryRetirementRecord, InventoryPortableManifest, InventoryPOCWorkflow:
		return true
	}
	return false
}

type InventoryEntry struct {
	Relative string        `json:"relative"`
	Kind     InventoryKind `json:"kind"`
	Digest   string        `json:"sha256,omitempty"`
	Bytes    int64         `json:"bytes,omitempty"`
}

type Inventory struct {
	Present bool
	Entries []InventoryEntry
}

// MaxInventoryEntries bounds one read-only inspection. It covers the 10,000
// live-artifact guardrail (three entries per object plus one retirement
// record) and ordinary records.
const MaxInventoryEntries = 50_000

var inventoryEntryLimit = MaxInventoryEntries

var ErrInventoryBound = errors.New("inventory entry bound exceeded")

var (
	hexDigestName   = regexp.MustCompile(`^[0-9a-f]{64}\.json$`)
	cleanupName     = regexp.MustCompile(`^cleanup-[0-9a-f]{32}\.json$`)
	versionDirName  = regexp.MustCompile(`^v([0-9]{1,4})$`)
	schemaVersionRe = regexp.MustCompile(`(?m)^schemaVersion:[ \t]*(-?[0-9]{1,6})[ \t]*$`)
)

type inventoryWalk struct {
	ctx     context.Context
	entries []InventoryEntry
	count   int
}

func (w *inventoryWalk) add(relative string, kind InventoryKind, wire []byte) error {
	w.count++
	if w.count > inventoryEntryLimit {
		return ErrInventoryBound
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	entry := InventoryEntry{Relative: relative, Kind: kind}
	if wire != nil {
		digest := sha256.Sum256(wire)
		entry.Digest = hex.EncodeToString(digest[:])
		entry.Bytes = int64(len(wire))
	}
	w.entries = append(w.entries, entry)
	return nil
}

// InspectStateInventory classifies every entry below a Lingo state root with
// the v1 decoders or the frozen historical POC signature. It never creates,
// locks, repairs, or removes anything; unsafe or unknown entries are reported.
func InspectStateInventory(ctx context.Context, path string) (Inventory, error) {
	return inspectInventory(ctx, path, walkStateRoot)
}

// InspectPortableInventory classifies portable Project working copies.
func InspectPortableInventory(ctx context.Context, path string) (Inventory, error) {
	return inspectInventory(ctx, path, walkPortableRoot)
}

func inspectInventory(ctx context.Context, path string, walk func(*inventoryWalk, *os.Root) error) (Inventory, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) == string(filepath.Separator) {
		return Inventory{}, ErrUnsafe
	}
	root, err := existingPrivateRoot(filepath.Clean(path))
	if errors.Is(err, ErrNotFound) {
		return Inventory{}, nil
	}
	if err != nil {
		return Inventory{Present: true, Entries: []InventoryEntry{{Relative: ".", Kind: InventoryUnsafe}}}, nil
	}
	defer root.Close()
	state := &inventoryWalk{ctx: ctx}
	if err := walk(state, root); err != nil {
		return Inventory{}, err
	}
	sort.Slice(state.entries, func(i, j int) bool { return state.entries[i].Relative < state.entries[j].Relative })
	return Inventory{Present: true, Entries: state.entries}, nil
}

func walkStateRoot(w *inventoryWalk, root *os.Root) error {
	return eachName(w, root, ".", func(name string) error {
		switch name {
		case "projects":
			return eachChildDirectory(w, root, name, func(projects *os.Root, id, relative string) error {
				if !validUUID(id) {
					return w.add(relative, InventoryUnknown, nil)
				}
				return eachFile(w, projects, id, relative, func(file, fileRelative string) (InventoryKind, int) {
					if strings.HasPrefix(file, ".lingo-install-") || strings.HasPrefix(file, ".lingo-attempt-install-") {
						return InventoryRecovery, 0
					}
					if file != "installation.json" {
						return InventoryUnknown, 0
					}
					return "", MaxRecordBytes
				}, func(wire []byte) InventoryKind {
					if _, issues := DecodeRecord(wire); len(issues) == 0 {
						return InventoryInstallation
					}
					return versionedFailure(wire)
				})
			})
		case "work-items":
			return eachChildDirectory(w, root, name, func(items *os.Root, id, relative string) error {
				if !validUUID(id) {
					return w.add(relative, InventoryUnknown, nil)
				}
				return eachFile(w, items, id, relative, func(file, _ string) (InventoryKind, int) {
					if protocolName(file) || strings.HasPrefix(file, ".lingo-work-item-") {
						return InventoryRecovery, 0
					}
					if !strings.HasSuffix(file, ".json") || strings.HasPrefix(file, ".") {
						return InventoryUnknown, 0
					}
					return "", MaxRecordBytes
				}, func(wire []byte) InventoryKind {
					if _, err := decodeWorkItem(wire); err == nil {
						return InventoryWorkItem
					}
					if _, err := decodeCreateAttempt(wire); err == nil {
						return InventoryCreateAttempt
					}
					return versionedFailure(wire)
				})
			})
		case "executions":
			return eachVersionDirectory(w, root, name, func(w *inventoryWalk, version *os.Root, relative string) error {
				return eachName(w, version, relative, func(id string) error {
					childRelative := relative + "/" + id
					if !validUUID(id) {
						return w.add(childRelative, InventoryUnknown, nil)
					}
					return eachFile(w, version, id, childRelative, func(file, _ string) (InventoryKind, int) {
						if protocolName(file) {
							return InventoryRecovery, 0
						}
						if !hexDigestName.MatchString(file) {
							return InventoryUnknown, 0
						}
						return "", MaxRecordBytes
					}, func(wire []byte) InventoryKind {
						if _, err := decodeExecution(wire); err == nil {
							return InventoryExecution
						}
						return versionedFailure(wire)
					})
				})
			})
		case "artifacts":
			return eachVersionDirectory(w, root, name, walkArtifactsV1)
		case "workflows":
			return eachChildDirectory(w, root, name, func(workflows *os.Root, id, relative string) error {
				if !validUUID(id) {
					return w.add(relative, InventoryUnknown, nil)
				}
				return eachFile(w, workflows, id, relative, func(file, _ string) (InventoryKind, int) {
					if strings.HasPrefix(file, ".lingo-workflow-") || protocolName(file) {
						return InventoryRecovery, 0
					}
					if !strings.HasSuffix(file, ".json") || strings.HasPrefix(file, ".") {
						return InventoryUnknown, 0
					}
					return "", MaxRecordBytes
				}, func(wire []byte) InventoryKind {
					if validPOCWorkflow(wire) {
						return InventoryPOCWorkflow
					}
					return versionedFailure(wire)
				})
			})
		default:
			if protocolName(name) {
				return w.add(name, InventoryRecovery, nil)
			}
			return w.add(name, InventoryUnknown, nil)
		}
	})
}

func walkArtifactsV1(w *inventoryWalk, version *os.Root, relative string) error {
	return eachName(w, version, relative, func(name string) error {
		childRelative := relative + "/" + name
		switch name {
		case "objects":
			return eachChildDirectory(w, version, name, func(objects *os.Root, shard, shardRelative string) error {
				if len(shard) != 2 || !lowerHex(shard) {
					if protocolName(shard) {
						return w.add(shardRelative, InventoryRecovery, nil)
					}
					return w.add(shardRelative, InventoryUnknown, nil)
				}
				shardRoot, err := existingPrivateChild(objects, shard)
				if err != nil {
					return w.add(shardRelative, InventoryUnsafe, nil)
				}
				defer shardRoot.Close()
				return eachName(w, shardRoot, shardRelative, func(id string) error {
					objectRelative := shardRelative + "/" + id
					if protocolName(id) {
						return w.add(objectRelative, InventoryRecovery, nil)
					}
					if !detailartifact.ValidID(id) || id[:2] != shard {
						return w.add(objectRelative, InventoryUnknown, nil)
					}
					return inventoryArtifact(w, shardRoot, id, objectRelative)
				})
			})
		case "cleanup":
			return eachFile(w, version, name, childRelative, func(file, _ string) (InventoryKind, int) {
				if protocolName(file) {
					return InventoryRecovery, 0
				}
				if !cleanupName.MatchString(file) {
					return InventoryUnknown, 0
				}
				return "", detailartifact.MaxCleanupRecord
			}, func(wire []byte) InventoryKind {
				if _, err := decodeCleanupRecord(wire); err == nil {
					return InventoryCleanupRecord
				}
				return versionedFailure(wire)
			})
		case "retirements":
			return eachFile(w, version, name, childRelative, func(file, _ string) (InventoryKind, int) {
				if protocolName(file) {
					return InventoryRecovery, 0
				}
				if !detailartifact.ValidRetirementRecordID(file) {
					return InventoryUnknown, 0
				}
				return "", detailartifact.MaxRetirementRecord
			}, func(wire []byte) InventoryKind {
				if _, err := decodeRetirementRecord(wire); err == nil {
					return InventoryRetirementRecord
				}
				return versionedFailure(wire)
			})
		default:
			if protocolName(name) {
				return w.add(childRelative, InventoryRecovery, nil)
			}
			return w.add(childRelative, InventoryUnknown, nil)
		}
	})
}

func inventoryArtifact(w *inventoryWalk, shard *os.Root, id, relative string) error {
	object, err := existingPrivateChild(shard, id)
	if err != nil {
		return w.add(relative, InventoryUnsafe, nil)
	}
	defer object.Close()
	names, err := readDirectoryNamesBounded(object, 2)
	if err != nil {
		return w.add(relative, InventoryUnknown, nil)
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "details.md" || names[1] != "metadata.json" {
		return w.add(relative, InventoryMalformed, nil)
	}
	for _, name := range names {
		if info, err := object.Lstat(name); err != nil || !info.Mode().IsRegular() {
			return w.add(relative, InventoryUnsafe, nil)
		}
	}
	metadata, err := readPrivateFileBounded(object, "metadata.json", detailartifact.MaxMetadataBytes)
	if err != nil {
		return w.add(relative, InventoryUnsafe, nil)
	}
	markdown, err := readPrivateFileBounded(object, "details.md", detailartifact.MaxContentBytes)
	if err != nil {
		return w.add(relative, InventoryUnsafe, nil)
	}
	artifact, err := detailartifact.Decode(metadata, markdown)
	if err != nil || artifact.ID != id {
		return w.add(relative, versionedFailure(metadata), metadata)
	}
	return w.add(relative, InventoryArtifact, append(metadata, markdown...))
}

func walkPortableRoot(w *inventoryWalk, root *os.Root) error {
	return eachName(w, root, ".", func(name string) error {
		if strings.HasPrefix(name, ".lingo-stage-") || strings.HasPrefix(name, ".lingo-attempt-") || protocolName(name) {
			return w.add(name, InventoryRecovery, nil)
		}
		if !project.ValidSlug(name) {
			return w.add(name, InventoryUnknown, nil)
		}
		return eachFile(w, root, name, name, func(file, _ string) (InventoryKind, int) {
			if strings.HasPrefix(file, ".lingo-manifest-") || strings.HasPrefix(file, ".lingo-attempt-update-") || protocolName(file) {
				return InventoryRecovery, 0
			}
			if file != manifestName {
				return InventoryUnknown, 0
			}
			return "", MaxRecordBytes
		}, func(wire []byte) InventoryKind {
			if _, issues := manifest.Decode(wire); len(issues) == 0 {
				return InventoryPortableManifest
			}
			match := schemaVersionRe.FindSubmatch(wire)
			if match == nil {
				return InventoryMalformed
			}
			version, err := strconv.Atoi(string(match[1]))
			switch {
			case err != nil:
				return InventoryMalformed
			case version > 1:
				return InventoryNewer
			case version < 1:
				return InventoryOlder
			}
			return InventoryMalformed
		})
	})
}

// fileRule returns a fixed kind with limit 0, or a positive read limit whose
// bounded content is then classified by the decoder.
type fileRule func(name, relative string) (InventoryKind, int)

func eachName(w *inventoryWalk, root *os.Root, relative string, visit func(string) error) error {
	names, err := readDirectoryNamesBounded(root, maxLocalDirectoryEntries)
	if err != nil {
		if errors.Is(err, ErrUnsafe) {
			return ErrInventoryBound
		}
		return w.add(relative, InventoryUnsafe, nil)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func eachChildDirectory(w *inventoryWalk, parent *os.Root, name string, visit func(*os.Root, string, string) error) error {
	info, err := parent.Lstat(name)
	if err != nil || !info.IsDir() {
		return w.add(name, InventoryUnsafe, nil)
	}
	child, err := existingPrivateChild(parent, name)
	if err != nil {
		return w.add(name, InventoryUnsafe, nil)
	}
	defer child.Close()
	return eachName(w, child, name, func(entry string) error {
		relative := name + "/" + entry
		if protocolName(entry) {
			return w.add(relative, InventoryRecovery, nil)
		}
		info, err := child.Lstat(entry)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return w.add(relative, InventoryUnsafe, nil)
		}
		return visit(child, entry, relative)
	})
}

func eachVersionDirectory(w *inventoryWalk, parent *os.Root, name string, walkV1 func(*inventoryWalk, *os.Root, string) error) error {
	child, err := existingPrivateChild(parent, name)
	if err != nil {
		return w.add(name, InventoryUnsafe, nil)
	}
	defer child.Close()
	return eachName(w, child, name, func(entry string) error {
		relative := name + "/" + entry
		match := versionDirName.FindStringSubmatch(entry)
		if match == nil {
			if protocolName(entry) {
				return w.add(relative, InventoryRecovery, nil)
			}
			return w.add(relative, InventoryUnknown, nil)
		}
		version, _ := strconv.Atoi(match[1])
		if version > 1 {
			return w.add(relative, InventoryNewer, nil)
		}
		if version < 1 {
			return w.add(relative, InventoryOlder, nil)
		}
		versionRoot, err := existingPrivateChild(child, entry)
		if err != nil {
			return w.add(relative, InventoryUnsafe, nil)
		}
		defer versionRoot.Close()
		return walkV1(w, versionRoot, relative)
	})
}

func eachFile(w *inventoryWalk, parent *os.Root, name, relative string, rule fileRule, decode func([]byte) InventoryKind) error {
	directory, err := existingPrivateChild(parent, name)
	if err != nil {
		return w.add(relative, InventoryUnsafe, nil)
	}
	defer directory.Close()
	return eachName(w, directory, relative, func(file string) error {
		fileRelative := relative + "/" + file
		kind, limit := rule(file, fileRelative)
		if limit == 0 {
			return w.add(fileRelative, kind, nil)
		}
		// Never open a non-regular entry: a FIFO or device could block or
		// have side effects before the post-open type check runs.
		if info, err := directory.Lstat(file); err != nil || !info.Mode().IsRegular() {
			return w.add(fileRelative, InventoryUnsafe, nil)
		}
		wire, err := readPrivateFileBounded(directory, file, limit)
		if err != nil {
			return w.add(fileRelative, InventoryUnsafe, nil)
		}
		return w.add(fileRelative, decode(wire), wire)
	})
}

func protocolName(name string) bool {
	return strings.HasPrefix(name, ".axiom-stage-") || strings.HasPrefix(name, ".axiom-recovery-")
}

func validUUID(value string) bool { return len(project.ValidateIdentity(value, "inventory")) == 0 }

// versionedFailure separates newer/older declared formats from malformed data
// without trusting any other field of a record that failed its decoder.
func versionedFailure(wire []byte) InventoryKind {
	var probe struct {
		FormatVersion *int `json:"formatVersion"`
	}
	if json.Unmarshal(wire, &probe) != nil || probe.FormatVersion == nil {
		return InventoryMalformed
	}
	switch {
	case *probe.FormatVersion > 1:
		return InventoryNewer
	case *probe.FormatVersion < 1:
		return InventoryOlder
	}
	return InventoryMalformed
}

// ReadOwnedFile reads one bounded regular file below an existing private
// root through anchored private directories. It never follows links and
// rejects unsafe ownership, modes, ACLs, and additional hard links.
func ReadOwnedFile(base, relative string, limit int) ([]byte, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if !filepath.IsAbs(base) || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, ErrUnsafe
	}
	root, err := existingPrivateRoot(filepath.Clean(base))
	if err != nil {
		return nil, err
	}
	parts := strings.Split(clean, string(filepath.Separator))
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, err := existingPrivateChild(current, part)
		current.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
	defer current.Close()
	if info, err := current.Lstat(parts[len(parts)-1]); err != nil || !info.Mode().IsRegular() {
		return nil, ErrUnsafe
	}
	return readPrivateFileBounded(current, parts[len(parts)-1], limit)
}

// CheckPrivateDirectory validates an existing owner-only directory with the
// same ownership, link, mode, and ACL checks used by local stores. It is a
// one-shot check: a caller that will then mutate the directory should use
// OpenOwnedDirectory instead and keep operating through that same handle, so
// validation and mutation cannot land on different objects.
func CheckPrivateDirectory(path string) error {
	directory, err := OpenOwnedDirectory(path)
	if err != nil {
		return err
	}
	return directory.Close()
}

// CheckPublicationDirectory validates an existing directory that Axiom
// publishes into without owning it, such as a user bin directory: user-owned,
// not a symlink, no group or other write, no extended ACL. It is a one-shot
// check; see OpenPublicationDirectory for a caller that will also mutate it.
func CheckPublicationDirectory(path string) error {
	directory, err := OpenPublicationDirectory(path)
	if err != nil {
		return err
	}
	return directory.Close()
}

// ReadPublishedFile reads one bounded owner-only regular file directly inside
// a publication directory. It never follows links and rejects unsafe
// ownership, modes, ACLs, and additional hard links on the file. It is a
// one-shot open; a caller that will also stage/rename in the same directory
// should use OpenPublicationDirectory once and call ReadFile on that handle.
func ReadPublishedFile(directory, name string, limit int) ([]byte, error) {
	opened, err := OpenPublicationDirectory(filepath.Clean(directory))
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	return opened.ReadFile(name, limit)
}

// pocWorkflowDTO is frozen from v0.1.0-poc.1
// (242d67c4cf2d4c3efe534dd894cb56a05558e139), internal/local/workflow_store.go.
type pocWorkflowDTO struct {
	FormatVersion  int                  `json:"formatVersion"`
	ProjectID      string               `json:"projectId"`
	RepositoryKey  string               `json:"repositoryKey"`
	RepositoryPath string               `json:"repositoryPath"`
	WorkItem       int                  `json:"workItem"`
	Status         string               `json:"status"`
	Current        int                  `json:"current"`
	Steps          []pocWorkflowStepDTO `json:"steps"`
}

type pocWorkflowStepDTO struct {
	Gate      string `json:"gate"`
	Status    string `json:"status"`
	Reference string `json:"reference,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

var pocGates = []string{"specification", "clarification", "plan", "tasks", "implementation", "review", "evidence", "reconciliation", "completion"}

// validPOCWorkflow reproduces the historical POSIX decoder and validator;
// a record that the POC itself would reject is not a positive signature.
func validPOCWorkflow(wire []byte) bool {
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(wire), MaxRecordBytes+1))
	decoder.DisallowUnknownFields()
	var dto pocWorkflowDTO
	if decoder.Decode(&dto) != nil {
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || dto.FormatVersion != 1 {
		return false
	}
	if !validUUID(dto.ProjectID) || !project.ValidSlug(dto.RepositoryKey) || dto.WorkItem <= 0 || !path.IsAbs(dto.RepositoryPath) {
		return false
	}
	if len(dto.Steps) != len(pocGates) || dto.Current < 0 || dto.Current > len(dto.Steps) {
		return false
	}
	if dto.Status != "active" && dto.Status != "interrupted" && dto.Status != "completed" {
		return false
	}
	for index, step := range dto.Steps {
		hasDigest := step.Digest != ""
		if hasDigest {
			decoded, err := hex.DecodeString(step.Digest)
			if err != nil || len(decoded) != sha256.Size || bytes.Equal(decoded, make([]byte, sha256.Size)) {
				return false
			}
		}
		if step.Gate != pocGates[index] || step.Status != "pending" && step.Status != "passed" && step.Status != "failed" {
			return false
		}
		if step.Gate == "completion" {
			if step.Reference != "" || hasDigest || step.Status == "failed" {
				return false
			}
		} else if step.Status == "pending" {
			if (step.Reference == "") != !hasDigest {
				return false
			}
		} else if step.Reference == "" || !hasDigest {
			return false
		}
		if index < dto.Current && step.Status != "passed" || index > dto.Current && step.Status != "pending" {
			return false
		}
	}
	if dto.Status == "completed" {
		return dto.Current == len(dto.Steps)
	}
	if dto.Current == len(dto.Steps) {
		return false
	}
	if dto.Status == "active" {
		return dto.Steps[dto.Current].Status == "pending"
	}
	return dto.Steps[dto.Current].Status == "failed"
}
