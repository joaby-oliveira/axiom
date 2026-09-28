package codexruntime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// UpgradeStagePrefix names private staging files that an authorized binary
// upgrade leaves inside an Axiom skill directory only when interrupted.
const UpgradeStagePrefix = ".axiom-upgrade-skill."

const maxUpgradeSkillBytes = 1 << 20

// ErrUpgradeConflict reports Axiom skill state that an upgrade must not touch.
var ErrUpgradeConflict = errors.New("codex skill set is not upgradeable")

// UpgradeSkill is a read-only observation of one Axiom-owned skill directory.
type UpgradeSkill struct {
	Name      string
	Directory bool
	SHA256    string
	Owned     bool
	Leftovers []string
}

// UpgradeInventory is the skill state an upgrade plans against. Configured is
// false only when no Axiom skill directory and no skill-set receipt exist.
type UpgradeInventory struct {
	Configured bool
	Receipt    bool
	Skills     []UpgradeSkill
}

// InspectUpgrade reads the Axiom skill directories without creating the root,
// taking the install lock, or modifying anything. Owned reports content equal
// to this binary's skill or a known legacy digest; callers may hold stronger
// ownership proofs. Unknown entries and unsafe files are a conflict.
func (s Service) InspectUpgrade(ctx context.Context) (UpgradeInventory, error) {
	if err := ctx.Err(); err != nil {
		return UpgradeInventory{}, err
	}
	inventory := UpgradeInventory{Skills: make([]UpgradeSkill, 0, len(skillNames))}
	if _, err := os.Lstat(s.root); os.IsNotExist(err) {
		for _, name := range skillNames {
			inventory.Skills = append(inventory.Skills, UpgradeSkill{Name: name})
		}
		return inventory, nil
	} else if err != nil || !skillRootDirectory(s.root) {
		return UpgradeInventory{}, ErrUpgradeConflict
	}
	if _, err := os.Lstat(filepath.Join(s.root, ".axiom-skill-set-receipt-stage")); !os.IsNotExist(err) {
		return UpgradeInventory{}, ErrUpgradeConflict
	}
	if _, err := os.Lstat(filepath.Join(s.root, receiptName)); err == nil {
		inventory.Receipt, inventory.Configured = true, true
	} else if !os.IsNotExist(err) {
		return UpgradeInventory{}, ErrUpgradeConflict
	}
	for _, name := range skillNames {
		skill, err := s.inspectUpgradeSkill(name)
		if err != nil {
			return UpgradeInventory{}, err
		}
		inventory.Configured = inventory.Configured || skill.Directory
		inventory.Skills = append(inventory.Skills, skill)
	}
	return inventory, nil
}

func (s Service) inspectUpgradeSkill(name string) (UpgradeSkill, error) {
	skill := UpgradeSkill{Name: name}
	directory := filepath.Join(s.root, name)
	if _, err := os.Lstat(directory); os.IsNotExist(err) {
		return skill, nil
	} else if err != nil || !privateDirectory(directory) {
		return UpgradeSkill{}, ErrUpgradeConflict
	}
	skill.Directory = true
	entries, err := os.ReadDir(directory)
	if err != nil {
		return UpgradeSkill{}, ErrUpgradeConflict
	}
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		switch {
		case entry.Name() == "SKILL.md" && privateRegularFile(path):
			content, ok := readBoundedSkill(path)
			if !ok {
				return UpgradeSkill{}, ErrUpgradeConflict
			}
			digest := sha256.Sum256(content)
			skill.SHA256 = hex.EncodeToString(digest[:])
			embedded, embeddedErr := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
			skill.Owned = embeddedErr == nil && string(embedded) == string(content) || s.integration.knownDigest(name, skill.SHA256)
		case strings.HasPrefix(entry.Name(), UpgradeStagePrefix) && privateRegularFile(path):
			skill.Leftovers = append(skill.Leftovers, path)
		default:
			return UpgradeSkill{}, ErrUpgradeConflict
		}
	}
	return skill, nil
}

// LockForUpgrade takes the same exclusive skill-set lock as Install so a
// concurrent runtime install cannot interleave with upgrade publication.
func (s Service) LockForUpgrade() (func(), error) {
	lock, category := acquireInstallLock(s.root)
	if category != "" {
		return nil, errors.New(category)
	}
	return func() { _ = lock.Close() }, nil
}

// PublishUpgradeSkill stages content privately inside the skill directory,
// rechecks that SKILL.md still has the expected digest (empty means absent),
// renames, and confirms the published digest. The caller holds LockForUpgrade.
func (s Service) PublishUpgradeSkill(name string, content []byte, expected string) error {
	known := false
	for _, candidate := range skillNames {
		known = known || candidate == name
	}
	if !known || len(content) == 0 || len(content) > maxUpgradeSkillBytes {
		return ErrUpgradeConflict
	}
	nextDigest := sha256.Sum256(content)
	next := hex.EncodeToString(nextDigest[:])
	directory := filepath.Join(s.root, name)
	if _, err := os.Lstat(directory); os.IsNotExist(err) && expected == "" {
		if err := os.Mkdir(directory, 0o700); err != nil {
			return err
		}
		syncPath(s.root)
	}
	if !privateDirectory(directory) {
		return ErrUpgradeConflict
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return err
	}
	stage := UpgradeStagePrefix + hex.EncodeToString(entropy[:])
	file, err := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = root.Remove(stage)
		}
	}()
	written, writeErr := file.Write(content)
	chmodErr := file.Chmod(0o600)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, chmodErr, syncErr, closeErr); err != nil || written != len(content) {
		return errors.New("skill stage write failed")
	}
	if staged, ok := readBoundedSkill(filepath.Join(directory, stage)); !ok || digestOf(staged) != next {
		return errors.New("skill stage verification failed")
	}
	if current := s.currentSkillDigest(name); current != expected {
		return ErrUpgradeConflict
	}
	if err := root.Rename(stage, "SKILL.md"); err != nil {
		return err
	}
	committed = true
	syncPath(directory)
	if s.currentSkillDigest(name) != next {
		return errors.New("skill publication uncertain")
	}
	return nil
}

// currentSkillDigest is "" for an absent SKILL.md and "unsafe" for anything
// that is not a private regular file.
func (s Service) currentSkillDigest(name string) string {
	path := filepath.Join(s.root, name, "SKILL.md")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return ""
	}
	if !privateRegularFile(path) {
		return "unsafe"
	}
	content, ok := readBoundedSkill(path)
	if !ok {
		return "unsafe"
	}
	return digestOf(content)
}

func readBoundedSkill(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxUpgradeSkillBytes {
		return nil, false
	}
	content, err := os.ReadFile(path)
	return content, err == nil && len(content) <= maxUpgradeSkillBytes
}

func digestOf(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func syncPath(path string) {
	if directory, err := os.Open(path); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
}
