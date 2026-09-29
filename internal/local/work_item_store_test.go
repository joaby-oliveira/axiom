package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/rgomids/axiom/internal/workitem"
)

func TestWorkItemStoreRoundTrip(t *testing.T) {
	store, err := NewWorkItemStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	link := testWorkItemLink()
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
	if err != nil || got.ProjectID != link.ProjectID || got.State != link.State || got.Revision == ([32]byte{}) {
		t.Fatalf("round trip = %#v, %v", got, err)
	}
	link.Revision = got.Revision
	link.State = "CLOSED"
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	got, err = store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
	if err != nil || got.State != "CLOSED" {
		t.Fatalf("updated state = %#v, %v", got, err)
	}
}

func TestWorkItemStoreLoadsExistingV1RecordIntoProviderNeutralLink(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	projectID := "123e4567-e89b-42d3-a456-426614174000"
	projectRoot := filepath.Join(state, "work-items", projectID)
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	wire := []byte("{\"formatVersion\":1,\"projectId\":\"" + projectID + "\",\"repositoryKey\":\"main\",\"providerRepository\":\"owner/repo\",\"number\":7,\"url\":\"https://github.com/owner/repo/issues/7\",\"state\":\"OPEN\"}\n")
	if err := os.WriteFile(filepath.Join(projectRoot, "main-7.json"), wire, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewWorkItemStore(state)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), projectID, "main", "github", "owner/repo", "7")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "github" || got.Resource != "owner/repo" || got.ExternalID != "7" || got.URL != "https://github.com/owner/repo/issues/7" || got.State != "OPEN" || got.Revision == ([32]byte{}) {
		t.Fatalf("loaded v1 link = %#v", got)
	}
	got.State = "CLOSED"
	if err := store.Save(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(filepath.Join(projectRoot, "main-7.json"))
	if err != nil || !strings.Contains(string(updated), `"state":"CLOSED"`) {
		t.Fatalf("updated legacy v1 = %q, %v", updated, err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, workItemName("main", "github", "owner/repo", "7"))); !os.IsNotExist(err) {
		t.Fatalf("legacy update created a second identity path: %v", err)
	}
}

func TestWorkItemStorePersistsProviderNeutralLinkUsingExistingV1WireFormat(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	store, err := NewWorkItemStore(state)
	if err != nil {
		t.Fatal(err)
	}
	link := testWorkItemLink()
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	wire, err := os.ReadFile(filepath.Join(state, "work-items", link.ProjectID, workItemName(link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"formatVersion\":1,\"projectId\":\"123e4567-e89b-42d3-a456-426614174000\",\"repositoryKey\":\"main\",\"providerRepository\":\"owner/repo\",\"number\":7,\"url\":\"https://github.com/owner/repo/issues/7\",\"state\":\"OPEN\"}\n"
	if string(wire) != want {
		t.Fatalf("wire = %q, want %q", wire, want)
	}
}

func TestWorkItemStoreSeparatesSameExternalIDAcrossResources(t *testing.T) {
	store, err := NewWorkItemStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	first := testWorkItemLink()
	second := first
	second.Resource = "other/repo"
	second.URL = "https://github.com/other/repo/issues/7"
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	for _, link := range []workitem.Link{first, second} {
		loaded, err := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
		if err != nil || loaded.Resource != link.Resource || loaded.URL != link.URL {
			t.Fatalf("load %s = %#v, %v", link.Resource, loaded, err)
		}
	}
	if _, err := store.Load(context.Background(), first.ProjectID, first.RepositoryKey, "", "", first.ExternalID); !errors.Is(err, workitem.ErrConflict) {
		t.Fatalf("unqualified collision = %v", err)
	}
}

func TestWorkItemStoreCreateAttemptRoundTripAndStaleRevision(t *testing.T) {
	store, err := NewWorkItemStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	target := workitem.DraftTarget{ProjectID: testWorkItemLink().ProjectID, RepositoryKey: "main", Provider: "github", Resource: "owner/repo"}
	attempt := workitem.CreateAttempt{Target: target, Correlation: strings.Repeat("a", 64), PreviewDigest: strings.Repeat("b", 64), State: workitem.CreateAttemptPending}
	if err := store.SaveCreateAttempt(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadCreateAttempt(context.Background(), target)
	if err != nil || loaded.State != workitem.CreateAttemptPending || loaded.Revision == ([32]byte{}) {
		t.Fatalf("loaded attempt = %#v, %v", loaded, err)
	}
	stale := loaded
	loaded.State = workitem.CreateAttemptConfirmed
	loaded.ExternalID = "7"
	if err := store.SaveCreateAttempt(context.Background(), loaded); err != nil {
		t.Fatal(err)
	}
	stale.State = workitem.CreateAttemptRetryAllowed
	if err := store.SaveCreateAttempt(context.Background(), stale); !errors.Is(err, workitem.ErrConflict) {
		t.Fatalf("stale attempt update = %v", err)
	}
}

func TestWorkItemStoreRejectsLinksV1CannotRepresentBeforeWriting(t *testing.T) {
	for name, mutate := range map[string]func(*workitem.Link){
		"other_provider": func(link *workitem.Link) { link.Provider = "gitlab" },
		"non_numeric":    func(link *workitem.Link) { link.ExternalID = "issue-seven" },
		"zero":           func(link *workitem.Link) { link.ExternalID = "0" },
		"non_canonical":  func(link *workitem.Link) { link.ExternalID = "007" },
	} {
		t.Run(name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "state")
			store, err := NewWorkItemStore(state)
			if err != nil {
				t.Fatal(err)
			}
			link := testWorkItemLink()
			mutate(&link)
			if err := store.Save(context.Background(), link); !errors.Is(err, ErrUnsafe) {
				t.Fatalf("save = %v", err)
			}
			if _, err := os.Stat(state); !os.IsNotExist(err) {
				t.Fatalf("state written before rejection: %v", err)
			}
		})
	}
}

func TestWorkItemStoreReportsMissingForFirstSelection(t *testing.T) {
	store, err := NewWorkItemStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Load(context.Background(), "123e4567-e89b-42d3-a456-426614174000", "main", "github", "owner/repo", "7")
	if !errors.Is(err, workitem.ErrNotFound) {
		t.Fatalf("missing load = %v", err)
	}
}

func TestWorkItemStoreRejectsRecordWhoseIdentityDoesNotMatchRequestedPath(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	store, err := NewWorkItemStore(state)
	if err != nil {
		t.Fatal(err)
	}
	link := testWorkItemLink()
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, "work-items", link.ProjectID, workItemName(link.RepositoryKey, link.Provider, link.Resource, link.ExternalID))
	wire, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wire = []byte(strings.Replace(string(wire), `"number":7`, `"number":8`, 1))
	wire = []byte(strings.Replace(string(wire), `/issues/7`, `/issues/8`, 1))
	if err := os.WriteFile(path, wire, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("identity mismatch = %v", err)
	}
}

func TestWorkItemStoreCreateCollisionConflictsWithoutReplacement(t *testing.T) {
	store, err := NewWorkItemStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	link := testWorkItemLink()
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatal(err)
	}

	if err := store.Save(context.Background(), link); !errors.Is(err, ErrConflict) || !errors.Is(err, workitem.ErrConflict) {
		t.Fatalf("create collision = %v", err)
	}
	loaded, err := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
	if err != nil || loaded.State != link.State || loaded.Resource != link.Resource || loaded.Provider != link.Provider {
		t.Fatalf("canonical link = %#v, %v", loaded, err)
	}
}

func TestWorkItemStoreRequiresExpectedRevisionAndFailsClosedAcrossF0F8(t *testing.T) {
	for index, stage := range []FaultStage{FaultF0, FaultF1, FaultF2, FaultF3, FaultF4, FaultF5, FaultF6, FaultF7, FaultF8} {
		t.Run(string(stage), func(t *testing.T) {
			store, err := NewWorkItemStore(filepath.Join(t.TempDir(), "state"))
			if err != nil {
				t.Fatal(err)
			}
			link := testWorkItemLink()
			if err := store.Save(context.Background(), link); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
			if err != nil {
				t.Fatal(err)
			}
			loaded.State = "CLOSED"
			store.hooks.fault = func(current FaultStage) error {
				if current == stage {
					return ErrSimulatedInterruption
				}
				return nil
			}
			err = store.Save(context.Background(), loaded)
			var publication *PublicationError
			if !errors.As(err, &publication) || publication.Committed != (index >= 6) {
				t.Fatalf("fault outcome: %v", err)
			}
			_, readErr := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
			if stage == FaultF0 || stage == FaultF1 {
				if readErr != nil {
					t.Fatalf("F0 lost prior authority: %v", readErr)
				}
				return
			}
			if !errors.Is(readErr, ErrRecoveryRequired) {
				t.Fatalf("stage %s reader = %v", stage, readErr)
			}
		})
	}
}

func TestWorkItemStoreRejectsStaleRevision(t *testing.T) {
	store, err := NewWorkItemStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	link := testWorkItemLink()
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	first, _ := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
	stale := first
	first.State = "CLOSED"
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), stale); !errors.Is(err, ErrConflict) || !errors.Is(err, workitem.ErrConflict) {
		t.Fatalf("stale update = %v", err)
	}
}

func TestPublishFileCleanupFailuresRequireRecoveryBeforeCommit(t *testing.T) {
	for _, failure := range []string{"marker_remove", "staging_remove", "cleanup_sync"} {
		t.Run(failure, func(t *testing.T) {
			store, err := NewWorkItemStore(filepath.Join(t.TempDir(), "state"))
			if err != nil {
				t.Fatal(err)
			}
			link := testWorkItemLink()
			if err := store.Save(context.Background(), link); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
			if err != nil {
				t.Fatal(err)
			}
			loaded.State = "CLOSED"
			store.hooks.fault = func(stage FaultStage) error {
				if stage == FaultF3 {
					return context.Canceled
				}
				return nil
			}
			if failure == "marker_remove" || failure == "staging_remove" {
				store.hooks.remove = func(root *os.Root, name string) error {
					prefix := ".axiom-recovery-"
					if failure == "staging_remove" {
						prefix = ".axiom-stage-file-"
					}
					if strings.HasPrefix(name, prefix) {
						return syscall.EIO
					}
					return root.Remove(name)
				}
			}
			if failure == "cleanup_sync" {
				syncCalls := 0
				store.hooks.sync = func(root *os.Root) error {
					syncCalls++
					if syncCalls == 2 {
						return syscall.EIO
					}
					return syncRoot(root)
				}
			}

			err = store.Save(context.Background(), loaded)
			var publication *PublicationError
			if !errors.As(err, &publication) || publication.Committed || !errors.Is(err, ErrRecoveryRequired) || !errors.Is(err, context.Canceled) {
				t.Fatalf("cleanup failure = %v", err)
			}
			if _, readErr := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID); !errors.Is(readErr, ErrRecoveryRequired) {
				t.Fatalf("reader after cleanup failure = %v", readErr)
			}
		})
	}
}

func TestPublishFileRestoresMarkerWhenRemovalSyncFailsAfterCommit(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	store, err := NewWorkItemStore(state)
	if err != nil {
		t.Fatal(err)
	}
	link := testWorkItemLink()
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.State = "CLOSED"
	projectPath := filepath.Join(state, "work-items", link.ProjectID)
	sentinel := filepath.Join(state, "outside-owned-sentinel")
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
	err = store.Save(context.Background(), loaded)
	var publication *PublicationError
	if !errors.As(err, &publication) || !publication.Committed || !errors.Is(err, ErrRecoveryRequired) || !removed || !failed {
		t.Fatalf("post-removal sync result = %v", err)
	}
	wire, err := os.ReadFile(filepath.Join(projectPath, workItemName(link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)))
	if err != nil || !strings.Contains(string(wire), `"state":"CLOSED"`) {
		t.Fatalf("canonical bytes = %q, %v", wire, err)
	}
	if _, err := store.Load(context.Background(), link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("reader after removal sync failure = %v", err)
	}
	if content, err := os.ReadFile(sentinel); err != nil || string(content) != "unchanged" {
		t.Fatalf("non-protocol object changed = %q, %v", content, err)
	}
}

func testWorkItemLink() workitem.Link {
	return workitem.Link{ProjectID: "123e4567-e89b-42d3-a456-426614174000", RepositoryKey: "main", Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}
}
