// Package codexruntime installs thin Axiom entrypoints at Codex user scope.
package codexruntime

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

//go:embed skills/*/SKILL.md
var embeddedSkills embed.FS

// skillFiles is the skill set this binary publishes.
var skillFiles fs.FS = embeddedSkills

var skillNames = []string{
	"axiom-project-configure",
	"axiom-project-show",
	"axiom-work-item-create",
	"axiom-work-item-run",
	"axiom-work-item-status",
}

var legacySkillDigests = map[string][]string{
	"axiom-project-configure": {"d481dc61ecd7a9afd1ffd0a79908515f15f04a75002a7d501eea5517f1f4844e", "87dc55d4a459d4abf70bb53c7f91695da9cbae18da3a5afd6112f964062b5b9c", "b9d55306f7f4e1b33b7606c4327f94cf18dd12287536be7f27d61f8f9a95dc1e", "05d8e420f440529df3bd75a521f3d9493d5cefe3d9fc16ddb1da9ffeed553cd1", "d5271f6a24676c2f8111776cc797232784ad7e75318aca500f6ad73274510b60", "237da8ea6a57e9240ae464d85a1fd1ad2d8c4d19ba8160c24943ae4752b2885d"},
	"axiom-project-show":      {"a80b3038c497fe3f3b817e5d5d28bca68de96d9ceaf76630f08ad1e34c5db0f4", "594fc02985f5884c780b2c584c6774424bb63c5234ee2f32e5800002cd3c5c02", "d7f86666dd2036b53a4cdbe2d6b67d096936f59b164ae9573806fbb9a40d97fd", "2542254b45ef2c1ac67e09ae1d1924fd0648787836b9bbbe60480a6f09646bcc", "74abd548a0b352b9464efb2b1a6d5aca453bc1e88a164903d8a6043dfeebe8ab", "93030842cc6bb1bba159e52f571d1bf1c4e52defce31d4d3998ccd47c1b34c01"},
	"axiom-work-item-create":  {"fe9c6ce1817f5246e749c7ab03d74cf41db07ed5678a07065d90940572dc12d3", "556fff5e6b38d204bd4acd6a37f74a23da33c88409a7fbc91c9ccfaf3d70c493", "6750abfe6cb817ff4f011d7c6b59a27e4b12832a7c1bbae68a9357bee249d4bd", "9b6d28569d02a97ff0273d08a9abd6bc70ec573050c2bd9f3cc5dd40e984fcaf", "8ecdd0553a999372522f7bc7ad0663e8474af7045f997c718e9c82e4799c5db3", "7d69ac3036d16a66df106b82ca21e7753c98b3bb0d203fc40c090659bdd1bfea"},
	"axiom-work-item-run":     {"a75f21684d38f325840461fbe8e959ed9fd2b925ac630c7d471147fdfef124dd", "35bf4d66efa1a182479579f882a408f9b394c32e5b0e02d7dfbf8ef9d129c59b", "49d269602abedde05dc357135dc9262f6146bccc87cb97784790659f5eed37a4", "b5ca1ecf4dd136ba5baa6c647b19080d2d129d539e31b27573e43694ae40982f", "5e1661d06a1caa7f7af6fd8c6253d3742f0cbb26df262a8c8357507e76f85f00"},
	"axiom-work-item-status":  {"4fbb6fda699dc50af88f96355cb9cbed05dbebf34a7ed3218bc26b72b7fd60c7", "009ab0f59c2992c79ca7732a2d451f2b652e4afd94697afd02572bb75ec3db0b", "9f4d5063347eb47ef38d7c7789f27fb13f3a224880e53915080b0ba9bbe8ec5d", "9fcfd0f9caf3a208e54d65cefab81d372cf1fa79c32ba3e1fe8efbc63d7f1990", "213c58a0b55e0b7d52ca97ea72b4d474b5c1577f0f473e4e8b4be8e07c3d9319"},
}

const (
	installLockName = ".axiom-skill-set.lock"
	installLockWire = "formatVersion=1\n"
)

var legacyReceiptWires = [][]byte{
	[]byte("formatVersion=1\nskillSetVersion=2\nbinaryCompatibility=2\nmanifestSha256=38c044c2f82de2dd26e4296a6e22db7f16b87f8c3478790323473fdf44a281d2\n"),
	[]byte("formatVersion=1\nskillSetVersion=2\nbinaryCompatibility=2\nmanifestSha256=aa50528dfd37acc2f5f95c2fc02937bc29cf6ea3cbdd51b8cd81c0a72d677adb\n"),
	[]byte("formatVersion=1\nskillSetVersion=2\nbinaryCompatibility=2\nmanifestSha256=98b58d88e51ad9e5c907067248245a1e758d15b561bbf2d8dc99cc50924cb67d\n"),
	[]byte("formatVersion=1\nskillSetVersion=2\nbinaryCompatibility=2\nmanifestSha256=46949cb5d6bb778069b5f065305f216e26101051198f583008711ee2a30a3fb9\n"),
}

type Status string

const (
	Applied      Status = "applied"
	Unchanged    Status = "unchanged"
	Ready        Status = "ready"
	Missing      Status = "missing"
	Partial      Status = "partial"
	Incompatible Status = "incompatible"
	Failed       Status = "failed"
)

type SkillState struct {
	Name, Digest, State string
}

type Result struct {
	Status              Status
	Category            string
	SkillSetVersion     string
	BinaryCompatibility string
	Skills              []SkillState
}

type Service struct {
	root                string
	binaryCompatibility string
	integration         integration
	afterSkill          func(string)
	afterLock           func()
}

func New(root string) (Service, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return Service{}, errors.New("unsafe Codex skill root")
	}
	return Service{root: filepath.Clean(root), binaryCompatibility: BinaryCompatibility, integration: codexIntegration}, nil
}

func NewForBinary(root, compatibility string) (Service, error) {
	service, err := New(root)
	if err != nil {
		return Service{}, err
	}
	service.binaryCompatibility = compatibility
	return service, nil
}

func (s Service) Install(ctx context.Context) Result {
	if err := ctx.Err(); err != nil {
		return Result{Status: Failed, Category: "cancelled"}
	}
	// The skill root is opened once here, validated (real directory, owned,
	// no group/other write, no ACL, every container ancestor safe from
	// replacement by another principal): the install lock and every skill's
	// install/replace below resolve through this same anchored object or an
	// object opened directly from it, never by re-deriving s.root's pathname.
	// This closes the gap where validating a directory by pathname and later
	// mutating it by pathname again leaves a window in which the two
	// operations can land on different objects (ADR-0005 property 3).
	// Every lock, create, and commit additionally proves first that the
	// anchored root (and, for a skill, its directory) is still the object
	// visible at its authorized pathname, and the result is declared only
	// after that proof holds once more: a replacement is refused, never
	// tolerated by completing the install in the displaced original object.
	root, identity, err := ensureRoot(s.root)
	if err != nil {
		return Result{Status: Failed, Category: s.integration.category("skill_root_unavailable")}
	}
	defer root.Close()
	verify := func() error { return identity.verify(root) }
	if verify() != nil {
		return Result{Status: Failed, Category: s.integration.category("skill_root_unavailable")}
	}
	lock, category := acquireInstallLock(root)
	if category == "skill_install_concurrent" {
		category = s.integration.category(category)
	}
	if category != "" {
		return s.inspectResultIn(root, Failed, category)
	}
	defer lock.Close()
	if verify() != nil {
		return Result{Status: Failed, Category: s.integration.category("skill_root_unavailable")}
	}
	if s.afterLock != nil {
		s.afterLock()
	}
	replaces := false
	for _, name := range skillNames {
		content, err := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if err != nil {
			return Result{Status: Failed, Category: s.integration.category("skill_package_invalid")}
		}
		if !s.integration.installableOne(root, name, content) {
			return s.inspectResultIn(root, Failed, s.integration.category("skill_conflict"))
		}
		replaces = replaces || !matchesSkillIn(root, name, content)
	}
	// A receipt that is neither absent, current nor an earlier Axiom-owned
	// receipt for this root is not Axiom evidence: no skill changes beside it.
	if replaces && !s.integration.receiptRecognizedIn(root, s.root) {
		return s.inspectResultIn(root, Failed, s.integration.category("skill_conflict"))
	}
	changed := false
	for _, name := range skillNames {
		if err := ctx.Err(); err != nil {
			return s.inspectResultIn(root, Partial, s.integration.category("skill_install_partial"))
		}
		content, err := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if err != nil {
			return Result{Status: Failed, Category: s.integration.category("skill_package_invalid")}
		}
		changedOne, createdOne, err := s.integration.installOne(root, name, content, verify)
		if err == ErrTargetReplaced && !changed {
			return Result{Status: Failed, Category: s.integration.category("skill_root_unavailable")}
		}
		if err != nil {
			return s.inspectResultIn(root, Partial, s.integration.category("skill_install_partial"))
		}
		_ = createdOne
		if changedOne {
			changed = true
		}
		if s.afterSkill != nil {
			s.afterSkill(name)
		}
	}
	receipt, err := s.integration.receipt(s.root)
	if err != nil {
		return s.inspectResultIn(root, Partial, s.integration.category("skill_receipt_incomplete"))
	}
	receiptChanged, receiptPublished := s.integration.publishReceiptIn(root, s.root, receipt, verify)
	if !receiptPublished {
		return s.inspectResultIn(root, Partial, s.integration.category("skill_receipt_incomplete"))
	}
	changed = changed || receiptChanged
	if verify() != nil {
		if changed {
			return s.inspectResultIn(root, Partial, s.integration.category("skill_install_partial"))
		}
		return Result{Status: Failed, Category: s.integration.category("skill_root_unavailable")}
	}
	if !changed {
		return s.inspectResultIn(root, Unchanged, s.integration.category("already_configured"))
	}
	return s.inspectResultIn(root, Applied, s.integration.category("configured"))
}

// installableOne decides, from the anchored skill root, whether name may be
// installed or replaced: absent, or an existing private directory whose
// content is already current or a known earlier Axiom revision.
func (i integration) installableOne(root *os.Root, name string, content []byte) bool {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false
	}
	child, err := privateChild(root, name)
	if err != nil {
		return false
	}
	defer child.Close()
	return matchesInstalledIn(child, content) || i.matchesLegacyInstalledIn(child, name)
}

func (s Service) Inspect(ctx context.Context) Result {
	if err := ctx.Err(); err != nil {
		return Result{Status: Failed, Category: "cancelled"}
	}
	_, err := os.Lstat(s.root)
	if os.IsNotExist(err) {
		return s.inspectResult(Missing, s.integration.category("not_configured"))
	}
	if err != nil || !skillRootDirectory(s.root) {
		return Result{Status: Failed, Category: s.integration.category("skill_root_unavailable")}
	}
	if s.binaryCompatibility != BinaryCompatibility {
		return s.inspectResult(Incompatible, s.integration.category("binary_skill_incompatible"))
	}
	for _, name := range skillNames {
		content, readErr := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if readErr != nil || !matchesInstalled(s.root, name, content) {
			return s.inspectResult(Missing, s.integration.category("skills_missing_or_changed"))
		}
	}
	receipt, err := s.integration.receipt(s.root)
	if err != nil || !matchesPrivateFile(filepath.Join(s.root, receiptName), receipt) {
		return s.inspectResult(Partial, s.integration.category("skill_receipt_incomplete"))
	}
	return s.inspectResult(Ready, s.integration.category("ready"))
}

func (s Service) inspectResult(status Status, category string) Result {
	result := Result{Status: status, Category: category, SkillSetVersion: SkillSetVersion, BinaryCompatibility: s.binaryCompatibility, Skills: make([]SkillState, 0, len(skillNames))}
	for _, name := range skillNames {
		content, _ := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		digest := sha256.Sum256(content)
		state := "missing"
		if matchesInstalled(s.root, name, content) {
			state = "equivalent"
		} else if s.integration.matchesLegacyInstalled(s.root, name) {
			state = "owned_older"
		} else if _, err := os.Lstat(filepath.Join(s.root, name)); err == nil {
			state = "modified_or_foreign"
		}
		result.Skills = append(result.Skills, SkillState{Name: name, Digest: hex.EncodeToString(digest[:]), State: state})
	}
	return result
}

func matchesSkillIn(root *os.Root, name string, content []byte) bool {
	child, err := privateChild(root, name)
	if err != nil {
		return false
	}
	defer child.Close()
	return matchesInstalledIn(child, content)
}

func (s Service) inspectResultIn(root *os.Root, status Status, category string) Result {
	result := Result{Status: status, Category: category, SkillSetVersion: SkillSetVersion, BinaryCompatibility: s.binaryCompatibility, Skills: make([]SkillState, 0, len(skillNames))}
	for _, name := range skillNames {
		content, _ := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		state := "missing"
		if _, err := root.Lstat(name); err == nil {
			state = "modified_or_foreign"
		}
		if child, err := privateChild(root, name); err == nil {
			if matchesInstalledIn(child, content) {
				state = "equivalent"
			} else if s.integration.matchesLegacyInstalledIn(child, name) {
				state = "owned_older"
			}
			child.Close()
		}
		result.Skills = append(result.Skills, SkillState{Name: name, Digest: digestOf(content), State: state})
	}
	return result
}

// publishReceiptIn publishes content as the receipt inside root, the same
// anchored skill-root object Install validated once and holds open, never by
// re-deriving rootPath, which is used only as Claude receipt content.
// verify must hold before the stage and the rename and again after the
// rename; otherwise the receipt is not reported as published.
func (i integration) publishReceiptIn(root *os.Root, rootPath string, content []byte, verify func() error) (bool, bool) {
	if matchesPrivateFileIn(root, receiptName, content) {
		return false, verify() == nil
	}
	if _, err := root.Lstat(receiptName); err == nil {
		if !i.matchesLegacyReceiptIn(root, rootPath) {
			return false, false
		}
		return i.replaceKnownReceiptIn(root, rootPath, content, verify)
	} else if !os.IsNotExist(err) {
		return false, false
	}
	if verify() != nil {
		return false, false
	}
	const temporary = ".axiom-skill-set-receipt-stage"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, false
	}
	defer root.Remove(temporary)
	written, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || written != len(content) {
		return false, false
	}
	if _, err := root.Lstat(receiptName); err == nil || !os.IsNotExist(err) {
		return false, false
	}
	if verify() != nil {
		return false, false
	}
	if err := root.Rename(temporary, receiptName); err != nil {
		return false, false
	}
	return true, verify() == nil
}

func (i integration) replaceKnownReceiptIn(root *os.Root, rootPath string, content []byte, verify func() error) (bool, bool) {
	expected, ok := privateRegularFileIn(root, receiptName)
	if !ok || !i.matchesLegacyReceiptIn(root, rootPath) {
		return false, false
	}
	if verify() != nil {
		return false, false
	}
	const temporary = ".axiom-skill-set-receipt-stage"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, false
	}
	defer root.Remove(temporary)
	written, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || written != len(content) {
		return false, false
	}
	if !matchesPrivateFileIn(root, receiptName, expected) || verify() != nil {
		return false, false
	}
	if err := root.Rename(temporary, receiptName); err != nil {
		return false, false
	}
	return true, verify() == nil
}

func matchesPrivateFile(path string, expected []byte) bool {
	if !privateRegularFile(path) {
		return false
	}
	actual, err := os.ReadFile(path)
	return err == nil && string(actual) == string(expected)
}

func privateRegularFile(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !privateRegularInfo(opened) {
		return false
	}
	return checkPrivateACL(file) == nil
}

// matchesPrivateFileIn and privateRegularFileIn are the anchored-root
// counterparts of matchesPrivateFile/privateRegularFile, used by the
// mutation path (publishReceiptIn) so a reread never re-derives root's
// pathname.
func matchesPrivateFileIn(root *os.Root, name string, expected []byte) bool {
	actual, ok := privateRegularFileIn(root, name)
	return ok && string(actual) == string(expected)
}

func privateRegularFileIn(root *os.Root, name string) ([]byte, bool) {
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, false
	}
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !privateRegularInfo(opened) {
		return nil, false
	}
	if checkPrivateACL(file) != nil {
		return nil, false
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, false
	}
	return content, true
}

// privateDirectory accepts a directory Axiom owns: user-owned, mode 0700
// and without extended ACL.
func privateDirectory(path string) bool {
	return directoryWithoutPermissions(path, 0o077)
}

// skillRootDirectory accepts a Runtime's user-global skill root. The root
// belongs to the Runtime, not to Axiom, and Runtimes commonly create it 0755.
// The Axiom skills are public content, so only mutation by another principal
// matters: the root must be a real user-owned directory that group and other
// cannot write and that carries no extended ACL. Everything Axiom creates
// under it stays privateDirectory/privateRegularFile.
func skillRootDirectory(path string) bool {
	return directoryWithoutPermissions(path, 0o022)
}

// directoryWithoutPermissions validates path itself AND every container
// ancestor up to "/": each must be a real directory, not a symlink, and safe
// from replacement by another principal (ancestorSafe); the final directory
// must additionally omit forbidden, be owned by the current user, and carry
// no extended ACL. This closes controlled ancestor/leaf replacement
// (ADR-0005 property 3), not only the final directory's own mode.
func directoryWithoutPermissions(path string, forbidden os.FileMode) bool {
	root, _, err := anchoredRoot(path, false)
	if err != nil {
		return false
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil || info.Mode().Perm()&forbidden != 0 || !ownedByUser(info) {
		return false
	}
	directory, err := root.Open(".")
	if err != nil {
		return false
	}
	defer directory.Close()
	return checkPrivateACL(directory) == nil
}

// trustedCanonical resolves path's existing prefix through real symlinks,
// trusting only a symlink owned by root (the standard system aliases such as
// macOS's /var -> /private/var), and reattaches any not-yet-existing suffix
// literally so a missing final component can still be created.
func trustedCanonical(path string) (string, error) {
	clean := filepath.Clean(path)
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(clean, current), current) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", errors.New("unsafe path")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != 0 {
				return "", errors.New("unsafe path")
			}
		}
	}
	probe := clean
	var suffix []string
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", errors.New("unsafe path")
		}
		suffix = append(suffix, filepath.Base(probe))
		probe = filepath.Dir(probe)
	}
	resolved, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", errors.New("unsafe path")
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	return resolved, nil
}

// anchoredRoot canonicalizes path (trusting only root-owned symlinks in its
// existing ancestry, such as macOS's /var -> /private/var) and then opens it
// one directory object at a time from "/", rejecting a symlinked or replaced
// component and any container ancestor mutable by another principal
// (ancestorSafe). With create, missing components are created owner-only.
// The final component's own ownership/mode/ACL are the caller's
// responsibility. The returned anchor records the identity of every component
// opened, for a later anchor.verify.
func anchoredRoot(path string, create bool) (*os.Root, anchor, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
		return nil, anchor{}, errors.New("unsafe path")
	}
	canonical, err := trustedCanonical(path)
	if err != nil {
		return nil, anchor{}, err
	}
	root, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return nil, anchor{}, err
	}
	container, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, anchor{}, errors.New("unsafe path")
	}
	identity := anchor{path: canonical}
	for _, part := range pathComponents(canonical) {
		if !ancestorSafe(container) {
			root.Close()
			return nil, anchor{}, errors.New("unsafe ancestor")
		}
		if create {
			if err := root.Mkdir(part, 0o700); err != nil && !os.IsExist(err) {
				root.Close()
				return nil, anchor{}, err
			}
		}
		info, err := root.Lstat(part)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			root.Close()
			return nil, anchor{}, errors.New("unsafe path component")
		}
		next, err := root.OpenRoot(part)
		root.Close()
		if err != nil {
			return nil, anchor{}, errors.New("unsafe path component")
		}
		actual, err := next.Stat(".")
		if err != nil || !os.SameFile(info, actual) {
			next.Close()
			return nil, anchor{}, errors.New("unsafe path component")
		}
		root, container = next, actual
		identity.chain = append(identity.chain, actual)
	}
	return root, identity, nil
}

func pathComponents(canonical string) []string {
	var parts []string
	for _, part := range strings.Split(strings.TrimPrefix(canonical, string(filepath.Separator)), string(filepath.Separator)) {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// ErrTargetReplaced reports that the skill root, one of its ancestors, or a
// skill directory is no longer the object visible at its authorized pathname.
var ErrTargetReplaced = errors.New("codex skill target replaced at its authorized pathname")

// errCommitUnconfirmed reports an effect committed in its anchored object that
// could not afterwards be proven visible at its authorized pathname: it is
// never a confirmed change.
var errCommitUnconfirmed = errors.New("codex skill commit not confirmed at its authorized pathname")

// anchor records the canonical pathname a skill-root handle was authorized
// for and the identity of every directory object on that path, as observed
// when the handle was opened. It mirrors internal/local's anchor.
type anchor struct {
	path  string
	chain []os.FileInfo
}

// verify re-walks the anchored pathname from "/" without following any
// symlink and requires every component to still be the object recorded at
// open time, ending at root itself: a renamed, recreated, or symlinked
// ancestor or skill root is detected and rejected with ErrTargetReplaced,
// not tolerated by mutating the original object (ADR-0005 property 3,
// ADR-0007 invariant 7). Callers invoke it at the declared inspection and
// commit boundaries.
func (a anchor) verify(root *os.Root) error {
	parts := pathComponents(a.path)
	if len(parts) == 0 || len(parts) != len(a.chain) {
		return ErrTargetReplaced
	}
	walk, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return ErrTargetReplaced
	}
	for index, part := range parts {
		info, err := walk.Lstat(part)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !os.SameFile(info, a.chain[index]) {
			walk.Close()
			return ErrTargetReplaced
		}
		next, err := walk.OpenRoot(part)
		walk.Close()
		if err != nil {
			return ErrTargetReplaced
		}
		walk = next
	}
	visible, err := walk.Stat(".")
	walk.Close()
	if err != nil {
		return ErrTargetReplaced
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(visible, opened) {
		return ErrTargetReplaced
	}
	return nil
}

// childStillAt reports ErrTargetReplaced unless child, a skill directory
// opened from parent, is still the real directory visible as name in parent.
func childStillAt(parent, child *os.Root, name string) error {
	visible, err := parent.Lstat(name)
	if err != nil || visible.Mode()&os.ModeSymlink != 0 || !visible.IsDir() {
		return ErrTargetReplaced
	}
	opened, err := child.Stat(".")
	if err != nil || !os.SameFile(visible, opened) {
		return ErrTargetReplaced
	}
	return nil
}

// ancestorSafe reports whether a directory that CONTAINS a later path
// component is safe from having that entry replaced, renamed or removed by a
// principal other than root or the current user: owned by root or by the
// current user, and disallowing group/other write unless the sticky bit
// restricts removal/rename of existing entries to each entry's own owner.
// An ancestor owned by neither is never trusted, sticky or not, since its
// owner already has unilateral control over what it contains. This mirrors
// internal/local's ancestorSafe (ADR-0005 property 3).
func ancestorSafe(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	if stat.Uid != 0 && stat.Uid != uint32(os.Getuid()) {
		return false
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return false
	}
	return true
}

// privateChild opens name inside parent as a private, owner-only Axiom
// subdirectory (mode 0700, no ACL, no symlink), verifying its identity
// against the same object the initial Lstat observed, so a caller that goes
// on to read or mutate it does so through this same anchored object.
func privateChild(parent *os.Root, name string) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || !ownedByUser(info) {
		return nil, errors.New("unsafe skill directory")
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, errors.New("unsafe skill directory")
	}
	actual, err := child.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		child.Close()
		return nil, errors.New("unsafe skill directory")
	}
	file, err := child.Open(".")
	if err != nil {
		child.Close()
		return nil, errors.New("unsafe skill directory")
	}
	aclErr := checkPrivateACL(file)
	file.Close()
	if aclErr != nil {
		child.Close()
		return nil, aclErr
	}
	return child, nil
}

// ensureRoot validates the skill root, creating it 0700 if missing, and
// returns it opened and anchored: every container ancestor up to "/" is
// validated safe from replacement by another principal, and the returned
// object is what Install operates on for the rest of the call.
func ensureRoot(root string) (*os.Root, anchor, error) {
	opened, identity, err := anchoredRoot(root, true)
	if err != nil {
		return nil, anchor{}, errors.New("invalid root")
	}
	info, err := opened.Stat(".")
	if err != nil || info.Mode().Perm()&0o022 != 0 || !ownedByUser(info) {
		opened.Close()
		return nil, anchor{}, errors.New("invalid root")
	}
	directory, err := opened.Open(".")
	if err != nil {
		opened.Close()
		return nil, anchor{}, errors.New("invalid root")
	}
	aclErr := checkPrivateACL(directory)
	directory.Close()
	if aclErr != nil {
		opened.Close()
		return nil, anchor{}, aclErr
	}
	return opened, identity, nil
}

// acquireInstallLock takes the skill-set lock inside root, the same anchored
// object the caller validated and holds open; it never reopens root by
// pathname.
func acquireInstallLock(directory *os.Root) (*os.File, string) {
	file, created, err := openInstallLock(directory)
	if err != nil {
		return nil, "recovery_required"
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, "skill_install_concurrent"
		}
		return nil, "recovery_required"
	}
	if !privateOpenRegular(file) || !lockStillAtPath(directory, file) {
		file.Close()
		return nil, "recovery_required"
	}
	if created {
		if _, err := file.WriteString(installLockWire); err != nil || file.Sync() != nil {
			file.Close()
			return nil, "recovery_required"
		}
		return file, ""
	}
	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return nil, "recovery_required"
	}
	wire, err := io.ReadAll(io.LimitReader(file, int64(len(installLockWire)+1)))
	if err != nil || string(wire) != installLockWire {
		file.Close()
		return nil, "recovery_required"
	}
	return file, ""
}

func openInstallLock(root *os.Root) (*os.File, bool, error) {
	file, err := root.OpenFile(installLockName, os.O_RDWR|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if err == nil {
		return file, true, nil
	}
	if !os.IsExist(err) {
		return nil, false, err
	}
	file, err = root.OpenFile(installLockName, os.O_RDWR|unix.O_NOFOLLOW, 0)
	return file, false, err
}

func lockStillAtPath(root *os.Root, file *os.File) bool {
	visible, err := root.Lstat(installLockName)
	if err != nil || visible.Mode()&os.ModeSymlink != 0 {
		return false
	}
	opened, err := file.Stat()
	return err == nil && os.SameFile(visible, opened)
}

func privateOpenRegular(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && privateRegularInfo(info) && checkPrivateACL(file) == nil
}

func privateRegularInfo(info os.FileInfo) bool {
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1 && stat.Uid == uint32(os.Getuid())
}

func ownedByUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}

// installOne installs or replaces name inside the anchored skill root
// parent: it opens (or creates) that one skill's directory once and performs
// every check and mutation on it (the "still matches" recheck, the replace,
// or the fresh write) through the same object, so a later replacement of the
// skill directory at its pathname cannot redirect any of them elsewhere.
// verify (the skill root's identity proof) and the skill directory's own
// identity at name are checked before each create or commit and again after
// it: a replacement seen first is ErrTargetReplaced with nothing committed;
// one seen only after the commit is errCommitUnconfirmed, never a change.
func (i integration) installOne(parent *os.Root, name string, content []byte, verify func() error) (bool, bool, error) {
	info, err := parent.Lstat(name)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, false, errors.New("skill conflict")
		}
		child, err := privateChild(parent, name)
		if err != nil {
			return false, false, errors.New("skill conflict")
		}
		defer child.Close()
		still := func() error {
			if err := verify(); err != nil {
				return err
			}
			return childStillAt(parent, child, name)
		}
		if matchesInstalledIn(child, content) {
			return false, false, still()
		}
		if !i.matchesLegacyInstalledIn(child, name) {
			return false, false, errors.New("skill conflict")
		}
		if err := i.replaceKnownSkillIn(child, name, content, still); err != nil {
			if errors.Is(err, ErrTargetReplaced) {
				return false, false, err
			}
			return false, false, errors.New("skill conflict")
		}
		if still() != nil {
			return false, false, errCommitUnconfirmed
		}
		return true, false, nil
	}
	if !os.IsNotExist(err) {
		return false, false, err
	}
	if err := verify(); err != nil {
		return false, false, err
	}
	if err := parent.Mkdir(name, 0o700); err != nil {
		return false, false, err
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		_ = parent.Remove(name)
		return false, false, err
	}
	defer child.Close()
	if err := errors.Join(verify(), childStillAt(parent, child, name)); err != nil {
		_ = parent.Remove(name)
		return false, false, ErrTargetReplaced
	}
	file, err := child.OpenFile("SKILL.md", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = parent.Remove(name)
		return false, false, err
	}
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = child.Remove("SKILL.md")
		_ = parent.Remove(name)
		return false, false, errors.New("skill write failed")
	}
	if errors.Join(verify(), childStillAt(parent, child, name)) != nil {
		return false, false, errCommitUnconfirmed
	}
	return true, true, nil
}

func (i integration) matchesLegacyInstalled(root, name string) bool {
	content, ok := singleSkillContent(filepath.Join(root, name))
	if !ok {
		return false
	}
	return i.knownDigest(name, digestOf(content))
}

// matchesInstalledIn, matchesLegacyInstalledIn, singleSkillContentIn, and
// replaceKnownSkillIn are the anchored-root counterparts of
// matchesInstalled/matchesLegacyInstalled/singleSkillContent/replaceKnownSkill,
// used by installOne so its "still matches" recheck and its replace happen
// through the same already-opened skill directory, never by reopening it.
func matchesInstalledIn(child *os.Root, expected []byte) bool {
	actual, ok := singleSkillContentIn(child)
	return ok && string(actual) == string(expected)
}

func (i integration) matchesLegacyInstalledIn(child *os.Root, name string) bool {
	content, ok := singleSkillContentIn(child)
	if !ok {
		return false
	}
	return i.knownDigest(name, digestOf(content))
}

func singleSkillContentIn(child *os.Root) ([]byte, bool) {
	directory, err := child.Open(".")
	if err != nil {
		return nil, false
	}
	entries, err := directory.ReadDir(-1)
	directory.Close()
	if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" || entries[0].Type()&os.ModeSymlink != 0 {
		return nil, false
	}
	return privateRegularFileIn(child, "SKILL.md")
}

func (i integration) replaceKnownSkillIn(child *os.Root, name string, content []byte, still func() error) error {
	expected, ok := privateRegularFileIn(child, "SKILL.md")
	if !ok || !i.knownDigest(name, digestOf(expected)) {
		return errors.New("skill changed during update")
	}
	if err := still(); err != nil {
		return err
	}
	const temporary = ".axiom-skill-update"
	file, err := child.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = child.Remove(temporary)
		return errors.New("skill update write failed")
	}
	defer child.Remove(temporary)
	current, ok := privateRegularFileIn(child, "SKILL.md")
	if !ok {
		return errors.New("skill changed during update")
	}
	if string(current) != string(expected) {
		return errors.New("skill changed during update")
	}
	if err := still(); err != nil {
		return err
	}
	return child.Rename(temporary, "SKILL.md")
}

func singleSkillContent(directory string) ([]byte, bool) {
	if !privateDirectory(directory) {
		return nil, false
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" || entries[0].Type()&os.ModeSymlink != 0 {
		return nil, false
	}
	path := filepath.Join(directory, "SKILL.md")
	if !privateRegularFile(path) {
		return nil, false
	}
	content, err := os.ReadFile(path)
	return content, err == nil
}

func matchesInstalled(root, name string, expected []byte) bool {
	directory := filepath.Join(root, name)
	if !privateDirectory(directory) {
		return false
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "SKILL.md" || entries[0].Type()&os.ModeSymlink != 0 {
		return false
	}
	path := filepath.Join(directory, "SKILL.md")
	if !privateRegularFile(path) {
		return false
	}
	actual, err := os.ReadFile(path)
	return err == nil && string(actual) == string(expected)
}
