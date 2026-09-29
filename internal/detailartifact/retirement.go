package detailartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	// EvidenceRetention starts only at an explicit, recorded retirement.
	EvidenceRetention   = 365 * 24 * time.Hour
	MaxRetirementRecord = 4 << 10
)

// Retirement is the authoritative record that every Evidence reference was
// explicitly retired. It lives beside, never inside, metadata format 1.
type Retirement struct {
	ArtifactID       string
	ArtifactRevision string
	ArtifactDigest   string
	RetiredAt        time.Time
	PreviewDigest    string
}

// RetirementObservation is one retirement record read under lock. Revision is
// the digest of the exact record bytes; Valid is false for any record that
// could not be decoded exactly.
type RetirementObservation struct {
	ArtifactID string
	Revision   string
	Retirement Retirement
	Valid      bool
}

type RetirementPreview struct {
	ObservedAt       time.Time `json:"observedAt"`
	ArtifactID       string    `json:"artifactId"`
	ArtifactRevision string    `json:"artifactRevision"`
	ArtifactDigest   string    `json:"artifactDigest"`
	Denied           string    `json:"denied,omitempty"`
	Digest           string    `json:"digest"`
}

type RetirementAuthority struct{ digest string }

// PlanRetirement grants nothing: it reports whether an explicit retirement of
// the exact artifact revision is possible. Age, CreatedAt, or cleanup pressure
// are never inputs; any live authoritative reference denies.
func PlanRetirement(now time.Time, artifact Artifact, revision string, references []string, existing *RetirementObservation) (RetirementPreview, error) {
	if now.IsZero() {
		return RetirementPreview{}, errors.New("retirement clock required")
	}
	preview := RetirementPreview{ObservedAt: now.UTC(), ArtifactID: artifact.ID, ArtifactRevision: revision, ArtifactDigest: hex.EncodeToString(artifact.Digest[:])}
	switch {
	case !artifact.Valid() || revision == "":
		preview.Denied = "uncertain"
	case artifact.Retention != Evidence:
		preview.Denied = "not_evidence"
	case len(references) > 0 || len(artifact.LiveReferences) > 0:
		preview.Denied = "referenced"
	case existing != nil:
		preview.Denied = "already_retired"
	}
	wire, _ := json.Marshal(struct {
		ID, Revision, Digest, Denied string
		References                   int
	}{preview.ArtifactID, preview.ArtifactRevision, preview.ArtifactDigest, preview.Denied, len(references)})
	digest := sha256.Sum256(wire)
	preview.Digest = hex.EncodeToString(digest[:])
	return preview, nil
}

func AuthorizeRetirement(preview RetirementPreview, reviewedDigest string) (RetirementAuthority, error) {
	if preview.Denied != "" || preview.Digest == "" || reviewedDigest != preview.Digest {
		return RetirementAuthority{}, errors.New("retirement authority denied")
	}
	return RetirementAuthority{digest: preview.Digest}, nil
}

func RetirementAuthorized(preview RetirementPreview, authority RetirementAuthority) bool {
	return preview.Denied == "" && preview.Digest != "" && preview.Digest == authority.digest
}

// ValidRetirementRecordID accepts only "<artifact-id>.json".
func ValidRetirementRecordID(value string) bool {
	id, ok := strings.CutSuffix(value, ".json")
	return ok && ValidID(id)
}

// evidenceEligibility returns the preserve reason, or "" when the exact
// retirement makes the artifact eligible. Every mismatch preserves.
func evidenceEligibility(now time.Time, artifact Artifact, revision string, retirement RetirementObservation, found bool) string {
	switch {
	case !found:
		return "evidence_not_retired"
	case !retirement.Valid || retirement.Revision == "" || retirement.Retirement.ArtifactID != artifact.ID:
		return "evidence_retirement_uncertain"
	case retirement.Retirement.ArtifactRevision != revision || retirement.Retirement.ArtifactDigest != hex.EncodeToString(artifact.Digest[:]) || retirement.Retirement.RetiredAt.Before(artifact.CreatedAt):
		return "evidence_retirement_stale"
	case now.Sub(retirement.Retirement.RetiredAt) < EvidenceRetention:
		return "evidence_within_365_days_of_retirement"
	}
	return ""
}
