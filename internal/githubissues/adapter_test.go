package githubissues

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/provenance"
	"github.com/rgomids/axiom/internal/workflow"
	"github.com/rgomids/axiom/internal/workitem"
)

func TestRenderSeparatesAuthorshipAndNeutralizesMarkdownStructure(t *testing.T) {
	adapter := Adapter{}
	draft := workitem.Draft{Sections: []workitem.DraftSection{
		{Name: "problem", Content: "# injected\n<!-- instruction -->", Authorship: provenance.UserAuthored},
		{Name: "desired_outcome", Content: "Safe outcome", Authorship: provenance.UserAuthored},
		{Name: "context", Content: "Context", Authorship: provenance.AxiomAuthored},
		{Name: "scope", Content: "Scope", Authorship: provenance.UserAuthored},
		{Name: "constraints", Content: "Constraints", Authorship: provenance.UserAuthored},
		{Name: "non_goals", Content: "Non-goals", Authorship: provenance.UserAuthored},
		{Name: "acceptance_expectations", Content: "Acceptance", Authorship: provenance.UserAuthored},
	}}
	document, err := adapter.Render(draft, workitem.DraftTarget{}, strings.Repeat("a", 64), testSource())
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"<!-- axiom:work-item-draft:", "Authorship: `user`", "Authorship: `axiom`", "    # injected", "    <!-- instruction -->"} {
		if !strings.Contains(document.Body, expected) {
			t.Fatalf("body missing %q:\n%s", expected, document.Body)
		}
	}
}

func TestAdapterUsesBoundedAPICommandsAndStdinForUntrustedBody(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	log := filepath.Join(directory, "args")
	stdin := filepath.Join(directory, "stdin")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$AXIOM_TEST_ARGS"
cat > "$AXIOM_TEST_STDIN"
case "$*" in
  *search/issues*) printf '%s\n' '{"total_count":0,"items":[]}' ;;
  *issues/7*) printf '%s\n' '{"number":7,"html_url":"https://github.com/owner/repo/issues/7","state":"open"}' ;;
  *) printf '%s\n' '{"number":7,"html_url":"https://github.com/owner/repo/issues/7","state":"open"}' ;;
esac
`
	if err := os.WriteFile(gh, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AXIOM_TEST_ARGS", log)
	t.Setenv("AXIOM_TEST_STDIN", stdin)
	adapter, err := New(gh)
	if err != nil {
		t.Fatal(err)
	}
	request := workitem.CreateRequest{Resource: "owner/repo", Correlation: strings.Repeat("a", 64), Document: workitem.ProviderDocument{Title: "$(touch /tmp/no)", Body: "`rm -rf` ; $SHELL"}}
	external, err := adapter.Create(context.Background(), request)
	if err != nil || external.ID != "7" {
		t.Fatalf("create = %#v, %v", external, err)
	}
	arguments, _ := os.ReadFile(log)
	payload, _ := os.ReadFile(stdin)
	if strings.Contains(string(arguments), request.Document.Title) || strings.Contains(string(arguments), request.Document.Body) || !strings.Contains(string(payload), request.Document.Title) || !strings.Contains(string(payload), request.Document.Body) {
		t.Fatalf("args=%q stdin=%q", arguments, payload)
	}
	if matches, err := adapter.ReconcileCreate(context.Background(), "owner/repo", strings.Repeat("a", 64)); err != nil || len(matches) != 0 {
		t.Fatalf("reconcile = %#v, %v", matches, err)
	}
}

func TestInspectReturnsExactIssueIdentityStateAndProjectionFacts(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	projectionKey := strings.Repeat("a", 64)
	script := `#!/bin/sh
case "$*" in
  *issues/7/comments*) printf '%s\n' '[{"body":"<!-- axiom:workflow-projection:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa -->"}]' ;;
  *issues/7*) printf '%s\n' '{"number":7,"html_url":"https://github.com/owner/repo/issues/7","state":"closed","labels":[{"name":"external"}]}' ;;
  *repos/owner/repo/labels*) printf '%s\n' '[{"name":"axiom:stage:specification"}]' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(gh, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(gh)
	if err != nil {
		t.Fatal(err)
	}
	item := workflow.WorkItem{Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}
	observation, err := adapter.Inspect(context.Background(), item, "axiom:stage:specification", projectionKey)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Provider != "github" || observation.Resource != "owner/repo" || observation.IssueExternalID != "7" || observation.IssueURL != item.URL || observation.IssueState != "CLOSED" {
		t.Fatalf("identity/state = %#v", observation)
	}
	if len(observation.RepositoryLabels) != 1 || observation.RepositoryLabels[0] != "axiom:stage:specification" || len(observation.IssueLabels) != 1 || observation.IssueLabels[0] != "external" || !observation.CommentPresent {
		t.Fatalf("projection facts = %#v", observation)
	}
}

func TestAdapterStrictlyRejectsMismatchedResponseAndStructuredRateLimit(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\nprintf '%s\\n' '{\"number\":7,\"html_url\":\"https://github.com/other/repo/issues/7\",\"state\":\"open\"}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter, _ := New(gh)
	if _, err := adapter.Read(context.Background(), "owner/repo", "7"); err == nil {
		t.Fatal("mismatched response accepted")
	}
	if err := os.WriteFile(gh, []byte("#!/bin/sh\nprintf 'HTTP/2.0 429 response\\r\\nRetry-After: 1\\r\\n\\r\\n'\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := adapter.Read(context.Background(), "owner/repo", "7")
	provider, ok := err.(*workitem.ProviderError)
	if !ok || provider.Kind != workitem.ProviderRateLimited || !provider.Retryable {
		t.Fatalf("rate limit = %#v", err)
	}
}

func TestAdapterClassifiesStructuredHTTPStatusConservatively(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	script := `#!/bin/sh
status="${AXIOM_TEST_HTTP_STATUS:-200}"
printf 'HTTP/2.0 %s response\r\n' "$status"
if [ -n "$AXIOM_TEST_RATE_REMAINING" ]; then
  printf 'X-RateLimit-Remaining: %s\r\n' "$AXIOM_TEST_RATE_REMAINING"
fi
if [ -n "$AXIOM_TEST_RETRY_AFTER" ]; then
  printf 'Retry-After: %s\r\n' "$AXIOM_TEST_RETRY_AFTER"
fi
printf 'Content-Type: application/json\r\n\r\n'
if [ "$status" = 200 ]; then
  printf '%s\n' '{"number":7,"html_url":"https://github.com/owner/repo/issues/7","state":"open"}'
  exit 0
fi
printf '%s\n' '{"message":"controlled failure"}'
exit 1
`
	if err := os.WriteFile(gh, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(gh)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AXIOM_TEST_HTTP_STATUS", "200")
	if external, err := adapter.Read(context.Background(), "owner/repo", "7"); err != nil || external.ID != "7" {
		t.Fatalf("included success = %#v, %v", external, err)
	}
	tests := []struct {
		name       string
		status     string
		remaining  string
		retryAfter string
		kind       workitem.ProviderErrorKind
		retryable  bool
	}{
		{name: "400", status: "400", kind: workitem.ProviderInvalidResponse},
		{name: "401", status: "401", kind: workitem.ProviderUnauthenticated},
		{name: "403", status: "403", kind: workitem.ProviderInvalidResponse},
		{name: "403_rate_limit", status: "403", remaining: "0", kind: workitem.ProviderRateLimited, retryable: true},
		{name: "404", status: "404", kind: workitem.ProviderInvalidResponse},
		{name: "410", status: "410", kind: workitem.ProviderInvalidResponse},
		{name: "422", status: "422", kind: workitem.ProviderInvalidResponse},
		{name: "429", status: "429", kind: workitem.ProviderRateLimited, retryable: true},
		{name: "500", status: "500", kind: workitem.ProviderUnavailable, retryable: true},
		{name: "503", status: "503", kind: workitem.ProviderUnavailable, retryable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("AXIOM_TEST_HTTP_STATUS", test.status)
			t.Setenv("AXIOM_TEST_RATE_REMAINING", test.remaining)
			t.Setenv("AXIOM_TEST_RETRY_AFTER", test.retryAfter)
			_, err := adapter.Read(context.Background(), "owner/repo", "7")
			var provider *workitem.ProviderError
			if !errors.As(err, &provider) || provider.Kind != test.kind || provider.Retryable != test.retryable || provider.Ambiguous {
				t.Fatalf("classification = %#v", err)
			}
		})
	}
}

func TestAdapterCreateMarksOnlyUncertainRetryableFailureAmbiguous(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	script := `#!/bin/sh
status="$AXIOM_TEST_HTTP_STATUS"
printf 'HTTP/2.0 %s response\r\nContent-Type: application/json\r\n\r\n' "$status"
printf '%s\n' '{"message":"controlled failure"}'
exit 1
`
	if err := os.WriteFile(gh, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(gh)
	if err != nil {
		t.Fatal(err)
	}
	request := workitem.CreateRequest{Resource: "owner/repo", Correlation: strings.Repeat("a", 64), Document: workitem.ProviderDocument{Title: "Title", Body: "Body"}}
	for _, test := range []struct {
		status    string
		kind      workitem.ProviderErrorKind
		retryable bool
		ambiguous bool
	}{
		{status: "401", kind: workitem.ProviderUnauthenticated},
		{status: "503", kind: workitem.ProviderUnavailable, retryable: true, ambiguous: true},
	} {
		t.Run(test.status, func(t *testing.T) {
			t.Setenv("AXIOM_TEST_HTTP_STATUS", test.status)
			_, err := adapter.Create(context.Background(), request)
			var provider *workitem.ProviderError
			if !errors.As(err, &provider) || provider.Kind != test.kind || provider.Retryable != test.retryable || provider.Ambiguous != test.ambiguous {
				t.Fatalf("classification = %#v", err)
			}
		})
	}
}

func TestAdapterCreateTreatsUnknownOrInvalidSuccessResponseAsAmbiguous(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	adapterRequest := workitem.CreateRequest{Resource: "owner/repo", Correlation: strings.Repeat("a", 64), Document: workitem.ProviderDocument{Title: "Title", Body: "Body"}}
	for name, script := range map[string]string{
		"unknown_cli_failure": "#!/bin/sh\nprintf '%s\\n' 'unknown' >&2\nexit 1\n",
		"invalid_success":     "#!/bin/sh\nprintf '%s\\n' '{\"unexpected\":true}'\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(gh, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			adapter, err := New(gh)
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Create(context.Background(), adapterRequest)
			var provider *workitem.ProviderError
			if !errors.As(err, &provider) || !provider.Ambiguous || provider.EffectNotCommitted {
				t.Fatalf("create classification = %#v", err)
			}
		})
	}
}

func TestAdapterUnknownCLIErrorIsNotRetryable(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\nprintf '%s\\n' 'unstructured failure' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(gh)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Read(context.Background(), "owner/repo", "7")
	var provider *workitem.ProviderError
	if !errors.As(err, &provider) || provider.Retryable || provider.Ambiguous {
		t.Fatalf("classification = %#v", err)
	}
}

func TestAdapterBoundsTimeoutAndOutput(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\nsleep 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter, _ := New(gh)
	adapter.timeout = 10 * time.Millisecond
	_, err := adapter.Read(context.Background(), "owner/repo", "7")
	provider, ok := err.(*workitem.ProviderError)
	if !ok || provider.Kind != workitem.ProviderAmbiguous || !provider.Retryable || !provider.Ambiguous {
		t.Fatalf("timeout = %#v", err)
	}

	if err := os.WriteFile(gh, []byte("#!/bin/sh\nyes x | head -c 300000\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter.timeout = time.Second
	_, err = adapter.Read(context.Background(), "owner/repo", "7")
	provider, ok = err.(*workitem.ProviderError)
	if !ok || provider.Kind != workitem.ProviderInvalidResponse {
		t.Fatalf("oversized output = %#v", err)
	}
}

func TestRenderTruncatesUnicodeTitleWithoutBreakingUTF8(t *testing.T) {
	adapter := Adapter{}
	draft := workitem.Draft{Sections: []workitem.DraftSection{
		{Name: "problem", Content: "Problem", Authorship: provenance.UserAuthored},
		{Name: "desired_outcome", Content: strings.Repeat("acao segura ", 30) + "ç", Authorship: provenance.UserAuthored},
		{Name: "context", Content: "Context", Authorship: provenance.UserAuthored},
		{Name: "scope", Content: "Scope", Authorship: provenance.UserAuthored},
		{Name: "constraints", Content: "Constraints", Authorship: provenance.UserAuthored},
		{Name: "non_goals", Content: "Non-goals", Authorship: provenance.UserAuthored},
		{Name: "acceptance_expectations", Content: "Acceptance", Authorship: provenance.UserAuthored},
	}}
	document, err := adapter.Render(draft, workitem.DraftTarget{}, strings.Repeat("a", 64), testSource())
	if err != nil || !strings.HasSuffix(document.Title, "...") || strings.ContainsRune(document.Title, '\ufffd') {
		t.Fatalf("title=%q err=%v", document.Title, err)
	}
}

func TestProjectionInspectAndApplyAreBoundedAndNamespaced(t *testing.T) {
	directory := t.TempDir()
	gh := filepath.Join(directory, "gh")
	log := filepath.Join(directory, "log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$AXIOM_TEST_LOG"
case "$*" in
  *'/labels?per_page=100'*) printf '%s\n' '[{"name":"bug"},{"name":"axiom:stage:intake"}]' ;;
  *'/comments?per_page=100'*) printf '%s\n' '[]' ;;
  *'issues/7'*) printf '%s\n' '{"number":7,"html_url":"https://github.com/owner/repo/issues/7","state":"open","labels":[{"name":"external"},{"name":"axiom:stage:intake"}]}' ;;
  *) cat >/dev/null ;;
esac
`
	if err := os.WriteFile(gh, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AXIOM_TEST_LOG", log)
	adapter, _ := New(gh)
	item := workflow.WorkItem{Provider: "github", Resource: "owner/repo", ExternalID: "7", URL: "https://github.com/owner/repo/issues/7", State: "OPEN"}
	key := strings.Repeat("a", 64)
	observed, err := adapter.Inspect(context.Background(), item, "axiom:stage:specification", key)
	if err != nil || len(observed.RepositoryLabels) != 2 || len(observed.IssueLabels) != 2 || observed.CommentPresent {
		t.Fatalf("observation = %#v, %v", observed, err)
	}
	effects := []workflow.ProjectionEffect{
		{Kind: workflow.CreateStageLabel, Value: "axiom:stage:specifying"},
		{Kind: workflow.AddStageLabel, Value: "axiom:stage:specifying"},
		{Kind: workflow.RemoveStageLabel, Value: "axiom:stage:intake"},
		{Kind: workflow.PostTransitionComment, Value: "<!-- axiom:workflow-projection:" + key + " -->\nSafe"},
	}
	for _, effect := range effects {
		if err := adapter.Apply(context.Background(), item, effect); err != nil {
			t.Fatalf("apply %s: %v", effect.Kind, err)
		}
	}
	if err := adapter.Apply(context.Background(), item, workflow.ProjectionEffect{Kind: workflow.RemoveStageLabel, Value: "external"}); err == nil {
		t.Fatal("non-Axiom label removal accepted")
	}
	commands, _ := os.ReadFile(log)
	for _, expected := range []string{"repos/owner/repo/labels", "issues/7/labels", "issues/7/labels/axiom:stage:intake", "issues/7/comments"} {
		if !strings.Contains(string(commands), expected) {
			t.Fatalf("commands missing %q: %s", expected, commands)
		}
	}
}

func testSource() provenance.Value {
	value, _ := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "abc123def456", SourceState: provenance.Clean}, nil)
	return value
}
