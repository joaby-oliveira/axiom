package local

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/workflow"
)

var cleanupClock = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

type cleanupFixture struct {
	state string
	store ArtifactStore
	next  int
}

func newCleanupFixture(t *testing.T) *cleanupFixture {
	t.Helper()
	state := filepath.Join(privateTestRoot(t), "state")
	store, err := NewArtifactStore(state)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &cleanupFixture{state: state, store: store}
	fixture.store.allocate = func() (string, error) {
		fixture.next++
		return fmt.Sprintf("123e4567-e89b-42d3-a456-%012d", fixture.next), nil
	}
	return fixture
}

func (f *cleanupFixture) create(t *testing.T, age time.Duration, mutate func(*detailartifact.Draft)) detailartifact.Artifact {
	t.Helper()
	created := cleanupClock.Add(-age)
	f.store.now = func() (time.Time, error) { return created, nil }
	draft := artifactDraft(t)
	if mutate != nil {
		mutate(&draft)
	}
	artifact, err := f.store.Create(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	f.store.now = func() (time.Time, error) { return cleanupClock, nil }
	return artifact
}

func (f *cleanupFixture) execution(t *testing.T, status workflow.ExecutionStatus, executionID string, references ...workflow.Reference) {
	t.Helper()
	store, err := NewWorkflowStore(f.state)
	if err != nil {
		t.Fatal(err)
	}
	state := advancedExecution(validExecution())
	if status == workflow.ExecutionInterrupted {
		state = interruptedExecution(validExecution())
	}
	state.ExecutionID = executionID
	f.next++
	state.WorkItem.ExternalID = fmt.Sprint(1000 + f.next)
	state.WorkItem.URL = "https://github.com/owner/repo/issues/" + state.WorkItem.ExternalID
	state.Transitions[0].References = references
	if err := store.Create(context.Background(), state); err != nil {
		t.Fatal(err)
	}
}

func preservedReason(preview detailartifact.CleanupPreview, id string) string {
	for _, entry := range preview.Preserved() {
		if value, reason, ok := strings.Cut(entry, ":"); ok && value == id {
			return reason
		}
	}
	return ""
}

func TestArtifactCleanupEligibilityMatrixUsesAuthoritativeReferences(t *testing.T) {
	fixture := newCleanupFixture(t)
	eligible := fixture.create(t, 31*24*time.Hour, nil)
	boundary := fixture.create(t, detailartifact.DiagnosticRetention, nil)
	young := fixture.create(t, detailartifact.DiagnosticRetention-time.Second, nil)
	active := fixture.create(t, 400*24*time.Hour, func(draft *detailartifact.Draft) { draft.Retention = detailartifact.Active })
	evidence := fixture.create(t, 400*24*time.Hour, func(draft *detailartifact.Draft) { draft.Retention = detailartifact.Evidence })
	review := fixture.create(t, 400*24*time.Hour, func(draft *detailartifact.Draft) { draft.Retention = detailartifact.PreservedReview })
	live := fixture.create(t, 400*24*time.Hour, func(draft *detailartifact.Draft) { draft.LiveReferences = []string{"review:open"} })
	crossTarget := fixture.create(t, 400*24*time.Hour, nil)
	fixture.create(t, time.Hour, func(draft *detailartifact.Draft) {
		draft.References = []detailartifact.Reference{{Kind: "artifact", Value: crossTarget.ID}}
	})
	referenced := fixture.create(t, 400*24*time.Hour, nil)
	fixture.execution(t, workflow.ExecutionActive, "123e4567-e89b-42d3-a456-000000000a11", workflow.Reference{Kind: "artifact", ID: referenced.ID, Digest: hex.EncodeToString(referenced.Digest[:])})
	openExecution := "123e4567-e89b-42d3-a456-000000000a12"
	owned := fixture.create(t, 400*24*time.Hour, func(draft *detailartifact.Draft) {
		draft.ExecutionID, draft.CorrelationID = openExecution, ""
	})
	fixture.execution(t, workflow.ExecutionInterrupted, openExecution)
	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil {
		t.Fatal(err)
	}
	effects := map[string]bool{}
	for _, effect := range preview.Effects {
		effects[effect.ID] = true
		if effect.Kind != detailartifact.EffectArtifact || effect.Basis != "diagnostic_unreferenced_30_days" {
			t.Fatalf("effect=%+v", effect)
		}
	}
	if len(effects) != 2 || !effects[eligible.ID] || !effects[boundary.ID] {
		t.Fatalf("effects=%+v", preview.Effects)
	}
	for id, reason := range map[string]string{young.ID: "diagnostic_within_30_days", active.ID: "active", evidence.ID: "evidence_not_retired", review.ID: "preserved_review", live.ID: "referenced", crossTarget.ID: "referenced", referenced.ID: "referenced", owned.ID: "referenced"} {
		if got := preservedReason(preview, id); got != reason {
			t.Fatalf("artifact %s reason=%q want %q", id, got, reason)
		}
	}
	if preview.ReclaimedBytes != int64(eligible.ContentBytes+boundary.ContentBytes) {
		t.Fatalf("reclaimed=%d", preview.ReclaimedBytes)
	}
	again, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil || again.Digest != preview.Digest {
		t.Fatal("cleanup preview is not deterministic")
	}
}

func TestArtifactCleanupRequiresExactAuthorityAndRecordsBoundedAudit(t *testing.T) {
	fixture := newCleanupFixture(t)
	removed := fixture.create(t, 31*24*time.Hour, nil)
	kept := fixture.create(t, time.Hour, nil)
	outside := filepath.Join(filepath.Dir(fixture.state), "outside")
	if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := detailartifact.AuthorizeCleanup(preview, "stale"); err == nil {
		t.Fatal("stale digest authorized cleanup")
	}
	if _, err := fixture.store.ApplyCleanup(context.Background(), preview, detailartifact.CleanupAuthority{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing authority accepted: %v", err)
	}
	authority, err := detailartifact.AuthorizeCleanup(preview, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.store.ApplyCleanup(context.Background(), preview, authority)
	if err != nil || len(result.Removed) != 1 || result.Partial || !result.AuditRecorded || result.ReclaimedBytes != int64(removed.ContentBytes) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := fixture.store.Read(context.Background(), removed.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed artifact still readable: %v", err)
	}
	if _, err := fixture.store.Read(context.Background(), kept.ID); err != nil {
		t.Fatal(err)
	}
	wire, err := os.ReadFile(filepath.Join(fixture.state, "artifacts", "v1", "cleanup", result.RecordID))
	if err != nil || len(wire) > detailartifact.MaxCleanupRecord || !bytes.Contains(wire, []byte(`"status":"confirmed"`)) || bytes.Contains(wire, []byte("Synthetic diagnosis")) {
		t.Fatalf("record=%s err=%v", wire, err)
	}
	if content, _ := os.ReadFile(outside); string(content) != "unchanged" {
		t.Fatal("cleanup changed content outside the artifact root")
	}
	if _, err := fixture.store.ApplyCleanup(context.Background(), preview, authority); !errors.Is(err, ErrConflict) {
		t.Fatalf("replayed authority accepted: %v", err)
	}
}

func TestArtifactCleanupStaleReferenceAndConcurrentWriterDenyAuthority(t *testing.T) {
	fixture := newCleanupFixture(t)
	artifact := fixture.create(t, 31*24*time.Hour, nil)
	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := detailartifact.AuthorizeCleanup(preview, preview.Digest)
	root, err := existingPrivateRoot(fixture.state)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := lockDirectory(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ApplyCleanup(context.Background(), preview, authority); !errors.Is(err, ErrConflict) {
		t.Fatalf("cleanup ignored a concurrent writer lock: %v", err)
	}
	lock.Close()
	root.Close()
	fixture.execution(t, workflow.ExecutionActive, "123e4567-e89b-42d3-a456-000000000a21", workflow.Reference{Kind: "artifact", ID: artifact.ID, Digest: hex.EncodeToString(artifact.Digest[:])})
	if _, err := fixture.store.ApplyCleanup(context.Background(), preview, authority); !errors.Is(err, ErrConflict) {
		t.Fatalf("new live reference did not stale the authority: %v", err)
	}
	if _, err := fixture.store.Read(context.Background(), artifact.ID); err != nil {
		t.Fatal("referenced artifact removed")
	}
}

func TestArtifactCleanupFailsClosedOnUncertainReferenceState(t *testing.T) {
	fixture := newCleanupFixture(t)
	fixture.create(t, 31*24*time.Hour, nil)
	fixture.execution(t, workflow.ExecutionActive, "123e4567-e89b-42d3-a456-000000000a31")
	matches, _ := filepath.Glob(filepath.Join(fixture.state, "executions", "v1", "*", "*.json"))
	if len(matches) != 1 {
		t.Fatalf("executions=%v", matches)
	}
	if err := os.WriteFile(matches[0], []byte("{corrupt}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("corrupt Execution reference state was not fail-closed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(matches[0]), ".axiom-recovery-pending"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("pending Execution protocol state was not fail-closed: %v", err)
	}
}

func TestArtifactCleanupPartialRemovalIsTruthfulAndRecorded(t *testing.T) {
	fixture := newCleanupFixture(t)
	first := fixture.create(t, 31*24*time.Hour, nil)
	second := fixture.create(t, 31*24*time.Hour, nil)
	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil || len(preview.Effects) != 2 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	authority, _ := detailartifact.AuthorizeCleanup(preview, preview.Digest)
	injected := errors.New("injected interruption")
	fixture.store.beforeCleanupRemoval = func(index int) error {
		if index == 1 {
			return injected
		}
		return nil
	}
	result, err := fixture.store.ApplyCleanup(context.Background(), preview, authority)
	if !errors.Is(err, injected) || !result.Partial || len(result.Removed) != 1 || result.Removed[0].ID != first.ID || !result.AuditRecorded {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := fixture.store.Read(context.Background(), second.ID); err != nil {
		t.Fatal("unremoved artifact damaged")
	}
	wire, _ := os.ReadFile(filepath.Join(fixture.state, "artifacts", "v1", "cleanup", result.RecordID))
	if !bytes.Contains(wire, []byte(`"status":"partial"`)) || !bytes.Contains(wire, []byte(first.ID)) || bytes.Contains(wire, []byte(`"id":"`+second.ID)) {
		t.Fatalf("partial record=%s", wire)
	}
}

func TestCleanupRecordRetentionAndBatchBounds(t *testing.T) {
	fixture := newCleanupFixture(t)
	for index := 0; index < detailartifact.MaxCleanupBatch+2; index++ {
		fixture.create(t, 31*24*time.Hour, nil)
	}
	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil || len(preview.Effects) != detailartifact.MaxCleanupBatch || preview.RemainingEligible != 2 {
		t.Fatalf("batch preview effects=%d remaining=%d err=%v", len(preview.Effects), preview.RemainingEligible, err)
	}
	authority, _ := detailartifact.AuthorizeCleanup(preview, preview.Digest)
	result, err := fixture.store.ApplyCleanup(context.Background(), preview, authority)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(fixture.state, "artifacts", "v1", "cleanup", result.RecordID))
	if err != nil || info.Size() > detailartifact.MaxCleanupRecord || info.Mode().Perm() != 0o600 {
		t.Fatalf("record info=%v err=%v", info, err)
	}
	for _, check := range []struct {
		at        time.Duration
		retention bool
	}{{detailartifact.CleanupRetention - time.Minute, true}, {detailartifact.CleanupRetention + time.Minute, false}} {
		preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock.Add(check.at))
		if err != nil {
			t.Fatal(err)
		}
		recordEffect := false
		for _, effect := range preview.Effects {
			if effect.Kind == detailartifact.EffectCleanupRecord && effect.ID == result.RecordID {
				recordEffect = true
			}
		}
		if recordEffect == check.retention || check.retention && preservedReason(preview, result.RecordID) != "cleanup_record_within_90_days" {
			t.Fatalf("record retention at %s: effect=%v preserved=%q", check.at, recordEffect, preservedReason(preview, result.RecordID))
		}
	}
}

func TestCapacityExhaustionNeverEvictsAndExplicitCleanupRestoresCapacity(t *testing.T) {
	fixture := newCleanupFixture(t)
	old := fixture.create(t, 31*24*time.Hour, nil)
	fixture.store.capacity = func(root *os.Root) (int, int64, error) { return detailartifact.MaxLiveArtifacts, 0, nil }
	fixture.store.now = func() (time.Time, error) { return cleanupClock, nil }
	if _, err := fixture.store.Create(context.Background(), artifactDraft(t)); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity exhaustion error=%v", err)
	}
	if _, err := fixture.store.Read(context.Background(), old.ID); err != nil {
		t.Fatal("capacity exhaustion evicted an artifact")
	}
	fixture.store.capacity = nil
	preview, err := fixture.store.PreviewCleanup(context.Background(), cleanupClock)
	if err != nil || len(preview.Effects) != 1 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	authority, _ := detailartifact.AuthorizeCleanup(preview, preview.Digest)
	if _, err := fixture.store.ApplyCleanup(context.Background(), preview, authority); err != nil {
		t.Fatal(err)
	}
}

// recoveryFixture interrupts a real file publication at one fault stage.
func recoveryFixture(t *testing.T, stage FaultStage, create bool) (string, string, []byte, []byte) {
	t.Helper()
	state := filepath.Join(privateTestRoot(t), "state")
	directory := filepath.Join(state, "work-items", "123e4567-e89b-42d3-a456-426614174000")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := existingPrivateRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	prior, next := []byte("{\"prior\":true}\n"), []byte("{\"next\":true}\n")
	if !create {
		if err := writePrivateFile(root, "main-7.json", prior); err != nil {
			t.Fatal(err)
		}
	}
	hooks := publicationHooks{fault: func(current FaultStage) error {
		if current == stage {
			return ErrSimulatedInterruption
		}
		return nil
	}}
	expected := prior
	if create {
		expected = nil
	}
	if err := publishFile(context.Background(), root, "main-7.json", expected, next, create, hooks); err == nil {
		t.Fatalf("interruption at %s did not stop publication", stage)
	}
	return state, directory, prior, next
}

func singlePlan(t *testing.T, roots RecoveryRoots) RecoveryPlan {
	t.Helper()
	report, err := InspectRecovery(context.Background(), roots)
	if err != nil || len(report.Plans) != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	return report.Plans[0]
}

func TestGuidedRecoveryCoversRecognizedFileFaultStages(t *testing.T) {
	for _, test := range []struct {
		stage  FaultStage
		create bool
		want   RecoveryAction
	}{
		{FaultF3, false, RestorePrior}, {FaultF4, false, RestorePrior}, {FaultF5, false, RestorePrior},
		{FaultF3, true, RestorePrior}, {FaultF5, true, RestorePrior},
		{FaultF6, false, FinalizeCommitted}, {FaultF7, false, FinalizeCommitted}, {FaultF8, false, FinalizeCommitted},
		{FaultF6, true, FinalizeCommitted},
	} {
		t.Run(fmt.Sprintf("%s-create-%t", test.stage, test.create), func(t *testing.T) {
			state, directory, prior, next := recoveryFixture(t, test.stage, test.create)
			roots := RecoveryRoots{State: state}
			plan := singlePlan(t, roots)
			if plan.Action != test.want || plan.Protocol != protocolFile || plan.Directory != "work-items/123e4567-e89b-42d3-a456-426614174000" {
				t.Fatalf("plan=%+v", plan)
			}
			if _, err := AuthorizeRecovery(plan, "stale"); err == nil {
				t.Fatal("stale digest authorized recovery")
			}
			authority, err := AuthorizeRecovery(plan, plan.Digest)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ApplyRecovery(context.Background(), roots, plan, authority)
			if err != nil || result.Action != test.want {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			wire, readErr := os.ReadFile(filepath.Join(directory, "main-7.json"))
			switch {
			case test.want == FinalizeCommitted && string(wire) != string(next):
				t.Fatalf("committed generation rolled back: %q", wire)
			case test.want == RestorePrior && test.create && !os.IsNotExist(readErr):
				t.Fatalf("pre-commit create left canonical state: %v", readErr)
			case test.want == RestorePrior && !test.create && string(wire) != string(prior):
				t.Fatalf("prior generation not canonical: %q", wire)
			}
			names, _ := os.ReadDir(directory)
			for _, name := range names {
				if strings.HasPrefix(name.Name(), ".") {
					t.Fatalf("protocol residue %s remained", name.Name())
				}
			}
			report, err := InspectRecovery(context.Background(), roots)
			if err != nil || len(report.Plans) != 0 {
				t.Fatalf("recovery was not idempotent: %+v %v", report, err)
			}
		})
	}
}

func TestGuidedRecoveryStageRemovedMarkerLeftRestoresPrior(t *testing.T) {
	state, directory, prior, _ := recoveryFixture(t, FaultF3, false)
	stages, _ := filepath.Glob(filepath.Join(directory, ".axiom-stage-file-*"))
	if len(stages) != 1 {
		t.Fatalf("stages=%v", stages)
	}
	if err := os.Remove(stages[0]); err != nil {
		t.Fatal(err)
	}
	roots := RecoveryRoots{State: state}
	plan := singlePlan(t, roots)
	authority, _ := AuthorizeRecovery(plan, plan.Digest)
	if plan.Action != RestorePrior {
		t.Fatalf("plan=%+v", plan)
	}
	if _, err := ApplyRecovery(context.Background(), roots, plan, authority); err != nil {
		t.Fatal(err)
	}
	if wire, _ := os.ReadFile(filepath.Join(directory, "main-7.json")); string(wire) != string(prior) {
		t.Fatalf("canonical=%q", wire)
	}
}

func TestGuidedRecoveryPreservesAmbiguousCorruptAndUnknownState(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{"contradictory committed canonical", func(t *testing.T, directory string) {
			if err := os.WriteFile(filepath.Join(directory, "main-7.json"), []byte("{\"third\":true}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"corrupt marker", func(t *testing.T, directory string) {
			markers, _ := filepath.Glob(filepath.Join(directory, ".axiom-recovery-*"))
			if err := os.WriteFile(markers[0], []byte("not-json\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"second marker", func(t *testing.T, directory string) {
			if err := os.WriteFile(filepath.Join(directory, ".axiom-recovery-second"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unknown protocol object", func(t *testing.T, directory string) {
			if err := os.WriteFile(filepath.Join(directory, ".axiom-stage-unknown"), []byte("?\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsafe stage link", func(t *testing.T, directory string) {
			stages, _ := filepath.Glob(filepath.Join(directory, ".axiom-stage-file-*"))
			if len(stages) == 0 {
				markers, _ := filepath.Glob(filepath.Join(directory, ".axiom-recovery-*"))
				marker, _ := os.ReadFile(markers[0])
				t.Fatalf("no stage for link test: %s", marker)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stage := FaultF6
			if test.name == "unsafe stage link" {
				stage = FaultF3
			}
			state, directory, _, _ := recoveryFixture(t, stage, false)
			test.mutate(t, directory)
			if test.name == "unsafe stage link" {
				stages, _ := filepath.Glob(filepath.Join(directory, ".axiom-stage-file-*"))
				if err := os.Remove(stages[0]); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), stages[0]); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotDirectory(t, directory)
			plan := singlePlan(t, RecoveryRoots{State: state})
			if plan.Action != PreservedReview {
				t.Fatalf("ambiguous state selected %s: %+v", plan.Action, plan)
			}
			if _, err := AuthorizeRecovery(plan, plan.Digest); err == nil {
				t.Fatal("preserved review plan authorized")
			}
			if after := snapshotDirectory(t, directory); after != before {
				t.Fatal("inspection changed preserved state")
			}
		})
	}
}

func TestGuidedRecoveryRejectsStaleAuthorityAndConcurrentWriter(t *testing.T) {
	state, directory, _, _ := recoveryFixture(t, FaultF3, false)
	roots := RecoveryRoots{State: state}
	plan := singlePlan(t, roots)
	authority, _ := AuthorizeRecovery(plan, plan.Digest)
	root, err := existingPrivateRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := lockDirectory(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRecovery(context.Background(), roots, plan, authority); !errors.Is(err, ErrConflict) {
		t.Fatalf("recovery ignored concurrent writer: %v", err)
	}
	lock.Close()
	root.Close()
	stages, _ := filepath.Glob(filepath.Join(directory, ".axiom-stage-file-*"))
	if err := os.WriteFile(stages[0], []byte("{\"changed\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotDirectory(t, directory)
	if _, err := ApplyRecovery(context.Background(), roots, plan, authority); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale recovery authority accepted: %v", err)
	}
	if after := snapshotDirectory(t, directory); after != before {
		t.Fatal("stale authority changed state")
	}
}

func TestGuidedRecoveryDirectoryPublications(t *testing.T) {
	for _, test := range []struct {
		stage FaultStage
		want  RecoveryAction
	}{{FaultF3, RestorePrior}, {FaultF5, RestorePrior}, {FaultF6, FinalizeCommitted}} {
		t.Run("artifact-"+string(test.stage), func(t *testing.T) {
			store := artifactStore(t)
			store.hooks.fault = func(current FaultStage) error {
				if current == test.stage {
					return ErrSimulatedInterruption
				}
				return nil
			}
			if _, err := store.Create(context.Background(), artifactDraft(t)); err == nil {
				t.Fatal("interrupted artifact creation reported success")
			}
			store.hooks.fault = nil
			roots := RecoveryRoots{State: store.root}
			plan := singlePlan(t, roots)
			if plan.Action != test.want || plan.Protocol != protocolDirectory {
				t.Fatalf("plan=%+v", plan)
			}
			authority, _ := AuthorizeRecovery(plan, plan.Digest)
			if _, err := ApplyRecovery(context.Background(), roots, plan, authority); err != nil {
				t.Fatal(err)
			}
			_, err := store.Read(context.Background(), testArtifactID)
			if test.want == FinalizeCommitted && err != nil || test.want == RestorePrior && !errors.Is(err, ErrNotFound) {
				t.Fatalf("post-recovery read error=%v", err)
			}
		})
	}
	t.Run("portable create pre-commit", func(t *testing.T) {
		projects := privateTestRoot(t)
		root, err := existingPrivateRoot(projects)
		if err != nil {
			t.Fatal(err)
		}
		manifestWire, err := os.ReadFile(filepath.Join("..", "compatibility", "testdata", "poc-v0.1.0-poc.1", "projects", "poc-fixture", "axiom.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		stageName := ".lingo-stage-poc-fixture-0001"
		stage, err := privateChild(root, stageName)
		if err != nil {
			t.Fatal(err)
		}
		if err := writePrivateFile(stage, manifestName, manifestWire); err != nil {
			t.Fatal(err)
		}
		stage.Close()
		if _, err := writeProtocolMarker(root, "poc-fixture", stageName, false, [32]byte{}, digestBytes(manifestWire), publicationHooks{}); err != nil {
			t.Fatal(err)
		}
		root.Close()
		roots := RecoveryRoots{Projects: projects}
		plan := singlePlan(t, roots)
		if plan.Action != RestorePrior || plan.Scope != RecoveryScopeProjects || plan.Protocol != protocolDirectory {
			t.Fatalf("plan=%+v", plan)
		}
		authority, _ := AuthorizeRecovery(plan, plan.Digest)
		if _, err := ApplyRecovery(context.Background(), roots, plan, authority); err != nil {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(projects)
		if len(entries) != 0 {
			t.Fatalf("residue after portable recovery: %v", entries)
		}
	})
}

func TestRecoveryAttemptLeftoversArePreservedForReview(t *testing.T) {
	state := filepath.Join(privateTestRoot(t), "state")
	directory := filepath.Join(state, "projects", "123e4567-e89b-42d3-a456-426614174000")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".lingo-attempt-install-0001"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := singlePlan(t, RecoveryRoots{State: state})
	if plan.Action != PreservedReview || plan.Protocol != protocolAttempt {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestInventoryEntryBoundFailsClosed(t *testing.T) {
	state := filepath.Join(privateTestRoot(t), "state")
	directory := filepath.Join(state, "work-items", "123e4567-e89b-42d3-a456-426614174000")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 6; index++ {
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("main-%d.json", index)), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	original := inventoryEntryLimit
	inventoryEntryLimit = 5
	defer func() { inventoryEntryLimit = original }()
	if _, err := InspectStateInventory(context.Background(), state); !errors.Is(err, ErrInventoryBound) {
		t.Fatalf("bounded inventory error=%v", err)
	}
}

func snapshotDirectory(t *testing.T, directory string) string {
	t.Helper()
	var builder strings.Builder
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, _ := os.Lstat(filepath.Join(directory, entry.Name()))
		builder.WriteString(entry.Name() + info.Mode().String())
		if info.Mode().IsRegular() {
			wire, _ := os.ReadFile(filepath.Join(directory, entry.Name()))
			builder.Write(wire)
		}
	}
	return builder.String()
}

// BenchmarkMaintenanceScanCost measures the bounded read cost of cleanup
// preview and compatibility inventory over 1,000 live artifacts.
func BenchmarkMaintenanceScanCost(b *testing.B) {
	state := filepath.Join(b.TempDir(), "state")
	store, err := NewArtifactStore(state)
	if err != nil {
		b.Fatal(err)
	}
	next := 0
	store.allocate = func() (string, error) {
		next++
		return fmt.Sprintf("123e4567-e89b-42d3-a456-%012d", next), nil
	}
	store.now = func() (time.Time, error) { return cleanupClock.Add(-31 * 24 * time.Hour), nil }
	for index := 0; index < 1000; index++ {
		if _, err := store.Create(context.Background(), artifactDraft(b)); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("cleanup-preview", func(b *testing.B) {
		for index := 0; index < b.N; index++ {
			if _, err := store.PreviewCleanup(context.Background(), cleanupClock); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("state-inventory", func(b *testing.B) {
		for index := 0; index < b.N; index++ {
			if _, err := InspectStateInventory(context.Background(), state); err != nil {
				b.Fatal(err)
			}
		}
	})
}
