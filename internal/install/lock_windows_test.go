package install

import (
	"github.com/rgomids/axiom/internal/windowsfs"
	"os"
)

func holdSkillLock(file *os.File) error { return windowsfs.LockFile(file) }
