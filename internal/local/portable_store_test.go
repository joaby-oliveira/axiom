package local

import (
	"context"
	"errors"
	"fmt"
	"github.com/rgomids/axiom/internal/testfs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func privateTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPortableStoreRejectsSymlinkProjectDirectory(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := testfs.Symlink(t, outside, filepath.Join(root, "sample")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("symlink read error = %v", err)
	}
}

func TestPortableStoreRejectsExtraArtifactsBeforeUpdate(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "sample")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, manifestName), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "unknown"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), "sample", []byte("old"), []byte("new")); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("update error = %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(target, manifestName)); err != nil || string(body) != "old" {
		t.Fatalf("unsafe update changed manifest: %q, %v", body, err)
	}
}

func TestPortableStoreRejectsUserSymlinkAncestor(t *testing.T) {
	outside := t.TempDir()
	link := filepath.Join(t.TempDir(), "redirect")
	if err := testfs.Symlink(t, outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPortableStore(filepath.Join(link, "projects")); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("symlink ancestor accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "projects")); !os.IsNotExist(err) {
		t.Fatalf("outside tree changed: %v", err)
	}
}

func TestPortableStoreRejectsHardLinkedManifest(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sample"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(root, "sample", manifestName)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("hard-linked manifest accepted: %v", err)
	}
}

func TestPortableStoreReportsInterruptedUpdate(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "sample")
	if err := os.Mkdir(projectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, manifestName), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".lingo-manifest-interrupted"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("interrupted update = %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(projectPath, manifestName)); err != nil || string(data) != "old" {
		t.Fatalf("authoritative manifest changed: %q, %v", data, err)
	}
}

func TestPortableStoreReportsInterruptedCreate(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".lingo-stage-sample-interrupted"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("interrupted create = %v", err)
	}
}

func TestPortableUpdatePermissionFailurePreservesOldBytes(t *testing.T) {
	testfs.POSIXModes(t)
	if os.Geteuid() == 0 {
		t.Skip("permission denial requires an unprivileged account")
	}
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(root, "sample")
	if err := os.Chmod(projectPath, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(projectPath, 0o700)
	if err := store.Update(context.Background(), "sample", []byte("old"), []byte("new")); err == nil {
		t.Fatal("read-only project directory accepted update")
	}
	if data, err := os.ReadFile(filepath.Join(projectPath, manifestName)); err != nil || string(data) != "old" {
		t.Fatalf("prior bytes changed: %q, %v", data, err)
	}
}

func TestPortableUpdateCancellationBeforePublicationPreservesOldBytes(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store.beforeUpdatePublication = cancel
	if err := store.Update(ctx, "sample", []byte("old"), []byte("new")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled update = %v", err)
	}
	data, err := store.Read(context.Background(), "sample")
	if err != nil || string(data) != "old" {
		t.Fatalf("authoritative manifest after cancellation = %q, %v", data, err)
	}
}

func TestPortableCancellationBeforeStagingPreservesOldBytes(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Update(ctx, "sample", []byte("old"), []byte("new")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled update = %v", err)
	}
	if body, err := store.Read(context.Background(), "sample"); err != nil || string(body) != "old" {
		t.Fatalf("prior bytes = %q, %v", body, err)
	}
}

func TestPortableConflictingWritersHaveOneWinner(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, next := range []string{"first", "second"} {
		workers.Add(1)
		go func(next string) {
			defer workers.Done()
			<-start
			results <- store.Update(context.Background(), "sample", []byte("old"), []byte(next))
		}(next)
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflict := 0, 0
	for result := range results {
		switch {
		case result == nil:
			success++
		case errors.Is(result, ErrConflict):
			conflict++
		default:
			t.Fatalf("unexpected writer result: %v", result)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("writer results: success=%d conflict=%d", success, conflict)
	}
	actual, err := store.Read(context.Background(), "sample")
	if err != nil || string(actual) != "first" && string(actual) != "second" {
		t.Fatalf("winner bytes = %q, %v", actual, err)
	}
}

func TestPortableCreateRejectsAncestorReplacementBeforePublication(t *testing.T) {
	parent := privateTestRoot(t)
	root := filepath.Join(parent, "projects")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.beforeCreatePublication = func() {
		if err := testfs.RenameOrSkipPinned(t, root, filepath.Join(parent, "moved")); err != nil {
			t.Fatal(err)
		}
		if err := testfs.Symlink(t, outside, root); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Create(context.Background(), "sample", []byte("manifest")); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("ancestor replacement = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "sample")); !os.IsNotExist(err) {
		t.Fatalf("outside target changed: %v", err)
	}
}

func TestPortableUpdateRejectsProjectRenameBeforePublication(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
		t.Fatal(err)
	}
	outside := privateTestRoot(t)
	store.beforeUpdatePublication = func() {
		if err := testfs.RenameOrSkipPinned(t, filepath.Join(root, "sample"), filepath.Join(root, "moved")); err != nil {
			t.Fatal(err)
		}
		if err := testfs.Symlink(t, outside, filepath.Join(root, "sample")); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Update(context.Background(), "sample", []byte("old"), []byte("new")); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("project replacement = %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(root, "moved", manifestName)); err != nil || string(body) != "old" {
		t.Fatalf("prior bytes = %q, %v", body, err)
	}
	if _, err := os.Lstat(filepath.Join(outside, manifestName)); !os.IsNotExist(err) {
		t.Fatalf("outside target changed: %v", err)
	}
}

func TestPortablePostPublicationSyncFailureRequiresRecovery(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			root := privateTestRoot(t)
			store, err := NewPortableStore(root)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "update" {
				if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
					t.Fatal(err)
				}
			}
			committed := false
			store.syncDirectory = func(directory *os.Root) error {
				if committed {
					return syscall.EIO
				}
				return syncRoot(directory)
			}
			if operation == "create" {
				store.afterCreatePublication = func() { committed = true }
			} else {
				store.afterUpdatePublication = func() { committed = true }
			}
			if operation == "create" {
				err = store.Create(context.Background(), "sample", []byte("new"))
			} else {
				err = store.Update(context.Background(), "sample", []byte("old"), []byte("new"))
			}
			if !errors.Is(err, ErrRecoveryRequired) {
				t.Fatalf("sync failure = %v", err)
			}
			if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrRecoveryRequired) {
				t.Fatalf("read after uncertain durability = %v", err)
			}
			actual, err := os.ReadFile(filepath.Join(root, "sample", manifestName))
			if err != nil || string(actual) != "new" {
				t.Fatalf("published bytes = %q, %v", actual, err)
			}
		})
	}
}

func TestPortableRecoveryMarkerIdentifiesPriorAndNewGeneration(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			root := privateTestRoot(t)
			store, err := NewPortableStore(root)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "update" {
				if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			syncCalls := 0
			cancelAt := 2
			if operation == "update" {
				cancelAt = 1
			}
			store.syncDirectory = func(directory *os.Root) error {
				syncCalls++
				err := syncRoot(directory)
				if syncCalls == cancelAt {
					cancel()
				}
				return err
			}
			store.removeAttempt = func(directory *os.Root, name string) error {
				if strings.HasPrefix(name, ".axiom-recovery-") {
					return syscall.EIO
				}
				return directory.Remove(name)
			}
			if operation == "create" {
				err = store.Create(ctx, "sample", []byte("new"))
			} else {
				err = store.Update(ctx, "sample", []byte("old"), []byte("new"))
			}
			var publication *PublicationError
			if !errors.As(err, &publication) || publication.Committed || !errors.Is(err, ErrRecoveryRequired) {
				t.Fatalf("interrupted %s = %v", operation, err)
			}
			if _, readErr := store.Read(context.Background(), "sample"); !errors.Is(readErr, ErrRecoveryRequired) {
				t.Fatalf("reader after %s interruption = %v", operation, readErr)
			}

			markerPath := root
			if operation == "update" {
				markerPath = filepath.Join(root, "sample")
			}
			markerRoot, err := os.OpenRoot(markerPath)
			if err != nil {
				t.Fatal(err)
			}
			defer markerRoot.Close()
			markerName := recoveryMarkerName(t, markerPath)
			marker, err := readProtocolMarker(markerRoot, markerName)
			if err != nil {
				t.Fatal(err)
			}
			if marker.Object == "" || marker.OperationID == "" || marker.Staging == "" || marker.Stage != FaultF3 || marker.CommitProtocol != "rename" || !marker.NewPresent {
				t.Fatalf("incomplete marker: %#v", marker)
			}
			if marker.PriorPresent != (operation == "update") || marker.NewRevision != fmt.Sprintf("%x", digestBytes([]byte("new"))) {
				t.Fatalf("generation marker: %#v", marker)
			}
			wantPrior := fmt.Sprintf("%x", [32]byte{})
			if operation == "update" {
				wantPrior = fmt.Sprintf("%x", digestBytes([]byte("old")))
			}
			if marker.PriorRevision != wantPrior {
				t.Fatalf("prior revision = %s want %s", marker.PriorRevision, wantPrior)
			}
		})
	}
}

func recoveryMarkerName(t *testing.T, path string) string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".axiom-recovery-") {
			return entry.Name()
		}
	}
	t.Fatal("recovery marker missing")
	return ""
}

func TestPortableCleanupFailurePreservesCommittedBytesAndRequiresRecovery(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.removeAttempt = func(*os.Root, string) error { return ErrRecoveryRequired }
	if err := store.Create(context.Background(), "sample", []byte("new")); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("cleanup failure = %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(root, "sample", manifestName)); err != nil || string(body) != "new" {
		t.Fatalf("committed bytes = %q, %v", body, err)
	}
	if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("reader after cleanup failure = %v", err)
	}
}

func TestPortableCreateRestoresMarkerWhenRemovalSyncFailsAfterCommit(t *testing.T) {
	root := privateTestRoot(t)
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(root, "owned-sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed := false
	failed := false
	store.removeAttempt = func(directory *os.Root, name string) error {
		err := directory.Remove(name)
		if err == nil && strings.HasPrefix(name, ".axiom-recovery-") {
			removed = true
		}
		return err
	}
	store.syncDirectory = func(directory *os.Root) error {
		if removed && !failed {
			failed = true
			return syscall.EIO
		}
		return syncRoot(directory)
	}
	err = store.Create(context.Background(), "sample", []byte("new"))
	var publication *PublicationError
	if !errors.As(err, &publication) || !publication.Committed || !errors.Is(err, ErrRecoveryRequired) || !removed || !failed {
		t.Fatalf("post-removal sync result = %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(root, "sample", manifestName)); err != nil || string(body) != "new" {
		t.Fatalf("canonical bytes = %q, %v", body, err)
	}
	if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("reader after removal sync failure = %v", err)
	}
	if content, err := os.ReadFile(sentinel); err != nil || string(content) != "unchanged" {
		t.Fatalf("non-protocol object changed = %q, %v", content, err)
	}
}

func TestPortableManualRecoveryPreservesEvidenceAndReopens(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(map[bool]string{false: "before publication", true: "after publication"}[published], func(t *testing.T) {
			root := privateTestRoot(t)
			store, err := NewPortableStore(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
				t.Fatal(err)
			}
			projectPath := filepath.Join(root, "sample")
			marker := filepath.Join(projectPath, ".lingo-attempt-update-synthetic")
			if err := os.WriteFile(marker, []byte("pending\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			want := "old"
			if published {
				if err := os.WriteFile(filepath.Join(projectPath, manifestName), []byte("new"), 0o600); err != nil {
					t.Fatal(err)
				}
				want = "new"
			} else {
				if err := os.WriteFile(filepath.Join(projectPath, ".lingo-manifest-synthetic"), []byte("new"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrRecoveryRequired) {
				t.Fatalf("interrupted read = %v", err)
			}
			quarantine := privateTestRoot(t)
			for _, name := range []string{".lingo-attempt-update-synthetic", ".lingo-manifest-synthetic"} {
				source := filepath.Join(projectPath, name)
				if _, err := os.Lstat(source); os.IsNotExist(err) {
					continue
				} else if err != nil {
					t.Fatal(err)
				}
				if err := testfs.RenameOrSkipPinned(t, source, filepath.Join(quarantine, name)); err != nil {
					t.Fatal(err)
				}
			}
			if data, err := os.ReadFile(filepath.Join(quarantine, ".lingo-attempt-update-synthetic")); err != nil || string(data) != "pending\n" {
				t.Fatalf("quarantined Evidence = %q, %v", data, err)
			}
			actual, err := store.Read(context.Background(), "sample")
			if err != nil || string(actual) != want {
				t.Fatalf("reopened after operator quarantine = %q, %v", actual, err)
			}
		})
	}
}

func TestPortableDiskFullBeforePublicationPreservesPriorState(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			root := privateTestRoot(t)
			store, err := NewPortableStore(root)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "update" {
				if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
					t.Fatal(err)
				}
			}
			store.writeFile = simulateDiskFull
			if operation == "create" {
				err = store.Create(context.Background(), "sample", []byte("new"))
			} else {
				err = store.Update(context.Background(), "sample", []byte("old"), []byte("new"))
			}
			if !errors.Is(err, syscall.ENOSPC) {
				t.Fatalf("disk-full failure = %v", err)
			}
			actual, err := store.Read(context.Background(), "sample")
			if operation == "create" && !errors.Is(err, ErrNotFound) {
				t.Fatalf("unpublished create read = %q, %v", actual, err)
			}
			if operation == "update" && (err != nil || string(actual) != "old") {
				t.Fatalf("prior manifest = %q, %v", actual, err)
			}
		})
	}
}

func TestPortableRejectsStagedHardLinkBeforePublication(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			root := privateTestRoot(t)
			store, err := NewPortableStore(root)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "update" {
				if err := store.Create(context.Background(), "sample", []byte("old")); err != nil {
					t.Fatal(err)
				}
			}
			outside := filepath.Join(privateTestRoot(t), "linked")
			linkStage := func(pattern string) {
				matches, err := filepath.Glob(pattern)
				if err != nil || len(matches) != 1 {
					t.Fatalf("stage paths = %v, %v", matches, err)
				}
				if err := os.Link(matches[0], outside); err != nil {
					t.Fatal(err)
				}
			}
			if operation == "create" {
				store.beforeCreatePublication = func() { linkStage(filepath.Join(root, ".lingo-stage-sample-*", manifestName)) }
				err = store.Create(context.Background(), "sample", []byte("new"))
			} else {
				store.beforeUpdatePublication = func() { linkStage(filepath.Join(root, "sample", ".lingo-manifest-*")) }
				err = store.Update(context.Background(), "sample", []byte("old"), []byte("new"))
			}
			if !errors.Is(err, ErrUnsafe) {
				t.Fatalf("hard-linked stage accepted: %v", err)
			}
			if operation == "create" {
				if _, err := store.Read(context.Background(), "sample"); !errors.Is(err, ErrNotFound) {
					t.Fatalf("unpublished create read = %v", err)
				}
			} else if actual, err := store.Read(context.Background(), "sample"); err != nil || string(actual) != "old" {
				t.Fatalf("prior manifest = %q, %v", actual, err)
			}
		})
	}
}
