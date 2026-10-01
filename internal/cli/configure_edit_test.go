package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/projectapp"
)

// editRecordingService captures the presence-aware configure input and returns
// a fixed complete preview; it performs no merge.
type editRecordingService struct {
	recordingService
	inputs []ConfigureInput
	result completion.Result
}

func (s *editRecordingService) Configure(_ context.Context, input ConfigureInput) Result {
	s.inputs = append(s.inputs, input)
	preview := projectapp.EditPreview{
		Mode: projectapp.EditMode, ProjectID: "123e4567-e89b-42d3-a456-426614174000", Slug: "sample", Name: "Sample",
		Capability:          projectapp.SetupCapabilityPreview{Capability: projectapp.WorkItemCapability, Provider: "github", Readiness: projectapp.CapabilityReady},
		PortableManifest:    "schemaVersion: 1\nproject:\n  slug: sample\n",
		Repositories:        []projectapp.EditRepositoryPreview{{Key: "api", Change: projectapp.RepositoryRemoved, LocalPath: "/work/api"}, {Key: "web", Change: projectapp.RepositoryPreserved}},
		PortableDestination: "/portable/sample", LocalDestination: "/state/projects/id", PortableRevision: "p1", LocalRevision: "l1",
		Effects: []projectapp.EditEffect{{Scope: projectapp.PortableScope, Code: "remove_portable_repository", Key: "api"}, {Scope: projectapp.LocalScope, Code: "remove_local_binding", Key: "api"}},
		Digest:  "edit-digest",
	}
	result := s.result
	return Result{Completion: &result, Edit: &preview}
}

func newEditRecordingService(t *testing.T) *editRecordingService {
	return &editRecordingService{result: canonicalResult(t, completion.Success, []string{"project:123e4567-e89b-42d3-a456-426614174000"}, "Review preview", completionProvenance(t))}
}

func TestConfigureEditCapturesPresenceWithoutMerging(t *testing.T) {
	for name, test := range map[string]struct {
		args []string
		want ConfigureInput
	}{
		"selector only preserves everything": {
			args: []string{"--project", "sample"},
			want: ConfigureInput{Project: "sample", Repositories: []RepositoryInput{}},
		},
		"name set": {
			args: []string{"--project", "sample", "--name", "Renamed"},
			want: ConfigureInput{Project: "sample", Name: "Renamed", NameSupplied: true, Repositories: []RepositoryInput{}},
		},
		"provider set": {
			args: []string{"--project=123e4567-e89b-42d3-a456-426614174000", "--work-item-provider", "github"},
			want: ConfigureInput{Project: "123e4567-e89b-42d3-a456-426614174000", WorkItemProvider: "github", WorkItemProviderSupplied: true, Repositories: []RepositoryInput{}},
		},
		"provider remove": {
			args: []string{"--project", "sample", "--remove-work-item-provider"},
			want: ConfigureInput{Project: "sample", RemoveWorkItemProvider: true, Repositories: []RepositoryInput{}},
		},
		"repeatable repositories": {
			args: []string{"--project", "sample", "--repository", "web=/work/web", "--repository=docs=/work/docs", "--remove-repository", "api", "--remove-repository=old"},
			want: ConfigureInput{Project: "sample", Repositories: []RepositoryInput{{Key: "web", Path: "/work/web"}, {Key: "docs", Path: "/work/docs"}}, RemoveRepositories: []string{"api", "old"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			// A guided (stdin) EDIT prompts for nothing: omission is preservation.
			for _, interactive := range []bool{false, true} {
				service := newEditRecordingService(t)
				var output, prompts bytes.Buffer
				var input *strings.Reader
				if interactive {
					input = strings.NewReader("unexpected answer\n")
				}
				args := append([]string{"--json", "project", "configure"}, test.args...)
				var code int
				if interactive {
					code = RunInteractive(context.Background(), args, service, completionProvenance(t), input, &output, &prompts)
				} else {
					code = RunInteractive(context.Background(), args, service, completionProvenance(t), nil, &output, &prompts)
				}
				if code != ExitSuccess || len(service.inputs) != 1 || prompts.Len() != 0 {
					t.Fatalf("interactive=%v exit=%d inputs=%d prompts=%q output=%s", interactive, code, len(service.inputs), prompts.String(), output.String())
				}
				if !reflect.DeepEqual(service.inputs[0], test.want) {
					t.Fatalf("interactive=%v input=%#v want=%#v", interactive, service.inputs[0], test.want)
				}
			}
		})
	}
}

func TestConfigureEditRejectsConflictsBeforeDispatch(t *testing.T) {
	for name, args := range map[string][]string{
		"slug rename":                  {"--project", "sample", "--slug", "renamed"},
		"provider set and remove":      {"--project", "sample", "--work-item-provider", "github", "--remove-work-item-provider"},
		"create-only none alias":       {"--project", "sample", "--work-item-provider", "none"},
		"upsert and remove same key":   {"--project", "sample", "--repository", "api=/work/api", "--remove-repository", "api"},
		"duplicate removal":            {"--project", "sample", "--remove-repository", "api", "--remove-repository", "api"},
		"empty selector":               {"--project", ""},
		"empty removal key":            {"--project", "sample", "--remove-repository", ""},
		"malformed repository":         {"--project", "sample", "--repository", "api"},
		"duplicate selector":           {"--project", "sample", "--project", "other"},
		"create provider removal":      {"--slug", "alpha", "--name", "Alpha", "--repository", "main=/tmp/main", "--remove-work-item-provider"},
		"create repository removal":    {"--slug", "alpha", "--name", "Alpha", "--repository", "main=/tmp/main", "--remove-repository", "main"},
		"guided create with removal":   {"--remove-repository", "main"},
		"remove provider takes no arg": {"--project", "sample", "--remove-work-item-provider", "github"},
	} {
		t.Run(name, func(t *testing.T) {
			assertStrictParserFailure(t, append([]string{"project", "configure"}, args...))
		})
	}
}

// EDIT publication/replay is I132-T02: its inputs fail during presence capture,
// so the service (selector resolution, source and local reads) is never invoked.
func TestConfigureEditRejectsReplayInputsBeforeDispatch(t *testing.T) {
	const id = "123e4567-e89b-42d3-a456-426614174000"
	for name, args := range map[string][]string{
		"preview digest":           {"--project", "sample", "--name", "Renamed", "--preview-digest", "edit-digest"},
		"authorize local":          {"--project", "sample", "--name", "Renamed", "--authorize-local"},
		"digest and authority":     {"--project", "sample", "--name", "Renamed", "--preview-digest", "edit-digest", "--authorize-local"},
		"complete replay tuple":    {"--project", "sample", "--project-id", id, "--preview-digest", "edit-digest", "--authorize-local"},
		"replay-only project id":   {"--project", "sample", "--project-id", id},
		"empty digest is supplied": {"--project", "sample", "--preview-digest="},
		"explicit false authority": {"--project", "sample", "--authorize-local=false"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, interactive := range []bool{false, true} {
				var input *strings.Reader
				if interactive {
					input = strings.NewReader("yes\n")
				}
				var output, prompts bytes.Buffer
				service := newEditRecordingService(t)
				var code int
				full := append([]string{"--json", "project", "configure"}, args...)
				if interactive {
					code = RunInteractive(context.Background(), full, service, completionProvenance(t), input, &output, &prompts)
				} else {
					code = RunInteractive(context.Background(), full, service, completionProvenance(t), nil, &output, &prompts)
				}
				var event completionEvent
				if err := json.Unmarshal(output.Bytes(), &event); err != nil {
					t.Fatalf("interactive=%v: %v: %s", interactive, err, output.String())
				}
				if code != ExitFailure || len(service.inputs) != 0 || service.call != "" || prompts.Len() != 0 || event.Status != completion.ValidationFailure || event.Result != "Project edit publication is not available" || strings.Contains(output.String(), `"edit"`) {
					t.Fatalf("interactive=%v: exit=%d inputs=%d call=%q prompts=%q output=%s", interactive, code, len(service.inputs), service.call, prompts.String(), output.String())
				}
			}
		})
	}
}

func TestConfigureCreateKeepsCompatibleInput(t *testing.T) {
	service := newEditRecordingService(t)
	var output bytes.Buffer
	args := []string{"project", "configure", "--slug", "alpha", "--name", "Alpha", "--repository", "main=/tmp/main", "--work-item-provider", "none"}
	if code := Run(context.Background(), args, service, completionProvenance(t), &output); code != ExitSuccess || len(service.inputs) != 1 {
		t.Fatalf("exit=%d inputs=%d output=%s", code, len(service.inputs), output.String())
	}
	want := ConfigureInput{Slug: "alpha", Name: "Alpha", Repositories: []RepositoryInput{{Key: "main", Path: "/tmp/main"}}}
	if !reflect.DeepEqual(service.inputs[0], want) {
		t.Fatalf("CREATE input=%#v", service.inputs[0])
	}
}

func TestConfigureEditRendersCompletePreviewCanonically(t *testing.T) {
	service := newEditRecordingService(t)
	var structured bytes.Buffer
	if code := Run(context.Background(), []string{"project", "configure", "--project", "sample", "--remove-repository", "api"}, service, completionProvenance(t), &structured); code != ExitSuccess {
		t.Fatalf("exit=%d output=%s", code, structured.String())
	}
	var event struct {
		Status     string                 `json:"status"`
		Result     string                 `json:"result"`
		Provenance map[string]any         `json:"provenance"`
		Edit       projectapp.EditPreview `json:"edit"`
	}
	if err := json.Unmarshal(structured.Bytes(), &event); err != nil {
		t.Fatalf("JSON: %v: %s", err, structured.String())
	}
	if event.Status != "success" || event.Provenance == nil || event.Edit.Mode != "edit" || event.Edit.Digest != "edit-digest" || event.Edit.PortableManifest == "" || len(event.Edit.Effects) != 2 {
		t.Fatalf("edit event = %+v", event)
	}
	if strings.Contains(structured.String(), `"setup"`) || bytes.Count(structured.Bytes(), []byte("\n")) != 1 {
		t.Fatalf("edit JSON is not one canonical edit event: %s", structured.String())
	}
	var human bytes.Buffer
	code := RunInteractive(context.Background(), []string{"project", "configure", "--project", "sample", "--remove-repository", "api"}, service, completionProvenance(t), nil, &human, nil)
	if code != ExitSuccess {
		t.Fatalf("human exit=%d", code)
	}
	for _, expected := range []string{"status: success", "mode: edit", "preview-digest: edit-digest", "repository: api change=removed local=\"/work/api\"", "repository: web change=preserved\n", "effect: portable remove_portable_repository key=api", "effect: local remove_local_binding key=api", "portable-manifest:\n  schemaVersion: 1\n"} {
		if !strings.Contains(human.String(), expected) {
			t.Fatalf("human edit preview missing %q:\n%s", expected, human.String())
		}
	}
}

func TestHelpDocumentsEditPreviewFlags(t *testing.T) {
	var output bytes.Buffer
	Help(&output)
	for _, expected := range []string{"--project <project-uuid-or-slug> previews an edit", "--remove-work-item-provider", "--remove-repository <key>", "already\nconfigured slug or --project-id fails", "Edit rejects --slug (rename)", "--project-id, --preview-digest and\n--authorize-local"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help missing %q", expected)
		}
	}
}
