//go:build darwin && !cgo

package gitworkspace

import (
	"os"

	"github.com/rgomids/axiom/internal/darwinacl"
)

func checkPrivateACL(file *os.File) error {
	return darwinacl.CheckPrivate(file)
}
