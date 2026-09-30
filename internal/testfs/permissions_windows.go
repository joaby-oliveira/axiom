package testfs

import (
	"errors"
	"os"

	"github.com/rgomids/axiom/internal/windowsfs"
	"golang.org/x/sys/windows"
)

func symlinkPrivilegeMissing(err error) bool { return errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) }
func pinnedDirectory(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

// SharedMode gives Everyone the corresponding read/write access, rather than
// relying on Windows chmod (which only changes the read-only attribute).
func SharedMode(path string, mode os.FileMode) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	extra := ""
	if mode&0o044 != 0 {
		extra += "(A;OICI;FR;;;WD)"
	}
	if mode&0o022 != 0 {
		extra += "(A;OICI;FW;;;WD)"
	}
	if mode&0o011 != 0 {
		extra += "(A;OICI;FX;;;WD)"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)" + extra)
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func PrivateMode(path string, _ os.FileMode) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return windowsfs.Check(f, true) == nil
}
