package compatibility

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/rgomids/axiom/internal/local"
	"github.com/rgomids/axiom/internal/manifest"
	"golang.org/x/sys/unix"
)

type TransferKind string

const (
	Backup TransferKind = "poc_backup"
	Export TransferKind = "portable_export"
)

const (
	TransferReady    = "ready"
	TransferComplete = "complete"
	manifestFile     = "manifest.json"
	maxTransferFile  = 1 << 20
	manifestReserve  = 256 << 10
)

var (
	ErrTransferSource   = errors.New("transfer source is not a complete recognized POC")
	ErrTransferTarget   = errors.New("transfer target is unsafe, occupied, overlapping, or on another filesystem")
	ErrTransferCapacity = errors.New("transfer target lacks observed free space")
	ErrTransferStale    = errors.New("transfer preview changed")
)

type TransferEffect struct {
	Category string `json:"category"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Digest   string `json:"sha256"`
	Bytes    int64  `json:"bytes"`
}

type TransferPreview struct {
	Kind           TransferKind     `json:"kind"`
	State          string           `json:"state"`
	SourceDigest   string           `json:"sourceDigest"`
	Target         string           `json:"target"`
	Effects        []TransferEffect `json:"effects"`
	Omitted        []string         `json:"omitted"`
	RequiredBytes  int64            `json:"requiredBytes"`
	AvailableBytes uint64           `json:"availableBytes"`
	Digest         string           `json:"digest"`
	roots          Roots
}

type TransferAuthority struct {
	kind   TransferKind
	digest string
	target string
}

// TransferManifest is written last; its presence marks a complete target.
type TransferManifest struct {
	FormatVersion int              `json:"formatVersion"`
	Kind          TransferKind     `json:"kind"`
	POCTag        string           `json:"pocTag"`
	POCRevision   string           `json:"pocRevision"`
	SourceDigest  string           `json:"sourceDigest"`
	PreviewDigest string           `json:"previewDigest"`
	Objects       []TransferEffect `json:"objects"`
	Omitted       []string         `json:"omitted"`
}

type TransferResult struct {
	Status          string           `json:"status"`
	Written         []TransferEffect `json:"written"`
	ManifestWritten bool             `json:"manifestWritten"`
}

// Deterministic seams for tests: a write fault and observed free space.
var (
	beforeTransferWrite func(int) error
	availableSpace      = func(path string) (uint64, error) {
		var filesystem unix.Statfs_t
		if err := unix.Statfs(path, &filesystem); err != nil {
			return 0, err
		}
		return uint64(filesystem.Bavail) * uint64(filesystem.Bsize), nil
	}
)

// PreviewTransfer is read-only. Backup and export produce distinct digests,
// and each authority is bound to its own kind, target, and effect set.
func PreviewTransfer(ctx context.Context, kind TransferKind, roots Roots, target string) (TransferPreview, error) {
	if kind != Backup && kind != Export {
		return TransferPreview{}, errors.New("unsupported transfer kind")
	}
	canonicalTarget, err := canonicalTransferTarget(target, roots)
	if err != nil {
		return TransferPreview{}, err
	}
	report, err := Inspect(ctx, roots)
	if err != nil {
		return TransferPreview{}, err
	}
	if report.Classification != RecognizedPOC {
		return TransferPreview{}, ErrTransferSource
	}
	preview := TransferPreview{Kind: kind, State: TransferReady, SourceDigest: report.Digest, Target: canonicalTarget, Effects: []TransferEffect{}, Omitted: []string{}, roots: roots}
	for _, object := range report.objects {
		if kind == Export && object.Kind != local.InventoryPortableManifest {
			preview.Omitted = append(preview.Omitted, object.Category+":"+object.Relative)
			continue
		}
		preview.Effects = append(preview.Effects, TransferEffect{Category: object.Category, Source: object.Relative, Target: object.Category + "/" + object.Relative, Digest: object.Digest, Bytes: object.Bytes})
		preview.RequiredBytes += object.Bytes
	}
	sort.Strings(preview.Omitted)
	if len(preview.Effects) == 0 {
		return TransferPreview{}, ErrTransferSource
	}
	if complete, err := completeTransfer(canonicalTarget, preview); err != nil {
		return TransferPreview{}, err
	} else if complete {
		preview.State = TransferComplete
	} else {
		available, err := checkTransferFilesystem(canonicalTarget, roots)
		if err != nil {
			return TransferPreview{}, err
		}
		preview.AvailableBytes = available
		if uint64(preview.RequiredBytes)+manifestReserve > available {
			return preview, ErrTransferCapacity
		}
	}
	preview.Digest = transferDigest(preview)
	return preview, nil
}

func transferDigest(preview TransferPreview) string {
	wire, _ := json.Marshal(struct {
		Kind          TransferKind     `json:"kind"`
		State         string           `json:"state"`
		SourceDigest  string           `json:"sourceDigest"`
		Target        string           `json:"target"`
		Effects       []TransferEffect `json:"effects"`
		Omitted       []string         `json:"omitted"`
		RequiredBytes int64            `json:"requiredBytes"`
	}{preview.Kind, preview.State, preview.SourceDigest, preview.Target, preview.Effects, preview.Omitted, preview.RequiredBytes})
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:])
}

func AuthorizeTransfer(preview TransferPreview, reviewedDigest string) (TransferAuthority, error) {
	if preview.State != TransferReady || len(preview.Effects) == 0 || preview.Digest == "" || reviewedDigest != preview.Digest {
		return TransferAuthority{}, errors.New("transfer authority denied")
	}
	return TransferAuthority{kind: preview.Kind, digest: preview.Digest, target: preview.Target}, nil
}

// ApplyTransfer creates the target directory itself, so an existing path is
// never adopted. Source content is re-read and digest-checked per object.
// Partial failure leaves the source untouched and reports exact complete
// objects; the absent manifest marks the target incomplete.
func ApplyTransfer(ctx context.Context, preview TransferPreview, authority TransferAuthority) (TransferResult, error) {
	if authority.digest == "" || authority.kind != preview.Kind || authority.digest != preview.Digest || authority.target != preview.Target {
		return TransferResult{Status: "denied_authority"}, ErrTransferStale
	}
	current, err := PreviewTransfer(ctx, preview.Kind, preview.roots, preview.Target)
	if err != nil || current.Digest != preview.Digest {
		return TransferResult{Status: "denied_authority"}, ErrTransferStale
	}
	result := TransferResult{Status: "failure", Written: []TransferEffect{}}
	if err := os.Mkdir(preview.Target, 0o700); err != nil {
		return result, ErrTransferTarget
	}
	root, err := os.OpenRoot(preview.Target)
	if err != nil {
		return result, ErrTransferTarget
	}
	defer root.Close()
	visible, err := os.Lstat(preview.Target)
	opened, openErr := root.Stat(".")
	if err != nil || openErr != nil || !os.SameFile(visible, opened) || opened.Mode().Perm() != 0o700 {
		return result, ErrTransferTarget
	}
	for index, effect := range preview.Effects {
		if err := ctx.Err(); err != nil {
			return partialTransfer(result), err
		}
		if beforeTransferWrite != nil {
			if err := beforeTransferWrite(index); err != nil {
				return partialTransfer(result), err
			}
		}
		wire, err := readTransferSource(preview.roots, effect)
		if err != nil {
			return partialTransfer(result), err
		}
		if err := writeTransferFile(root, effect.Target, wire); err != nil {
			return partialTransfer(result), err
		}
		result.Written = append(result.Written, effect)
	}
	document := TransferManifest{FormatVersion: 1, Kind: preview.Kind, POCTag: HistoricalPOCTag, POCRevision: HistoricalPOCRevision, SourceDigest: preview.SourceDigest, PreviewDigest: preview.Digest, Objects: preview.Effects, Omitted: preview.Omitted}
	wire, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return partialTransfer(result), err
	}
	if err := writeTransferFile(root, manifestFile, append(wire, '\n')); err != nil {
		return partialTransfer(result), err
	}
	result.Status, result.ManifestWritten = "complete", true
	return result, nil
}

func partialTransfer(result TransferResult) TransferResult {
	if len(result.Written) > 0 {
		result.Status = "partial"
	}
	return result
}

func readTransferSource(roots Roots, effect TransferEffect) ([]byte, error) {
	base := map[string]string{CategoryProjects: roots.Projects, CategoryState: roots.State, CategorySkills: roots.Skills}[effect.Category]
	if base == "" || effect.Bytes < 0 {
		return nil, ErrTransferStale
	}
	wire, err := local.ReadOwnedFile(base, effect.Source, maxTransferFile)
	if err != nil {
		return nil, ErrTransferStale
	}
	digest := sha256.Sum256(wire)
	if hex.EncodeToString(digest[:]) != effect.Digest || int64(len(wire)) != effect.Bytes {
		return nil, ErrTransferStale
	}
	if effect.Category == CategoryProjects {
		if _, issues := manifest.Decode(wire); len(issues) != 0 {
			return nil, ErrTransferStale
		}
	}
	return wire, nil
}

// writeTransferFile creates private directories and one new file inside the
// anchored target; it never replaces an existing name.
func writeTransferFile(root *os.Root, relative string, wire []byte) error {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ErrTransferTarget
	}
	parts := strings.Split(clean, string(filepath.Separator))
	directory := ""
	for _, part := range parts[:len(parts)-1] {
		directory = filepath.Join(directory, part)
		if err := root.Mkdir(directory, 0o700); err != nil && !os.IsExist(err) {
			return err
		}
		info, err := root.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
			return ErrTransferTarget
		}
	}
	file, err := root.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(wire)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || written != len(wire) {
		return errors.Join(writeErr, syncErr, closeErr, io.ErrShortWrite)
	}
	parent, err := root.Open(filepath.Dir(clean))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

// completeTransfer reports whether the target already holds exactly the
// authorized transfer, making an equivalent retry a no-op.
func completeTransfer(target string, preview TransferPreview) (bool, error) {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, ErrTransferTarget
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return false, ErrTransferTarget
	}
	defer root.Close()
	wire, err := readAnchored(root, manifestFile, 1<<20)
	if err != nil {
		return false, ErrTransferTarget
	}
	var document TransferManifest
	decoder := json.NewDecoder(bytes.NewReader(wire))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&document) != nil || document.FormatVersion != 1 || document.Kind != preview.Kind || document.SourceDigest != preview.SourceDigest {
		return false, ErrTransferTarget
	}
	expected, _ := json.Marshal(preview.Effects)
	observed, _ := json.Marshal(document.Objects)
	if !bytes.Equal(expected, observed) {
		return false, ErrTransferTarget
	}
	for _, object := range document.Objects {
		content, err := readAnchored(root, object.Target, maxTransferFile)
		if err != nil {
			return false, ErrTransferTarget
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != object.Digest {
			return false, ErrTransferTarget
		}
	}
	return true, nil
}

func readAnchored(root *os.Root, relative string, limit int64) ([]byte, error) {
	file, err := root.OpenFile(filepath.FromSlash(relative), os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, ErrTransferTarget
	}
	wire, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(wire)) > limit {
		return nil, ErrTransferTarget
	}
	return wire, nil
}

// canonicalTransferTarget resolves the existing parent, rejects root and
// relative targets, and rejects any overlap with a source root.
func canonicalTransferTarget(target string, roots Roots) (string, error) {
	if !filepath.IsAbs(target) {
		return "", ErrTransferTarget
	}
	clean := filepath.Clean(target)
	base := filepath.Base(clean)
	if clean == string(filepath.Separator) || base == "." || base == ".." || strings.HasPrefix(base, ".") {
		return "", ErrTransferTarget
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return "", ErrTransferTarget
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() {
		return "", ErrTransferTarget
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return "", ErrTransferTarget
	}
	canonical := filepath.Join(parent, base)
	for _, source := range []string{roots.Projects, roots.State, roots.Skills} {
		if source == "" {
			continue
		}
		resolved := filepath.Clean(source)
		if evaluated, err := filepath.EvalSymlinks(resolved); err == nil {
			resolved = evaluated
		}
		if within(resolved, canonical) || within(canonical, resolved) {
			return "", ErrTransferTarget
		}
	}
	return canonical, nil
}

func within(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// checkTransferFilesystem requires the target parent to share a filesystem
// with every present source root and reports observed free space.
func checkTransferFilesystem(target string, roots Roots) (uint64, error) {
	parent := filepath.Dir(target)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return 0, ErrTransferTarget
	}
	parentStat, ok := parentInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, ErrTransferTarget
	}
	for _, source := range []string{roots.Projects, roots.State, roots.Skills} {
		if source == "" {
			continue
		}
		info, err := os.Stat(source)
		if os.IsNotExist(err) {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if err != nil || !ok || stat.Dev != parentStat.Dev {
			return 0, ErrTransferTarget
		}
	}
	available, err := availableSpace(parent)
	if err != nil {
		return 0, ErrTransferTarget
	}
	return available, nil
}
