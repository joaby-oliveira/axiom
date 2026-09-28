package codexruntime

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// integration is one Runtime's user-global Axiom skill integration: the
// shared thin skill set published under that Runtime's skill root. Content is
// owned only when it is this binary's skill set, a revision in
// sharedSkillHistory, or a revision in the Runtime's own earlier history.
// Content outside that history is never replaced.
type integration struct {
	runtime string
	// legacySkills and legacyReceipts are ownership history only this
	// Runtime had before the shared history began. They are frozen: later
	// revisions go to sharedSkillHistory.
	legacySkills   map[string][]string
	legacyReceipts [][]byte
	receiptFor     func(root string, revision skillSetRevision) ([]byte, error)
}

var codexIntegration = integration{
	runtime:        "codex",
	legacySkills:   legacySkillDigests,
	legacyReceipts: legacyReceiptWires,
	receiptFor:     codexReceiptBytes,
}

// claudeIntegration has no history of its own: no Axiom version before the
// shared history ever installed into a Claude skill root, so only shared
// revisions are Claude-owned.
var claudeIntegration = integration{
	runtime:      "claude",
	legacySkills: map[string][]string{},
	receiptFor:   claudeReceiptBytes,
}

// NewClaude returns the Axiom integration for the Claude user-global skill
// root, normally <Claude configuration directory>/skills.
func NewClaude(root string) (Service, error) {
	service, err := New(root)
	if err != nil {
		return Service{}, err
	}
	service.integration = claudeIntegration
	return service, nil
}

// Runtime reports the Runtime this service integrates with.
func (s Service) Runtime() string { return s.integration.runtime }

func (i integration) category(suffix string) string { return i.runtime + "_" + suffix }

// receipt is this binary's skill-set receipt for root.
func (i integration) receipt(root string) ([]byte, error) {
	revision, err := currentRevision()
	if err != nil {
		return nil, err
	}
	return i.receiptFor(root, revision)
}

// knownDigest reports an earlier Axiom-owned revision of one skill.
func (i integration) knownDigest(name, digest string) bool {
	if slices.Contains(i.legacySkills[name], digest) {
		return true
	}
	for _, revision := range sharedSkillHistory {
		if revision.skills[name] == digest {
			return true
		}
	}
	return false
}

// matchesLegacyReceipt reports that the receipt in root is exactly the
// receipt of an earlier Axiom-owned revision for this Runtime and root.
func (i integration) matchesLegacyReceipt(root string) bool {
	path := filepath.Join(root, receiptName)
	for _, wire := range i.legacyReceipts {
		if matchesPrivateFile(path, wire) {
			return true
		}
	}
	for _, revision := range sharedSkillHistory {
		if wire, err := i.receiptFor(root, revision); err == nil && matchesPrivateFile(path, wire) {
			return true
		}
	}
	return false
}

// rootPath supplies receipt data only; ownership is read from root.
func (i integration) matchesLegacyReceiptIn(root *os.Root, rootPath string) bool {
	for _, wire := range i.legacyReceipts {
		if matchesPrivateFileIn(root, receiptName, wire) {
			return true
		}
	}
	for _, revision := range sharedSkillHistory {
		if wire, err := i.receiptFor(rootPath, revision); err == nil && matchesPrivateFileIn(root, receiptName, wire) {
			return true
		}
	}
	return false
}

func (i integration) receiptRecognizedIn(root *os.Root, rootPath string) bool {
	if _, err := root.Lstat(receiptName); os.IsNotExist(err) {
		return true
	} else if err != nil {
		return false
	}
	current, err := i.receipt(rootPath)
	return err == nil && matchesPrivateFileIn(root, receiptName, current) || i.matchesLegacyReceiptIn(root, rootPath)
}

// claudeReceiptBytes records the Runtime, the skill root the set was
// published to, and each skill's identity and digest. It records ownership
// facts only; it never authorizes replacing content that is not a known
// Axiom revision.
func claudeReceiptBytes(root string, revision skillSetRevision) ([]byte, error) {
	if strings.ContainsAny(root, "\n\r") {
		return nil, errors.New("unsafe skill root")
	}
	digest, err := revision.manifestDigest()
	if err != nil {
		return nil, err
	}
	var builder strings.Builder
	builder.WriteString("formatVersion=1\nruntime=claude\nskillsRoot=" + root + "\nskillSetVersion=" + revision.skillSetVersion + "\nbinaryCompatibility=" + revision.binaryCompatibility + "\nmanifestSha256=" + digest + "\n")
	for _, name := range skillNames {
		builder.WriteString("skill." + name + "=" + revision.skills[name] + "\n")
	}
	return []byte(builder.String()), nil
}
