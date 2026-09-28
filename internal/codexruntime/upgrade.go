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

// UpgradeSession holds the skill-set lock and an anchored handle to the
// skill root, opened once by LockForUpgrade: every PublishSkill call for this
// session resolves against that same validated directory object (and a
// per-skill child directory opened from it), so a later replacement of the
// root or a skill directory at its pathname cannot redirect a publication,
// and the session's anchor lets every publication detect and refuse such a
// replacement instead of completing in the displaced object (ADR-0005
// property 3, controlled ancestor/leaf replacement).
type UpgradeSession struct {
	lock   *os.File
	root   *os.Root
	anchor anchor
}

// LockForUpgrade takes the same exclusive skill-set lock as Install and opens
// the skill root once, anchored: a real directory, owned by the current
// user, without group/other write, without extended ACL, with every
// container ancestor safe from replacement by another principal. The
// returned session is what every PublishSkill call for this upgrade
// operates through. The lock is taken only while the root is still the object
// visible at its authorized pathname.
func (s Service) LockForUpgrade() (*UpgradeSession, error) {
	root, identity, err := anchoredRoot(s.root, false)
	if err != nil {
		return nil, ErrUpgradeConflict
	}
	info, err := root.Stat(".")
	if err != nil || info.Mode().Perm()&0o022 != 0 || !ownedByUser(info) {
		root.Close()
		return nil, ErrUpgradeConflict
	}
	directory, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, ErrUpgradeConflict
	}
	aclErr := checkPrivateACL(directory)
	directory.Close()
	if aclErr != nil {
		root.Close()
		return nil, ErrUpgradeConflict
	}
	if identity.verify(root) != nil {
		root.Close()
		return nil, ErrTargetReplaced
	}
	lock, category := acquireInstallLock(root)
	if category != "" {
		root.Close()
		return nil, errors.New(category)
	}
	if identity.verify(root) != nil {
		_ = lock.Close()
		root.Close()
		return nil, ErrTargetReplaced
	}
	return &UpgradeSession{lock: lock, root: root, anchor: identity}, nil
}

// Close releases the install lock and the anchored skill-root handle.
func (u *UpgradeSession) Close() {
	_ = u.lock.Close()
	_ = u.root.Close()
}

// RemoveSkillLeftover removes a stray interrupted-stage artifact directly
// inside name's skill directory, through this session's anchored skill-root
// handle and a child directory opened from it once, never by re-deriving
// either by pathname.
func (u *UpgradeSession) RemoveSkillLeftover(name, leftoverName string) error {
	child, err := privateChild(u.root, name)
	if err != nil {
		return err
	}
	defer child.Close()
	return child.Remove(leftoverName)
}

// PublishSkill stages content privately inside name's skill directory
// (creating it if absent and expected is ""), rechecks that SKILL.md still
// has the expected digest, renames, and confirms the published digest,
// entirely through this session's anchored skill-root handle and a child
// directory opened from it once, never by re-deriving either by pathname.
// Before the directory create, the stage, and the rename, the skill root (with
// every ancestor) and the skill directory must still be the objects visible
// at their authorized pathnames, otherwise ErrTargetReplaced is returned with
// nothing committed; the same proof must hold after the rename for the
// publication to be confirmed.
func (u *UpgradeSession) PublishSkill(name string, content []byte, expected string) error {
	known := false
	for _, candidate := range skillNames {
		known = known || candidate == name
	}
	if !known || len(content) == 0 || len(content) > maxUpgradeSkillBytes {
		return ErrUpgradeConflict
	}
	nextDigest := sha256.Sum256(content)
	next := hex.EncodeToString(nextDigest[:])
	if err := u.anchor.verify(u.root); err != nil {
		return err
	}
	if _, err := u.root.Lstat(name); os.IsNotExist(err) && expected == "" {
		if err := u.root.Mkdir(name, 0o700); err != nil {
			return err
		}
		syncRootObject(u.root)
	}
	child, err := privateChild(u.root, name)
	if err != nil {
		return ErrUpgradeConflict
	}
	defer child.Close()
	still := func() error {
		if err := u.anchor.verify(u.root); err != nil {
			return err
		}
		return childStillAt(u.root, child, name)
	}
	if err := still(); err != nil {
		return err
	}
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return err
	}
	stage := UpgradeStagePrefix + hex.EncodeToString(entropy[:])
	file, err := child.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = child.Remove(stage)
		}
	}()
	written, writeErr := file.Write(content)
	chmodErr := file.Chmod(0o600)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, chmodErr, syncErr, closeErr); err != nil || written != len(content) {
		return errors.New("skill stage write failed")
	}
	if staged, ok := readBoundedSkillIn(child, stage); !ok || digestOf(staged) != next {
		return errors.New("skill stage verification failed")
	}
	if current := currentSkillDigestIn(child); current != expected {
		return ErrUpgradeConflict
	}
	if err := still(); err != nil {
		return err
	}
	if err := child.Rename(stage, "SKILL.md"); err != nil {
		return err
	}
	committed = true
	syncRootObject(child)
	if currentSkillDigestIn(child) != next || still() != nil {
		return errors.New("skill publication uncertain")
	}
	return nil
}

// currentSkillDigestIn is "" for an absent SKILL.md and "unsafe" for anything
// that is not a private regular file, evaluated through child, the already
// anchored skill directory.
func currentSkillDigestIn(child *os.Root) string {
	info, err := child.Lstat("SKILL.md")
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxUpgradeSkillBytes {
		return "unsafe"
	}
	content, ok := privateRegularFileIn(child, "SKILL.md")
	if !ok || int64(len(content)) > maxUpgradeSkillBytes {
		return "unsafe"
	}
	return digestOf(content)
}

func readBoundedSkillIn(child *os.Root, name string) ([]byte, bool) {
	info, err := child.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxUpgradeSkillBytes {
		return nil, false
	}
	content, err := child.ReadFile(name)
	return content, err == nil && len(content) <= maxUpgradeSkillBytes
}

// syncRootObject fsyncs the anchored directory itself, to surface a reported
// I/O error and make a preceding create/rename/remove durable against the
// directory entry.
func syncRootObject(root *os.Root) {
	if directory, err := root.Open("."); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
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
