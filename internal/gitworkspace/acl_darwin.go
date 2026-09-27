//go:build darwin && cgo

package gitworkspace

/*
#include <sys/types.h>
#include <sys/acl.h>
#include <errno.h>

static int axiom_gitworkspace_extended_acl_present(int fd) {
	errno = 0;
	acl_t acl = acl_get_fd_np(fd, ACL_TYPE_EXTENDED);
	if (acl == NULL) {
		return errno == ENOENT ? 0 : -1;
	}
	acl_free(acl);
	return 1;
}
*/
import "C"

import (
	"errors"
	"os"
)

func checkPrivateACL(file *os.File) error {
	if C.axiom_gitworkspace_extended_acl_present(C.int(file.Fd())) != 0 {
		return errors.New("extended ACL present or unavailable")
	}
	return nil
}
