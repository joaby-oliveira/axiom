package install

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/compatibility"
	"github.com/rgomids/axiom/internal/local"
)

func windowsCandidate(t *testing.T, version string) Candidate {
	t.Helper()
	b := newBundle(version, []byte("Windows binary "+version))
	metadata := string(b.contents()["release-metadata.txt"])
	metadata = strings.NewReplacer("platform=macos-27", "platform=windows", "goos=darwin", "goos=windows", "architecture=arm64", "architecture=amd64").Replace(metadata)
	b.files["release-metadata.txt"] = []byte(metadata)
	archive, sums := b.write(t, t.TempDir())
	candidate, err := LoadCandidate(archive, sums)
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func windowsTarget(t *testing.T) Target {
	base := t.TempDir()
	return Target{BinaryDir: filepath.Join(base, "bin with spaces"), ReceiptDir: filepath.Join(base, "receipt"), State: compatibility.Roots{Projects: filepath.Join(base, "projects"), State: filepath.Join(base, "state")}}
}

func TestWindowsFreshInstallReinstallAndUpgrade(t *testing.T) {
	target := windowsTarget(t)
	first := windowsCandidate(t, "1.0.0")
	if status, err := InstallRelease(context.Background(), target, first); err != nil || status != "installed" {
		t.Fatalf("fresh: %s %v", status, err)
	}
	before, err := os.ReadFile(filepath.Join(target.ReceiptDir, receiptName))
	if err != nil {
		t.Fatal(err)
	}
	if status, err := InstallRelease(context.Background(), target, first); err != nil || status != "unchanged" {
		t.Fatalf("repeat: %s %v", status, err)
	}
	next := windowsCandidate(t, "1.1.0")
	if status, err := InstallRelease(context.Background(), target, next); err != nil {
		t.Fatalf("upgrade: %s %v", status, err)
	}
	after, err := os.ReadFile(filepath.Join(target.ReceiptDir, receiptName))
	if err != nil {
		t.Fatal(err)
	}
	oldFields, _ := parseReceipt(before)
	newFields, err := parseReceipt(after)
	if err != nil || oldFields["installedAt"] != newFields["installedAt"] || newFields["version"] != "1.1.0" {
		t.Fatalf("receipt: %v %v", newFields, err)
	}
	wire, err := os.ReadFile(filepath.Join(target.BinaryDir, binaryName))
	if err != nil || !bytes.Equal(wire, next.Binary) {
		t.Fatalf("binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target.ReceiptDir, markerName)); !os.IsNotExist(err) {
		t.Fatalf("marker remains: %v", err)
	}
}

func TestWindowsFreshInstallPreservesForeignBinary(t *testing.T) {
	target := windowsTarget(t)
	root, err := local.CreateOwnedDirectory(target.BinaryDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.CreateExclusive(binaryName, []byte("foreign"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallRelease(context.Background(), target, windowsCandidate(t, "1.0.0")); category(err) != "foreign_binary_preserved" {
		t.Fatalf("foreign: %v", err)
	}
	wire, _ := root.ReadFile(binaryName, 100)
	if string(wire) != "foreign" {
		t.Fatal("foreign binary changed")
	}
}

func TestWindowsFreshInstallFinalizesExactInterruptedReceipt(t *testing.T) {
	target := windowsTarget(t)
	candidate := windowsCandidate(t, "1.0.0")
	if _, err := InstallRelease(context.Background(), target, candidate); err != nil {
		t.Fatal(err)
	}
	root, err := local.OpenOwnedDirectory(target.ReceiptDir)
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("formatVersion=1\nstage=prepare\narchiveSha256=" + candidate.ArchiveSHA256 + "\n")
	if err := root.CreateExclusive(markerName, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	root.Close()
	if status, err := InstallRelease(context.Background(), target, candidate); err != nil || status != "installed" {
		t.Fatalf("resume: %s %v", status, err)
	}
}
