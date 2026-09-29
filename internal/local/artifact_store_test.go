package local

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/provenance"
)

const testArtifactID = "123e4567-e89b-42d3-a456-426614174001"

func artifactDraft(t testing.TB) detailartifact.Draft {
	t.Helper()
	source, err := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "123456789abc", SourceState: provenance.Clean}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return detailartifact.Draft{CorrelationID: "123e4567-e89b-42d3-a456-426614174000", Category: "diagnostic", Outcome: "failure", Retention: detailartifact.Diagnostic, Markdown: []byte("# Detail\n\nSynthetic diagnosis.\n"), Provenance: source}
}

func TestArtifactStoreCoordinatesProcessesAndPreservesInterruptedState(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	command := exec.Command(os.Args[0], "-test.run=^TestArtifactStoreProcessHelper$")
	command.Env = append(os.Environ(), "AXIOM_ARTIFACT_HELPER=hold", "AXIOM_ARTIFACT_ROOT="+state)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "staged" {
		t.Fatalf("helper barrier: %q %v", line, err)
	}
	store, err := NewArtifactStore(state)
	if err != nil {
		t.Fatal(err)
	}
	store.allocate = func() (string, error) { return "223e4567-e89b-42d3-a456-426614174001", nil }
	if _, err := store.Create(context.Background(), artifactDraft(t)); !errors.Is(err, ErrConflict) {
		t.Fatalf("concurrent writer = %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	if _, err := store.Create(context.Background(), artifactDraft(t)); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("interrupted state not preserved: %v", err)
	}
}

func TestArtifactStoreProcessHelper(t *testing.T) {
	if os.Getenv("AXIOM_ARTIFACT_HELPER") == "" {
		return
	}
	store, err := NewArtifactStore(os.Getenv("AXIOM_ARTIFACT_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	store.allocate = func() (string, error) { return testArtifactID, nil }
	store.hooks.fault = func(stage FaultStage) error {
		if stage == FaultF3 {
			_, _ = fmt.Fprintln(os.Stdout, "staged")
			time.Sleep(time.Hour)
		}
		return nil
	}
	if _, err := store.Create(context.Background(), artifactDraft(t)); err != nil {
		t.Fatal(err)
	}
}

func artifactStore(t *testing.T) ArtifactStore {
	t.Helper()
	store, err := NewArtifactStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	store.allocate = func() (string, error) { return testArtifactID, nil }
	store.now = func() (time.Time, error) { return time.Unix(10, 20).UTC(), nil }
	return store
}

func TestArtifactStoreCreateReadAndConfinement(t *testing.T) {
	base := t.TempDir()
	state := filepath.Join(base, "state")
	portable := filepath.Join(base, "portable")
	repository := filepath.Join(base, "repository")
	for _, path := range []string{portable, repository} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "sentinel"), []byte("unchanged"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewArtifactStore(state)
	if err != nil {
		t.Fatal(err)
	}
	store.allocate = func() (string, error) { return testArtifactID, nil }
	store.now = func() (time.Time, error) { return time.Unix(10, 20).UTC(), nil }
	created, err := store.Create(context.Background(), artifactDraft(t))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Read(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Reference() != "artifact:"+testArtifactID || loaded.Digest != created.Digest || string(loaded.Markdown) != string(created.Markdown) {
		t.Fatalf("artifact mismatch: %#v", loaded)
	}
	object := filepath.Join(state, "artifacts", "v1", "objects", "12", testArtifactID)
	for _, path := range []string{state, filepath.Join(state, "artifacts"), filepath.Join(state, "artifacts", "v1"), filepath.Join(state, "artifacts", "v1", "objects"), filepath.Join(state, "artifacts", "v1", "objects", "12"), object} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("unsafe directory %s: %v %v", path, info, err)
		}
	}
	for _, name := range []string{"metadata.json", "details.md"} {
		info, err := os.Stat(filepath.Join(object, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("unsafe file %s: %v %v", name, info, err)
		}
	}
	for _, path := range []string{portable, repository} {
		content, err := os.ReadFile(filepath.Join(path, "sentinel"))
		if err != nil || string(content) != "unchanged" {
			t.Fatalf("outside tree changed: %s", path)
		}
	}
	if _, err := store.Read(context.Background(), "../../repository/sentinel"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("path lookup accepted: %v", err)
	}
}

func TestArtifactStoreCapacityAndCollisionFailWithoutEviction(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
		bytes int64
	}{
		{"count", detailartifact.MaxLiveArtifacts, 0},
		{"bytes", 0, detailartifact.MaxAggregateBytes},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := artifactStore(t)
			store.capacity = func(*os.Root) (int, int64, error) { return test.count, test.bytes, nil }
			if _, err := store.Create(context.Background(), artifactDraft(t)); !errors.Is(err, ErrCapacity) {
				t.Fatalf("capacity result: %v", err)
			}
		})
	}
	store := artifactStore(t)
	if _, err := store.Create(context.Background(), artifactDraft(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), artifactDraft(t)); !errors.Is(err, ErrConflict) {
		t.Fatalf("collision replaced object: %v", err)
	}
	if _, err := store.Read(context.Background(), testArtifactID); err != nil {
		t.Fatalf("collision damaged prior artifact: %v", err)
	}
}

func TestArtifactStoreCapacityBoundariesAndInvalidEntriesDoNotEvict(t *testing.T) {
	for _, test := range []struct {
		name    string
		count   int
		wantErr error
	}{
		{name: "below", count: detailartifact.MaxLiveArtifacts - 1},
		{name: "exactly", count: detailartifact.MaxLiveArtifacts, wantErr: ErrCapacity},
		{name: "above", count: detailartifact.MaxLiveArtifacts + 1, wantErr: ErrCapacity},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := artifactStore(t)
			store.capacity = func(*os.Root) (int, int64, error) { return test.count, 0, nil }
			_, err := store.Create(context.Background(), artifactDraft(t))
			if !errors.Is(err, test.wantErr) || test.wantErr == nil && err != nil {
				t.Fatalf("capacity boundary = %v", err)
			}
		})
	}

	store := artifactStore(t)
	root, objects, err := store.openObjects(true)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	defer objects.Close()
	for index := 0; index < 129; index++ {
		name := fmt.Sprintf("invalid-%03d", index)
		if err := objects.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := artifactCapacity(objects); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("invalid capacity state = %v", err)
	}
	for index := 0; index < 129; index++ {
		name := fmt.Sprintf("invalid-%03d", index)
		if _, err := objects.Lstat(name); err != nil {
			t.Fatalf("capacity scan removed %s: %v", name, err)
		}
	}
}

func TestArtifactObjectEnumerationRejectsAdditionalEntriesBoundedly(t *testing.T) {
	store := artifactStore(t)
	if _, err := store.Create(context.Background(), artifactDraft(t)); err != nil {
		t.Fatal(err)
	}
	object := filepath.Join(store.root, "artifacts", "v1", "objects", "12", testArtifactID)
	for index := 0; index < 129; index++ {
		if err := os.WriteFile(filepath.Join(object, fmt.Sprintf("unexpected-%03d", index)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Read(context.Background(), testArtifactID); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("artifact with extra entries = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(object, "unexpected-128")); err != nil {
		t.Fatalf("artifact read removed unknown entry: %v", err)
	}
}

func TestArtifactStoreFaultStagesPreserveCommitTruthAndFailClosed(t *testing.T) {
	for index, stage := range []FaultStage{FaultF0, FaultF1, FaultF2, FaultF3, FaultF4, FaultF5, FaultF6, FaultF7, FaultF8} {
		t.Run(string(stage), func(t *testing.T) {
			store := artifactStore(t)
			store.hooks.fault = func(current FaultStage) error {
				if current == stage {
					return ErrSimulatedInterruption
				}
				return nil
			}
			_, err := store.Create(context.Background(), artifactDraft(t))
			var publication *PublicationError
			if !errors.As(err, &publication) || publication.Stage != stage {
				t.Fatalf("fault result: %v", err)
			}
			wantCommitted := index >= 6
			if publication.Committed != wantCommitted {
				t.Fatalf("committed=%t want %t", publication.Committed, wantCommitted)
			}
			_, readErr := store.Read(context.Background(), testArtifactID)
			if stage == FaultF0 {
				if readErr == nil {
					t.Fatal("F0 exposed artifact")
				}
				return
			}
			if !errors.Is(readErr, ErrRecoveryRequired) {
				t.Fatalf("reader did not fail closed: %v", readErr)
			}
		})
	}
}

func TestArtifactStoreRestoresMarkerWhenRemovalSyncFailsAfterCommit(t *testing.T) {
	store := artifactStore(t)
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(store.root, "outside-owned-sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed := false
	failed := false
	store.hooks.remove = func(root *os.Root, name string) error {
		err := root.Remove(name)
		if err == nil && strings.HasPrefix(name, ".axiom-recovery-") {
			removed = true
		}
		return err
	}
	store.hooks.sync = func(root *os.Root) error {
		if removed && !failed {
			failed = true
			return syscall.EIO
		}
		return syncRoot(root)
	}
	draft := artifactDraft(t)
	_, err := store.Create(context.Background(), draft)
	var publication *PublicationError
	if !errors.As(err, &publication) || !publication.Committed || !errors.Is(err, ErrRecoveryRequired) || !removed || !failed {
		t.Fatalf("post-removal sync result = %v", err)
	}
	object := filepath.Join(store.root, "artifacts", "v1", "objects", "12", testArtifactID)
	if _, err := os.ReadFile(filepath.Join(object, "metadata.json")); err != nil {
		t.Fatalf("canonical metadata unavailable: %v", err)
	}
	if markdown, err := os.ReadFile(filepath.Join(object, "details.md")); err != nil || string(markdown) != string(draft.Markdown) {
		t.Fatalf("canonical content = %q, %v", markdown, err)
	}
	if _, err := store.Read(context.Background(), testArtifactID); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("reader after removal sync failure = %v", err)
	}
	if content, err := os.ReadFile(sentinel); err != nil || string(content) != "unchanged" {
		t.Fatalf("non-protocol object changed = %q, %v", content, err)
	}
}

func TestArtifactStoreRejectsDigestModeLinkAndTypeChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("MVP excludes Windows")
	}
	tests := []struct {
		name   string
		mutate func(string) error
	}{
		{"digest", func(object string) error {
			return os.WriteFile(filepath.Join(object, "details.md"), []byte("changed"), 0o600)
		}},
		{"mode", func(object string) error { return os.Chmod(filepath.Join(object, "details.md"), 0o644) }},
		{"symlink", func(object string) error {
			if err := os.Remove(filepath.Join(object, "details.md")); err != nil {
				return err
			}
			return os.Symlink("metadata.json", filepath.Join(object, "details.md"))
		}},
		{"hardlink", func(object string) error {
			return os.Link(filepath.Join(object, "details.md"), filepath.Join(object, "copy.md"))
		}},
		{"type", func(object string) error {
			if err := os.Remove(filepath.Join(object, "details.md")); err != nil {
				return err
			}
			return os.Mkdir(filepath.Join(object, "details.md"), 0o700)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := artifactStore(t)
			if _, err := store.Create(context.Background(), artifactDraft(t)); err != nil {
				t.Fatal(err)
			}
			object := filepath.Join(store.root, "artifacts", "v1", "objects", "12", testArtifactID)
			if err := test.mutate(object); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Read(context.Background(), testArtifactID); err == nil {
				t.Fatal("unsafe artifact returned")
			}
		})
	}
}
