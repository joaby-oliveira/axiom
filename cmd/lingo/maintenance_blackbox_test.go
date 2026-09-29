package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const maintenanceSentinel = "AXIOM_S7_BLACKBOX_SENTINEL_71c2"

type maintenanceEvent struct {
	Status      string          `json:"status"`
	Category    string          `json:"category"`
	Result      string          `json:"result"`
	Next        string          `json:"next"`
	Maintenance json.RawMessage `json:"maintenance"`
}

type maintenanceHarness struct {
	binary string
	env    []string
	base   string
}

func newMaintenanceHarness(t *testing.T) maintenanceHarness {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "lingo")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build executable: %v: %s", err, output)
	}
	base := filepath.Join(t.TempDir(), "roots")
	if err := os.Mkdir(base, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join("..", "..", "internal", "compatibility", "testdata", "poc-v0.1.0-poc.1")
	for _, tree := range []string{"projects", "state", "skills"} {
		err := filepath.WalkDir(filepath.Join(fixture, tree), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(fixture, path)
			if entry.IsDir() {
				return os.Mkdir(filepath.Join(base, relative), 0o700)
			}
			wire, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(base, relative), wire, 0o600)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(), "LINGO_PROJECTS_ROOT="+filepath.Join(base, "projects"), "LINGO_STATE_ROOT="+filepath.Join(base, "state"), "AXIOM_CODEX_SKILLS_ROOT="+filepath.Join(base, "skills"))
	return maintenanceHarness{binary: binary, env: env, base: base}
}

func (h maintenanceHarness) run(t *testing.T, args ...string) (maintenanceEvent, string, int) {
	t.Helper()
	command := exec.Command(h.binary, append([]string{"--json"}, args...)...)
	command.Env = h.env
	command.Dir = t.TempDir()
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	if exitError, ok := err.(*exec.ExitError); ok {
		code = exitError.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	var event maintenanceEvent
	if decodeErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &event); decodeErr != nil {
		t.Fatalf("output is not one JSON event: %v: %s", decodeErr, stdout.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), maintenanceSentinel) {
		t.Fatal("sentinel leaked into maintenance output")
	}
	return event, stdout.String(), code
}

func previewDigest(t *testing.T, event maintenanceEvent, path ...string) string {
	t.Helper()
	var value any
	if err := json.Unmarshal(event.Maintenance, &value); err != nil {
		t.Fatal(err)
	}
	for _, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("missing %s in %s", key, event.Maintenance)
		}
		value = object[key]
	}
	digest, _ := value.(string)
	if len(digest) != 64 {
		t.Fatalf("digest=%v", value)
	}
	return digest
}

func TestExecutableCompatibilityBackupExportAndAuthority(t *testing.T) {
	harness := newMaintenanceHarness(t)
	inspect, _, code := harness.run(t, "compatibility", "inspect")
	if code != 0 || inspect.Status != "success" || inspect.Result != "Compatibility: recognized_poc" {
		t.Fatalf("inspect=%+v code=%d", inspect, code)
	}
	target := filepath.Join(harness.base, "..", "backup")
	preview, _, code := harness.run(t, "compatibility", "backup", "--target", target)
	if code != 0 || preview.Status != "success" {
		t.Fatalf("preview=%+v code=%d", preview, code)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("preview created target")
	}
	digest := previewDigest(t, preview, "preview", "digest")
	denied, _, code := harness.run(t, "compatibility", "backup", "--target", target, "--preview-digest", strings.Repeat("0", 64), "--authorize-local")
	if code == 0 || denied.Status != "denied_authority" {
		t.Fatalf("stale digest result=%+v", denied)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("denied authority created target")
	}
	applied, _, code := harness.run(t, "compatibility", "backup", "--target", target, "--preview-digest", digest, "--authorize-local")
	if code != 0 || applied.Status != "success" || applied.Result != "POC backup completed and verified" {
		t.Fatalf("applied=%+v", applied)
	}
	again, _, _ := harness.run(t, "compatibility", "backup", "--target", target)
	if again.Result != "POC backup already complete at the target" {
		t.Fatalf("equivalent retry=%+v", again)
	}
	exportTarget := filepath.Join(harness.base, "..", "export")
	exportPreview, _, _ := harness.run(t, "compatibility", "export", "--target", exportTarget)
	exportDigest := previewDigest(t, exportPreview, "preview", "digest")
	if exportDigest == digest {
		t.Fatal("backup and export share authority")
	}
	exported, _, code := harness.run(t, "compatibility", "export", "--target", exportTarget, "--preview-digest", exportDigest, "--authorize-local")
	if code != 0 || exported.Status != "success" {
		t.Fatalf("export=%+v", exported)
	}
	if _, err := os.Lstat(filepath.Join(exportTarget, "state")); !os.IsNotExist(err) {
		t.Fatal("local state entered portable export")
	}
	after, _, _ := harness.run(t, "compatibility", "inspect")
	if after.Result != "Compatibility: recognized_poc" {
		t.Fatal("preservation changed the POC source classification")
	}
}

func TestExecutableMaintenanceInputIsStrictAndSafe(t *testing.T) {
	harness := newMaintenanceHarness(t)
	for _, args := range [][]string{
		{"compatibility", "backup"},
		{"compatibility", "backup", "--target", "/tmp/x", "--authorize-local"},
		{"compatibility", "inspect", "--target", "/tmp/x"},
		{"compatibility", "backup", "-target", "/tmp/x"},
		{"compatibility", "backup", "--target", "/tmp/x", "--target", "/tmp/y"},
		{"recovery", "apply", "--preview-digest", strings.Repeat("a", 64)},
		{"upgrade", "--archive", "/tmp/a"},
		{"artifact", "cleanup", "--authorize-local"},
	} {
		event, _, code := harness.run(t, args...)
		if code == 0 || event.Status != "validation_failure" {
			t.Fatalf("args %q accepted: %+v", args, event)
		}
	}
	event, output, code := harness.run(t, "compatibility", "backup", "--target", "relative/"+maintenanceSentinel)
	if code == 0 || event.Status != "validation_failure" || strings.Contains(output, maintenanceSentinel) {
		t.Fatalf("relative target accepted: %s", output)
	}
	unknown, _, code := harness.run(t, "recovery", "rollback")
	if code == 0 || unknown.Status != "error" || unknown.Category != "invalid_command" {
		t.Fatalf("unknown maintenance command accepted: %+v", unknown)
	}
	// Shell metacharacters in operator paths are data: echoed, never run.
	injected := filepath.Join(harness.base, "..", "$(touch s7-pwned);`touch s7-pwned`")
	if event, _, _ := harness.run(t, "compatibility", "export", "--target", injected); event.Status != "success" {
		t.Fatalf("metacharacter target must stay data: %+v", event)
	}
	for _, directory := range []string{filepath.Join(harness.base, ".."), harness.base} {
		if _, err := os.Stat(filepath.Join(directory, "s7-pwned")); !os.IsNotExist(err) {
			t.Fatal("attack string executed")
		}
	}
}

func TestExecutableRecoveryAndCleanupAreExplicit(t *testing.T) {
	harness := newMaintenanceHarness(t)
	clean, _, code := harness.run(t, "recovery", "inspect")
	if code != 0 || clean.Result != "No interrupted local publication found" {
		t.Fatalf("recovery inspect=%+v", clean)
	}
	directory := filepath.Join(harness.base, "state", "work-items", "46f9e9bf-9da2-4769-b9b7-f058f0ab6e90")
	if err := os.WriteFile(filepath.Join(directory, ".axiom-recovery-0001"), []byte(maintenanceSentinel+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	found, output, code := harness.run(t, "recovery", "inspect")
	if code != 0 || found.Result != "Interrupted local publication found" || !strings.Contains(output, `"action":"preserved_review"`) {
		t.Fatalf("recovery inspect=%s", output)
	}
	planDigest := ""
	var view struct {
		Plans []struct{ Digest string } `json:"plans"`
	}
	if err := json.Unmarshal(found.Maintenance, &view); err == nil && len(view.Plans) == 1 {
		planDigest = view.Plans[0].Digest
	}
	refused, _, code := harness.run(t, "recovery", "apply", "--preview-digest", planDigest, "--authorize-local")
	if code == 0 || refused.Status != "validation_failure" {
		t.Fatalf("preserved plan applied: %+v", refused)
	}
	if wire, _ := os.ReadFile(filepath.Join(directory, ".axiom-recovery-0001")); !bytes.Contains(wire, []byte(maintenanceSentinel)) {
		t.Fatal("preserved marker changed")
	}
	compat, _, _ := harness.run(t, "compatibility", "inspect")
	if compat.Result != "Compatibility: recovery_required" {
		t.Fatalf("recovery marker did not win: %+v", compat)
	}
	cleanup, _, code := harness.run(t, "artifact", "cleanup")
	if code != 0 || cleanup.Result != "No artifact is eligible for cleanup" {
		t.Fatalf("cleanup=%+v", cleanup)
	}
}

func TestExecutableArtifactRetireRequiresExactEvidenceIdentity(t *testing.T) {
	harness := newMaintenanceHarness(t)
	for _, args := range [][]string{
		{"artifact", "retire"},
		{"artifact", "retire", "--artifact", "../escape"},
		{"artifact", "retire", "--artifact", "123e4567-e89b-42d3-a456-426614174000"},
		{"artifact", "retire", "--artifact", "123e4567-e89b-42d3-a456-426614174000", "--authorize-local"},
	} {
		event, output, code := harness.run(t, args...)
		if code == 0 || event.Status == "completed" || strings.Contains(output, "retirement-record:") {
			t.Fatalf("%v: %s", args, output)
		}
	}
	if _, err := os.Lstat(filepath.Join(harness.base, "state", "artifacts", "v1", "retirements")); !os.IsNotExist(err) {
		t.Fatalf("retirement namespace created without authority: %v", err)
	}
}
