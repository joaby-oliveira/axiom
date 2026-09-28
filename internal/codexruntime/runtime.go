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
	"syscall"

	"golang.org/x/sys/unix"
)

//go:embed skills/*/SKILL.md
var skillFiles embed.FS

var skillNames = []string{
	"axiom-project-configure",
	"axiom-project-show",
	"axiom-work-item-create",
	"axiom-work-item-run",
	"axiom-work-item-status",
}

var legacySkillDigests = map[string][]string{
	"axiom-project-configure": {"d481dc61ecd7a9afd1ffd0a79908515f15f04a75002a7d501eea5517f1f4844e", "87dc55d4a459d4abf70bb53c7f91695da9cbae18da3a5afd6112f964062b5b9c", "b9d55306f7f4e1b33b7606c4327f94cf18dd12287536be7f27d61f8f9a95dc1e", "05d8e420f440529df3bd75a521f3d9493d5cefe3d9fc16ddb1da9ffeed553cd1", "d5271f6a24676c2f8111776cc797232784ad7e75318aca500f6ad73274510b60", "237da8ea6a57e9240ae464d85a1fd1ad2d8c4d19ba8160c24943ae4752b2885d"},
	"axiom-project-show":      {"a80b3038c497fe3f3b817e5d5d28bca68de96d9ceaf76630f08ad1e34c5db0f4", "594fc02985f5884c780b2c584c6774424bb63c5234ee2f32e5800002cd3c5c02", "d7f86666dd2036b53a4cdbe2d6b67d096936f59b164ae9573806fbb9a40d97fd", "2542254b45ef2c1ac67e09ae1d1924fd0648787836b9bbbe60480a6f09646bcc", "74abd548a0b352b9464efb2b1a6d5aca453bc1e88a164903d8a6043dfeebe8ab"},
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
	afterSkill          func(string)
}

func New(root string) (Service, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return Service{}, errors.New("unsafe Codex skill root")
	}
	return Service{root: filepath.Clean(root), binaryCompatibility: BinaryCompatibility}, nil
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
	if err := ensureRoot(s.root); err != nil {
		return Result{Status: Failed, Category: "codex_skill_root_unavailable"}
	}
	lock, category := acquireInstallLock(s.root)
	if category != "" {
		return s.inspectResult(Failed, category)
	}
	defer lock.Close()
	for _, name := range skillNames {
		content, err := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if err != nil {
			return Result{Status: Failed, Category: "codex_skill_package_invalid"}
		}
		if !installableOne(s.root, name, content) {
			return s.inspectResult(Failed, "codex_skill_conflict")
		}
	}
	changed := false
	for _, name := range skillNames {
		if err := ctx.Err(); err != nil {
			return s.inspectResult(Partial, "codex_skill_install_partial")
		}
		content, err := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if err != nil {
			return Result{Status: Failed, Category: "codex_skill_package_invalid"}
		}
		changedOne, createdOne, err := installOne(s.root, name, content)
		if err != nil {
			return s.inspectResult(Partial, "codex_skill_install_partial")
		}
		_ = createdOne
		if changedOne {
			changed = true
		}
		if s.afterSkill != nil {
			s.afterSkill(name)
		}
	}
	receipt, err := receiptBytes()
	if err != nil {
		return s.inspectResult(Partial, "codex_skill_receipt_incomplete")
	}
	receiptChanged, receiptPublished := publishReceipt(s.root, receipt)
	if !receiptPublished {
		return s.inspectResult(Partial, "codex_skill_receipt_incomplete")
	}
	changed = changed || receiptChanged
	if !changed {
		return s.inspectResult(Unchanged, "codex_already_configured")
	}
	return s.inspectResult(Applied, "codex_configured")
}

func installableOne(root, name string, content []byte) bool {
	info, err := os.Lstat(filepath.Join(root, name))
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false
	}
	if !privateDirectory(filepath.Join(root, name)) {
		return false
	}
	return matchesInstalled(root, name, content) || matchesLegacyInstalled(root, name)
}

func (s Service) Inspect(ctx context.Context) Result {
	if err := ctx.Err(); err != nil {
		return Result{Status: Failed, Category: "cancelled"}
	}
	_, err := os.Lstat(s.root)
	if os.IsNotExist(err) {
		return s.inspectResult(Missing, "codex_not_configured")
	}
	if err != nil || !privateDirectory(s.root) {
		return Result{Status: Failed, Category: "codex_skill_root_unavailable"}
	}
	if s.binaryCompatibility != BinaryCompatibility {
		return s.inspectResult(Incompatible, "codex_binary_skill_incompatible")
	}
	for _, name := range skillNames {
		content, readErr := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if readErr != nil || !matchesInstalled(s.root, name, content) {
			return s.inspectResult(Missing, "codex_skills_missing_or_changed")
		}
	}
	receipt, err := receiptBytes()
	if err != nil || !matchesPrivateFile(filepath.Join(s.root, receiptName), receipt) {
		return s.inspectResult(Partial, "codex_skill_receipt_incomplete")
	}
	return s.inspectResult(Ready, "codex_ready")
}

func (s Service) inspectResult(status Status, category string) Result {
	result := Result{Status: status, Category: category, SkillSetVersion: SkillSetVersion, BinaryCompatibility: s.binaryCompatibility, Skills: make([]SkillState, 0, len(skillNames))}
	for _, name := range skillNames {
		content, _ := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		digest := sha256.Sum256(content)
		state := "missing"
		if matchesInstalled(s.root, name, content) {
			state = "equivalent"
		} else if matchesLegacyInstalled(s.root, name) {
			state = "owned_older"
		} else if _, err := os.Lstat(filepath.Join(s.root, name)); err == nil {
			state = "modified_or_foreign"
		}
		result.Skills = append(result.Skills, SkillState{Name: name, Digest: hex.EncodeToString(digest[:]), State: state})
	}
	return result
}

func publishReceipt(root string, content []byte) (bool, bool) {
	path := filepath.Join(root, receiptName)
	if matchesPrivateFile(path, content) {
		return false, true
	}
	if _, err := os.Lstat(path); err == nil {
		if !matchesLegacyReceipt(path) {
			return false, false
		}
		return replaceKnownReceipt(root, content)
	} else if !os.IsNotExist(err) {
		return false, false
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return false, false
	}
	defer directory.Close()
	const temporary = ".axiom-skill-set-receipt-stage"
	file, err := directory.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, false
	}
	defer directory.Remove(temporary)
	written, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || written != len(content) {
		return false, false
	}
	if _, err := directory.Stat(receiptName); err == nil || !os.IsNotExist(err) {
		return false, false
	}
	if err := directory.Rename(temporary, receiptName); err != nil {
		return false, false
	}
	return true, true
}

func matchesLegacyReceipt(path string) bool {
	for _, wire := range legacyReceiptWires {
		if matchesPrivateFile(path, wire) {
			return true
		}
	}
	return false
}

func replaceKnownReceipt(root string, content []byte) (bool, bool) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return false, false
	}
	defer directory.Close()
	const temporary = ".axiom-skill-set-receipt-stage"
	file, err := directory.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, false
	}
	defer directory.Remove(temporary)
	written, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || written != len(content) {
		return false, false
	}
	if !matchesLegacyReceipt(filepath.Join(root, receiptName)) {
		return false, false
	}
	if err := directory.Rename(temporary, receiptName); err != nil {
		return false, false
	}
	return true, true
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

func privateDirectory(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	directory, err := os.OpenFile(path, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false
	}
	defer directory.Close()
	opened, err := directory.Stat()
	if err != nil || !os.SameFile(info, opened) || !ownedByUser(opened) {
		return false
	}
	return checkPrivateACL(directory) == nil
}

func ensureRoot(root string) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
		if !privateDirectory(root) {
			return errors.New("unsafe created root")
		}
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !privateDirectory(root) {
		return errors.New("invalid root")
	}
	return nil
}

func acquireInstallLock(root string) (*os.File, string) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, "recovery_required"
	}
	defer directory.Close()
	file, created, err := openInstallLock(directory)
	if err != nil {
		return nil, "recovery_required"
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, "codex_skill_install_concurrent"
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

func installOne(root, name string, content []byte) (bool, bool, error) {
	directory := filepath.Join(root, name)
	info, err := os.Lstat(directory)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, false, errors.New("skill conflict")
		}
		if matchesInstalled(root, name, content) {
			return false, false, nil
		}
		if !matchesLegacyInstalled(root, name) || replaceKnownSkill(directory, name, content) != nil {
			return false, false, errors.New("skill conflict")
		}
		return true, false, nil
	}
	if !os.IsNotExist(err) {
		return false, false, err
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return false, false, err
	}
	file, err := os.OpenFile(filepath.Join(directory, "SKILL.md"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = os.Remove(directory)
		return false, false, err
	}
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.RemoveAll(directory)
		return false, false, errors.New("skill write failed")
	}
	return true, true, nil
}

func matchesLegacyInstalled(root, name string) bool {
	content, ok := singleSkillContent(filepath.Join(root, name))
	if !ok {
		return false
	}
	digest := sha256.Sum256(content)
	value := hex.EncodeToString(digest[:])
	for _, known := range legacySkillDigests[name] {
		if value == known {
			return true
		}
	}
	return false
}

func replaceKnownSkill(directory, name string, content []byte) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	const temporary = ".axiom-skill-update"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = root.Remove(temporary)
		return errors.New("skill update write failed")
	}
	defer root.Remove(temporary)
	current, err := root.ReadFile("SKILL.md")
	if err != nil {
		return err
	}
	digest := sha256.Sum256(current)
	known := false
	for _, expected := range legacySkillDigests[name] {
		if hex.EncodeToString(digest[:]) == expected {
			known = true
		}
	}
	if !known {
		return errors.New("skill changed during update")
	}
	return root.Rename(temporary, "SKILL.md")
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
