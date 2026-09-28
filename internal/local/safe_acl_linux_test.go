//go:build linux

package local

import (
	"encoding/binary"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPrivateRootRejectsDefaultACLDespiteMode0700(t *testing.T) {
	root := privateTestRoot(t)
	// Linux POSIX ACL xattr v2: owner, named user, group, mask, other.
	// A default ACL can expose future files without changing this directory's mode.
	acl := make([]byte, 4+5*8)
	binary.LittleEndian.PutUint32(acl[:4], 2)
	entries := []struct {
		tag  uint16
		perm uint16
		id   uint32
	}{
		{1, 7, ^uint32(0)},
		{2, 4, 65534},
		{4, 0, ^uint32(0)},
		{16, 4, ^uint32(0)},
		{32, 0, ^uint32(0)},
	}
	for index, entry := range entries {
		position := 4 + index*8
		binary.LittleEndian.PutUint16(acl[position:], entry.tag)
		binary.LittleEndian.PutUint16(acl[position+2:], entry.perm)
		binary.LittleEndian.PutUint32(acl[position+4:], entry.id)
	}
	if err := unix.Setxattr(root, "system.posix_acl_default", acl, 0); err != nil {
		t.Fatalf("set synthetic default ACL: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("mode changed: %v, %v", info, err)
	}
	opened, err := privateRoot(root)
	if opened != nil {
		opened.Close()
	}
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("default ACL accepted: %v", err)
	}
	// The publication-directory rule for a 0755 user directory refuses
	// the same ACL.
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CheckPublicationDirectory(root); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("publication directory with ACL accepted: %v", err)
	}
}
