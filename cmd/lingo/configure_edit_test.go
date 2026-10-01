package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/cli"
	"github.com/rgomids/axiom/internal/local"
	"github.com/rgomids/axiom/internal/projectapp"
)

// Issue #132 I132-T01 Evidence: EDIT is a zero-write preview over the real
// portable/local stores; CREATE against a configured slug fails closed.

type editEnvironment struct {
	root, state   string
	api, web      string
	extra         []string
	service       cli.Service
	projectID     string
	createPreview projectapp.SetupPreview
}

func newEditEnvironment(t *testing.T) editEnvironment {
	t.Helper()
	env := editEnvironment{
		root:  filepath.Join(t.TempDir(), "projects"),
		state: filepath.Join(t.TempDir(), "state"),
		api:   filepath.Join(t.TempDir(), "api"),
		web:   filepath.Join(t.TempDir(), "web"),
	}
	for _, path := range []string{env.api, env.web} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("LINGO_PROJECTS_ROOT", env.root)
	t.Setenv("LINGO_STATE_ROOT", env.state)
	env.service = compose()
	env.createPreview = configureProjectWith(t, env.service, "sample", "Sample", "github", "api="+env.api, "web="+env.web)
	env.projectID = env.createPreview.ProjectID
	return env
}

func configureProjectWith(t *testing.T, service cli.Service, slug, name, provider string, repositories ...string) projectapp.SetupPreview {
	t.Helper()
	args := []string{"project", "configure", "--slug", slug, "--name", name, "--work-item-provider", provider}
	for _, repository := range repositories {
		args = append(args, "--repository", repository)
	}
	var output bytes.Buffer
	if code := cli.Run(context.Background(), args, service, currentProvenance(), &output); code != cli.ExitSuccess {
		t.Fatalf("create preview: %s", output.String())
	}
	var event struct {
		Setup projectapp.SetupPreview `json:"setup"`
	}
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	authorized := append(args, "--project-id", event.Setup.ProjectID, "--preview-digest", event.Setup.Digest, "--authorize-local")
	if code := cli.Run(context.Background(), authorized, service, currentProvenance(), &output); code != cli.ExitSuccess {
		t.Fatalf("create publish: %s", output.String())
	}
	return event.Setup
}

type editEvent struct {
	Status     string                 `json:"status"`
	Result     string                 `json:"result"`
	Next       string                 `json:"next"`
	References []string               `json:"references"`
	Edit       projectapp.EditPreview `json:"edit"`
}

// runEdit executes one EDIT request and proves that portable and local roots
// are byte-identical before and after it.
func (env editEnvironment) runEdit(t *testing.T, wantCode int, wantStatus, wantResult string, args ...string) (editEvent, string) {
	t.Helper()
	roots := append([]string{env.root, env.state}, env.extra...)
	before := snapshotTrees(t, roots...)
	var output bytes.Buffer
	code := cli.Run(context.Background(), append([]string{"project", "configure"}, args...), env.service, currentProvenance(), &output)
	if after := snapshotTrees(t, roots...); !bytes.Equal(before, after) {
		t.Fatalf("%v wrote state\nbefore=%s\nafter=%s", args, before, after)
	}
	var event editEvent
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("%v: %v: %s", args, err, output.String())
	}
	if code != wantCode || event.Status != wantStatus || event.Result != wantResult {
		t.Fatalf("%v: code=%d event=%+v output=%s", args, code, event, output.String())
	}
	return event, output.String()
}

func effectList(effects []projectapp.EditEffect) []string {
	result := []string{}
	for _, effect := range effects {
		value := effect.Scope + ":" + effect.Code
		if effect.Key != "" {
			value += ":" + effect.Key
		}
		result = append(result, value)
	}
	return result
}

func TestEditPreviewBySlugAndUUIDIsCompleteAndZeroWrite(t *testing.T) {
	env := newEditEnvironment(t)
	bySlug, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample", "--name", "Renamed")
	byID, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", env.projectID, "--name", "Renamed")
	if !reflect.DeepEqual(bySlug.Edit, byID.Edit) {
		t.Fatalf("slug and UUID produced different previews:\n%+v\n%+v", bySlug.Edit, byID.Edit)
	}
	preview := bySlug.Edit
	if preview.Mode != "edit" || preview.ProjectID != env.projectID || preview.Slug != "sample" || preview.Name != "Renamed" || preview.Capability.Provider != "github" {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.PortableDestination != filepath.Join(env.root, "sample") || preview.LocalDestination != filepath.Join(env.state, "projects", env.projectID) {
		t.Fatalf("destinations = %s %s", preview.PortableDestination, preview.LocalDestination)
	}
	portable, err := local.NewPortableStore(env.root)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := portable.Inspect(context.Background(), "sample")
	if err != nil || preview.PortableRevision != observed.Revision {
		t.Fatalf("portable revision %s != observed %s (%v)", preview.PortableRevision, observed.Revision, err)
	}
	for _, expected := range []string{"name: Renamed", "slug: sample", "key: api", "key: web", "work-items"} {
		if !strings.Contains(preview.PortableManifest, expected) {
			t.Fatalf("portable result missing %q:\n%s", expected, preview.PortableManifest)
		}
	}
	if strings.Contains(preview.PortableManifest, env.api) {
		t.Fatal("portable candidate carries a machine-local path")
	}
	if got := effectList(preview.Effects); !reflect.DeepEqual(got, []string{"portable:update_portable_project", "local:update_local_record"}) {
		t.Fatalf("effects = %v", got)
	}
	for _, repository := range preview.Repositories {
		if repository.Change != projectapp.RepositoryPreserved || repository.LocalPath != "" {
			t.Fatalf("omitted repository exposed or changed: %+v", repository)
		}
	}
	noop, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample")
	if len(noop.Edit.Effects) != 0 || noop.Next != "No change is required" || noop.Edit.Name != "Sample" {
		t.Fatalf("selector-only edit = %+v", noop)
	}
}

func TestEditPreviewRepositoryAndProviderSemanticsOverRealStores(t *testing.T) {
	env := newEditEnvironment(t)
	docs := filepath.Join(t.TempDir(), "docs")
	moved := filepath.Join(t.TempDir(), "web-moved")
	for _, path := range []string{docs, moved} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	event, output := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready",
		"--project", "sample", "--remove-repository", "api", "--repository", "web="+moved, "--repository", "docs="+docs, "--remove-work-item-provider")
	want := []string{
		"portable:update_portable_project", "portable:remove_portable_repository:api", "portable:add_portable_repository:docs",
		"local:update_local_record", "local:remove_local_binding:api", "local:add_local_binding:docs", "local:update_local_binding:web",
	}
	if got := effectList(event.Edit.Effects); !reflect.DeepEqual(got, want) {
		t.Fatalf("effects = %v", got)
	}
	if event.Edit.Capability.Provider != "" || event.Edit.Capability.Readiness != projectapp.CapabilityMissing || strings.Contains(event.Edit.PortableManifest, "work-items") {
		t.Fatalf("provider removal not reflected: %+v", event.Edit.Capability)
	}
	if strings.Contains(event.Edit.PortableManifest, "key: api") || !strings.Contains(event.Edit.PortableManifest, "key: docs") {
		t.Fatalf("portable associations wrong:\n%s", event.Edit.PortableManifest)
	}
	if !strings.Contains(output, moved) || !strings.Contains(output, docs) {
		t.Fatal("changed local bindings are not reviewable")
	}

	env.runEdit(t, cli.ExitFailure, "validation_failure", "Repository to remove is not configured", "--project", "sample", "--remove-repository", "unknown")
	env.runEdit(t, cli.ExitFailure, "validation_failure", "Repository path is unavailable", "--project", "sample", "--repository", "docs="+filepath.Join(t.TempDir(), "absent"))
	for _, conflict := range [][]string{
		{"--project", "sample", "--repository", "api=" + env.api, "--remove-repository", "api"},
		{"--project", "sample", "--work-item-provider", "linear", "--remove-work-item-provider"},
		{"--project", "sample", "--slug", "renamed"},
	} {
		env.runEdit(t, cli.ExitFailure, "validation_failure", "Explicit selector input is invalid", conflict...)
	}
	linear, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample", "--work-item-provider", "linear")
	if linear.Edit.Capability.Provider != "linear" || linear.Edit.Capability.Readiness != projectapp.CapabilityUnsupported || linear.Edit.Name != "Sample" {
		t.Fatalf("provider set = %+v", linear.Edit)
	}
}

func TestEditSelectorFailuresNeverFallBackToCreate(t *testing.T) {
	env := newEditEnvironment(t)
	env.runEdit(t, cli.ExitFailure, "validation_failure", "Project was not found", "--project", "missing", "--name", "Missing", "--repository", "api="+env.api)
	if _, err := os.Stat(filepath.Join(env.root, "missing")); !os.IsNotExist(err) {
		t.Fatalf("unknown EDIT selector created a Project: %v", err)
	}

	// A second installed record observing the same slug is representable and
	// must fail as ambiguous rather than choosing either Project.
	other := "123e4567-e89b-42d3-a456-426614174001"
	record, issues := local.NewRecord(local.RecordState{
		ProjectID: other, ObservedSlug: "sample", SourceLocation: filepath.Join(env.root, "sample"),
		PortableRevision: projectapp.RecordedPortableRevision([32]byte{1}),
		ArtifactDigests:  []projectapp.ArtifactDigest{{Name: "axiom.yaml", Digest: [32]byte{1}}},
	})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	wire, issues := local.EncodeRecord(record)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if err := os.MkdirAll(filepath.Join(env.state, "projects", other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.state, "projects", other, "installation.json"), wire, 0o600); err != nil {
		t.Fatal(err)
	}
	env.runEdit(t, cli.ExitFailure, "validation_failure", "Project selector is ambiguous", "--project", "sample", "--name", "Renamed")
	uuid, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", env.projectID, "--name", "Renamed")
	if uuid.Edit.ProjectID != env.projectID {
		t.Fatal("UUID did not disambiguate the selection")
	}
}

func TestEditPreviewRepairsBrokenBindingWithoutAvailabilityGate(t *testing.T) {
	env := newEditEnvironment(t)
	if err := os.Remove(env.web); err != nil {
		t.Fatal(err)
	}
	runCanonicalCLI(t, env.service, []string{"project", "show", "--selector", "sample"}, cli.ExitFailure, "retryable_failure", "Project repository is unavailable")
	removed, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample", "--remove-repository", "web")
	if got := effectList(removed.Edit.Effects); !reflect.DeepEqual(got, []string{"portable:update_portable_project", "portable:remove_portable_repository:web", "local:update_local_record", "local:remove_local_binding:web"}) {
		t.Fatalf("broken binding removal effects = %v", got)
	}
	replacement := filepath.Join(t.TempDir(), "web-replacement")
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	repaired, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample", "--repository", "web="+replacement)
	if got := effectList(repaired.Edit.Effects); !reflect.DeepEqual(got, []string{"local:update_local_record", "local:update_local_binding:web"}) {
		t.Fatalf("broken binding repair effects = %v", got)
	}
}

func TestEditPreviewHidesUnrelatedLocalMetadataButBindsIt(t *testing.T) {
	env := newEditEnvironment(t)
	path := filepath.Join(env.state, "projects", env.projectID, "installation.json")
	base, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample", "--remove-repository", "api")

	wire, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	record, issues := local.DecodeRecord(wire)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	state := record.State()
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	state.Credentials = []projectapp.CredentialBinding{{ReferenceKey: "chat-token", SourceKind: "keychain", ItemReference: "axiom/hidden-credential-item"}}
	state.Runtime = projectapp.RuntimeBinding{RuntimeID: "codex", ExplicitPath: "/opt/hidden-runtime-path", Observation: projectapp.Observation{Availability: projectapp.Available, Basis: projectapp.PresentMetadata, ObservedAt: at}}
	state.Attempt = projectapp.AttemptMetadata{Correlation: "hidden-attempt-correlation", At: at}
	enriched, issues := local.NewRecord(state)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	enrichedWire, issues := local.EncodeRecord(enriched)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if err := os.WriteFile(path, enrichedWire, 0o600); err != nil {
		t.Fatal(err)
	}

	event, output := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample", "--remove-repository", "api")
	for _, hidden := range []string{"hidden-credential-item", "hidden-runtime-path", "hidden-attempt-correlation", env.web, "observedAt", "present_metadata"} {
		if strings.Contains(output, hidden) {
			t.Fatalf("preview exposed unrelated local metadata %q: %s", hidden, output)
		}
	}
	if event.Edit.Digest == base.Edit.Digest || event.Edit.LocalRevision == base.Edit.LocalRevision {
		t.Fatal("digest/observation did not bind hidden local metadata")
	}
	if !reflect.DeepEqual(event.Edit.Effects, base.Edit.Effects) || event.Edit.PortableManifest != base.Edit.PortableManifest {
		t.Fatal("hidden metadata changed visible candidate")
	}
}

// EDIT replay/publication is I132-T02. Its inputs are rejected before any
// selector resolution or portable/local read, and no preview is built.
func TestEditReplayInputsFailBeforeAnyStateRead(t *testing.T) {
	env := newEditEnvironment(t)
	preview, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample", "--name", "Renamed")
	for name, replay := range map[string][]string{
		"preview digest":        {"--preview-digest", preview.Edit.Digest},
		"authorize local":       {"--authorize-local"},
		"digest and authority":  {"--preview-digest", preview.Edit.Digest, "--authorize-local"},
		"complete replay tuple": {"--project-id", env.projectID, "--preview-digest", preview.Edit.Digest, "--authorize-local"},
		"replay-only identity":  {"--project-id", env.projectID},
	} {
		t.Run(name, func(t *testing.T) {
			args := append([]string{"--project", "sample", "--name", "Renamed"}, replay...)
			event, output := env.runEdit(t, cli.ExitFailure, "validation_failure", "Project edit publication is not available", args...)
			if strings.Contains(output, `"edit"`) || event.Edit.Digest != "" {
				t.Fatalf("replay input built a preview: %s", output)
			}
		})
	}

	// The service guard also precedes selection: with an unknown selector and
	// unreadable roots, any read would fail with a different result.
	before := snapshotTrees(t, env.root, env.state)
	for _, root := range []string{env.root, env.state} {
		if err := os.Chmod(root, 0); err != nil {
			t.Fatal(err)
		}
	}
	result := env.service.Configure(context.Background(), cli.ConfigureInput{Project: "missing", Name: "Renamed", NameSupplied: true, PreviewDigest: preview.Edit.Digest, AuthorizeLocal: true})
	for _, root := range []string{env.root, env.state} {
		if err := os.Chmod(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if result.Completion == nil || result.Completion.Result().String() != "Project edit publication is not available" || result.Edit != nil {
		t.Fatalf("service replay guard = %+v", result)
	}
	if after := snapshotTrees(t, env.root, env.state); !bytes.Equal(before, after) {
		t.Fatal("service replay guard changed state")
	}
}

// installExternal initializes a portable Project outside the configured
// projects root and installs it with project install, so its recorded
// SourceLocation is not <projects-root>/<slug>.
func (env *editEnvironment) installExternal(t *testing.T) string {
	t.Helper()
	sourceRoot := filepath.Join(t.TempDir(), "elsewhere")
	t.Setenv("LINGO_PROJECTS_ROOT", sourceRoot)
	runCLI(t, compose(), []string{"project", "init", "--slug", "external", "--name", "External"}, cli.ExitSuccess, "applied")
	t.Setenv("LINGO_PROJECTS_ROOT", env.root)
	source := filepath.Join(sourceRoot, "external")
	runCLI(t, env.service, []string{"project", "install", "--source", source}, cli.ExitSuccess, "installed")
	env.extra = append(env.extra, sourceRoot)
	return source
}

func rewriteManifest(t *testing.T, source string, rewrite func(string) string) {
	t.Helper()
	path := filepath.Join(source, "axiom.yaml")
	wire, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := rewrite(string(wire))
	if changed == string(wire) {
		t.Fatalf("manifest rewrite had no effect:\n%s", wire)
	}
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEditPreviewLoadsArbitraryRecordedSource(t *testing.T) {
	env := newEditEnvironment(t)
	source := env.installExternal(t)
	bySlug, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "external", "--name", "External Renamed", "--repository", "api="+env.api)
	preview := bySlug.Edit
	if preview.PortableDestination != source || preview.Slug != "external" || preview.Name != "External Renamed" || preview.LocalDestination != filepath.Join(env.state, "projects", preview.ProjectID) {
		t.Fatalf("recorded-source preview = %+v", preview)
	}
	byID, _ := env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", preview.ProjectID, "--name", "External Renamed", "--repository", "api="+env.api)
	if !reflect.DeepEqual(byID.Edit, preview) {
		t.Fatal("slug and UUID selection of a recorded source differ")
	}
	portable, err := local.NewPortableStore(env.root)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := portable.InspectRecordedSource(context.Background(), source, "external")
	if err != nil || preview.PortableRevision != observed.Revision {
		t.Fatalf("portable revision %s != recorded source %s (%v)", preview.PortableRevision, observed.Revision, err)
	}
	if got := effectList(preview.Effects); !reflect.DeepEqual(got, []string{"portable:update_portable_project", "portable:add_portable_repository:api", "local:update_local_record", "local:add_local_binding:api"}) {
		t.Fatalf("effects = %v", got)
	}
	if !strings.Contains(preview.PortableManifest, "name: External Renamed") || strings.Contains(preview.PortableManifest, env.api) {
		t.Fatalf("portable candidate:\n%s", preview.PortableManifest)
	}
	// The default-root Project in the same installation stays editable.
	env.runEdit(t, cli.ExitSuccess, "success", "Project edit preview ready", "--project", "sample", "--name", "Renamed")
}

func TestEditFailsClosedOnUnsafeOrIncoherentRecordedSource(t *testing.T) {
	for name, mutate := range map[string]func(t *testing.T, env editEnvironment, source string){
		"missing source": func(t *testing.T, _ editEnvironment, source string) {
			if err := os.RemoveAll(source); err != nil {
				t.Fatal(err)
			}
		},
		"symlinked source": func(t *testing.T, _ editEnvironment, source string) {
			if err := os.Rename(source, source+"-real"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(source+"-real", source); err != nil {
				t.Fatal(err)
			}
		},
		"shared source permissions": func(t *testing.T, _ editEnvironment, source string) {
			if err := os.Chmod(source, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"different portable project id": func(t *testing.T, _ editEnvironment, source string) {
			rewriteManifest(t, source, func(wire string) string {
				lines := strings.Split(wire, "\n")
				for i, line := range lines {
					if strings.HasPrefix(strings.TrimSpace(line), "id: ") {
						lines[i] = strings.Repeat(" ", len(line)-len(strings.TrimLeft(line, " "))) + "id: 123e4567-e89b-42d3-a456-426614174999"
					}
				}
				return strings.Join(lines, "\n")
			})
		},
		"different portable slug": func(t *testing.T, _ editEnvironment, source string) {
			rewriteManifest(t, source, func(wire string) string { return strings.Replace(wire, "slug: external", "slug: other", 1) })
		},
		"different portable revision": func(t *testing.T, _ editEnvironment, source string) {
			rewriteManifest(t, source, func(wire string) string { return strings.Replace(wire, "name: External", "name: Changed", 1) })
		},
		"relative recorded source": func(t *testing.T, env editEnvironment, source string) {
			records, err := filepath.Glob(filepath.Join(env.state, "projects", "*", "installation.json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, record := range records {
				wire, err := os.ReadFile(record)
				if err != nil {
					t.Fatal(err)
				}
				if changed := strings.Replace(string(wire), `"`+source+`"`, `"elsewhere/external"`, 1); changed != string(wire) {
					if err := os.WriteFile(record, []byte(changed), 0o600); err != nil {
						t.Fatal(err)
					}
					return
				}
			}
			t.Fatal("recorded source not found")
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := newEditEnvironment(t)
			source := env.installExternal(t)
			mutate(t, env, source)
			env.runEdit(t, cli.ExitFailure, "failure", "Selected Project state is not safe to edit", "--project", "external", "--name", "Renamed")
		})
	}
}

func TestEditFailsClosedOnStalePortableLocalRelationship(t *testing.T) {
	env := newEditEnvironment(t)
	// The legacy update changes only the portable manifest, leaving the local
	// record's recorded portable revision stale.
	runCLI(t, env.service, []string{"project", "update", "--slug", "sample", "--name", "Changed"}, cli.ExitSuccess, "applied")
	env.runEdit(t, cli.ExitFailure, "failure", "Selected Project state is not safe to edit", "--project", "sample", "--name", "Renamed")
}

func TestCreateCollisionFailsWithoutReuseOrEditFallback(t *testing.T) {
	env := newEditEnvironment(t)
	const slugTaken, identityTaken = "Project slug is already configured", "Project identity is already configured"
	for name, test := range map[string]struct {
		want string
		args []string
	}{
		"equivalent values":                  {slugTaken, []string{"--slug", "sample", "--name", "Sample", "--work-item-provider", "github", "--repository", "api=" + env.api, "--repository", "web=" + env.web}},
		"different values":                   {slugTaken, []string{"--slug", "sample", "--name", "Other", "--work-item-provider", "none", "--repository", "api=" + env.api}},
		"existing identity":                  {slugTaken, []string{"--slug", "sample", "--name", "Sample", "--work-item-provider", "github", "--repository", "api=" + env.api, "--repository", "web=" + env.web, "--project-id", env.projectID}},
		"authorized replay":                  {slugTaken, []string{"--slug", "sample", "--name", "Sample", "--work-item-provider", "github", "--repository", "api=" + env.api, "--repository", "web=" + env.web, "--project-id", env.projectID, "--preview-digest", env.createPreview.Digest, "--authorize-local"}},
		"fresh slug with existing identity":  {identityTaken, []string{"--slug", "fresh", "--name", "Fresh", "--work-item-provider", "github", "--repository", "api=" + env.api, "--project-id", env.projectID}},
		"authorized fresh slug, existing id": {identityTaken, []string{"--slug", "fresh", "--name", "Fresh", "--work-item-provider", "github", "--repository", "api=" + env.api, "--project-id", env.projectID, "--preview-digest", env.createPreview.Digest, "--authorize-local"}},
	} {
		t.Run(name, func(t *testing.T) {
			event, output := env.runEdit(t, cli.ExitFailure, "validation_failure", test.want, test.args...)
			if strings.Contains(output, `"edit"`) || strings.Contains(output, `"setup"`) || strings.Contains(output, env.projectID) || len(event.References) != 0 {
				t.Fatalf("collision reused or exposed existing Project state: %s", output)
			}
		})
	}
	records, err := filepath.Glob(filepath.Join(env.state, "projects", "*", "installation.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("collision allocated another identity: %v %v", records, err)
	}
}
