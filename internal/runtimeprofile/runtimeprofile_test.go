package runtimeprofile

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fakeObserver struct {
	observations map[string]Observation
	calls        []string
}

func (f *fakeObserver) Observe(_ context.Context, runtimeID string) (Observation, error) {
	f.calls = append(f.calls, runtimeID)
	observation, ok := f.observations[runtimeID]
	if !ok {
		return Observation{}, errors.New("unavailable")
	}
	return observation, nil
}

func TestResolveRequiresExactAllowedAvailableCapableChoice(t *testing.T) {
	cfg := testConfiguration()
	observer := &fakeObserver{observations: map[string]Observation{
		"codex":  testObservation("codex", "codex", true),
		"claude": testObservation("claude", "claude", true),
	}}
	result, err := NewResolver(observer).Resolve(context.Background(), cfg, Request{
		ConfigurationRevision: 7, Role: "implementation", Complexity: "high", Capabilities: []string{"go", "repository-write"},
	})
	if err != nil || result.Choice == nil || result.Choice.RuntimeID != "codex" || result.Choice.Model != "local-codex-model" || result.Blocker != nil {
		t.Fatalf("unexpected resolution: result=%+v err=%v", result, err)
	}
	if !reflect.DeepEqual(observer.calls, []string{"claude", "codex"}) {
		t.Fatalf("observer order = %v", observer.calls)
	}
}

func TestResolveBlocksWithoutFallback(t *testing.T) {
	cfg := testConfiguration()
	observer := &fakeObserver{observations: map[string]Observation{
		"codex":  testObservation("codex", "codex", false),
		"claude": testObservation("claude", "claude", true),
	}}
	result, err := NewResolver(observer).Resolve(context.Background(), cfg, Request{
		ConfigurationRevision: 7, Role: "implementation", Complexity: "high", Capabilities: []string{"go", "repository-write"},
	})
	if !errors.Is(err, ErrNoMatch) || result.Blocker == nil || result.Blocker.Code != "no_allowed_match" || result.Choice != nil {
		t.Fatalf("unexpected block: result=%+v err=%v", result, err)
	}
}

func TestResolveDistinguishesDeclaredFromProvenCapability(t *testing.T) {
	cfg := testConfiguration()
	observation := testObservation("codex", "codex", true)
	observation.CapabilityStatus["repository-write"] = CapabilityDeclared
	observer := &fakeObserver{observations: map[string]Observation{"codex": observation, "claude": testObservation("claude", "claude", true)}}
	_, err := NewResolver(observer).Resolve(context.Background(), cfg, Request{ConfigurationRevision: 7, Role: "implementation", Complexity: "high", Capabilities: []string{"go", "repository-write"}})
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveRejectsStaleConfigurationBeforeObservation(t *testing.T) {
	observer := &fakeObserver{}
	result, err := NewResolver(observer).Resolve(context.Background(), testConfiguration(), Request{ConfigurationRevision: 6, Role: "implementation", Complexity: "high", Capabilities: []string{"go"}})
	if !errors.Is(err, ErrStaleConfiguration) || result.Blocker.Code != "stale_configuration" || len(observer.calls) != 0 {
		t.Fatalf("result=%+v err=%v calls=%v", result, err, observer.calls)
	}
}

func TestResolveRejectsAmbiguousChoiceWithoutRanking(t *testing.T) {
	cfg := testConfiguration()
	cfg.Preferences = nil
	for index := range cfg.ModelProfiles {
		cfg.ModelProfiles[index].Complexities = []string{"high"}
	}
	observer := &fakeObserver{observations: map[string]Observation{"codex": testObservation("codex", "codex", true), "claude": testObservation("claude", "claude", true)}}
	result, err := NewResolver(observer).Resolve(context.Background(), cfg, Request{ConfigurationRevision: 7, Role: "implementation", Complexity: "high", Capabilities: []string{"go"}})
	if !errors.Is(err, ErrAmbiguousMatch) || result.Blocker == nil || len(result.Blocker.ProfileIDs) != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestConfigurationClosedSchemaAndSecretReferenceSafety(t *testing.T) {
	cfg := testConfiguration()
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	first, err := Digest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Runtimes[0].CredentialReference = "env:AXIOM_TEST_SECRET"
	second, err := Digest(cfg)
	if err != nil || first == second {
		t.Fatalf("digest must bind reference only: first=%s second=%s err=%v", first, second, err)
	}
	invalid := []Configuration{
		{},
		{FormatVersion: 2, Revision: 1, Runtimes: cfg.Runtimes, ModelProfiles: cfg.ModelProfiles},
		{FormatVersion: 1, Revision: 1, Runtimes: []Runtime{{ID: "other", Adapter: "other", Enabled: true, AllowlistedProfileIDs: []string{"p"}}}, ModelProfiles: []ModelProfile{{ID: "p", RuntimeID: "other", Model: "m", Capabilities: []string{"go"}}}},
	}
	for index, candidate := range invalid {
		if err := Validate(candidate); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("invalid[%d] err=%v", index, err)
		}
	}
}

func testConfiguration() Configuration {
	return Configuration{
		FormatVersion: 1, Revision: 7,
		Runtimes: []Runtime{
			{ID: "codex", Adapter: "codex", Enabled: true, AllowlistedProfileIDs: []string{"codex-high"}, CredentialReference: "keychain:codex"},
			{ID: "claude", Adapter: "claude", Enabled: true, AllowlistedProfileIDs: []string{"claude-low"}},
		},
		ModelProfiles: []ModelProfile{
			{ID: "codex-high", RuntimeID: "codex", Model: "local-codex-model", Capabilities: []string{"go", "repository-write"}, Complexities: []string{"high"}},
			{ID: "claude-low", RuntimeID: "claude", Model: "local-claude-model", Capabilities: []string{"go"}, Complexities: []string{"low"}},
		},
		Preferences: []Preference{{Role: "implementation", Complexity: "high", ModelProfileID: "codex-high"}},
	}
}

func testObservation(runtimeID, adapter string, available bool) Observation {
	return Observation{RuntimeID: runtimeID, Adapter: adapter, Installed: true, Available: available, Version: "test", Revision: 7, ObservedAt: time.Unix(1, 0).UTC(), CapabilityStatus: map[string]CapabilityStatus{"go": CapabilityProven, "repository-write": CapabilityProven}}
}
