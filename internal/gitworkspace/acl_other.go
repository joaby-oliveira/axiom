//go:build !darwin && !linux

package gitworkspace

import (
	"errors"
	"os"
)

func checkPrivateACL(*os.File) error {
	return errors.New("ACL inspection unavailable")
}
