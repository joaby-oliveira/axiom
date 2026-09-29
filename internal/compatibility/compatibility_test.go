package compatibility

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/detailartifact"
	"github.com/rgomids/axiom/internal/local"
	"github.com/rgomids/axiom/internal/provenance"
)

const (
	pocFixture      = "testdata/poc-v0.1.0-poc.1"
	pocProjectID    = "46f9e9bf-9da2-4769-b9b7-f058f0ab6e90"
	contentSentinel = "axiom-s7-content-sentinel"
)

// copyFixture reproduces the historical tree with the private modes the POC
// wrote; Git does not preserve file modes.
func copyFixture(t *testing.T, trees ...string) Roots {
	t.Helper()
	base := privateDir(t, t.TempDir(), "roots")
	roots := Roots{}
	for _, tree := range trees {
		target := filepath.Join(base, tree)
		err := filepath.WalkDir(filepath.Join(pocFixture, tree), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(filepath.Join(pocFixture, tree), path)
			destination := filepath.Join(target, relative)
			if entry.IsDir() {
				return os.Mkdir(destination, 0o700)
			}
			wire, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(destination, wire, 0o600)
		})
		if err != nil {
			t.Fatal(err)
		}
		switch tree {
		case "projects":
			roots.Projects = target
		case "state":
			roots.State = target
		case "skills":
			roots.Skills = target
		}
	}
	return roots
}

func pocRoots(t *testing.T) Roots { return copyFixture(t, "projects", "state", "skills") }

func inspect(t *testing.T, roots Roots) Report {
	t.Helper()
	report, err := Inspect(context.Background(), roots)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	return report
}

func TestFixtureProvenanceMatchesHistoricalTag(t *testing.T) {
	wire, err := os.ReadFile(filepath.Join(pocFixture, "PROVENANCE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"tag=" + HistoricalPOCTag + "\n", "revision=" + HistoricalPOCRevision + "\n"} {
		if !strings.Contains(string(wire), expected) {
			t.Fatalf("fixture provenance missing %q", expected)
		}
	}
	if output, err := exec.Command("git", "rev-parse", HistoricalPOCTag+"^{commit}").Output(); err == nil {
		if strings.TrimSpace(string(output)) != HistoricalPOCRevision {
			t.Fatalf("tag revision = %s", output)
		}
	}
}

func TestRecognizedPOCFromHistoricalBinaryOutput(t *testing.T) {
	roots := pocRoots(t)
	before := treeHash(t, filepath.Dir(roots.State))
	report := inspect(t, roots)
	if report.Classification != RecognizedPOC || report.POCRevision != HistoricalPOCRevision || report.POCTag != HistoricalPOCTag {
		t.Fatalf("report=%+v", report)
	}
	if report.SkillSet.State != "upgradable" {
		t.Fatalf("POC skills must be the known legacy set: %+v", report.SkillSet)
	}
	found := false
	for _, finding := range report.Findings {
		if finding.Kind == string(local.InventoryPOCWorkflow) && finding.Relative == "workflows/"+pocProjectID+"/main-7.json" {
			found = true
		}
	}
	if !found {
		t.Fatalf("POC workflow signature not reported: %+v", report.Findings)
	}
	if after := treeHash(t, filepath.Dir(roots.State)); after != before {
		t.Fatal("read-only inspection changed the tree")
	}
	again := inspect(t, roots)
	if again.Digest != report.Digest {
		t.Fatal("inspection digest is not deterministic")
	}
}

func TestClassificationMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, Roots) Roots
		want   Classification
		reason string
	}{
		{"absent roots", func(t *testing.T, roots Roots) Roots {
			base := t.TempDir()
			return Roots{Projects: filepath.Join(base, "p"), State: filepath.Join(base, "s"), Skills: filepath.Join(base, "k")}
		}, AbsentV1, "no_owned_state"},
		{"empty private roots", func(t *testing.T, roots Roots) Roots {
			base := privateDir(t, t.TempDir(), "empty")
			return Roots{Projects: privateDir(t, base, "p"), State: privateDir(t, base, "s")}
		}, AbsentV1, "no_owned_state"},
		{"v1 readable shared formats", func(t *testing.T, roots Roots) Roots {
			removeAll(t, filepath.Join(roots.State, "workflows"))
			return roots
		}, ValidV1, "v1_readable_state"},
		{"v1 work item only", func(t *testing.T, roots Roots) Roots {
			removeAll(t, filepath.Join(roots.State, "workflows"))
			removeAll(t, filepath.Join(roots.State, "projects"))
			return Roots{State: roots.State}
		}, ValidV1, "v1_readable_state"},
		{"v1 artifact", func(t *testing.T, roots Roots) Roots {
			removeAll(t, filepath.Join(roots.State, "workflows"))
			createArtifact(t, roots.State)
			return roots
		}, ValidV1, "v1_readable_state"},
		{"mixed POC and v1", func(t *testing.T, roots Roots) Roots {
			createArtifact(t, roots.State)
			return roots
		}, Malformed, "mixed_poc_and_v1_state"},
		{"partial POC workflow", func(t *testing.T, roots Roots) Roots {
			path := filepath.Join(roots.State, "workflows", pocProjectID, "main-7.json")
			rewrite(t, path, strings.Replace(read(t, path), `"gate":"plan"`, `"gate":"planning"`, 1))
			return roots
		}, Malformed, "unrecognized_or_unsafe_content"},
		{"unknown state entry", func(t *testing.T, roots Roots) Roots {
			writePrivate(t, filepath.Join(roots.State, "notes.txt"), []byte("unknown\n"))
			return roots
		}, Malformed, "unrecognized_or_unsafe_content"},
		{"newer record", func(t *testing.T, roots Roots) Roots {
			path := filepath.Join(roots.State, "work-items", pocProjectID, "main-7.json")
			rewrite(t, path, strings.Replace(read(t, path), `"formatVersion":1`, `"formatVersion":2`, 1))
			return roots
		}, UnsupportedNewer, "newer_format_version"},
		{"newer execution layout", func(t *testing.T, roots Roots) Roots {
			privateDir(t, privateDir(t, roots.State, "executions"), "v2")
			return roots
		}, UnsupportedNewer, "newer_format_version"},
		{"newer portable schema", func(t *testing.T, roots Roots) Roots {
			path := filepath.Join(roots.Projects, "poc-fixture", "axiom.yaml")
			rewrite(t, path, strings.Replace(read(t, path), "schemaVersion: 1", "schemaVersion: 2", 1))
			return roots
		}, UnsupportedNewer, "newer_format_version"},
		{"older record", func(t *testing.T, roots Roots) Roots {
			path := filepath.Join(roots.State, "projects", pocProjectID, "installation.json")
			rewrite(t, path, strings.Replace(read(t, path), `"formatVersion":1`, `"formatVersion":0`, 1))
			return roots
		}, UnsupportedOlder, "older_format_version"},
		{"recovery marker wins", func(t *testing.T, roots Roots) Roots {
			path := filepath.Join(roots.State, "work-items", pocProjectID, "main-7.json")
			rewrite(t, path, strings.Replace(read(t, path), `"formatVersion":1`, `"formatVersion":2`, 1))
			writePrivate(t, filepath.Join(roots.State, "work-items", pocProjectID, ".axiom-recovery-0001"), []byte(contentSentinel))
			return roots
		}, RecoveryRequired, "interrupted_protocol_state"},
		{"S2 attempt leftover", func(t *testing.T, roots Roots) Roots {
			writePrivate(t, filepath.Join(roots.State, "projects", pocProjectID, ".lingo-attempt-install-0001"), []byte("{}\n"))
			return roots
		}, RecoveryRequired, "interrupted_protocol_state"},
		{"foreign Axiom skill", func(t *testing.T, roots Roots) Roots {
			rewrite(t, filepath.Join(roots.Skills, "axiom-project-show", "SKILL.md"), "operator edit\n")
			return roots
		}, Malformed, "unrecognized_or_unsafe_content"},
		{"unrelated skill is not Axiom-owned", func(t *testing.T, roots Roots) Roots {
			removeAll(t, filepath.Join(roots.State, "workflows"))
			writePrivate(t, filepath.Join(privateDir(t, roots.Skills, "someone-elses-skill"), "SKILL.md"), []byte("third party\n"))
			return roots
		}, ValidV1, "v1_readable_state"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roots := test.mutate(t, pocRoots(t))
			report := inspect(t, roots)
			if report.Classification != test.want || report.Reason != test.reason {
				t.Fatalf("classification=%s reason=%s want %s/%s findings=%+v", report.Classification, report.Reason, test.want, test.reason, report.Findings)
			}
			if len(report.Next) == 0 {
				t.Fatal("classification has no safe next action")
			}
			wire, _ := json.Marshal(report)
			if strings.Contains(string(wire), contentSentinel) {
				t.Fatal("file content leaked into the compatibility report")
			}
		})
	}
}

func TestUnsafeFilesystemFactsFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, Roots)
	}{
		{"symlink entry", func(t *testing.T, roots Roots) {
			if err := os.Symlink(t.TempDir(), filepath.Join(roots.State, "work-items", pocProjectID, "link.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{"permissive file mode", func(t *testing.T, roots Roots) {
			chmod(t, filepath.Join(roots.State, "work-items", pocProjectID, "main-7.json"), 0o644)
		}},
		{"permissive directory mode", func(t *testing.T, roots Roots) {
			chmod(t, filepath.Join(roots.State, "work-items", pocProjectID), 0o755)
		}},
		{"permissive root mode", func(t *testing.T, roots Roots) { chmod(t, roots.State, 0o755) }},
		{"hard link", func(t *testing.T, roots Roots) {
			if err := os.Link(filepath.Join(roots.State, "work-items", pocProjectID, "main-7.json"), filepath.Join(t.TempDir(), "alias")); err != nil {
				t.Skipf("hard links unavailable: %v", err)
			}
		}},
		{"oversized record", func(t *testing.T, roots Roots) {
			writePrivate(t, filepath.Join(roots.State, "work-items", pocProjectID, "large.json"), make([]byte, local.MaxRecordBytes+1))
		}},
		{"non-regular entry", func(t *testing.T, roots Roots) {
			if err := syscall.Mkfifo(filepath.Join(roots.State, "work-items", pocProjectID, "pipe.json"), 0o600); err != nil {
				t.Skipf("fifo unavailable: %v", err)
			}
		}},
	}
	if runtime.GOOS == "darwin" {
		tests = append(tests, struct {
			name   string
			mutate func(*testing.T, Roots)
		}{"permissive ACL", func(t *testing.T, roots Roots) {
			path := filepath.Join(roots.State, "work-items", pocProjectID, "main-7.json")
			if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", path).CombinedOutput(); err != nil {
				t.Fatalf("set synthetic ACL: %v: %s", err, output)
			}
		}})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roots := pocRoots(t)
			removeAll(t, filepath.Join(roots.State, "workflows"))
			test.mutate(t, roots)
			before := treeHash(t, roots.State)
			report := inspect(t, roots)
			if report.Classification != Malformed {
				t.Fatalf("unsafe fact classified %s: %+v", report.Classification, report.Findings)
			}
			if after := treeHash(t, roots.State); after != before {
				t.Fatal("fail-closed inspection mutated state")
			}
		})
	}
}

func TestInspectRejectsUnsafeRootsWithoutEffects(t *testing.T) {
	for _, roots := range []Roots{{State: "relative"}, {Projects: "/"}, {Skills: "skills"}} {
		if _, err := Inspect(context.Background(), roots); !errors.Is(err, ErrUnsafeRoot) {
			t.Fatalf("roots %+v error=%v", roots, err)
		}
	}
}

func TestBackupAndExportUseSeparateExactAuthorities(t *testing.T) {
	roots := pocRoots(t)
	before := treeHash(t, filepath.Dir(roots.State))
	backupTarget := filepath.Join(privateDir(t, filepath.Dir(filepath.Dir(roots.State)), "out"), "backup")
	backup, err := PreviewTransfer(context.Background(), Backup, roots, backupTarget)
	if err != nil || backup.State != TransferReady || len(backup.Effects) != 9 || len(backup.Omitted) != 0 {
		t.Fatalf("backup=%+v err=%v", backup, err)
	}
	if _, err := os.Lstat(backupTarget); !os.IsNotExist(err) {
		t.Fatal("preview created the target")
	}
	export, err := PreviewTransfer(context.Background(), Export, roots, filepath.Join(filepath.Dir(backupTarget), "export"))
	if err != nil || len(export.Effects) != 1 || export.Effects[0].Target != "projects/poc-fixture/axiom.yaml" || len(export.Omitted) != 8 {
		t.Fatalf("export=%+v err=%v", export, err)
	}
	if _, err := AuthorizeTransfer(export, backup.Digest); err == nil {
		t.Fatal("backup digest authorized export")
	}
	backupAuthority, err := AuthorizeTransfer(backup, backup.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyTransfer(context.Background(), export, backupAuthority); !errors.Is(err, ErrTransferStale) {
		t.Fatalf("backup authority applied export: %v", err)
	}
	result, err := ApplyTransfer(context.Background(), backup, backupAuthority)
	if err != nil || result.Status != "complete" || len(result.Written) != 9 || !result.ManifestWritten {
		t.Fatalf("backup result=%+v err=%v", result, err)
	}
	assertPrivateTree(t, backupTarget, 10)
	exportAuthority, _ := AuthorizeTransfer(export, export.Digest)
	if _, err := ApplyTransfer(context.Background(), export, exportAuthority); err != nil {
		t.Fatal(err)
	}
	assertPrivateTree(t, export.Target, 2)
	if _, err := os.Lstat(filepath.Join(export.Target, "state")); !os.IsNotExist(err) {
		t.Fatal("machine-local state entered portable export")
	}
	store, err := local.NewPortableStore(filepath.Join(export.Target, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := store.Inspect(context.Background(), "poc-fixture")
	if err != nil || !observation.Exists || observation.Snapshot.Project().State().ID != pocProjectID {
		t.Fatalf("exported Project is not v1-readable: %+v err=%v", observation, err)
	}
	if after := treeHash(t, filepath.Dir(roots.State)); after != before {
		t.Fatal("transfer mutated source state")
	}
	retry, err := PreviewTransfer(context.Background(), Backup, roots, backupTarget)
	if err != nil || retry.State != TransferComplete {
		t.Fatalf("equivalent retry is not a no-op: %+v err=%v", retry, err)
	}
	if _, err := AuthorizeTransfer(retry, retry.Digest); err == nil {
		t.Fatal("complete transfer authorized a second write")
	}
}

func TestTransferRejectsUnsafeTargetsAndStaleSource(t *testing.T) {
	roots := pocRoots(t)
	parent := privateDir(t, t.TempDir(), "parent")
	occupied := privateDir(t, parent, "occupied")
	writePrivate(t, filepath.Join(occupied, "foreign"), []byte("preserve"))
	for _, target := range []string{occupied, "relative/backup", "/", filepath.Join(roots.State, "inside"), filepath.Dir(roots.State), filepath.Join(parent, ".hidden")} {
		if _, err := PreviewTransfer(context.Background(), Backup, roots, target); err == nil {
			t.Fatalf("unsafe target %q accepted", target)
		}
	}
	if wire := read(t, filepath.Join(occupied, "foreign")); wire != "preserve" {
		t.Fatal("occupied target changed")
	}
	removeAll(t, filepath.Join(roots.State, "workflows"))
	if _, err := PreviewTransfer(context.Background(), Backup, roots, filepath.Join(parent, "v1")); !errors.Is(err, ErrTransferSource) {
		t.Fatalf("v1 state accepted as POC source: %v", err)
	}
	roots = pocRoots(t)
	target := filepath.Join(parent, "backup")
	preview, err := PreviewTransfer(context.Background(), Backup, roots, target)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := AuthorizeTransfer(preview, preview.Digest)
	path := filepath.Join(roots.State, "work-items", pocProjectID, "main-7.json")
	rewrite(t, path, strings.Replace(read(t, path), `"OPEN"`, `"CLOSED"`, 1))
	if _, err := ApplyTransfer(context.Background(), preview, authority); !errors.Is(err, ErrTransferStale) {
		t.Fatalf("stale source accepted: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("stale authority created the target")
	}
}

func TestTransferCapacityAndInterruptionPreserveSource(t *testing.T) {
	roots := pocRoots(t)
	parent := privateDir(t, t.TempDir(), "parent")
	original := availableSpace
	availableSpace = func(string) (uint64, error) { return 1024, nil }
	if _, err := PreviewTransfer(context.Background(), Backup, roots, filepath.Join(parent, "full")); !errors.Is(err, ErrTransferCapacity) {
		t.Fatalf("insufficient space accepted: %v", err)
	}
	availableSpace = original
	before := treeHash(t, filepath.Dir(roots.State))
	target := filepath.Join(parent, "partial")
	preview, err := PreviewTransfer(context.Background(), Backup, roots, target)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := AuthorizeTransfer(preview, preview.Digest)
	beforeTransferWrite = func(index int) error {
		if index == 2 {
			return syscall.ENOSPC
		}
		return nil
	}
	defer func() { beforeTransferWrite = nil }()
	result, err := ApplyTransfer(context.Background(), preview, authority)
	if !errors.Is(err, syscall.ENOSPC) || result.Status != "partial" || len(result.Written) != 2 || result.ManifestWritten {
		t.Fatalf("partial result=%+v err=%v", result, err)
	}
	if _, err := os.Lstat(filepath.Join(target, manifestFile)); !os.IsNotExist(err) {
		t.Fatal("incomplete transfer published a manifest")
	}
	if after := treeHash(t, filepath.Dir(roots.State)); after != before {
		t.Fatal("interrupted transfer mutated source")
	}
	if _, err := PreviewTransfer(context.Background(), Backup, roots, target); !errors.Is(err, ErrTransferTarget) {
		t.Fatalf("incomplete target adopted: %v", err)
	}
	beforeTransferWrite = nil
	retry, err := PreviewTransfer(context.Background(), Backup, roots, filepath.Join(parent, "retry"))
	if err != nil {
		t.Fatal(err)
	}
	retryAuthority, _ := AuthorizeTransfer(retry, retry.Digest)
	if result, err := ApplyTransfer(context.Background(), retry, retryAuthority); err != nil || result.Status != "complete" {
		t.Fatalf("retry result=%+v err=%v", result, err)
	}
}

func createArtifact(t *testing.T, state string) {
	t.Helper()
	store, err := local.NewArtifactStore(state)
	if err != nil {
		t.Fatal(err)
	}
	source, err := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "123456789abc", SourceState: provenance.Clean}, nil)
	if err != nil {
		t.Fatal(err)
	}
	draft := detailartifact.Draft{CorrelationID: "123e4567-e89b-42d3-a456-426614174000", Category: "diagnostic", Outcome: "failure", Retention: detailartifact.Diagnostic, Markdown: []byte("# Detail\n"), Provenance: source}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := store.Create(ctx, draft); err != nil {
		t.Fatal(err)
	}
}

func assertPrivateTree(t *testing.T, root string, files int) {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() && info.Mode().Perm() != 0o700 || !entry.IsDir() && info.Mode().Perm() != 0o600 || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("unsafe transfer mode %s for %s", info.Mode(), path)
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if err != nil || count != files {
		t.Fatalf("transfer files=%d want=%d err=%v", count, files, err)
	}
}

func privateDir(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePrivate(t *testing.T, path string, wire []byte) {
	t.Helper()
	if err := os.WriteFile(path, wire, 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	wire, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(wire)
}

func rewrite(t *testing.T, path, content string) {
	t.Helper()
	writePrivate(t, path, []byte(content))
}

func removeAll(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func treeHash(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		hash.Write([]byte(relative))
		hash.Write([]byte(info.Mode().String()))
		if info.Mode().IsRegular() {
			wire, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash.Write(wire)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
