package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rgomids/axiom/internal/workflow"
)

const maxWorkflowEvidenceBytes = 1 << 20

type WorkflowReferenceValidator struct{ artifacts ArtifactStore }

func NewWorkflowReferenceValidator(artifacts ArtifactStore) WorkflowReferenceValidator {
	return WorkflowReferenceValidator{artifacts: artifacts}
}

func (v WorkflowReferenceValidator) Validate(ctx context.Context, executionID, repositoryPath string, reference workflow.Reference) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch reference.Kind {
	case "artifact":
		artifact, err := v.artifacts.Read(ctx, reference.ID)
		if err != nil || artifact.ExecutionID != executionID || hex.EncodeToString(artifact.Digest[:]) != reference.Digest {
			return ErrUnsafe
		}
		return nil
	case "evidence", "specification", "decision", "plan", "tasks", "pull_request":
		return validateWorkflowEvidence(repositoryPath, reference)
	default:
		return ErrUnsafe
	}
}

func validateWorkflowEvidence(repositoryPath string, reference workflow.Reference) error {
	if !filepath.IsAbs(repositoryPath) || filepath.IsAbs(reference.ID) || strings.ContainsRune(reference.ID, '\\') {
		return ErrUnsafe
	}
	clean := filepath.Clean(reference.ID)
	if clean != reference.ID || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ErrUnsafe
	}
	root, err := os.OpenRoot(repositoryPath)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(clean)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxWorkflowEvidenceBytes {
		return ErrUnsafe
	}
	file, err := root.Open(clean)
	if err != nil {
		return err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxWorkflowEvidenceBytes+1))
	if err != nil || len(content) > maxWorkflowEvidenceBytes {
		return ErrUnsafe
	}
	digest, err := hex.DecodeString(reference.Digest)
	if err != nil || len(digest) != sha256.Size {
		return ErrUnsafe
	}
	observed := sha256.Sum256(content)
	if !equalBytes(observed[:], digest) {
		return ErrUnsafe
	}
	return nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	value := byte(0)
	for index := range left {
		value |= left[index] ^ right[index]
	}
	return value == 0
}
