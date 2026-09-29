package githubissues

import (
	"testing"

	"github.com/rgomids/axiom/internal/workitem"
)

func TestMetadataEffectsKeepGitHubFieldsInAdapterAndSeparateSurfaces(t *testing.T) {
	resolution := workitem.MetadataResolution{Resolved: map[string][]string{
		"classification":  {"bug"},
		"owner":           {"rafael"},
		"delivery_target": {"v1"},
	}}
	effects, err := MetadataEffects(workitem.WorkItemSurface, resolution)
	if err != nil || len(effects) != 3 || effects[0].Field != "labels" || effects[1].Field != "milestone" || effects[2].Field != "assignees" {
		t.Fatalf("work item effects = %#v err=%v", effects, err)
	}
	if _, err := MetadataEffects(workitem.PullRequestSurface, resolution); err == nil {
		t.Fatal("work-item-only intention accepted for pull request")
	}
	preview, err := PrepareMetadataPreview(workitem.WorkItemSurface, "owner/repo#94", testMetadataDigest, resolution)
	if err != nil || preview.Digest == "" {
		t.Fatalf("preview = %#v err=%v", preview, err)
	}
	authority := MetadataAuthority{Target: preview.Target, ObservationDigest: preview.ObservationDigest, PreviewDigest: preview.Digest}
	if !authority.Covers(preview) {
		t.Fatal("exact authority rejected")
	}
	authority.ObservationDigest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if authority.Covers(preview) {
		t.Fatal("stale observation authority accepted")
	}
}

const testMetadataDigest = "0000000000000000000000000000000000000000000000000000000000000000"
