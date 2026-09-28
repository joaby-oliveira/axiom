//go:build darwin

package local

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrivateRootRejectsPermissiveACLDespiteMode0700(t *testing.T) {
	root := privateTestRoot(t)
	command := exec.Command("/bin/chmod", "+a", "everyone allow read,search", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("set synthetic ACL: %v: %s", err, output)
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
		t.Fatalf("permissive ACL accepted: %v", err)
	}
}

func TestPrivateFileRejectsPermissiveACL(t *testing.T) {
	root := privateTestRoot(t)
	file := filepath.Join(root, "record")
	if err := os.WriteFile(file, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/chmod", "+a", "everyone allow read", file)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("set synthetic file ACL: %v: %s", err, output)
	}
	opened, err := privateRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if _, err := readPrivateFile(opened, "record"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("permissive file ACL accepted: %v", err)
	}
}

func TestPublicationDirectoryRejectsACLDespiteSafeMode(t *testing.T) {
	directory := filepath.Join(privateTestRoot(t), "bin")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/chmod", "+a", "everyone allow add_file,delete_child", directory)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("set synthetic ACL: %v: %s", err, output)
	}
	if info, err := os.Stat(directory); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode changed: %v, %v", info, err)
	}
	if err := CheckPublicationDirectory(directory); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("mutation ACL accepted: %v", err)
	}
}
