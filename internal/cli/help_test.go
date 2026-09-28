package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpMapsEveryCodexSkillToPublicAxiomExecutable(t *testing.T) {
	var output bytes.Buffer
	if code := Help(&output); code != ExitSuccess {
		t.Fatalf("help code = %d", code)
	}
	for _, expected := range []string{"$axiom-project-configure", "$axiom-project-show", "$axiom-work-item-create", "$axiom-work-item-run", "$axiom-work-item-status", "axiom --json", "axiom [--human|--json] <command>", "--authorize-external"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help missing %q: %s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "lingo ") {
		t.Fatalf("help still presents lingo as the public executable: %s", output.String())
	}
}
