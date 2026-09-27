//go:build linux

package gitworkspace

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func checkPrivateACL(file *os.File) error {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		_, err := unix.Fgetxattr(int(file.Fd()), name, nil)
		if err == nil {
			return errors.New("extended ACL present")
		}
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.EOPNOTSUPP) {
			continue
		}
		return errors.New("ACL state unavailable")
	}
	return nil
}
