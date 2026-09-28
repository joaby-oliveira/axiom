package codexruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"strings"
)

// integration is one Runtime's user-global Axiom skill integration: the
// shared thin skill set published under that Runtime's skill root, with only
// the ownership history that Runtime actually had. Content outside that
// history is never replaced.
type integration struct {
	runtime        string
	legacySkills   map[string][]string
	legacyReceipts [][]byte
	receipt        func(root string) ([]byte, error)
}

var codexIntegration = integration{
	runtime:        "codex",
	legacySkills:   legacySkillDigests,
	legacyReceipts: legacyReceiptWires,
	receipt:        func(string) ([]byte, error) { return receiptBytes() },
}

// claudeIntegration starts without history: no earlier Axiom version was
// ever installed into a Claude skill root, so no older content is owned.
var claudeIntegration = integration{
	runtime:      "claude",
	legacySkills: map[string][]string{},
	receipt:      claudeReceiptBytes,
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

func (i integration) knownDigest(name, digest string) bool {
	for _, known := range i.legacySkills[name] {
		if digest == known {
			return true
		}
	}
	return false
}

// claudeReceiptBytes records the Runtime, the skill root the set was
// published to, and each skill's identity and digest. It records ownership
// facts only; it never authorizes replacing content that is not a known
// Axiom revision.
func claudeReceiptBytes(root string) ([]byte, error) {
	if strings.ContainsAny(root, "\n\r") {
		return nil, errors.New("unsafe skill root")
	}
	digest, err := manifestDigest()
	if err != nil {
		return nil, err
	}
	var builder strings.Builder
	builder.WriteString("formatVersion=1\nruntime=claude\nskillsRoot=" + root + "\nskillSetVersion=" + SkillSetVersion + "\nbinaryCompatibility=" + BinaryCompatibility + "\nmanifestSha256=" + digest + "\n")
	for _, name := range skillNames {
		content, err := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(content)
		builder.WriteString("skill." + name + "=" + hex.EncodeToString(sum[:]) + "\n")
	}
	return []byte(builder.String()), nil
}
