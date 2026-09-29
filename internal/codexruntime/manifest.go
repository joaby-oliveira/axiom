package codexruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
)

const (
	SkillSetVersion     = "2"
	BinaryCompatibility = "2"
	receiptName         = ".axiom-skill-set.receipt"
)

type SkillDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	FormatVersion       int           `json:"formatVersion"`
	SkillSetVersion     string        `json:"skillSetVersion"`
	BinaryCompatibility string        `json:"binaryCompatibility"`
	Skills              []SkillDigest `json:"skills"`
}

// skillSetRevision is one skill set as an Axiom release published it: the
// skill-set and binary compatibility versions and each SKILL.md digest.
type skillSetRevision struct {
	skillSetVersion     string
	binaryCompatibility string
	skills              map[string]string
}

// sharedSkillHistory lists, oldest first, every earlier skill set that Axiom
// published through the shared Runtime integration (S9/T40 onward). Every
// Runtime installer recognizes these revisions, and the receipts derived from
// them, as Axiom-owned, so a binary whose skill text changed converges each
// Runtime root it finds instead of only the root `axiom upgrade` publishes.
// When the embedded skill text changes, append the revision being replaced
// here; never extend one Runtime's own history instead. Empty today: the
// embedded skill set is the first one published to Claude.
var sharedSkillHistory = []skillSetRevision{}

// currentRevision is the skill set embedded in this binary.
func currentRevision() (skillSetRevision, error) {
	revision := skillSetRevision{skillSetVersion: SkillSetVersion, binaryCompatibility: BinaryCompatibility, skills: make(map[string]string, len(skillNames))}
	for _, name := range skillNames {
		content, err := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if err != nil {
			return skillSetRevision{}, err
		}
		revision.skills[name] = digestOf(content)
	}
	return revision, nil
}

func (r skillSetRevision) manifest() Manifest {
	manifest := Manifest{FormatVersion: 1, SkillSetVersion: r.skillSetVersion, BinaryCompatibility: r.binaryCompatibility, Skills: make([]SkillDigest, 0, len(skillNames))}
	for _, name := range skillNames {
		manifest.Skills = append(manifest.Skills, SkillDigest{Name: name, SHA256: r.skills[name]})
	}
	sort.Slice(manifest.Skills, func(i, j int) bool { return manifest.Skills[i].Name < manifest.Skills[j].Name })
	return manifest
}

func (r skillSetRevision) manifestDigest() (string, error) {
	for _, name := range skillNames {
		if r.skills[name] == "" {
			return "", errors.New("incomplete skill set revision")
		}
	}
	wire, err := json.Marshal(r.manifest())
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func CurrentManifest() (Manifest, error) {
	revision, err := currentRevision()
	if err != nil {
		return Manifest{}, err
	}
	return revision.manifest(), nil
}

func ManifestJSON() ([]byte, error) {
	manifest, err := CurrentManifest()
	if err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func receiptBytes() ([]byte, error) {
	revision, err := currentRevision()
	if err != nil {
		return nil, err
	}
	return codexReceiptBytes("", revision)
}

// codexReceiptBytes is the Codex skill-set receipt of one revision; the
// Codex receipt does not bind its root.
func codexReceiptBytes(_ string, revision skillSetRevision) ([]byte, error) {
	digest, err := revision.manifestDigest()
	if err != nil {
		return nil, err
	}
	return []byte("formatVersion=1\nskillSetVersion=" + revision.skillSetVersion + "\nbinaryCompatibility=" + revision.binaryCompatibility + "\nmanifestSha256=" + digest + "\n"), nil
}
