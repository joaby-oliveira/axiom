package detailartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	DiagnosticRetention = 30 * 24 * time.Hour
	CleanupRetention    = 90 * 24 * time.Hour
	MaxCleanupRecord    = 64 << 10
	// MaxCleanupBatch keeps every removed identity inside one bounded cleanup
	// record; larger cleanup is explicit repeated batches, never omission.
	MaxCleanupBatch = 128
)

const (
	EffectArtifact      = "artifact"
	EffectCleanupRecord = "cleanup_record"
)

type CleanupEffect struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Digest   string `json:"digest"`
	Bytes    int64  `json:"bytes"`
	Basis    string `json:"basis"`
	// Retirement is the exact retirement record revision an Evidence removal
	// depends on; it is revalidated before removal.
	Retirement string `json:"retirement,omitempty"`
}

// RecordObservation is one existing cleanup audit record, read under lock.
type RecordObservation struct {
	ID          string
	Revision    string
	Bytes       int64
	Status      string
	CompletedAt time.Time
	Valid       bool
}

type CleanupPreview struct {
	ObservedAt        time.Time       `json:"observedAt"`
	Effects           []CleanupEffect `json:"effects"`
	PreservedSummary  map[string]int  `json:"preservedSummary"`
	RemainingEligible int             `json:"remainingEligible"`
	ReclaimedBytes    int64           `json:"reclaimedBytes"`
	Digest            string          `json:"digest"`
	preserved         []string
}

type CleanupAuthority struct{ digest string }

// PlanCleanup derives eligibility only from authoritative references, the
// retention class, and the preview clock. Uncertainty always preserves.
//
// Evidence-class artifacts become eligible only 365 days after an explicit
// retirement record bound to the exact artifact revision; CreatedAt and the
// momentary absence of references never substitute for that record.
func PlanCleanup(now time.Time, artifacts []Artifact, references map[string][]string, revisions map[string]string, records []RecordObservation, retirements map[string]RetirementObservation) (CleanupPreview, error) {
	if now.IsZero() {
		return CleanupPreview{}, errors.New("cleanup clock required")
	}
	preview := CleanupPreview{ObservedAt: now.UTC(), PreservedSummary: map[string]int{}}
	ordered := append([]Artifact(nil), artifacts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	derived := map[string][]string{}
	for _, artifact := range ordered {
		for _, reference := range artifact.References {
			if reference.Kind == "artifact" && ValidID(reference.Value) {
				derived[reference.Value] = append(derived[reference.Value], "artifact:"+artifact.ID)
			}
		}
		if ValidID(artifact.SupersededBy) {
			derived[artifact.SupersededBy] = append(derived[artifact.SupersededBy], "superseded-by:"+artifact.ID)
		}
	}
	eligible := make([]CleanupEffect, 0)
	for _, artifact := range ordered {
		revision := revisions[artifact.ID]
		reason, basis := "", "diagnostic_unreferenced_30_days"
		retirement, retired := retirements[artifact.ID]
		switch {
		case !artifact.Valid() || revision == "":
			reason = "uncertain"
		case len(references[artifact.ID]) > 0 || len(derived[artifact.ID]) > 0 || len(artifact.LiveReferences) > 0:
			reason = "referenced"
		case artifact.Retention == Active:
			reason = "active"
		case artifact.Retention == PreservedReview:
			reason = "preserved_review"
		case artifact.Retention == Evidence:
			reason, basis = evidenceEligibility(now, artifact, revision, retirement, retired), "evidence_retired_365_days"
		case artifact.Retention != Diagnostic:
			reason = "uncertain"
		case now.Sub(artifact.CreatedAt) < DiagnosticRetention:
			reason = "diagnostic_within_30_days"
		}
		if reason != "" {
			preview.preserved = append(preview.preserved, artifact.ID+":"+reason)
			preview.PreservedSummary[reason]++
			continue
		}
		effect := CleanupEffect{Kind: EffectArtifact, ID: artifact.ID, Revision: revision, Digest: hex.EncodeToString(artifact.Digest[:]), Bytes: int64(artifact.ContentBytes), Basis: basis}
		if artifact.Retention == Evidence {
			effect.Retirement = retirement.Revision
		}
		eligible = append(eligible, effect)
	}
	// A retirement without its artifact grants nothing and is only reported.
	orphans := make([]string, 0)
	for id := range retirements {
		if _, ok := revisions[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	sort.Strings(orphans)
	for _, id := range orphans {
		preview.preserved = append(preview.preserved, id+":retirement_orphan")
		preview.PreservedSummary["retirement_orphan"]++
	}
	orderedRecords := append([]RecordObservation(nil), records...)
	sort.Slice(orderedRecords, func(i, j int) bool { return orderedRecords[i].ID < orderedRecords[j].ID })
	for _, record := range orderedRecords {
		reason := ""
		switch {
		case !record.Valid || record.Revision == "":
			reason = "cleanup_record_uncertain"
		case record.Status != "confirmed":
			reason = "cleanup_record_not_confirmed"
		case now.Sub(record.CompletedAt) < CleanupRetention:
			reason = "cleanup_record_within_90_days"
		}
		if reason != "" {
			preview.preserved = append(preview.preserved, record.ID+":"+reason)
			preview.PreservedSummary[reason]++
			continue
		}
		eligible = append(eligible, CleanupEffect{Kind: EffectCleanupRecord, ID: record.ID, Revision: record.Revision, Digest: record.Revision, Bytes: record.Bytes, Basis: "confirmed_record_90_days"})
	}
	if len(eligible) > MaxCleanupBatch {
		preview.RemainingEligible = len(eligible) - MaxCleanupBatch
		eligible = eligible[:MaxCleanupBatch]
	}
	preview.Effects = eligible
	for _, effect := range eligible {
		preview.ReclaimedBytes += effect.Bytes
	}
	// The digest binds the eligibility outcome, not the raw clock, so a later
	// stateless revalidation authorizes only an identical effect set.
	wire, _ := json.Marshal(struct {
		Effects   []CleanupEffect `json:"effects"`
		Preserved []string        `json:"preserved"`
		Remaining int             `json:"remaining"`
	}{preview.Effects, preview.preserved, preview.RemainingEligible})
	digest := sha256.Sum256(wire)
	preview.Digest = hex.EncodeToString(digest[:])
	return preview, nil
}

func (p CleanupPreview) Preserved() []string { return append([]string(nil), p.preserved...) }

func AuthorizeCleanup(preview CleanupPreview, reviewedDigest string) (CleanupAuthority, error) {
	if len(preview.Effects) == 0 || preview.Digest == "" || reviewedDigest != preview.Digest {
		return CleanupAuthority{}, errors.New("cleanup authority denied")
	}
	return CleanupAuthority{digest: preview.Digest}, nil
}

func CleanupAuthorized(preview CleanupPreview, authority CleanupAuthority) bool {
	return preview.Digest != "" && preview.Digest == authority.digest
}

// ValidCleanupRecordID accepts only names derived from a preview digest.
func ValidCleanupRecordID(value string) bool {
	name, ok := strings.CutPrefix(value, "cleanup-")
	if !ok {
		return false
	}
	name, ok = strings.CutSuffix(name, ".json")
	if !ok || len(name) != 32 {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil && strings.ToLower(name) == name
}
