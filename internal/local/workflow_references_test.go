package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/provenance"
	"github.com/rgomids/axiom/internal/workflow"
)

func TestWorkflowReferenceValidatorBindsArtifactToExecutionAndDigest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := NewArtifactStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.allocate = func() (string, error) { return "123e4567-e89b-42d3-a456-426614174001", nil }
	source, _ := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "abc123", SourceState: provenance.Clean}, nil)
	executionID := "123e4567-e89b-42d3-a456-426614174000"
	artifact, err := store.Create(context.Background(), detailartifact.Draft{ExecutionID: executionID, Category: "evidence", Outcome: "success", Retention: detailartifact.Evidence, Markdown: []byte("# Evidence\n"), Provenance: source})
	if err != nil {
		t.Fatal(err)
	}
	validator := NewWorkflowReferenceValidator(store)
	valid := workflow.Reference{Kind: "artifact", ID: artifact.ID, Digest: hex.EncodeToString(artifact.Digest[:])}
	if err := validator.Validate(context.Background(), executionID, t.TempDir(), valid); err != nil {
		t.Fatalf("valid artifact = %v", err)
	}
	if err := validator.Validate(context.Background(), "223e4567-e89b-42d3-a456-426614174000", t.TempDir(), valid); err == nil {
		t.Fatal("foreign execution artifact accepted")
	}
	valid.Digest = hex.EncodeToString(make([]byte, sha256.Size))
	if err := validator.Validate(context.Background(), executionID, t.TempDir(), valid); err == nil {
		t.Fatal("wrong artifact digest accepted")
	}
}

func TestWorkflowReferenceValidatorConfinesEvidenceToRepository(t *testing.T) {
	repository := t.TempDir()
	content := []byte("bounded evidence\n")
	if err := os.WriteFile(filepath.Join(repository, "evidence.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	validator := NewWorkflowReferenceValidator(ArtifactStore{})
	valid := workflow.Reference{Kind: "evidence", ID: "evidence.txt", Digest: hex.EncodeToString(digest[:])}
	if err := validator.Validate(context.Background(), "execution", repository, valid); err != nil {
		t.Fatalf("valid evidence = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, content, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(repository, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../outside.txt", "link.txt", "missing.txt"} {
		invalid := valid
		invalid.ID = id
		if err := validator.Validate(context.Background(), "execution", repository, invalid); err == nil {
			t.Fatalf("unsafe evidence accepted: %s", id)
		}
	}
}
