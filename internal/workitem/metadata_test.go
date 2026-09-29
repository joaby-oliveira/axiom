package workitem

import "testing"

func TestMetadataPolicyClosedSchemaAndResolutionPrecedence(t *testing.T) {
	policy, err := DecodeMetadataPolicy([]byte(`kind: work-item-metadata
policyVersion: 1
workItem:
  classification:
    requirement: required
    default: bug
  owner:
    requirement: required
    reference: primary-owner
  tracking_state:
    requirement: optional
pullRequest:
  reviewers:
    requirement: required
    default: [alice, bob]
`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := ResolveMetadata(WorkItemSurface, policy,
		map[string][]string{"classification": {"security"}},
		map[string][]string{"primary-owner": {"rafael"}},
		map[string]bool{"classification": true, "owner": true, "tracking_state": false})
	if err != nil || len(result.Prompts) != 0 || result.Resolved["classification"][0] != "security" || result.Sources["classification"] != "explicit" || result.Resolved["owner"][0] != "rafael" || len(result.Unsupported) != 1 {
		t.Fatalf("resolution = %#v err=%v", result, err)
	}

	for name, document := range map[string]string{
		"unknown":  "kind: work-item-metadata\npolicyVersion: 1\nunknown: true\n",
		"version":  "kind: work-item-metadata\npolicyVersion: 2\n",
		"bad type": "kind: work-item-metadata\npolicyVersion: one\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeMetadataPolicy([]byte(document)); err == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
}

func TestMetadataMissingOnlyAndDuplicateKind(t *testing.T) {
	policy, err := DecodeMetadataPolicy([]byte("kind: work-item-metadata\npolicyVersion: 1\nworkItem:\n  owner:\n    requirement: required\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := ResolveMetadata(WorkItemSurface, policy, nil, nil, map[string]bool{"owner": true})
	if err != nil || len(result.Prompts) != 1 || result.Prompts[0] != "owner" {
		t.Fatalf("missing-only result = %#v err=%v", result, err)
	}
	document := []byte("kind: work-item-metadata\npolicyVersion: 1\n")
	if _, err := DecodeMetadataPolicies([][]byte{document, document}); err == nil {
		t.Fatal("duplicate recognized policy accepted")
	}
}

func TestMetadataAbsentEmptyCapabilityAndSurfaceMatrix(t *testing.T) {
	policy, err := DecodeMetadataPolicies(nil)
	if err != nil || policy != nil {
		t.Fatalf("absent policy = %#v err=%v", policy, err)
	}
	if _, err := DecodeMetadataPolicies([][]byte{{}}); err == nil {
		t.Fatal("empty referenced policy accepted")
	}

	configured := MetadataPolicy{
		WorkItem: map[string]MetadataRule{
			"classification":  {Requirement: MetadataRequired, Default: []string{"bug"}},
			"delivery_target": {Requirement: MetadataOptional, Default: []string{"v1"}},
		},
		PullRequest: map[string]MetadataRule{
			"reviewers": {Requirement: MetadataRequired},
		},
	}
	resolved, err := ResolveMetadata(WorkItemSurface, configured, nil, nil, map[string]bool{"classification": true, "delivery_target": false})
	if err != nil || len(resolved.Prompts) != 0 || len(resolved.Unsupported) != 1 || resolved.Resolved["classification"][0] != "bug" {
		t.Fatalf("work item matrix = %#v err=%v", resolved, err)
	}
	missing, err := ResolveMetadata(PullRequestSurface, configured, nil, nil, map[string]bool{"reviewers": true})
	if err != nil || len(missing.Prompts) != 1 || missing.Prompts[0] != "reviewers" {
		t.Fatalf("pull request missing = %#v err=%v", missing, err)
	}
	if _, err := ResolveMetadata(WorkItemSurface, configured, nil, nil, map[string]bool{"classification": false}); err == nil {
		t.Fatal("unsupported mandatory capability accepted")
	}
	if _, err := ResolveMetadata(PullRequestSurface, configured, map[string][]string{"delivery_target": {"v1"}}, nil, map[string]bool{"delivery_target": true}); err == nil {
		t.Fatal("work-item intention accepted on pull request")
	}
}
