//go:build !windows

package install

import (
	"os"
	"syscall"
)

func holdSkillLock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
