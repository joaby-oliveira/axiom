// Package windowsfs implements the Windows filesystem security boundary.
// Handles, rather than path strings, are the authority for ownership and ACL checks.
package windowsfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var ErrUnsafe = errors.New("unsafe Windows filesystem object")

const fileDeleteChild = 0x0040

func privileged(sid *windows.SID) bool {
	return sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) || sid.String() == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
}

// Check requires a current-user owner for private objects. Administrators and
// SYSTEM remain trusted, like root on POSIX. Unknown ACE types fail closed.
// Ancestors may grant read/create access, but never replacement of children.
func Check(file *os.File, private bool) error {
	h := windows.Handle(file.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrUnsafe
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 && info.NumberOfLinks != 1 {
		return ErrUnsafe
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil || !sd.IsValid() {
		return ErrUnsafe
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || (!owner.Equals(user.User.Sid) && (private || !privileged(owner))) {
		return ErrUnsafe
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return ErrUnsafe
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil {
			return ErrUnsafe
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrUnsafe
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsValid() {
			return ErrUnsafe
		}
		if sid.Equals(user.User.Sid) || privileged(sid) {
			continue
		}
		mask := uint32(windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | fileDeleteChild | windows.GENERIC_WRITE | windows.GENERIC_ALL)
		if private {
			mask |= windows.FILE_READ_DATA | windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_READ_EA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES | windows.FILE_EXECUTE | windows.GENERIC_READ | windows.GENERIC_EXECUTE
		}
		if uint32(ace.Mask)&mask != 0 {
			return ErrUnsafe
		}
	}
	return nil
}

// Canonical rejects namespaces, UNC/network paths, alternate streams, ambiguous
// Win32 names and all reparse points, including junctions and mount points.
func Canonical(path string) (string, error) {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(clean) || strings.HasPrefix(clean, `\\`) {
		return "", ErrUnsafe
	}
	current := volume + `\`
	volumePath, err := windows.UTF16PtrFromString(current)
	if err != nil {
		return "", err
	}
	var filesystem [32]uint16
	if windows.GetDriveType(volumePath) != windows.DRIVE_FIXED {
		return "", ErrUnsafe
	}
	if err := windows.GetVolumeInformation(volumePath, nil, 0, nil, nil, nil, &filesystem[0], uint32(len(filesystem))); err != nil {
		return "", err
	}
	if windows.UTF16ToString(filesystem[:]) != "NTFS" {
		return "", ErrUnsafe
	}
	for _, part := range strings.Split(strings.TrimPrefix(clean, current), `\`) {
		if part == "" {
			continue
		}
		if strings.ContainsAny(part, ":<>\"|?*\x00") || strings.TrimRight(part, " .") != part {
			return "", ErrUnsafe
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
			return "", ErrUnsafe
		}
		current = filepath.Join(current, part)
		name, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return "", err
		}
		attrs, err := windows.GetFileAttributes(name)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			continue
		}
		if err != nil || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return "", ErrUnsafe
		}
	}
	return clean, nil
}

func Identity(file *os.File) (string, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return "", err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", ErrUnsafe
	}
	return fmt.Sprintf("%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}

func Available(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	err = windows.GetDiskFreeSpaceEx(p, &available, nil, nil)
	return available, err
}

// LockDirectory uses Windows share-mode exclusion on the directory itself, so
// inspecting state never needs to create a sidecar file. Closing releases it.
// LockDirectory serializes readers and writers through a kernel object whose
// lifetime is exactly the returned handle's lifetime. No persistent lock file or
// thread-affine mutex ownership is involved. Global scope covers other sessions.
func LockDirectory(root *os.Root, exclusive bool) (*os.File, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	if err := Check(dir, true); err != nil {
		return nil, err
	}
	id, err := Identity(dir)
	if err != nil {
		return nil, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString("Global\\Axiom.Directory." + user.User.Sid.String() + "." + id)
	if err != nil {
		return nil, err
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateMutex(&attrs, false, name)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return nil, windows.ERROR_LOCK_VIOLATION
		}
		return nil, err
	}
	return os.NewFile(uintptr(h), "axiom-directory-lock"), nil
}

func LockFile(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

// Mkdir creates first, then restricts only the directory created by this call.
// Existing directories are never silently re-permissioned or adopted.
func Mkdir(root *os.Root, name string) error {
	if err := root.Mkdir(name, 0o700); err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(filepath.Join(root.Name(), name))
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.WRITE_DAC|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), name)
	defer f.Close()
	expected, e1 := root.Lstat(name)
	actual, e2 := f.Stat()
	if e1 != nil || e2 != nil || !os.SameFile(expected, actual) || expected.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafe
	}
	return windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
