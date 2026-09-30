package local

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func renameNoReplace(root *os.Root, oldName, newName string) error {
	if !safeEntryName(oldName) || !safeEntryName(newName) {
		return ErrUnsafe
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	path, err := windows.UTF16PtrFromString(filepath.Join(root.Name(), oldName))
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(path, windows.DELETE|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), oldName)
	defer f.Close()
	expected, e1 := root.Lstat(oldName)
	actual, e2 := f.Stat()
	if e1 != nil || e2 != nil || expected.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected, actual) {
		return ErrUnsafe
	}
	name, err := windows.UTF16FromString(newName)
	if err != nil {
		return err
	}
	name = name[:len(name)-1]
	type renameInfo struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
		Name    [1]uint16
	}
	var shape renameInfo
	size := int(unsafe.Offsetof(shape.Name)) + len(name)*2
	buf := make([]byte, size)
	info := (*renameInfo)(unsafe.Pointer(&buf[0]))
	info.Root = windows.Handle(dir.Fd())
	info.Length = uint32(len(name) * 2)
	copy(unsafe.Slice(&info.Name[0], len(name)), name)
	var status windows.IO_STATUS_BLOCK
	err = windows.NtSetInformationFile(h, &status, &buf[0], uint32(size), windows.FileRenameInformation)
	if status, ok := err.(windows.NTStatus); ok {
		return status.Errno()
	}
	return err
}
