package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/completion"
)

type runtimeProfileRecordingService struct {
	recordingService
	calls int
}

func (s *runtimeProfileRecordingService) RuntimeProfileValidate(context.Context) Result {
	s.calls++
	return Result{Status: Succeeded, Category: "validated"}
}

func TestRuntimeProfileValidateDelegatesOptionalOperation(t *testing.T) {
	service := &runtimeProfileRecordingService{}
	var output bytes.Buffer
	if code := Run(context.Background(), []string{"runtime", "profile", "validate"}, service, completionProvenance(t), &output); code != ExitSuccess || service.calls != 1 || service.call != "" {
		t.Fatalf("code=%d calls=%d output=%s", code, service.calls, &output)
	}
	output.Reset()
	if code := Run(context.Background(), []string{"runtime", "profile", "validate"}, &recordingService{}, completionProvenance(t), &output); code != ExitFailure || !strings.Contains(output.String(), "application_unavailable") {
		t.Fatalf("unsupported service: code=%d output=%s", code, &output)
	}
}

func TestRuntimeProfileValidateRejectsAllExtraArguments(t *testing.T) {
	for _, mode := range []string{"--json", "--human"} {
		for _, extra := range [][]string{{"private-input"}, {"--profile", "private-input"}, {"--authorize-local"}, {"--json"}, {"--human"}, {"--"}, {"--help"}, {"-h"}} {
			t.Run(mode+strings.Join(extra, "/"), func(t *testing.T) {
				service := &runtimeProfileRecordingService{}
				args := append([]string{mode, "runtime", "profile", "validate"}, extra...)
				var output, prompts bytes.Buffer
				code := RunInteractive(context.Background(), args, service, completionProvenance(t), strings.NewReader(""), &output, &prompts)
				if code != ExitFailure || service.calls != 0 || service.call != "" || prompts.Len() != 0 || strings.Contains(output.String(), "private-input") {
					t.Fatalf("code=%d calls=%d output=%s prompts=%s", code, service.calls, &output, &prompts)
				}
				if mode == "--json" {
					var event struct {
						Status completion.Status `json:"status"`
					}
					if err := json.Unmarshal(output.Bytes(), &event); err != nil || event.Status != completion.ValidationFailure {
						t.Fatalf("event=%+v err=%v", event, err)
					}
				} else if !strings.Contains(output.String(), "status: validation_failure") {
					t.Fatalf("output=%s", &output)
				}
			})
		}
	}
}

func TestRuntimeProfileValidateHelp(t *testing.T) {
	var output bytes.Buffer
	if Help(&output) != ExitSuccess || !strings.Contains(output.String(), "runtime profile validate") || !strings.Contains(output.String(), "no flags or arguments") {
		t.Fatalf("help=%s", &output)
	}
}
