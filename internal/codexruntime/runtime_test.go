package codexruntime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/cli"
	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/provenance"
)

type codexCompletionView struct {
	Status     completion.Status `json:"status"`
	Result     string            `json:"result"`
	References []string          `json:"references"`
	Next       string            `json:"next"`
	Details    string            `json:"details"`
	Provenance struct {
		Product     string                 `json:"product"`
		Version     string                 `json:"version"`
		Revision    string                 `json:"revision"`
		SourceState provenance.SourceState `json:"sourceState"`
	} `json:"provenance"`
}

func TestCodexCompletionContractPreservesCLISevenStatusMatrix(t *testing.T) {
	source, err := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "abc123def456", SourceState: provenance.Clean}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		status     completion.Status
		facts      completion.Facts
		references []string
		next       string
		details    string
	}{
		{completion.Success, completion.Facts{Completed: true}, []string{"project:alpha"}, "", ""},
		{completion.Failure, completion.Facts{Failed: true}, nil, "Inspect application diagnostics", "artifact:failure"},
		{completion.ValidationFailure, completion.Facts{ValidationFailed: true}, nil, "Correct selectors", ""},
		{completion.DeniedAuthority, completion.Facts{AuthorityDenied: true}, nil, "Obtain exact authority", ""},
		{completion.Partial, completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}, []string{"provider:github#7"}, "Retry local persistence", "artifact:partial"},
		{completion.Interrupted, completion.Facts{WasInterrupted: true}, []string{"execution:123"}, "Resume execution", ""},
		{completion.RetryableFailure, completion.Facts{RetrySafeFailure: true}, nil, "Retry after dependency recovery", "artifact:retryable"},
	}
	for _, test := range cases {
		t.Run(string(test.status), func(t *testing.T) {
			statement, err := provenance.NewText("operation completed", provenance.AxiomAuthored)
			if err != nil {
				t.Fatal(err)
			}
			var next provenance.Text
			if test.next != "" {
				next, err = provenance.NewText(test.next, provenance.AxiomAuthored)
				if err != nil {
					t.Fatal(err)
				}
			}
			result, err := completion.New(test.facts, statement, test.references, next, test.details, source)
			if err != nil {
				t.Fatal(err)
			}
			var rendered bytes.Buffer
			cli.WriteCompletion(&rendered, cli.CompletionJSON, result)
			var observed codexCompletionView
			if err := json.Unmarshal(rendered.Bytes(), &observed); err != nil {
				t.Fatalf("decode runtime-facing completion: %v: %s", err, rendered.String())
			}
			if observed.Status != test.status || observed.Result != "operation completed" || !sameStrings(observed.References, test.references) || observed.Next != test.next || observed.Details != test.details {
				t.Fatalf("completion mismatch: %+v", observed)
			}
			if observed.Provenance.Product != provenance.Product || observed.Provenance.Version != provenance.Development || observed.Provenance.Revision != "abc123def456" || observed.Provenance.SourceState != provenance.Clean {
				t.Fatalf("provenance mismatch: %+v", observed.Provenance)
			}
		})
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestSkillSetV2KeepsSelectorsAndCanonicalResultThin(t *testing.T) {
	if SkillSetVersion != "2" || BinaryCompatibility != "2" {
		t.Fatalf("compatibility=%s/%s", SkillSetVersion, BinaryCompatibility)
	}
	for _, name := range skillNames {
		content, err := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		for _, required := range []string{"axiom --json", "`status`", "`result`", "`references`", "`next`", "`details`", "`provenance`"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing thin adapter contract %q", name, required)
			}
		}
		for _, required := range []string{"top-level JSON object", "Never derive, synthesize, or reinterpret"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing exact canonical projection rule %q", name, required)
			}
		}
	}
	for _, name := range []string{"axiom-work-item-run", "axiom-work-item-status"} {
		content, _ := fs.ReadFile(skillFiles, "skills/"+name+"/SKILL.md")
		for _, required := range []string{"--project", "--repository", "--work-item", "--execution", "github:<owner>/<repository>#<number>"} {
			if !strings.Contains(string(content), required) {
				t.Fatalf("%s missing selector %q", name, required)
			}
		}
	}
	create, _ := fs.ReadFile(skillFiles, "skills/axiom-work-item-create/SKILL.md")
	if !strings.Contains(string(create), "do not extract, duplicate, rename, or relocate") {
		t.Fatal("axiom-work-item-create may not preserve the original Lingo payload shape")
	}
}

func TestSkillOutputContractsIsolateCanonicalCompletionAndPreserveOperationPayloads(t *testing.T) {
	canonical := []string{"status", "result", "references", "next", "details", "provenance"}
	tests := []struct {
		name     string
		payloads []string
	}{
		{"axiom-project-configure", []string{"setup"}},
		{"axiom-project-show", []string{"project"}},
		{"axiom-work-item-create", []string{"draft", "selection", "workItem"}},
		{"axiom-work-item-run", []string{"workflow", "projection"}},
		{"axiom-work-item-status", []string{"workflow", "projection"}},
	}
	fixture := []byte(`{
		"status":"top-status",
		"result":"top-result",
		"references":["top-reference"],
		"next":"top-next",
		"details":"top-details",
		"provenance":{"product":"Axiom","revision":"top-revision"},
		"setup":{"status":"setup-status","details":"setup-details","digest":"setup-digest","effects":["write project"]},
		"project":{"result":"project-result","details":"project-details","slug":"alpha","repositories":[{"key":"main"}]},
		"draft":{"next":"draft-next","details":"draft-details","digest":"draft-digest","target":{"provider":"github"},"effects":["create issue"]},
		"selection":{"references":["selection-reference"],"digest":"selection-digest"},
		"workItem":{"status":"work-item-status","details":"work-item-details","externalId":"7"},
		"workflow":{"status":"workflow-payload-status","details":"workflow-details","executionId":"execution-7","currentGate":"review","revision":4},
		"projection":{"provenance":{"revision":"projection-revision"},"digest":"projection-digest"}
	}`)
	var event map[string]json.RawMessage
	if err := json.Unmarshal(fixture, &event); err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := fs.ReadFile(skillFiles, "skills/"+test.name+"/SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			contract, err := parseSkillOutputContract(string(content))
			if err != nil {
				t.Fatal(err)
			}
			if !sameStrings(contract.canonical, canonical) || !sameStrings(contract.payloads, test.payloads) {
				t.Fatalf("contract = canonical %v payloads %v", contract.canonical, contract.payloads)
			}
			canonicalOutput := selectJSONFields(event, contract.canonical)
			payloadOutput := selectJSONFields(event, contract.payloads)
			for _, field := range canonical {
				if !bytes.Equal(canonicalOutput[field], event[field]) {
					t.Fatalf("canonical %s was not copied from top level", field)
				}
			}
			for _, field := range test.payloads {
				if !bytes.Equal(payloadOutput[field], event[field]) {
					t.Fatalf("operation payload %s was not preserved separately", field)
				}
			}
			if bytes.Contains(canonicalOutput["details"], []byte("setup-details")) || bytes.Contains(canonicalOutput["details"], []byte("draft-details")) || bytes.Contains(canonicalOutput["details"], []byte("workflow-details")) {
				t.Fatalf("operation payload leaked into canonical details: %s", canonicalOutput["details"])
			}
			minimal := selectJSONFields(map[string]json.RawMessage{"status": json.RawMessage(`"success"`)}, contract.canonical)
			if _, exists := minimal["details"]; exists || len(minimal) != 1 {
				t.Fatalf("absent canonical fields were not omitted: %v", minimal)
			}
		})
	}
}

type skillOutputContract struct {
	canonical []string
	payloads  []string
}

func parseSkillOutputContract(content string) (skillOutputContract, error) {
	const canonicalPrefix = "Canonical completion fields: "
	const payloadPrefix = "Operation-specific payloads preserved separately: "
	contract := skillOutputContract{}
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, canonicalPrefix):
			contract.canonical = parseContractFields(strings.TrimPrefix(line, canonicalPrefix))
		case strings.HasPrefix(line, payloadPrefix):
			contract.payloads = parseContractFields(strings.TrimPrefix(line, payloadPrefix))
		}
	}
	if len(contract.canonical) == 0 || len(contract.payloads) == 0 {
		return skillOutputContract{}, fmt.Errorf("structured output contract absent")
	}
	return contract, nil
}

func parseContractFields(value string) []string {
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.Trim(strings.TrimSpace(parts[index]), "`")
	}
	return parts
}

func selectJSONFields(event map[string]json.RawMessage, fields []string) map[string]json.RawMessage {
	selected := make(map[string]json.RawMessage, len(fields))
	for _, field := range fields {
		if value, ok := event[field]; ok {
			selected[field] = append(json.RawMessage(nil), value...)
		}
	}
	return selected
}

func TestReviewRemediatedV2SkillsRemainUpgradeable(t *testing.T) {
	prior := map[string]string{
		"axiom-project-configure": "d5271f6a24676c2f8111776cc797232784ad7e75318aca500f6ad73274510b60",
		"axiom-project-show":      "2542254b45ef2c1ac67e09ae1d1924fd0648787836b9bbbe60480a6f09646bcc",
		"axiom-work-item-create":  "8ecdd0553a999372522f7bc7ad0663e8474af7045f997c718e9c82e4799c5db3",
		"axiom-work-item-run":     "b5ca1ecf4dd136ba5baa6c647b19080d2d129d539e31b27573e43694ae40982f",
		"axiom-work-item-status":  "9fcfd0f9caf3a208e54d65cefab81d372cf1fa79c32ba3e1fe8efbc63d7f1990",
	}
	for name, digest := range prior {
		if !containsString(legacySkillDigests[name], digest) {
			t.Fatalf("%s prior owned digest is not upgradeable", name)
		}
	}
}

// T37 renamed the public executable in the thin entrypoints from lingo to
// axiom; the previously installed owned skill set and its receipt must stay
// recognized so the existing install and upgrade paths can replace them.
func TestPreAxiomExecutableSkillsAndReceiptRemainUpgradeable(t *testing.T) {
	prior := map[string]string{
		"axiom-project-configure": "237da8ea6a57e9240ae464d85a1fd1ad2d8c4d19ba8160c24943ae4752b2885d",
		"axiom-project-show":      "74abd548a0b352b9464efb2b1a6d5aca453bc1e88a164903d8a6043dfeebe8ab",
		"axiom-work-item-create":  "7d69ac3036d16a66df106b82ca21e7753c98b3bb0d203fc40c090659bdd1bfea",
		"axiom-work-item-run":     "5e1661d06a1caa7f7af6fd8c6253d3742f0cbb26df262a8c8357507e76f85f00",
		"axiom-work-item-status":  "213c58a0b55e0b7d52ca97ea72b4d474b5c1577f0f473e4e8b4be8e07c3d9319",
	}
	for name, digest := range prior {
		if !containsString(legacySkillDigests[name], digest) {
			t.Fatalf("%s pre-axiom owned digest is not upgradeable", name)
		}
		content, err := skillFiles.ReadFile("skills/" + name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "`lingo ") {
			t.Fatalf("%s still invokes the lingo executable", name)
		}
	}
	priorReceipt := []byte("formatVersion=1\nskillSetVersion=2\nbinaryCompatibility=2\nmanifestSha256=98b58d88e51ad9e5c907067248245a1e758d15b561bbf2d8dc99cc50924cb67d\n")
	found := false
	for _, wire := range legacyReceiptWires {
		found = found || string(wire) == string(priorReceipt)
	}
	if !found {
		t.Fatal("pre-axiom skill-set receipt is not upgradeable")
	}
}

func TestPublishReceiptUpgradesOnlyExactPriorAxiomReceipt(t *testing.T) {
	current, err := receiptBytes()
	if err != nil {
		t.Fatal(err)
	}
	prior := []byte("formatVersion=1\nskillSetVersion=2\nbinaryCompatibility=2\nmanifestSha256=aa50528dfd37acc2f5f95c2fc02937bc29cf6ea3cbdd51b8cd81c0a72d677adb\n")
	root := t.TempDir()
	path := filepath.Join(root, receiptName)
	if err := os.WriteFile(path, prior, 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, published := codexIntegration.publishReceipt(root, current); !changed || !published {
		t.Fatalf("known prior receipt upgrade = changed %t published %t", changed, published)
	}
	if !matchesPrivateFile(path, current) {
		t.Fatal("known prior receipt was not replaced with current receipt")
	}
	if err := os.WriteFile(path, []byte("foreign\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, published := codexIntegration.publishReceipt(root, current); changed || published {
		t.Fatalf("foreign receipt changed = changed %t published %t", changed, published)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestEmbeddedSkillsUseSupportedNamesAndThinEntrypoints(t *testing.T) {
	validName := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	for _, name := range skillNames {
		if !validName.MatchString(name) {
			t.Fatalf("unsupported skill name: %q", name)
		}
		content, err := skillFiles.ReadFile("skills/" + name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		if !strings.Contains(text, "name: "+name) || !strings.Contains(text, "axiom --json") {
			t.Fatalf("skill is not a named thin axiom entrypoint: %s", name)
		}
	}
}

func TestInstallUpgradesExactPriorAxiomSkill(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	directory := filepath.Join(root, "axiom-project-show")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `---
name: axiom-project-show
description: Inspect or resolve a configured Axiom Project through Lingo from any working directory.
---

# Show Axiom Project

Collect a Project slug or ID when absent. Run ` + "`lingo project show`" + ` with that
selector and report its structured result. Never infer a repository from Codex's
current working directory.
`
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Applied {
		t.Fatalf("upgrade = %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(directory, "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "axiom --json project show") {
		t.Fatalf("skill not upgraded: %q, %v", data, err)
	}
}

func TestInstallAndInspectGlobalSkills(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Inspect(context.Background()); got.Status != Missing || len(got.Skills) != 5 || got.Skills[0].State != "missing" {
		t.Fatalf("initial status = %#v", got)
	}
	if got := service.Install(context.Background()); got.Status != Applied || got.Category != "codex_configured" {
		t.Fatalf("install = %#v", got)
	}
	if got := service.Inspect(context.Background()); got.Status != Ready || got.Category != "codex_ready" {
		t.Fatalf("inspect = %#v", got)
	}
	if got := service.Install(context.Background()); got.Status != Unchanged || got.Category != "codex_already_configured" {
		t.Fatalf("rerun = %#v", got)
	}
	if err := os.Remove(filepath.Join(service.root, receiptName)); err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Applied || got.Category != "codex_configured" {
		t.Fatalf("receipt completion = %#v", got)
	}
}

func TestInstallRefusesAmbiguousLegacyLockState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".axiom-skill-set.lock"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "recovery_required" {
		t.Fatalf("ambiguous lock = %#v", got)
	}
	if info, err := os.Stat(filepath.Join(root, ".axiom-skill-set.lock")); err != nil || !info.IsDir() {
		t.Fatalf("ambiguous lock changed: %v, %v", info, err)
	}
	for _, skill := range skillNames {
		if _, err := os.Stat(filepath.Join(root, skill)); !os.IsNotExist(err) {
			t.Fatalf("ambiguous install published %s: %v", skill, err)
		}
	}
}

func TestInstallPreservesUnknownLockContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(root, installLockName)
	if err := os.WriteFile(lock, []byte("unknown\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "recovery_required" {
		t.Fatalf("unknown lock = %#v", got)
	}
	wire, err := os.ReadFile(lock)
	if err != nil || string(wire) != "unknown\n" {
		t.Fatalf("unknown lock changed: %q, %v", wire, err)
	}
}

func TestInstallProcessLockRefusesActiveAndResumesAfterSIGKILL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	command := exec.Command(os.Args[0], "-test.run=^TestInstallProcessHelper$")
	command.Env = append(os.Environ(), "AXIOM_CODEX_INSTALL_HELPER=hold", "AXIOM_CODEX_INSTALL_ROOT="+root)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("lock barrier not reached: %q, %v", line, err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "codex_skill_install_concurrent" {
		t.Fatalf("active lock = %#v", got)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	if got := service.Install(context.Background()); got.Status != Applied || got.Category != "codex_configured" {
		t.Fatalf("abandoned lock resume = %#v", got)
	}
	if got := service.Inspect(context.Background()); got.Status != Ready {
		t.Fatalf("resumed install = %#v", got)
	}
}

func TestInstallProcessHelper(t *testing.T) {
	if os.Getenv("AXIOM_CODEX_INSTALL_HELPER") != "hold" {
		return
	}
	service, err := New(os.Getenv("AXIOM_CODEX_INSTALL_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	service.afterSkill = func(string) {
		fmt.Fprintln(os.Stdout, "locked")
		_ = os.Stdout.Sync()
		time.Sleep(time.Hour)
	}
	result := service.Install(context.Background())
	if result.Status != Applied {
		t.Fatal(result)
	}
}

func TestInstallAndInspectRejectUnsafeRootPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o770); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "codex_skill_root_unavailable" {
		t.Fatalf("unsafe install root = %#v", got)
	}
	if got := service.Inspect(context.Background()); got.Status != Failed || got.Category != "codex_skill_root_unavailable" {
		t.Fatalf("unsafe inspect root = %#v", got)
	}
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm() != 0o770 {
		t.Fatalf("unsafe root changed: %v, %v", info, err)
	}
}

func TestInstallRefusesConflictAndRollsBackCurrentAttempt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	if err := os.MkdirAll(filepath.Join(root, "axiom-work-item-create"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "axiom-work-item-create", "SKILL.md"), []byte("unowned"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "codex_skill_conflict" {
		t.Fatalf("install = %#v", got)
	} else if len(got.Skills) != 5 || got.Skills[2].State != "modified_or_foreign" {
		t.Fatalf("conflict detail = %#v", got.Skills)
	}
	for _, name := range []string{"axiom-project-configure", "axiom-project-show"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("partial skill retained: %s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "axiom-work-item-create", "SKILL.md"))
	if err != nil || string(data) != "unowned" {
		t.Fatalf("conflicting skill changed: %q, %v", data, err)
	}
}

func TestInspectRejectsHardLinkedOwnedSkill(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Applied {
		t.Fatalf("install = %#v", got)
	}
	path := filepath.Join(root, "axiom-project-show", "SKILL.md")
	if err := os.Link(path, filepath.Join(t.TempDir(), "skill-copy")); err != nil {
		t.Fatal(err)
	}
	if got := service.Inspect(context.Background()); got.Status != Missing || got.Skills[1].State != "modified_or_foreign" {
		t.Fatalf("hard link inspection = %#v", got)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "codex_skill_conflict" {
		t.Fatalf("hard link install = %#v", got)
	}
}

func TestInspectRejectsUnsafeSkillPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Applied {
		t.Fatalf("install = %#v", got)
	}
	directory := filepath.Join(root, "axiom-project-show")
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := service.Inspect(context.Background()); got.Status != Missing || got.Skills[1].State != "modified_or_foreign" {
		t.Fatalf("permission inspection = %#v", got)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "codex_skill_conflict" {
		t.Fatalf("permission install = %#v", got)
	}
}

func TestInstallRejectsSymlinkSkill(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "axiom-project-configure")); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Failed {
		t.Fatalf("symlink accepted: %#v", got)
	}
}

func TestSkillManifestIsClosedVersionedAndComplete(t *testing.T) {
	manifest, err := CurrentManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.FormatVersion != 1 || manifest.SkillSetVersion != SkillSetVersion || manifest.BinaryCompatibility != BinaryCompatibility || len(manifest.Skills) != 5 {
		t.Fatalf("manifest = %#v", manifest)
	}
	seen := map[string]bool{}
	for _, skill := range manifest.Skills {
		if seen[skill.Name] || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(skill.SHA256) {
			t.Fatalf("invalid skill manifest entry: %#v", skill)
		}
		seen[skill.Name] = true
	}
}

func TestInspectReportsBinaryCompatibilityAndPartialResume(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	service.afterSkill = func(string) {
		calls++
		if calls == 2 {
			service.afterSkill = nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	service.afterSkill = func(string) {
		calls++
		if calls == 2 {
			cancel()
		}
	}
	if got := service.Install(ctx); got.Status != Partial || got.Category != "codex_skill_install_partial" {
		t.Fatalf("partial install = %#v", got)
	}
	if got := service.Inspect(context.Background()); got.Status != Missing {
		t.Fatalf("partial inspection = %#v", got)
	}
	if got := service.Install(context.Background()); got.Status != Applied {
		t.Fatalf("resume = %#v", got)
	}
	incompatible, err := NewForBinary(root, "3")
	if err != nil {
		t.Fatal(err)
	}
	if got := incompatible.Inspect(context.Background()); got.Status != Incompatible || got.Category != "codex_binary_skill_incompatible" {
		t.Fatalf("compatibility = %#v", got)
	}
}

func TestClaudeIntegrationUpgradesOnlyRegisteredClaudeHistory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "claude", "skills")
	service, err := NewClaude(root)
	if err != nil || service.Runtime() != "claude" {
		t.Fatalf("service = %v, %v", service.Runtime(), err)
	}
	if got := service.Inspect(context.Background()); got.Status != Missing || got.Category != "claude_not_configured" {
		t.Fatalf("inspect = %#v", got)
	}
	previous := []byte("---\nname: axiom-project-show\n---\nprevious Claude revision\n")
	directory := filepath.Join(root, "axiom-project-show")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), previous, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := service.Install(context.Background()); got.Status != Failed || got.Category != "claude_skill_conflict" {
		t.Fatalf("unregistered content = %#v", got)
	}
	digest := sha256.Sum256(previous)
	service.integration.legacySkills = map[string][]string{"axiom-project-show": {hex.EncodeToString(digest[:])}}
	if got := service.Install(context.Background()); got.Status != Applied || got.Category != "claude_configured" {
		t.Fatalf("registered history = %#v", got)
	}
	if got := service.Inspect(context.Background()); got.Status != Ready || got.Category != "claude_ready" {
		t.Fatalf("inspect = %#v", got)
	}
	receipt, err := os.ReadFile(filepath.Join(root, receiptName))
	if err != nil || !strings.HasPrefix(string(receipt), "formatVersion=1\nruntime=claude\nskillsRoot="+root+"\n") {
		t.Fatalf("receipt = %q, %v", receipt, err)
	}
	if codexReceipt, _ := receiptBytes(); string(codexReceipt) == string(receipt) {
		t.Fatal("Claude receipt is indistinguishable from the Codex receipt")
	}
}
