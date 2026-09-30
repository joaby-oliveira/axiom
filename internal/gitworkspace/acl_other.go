//go:build !darwin && !linux && !windows

package gitworkspace

import (
	"errors"
	"os"
)

func checkPrivateACL(*os.File) error {
	return errors.New("ACL inspection unavailable")
}
