package local

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/rgomids/axiom/internal/workitem"
)

func TestEveryT03StorePreservesOutsideRootHash(t *testing.T) {
	base := t.TempDir()
	sentinel := filepath.Join(base, "outside-sentinel")
	content := []byte("unchanged")
	want := sha256.Sum256(content)
	if err := os.WriteFile(sentinel, content, 0o600); err != nil {
		t.Fatal(err)
	}

	portable, err := NewPortableStore(filepath.Join(base, "portable"))
	if err != nil {
		t.Fatal(err)
	}
	if err := portable.Create(context.Background(), "sample", []byte("manifest")); err != nil {
		t.Fatal(err)
	}

	source := privateTestRoot(t)
	manifest := []byte("schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n")
	if err := os.WriteFile(filepath.Join(source, manifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	installation, err := NewInstallationStore(filepath.Join(base, "installation"))
	if err != nil {
		t.Fatal(err)
	}
	if result := installation.Install(context.Background(), source); result.Status != InstallationApplied {
		t.Fatal(result)
	}

	link := workitem.Link{ProjectID: "123e4567-e89b-42d3-a456-426614174000", RepositoryKey: "main", Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}
	items, err := NewWorkItemStore(filepath.Join(base, "work-items"))
	if err != nil {
		t.Fatal(err)
	}
	if err := items.Save(context.Background(), link); err != nil {
		t.Fatal(err)
	}

	workflows, err := NewWorkflowStore(filepath.Join(base, "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	if err := workflows.Create(context.Background(), validExecution()); err != nil {
		t.Fatal(err)
	}

	artifacts, err := NewArtifactStore(filepath.Join(base, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	artifacts.allocate = func() (string, error) { return testArtifactID, nil }
	if _, err := artifacts.Create(context.Background(), artifactDraft(t)); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(sentinel)
	if err != nil || sha256.Sum256(after) != want {
		t.Fatalf("outside-root hash changed: %x, %v", sha256.Sum256(after), err)
	}
}
