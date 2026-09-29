package local

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/workflow"
)

const day = 24 * time.Hour

func evidenceDraft(draft *detailartifact.Draft) { draft.Retention = detailartifact.Evidence }

func (f *cleanupFixture) retire(t *testing.T, id string, at time.Time) RetirementResult {
	t.Helper()
	f.store.now = func() (time.Time, error) { return at, nil }
	defer func() { f.store.now = func() (time.Time, error) { return cleanupClock, nil } }()
	preview, err := f.store.PreviewRetirement(context.Background(), at, id)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := detailartifact.AuthorizeRetirement(preview, preview.Digest)
	if err != nil {
		t.Fatalf("retirement denied: %s", preview.Denied)
	}
	result, err := f.store.ApplyRetirement(context.Background(), preview, authority)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *cleanupFixture) cleanupAt(t *testing.T, at time.Time) CleanupResult {
	t.Helper()
	f.store.now = func() (time.Time, error) { return at, nil }
	defer func() { f.store.now = func() (time.Time, error) { return cleanupClock, nil } }()
	preview, err := f.store.PreviewCleanup(context.Background(), at)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := detailartifact.AuthorizeCleanup(preview, preview.Digest)
	if err != nil {
		t.Fatalf("cleanup authority: %v preserved=%v", err, preview.Preserved())
	}
	result, err := f.store.ApplyCleanup(context.Background(), preview, authority)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *cleanupFixture) retirementPath(id string) string {
	return filepath.Join(f.state, "artifacts", "v1", "retirements", id+".json")
}

func effectBasis(preview detailartifact.CleanupPreview) map[string]string {
	result := map[string]string{}
	for _, effect := range preview.Effects {
		result[effect.ID] = effect.Basis
	}
	return result
}

// The 30/365/90 policy in one fake-clock matrix: 30-day diagnostic, 365 days
// after explicit Evidence retirement, and 90-day confirmed cleanup records.
func TestRetentionEligibilityMatrix30_365_90(t *testing.T) {
	fixture := newCleanupFixture(t)
	oldRecord := fixture.create(t, 800*day, nil)
	fixture.cleanupAt(t, cleanupClock.Add(-detailartifact.CleanupRetention))
	youngRecord := fixture.create(t, 800*day, nil)
	fixture.cleanupAt(t, cleanupClock.Add(-detailartifact.CleanupRetention+time.Second))
	_ = oldRecord
	_ = youngRecord

	diagnosticAt := fixture.create(t, detailartifact.DiagnosticRetention, nil)
	diagnosticYoung := fixture.create(t, detailartifact.DiagnosticRetention-time.Second, nil)
	before := fixture.create(t, 800*day, evidenceDraft)
	exact := fixture.create(t, 800*day, evidenceDraft)
	after := fixture.create(t, 800*day, evidenceDraft)
	unretired := fixture.create(t, 800*day, evidenceDraft)
	fixture.retire(t, before.ID, cleanupClock.Add(-detailartifact.EvidenceRetention+time.Second))
	fixture.retire(t, exact.ID, cleanupClock.Add(-detailartifact.EvidenceRetention))
	fixture.retire(t, after.ID, cleanupClock.Add(-detailartifact.EvidenceRetention-time.Second))

	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil {
		t.Fatal(err)
	}
	basis := effectBasis(preview)
	if basis[diagnosticAt.ID] != "diagnostic_unreferenced_30_days" || basis[exact.ID] != "evidence_retired_365_days" || basis[after.ID] != "evidence_retired_365_days" {
		t.Fatalf("effects=%+v", preview.Effects)
	}
	for id, reason := range map[string]string{diagnosticYoung.ID: "diagnostic_within_30_days", before.ID: "evidence_within_365_days_of_retirement", unretired.ID: "evidence_not_retired"} {
		if got := preservedReason(preview, id); got != reason {
			t.Fatalf("artifact %s reason=%q want %q", id, got, reason)
		}
	}
	records := 0
	for _, effect := range preview.Effects {
		if effect.Kind == detailartifact.EffectCleanupRecord {
			records++
			if effect.Basis != "confirmed_record_90_days" {
				t.Fatalf("record effect=%+v", effect)
			}
		}
	}
	if records != 1 || preview.PreservedSummary["cleanup_record_within_90_days"] != 1 || len(preview.Effects) != 4 {
		t.Fatalf("effects=%+v summary=%v", preview.Effects, preview.PreservedSummary)
	}

	authority, err := detailartifact.AuthorizeCleanup(preview, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.store.ApplyCleanup(context.Background(), preview, authority)
	if err != nil || len(result.Removed) != 4 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, removed := range []detailartifact.Artifact{exact, after} {
		if _, err := fixture.store.Read(context.Background(), removed.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("retired Evidence still present: %v", err)
		}
		if _, err := os.Lstat(fixture.retirementPath(removed.ID)); !os.IsNotExist(err) {
			t.Fatalf("consumed retirement still present: %v", err)
		}
	}
	for _, kept := range []detailartifact.Artifact{before, unretired, diagnosticYoung} {
		if _, err := fixture.store.Read(context.Background(), kept.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetirementDeniedWhileReferenced(t *testing.T) {
	fixture := newCleanupFixture(t)
	byExecution := fixture.create(t, 800*day, evidenceDraft)
	fixture.execution(t, workflow.ExecutionActive, "123e4567-e89b-42d3-a456-000000000b11", workflow.Reference{Kind: "artifact", ID: byExecution.ID, Digest: hex.EncodeToString(byExecution.Digest[:])})
	live := fixture.create(t, 800*day, func(draft *detailartifact.Draft) {
		evidenceDraft(draft)
		draft.LiveReferences = []string{"review:open"}
	})
	byArtifact := fixture.create(t, 800*day, evidenceDraft)
	fixture.create(t, day, func(draft *detailartifact.Draft) {
		draft.References = []detailartifact.Reference{{Kind: "artifact", Value: byArtifact.ID}}
	})
	diagnostic := fixture.create(t, 800*day, nil)
	for id, denied := range map[string]string{byExecution.ID: "referenced", live.ID: "referenced", byArtifact.ID: "referenced", diagnostic.ID: "not_evidence"} {
		preview, err := fixture.store.PreviewRetirement(context.Background(), cleanupClock, id)
		if err != nil || preview.Denied != denied {
			t.Fatalf("artifact %s preview=%+v err=%v", id, preview, err)
		}
		if _, err := detailartifact.AuthorizeRetirement(preview, preview.Digest); err == nil {
			t.Fatalf("artifact %s: denied retirement authorized", id)
		}
		if _, err := fixture.store.ApplyRetirement(context.Background(), preview, detailartifact.RetirementAuthority{}); !errors.Is(err, ErrConflict) {
			t.Fatalf("artifact %s: apply without authority: %v", id, err)
		}
		if _, err := os.Lstat(fixture.retirementPath(id)); !os.IsNotExist(err) {
			t.Fatalf("artifact %s: retirement published: %v", id, err)
		}
	}
	if _, err := fixture.store.PreviewRetirement(context.Background(), cleanupClock, "123e4567-e89b-42d3-a456-999999999999"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing artifact: %v", err)
	}
}

func TestRetirementStaleAuthorityAndDuplicateAreDenied(t *testing.T) {
	fixture := newCleanupFixture(t)
	evidence := fixture.create(t, 800*day, evidenceDraft)
	preview, err := fixture.store.PreviewRetirement(context.Background(), cleanupClock, evidence.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := detailartifact.AuthorizeRetirement(preview, "stale"); err == nil {
		t.Fatal("stale digest authorized retirement")
	}
	authority, err := detailartifact.AuthorizeRetirement(preview, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	// A reference appears after review: the reviewed authority is stale.
	fixture.execution(t, workflow.ExecutionActive, "123e4567-e89b-42d3-a456-000000000b21", workflow.Reference{Kind: "artifact", ID: evidence.ID, Digest: hex.EncodeToString(evidence.Digest[:])})
	if _, err := fixture.store.ApplyRetirement(context.Background(), preview, authority); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale retirement accepted: %v", err)
	}
	if _, err := os.Lstat(fixture.retirementPath(evidence.ID)); !os.IsNotExist(err) {
		t.Fatal("stale retirement published")
	}

	other := fixture.create(t, 800*day, evidenceDraft)
	fixture.retire(t, other.ID, cleanupClock)
	again, err := fixture.store.PreviewRetirement(context.Background(), cleanupClock, other.ID)
	if err != nil || again.Denied != "already_retired" {
		t.Fatalf("duplicate retirement preview=%+v err=%v", again, err)
	}
}

func TestRetirementCorruptStaleOrUnsafeRecordPreservesEvidence(t *testing.T) {
	cases := map[string]struct {
		mutate func(t *testing.T, path string, wire []byte)
		reason string
	}{
		"garbage": {func(t *testing.T, path string, _ []byte) { writeTestFile(t, path, []byte("{not json\n"), 0o600) }, "evidence_retirement_uncertain"},
		"non-canonical": {func(t *testing.T, path string, wire []byte) {
			writeTestFile(t, path, []byte(strings.Replace(string(wire), `{"formatVersion":1`, `{ "formatVersion":1`, 1)), 0o600)
		}, "evidence_retirement_uncertain"},
		"unknown-version": {func(t *testing.T, path string, wire []byte) {
			writeTestFile(t, path, []byte(strings.Replace(string(wire), `"formatVersion":1`, `"formatVersion":2`, 1)), 0o600)
		}, "evidence_retirement_uncertain"},
		"other-revision": {func(t *testing.T, path string, wire []byte) {
			record, err := decodeRetirementRecord(wire)
			if err != nil {
				t.Fatal(err)
			}
			record.ArtifactRevision = strings.Repeat("0", 64)
			next, err := encodeRetirementRecord(record)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, path, next, 0o600)
		}, "evidence_retirement_stale"},
		"other-digest": {func(t *testing.T, path string, wire []byte) {
			record, _ := decodeRetirementRecord(wire)
			record.ArtifactDigest = strings.Repeat("a", 64)
			next, _ := encodeRetirementRecord(record)
			writeTestFile(t, path, next, 0o600)
		}, "evidence_retirement_stale"},
		"group-readable": {func(t *testing.T, path string, _ []byte) {
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
		}, "evidence_retirement_uncertain"},
		"symlink": {func(t *testing.T, path string, wire []byte) {
			target := filepath.Join(filepath.Dir(filepath.Dir(path)), "outside.json")
			writeTestFile(t, target, wire, 0o600)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}, "evidence_retirement_uncertain"},
		"directory": {func(t *testing.T, path string, _ []byte) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, "evidence_retirement_uncertain"},
	}
	for name, current := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newCleanupFixture(t)
			evidence := fixture.create(t, 800*day, evidenceDraft)
			fixture.retire(t, evidence.ID, cleanupClock.Add(-500*day))
			path := fixture.retirementPath(evidence.ID)
			wire, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			current.mutate(t, path, wire)
			preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
			if err != nil {
				t.Fatal(err)
			}
			if got := preservedReason(preview, evidence.ID); got != current.reason || len(preview.Effects) != 0 {
				t.Fatalf("reason=%q effects=%+v", got, preview.Effects)
			}
		})
	}
}

func TestRetirementNamespaceFailsClosed(t *testing.T) {
	fixture := newCleanupFixture(t)
	evidence := fixture.create(t, 800*day, evidenceDraft)
	fixture.retire(t, evidence.ID, cleanupClock.Add(-500*day))
	directory := filepath.Dir(fixture.retirementPath(evidence.ID))
	writeTestFile(t, filepath.Join(directory, "unexpected.json"), []byte("{}\n"), 0o600)
	if _, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("unknown retirement name accepted: %v", err)
	}
	if err := os.Remove(filepath.Join(directory, "unexpected.json")); err != nil {
		t.Fatal(err)
	}
	moved := directory + "-real"
	if err := os.Rename(directory, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, directory); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock); err == nil {
		t.Fatal("symlinked retirement namespace accepted")
	}
}

// Re-reference after retirement supersedes the old clock; the reference later
// disappearing never revives it, and a new explicit retirement starts anew.
func TestReReferenceSupersedesRetirementAndRequiresNewRetirement(t *testing.T) {
	fixture := newCleanupFixture(t)
	evidence := fixture.create(t, 2000*day, evidenceDraft)
	fixture.retire(t, evidence.ID, cleanupClock.Add(-1500*day))
	fixture.create(t, 1400*day, func(draft *detailartifact.Draft) {
		draft.References = []detailartifact.Reference{{Kind: "artifact", Value: evidence.ID}}
	})
	if _, err := os.Lstat(fixture.retirementPath(evidence.ID)); !os.IsNotExist(err) {
		t.Fatalf("re-reference did not supersede retirement: %v", err)
	}
	// The diagnostic referrer ages out and is removed: the reference disappears.
	fixture.cleanupAt(t, cleanupClock.Add(-1000*day))
	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil {
		t.Fatal(err)
	}
	if got := preservedReason(preview, evidence.ID); got != "evidence_not_retired" {
		t.Fatalf("old retirement revived: reason=%q effects=%+v", got, preview.Effects)
	}
	fixture.retire(t, evidence.ID, cleanupClock.Add(-100*day))
	preview, err = fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil || preservedReason(preview, evidence.ID) != "evidence_within_365_days_of_retirement" {
		t.Fatalf("new retirement clock: %v err=%v", preview.Preserved(), err)
	}
	later := cleanupClock.Add(265 * day)
	preview, err = fixture.store.PreviewCleanup(context.Background(), later)
	if err != nil || effectBasis(preview)[evidence.ID] != "evidence_retired_365_days" {
		t.Fatalf("new retirement not eligible at 365d: %+v err=%v", preview.Effects, err)
	}
}

func TestRetirementChangedAfterCleanupReviewDeniesAuthority(t *testing.T) {
	fixture := newCleanupFixture(t)
	evidence := fixture.create(t, 800*day, evidenceDraft)
	fixture.retire(t, evidence.ID, cleanupClock.Add(-400*day))
	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil || len(preview.Effects) != 1 || preview.Effects[0].Retirement == "" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	authority, err := detailartifact.AuthorizeCleanup(preview, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	fixture.create(t, day, func(draft *detailartifact.Draft) {
		draft.References = []detailartifact.Reference{{Kind: "artifact", Value: evidence.ID}}
	})
	if _, err := fixture.store.ApplyCleanup(context.Background(), preview, authority); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale cleanup accepted: %v", err)
	}
	if _, err := fixture.store.Read(context.Background(), evidence.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRetirementPublicationInterruptionRequiresRecovery(t *testing.T) {
	for _, stage := range []FaultStage{FaultF1, FaultF2, FaultF3, FaultF5, FaultF6} {
		t.Run(string(stage), func(t *testing.T) {
			fixture := newCleanupFixture(t)
			evidence := fixture.create(t, 800*day, evidenceDraft)
			preview, err := fixture.store.PreviewRetirement(context.Background(), cleanupClock, evidence.ID)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := detailartifact.AuthorizeRetirement(preview, preview.Digest)
			if err != nil {
				t.Fatal(err)
			}
			fixture.store.hooks.fault = func(current FaultStage) error {
				if current == stage {
					return ErrSimulatedInterruption
				}
				return nil
			}
			if _, err := fixture.store.ApplyRetirement(context.Background(), preview, authority); err == nil {
				t.Fatal("interrupted retirement reported success")
			}
			fixture.store.hooks.fault = nil
			report, err := InspectRecovery(context.Background(), RecoveryRoots{State: fixture.state})
			if err != nil {
				t.Fatal(err)
			}
			pending := false
			for _, plan := range report.Plans {
				pending = pending || plan.Directory == "artifacts/v1/retirements"
			}
			if stage == FaultF1 {
				// F1 precedes any staged byte: a pre-effect abort with no
				// retirement, so Evidence stays unretired.
				preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock.Add(800*day))
				if pending || err != nil || preservedReason(preview, evidence.ID) != "evidence_not_retired" {
					t.Fatalf("pending=%t reason=%q err=%v", pending, preservedReason(preview, evidence.ID), err)
				}
				return
			}
			if !pending {
				t.Fatalf("interruption not surfaced for recovery: %+v", report.Plans)
			}
			if _, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock.Add(800*day)); !errors.Is(err, ErrRecoveryRequired) {
				t.Fatalf("cleanup proceeded over pending retirement: %v", err)
			}
			if _, err := fixture.store.Read(context.Background(), evidence.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRetirementRecordIsInventoried(t *testing.T) {
	fixture := newCleanupFixture(t)
	evidence := fixture.create(t, 800*day, evidenceDraft)
	fixture.retire(t, evidence.ID, cleanupClock)
	inventory, err := InspectStateInventory(context.Background(), fixture.state)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range inventory.Entries {
		if entry.Relative == "artifacts/v1/retirements/"+evidence.ID+".json" {
			if entry.Kind != InventoryRetirementRecord {
				t.Fatalf("entry=%+v", entry)
			}
			return
		}
	}
	t.Fatalf("retirement missing from inventory: %+v", inventory.Entries)
}

func writeTestFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
