package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/completion"
)

func TestStrictSelectorLongFormsBeforeDispatch(t *testing.T) {
	surfaces := []struct {
		command string
		flags   string
	}{
		{"project show", "selector"},
		{"project resolve", "selector"},
		{"project configure", "project-id slug name work-item-provider preview-digest authorize-local"},
		{"work-item create", "project repository work-item provider-repository"},
		{"work-item select", "project repository work-item provider-repository number"},
		{"work-item show", "project repository work-item provider-repository number"},
		{"work-item comment", "project repository work-item provider-repository number"},
		{"work-item complete", "project repository work-item provider-repository number"},
		{"workflow start", "project repository work-item execution number runtime"},
		{"workflow advance", "project repository work-item execution number"},
		{"workflow resume", "project repository work-item execution number"},
		{"workflow status", "project repository work-item execution number"},
		{"workflow evidence", "project repository work-item execution number"},
		{"workflow reconcile", "project repository work-item execution number"},
	}
	for _, surface := range surfaces {
		for _, name := range strings.Fields(surface.flags) {
			first, second := "alpha", "beta"
			if name == "number" {
				first, second = "7", "8"
			}
			if name == "authorize-local" {
				first, second = "true", "false"
			}
			if name == "runtime" {
				first, second = "claude", "codex"
			}
			for _, suffix := range [][]string{
				{"-" + name, first},
				{"-" + name + "=" + first},
				{"-" + name, first, "-" + name, second},
				{"--" + name, first, "-" + name, second},
				{"--" + name, first, "--" + name, second},
				{"--" + name + "=" + first, "--" + name + "=" + second},
			} {
				args := append(strings.Fields(surface.command), suffix...)
				t.Run(strings.Join(args, " "), func(t *testing.T) {
					assertStrictParserFailure(t, args)
				})
			}
		}
	}
	for _, suffix := range [][]string{
		{"-repository", "main=/tmp/main"},
		{"-repository=main=/tmp/main"},
		{"--repository", "main=/tmp/main", "-repository", "other=/tmp/other"},
		{"--unknown", "value"},
	} {
		t.Run("configure "+strings.Join(suffix, " "), func(t *testing.T) {
			assertStrictParserFailure(t, append([]string{"project", "configure"}, suffix...))
		})
	}
}

func assertStrictParserFailure(t *testing.T, args []string) {
	t.Helper()
	for _, interactive := range []bool{false, true} {
		var input io.Reader
		if interactive {
			input = strings.NewReader("")
		}
		var output, prompts bytes.Buffer
		service := &recordingService{}
		code := RunInteractive(context.Background(), append([]string{"--json"}, args...), service, completionProvenance(t), input, &output, &prompts)
		var event completionEvent
		if err := json.Unmarshal(output.Bytes(), &event); err != nil {
			t.Fatalf("interactive=%v: output=%s err=%v", interactive, output.String(), err)
		}
		if code != ExitFailure || service.call != "" || prompts.Len() != 0 || event.Status != completion.ValidationFailure || event.Result != "Explicit selector input is invalid" || event.Next != "Remove unknown, duplicate, or conflicting inputs and retry" {
			t.Fatalf("interactive=%v: exit=%d call=%q prompts=%q event=%+v", interactive, code, service.call, prompts.String(), event)
		}
	}
}

func TestStrictProjectConfigurePreservesRepeatableRepository(t *testing.T) {
	values, ok := flags(configureAction, []string{"--slug", "alpha", "--name", "Alpha", "--repository", "main=/tmp/main", "--repository=other=/tmp/other"})
	if !ok || len(values.repositories) != 2 || values.repositories[0] != "main=/tmp/main" || values.repositories[1] != "other=/tmp/other" {
		t.Fatalf("repeatable repositories: ok=%v values=%+v", ok, values)
	}
}

func TestWorkflowStartRuntimeSelector(t *testing.T) {
	base := []string{"workflow", "start", "--project", "sample", "--repository", "main", "--number", "7"}
	for name, test := range map[string]struct {
		args []string
		want string
	}{
		"claude":             {[]string{"--runtime", "claude"}, "workflow-start:sample:main:7:claude"},
		"codex":              {[]string{"--runtime=codex"}, "workflow-start:sample:main:7:codex"},
		"historical default": {nil, "workflow-start:sample:main:7:codex"},
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			service := &recordingService{}
			if code := Run(context.Background(), append(append([]string{}, base...), test.args...), service, completionProvenance(t), &output); code != ExitSuccess || service.call != test.want {
				t.Fatalf("exit=%d call=%q output=%s", code, service.call, output.String())
			}
		})
	}
	for _, invalid := range [][]string{{"--runtime", "gemini"}, {"--runtime", ""}, {"--runtime="}, {"--runtime", "Claude"}, {"--runtime", "codex "}} {
		t.Run("invalid "+strings.Join(invalid, " "), func(t *testing.T) {
			assertStrictParserFailure(t, append(append([]string{}, base...), invalid...))
		})
	}
	// Only start selects a Runtime; later operations cannot name or switch it.
	for _, command := range []string{"advance", "fact", "resume", "status", "evidence", "reconcile"} {
		t.Run(command+" rejects --runtime", func(t *testing.T) {
			assertStrictParserFailure(t, []string{"workflow", command, "--project", "sample", "--repository", "main", "--number", "7", "--execution", "018f4a44-7c31-7dd4-9d00-111111111111", "--expected-revision", "1", "--runtime", "claude"})
		})
	}
}
