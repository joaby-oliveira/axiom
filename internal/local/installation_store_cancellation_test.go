package local

import (
	"context"
	"github.com/rgomids/axiom/internal/testfs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestInstallationCancellationBeforePublicationLeavesNoRecord(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := privateTestRoot(t)
	store, err := NewInstallationStore(state)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store.beforePublication = cancel
	if got := store.Install(ctx, source); got.Status != InstallationFailed || got.Category != "cancelled" {
		t.Fatalf("cancelled install = %+v", got)
	}
	reopened := store.Reopen(context.Background(), source)
	if reopened.Category != "reopened_without_local_state" {
		t.Fatalf("reopen after cancelled install = %+v", reopened)
	}
	record := filepath.Join(state, "projects", "123e4567-e89b-42d3-a456-426614174000", "installation.json")
	if _, err := os.Lstat(record); !os.IsNotExist(err) {
		t.Fatalf("record after cancelled install = %v", err)
	}
}

func TestInstallationCancellationBeforeStagingLeavesNoRecord(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewInstallationStore(privateTestRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := store.Install(ctx, source); got.Status != InstallationFailed || got.Category != "cancelled" {
		t.Fatalf("cancelled install = %+v", got)
	}
}

func TestInstallationPublicationCollisionPreservesCompleteWinner(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := privateTestRoot(t)
	store, err := NewInstallationStore(state)
	if err != nil {
		t.Fatal(err)
	}
	store.beforePublication = func() {
		target := filepath.Join(state, "projects", "123e4567-e89b-42d3-a456-426614174000")
		stages, err := filepath.Glob(filepath.Join(target, ".lingo-install-*"))
		if err != nil || len(stages) != 1 {
			t.Fatalf("stages = %v, %v", stages, err)
		}
		wire, err := os.ReadFile(stages[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "installation.json"), wire, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result := store.Install(context.Background(), source)
	if result.Status != InstallationConflict || result.Category != "explicit_replacement_required" {
		t.Fatalf("publication collision = %+v", result)
	}
	if reopened := store.Reopen(context.Background(), source); reopened.Status != InstallationUnchanged || reopened.Category != "reopened_with_local_state" {
		t.Fatalf("winner = %+v", reopened)
	}
}

func TestInstallationRejectsTargetReplacementBeforePublication(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := privateTestRoot(t)
	store, err := NewInstallationStore(state)
	if err != nil {
		t.Fatal(err)
	}
	outside := privateTestRoot(t)
	target := filepath.Join(state, "projects", "123e4567-e89b-42d3-a456-426614174000")
	store.beforePublication = func() {
		if err := testfs.RenameOrSkipPinned(t, target, target+"-moved"); err != nil {
			t.Fatal(err)
		}
		if err := testfs.Symlink(t, outside, target); err != nil {
			t.Fatal(err)
		}
	}
	if result := store.Install(context.Background(), source); result.Status != InstallationFailed || result.Category != "storage_failure" {
		t.Fatalf("target replacement = %+v", result)
	}
	if _, err := os.Lstat(filepath.Join(outside, "installation.json")); !os.IsNotExist(err) {
		t.Fatalf("outside target changed: %v", err)
	}
}

func TestInstallationPostPublicationSyncFailureRequiresRecovery(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := privateTestRoot(t)
	store, err := NewInstallationStore(state)
	if err != nil {
		t.Fatal(err)
	}
	store.syncDirectory = func(*os.Root) error { return syscall.EIO }
	if result := store.Install(context.Background(), source); result.Status != InstallationFailed || result.Category != "recovery_required" {
		t.Fatalf("sync failure = %+v", result)
	}
	if result := store.Reopen(context.Background(), source); result.Status != InstallationFailed || result.Category != "recovery_required" {
		t.Fatalf("reopen after uncertain durability = %+v", result)
	}
	record := filepath.Join(state, "projects", "123e4567-e89b-42d3-a456-426614174000", "installation.json")
	if _, err := os.ReadFile(record); err != nil {
		t.Fatalf("published record unavailable: %v", err)
	}
}

func TestInstallationCleanupFailurePreservesCommittedRecordAndRequiresRecovery(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := privateTestRoot(t)
	store, err := NewInstallationStore(state)
	if err != nil {
		t.Fatal(err)
	}
	store.removeAttempt = func(*os.Root, string) error { return ErrRecoveryRequired }
	result := store.Install(context.Background(), source)
	if result.Status != InstallationFailed || result.Category != "recovery_required" {
		t.Fatalf("cleanup failure = %+v", result)
	}
	record := filepath.Join(state, "projects", "123e4567-e89b-42d3-a456-426614174000", "installation.json")
	if _, err := os.ReadFile(record); err != nil {
		t.Fatalf("committed record = %v", err)
	}
	if reopened := store.Reopen(context.Background(), source); reopened.Status != InstallationFailed || reopened.Category != "recovery_required" {
		t.Fatalf("reader after cleanup failure = %+v", reopened)
	}
}

func TestInstallationRestoresAttemptWhenRemovalSyncFailsAfterCommit(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := privateTestRoot(t)
	store, err := NewInstallationStore(state)
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	failed := false
	store.removeAttempt = func(root *os.Root, name string) error {
		err := root.Remove(name)
		if err == nil {
			removed = true
		}
		return err
	}
	store.syncDirectory = func(root *os.Root) error {
		if removed && !failed {
			failed = true
			return syscall.EIO
		}
		return syncRoot(root)
	}
	result := store.Install(context.Background(), source)
	if result.Status != InstallationFailed || result.Category != "recovery_required" || !removed || !failed {
		t.Fatalf("post-removal sync result = %+v", result)
	}
	record := filepath.Join(state, "projects", "123e4567-e89b-42d3-a456-426614174000", "installation.json")
	if _, err := os.ReadFile(record); err != nil {
		t.Fatalf("canonical record unavailable: %v", err)
	}
	if reopened := store.Reopen(context.Background(), source); reopened.Status != InstallationFailed || reopened.Category != "recovery_required" {
		t.Fatalf("reader after removal sync failure = %+v", reopened)
	}
}

func TestInstallationDiskFullBeforePublicationLeavesNoRecord(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := privateTestRoot(t)
	store, err := NewInstallationStore(state)
	if err != nil {
		t.Fatal(err)
	}
	store.writeFile = simulateDiskFull
	if result := store.Install(context.Background(), source); result.Status != InstallationFailed || result.Category != "storage_failure" {
		t.Fatalf("disk-full failure = %+v", result)
	}
	if result := store.Reopen(context.Background(), source); result.Category != "reopened_without_local_state" {
		t.Fatalf("reopen after disk-full = %+v", result)
	}
}

func TestInstallationRejectsStagedHardLinkBeforePublication(t *testing.T) {
	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := privateTestRoot(t)
	store, err := NewInstallationStore(state)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(privateTestRoot(t), "linked")
	store.beforePublication = func() {
		pattern := filepath.Join(state, "projects", "123e4567-e89b-42d3-a456-426614174000", ".lingo-install-*")
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) != 1 {
			t.Fatalf("install stage paths = %v, %v", matches, err)
		}
		if err := os.Link(matches[0], outside); err != nil {
			t.Fatal(err)
		}
	}
	if result := store.Install(context.Background(), source); result.Status != InstallationFailed || result.Category != "storage_failure" {
		t.Fatalf("hard-linked stage accepted: %+v", result)
	}
	if result := store.Reopen(context.Background(), source); result.Category != "reopened_without_local_state" {
		t.Fatalf("reopen after rejected stage = %+v", result)
	}
}
