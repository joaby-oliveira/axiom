package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsPrivatePublicationAndLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	root, err := privateRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"file:stream", "trailing.", "CON", "COM¹", "CONOUT$"} {
		if safeEntryName(name) {
			t.Fatalf("unsafe entry accepted: %q", name)
		}
	}
	lock, err := lockDirectory(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if other, e := lockDirectory(root, true); e == nil {
		other.Close()
		t.Fatal("second writer acquired lock")
	}
	defer lock.Close()
	if err := writePrivateFile(root, "first", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplace(root, "first", "published"); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(root, "second", []byte("different")); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(root, "replacement", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := root.Rename("replacement", "published"); err != nil {
		t.Fatalf("replace while locked: %v", err)
	}
	if err := renameNoReplace(root, "second", "published"); err == nil {
		t.Fatal("replaced existing destination")
	}
	got, err := readPrivateFile(root, "published")
	if err != nil || string(got) != "payload" {
		t.Fatalf("read %q: %v", got, err)
	}
	if err := os.Link(filepath.Join(path, "published"), filepath.Join(path, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateFile(root, "published"); err == nil {
		t.Fatal("accepted hardlink")
	}
}

func windowsTestACL(t *testing.T, path, additional string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)" + additional)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsRejectsForeignAccessAndUnsafeAncestor(t *testing.T) {
	for _, access := range []string{"FR", "FW", "FA", "WD", "WO"} {
		t.Run(access, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private")
			root, err := privateRoot(path)
			if err != nil {
				t.Fatal(err)
			}
			root.Close()
			windowsTestACL(t, path, "(A;OICI;"+access+";;;WD)")
			if err := CheckPrivateDirectory(path); err == nil {
				t.Fatal("accepted foreign access")
			}
		})
	}
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	root, err := privateRoot(child)
	if err != nil {
		t.Fatal(err)
	}
	root.Close()
	windowsTestACL(t, parent, "(A;;0x40;;;WD)")
	if err := CheckPrivateDirectory(child); err == nil {
		t.Fatal("accepted replaceable ancestor")
	}
}

func TestWindowsRejectsJunctionAndAmbiguousPaths(t *testing.T) {
	parent := t.TempDir()
	target := t.TempDir()
	junction := filepath.Join(parent, "junction")
	command := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, target)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v %s", err, output)
	}
	if _, err := trustedCanonical(junction); err == nil {
		t.Fatal("accepted junction")
	}
	for _, path := range []string{`\\server\share\state`, `\\?\C:\state`, filepath.Join(parent, "file:stream"), filepath.Join(parent, "CON"), filepath.Join(parent, "trailing.")} {
		if _, err := trustedCanonical(path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}

func TestWindowsStateRootUsesLocalAppData(t *testing.T) {
	home := t.TempDir()
	appData := filepath.Join(t.TempDir(), "redirected")
	got, err := WindowsStateRoot(home, appData)
	if err != nil || got != filepath.Join(appData, "Axiom", "state") {
		t.Fatalf("state root: %s %v", got, err)
	}
}
