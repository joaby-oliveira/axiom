package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxArchiveBytes   = 256 << 20
	maxBinaryBytes    = 256 << 20
	maxBundleFile     = 1 << 20
	maxChecksumsBytes = 64 << 10
	maxArchiveEntries = 64
)

var (
	digestPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	semverPattern   = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
	revisionPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)
	metadataFields  = []string{"formatVersion", "product", "version", "revision", "sourceState", "release", "platform", "goos", "architecture", "skillSetVersion"}
	skillNames      = []string{"axiom-project-configure", "axiom-project-show", "axiom-work-item-create", "axiom-work-item-run", "axiom-work-item-status"}
)

// Candidate is a verified release bundle held in memory. Nothing from the
// archive is executed or extracted to disk during verification.
type Candidate struct {
	ArchiveSHA256       string
	Version             string
	Metadata            []string
	Values              map[string]string
	Binary              []byte
	SkillManifestSHA256 string
	Skills              map[string]string
	SkillFiles          map[string][]byte
}

// LoadCandidate mirrors install-release.sh verification: exact SHA256SUMS
// entry, safe single-root archive with regular files only, complete bundle
// manifest, closed release metadata, and the approved skill manifest.
func LoadCandidate(archivePath, checksumsPath string) (Candidate, error) {
	if !filepath.IsAbs(archivePath) || !filepath.IsAbs(checksumsPath) {
		return Candidate{}, &Error{Category: "invalid_input"}
	}
	archive, err := readBoundedRegular(archivePath, maxArchiveBytes)
	if err != nil {
		return Candidate{}, &Error{Category: "archive_unavailable"}
	}
	checksums, err := readBoundedRegular(checksumsPath, maxChecksumsBytes)
	if err != nil {
		return Candidate{}, &Error{Category: "checksum_unavailable"}
	}
	expected := ""
	for _, line := range strings.Split(strings.TrimSuffix(string(checksums), "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filepath.Base(archivePath) {
			if expected != "" {
				return Candidate{}, &Error{Category: "checksum_unavailable"}
			}
			expected = fields[0]
		}
	}
	actual := digest(archive)
	if !digestPattern.MatchString(expected) {
		return Candidate{}, &Error{Category: "checksum_unavailable"}
	}
	if actual != expected {
		return Candidate{}, &Error{Category: "checksum_mismatch"}
	}
	files, err := readBundle(archive)
	if err != nil {
		return Candidate{}, &Error{Category: "archive_invalid"}
	}
	if err := verifyBundleManifest(files); err != nil {
		return Candidate{}, &Error{Category: "archive_invalid"}
	}
	candidate := Candidate{ArchiveSHA256: actual, Binary: files[binaryName], Skills: map[string]string{}, SkillFiles: map[string][]byte{}}
	if candidate.Metadata, candidate.Values, err = parseMetadata(files["release-metadata.txt"]); err != nil {
		return Candidate{}, &Error{Category: "release_metadata_invalid"}
	}
	candidate.Version = candidate.Values["version"]
	skillManifest := files["skills-manifest.txt"]
	if err := parseSkillManifest(skillManifest, files, candidate.Skills); err != nil {
		return Candidate{}, &Error{Category: "skill_manifest_invalid"}
	}
	candidate.SkillManifestSHA256 = digest(skillManifest)
	for _, name := range skillNames {
		candidate.SkillFiles[name] = files["skills/"+name+"/SKILL.md"]
	}
	if len(candidate.Binary) == 0 {
		return Candidate{}, &Error{Category: "archive_invalid"}
	}
	return candidate, nil
}

func readBundle(archive []byte) (map[string][]byte, error) {
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer compressed.Close()
	reader := tar.NewReader(io.LimitReader(compressed, maxArchiveBytes+maxBinaryBytes))
	root := ""
	files := map[string][]byte{}
	for entries := 0; ; entries++ {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || entries >= maxArchiveEntries {
			return nil, errors.New("invalid archive")
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") {
			return nil, errors.New("unsafe archive path")
		}
		first, rest, nested := strings.Cut(name, "/")
		if root == "" {
			root = first
		}
		if first != root || !strings.HasPrefix(root, "axiom-") {
			return nil, errors.New("invalid archive root")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			return nil, errors.New("archive links or special files refused")
		}
		if !nested || rest == "" {
			return nil, errors.New("file outside bundle root")
		}
		limit := int64(maxBundleFile)
		if rest == binaryName {
			limit = maxBinaryBytes
		}
		if header.Size < 0 || header.Size > limit {
			return nil, errors.New("archive entry exceeds limit")
		}
		wire, err := io.ReadAll(io.LimitReader(reader, limit+1))
		if err != nil || int64(len(wire)) != header.Size {
			return nil, errors.New("archive entry truncated")
		}
		if _, exists := files[rest]; exists {
			return nil, errors.New("duplicate archive entry")
		}
		files[rest] = wire
	}
	return files, nil
}

func verifyBundleManifest(files map[string][]byte) error {
	manifest, ok := files["MANIFEST.sha256"]
	if !ok {
		return errors.New("bundle manifest missing")
	}
	listed := []string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(manifest), "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !digestPattern.MatchString(fields[0]) {
			return errors.New("bundle manifest malformed")
		}
		wire, ok := files[fields[1]]
		if !ok || digest(wire) != fields[0] {
			return errors.New("bundle manifest mismatch")
		}
		listed = append(listed, fields[1])
	}
	actual := []string{}
	for name := range files {
		if name != "MANIFEST.sha256" {
			actual = append(actual, name)
		}
	}
	sort.Strings(listed)
	sort.Strings(actual)
	if strings.Join(listed, "\n") != strings.Join(actual, "\n") {
		return errors.New("bundle manifest incomplete")
	}
	return nil
}

func parseMetadata(wire []byte) ([]string, map[string]string, error) {
	lines := strings.Split(strings.TrimSuffix(string(wire), "\n"), "\n")
	if len(lines) != len(metadataFields) || !strings.HasSuffix(string(wire), "\n") {
		return nil, nil, errors.New("metadata schema")
	}
	values := map[string]string{}
	allowed := map[string]bool{}
	for _, field := range metadataFields {
		allowed[field] = true
	}
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok || !allowed[key] || values[key] != "" || value == "" {
			return nil, nil, errors.New("metadata schema")
		}
		values[key] = value
	}
	if values["formatVersion"] != "1" || values["product"] != "Axiom" || !semverPattern.MatchString(values["version"]) || !revisionPattern.MatchString(values["revision"]) {
		return nil, nil, errors.New("unsupported metadata")
	}
	if !(values["release"] == "false" || values["release"] == "true" && values["sourceState"] == "clean") {
		return nil, nil, errors.New("unsupported metadata")
	}
	return lines, values, nil
}

func parseSkillManifest(wire []byte, files map[string][]byte, skills map[string]string) error {
	lines := strings.Split(strings.TrimSuffix(string(wire), "\n"), "\n")
	if len(lines) != 3+len(skillNames) {
		return errors.New("skill manifest schema")
	}
	values := map[string]string{}
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok || values[key] != "" {
			return errors.New("skill manifest schema")
		}
		values[key] = value
	}
	if values["formatVersion"] != "1" || values["skillSetVersion"] != "1" || values["binaryCompatibility"] != "1" {
		return errors.New("unsupported skill manifest")
	}
	for _, name := range skillNames {
		hash := values["skill."+name]
		wire, ok := files["skills/"+name+"/SKILL.md"]
		if !digestPattern.MatchString(hash) || !ok || digest(wire) != hash {
			return errors.New("skill manifest mismatch")
		}
		skills[name] = hash
	}
	return nil
}

func readBoundedRegular(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("unsafe input file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("input file changed")
	}
	wire, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(wire)) > limit {
		return nil, errors.New("input file exceeds limit")
	}
	return wire, nil
}

func digest(wire []byte) string {
	sum := sha256.Sum256(wire)
	return hex.EncodeToString(sum[:])
}
