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
		{"workflow start", "project repository work-item execution number"},
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
