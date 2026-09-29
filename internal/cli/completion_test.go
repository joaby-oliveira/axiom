package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/completion"
	"github.com/rgomids/axiom/internal/provenance"
	"github.com/rgomids/axiom/internal/workitem"
)

func TestCompletionGoldenMatrix(t *testing.T) {
	provenanceValue := completionProvenance(t)
	tests := []struct {
		status     completion.Status
		references []string
		next       string
		wantCode   int
	}{
		{completion.Success, nil, "", ExitSuccess},
		{completion.Failure, nil, "", ExitFailure},
		{completion.ValidationFailure, nil, "", ExitFailure},
		{completion.DeniedAuthority, nil, "", ExitFailure},
		{completion.Partial, []string{"provider:github#7"}, "Retry local persistence", ExitFailure},
		{completion.Interrupted, []string{"execution:123"}, "Resume execution", ExitCancelled},
		{completion.RetryableFailure, nil, "Retry after dependency recovery", ExitFailure},
	}

	for _, test := range tests {
		t.Run(string(test.status), func(t *testing.T) {
			result := canonicalResult(t, test.status, test.references, test.next, provenanceValue)
			var human, structured bytes.Buffer
			if code := WriteCompletion(&human, CompletionHuman, result); code != test.wantCode {
				t.Fatalf("human exit = %d, want %d", code, test.wantCode)
			}
			if code := WriteCompletion(&structured, CompletionJSON, result); code != test.wantCode {
				t.Fatalf("JSON exit = %d, want %d", code, test.wantCode)
			}

			wantHuman := "status: " + string(test.status) + "\nresult: operation completed\n"
			for _, reference := range test.references {
				wantHuman += "reference: " + reference + "\n"
			}
			if test.next != "" {
				wantHuman += "next: " + test.next + "\n"
			}
			wantHuman += "provenance: Axiom development revision=abc123def456 source=clean\n"
			if human.String() != wantHuman {
				t.Fatalf("human output:\n%s\nwant:\n%s", human.String(), wantHuman)
			}

			wantJSON := `{"status":"` + string(test.status) + `","result":"operation completed"`
			if len(test.references) > 0 {
				wantJSON += `,"references":["` + strings.Join(test.references, `","`) + `"]`
			}
			if test.next != "" {
				wantJSON += `,"next":"` + test.next + `"`
			}
			wantJSON += `,"provenance":{"product":"Axiom","version":"development","revision":"abc123def456","sourceState":"clean"}}` + "\n"
			if structured.String() != wantJSON {
				t.Fatalf("JSON output:\n%s\nwant:\n%s", structured.String(), wantJSON)
			}
		})
	}
}

func TestRendererFailurePreservesConfirmedEffect(t *testing.T) {
	tests := []struct {
		status completion.Status
		next   string
	}{
		{completion.Success, ""},
		{completion.Partial, "Retry local persistence"},
	}
	for _, test := range tests {
		result := canonicalResult(t, test.status, []string{"provider:github#7"}, test.next, completionProvenance(t))
		before := result.References()
		for _, writer := range []interface{ Write([]byte) (int, error) }{failingWriter{}, shortWriter{}} {
			if code := WriteCompletion(writer, CompletionJSON, result); code != ExitFailure {
				t.Fatalf("%s exit = %d", test.status, code)
			}
		}
		if result.Status() != test.status || !equalStrings(result.References(), before) {
			t.Fatalf("renderer changed confirmed result: status=%s references=%v", result.Status(), result.References())
		}
	}
}

func TestCompletionOutputIsBounded(t *testing.T) {
	result := canonicalResult(t, completion.Success, []string{"project:123"}, "", completionProvenance(t))
	for _, format := range []CompletionFormat{CompletionHuman, CompletionJSON} {
		var output bytes.Buffer
		if code := WriteCompletion(&output, format, result); code != ExitSuccess {
			t.Fatalf("%s exit = %d", format, code)
		}
		if output.Len() == 0 || output.Len() > MaxCompletionOutputBytes {
			t.Fatalf("%s output bytes = %d", format, output.Len())
		}
	}
}

func TestCompletionPreservesStableDetailReferenceAcrossRenderers(t *testing.T) {
	statement, err := provenance.NewText("diagnostic available", provenance.AxiomAuthored)
	if err != nil {
		t.Fatal(err)
	}
	result, err := completion.New(completion.Facts{Completed: true}, statement, nil, provenance.Text{}, "artifact:123e4567-e89b-42d3-a456-426614174000", completionProvenance(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		format CompletionFormat
		want   string
	}{{CompletionJSON, `"details":"artifact:123e4567-e89b-42d3-a456-426614174000"`}, {CompletionHuman, "details: artifact:123e4567-e89b-42d3-a456-426614174000"}} {
		var output bytes.Buffer
		if code := WriteCompletion(&output, test.format, result); code != ExitSuccess || !strings.Contains(output.String(), test.want) {
			t.Fatalf("%s code=%d output=%q", test.format, code, output.String())
		}
	}
}

func TestWorkItemPreviewWithWorstCaseEscapingRemainsBounded(t *testing.T) {
	content := strings.Repeat(`"\\<>`, 4*1024)
	preview := &workitem.DraftPreview{
		Draft:            workitem.Draft{Sections: []workitem.DraftSection{{Name: "problem", Content: content, Authorship: provenance.UserAuthored}}},
		ProviderDocument: workitem.ProviderDocument{Title: "Axiom draft", Body: content},
		Digest:           strings.Repeat("a", 64),
	}
	result := canonicalResult(t, completion.Success, nil, "Review preview", completionProvenance(t))
	for _, mode := range []outputMode{humanOutput, jsonOutput} {
		var output bytes.Buffer
		if code := emitWorkItemCompletion(&output, mode, result, Result{Draft: preview}); code != ExitSuccess {
			t.Fatalf("%s exit=%d bytes=%d", mode, code, output.Len())
		}
		if output.Len() == 0 || output.Len() > maxWorkItemPreviewOutputBytes {
			t.Fatalf("%s bytes=%d", mode, output.Len())
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("controlled write failure") }

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) { return len(value) - 1, nil }

func canonicalResult(t *testing.T, status completion.Status, references []string, next string, source provenance.Value) completion.Result {
	t.Helper()
	statement, err := provenance.NewText("operation completed", provenance.AxiomAuthored)
	if err != nil {
		t.Fatal(err)
	}
	var nextText provenance.Text
	if next != "" {
		nextText, err = provenance.NewText(next, provenance.AxiomAuthored)
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := completion.New(factsForStatus(status), statement, references, nextText, "", source)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func factsForStatus(status completion.Status) completion.Facts {
	switch status {
	case completion.Success:
		return completion.Facts{Completed: true}
	case completion.Failure:
		return completion.Facts{Failed: true}
	case completion.ValidationFailure:
		return completion.Facts{ValidationFailed: true}
	case completion.DeniedAuthority:
		return completion.Facts{AuthorityDenied: true}
	case completion.Partial:
		return completion.Facts{RequestedEffectConfirmed: true, SecondaryFailure: true}
	case completion.Interrupted:
		return completion.Facts{WasInterrupted: true}
	case completion.RetryableFailure:
		return completion.Facts{RetrySafeFailure: true}
	default:
		return completion.Facts{}
	}
}

func completionProvenance(t *testing.T) provenance.Value {
	t.Helper()
	value, err := provenance.FromBuild(provenance.Build{Version: provenance.Development, Revision: "abc123def456", SourceState: provenance.Clean}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func equalStrings(left, right []string) bool {
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
